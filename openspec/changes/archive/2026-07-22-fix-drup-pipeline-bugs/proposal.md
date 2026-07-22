# Proposal: Fix Drup Pipeline Bugs

## Intent

Real Drupal 10.6→11.4 upgrade exposed 6 bugs across the drup pipeline: `upgrade_status:analyze` returns empty results without `--all`, `composer require` fails on major version bumps without `-W` and advisory bypass, preflight crashes on config conflicts when enabling `upgrade_status`, and backup files leak on success. All fixes are minimal, independent, and target exact source locations identified in exploration.

## Scope

### In Scope
- Add `--all` flag to all 4 `upgrade_status:analyze` call sites (CLI + MCP tools)
- Fix `upgrade-core` composer flow: `-W` flag, advisory bypass, full `composer update -W`
- Handle `update.settings` config conflict before enabling `upgrade_status`
- Clean up `composer.json.bak` on success, improve rollback error messages

### Out of Scope
- Adding `--dry-run` to scan/fix/preflight commands
- Go-level pipeline orchestrator (sub-agents are prompt-driven by design)
- New MCP tools or capabilities

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `scan`: drush `upgrade_status:analyze` invocation MUST include `--all` flag
- `core-upgrade`: composer require MUST use `--with-all-dependencies`, disable advisory blocking, run full `composer update -W`; backup file MUST be cleaned on success
- `preflight`: enabling `upgrade_status` MUST handle existing `update.settings` config conflict
- `mcp-server`: `scan`, `autofix`, `validate` tools MUST pass `--all` (or module name when scoped); `upgrade_scan` MUST handle config conflict on module enable

## Approach

Four independent fix groups, each testable in isolation:

1. **`--all` flag** (Bugs 1-4): Add `"--all"` to drush args at `commands.go:61`, `mcp_tools.go:78,122`. For `realHandleValidate` (`mcp_tools.go:204`), use module name from params when set, else `--all`.
2. **Composer upgrade** (Bug 5): In `RunUpgradeCore` (`commands.go:661-674`): run `composer config policy.advisories.block false` before require, change flag to `--with-all-dependencies`, add `composer update -W` after.
3. **Config conflict** (Bug 6): Before `drush en upgrade_status` in `RunPreflight` (`commands.go:469-485`) and `realHandleUpgradeScan` (`mcp_tools.go:602-612`), delete conflicting `update.settings` config via `drush config:delete update.settings` or equivalent.
4. **Cleanup** (Bug 8): Add `defer os.Remove(backupPath)` after backup creation. Include checkpoint SHA in rollback error messages.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modified | `RunScan` drush args, `RunPreflight` config handling, `RunUpgradeCore` composer flow + cleanup |
| `internal/app/mcp_tools.go` | Modified | `realHandleScan`, `realHandleAutofix`, `realHandleValidate` drush args; `realHandleUpgradeScan` config handling |
| `internal/app/commands_test.go` | Modified | Update test assertions for new drush args and composer commands |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Composer advisory config persists after upgrade | Low | Acceptable — user explicitly chose to upgrade; document in output |
| Config delete could lose user customizations | Medium | Only delete `update.settings` (known conflict), not arbitrary config; log what was deleted |
| Existing tests assert exact arg lists | High | Update test fixtures alongside code changes |

## Rollback Plan

Each fix group is an independent commit. Revert any commit individually if issues arise. The `--all` flag and cleanup fixes are zero-risk reverts. Composer and config changes should be tested against a real Drupal 10 project before merging.

## Dependencies

- None external. All fixes use existing exec layer and drush/composer CLIs.

## Success Criteria

- [ ] `drup scan` returns full analysis results (not empty) on a Drupal 10 project
- [ ] `drup upgrade-core 11` completes composer require + update without manual intervention
- [ ] `drup preflight` succeeds on projects with existing `update.settings` config
- [ ] No `composer.json.bak` left after successful `upgrade-core`
- [ ] All existing tests pass with updated assertions
- [ ] `go vet ./...` clean
