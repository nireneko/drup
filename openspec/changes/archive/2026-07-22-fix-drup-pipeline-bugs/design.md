# Design: Fix Drup Pipeline Bugs

## Technical Approach

Six targeted fixes across two files, grouped into four independent commits. Each fix modifies the exact call sites identified in the real Drupal 10.6→11.4 upgrade failure. No new abstractions — just correct arguments and sequencing at existing `drupexec.Run` / `execRunFn` call sites.

## Architecture Decisions

| Decision | Options | Choice | Rationale |
|----------|---------|--------|-----------|
| `--all` flag placement | Add to all 4 sites vs. helper function | Inline at each site | 4 sites, each with slightly different arg construction. A helper adds indirection for no gain — the args are already visible in context. |
| Validate scoping | Always `--all` vs. module name when available | Module name when `params.Module != ""`, else `--all` | Validate already has module-scoped filtering in Go code post-parse. Passing the module to drush avoids analyzing the entire project when only one module is being validated. |
| Composer advisory bypass | `composer config` before require vs. env var | `execRunFn("composer", "config", "policy.advisories.block", "false")` before require | Persistent config change is safer than env var — env vars don't survive subprocess chains in all shells. The config change is intentional and documented. |
| Config conflict resolution | Delete `update.settings` before enable vs. catch error and retry | Pre-emptive `drush config:delete update.settings` before `drush en` | Simpler control flow, no error-message parsing needed. Only deletes the specific known-conflicting config key. |
| Backup cleanup | `defer os.Remove` vs. explicit remove at end | `defer os.Remove(backupPath)` immediately after `os.WriteFile` | defer guarantees cleanup on all exit paths (success AND early returns from drush updb failure, etc.). Zero risk of forgetting a path. |

## Data Flow

### Fix Group 1: `--all` flag (4 sites)

```
RunScan / realHandleScan / realHandleAutofix:
  drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--all", "--format=json")
                                                        ^^^^^^^^ added

realHandleValidate:
  if params.Module != "":
    drupexec.Run("drush", "-r", path, "upgrade_status:analyze", params.Module, "--format=json")
  else:
    drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--all", "--format=json")
```

### Fix Group 2: Composer upgrade flow

```
RunUpgradeCore (commands.go:656-674):

  os.WriteFile(backupPath, ...)
  defer os.Remove(backupPath)                    ← NEW: cleanup

  execRunFn("composer", "config",                ← NEW: advisory bypass
            "policy.advisories.block", "false")

  execRunFn("composer", "require",               ← MODIFIED: add -W, --no-update
            "drupal/core-recommended:^11.0",
            "drupal/core-composer-scaffold:^11.0",
            "drupal/core-project-message:^11.0",
            "-W", "--no-update")

  execRunFn("composer", "update", "-W")          ← NEW: full resolve

  execRunFn("drush", "updb", "-y")              ← unchanged
  execRunFn("drush", "status", "--format=json")  ← unchanged
```

### Fix Group 3: Config conflict (2 sites)

```
RunPreflight (commands.go:469) & realHandleUpgradeScan (mcp_tools.go:602):

  BEFORE: drush en upgrade_status -y
  AFTER:  drush config:delete update.settings   ← NEW: remove conflict
          drush en upgrade_status -y
```

### Fix Group 4: Error message improvement

```
commands.go:642:
  BEFORE: "core upgrade failed: %s"
  AFTER:  "core upgrade failed (checkpoint: %s): %s"
          with applyResult.RollbackCheckpoint
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modify | Add `--all` to `RunScan:61`; add config:delete before `drush en` in `RunPreflight:469-485`; rewrite composer flow in `RunUpgradeCore:656-674` with `-W`, advisory bypass, `composer update -W`, defer cleanup, improved error message |
| `internal/app/mcp_tools.go` | Modify | Add `--all` to `realHandleScan:78`, `realHandleAutofix:122`; add module-scoped or `--all` to `realHandleValidate:204`; add config:delete before `drush en` in `realHandleUpgradeScan:602-612` |
| `internal/app/commands_test.go` | Modify | Update `TestRunUpgradeCore_Integration` mock to handle new composer call sequence (config, require with -W, update -W); update backup assertion (file should NOT exist after success) |

## Interfaces / Contracts

No new interfaces. All changes are argument-level modifications to existing `drupexec.Run` and `execRunFn` calls.

The `execRunFn` variable signature remains:
```go
var execRunFn = drupexec.Run
// func(string, ...string) (string, string, int, error)
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `--all` flag present in all 4 drush analyze calls | Existing test patterns: mock `drupexec.Run`, assert args contain `"--all"` |
| Unit | Composer call sequence: config → require (-W, --no-update) → update (-W) | Extend `TestRunUpgradeCore_Integration` mock switch to track 3 composer calls with correct args |
| Unit | `config:delete update.settings` called before `drush en upgrade_status` | Mock `drupexec.Run`, assert call ordering |
| Unit | Backup file removed after successful upgrade | Add `os.Stat(backupPath)` assertion expecting `os.IsNotExist` |
| Unit | Error message includes checkpoint SHA on failure | Assert error string contains checkpoint value |
| Integration | `go vet ./...` clean | CI gate |
| Integration | `go test ./...` all pass | CI gate |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. All fixes are backward-compatible argument additions. The `composer config policy.advisories.block false` change persists in the target project's composer config — this is intentional and documented in upgrade output.

## Open Questions

- [ ] Should `composer config policy.advisories.block false` be reverted after successful upgrade? Current approach leaves it set — acceptable per proposal risk assessment.
