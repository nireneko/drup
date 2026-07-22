# Delta for core-upgrade

## MODIFIED Requirements

### Requirement: Composer Execution

The system MUST run `composer config policy.advisories.block false` before the require command to disable advisory blocking, then run `composer require drupal/core-recommended:^<target> drupal/core:^<target> --with-all-dependencies`, followed by `composer update --with-all-dependencies` to ensure full dependency resolution.

#### Scenario: Composer update with advisory bypass

- GIVEN a valid composer.json with updated constraint
- WHEN composer execution runs
- THEN it MUST disable advisory blocking, invoke `composer require` with `--with-all-dependencies`, run `composer update --with-all-dependencies`, and propagate the final exit code

#### Scenario: Composer not available

- GIVEN `composer` is not in PATH
- WHEN composer execution runs
- THEN it MUST exit non-zero with "composer not found" error

(Previously: The system ran `composer require` with `--update-with-dependencies` but did not disable advisory blocking or run a full `composer update -W`, causing failures on major version bumps.)

### Requirement: Backup

The system SHOULD create a backup of `composer.json` before modifying it, and MUST remove the backup file after successful completion of all upgrade steps.

#### Scenario: Backup created and cleaned on success

- GIVEN a valid composer.json
- WHEN `drup upgrade-core` modifies it and completes successfully
- THEN a backup file `composer.json.bak` MUST be created during modification and MUST be removed after all steps succeed

#### Scenario: Backup retained on failure

- GIVEN a valid composer.json
- WHEN `drup upgrade-core` fails during execution
- THEN the backup file `composer.json.bak` MUST remain for rollback purposes

(Previously: The system created `composer.json.bak` but never cleaned it up, leaving backup files after successful upgrades.)
