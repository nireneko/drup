# Core Upgrade Specification

## Purpose

The `drup upgrade-core` CLI command performs a deterministic Drupal core version upgrade: composer.json manipulation, dependency resolution, database updates, and result verification.

## Requirements

### Requirement: Version Detection

The system MUST read `composer.json` in the current working directory and extract the current Drupal core version from `require.drupal/core-recommended` or `require.drupal/core`.

#### Scenario: Detect current version

- GIVEN a Drupal project with `composer.json` containing `"drupal/core-recommended": "^10.3"`
- WHEN `drup upgrade-core` reads the project
- THEN it MUST report current constraint `^10.3`

#### Scenario: No composer.json found

- GIVEN a directory without `composer.json`
- WHEN `drup upgrade-core` runs
- THEN it MUST exit with a non-zero code and an error message indicating no composer.json found

### Requirement: Core Version Update

The system MUST accept a target version argument and update the `drupal/core-recommended` (or `drupal/core`) constraint in `composer.json`.

#### Scenario: Update to target version

- GIVEN `composer.json` with `"drupal/core-recommended": "^10.3"`
- WHEN `drup upgrade-core 11` runs
- THEN it MUST update the constraint to `^11` in `composer.json`

#### Scenario: Already at target version

- GIVEN `composer.json` with `"drupal/core-recommended": "^11.0"`
- WHEN `drup upgrade-core 11` runs
- THEN it MUST exit with info message "already at target" and make no changes

### Requirement: Composer Execution

The system MUST run `composer config policy.advisories.block false` before the require command to disable advisory blocking, then run `composer require drupal/core-recommended:^<target> drupal/core:^<target> --with-all-dependencies`, followed by `composer update --with-all-dependencies` to ensure full dependency resolution. When DDEV is detected via `envdetect.Detect()`, the system SHALL prefix all composer commands with `ddev composer` instead of bare `composer`.

(Previously: always used bare `composer` regardless of environment)

#### Scenario: Composer update with advisory bypass

- GIVEN a valid composer.json with updated constraint
- WHEN composer execution runs
- THEN it MUST disable advisory blocking, invoke `composer require` with `--with-all-dependencies`, run `composer update --with-all-dependencies`, and propagate the final exit code

#### Scenario: Composer not available

- GIVEN `composer` is not in PATH
- WHEN composer execution runs
- THEN it MUST exit non-zero with "composer not found" error

#### Scenario: DDEV environment detected

- GIVEN a DDEV project (`.ddev/` directory present)
- WHEN composer execution runs
- THEN the system SHALL execute `ddev composer config policy.advisories.block false`, `ddev composer require ...`, and `ddev composer update ...`

#### Scenario: Non-DDEV environment

- GIVEN a direct composer project (no DDEV)
- WHEN composer execution runs
- THEN the system SHALL use bare `composer` commands as before

### Requirement: Database Update

After successful composer update, the system MUST run `drush updb -y` to apply pending database updates.

#### Scenario: Drush updb succeeds

- GIVEN composer update completed successfully
- WHEN database update runs
- THEN it MUST execute `drush updb -y` and propagate its exit code

#### Scenario: Drush not available

- GIVEN `drush` is not in PATH
- WHEN database update runs
- THEN it MUST exit non-zero with "drush not found" error

### Requirement: Verification

After database update, the system MUST verify the upgrade by running `drush status` and confirming the reported Drupal version matches the target.

#### Scenario: Verification passes

- GIVEN drush updb completed successfully
- WHEN verification runs
- THEN it MUST confirm Drupal version matches target and exit 0

#### Scenario: Verification fails

- GIVEN drush status reports a version different from target
- WHEN verification runs
- THEN it MUST exit non-zero with a version mismatch error

### Requirement: Dry Run Mode

The system SHOULD support a `--dry-run` flag that previews changes without modifying files or executing commands.

#### Scenario: Dry run output

- GIVEN `--dry-run` flag is passed
- WHEN `drup upgrade-core 11 --dry-run` runs
- THEN it MUST print the planned composer.json change and commands without executing them

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
