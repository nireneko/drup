# Apply Progress: drup-orchestrator-delegation-fix

**Status**: all_done
**Mode**: Strict TDD
**Delivery**: single-pr (size:exception approved)

## Completed Tasks (20/20)

### Phase 1: Cross-Platform SKILL.md and Bootstrap Templates (7/7)
- [x] 1.1 Rewrote `internal/packaging/templates/opencode/SKILL.md` — replaced sub-agent roster with 8-stage `drup <stage>` CLI pipeline
- [x] 1.2 Copied identical SKILL.md to claude/ and codex/
- [x] 1.3 Deleted all 6 agent files in opencode/agents/
- [x] 1.4 Deleted all 6 agent files in claude/agents/
- [x] 1.5 Deleted all 6 agent files in codex/agents/
- [x] 1.6 Created `internal/packaging/templates/claude/CLAUDE.md` bootstrap
- [x] 1.7 Created `internal/packaging/templates/codex/copilot-instructions.md` bootstrap

### Phase 2: `drup upgrade-core` CLI Command (6/6)
- [x] 2.1 Added `RunUpgradeCore(args []string) error` to commands.go
- [x] 2.2 Added `upgrade-core` case to app.go Run() switch
- [x] 2.3 Added `upgrade-core` to printUsage()
- [x] 2.4 RED test: relative path and `..` segment rejected
- [x] 2.5 RED test: dirty working tree returns error with file list
- [x] 2.6 Unit tests: arg parsing, missing composer.json, already-at-target, dry-run, composer-not-found, drush-not-found

### Phase 3: Packaging and Installer Wiring (4/4)
- [x] 3.1 Updated packaging.go with `{{SKILL_PATH}}` substitution
- [x] 3.2 Updated installer.go: CLAUDE.md → project root, copilot-instructions.md → .github/
- [x] 3.3 Table-driven bootstrap template rendering tests
- [x] 3.4 SKILL.md content test: zero platform primitives

### Phase 4: Integration Verification (3/3)
- [x] 4.1 Integration test with fixture composer.json + mocked exec
- [x] 4.2 `go test ./...` passes all 16 packages
- [x] 4.3 `go build ./cmd/drup` produces working binary

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/packaging/templates/opencode/SKILL.md` | Rewritten | Cross-platform 8-stage CLI pipeline |
| `internal/packaging/templates/claude/SKILL.md` | Rewritten | Identical to opencode |
| `internal/packaging/templates/codex/SKILL.md` | Rewritten | Identical to opencode |
| `internal/packaging/templates/opencode/agents/*` | Deleted | 6 agent files removed |
| `internal/packaging/templates/claude/agents/*` | Deleted | 6 agent files removed |
| `internal/packaging/templates/codex/agents/*` | Deleted | 6 agent files removed |
| `internal/packaging/templates/claude/CLAUDE.md` | Created | Bootstrap referencing SKILL.md |
| `internal/packaging/templates/codex/copilot-instructions.md` | Created | Bootstrap referencing SKILL.md |
| `internal/packaging/packaging.go` | Modified | Added `{{SKILL_PATH}}` substitution |
| `internal/packaging/packaging_test.go` | Modified | 7 new tests for content/bootstrap |
| `internal/app/commands.go` | Modified | Added RunUpgradeCore + test hooks |
| `internal/app/commands_test.go` | Modified | 12 new tests for upgrade-core |
| `internal/app/app.go` | Modified | Added upgrade-core case + usage |
| `internal/app/app_test.go` | Modified | 2 new routing tests |
| `internal/coreupgrade/apply.go` | Modified | Exported ValidateProjectPath |
| `internal/coreupgrade/check.go` | Modified | Exported MajorVersion |
| `internal/coreupgrade/rollback.go` | Modified | Updated to use exported name |
| `internal/installer/installer.go` | Modified | Bootstrap file routing in Install() |
| `internal/installer/installer_test.go` | Modified | 2 new bootstrap installation tests |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1-1.2 | `packaging_test.go` | Unit | ✅ 5/5 | ✅ Written | ✅ Passed | ✅ 3 platforms | ✅ Clean |
| 1.3-1.5 | `packaging_test.go` | Unit | ✅ 5/5 | ✅ Written | ✅ Passed | ✅ 3 platforms | ✅ Clean |
| 1.6-1.7 | `packaging_test.go` | Unit | ✅ 5/5 | ✅ Written | ✅ Passed | ➖ Single | ✅ Clean |
| 2.1 | `commands_test.go` | Unit | ✅ 12/12 | ✅ Written | ✅ Passed | ✅ 8 cases | ✅ Clean |
| 2.2-2.3 | `app_test.go` | Unit | ✅ 8/8 | ✅ Written | ✅ Passed | ➖ Single | ✅ Clean |
| 2.4 | `commands_test.go` | Unit | ✅ 12/12 | ✅ Written | ✅ Passed | ✅ 2 cases | ✅ Clean |
| 2.5 | `commands_test.go` | Unit | ✅ 12/12 | ✅ Written | ✅ Passed | ➖ Single | ✅ Clean |
| 2.6 | `commands_test.go` | Unit | ✅ 12/12 | ✅ Written | ✅ Passed | ✅ 6 cases | ✅ Clean |
| 3.1-3.4 | `packaging_test.go` | Unit | ✅ 12/12 | ✅ Written | ✅ Passed | ✅ 3 platforms | ✅ Clean |
| 3.2 | `installer_test.go` | Unit | ✅ 25/25 | ✅ Written | ✅ Passed | ✅ 2 platforms | ✅ Clean |
| 4.1 | `commands_test.go` | Integration | ✅ 14/14 | ✅ Written | ✅ Passed | ➖ Single | ✅ Clean |
| 4.2 | N/A | Verification | N/A | N/A | ✅ 16 pkgs | N/A | N/A |
| 4.3 | N/A | Verification | N/A | N/A | ✅ Binary | N/A | N/A |

## Test Summary
- **Total tests written**: 23 new tests
- **Total tests passing**: All (16 packages, 0 failures)
- **Layers used**: Unit (21), Integration (1), Verification (2)
- **Approval tests**: None — no refactoring of existing behavior
- **Pure functions created**: 0 (RunUpgradeCore is inherently impure — orchestrates exec)

## Workload / PR Boundary
- Mode: single PR (size:exception approved)
- Current work unit: All 3 units combined
- Boundary: Full change from scratch to complete
- Estimated review budget impact: ~750 changed lines (exception approved)
