# Tasks: Configurable Per-Phase Models

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 600-700 |
| 400-line budget risk | Low (project review budget: 800) |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Full slice: state, substitution, templates, sync wiring, docs | PR 1 | `go test ./internal/state/... ./internal/packaging/... ./internal/app/...` | `drup sync` with a temp `~/.config/drup/state.json` carrying a custom `model_assignments` block | Revert commit; `model_assignments` is additive, older builds ignore it |

## Phase 1: State & Validation (Foundation)

- [x] 1.1 `internal/state/state.go`: add `ModelPhaseAssignment{Default, Escalation string}` + `ModelAssignments map[string]map[string]ModelPhaseAssignment` (JSON `model_assignments`); deprecate `ModelOverrides` (read-and-drop)
- [x] 1.2 Add `ValidateModelAssignments()` — reject newline, `"`, `\`, `#`, leading/trailing whitespace in values
- [x] 1.3 `state.Load()`: parse `model_assignments`; warn once and drop legacy `model_overrides` if present
- [x] 1.4 Unit tests: round-trip, backward compat (nil field), legacy key warned+dropped, injection chars rejected (7 scenarios per spec)

## Phase 2: Substitution Mechanism (Core)

- [x] 2.1 Create `internal/packaging/models.go`: built-in per-platform/agent default table (today's literals) + `AgentNames(platform)`
- [x] 2.2 Extend `Render(platform, binaryPath, assignments map[string]map[string]state.ModelPhaseAssignment)`; resolve agent → builtin per platform
- [x] 2.3 Implement substitution: `{{MODEL_DEFAULT:<agent>}}` / `{{MODEL_ESCALATION:<agent>}}`; fail closed on unknown platform/agent; assert zero residual `{{MODEL_`
- [x] 2.4 Fix `renderCodexAgentConfig`: emit resolved `model = "<value>"` (currently dropped); substitute before TOML conversion; same quoting check as `description`
- [x] 2.5 Unit tests: nil ⇒ byte-identical output; precedence; injection rejected; unknown key errors with zero files written; Codex retains `model`

## Phase 3: Template Reconciliation (Surface)

- [x] 3.1 Update 18 agent templates (`internal/packaging/templates/{claude,opencode,codex}/agents/*`): replace literal models with placeholders in frontmatter/TOML `model` and "Default model:" prose
- [x] 3.2 Update 3 `SKILL.md` roster tables: replace literal names with same placeholders per agent row
- [x] 3.3 Reconcile OpenCode: frontmatter and prose resolve from the same placeholder pair (fixes `qwen3-30b` vs "Default: haiku")
- [x] 3.4 Fix `drup-validator` prose: remove "Default: haiku"; align to strong-tier default, matching frontmatter
- [x] 3.5 Integration test: `drup sync` per platform — frontmatter/prose/roster agree, zero `{{MODEL_` remains

## Phase 4: Sync Flow Integration

- [x] 4.1 `internal/app/commands.go`: `RunInstall`/`RunSync`/`installAgents()` load state before render and pass resolved assignments to `Render()`
- [x] 4.2 Integration test: custom `model_assignments` produces substituted agent files; one bad platform fails alone

## Phase 5: Documentation

- [x] 5.1 Write `docs/model-configuration.md`: config shape, per-platform examples, edit instructions, backward-compat/downgrade notes
- [x] 5.2 Update `docs/configuration.md`: short mention + link to `model-configuration.md` — **deviation**: `docs/configuration.md` does not exist in this repo; added the mention + link to README.md's existing `## Configuration` section instead (see Apply Progress for detail)
- [x] 5.3 Add code comments: `state.go` (deprecated field, validation rules), `packaging.go`/`models.go` (resolution precedence, placeholder grammar)
