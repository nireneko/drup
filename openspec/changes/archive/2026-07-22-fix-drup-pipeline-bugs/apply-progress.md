# Apply Progress: Fix Drup Pipeline Bugs

## Status: COMPLETE

All 4 bug groups implemented and verified.

## Completed Tasks

### Group 1: Missing `--all` flag (4 sites)
- [x] 1.1 Test: `RunScan` passes `--all` to drush
- [x] 1.2 Test: `realHandleScan` passes `--all` to drush
- [x] 1.3 Test: `realHandleAutofix` re-scan passes `--all` to drush
- [x] 1.4 Test: `realHandleValidate` passes module name when set, else `--all`
- [x] 2.1 Implementation: `RunScan` (`commands.go:61`)
- [x] 2.2 Implementation: `realHandleScan` (`mcp_tools.go:78`)
- [x] 2.3 Implementation: `realHandleAutofix` (`mcp_tools.go:122`)
- [x] 2.4 Implementation: `realHandleValidate` (`mcp_tools.go:204`) with conditional logic
- [x] 2.5 All `--all` tests pass

### Group 2: Composer upgrade fixes
- [x] 3.1 Test: Track 3 composer calls (config, require, update)
- [x] 3.2 Test: Backup file removed after success
- [x] 3.3 Test: Error message includes checkpoint SHA
- [x] 4.1 Implementation: `composer config policy.advisories.block false` before require
- [x] 4.2 Implementation: Changed require args to use `-W` and `--no-update`
- [x] 4.3 Implementation: Added `composer update -W` after require
- [x] 4.4 Implementation: Added `defer os.Remove(backupPath)` for cleanup
- [x] 4.5 Implementation: Updated error message to include checkpoint SHA
- [x] 4.6 All composer tests pass

### Group 3: Preflight config conflict handling
- [x] 5.1 Test: `RunPreflight` calls `config:delete update.settings` before `en upgrade_status`
- [x] 5.2 Test: `realHandleUpgradeScan` calls `config:delete update.settings` before enabling
- [x] 6.1 Implementation: `RunPreflight` (`commands.go:469-471`)
- [x] 6.2 Implementation: `realHandleUpgradeScan` (`mcp_tools.go:602-604`)
- [x] 6.3 All config conflict tests pass

### Group 4: Verification
- [x] 7.1 `go test ./...` — all tests pass
- [x] 7.2 `go vet ./...` — clean
- [x] 7.3 `gofmt -l .` — no formatting issues

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modified | Added `--all` to `RunScan`; added config:delete before enable in `RunPreflight`; rewrote composer flow in `RunUpgradeCore` with advisory bypass, `-W` flags, `composer update -W`, defer cleanup, improved error messages |
| `internal/app/mcp_tools.go` | Modified | Added `--all` to `realHandleScan`, `realHandleAutofix`; added conditional module/`--all` to `realHandleValidate`; added config:delete before enable in `realHandleUpgradeScan`; changed `defaultEnvDetector` to interface type for testability |
| `internal/app/commands_test.go` | Modified | Added tests for `--all` flag in `RunScan`; updated `TestRunUpgradeCore_Integration` to verify new composer call sequence and backup cleanup; added test for checkpoint in error messages |
| `internal/app/mcp_tools_test.go` | Modified | Added tests for `--all` flag in `realHandleScan`, `realHandleAutofix`, `realHandleValidate`; added test for config:delete in `realHandleUpgradeScan`; added mock environment detector |
| `internal/app/preflight_test.go` | Modified | Added test for config:delete in `RunPreflight` |

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `commands_test.go` | Unit | ✅ 15/15 | ✅ Written | ✅ Passed | ✅ 1 case | ✅ Clean |
| 1.2 | `mcp_tools_test.go` | Unit | ✅ 30/30 | ✅ Written | ✅ Passed | ✅ 1 case | ✅ Clean |
| 1.3 | `mcp_tools_test.go` | Unit | ✅ 30/30 | ✅ Written | ✅ Passed | ✅ 1 case | ✅ Clean |
| 1.4 | `mcp_tools_test.go` | Unit | ✅ 30/30 | ✅ Written | ✅ Passed | ✅ 2 cases (module + all) | ✅ Clean |
| 3.1-3.3 | `commands_test.go` | Integration | ✅ 15/15 | ✅ Written | ✅ Passed | ✅ 3 assertions | ✅ Clean |
| 5.1 | `preflight_test.go` | Unit | ✅ 3/3 | ✅ Written | ✅ Passed | ✅ 1 case | ✅ Clean |
| 5.2 | `mcp_tools_test.go` | Unit | ✅ 30/30 | ✅ Written | ✅ Passed | ✅ 1 case | ✅ Clean |

## Test Summary
- **Total tests written**: 9 new tests
- **Total tests passing**: 45+ (all existing + new)
- **Layers used**: Unit (7), Integration (2)
- **Approval tests**: None — no refactoring tasks
- **Pure functions created**: 0 — all changes were argument-level modifications

## Workload / PR Boundary
- Mode: single PR (size-exception accepted)
- Current work unit: All 4 bug groups
- Boundary: Complete implementation of all fixes from proposal
- Estimated review budget impact: ~200 lines changed (within exception budget)

## Deviations from Design
None — implementation matches design exactly.

## Issues Found
None.

## Next Steps
Ready for verify phase.
