# Design: Configurable Per-Agent Models

## Technical Approach

`state` grows one additive, typed key. `packaging.Render` gains a third parameter, validates and resolves it to a `platform+agent → {default, escalation}` table, then substitutes qualified placeholders into all 21 surfaces *before* the Codex TOML conversion. `renderCodexAgentConfig` stops dropping `model`. Absent config resolves to a built-in table holding today's literals, so unset config renders byte-identical output.

Pattern adopted from gentle-ai (`internal/state/state.go` `ClaudePhaseAssignmentState`; `internal/agents/claude/adapter.go` `{{CLAUDE_MODEL}}`): struct-per-assignment with `omitempty`, sentinel placeholders stamped at render time, **no model-name allowlist**. `Effort` is not adopted — drup has no effort surface.

## Architecture Decisions

| # | Decision | Rejected | Rationale |
|---|---|---|---|
| 1 | Nested `map[platform]map[agent]ModelAssignment`, reserved agent key `"*"` as platform-wide fallback | Flat dotted keys; `Effort` field | Mirrors the two-level semantics; `"*"` serves the primary story ("move everything to my provider") in one line |
| 2 | **Qualified** placeholders `{{MODEL_DEFAULT:drup-rector}}` / `{{MODEL_ESCALATION:drup-rector}}` everywhere | Bare `{{MODEL_DEFAULT}}` resolved from filename | SKILL.md rosters name all 6 agents in one file, so bare form needs a second grammar. One grammar = one resolver, and unknown agent keys are detectable |
| 3 | No model-string allowlist; validate *structure* only | Allowlist of known IDs | Allowlists age badly against new releases; gentle-ai sets the precedent. Structural validation still blocks the real failure (asset corruption) |
| 4 | Validate + resolve inside `Render`, not `state.Load` | Parse-time validation | `Load` is on the path of every command; a bad key must not brick `drup uninstall`. `installAgents` already isolates per-agent render failures, so one bad platform fails alone |
| 5 | Substitute → then TOML convert. Converter copies the resolved `model = "…"` line verbatim, required, same quoting check as `description` | Inject model during TOML assembly | Keeps the converter a pure Markdown→TOML mapper; one substitution pass covers all platforms |
| 6 | One resolved string feeds frontmatter **and** prose | Separate alias vocabulary for prose | Two vocabularies are exactly how the current OpenCode contradiction (`qwen3-30b` vs "Default: haiku") arose |
| 7 | `drup-validator` default resolves to the **strong** tier, not haiku | Keep its "Default model: haiku" prose | Its prose contradicts `SKILL.md` line 34, which argues the gate must never be cheap. Uniform `{default, escalation}` shape then needs no special case |
| 8 | Legacy `model_overrides` read, warned once, then dropped on next `Save` | Silent drop; migration | Warning needs the field present; nil-ing before `Save` retires the dead key without a migration path |

## Data Flow

    ~/.config/drup/state.json
      │ state.Load()            tolerant — unknown/legacy keys never error
      ▼
    State.ModelAssignments      map[platform]map[agent]ModelAssignment
      │ RunInstall / RunSync    (RunInstall must Load *before* installAgents)
      ▼
    installAgents(agents, binaryPath, action, assignments)
      │ per-agent isolated failure
      ▼
    packaging.Render(platform, binaryPath, assignments)
      ├─ ValidateAssignments(platform, …)         fail closed
      ├─ resolve: agent → platform "*" → builtin[platform][agent]
      ├─ walk templates/<platform>/**
      │    ├─ {{BINARY_PATH}}, {{SKILL_PATH}}     (existing)
      │    └─ {{MODEL_DEFAULT:<agent>}} / {{MODEL_ESCALATION:<agent>}}
      ├─ assert no residual "{{MODEL_"            fail closed
      └─ codex: renderCodexAgentConfig → .toml    now carries model
      ▼
    installer.Install → ~/.claude/agents/*.md | opencode/agent/*.md | ~/.codex/agents/*.toml

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/state/state.go` | Modify | Add `ModelAssignment` type + `ModelAssignments`; keep `ModelOverrides` as deprecated read-and-drop |
| `internal/packaging/packaging.go` | Modify | 3rd `Render` param; `ValidateAssignments`, `resolve`, `substituteModels`, residual-placeholder guard; emit `model` in TOML |
| `internal/packaging/models.go` | Create | Built-in per-platform default table + `AgentNames(platform)` |
| `internal/packaging/templates/**` (21) | Modify | 18 agent frontmatter + "Default model:" prose; 3 SKILL.md rosters + retry prose |
| `internal/app/commands.go` | Modify | Load state before `installAgents`; pass assignments; legacy warning |
| `internal/packaging/packaging_test.go`, `internal/app/commands_test.go` | Modify | Existing `Render(p, path)` call sites take `nil` |
| `docs/model-configuration.md` | Create | Shape, precedence, examples, downgrade caveat |

## Interfaces / Contracts

```go
// internal/state
type ModelAssignment struct {
    Default    string `json:"default,omitempty"`
    Escalation string `json:"escalation,omitempty"`
}
// platform → agent ("*" = platform-wide fallback) → assignment
ModelAssignments map[string]map[string]ModelAssignment `json:"model_assignments,omitempty"`

// internal/packaging
func Render(platform, binaryPath string, assignments map[string]map[string]state.ModelAssignment) (map[string]string, error)
```

`Assignments` is passed as a plain map to avoid a `packaging → state` import cycle if one appears; if it does, mirror the struct in `packaging` as gentle-ai does.

Structural validation rejects: unknown platform key, unknown agent key (not in `AgentNames(platform) ∪ {"*"}`), any value containing a newline, `"`, `\`, `#`, or leading/trailing whitespace. Empty string = "unset", falls through precedence.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit `state` | Round-trip `ModelAssignments`; unknown JSON key tolerated; legacy `model_overrides` parsed then dropped | temp-dir `configDir` stub, existing pattern |
| Unit `packaging` | Precedence (agent > `"*"` > builtin); unknown platform/agent rejected; injection strings rejected; **nil assignments ⇒ output equals current literals** | table-driven, golden string per agent |
| Unit `packaging` | No `{{MODEL_` survives any render, all 3 platforms × all files | assert over full `files` map |
| Integration Codex | `.toml` contains `model = "<resolved>"`; quoting check rejects unquotable model | extend `TestRenderCodexAgentConfig_*` |
| Integration app | `installAgents` with configured assignments writes substituted files; one bad platform fails alone | extend `commands_test.go:1776` |
| Consistency | Frontmatter model == prose model for every agent file | regex both, compare |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. The one adversarial surface is config-string injection into generated YAML/TOML, handled by structural validation (see Risks) and covered by a RED test.

## Migration / Rollout

No migration. `model_assignments` is additive; older builds ignore it on read. Single slice — the Codex `model` fix is a prerequisite for Codex config to have any effect, so splitting it would ship a knob that does nothing.

## Risks

| Risk | Mitigation |
|---|---|
| Config string corrupts YAML/TOML asset | Structural validation before substitution; Codex quoting check; RED test per rejected character |
| Partial substitution → contradictory assets | Residual `{{MODEL_` guard fails the render, plus frontmatter-vs-prose consistency test |
| Prose readability regression (`Default model: claude-haiku-4-5-20251001`) | Accepted: correctness over brevity; decision 6 |
| **Downgrade data loss** — an older drup `Save()` rewrites `state.json` without `model_assignments` | Document "reconfigure after downgrade". Optional hardening: add an unknown-key passthrough now so the *next* additive key is skew-safe. Not required for this slice |
| `Render` signature churn across ~15 test call sites | Explicit param chosen over variadic options on purpose: a variadic hides a semantically required input and lets new call sites silently render default models |

## Open Questions

- [ ] Decision 7 rewrites `drup-validator`'s "Default model: haiku" prose as stale. Confirm that is intended rather than a de-facto downgrade of the gate.
- [ ] Is the `"*"` platform-wide fallback in slice 1, or deferred? It is the cheapest path to the "different provider" story but adds a reserved key to the config grammar.
