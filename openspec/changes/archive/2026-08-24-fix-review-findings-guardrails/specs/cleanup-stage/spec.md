# Delta for Cleanup Stage

## MODIFIED Requirements

### Requirement: Post-Validation Cleanup

The system SHALL execute a cleanup stage (Stage 8) ONLY after the validation stage (Stage 7) exits with code 0. The cleanup stage MUST uninstall `upgrade_status` via drush, remove `drupal/upgrade_status` from `composer.json`, and create an atomic commit. The cleanup implementation MUST accept an `io.Writer` for its output instead of printing to the global `os.Stdout`, and MUST stage only the files it declares changed (via the `gitops` scoped-commit helper) instead of `git add -A`.

| Req | Strength | Behavior |
|-----|----------|----------|
| Gate on validate | MUST | Run only if validate exit code == 0 |
| Skip on failure | MUST | Skip entirely with log message if validate fails |
| Drush uninstall | MUST | Run `drush pm:uninstall upgrade_status -y` |
| Composer remove | MUST | Run `composer remove drupal/upgrade_status` |
| Scoped commit | MUST | Stage only the declared changed paths via the scoped-commit helper; MUST NOT use `git add -A` |
| Writer-injected output | MUST | Write progress/output to the caller-supplied `io.Writer`, not global `os.Stdout` |
| Atomic commit | MUST | Commit with message `chore(cleanup): remove upgrade_status post D11 migration` |
| Idempotent | SHOULD | Skip steps for already-removed components |

(Previously: printed via `fmt.Println` to global `os.Stdout`, causing an MCP handler to swap `os.Stdout` and risk deadlock on large output; staged the commit with `git add -A`.)

#### Scenario: Validate passes, cleanup runs

- GIVEN validation stage exited with code 0
- WHEN Stage 8 begins
- THEN the system SHALL run `drush pm:uninstall upgrade_status -y`, then `composer remove drupal/upgrade_status`, then commit with the specified message, writing all output to the supplied `io.Writer`

#### Scenario: Validate fails, cleanup skipped

- GIVEN validation stage exited with non-zero code
- WHEN Stage 8 is reached
- THEN the system SHALL write "cleanup skipped: validation failed" to the supplied writer and exit without modifications

#### Scenario: upgrade_status already removed

- GIVEN `upgrade_status` is not in `composer.json` and not enabled
- WHEN cleanup stage runs
- THEN the system SHALL detect the absent module, skip uninstall/remove steps, and write "cleanup: nothing to do" to the supplied writer

#### Scenario: Drush uninstall fails

- GIVEN `drush pm:uninstall upgrade_status -y` returns non-zero
- WHEN cleanup stage runs
- THEN the system SHALL halt cleanup, report the error, and NOT proceed to composer remove or commit

#### Scenario: MCP handler no longer swaps stdout

- GIVEN the `cleanup` MCP tool invokes `RunCleanup`
- WHEN cleanup produces output, including output larger than a pipe buffer
- THEN the handler SHALL capture it via the injected `io.Writer` without swapping `os.Stdout` and without risk of deadlock

#### Scenario: Scoped staging excludes unrelated changes

- GIVEN an unrelated uncommitted file exists outside the cleanup's declared paths (composer.json, composer.lock, and drush-modified config)
- WHEN cleanup creates its commit
- THEN the system SHALL stage only the declared paths via the scoped-commit helper and SHALL NOT include the unrelated file
