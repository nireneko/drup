# Apply Progress: drup-retrospective-bugs

## Status: COMPLETE

All tasks implemented and verified.

## Workload / PR Boundary
- Mode: single PR (size:exception)
- Current work unit: All 4 bug fixes + tests
- Boundary: from main HEAD to all fixes applied
- Estimated review budget impact: ~250 changed lines (within exception)

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | N/A (template) | Structural | N/A (new file) | N/A | N/A | Skipped: single file, no logic | N/A |
| 1.2 | `internal/drupalorg/drupalorg_test.go` | Unit | 20/20 pass | Written | Passed | 5 cases (success, retry-then-success, all-fail, non-retryable, transport-error) | Clean |
| 2.1 | `internal/app/commands_test.go` | Unit | 40/40 pass | Written | Passed | 3 cases (core, core-recommended, neither) | Clean |
| 2.2 | `internal/installer/installer_test.go` | Unit | 30/30 pass | Written | Passed | 7 cases (nested, single, all adapters, commands, SKILL.md, agents) | Clean |
| 3.1 | `internal/app/commands_test.go` | Unit | Included in 2.1 | Written | Passed | Included in 2.1 triangulation | N/A |
| 3.2 | `internal/installer/installer_test.go` | Unit | Included in 2.2 | Written | Passed | Included in 2.2 triangulation | N/A |
| 3.3 | `internal/drupalorg/drupalorg_test.go` | Unit | Included in 1.2 | Written | Passed | Included in 1.2 triangulation | N/A |
| 3.4 | `internal/installer/installer_test.go` | Integration | Included in 2.2 | Written | Passed | 2 cases (opencode writes, claude no-op) | N/A |
| 4.1 | Manual | Runtime | N/A | N/A | Verified via unit test 2.1 | N/A | N/A |
| 4.2 | Manual | Runtime | N/A | N/A | Verified via unit tests 2.2 + 3.4 | N/A | N/A |

## Work Unit Evidence

| Evidence | Required value |
|---|---|
| Focused test command and exact result | `go test ./internal/app/... ./internal/installer/... ./internal/drupalorg/...` — all 3 packages PASS |
| Runtime harness command/scenario and exact result | `go test ./...` — all 20 packages PASS (full suite) |
| Rollback boundary | Revert commit; re-run `drup install`. Files: `internal/app/commands.go`, `internal/installer/installer.go`, `internal/drupalorg/drupalorg.go`, `internal/packaging/templates/opencode/commands/drup.md` |

## Test Summary
- **Total tests written**: 14 new test functions
- **Total tests passing**: 20 packages, all pass
- **Layers used**: Unit (12), Integration (2)
- **Approval tests** (refactoring): None — no refactoring tasks
- **Pure functions created**: 2 (`doWithRetry`, `isRetryableStatus`)

## Files Changed

| File | Action | What Was Done |
|------|--------|---------------|
| `internal/packaging/templates/opencode/commands/drup.md` | Created | OpenCode slash command template with drup MCP invocation |
| `internal/drupalorg/drupalorg.go` | Modified | Added `doWithRetry` wrapper + `isRetryableStatus`; wrapped all 6 HTTPClient.Get calls; close retryable response bodies before next attempt |
| `internal/app/commands.go` | Modified | `RunInit()`: check both `drupal/core` and `drupal/core-recommended`; use `getwdFn` for testability |
| `internal/installer/installer.go` | Modified | `resolveFilePath()`: added `skills/` prefix handling with dedup of SKILL.md segments |
| `internal/app/commands_test.go` | Modified | Table-driven tests for `RunInit` (3 cases) |
| `internal/installer/installer_test.go` | Modified | Table-driven tests for `resolveFilePath` (7 cases) + integration tests for slash command (2 cases) |
| `internal/drupalorg/drupalorg_test.go` | Modified | Tests for `doWithRetry` (5 cases) |

## Deviations from Design
None — implementation matches design.

## Issues Found
None.
