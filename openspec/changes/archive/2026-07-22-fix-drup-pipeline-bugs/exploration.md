## Exploration: Fix drup pipeline bugs from upgrade report

### Current State

**CLI architecture**: Custom switch-based dispatcher in `internal/app/app.go`. NOT cobra. `Run()` switches on `args[0]` to dispatch to `RunScan`, `RunFix`, `RunUpgradeCore`, etc.

**MCP architecture**: Two-layer system:
1. `internal/mcp/tools.go` — placeholder handlers registered via `defaultTools()` in `NewServer()`
2. `internal/app/mcp_tools.go` — real handlers (`WireMCPTools`) that override placeholders via `s.RegisterTool()`

The real handlers in `mcp_tools.go` are what actually run when agents call MCP tools. The placeholders in `tools.go` are dead code for production — they're only used if `WireMCPTools` is NOT called.

**Pipeline orchestration**: There is NO Go-level pipeline orchestrator. Sub-agents (`drup-contrib`, `drup-custom`, `drup-theme`) are prompt template files written by `internal/packaging` to agent config directories. The AI agent reading these skill files IS the orchestrator. There is no sub-agent dispatch mechanism in Go code — by design.

**Exec layer**: `internal/exec/exec.go` provides `Run(cmd, args...)` and `RunWithEnv(prefix, cmd, args...)`. Both return `(stdout, stderr, exitCode, err)`. Non-zero exit is NOT an error.

### Affected Areas

- `internal/app/commands.go:61` — `RunScan` missing `--all` in drush command
- `internal/app/commands.go:661-674` — `RunUpgradeCore` composer require missing `-W`, advisory bypass, and full update
- `internal/app/commands.go:657-658` — `composer.json.bak` created but never cleaned up
- `internal/app/mcp_tools.go:78` — `realHandleScan` missing `--all`
- `internal/app/mcp_tools.go:122` — `realHandleAutofix` re-scan missing `--all`
- `internal/app/mcp_tools.go:204` — `realHandleValidate` missing `--all`
- `internal/app/mcp_tools.go:602-612` — `realHandleUpgradeScan` enabling upgrade_status may conflict with existing config
- `internal/app/commands.go:469-485` — `RunPreflight` enabling upgrade_status may conflict with existing config

### Bug-by-Bug Analysis

#### Bug 1: `drup scan` CLI missing `--all`
- **File**: `internal/app/commands.go`, line 61
- **Function**: `RunScan(path string)`
- **Current code**: `drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--format=json")`
- **Problem**: `drush upgrade_status:analyze` requires either `--all` or a specific module name. Without either, it returns no results or errors.
- **Fix**: Change to `drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--all", "--format=json")`
- **Risk**: Low. Single argument addition. The `--all` flag is documented drush behavior.
- **Test impact**: `commands_test.go` may need updating if it asserts on exact drush args.

#### Bug 2: MCP `scan` tool missing `--all`
- **File**: `internal/app/mcp_tools.go`, line 78
- **Function**: `realHandleScan`
- **Current code**: `drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")`
- **Fix**: Add `"--all"` to the drush args.
- **Risk**: Low. Same fix as Bug 1.

#### Bug 3: MCP `validate` tool missing `--all`
- **File**: `internal/app/mcp_tools.go`, line 204
- **Function**: `realHandleValidate`
- **Current code**: `drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")`
- **Fix**: Add `"--all"` to the drush args. Note: when `params.Module` is set, should use module name instead of `--all`.
- **Risk**: Low. Need to handle the module-scoped case: if `params.Module != ""`, use the module name as the analyze target instead of `--all`.

#### Bug 4: MCP `autofix` re-scan missing `--all`
- **File**: `internal/app/mcp_tools.go`, line 122
- **Function**: `realHandleAutofix`
- **Current code**: `drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")`
- **Fix**: Add `"--all"` to the drush args.
- **Risk**: Low.

#### Bug 5: `upgrade-core` composer require failures
- **File**: `internal/app/commands.go`, lines 661-674
- **Function**: `RunUpgradeCore`
- **Current code**:
  ```go
  composerArgs := []string{
      "require",
      "drupal/core-recommended:^11.0",
      "drupal/core:^11.0",
      "--update-with-dependencies",
  }
  ```
- **Problems**:
  1. `--update-with-dependencies` only updates direct dependencies of the named packages. For a major version bump (10→11), transitive dependencies (like Symfony) need `--with-all-dependencies` (`-W`).
  2. Security advisories block Symfony 7.4 installation. Need `composer config policy.advisories.block false` before the require.
  3. After `composer require`, a full `composer update -W` ensures all transitive deps resolve correctly.
- **Fix**:
  1. Change `--update-with-dependencies` to `--with-all-dependencies` (or `-W`).
  2. Before composer require, run `composer config policy.advisories.block false`.
  3. After successful composer require, run `composer update -W` to finalize.
- **Risk**: Medium. Composer behavior changes across versions. Need to test with actual Drupal 10→11 upgrade. The advisory config change is persistent — should we restore it after? Probably not, since the user explicitly wants to upgrade.

#### Bug 6: Preflight config conflict when enabling modules
- **File**: `internal/app/commands.go`, lines 469-485 (`RunPreflight`)
- **Also**: `internal/app/mcp_tools.go`, lines 602-612 (`realHandleUpgradeScan`)
- **Problem**: When enabling `upgrade_status` (which depends on `update` module), if `update.settings` config already exists in the active config directory, `drush en` fails with a config conflict.
- **Fix**: Before enabling, import existing config or use `drush config:set` to handle the conflict. Alternative: use `drush en upgrade_status -y --preview=null` or handle the specific error.
- **Risk**: Medium. Config management in Drupal is complex. The fix needs to handle the specific conflict without breaking other config.

#### Bug 7: Sub-agent dispatch not implemented
- **Finding**: This is NOT a bug in Go code. Sub-agents are prompt templates (skill files) installed to agent config directories by `internal/packaging`. The AI agent (OpenCode, Claude, etc.) reads these skill files and orchestrates the pipeline. There is no Go-level dispatch because the AI agent IS the orchestrator.
- **Status**: Working as designed. No code change needed.
- **Note**: If the report expected Go-level dispatch, that's a misunderstanding of the architecture. The pipeline is: AI agent reads skill → calls MCP tools → drup executes.

#### Bug 8: Cleanup and polish
- **`composer.json.bak`**: Created at `internal/app/commands.go:657-658`, never removed on success. Fix: `defer os.Remove(backupPath)` after verifying success, or remove after `result.Success = true`.
- **Error messages**: When `coreupgrade.Apply` fails with rollback, the error at line 643 says "core upgrade failed" but doesn't mention the checkpoint for manual rollback. Fix: include checkpoint SHA in error message.
- **Dry-run mode**: `upgrade-core` supports `--dry-run` but `scan`, `fix`, `preflight` don't. Low priority.
- **Risk**: Low for all.

### Approach Comparison

For the `--all` fix (bugs 1-4), there are two approaches:

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| A. Add `--all` always | Always pass `--all` to `upgrade_status:analyze` | Simple, one-line fix per call site | Can't scope to a single module |
| B. Conditional `--all` or module name | Pass `--all` when no module specified, module name when specified | Supports scoped scans | Slightly more logic |

**Recommendation**: Approach B for MCP tools (validate already has a `module` param), Approach A for CLI scan (no module param).

For the composer fix (bug 5):

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| A. Minimal: add `-W` | Just change the flag | Smallest diff | May still fail on advisories |
| B. Full: `-W` + advisory bypass + `composer update -W` | Handle all three issues | Actually works end-to-end | More changes, need to test |

**Recommendation**: Approach B. The report showed all three issues in a real upgrade; fixing only one won't resolve the failure.

### Recommendation

**Fix order** (by dependency and severity):

1. **Bugs 1-4** (independent, critical): Add `--all` to all four `upgrade_status:analyze` call sites. Can be done in a single commit.
2. **Bug 5** (independent, critical): Fix composer require in `RunUpgradeCore`. Needs careful testing.
3. **Bug 6** (independent, high): Fix preflight/upgrade_scan config conflict. Needs Drupal testing environment.
4. **Bug 8** (independent, low): Cleanup `composer.json.bak`, improve error messages.

Bugs 1-4 are the simplest and most impactful — fix first.

### Risks

- **Composer flag behavior**: `--with-all-dependencies` vs `--update-with-dependencies` has subtle differences. Test with a real Drupal 10 project.
- **Advisory config persistence**: `composer config policy.advisories.block false` is persistent in composer.json. This is probably fine for an upgrade scenario but should be documented.
- **Config conflict fix**: Drupal config management is fragile. The fix must not corrupt existing config. Test with a project that has existing `update.settings` config.
- **Test coverage**: Existing tests mock `execRunFn` — they may assert exact arg lists. Adding `--all` will break those assertions. Update tests alongside the fix.

### Ready for Proposal

Yes. All bugs are well-scoped with exact file paths and line numbers. The fixes are minimal and independent. Bugs 1-4 can be a single PR, bug 5 should be its own PR (more complex), and bugs 6+8 can be a third PR.
