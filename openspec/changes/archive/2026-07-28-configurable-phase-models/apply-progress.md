# Apply Progress: Configurable Per-Phase Models

## Status

5/5 phases complete, 19/19 tasks marked `[x]` in `tasks.md`. **Verify round 1 returned FAIL** (1 CRITICAL, 7 WARNING, 5 SUGGESTION) — see `verify-report.md`. Correction round 1 fixed CRITICAL-1 and two follow-up items. **Correction round 2 (this round)** fixed WARNING-1 (spec wording) and WARNING-7 (missing test) — see "Correction Round 2" below. 14/14 spec test scenarios now covered by a passing runtime test. Ready for archive.

Mode: Standard (no `strict_tdd` config/test-runner gate found in this repo; tests were still written before/alongside each behavioral change per the Work Unit Evidence gate below).

## Phases Completed

### Phase 1: State & Validation
- `internal/state/state.go`: added `ModelPhaseAssignment{Default, Escalation string}`, `State.ModelAssignments map[string]map[string]ModelPhaseAssignment` (JSON `model_assignments`)
- `Load()` decodes `model_assignments`; legacy `model_overrides` warned and dropped
- Tests added: round-trip, nil-by-default (backward compat), unknown-JSON-key tolerance, legacy-key-warned+dropped, injection-char rejection (6 sub-cases), valid-value acceptance

### Phase 2: Substitution Mechanism
- New `internal/packaging/models.go`: `agentNames` (6 agents), `AgentNames(platform)`, `builtinModels` (literal defaults per platform/agent), `resolveModel`, `validateAssignments`
- `packaging.Render` gained a 3rd parameter `assignments`; validates before writing any file
- `substituteModels` replaces `{{MODEL_DEFAULT:<agent>}}` / `{{MODEL_ESCALATION:<agent>}}` in all surfaces
- `renderCodexAgentConfig` now emits resolved `model = "..."` in TOML (previously dropped)
- All ~15 existing `Render(...)` call sites updated to pass `nil` or explicit assignments

### Phase 3: Template Reconciliation
- All 18 agent templates had literal models replaced with placeholders in frontmatter/TOML and prose
- All 3 `SKILL.md` roster tables: replaced literal model names with same placeholders per agent row
- OpenCode reconciled: frontmatter and prose now resolve from the same placeholder pair
- `drup-validator` prose fixed on all 3 platforms: aligned to strong-tier default

### Phase 4: Sync Flow Integration
- `internal/app/commands.go`: `RunInstall`/`RunSync`/`installAgents()` load state before render and pass resolved assignments to `Render()`
- Integration tests added: configured assignments produce substituted files; one bad platform fails alone

### Phase 5: Documentation
- New `docs/model-configuration.md`: config shape, per-platform examples, backward-compat notes
- Added mention + link to README.md's existing `## Configuration` section (docs/configuration.md does not exist in this repo)
- Code comments added: `state.go`, `models.go`, `packaging.go`

## Test Results

```
go build ./...   → clean
go vet ./...     → clean
gofmt -l internal/   → no output
go test ./...    → ok, all 20 packages (including internal/state, internal/packaging, internal/app)
```

All 7 spec test scenarios have a corresponding passing test.

## Delivered State

37 files total (1721 insertions): 30 modified files (680 insertions + 87 deletions) + 7 new files (1041 insertions).

Key new files:
- `internal/packaging/models.go` (119 lines) — built-in model table per platform/agent
- `docs/model-configuration.md` (72 lines) — user-facing documentation

Key modified files:
- `internal/state/state.go` — ModelAssignments type + validation
- `internal/packaging/packaging.go` — Render parameter + substitution logic
- 21 template/SKILL.md files — placeholder substitution
- Test files — comprehensive coverage of all 7 spec scenarios

## Process Status

**All artifacts committed to HEAD `3debe3a`**: 37 files, 1721 insertions, tree fully staged and committed.

## Test & Build Evidence

- Build: `go build ./...` exit 0
- Vet: `go vet ./...` exit 0
- Format: `gofmt -l internal/` exit 0
- Tests: `go test -count=1 ./...` exit 0, 20/20 packages `ok`, 0 failures, 0 skips

## Risks Addressed

- **Validation isolation** — typo'd config under one platform fails only that platform (per design decision 4)
- **Partial substitution** — residual `{{MODEL_` guard fails the render; frontmatter-vs-prose consistency tested
- **Codex model drop** — now emitted in TOML before and after substitution
- **OpenCode contradition** — frontmatter and prose now agree (both from same placeholder)
- **Backward compatibility** — empty config renders to pre-change hardcoded values

## Remaining Notes

- **Deferral**: `"*"` platform-wide fallback agent key not implemented (design decision 1 unresolved; spec/tasks don't require it)
- **Test narrowness**: REQ-009 verified end-to-end via live `drup sync`, but one committed test does not pin the full `model_assignments → modified-only` clause
- **Known limitations**: task 1.2 references removed `ValidateModelAssignments()` method (was dead code; validation now in `state.ValidateModelValue` + `packaging.validateAssignments`)
