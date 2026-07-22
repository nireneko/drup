# Tasks: Fix Drup Pipeline Bugs

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~150-200 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr (size-exception accepted) |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

## Phase 1: RED — Tests for `--all` flag (4 sites)

- [x] 1.1 Add test assertion in `commands_test.go` verifying `RunScan` passes `"--all"` to drush args
- [x] 1.2 Add test or assertion verifying `realHandleScan` (mcp_tools.go) passes `"--all"` to drush
- [x] 1.3 Add test or assertion verifying `realHandleAutofix` re-scan passes `"--all"` to drush
- [x] 1.4 Add test verifying `realHandleValidate` passes `params.Module` when set, else `"--all"`

## Phase 2: GREEN — Implement `--all` flag (4 sites)

- [x] 2.1 Add `"--all"` to drush args in `RunScan` (`internal/app/commands.go:61`)
- [x] 2.2 Add `"--all"` to drush args in `realHandleScan` (`internal/app/mcp_tools.go:78`)
- [x] 2.3 Add `"--all"` to drush args in `realHandleAutofix` re-scan (`internal/app/mcp_tools.go:122`)
- [x] 2.4 Add conditional in `realHandleValidate` (`internal/app/mcp_tools.go:204`): use `params.Module` when non-empty, else `"--all"`
- [x] 2.5 Run `go test ./internal/app/...` — all `--all` tests pass

## Phase 3: RED — Tests for composer upgrade flow

- [x] 3.1 Update `TestRunUpgradeCore_Integration` mock to track 3 composer calls: `config policy.advisories.block false`, `require ... -W --no-update`, `update -W`
- [x] 3.2 Add assertion: `composer.json.bak` does NOT exist after successful upgrade (`os.Stat` → `os.IsNotExist`)
- [x] 3.3 Add assertion: error message includes checkpoint SHA on failure

## Phase 4: GREEN — Implement composer upgrade fixes

- [x] 4.1 Add `execRunFn("composer", "config", "policy.advisories.block", "false")` before require in `RunUpgradeCore` (`commands.go:660`)
- [x] 4.2 Change composer require args: replace `"--update-with-dependencies"` with `"-W", "--no-update"` (`commands.go:661-666`)
- [x] 4.3 Add `execRunFn("composer", "update", "-W")` after require succeeds (`commands.go:674`)
- [x] 4.4 Add `defer os.Remove(backupPath)` after `os.WriteFile` (`commands.go:658`)
- [x] 4.5 Update error message at `commands.go:642` to include `applyResult.RollbackCheckpoint`
- [x] 4.6 Run `go test ./internal/app/...` — composer tests pass

## Phase 5: RED — Tests for config conflict handling

- [x] 5.1 Add test verifying `RunPreflight` calls `drush config:delete update.settings` before `drush en upgrade_status`
- [x] 5.2 Add test verifying `realHandleUpgradeScan` calls `drush config:delete update.settings` before enabling module

## Phase 6: GREEN — Implement config conflict fixes

- [x] 6.1 Add `drupexec.Run("drush", "config:delete", "update.settings")` before `drush en upgrade_status` in `RunPreflight` (`commands.go:469-471`)
- [x] 6.2 Add `drupexec.RunWithEnv(...)` for `config:delete update.settings` before `drush en upgrade_status` in `realHandleUpgradeScan` (`mcp_tools.go:602-604`)
- [x] 6.3 Run `go test ./internal/app/...` — config conflict tests pass

## Phase 7: Verification

- [x] 7.1 Run `go test ./...` — all tests pass
- [x] 7.2 Run `go vet ./...` — clean
- [x] 7.3 Run `gofmt -l .` — no formatting issues
