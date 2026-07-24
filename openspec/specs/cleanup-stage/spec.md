# Cleanup Stage Specification

## Purpose

Post-validation cleanup stage that removes `upgrade_status` after a successful D11 migration.

## Requirements

### Requirement: Post-Validation Cleanup

The system SHALL execute a cleanup stage (Stage 8) ONLY after the validation stage (Stage 7) exits with code 0. The cleanup stage MUST uninstall `upgrade_status` via drush, remove `drupal/upgrade_status` from `composer.json`, and create an atomic commit.

| Req | Strength | Behavior |
|-----|----------|----------|
| Gate on validate | MUST | Run only if validate exit code == 0 |
| Skip on failure | MUST | Skip entirely with log message if validate fails |
| Drush uninstall | MUST | Run `drush pm:uninstall upgrade_status -y` |
| Composer remove | MUST | Run `composer remove drupal/upgrade_status` |
| Atomic commit | MUST | Commit with message `chore(cleanup): remove upgrade_status post D11 migration` |
| Idempotent | SHOULD | Skip steps for already-removed components |

#### Scenario: Validate passes, cleanup runs

- GIVEN validation stage exited with code 0
- WHEN Stage 8 begins
- THEN the system SHALL run `drush pm:uninstall upgrade_status -y`, then `composer remove drupal/upgrade_status`, then commit with the specified message

#### Scenario: Validate fails, cleanup skipped

- GIVEN validation stage exited with non-zero code
- WHEN Stage 8 is reached
- THEN the system SHALL log "cleanup skipped: validation failed" and exit without modifications

#### Scenario: upgrade_status already removed

- GIVEN `upgrade_status` is not in `composer.json` and not enabled
- WHEN cleanup stage runs
- THEN the system SHALL detect the absent module, skip uninstall/remove steps, and log "cleanup: nothing to do"

#### Scenario: Drush uninstall fails

- GIVEN `drush pm:uninstall upgrade_status -y` returns non-zero
- WHEN cleanup stage runs
- THEN the system SHALL halt cleanup, report the error, and NOT proceed to composer remove or commit
