# Preflight Specification

## Purpose

Pre-flight checks before Drupal upgrade: git clean verification, tool detection, core version identification, and dev dependency installation.

## Requirements

### Requirement: Environment Detection Terminal State

The system SHALL detect the project's execution environment during preflight as one of: `ddev`, `lando`, `docker4drupal`, or `direct` (composer.json present, no container marker). If none of these are detected, the system SHALL halt with an explicit "unsupported project manager/environment" error and SHALL NOT proceed to any later pipeline stage.

#### Scenario: DDEV project detected

- GIVEN a project directory containing a `.ddev` directory
- WHEN preflight runs environment detection
- THEN the system SHALL report the environment as `ddev` and proceed with the `ddev` command prefix

#### Scenario: Direct composer project detected

- GIVEN a project directory with a `composer.json` but no `.ddev`, `.lando.yml`, or Drupal-referencing `docker-compose.yml`
- WHEN preflight runs environment detection
- THEN the system SHALL report the environment as `direct` and proceed with no command prefix

#### Scenario: Unsupported environment halts the pipeline

- GIVEN a project directory with none of `.ddev`, `.lando.yml`, a Drupal `docker-compose.yml`, or `composer.json`
- WHEN preflight runs environment detection
- THEN the system SHALL report an "unsupported project manager/environment" terminal error and SHALL halt before any subsequent stage runs

### Requirement: Git Clean Check

The system SHALL verify the git working tree is clean before proceeding with any upgrade operations.

#### Scenario: Clean working tree

- GIVEN a Drupal project with no uncommitted changes
- WHEN preflight checks run
- THEN the system SHALL report git status as clean and proceed

#### Scenario: Dirty working tree

- GIVEN a Drupal project with uncommitted changes or untracked files
- WHEN preflight checks run
- THEN the system SHALL report the dirty state and halt with a list of uncommitted files

#### Scenario: Not a git repository

- GIVEN a directory that is not a git repository
- WHEN preflight checks run
- THEN the system SHALL report "not a git repository" and halt

### Requirement: Composer Detection

The system SHALL detect whether `composer` is available on the system PATH.

#### Scenario: Composer available

- GIVEN `composer` is on PATH
- WHEN preflight checks run
- THEN the system SHALL report composer as detected with its version

#### Scenario: Composer missing

- GIVEN `composer` is not on PATH
- WHEN preflight checks run
- THEN the system SHALL report composer as missing and halt

### Requirement: Drush Detection

The system SHALL detect whether `drush` is available on the system PATH or as a project dependency.

#### Scenario: Drush available globally

- GIVEN `drush` is on PATH
- WHEN preflight checks run
- THEN the system SHALL report drush as detected with its version

#### Scenario: Drush available as project dependency

- GIVEN `drush` is not on PATH but `vendor/bin/drush` exists in the project
- WHEN preflight checks run
- THEN the system SHALL report drush as detected via vendor/bin

#### Scenario: Drush missing

- GIVEN `drush` is neither on PATH nor in vendor/bin
- WHEN preflight checks run
- THEN the system SHALL report drush as missing and halt

### Requirement: Core Version Detection

The system SHALL detect the Drupal core version from `composer.lock`.

#### Scenario: Drupal 10 project

- GIVEN a `composer.lock` with `drupal/core` at version 10.x
- WHEN preflight checks run
- THEN the system SHALL report core version as 10.x.y

#### Scenario: Drupal 9 project

- GIVEN a `composer.lock` with `drupal/core` at version 9.x
- WHEN preflight checks run
- THEN the system SHALL report core version as 9.x.y

#### Scenario: Missing composer.lock

- GIVEN no `composer.lock` file exists
- WHEN preflight checks run
- THEN the system SHALL report "composer.lock not found" and halt

### Requirement: Dev Dependency Installation

The system SHALL install required dev dependencies if they are not already present: `drupal/upgrade_status`, `palantirnet/drupal-rector`, `mglaman/phpstan-drupal`. Before enabling `upgrade_status`, the system MUST check for and resolve any `update.settings` configuration conflicts by deleting the conflicting configuration. The system SHALL detect PHP 8.4+ and auto-patch `settings.php` to suppress `E_DEPRECATED` after the DDEV include block.

| Req | Strength | Behavior |
|-----|----------|----------|
| PHP 8.4 detection | MUST | Run `php -v` (or env-equivalent) and parse version |
| Auto-patch settings.php | MUST | Append `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` |
| Patch placement | MUST | Insert after DDEV include block (or at end if no DDEV) |
| Backup before patch | SHOULD | Create `settings.php.bak` before modifying |
| Idempotent | MUST | Skip patch if suppression line already present |

#### Scenario: All dev deps already installed

- GIVEN all three dev dependencies are present in `composer.json` require-dev
- WHEN preflight checks run
- THEN the system SHALL report all dependencies as present and skip installation

#### Scenario: Missing dev deps

- GIVEN `drupal/upgrade_status` is not in require-dev
- WHEN preflight checks run
- THEN the system SHALL run `composer require --dev drupal/upgrade_status palantirnet/drupal-rector mglaman/phpstan-drupal` and report success

#### Scenario: Config conflict before enable

- GIVEN `update.settings` configuration exists in the active config
- WHEN the system attempts to enable `upgrade_status`
- THEN the system SHALL delete `update.settings` configuration via `drush config:delete update.settings` before enabling the module, log the deletion, and proceed with enablement

#### Scenario: Composer require fails

- GIVEN network issues or dependency conflicts prevent installation
- WHEN `composer require --dev` is executed
- THEN the system SHALL report the failure with the composer error output and halt

#### Scenario: PHP 8.4 project under DDEV

- GIVEN a DDEV project running PHP 8.4
- WHEN `drup preflight` runs
- THEN the system SHALL detect PHP 8.4, append deprecation suppression to `settings.php` after the DDEV include, and proceed

#### Scenario: PHP 8.3 project

- GIVEN a project running PHP 8.3
- WHEN preflight runs
- THEN the system SHALL skip the settings.php patch and proceed normally

#### Scenario: Patch already applied

- GIVEN `settings.php` already contains the deprecation suppression line
- WHEN preflight runs
- THEN the system SHALL detect the existing line and skip patching

### Requirement: Core Readiness Check

The system SHALL verify that `composer.json` constraints allow Drupal 11 before proceeding with the upgrade pipeline. The system MUST parse all `core_version_requirement` values in `web/modules/custom/*/` and `web/themes/custom/*/*.info.yml` files and compare them against the target Drupal version.

| Req | Strength | Behavior |
|-----|----------|----------|
| Constraint check | MUST | Parse `composer.json` `require.drupal/core` constraint and verify it permits target major version |
| Module scan | MUST | Scan all custom module/theme `.info.yml` files for `core_version_requirement` |
| Blockers report | MUST | List every module/theme with incompatible `core_version_requirement` |
| Early abort | MUST | Halt pipeline with exit code and blockers report if any incompatibility found |

#### Scenario: All constraints allow Drupal 11

- GIVEN `composer.json` with `"drupal/core": "^10.3 || ^11"` and all `.info.yml` files with `core_version_requirement: ">=10.0 || ^11"`
- WHEN preflight runs core readiness check
- THEN the system SHALL report "core readiness: OK" and proceed

#### Scenario: Composer constraint blocks Drupal 11

- GIVEN `composer.json` with `"drupal/core": "^10.3"` (no `^11` allowance)
- WHEN preflight runs core readiness check
- THEN the system SHALL halt with message "composer.json constraint ^10.3 does not permit Drupal 11" and list the constraint

#### Scenario: Custom modules with incompatible core_version_requirement

- GIVEN 3 custom modules where 2 have `core_version_requirement: "<11"` and 1 has `">=10.0"`
- WHEN preflight runs core readiness check
- THEN the system SHALL halt and report: `blockers: [{module: "mod_a", constraint: "<11"}, {module: "mod_b", constraint: "<11"}]`

#### Scenario: No custom modules exist

- GIVEN a project with empty `web/modules/custom/` and `web/themes/custom/`
- WHEN preflight runs core readiness check
- THEN the system SHALL skip the module scan and report "no custom code to check"

### Requirement: Semver-Based PHP Compatibility Check

The system SHALL replace string-based PHP version comparison with proper semantic version comparison. The system MUST parse PHP version constraints (e.g., `">=8.1"`, `"^8.2"`) and compare them against the detected PHP version using semver rules.

| Req | Strength | Behavior |
|-----|----------|----------|
| Semver parsing | MUST | Parse version strings into comparable semver components |
| Constraint evaluation | MUST | Evaluate constraint operators (`>=`, `^`, `~`, `||`) correctly |
| No string comparison | MUST NOT | Use lexicographic string comparison for versions |
| Implementation | SHOULD | Use stdlib-only minimal semver implementation (no external deps) |

#### Scenario: Compatible PHP version

- GIVEN PHP 8.3 detected and constraint `">=8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `true`

#### Scenario: Incompatible PHP version

- GIVEN PHP 8.0 detected and constraint `">=8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `false`

#### Scenario: Caret constraint

- GIVEN PHP 8.2 detected and constraint `"^8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `true` (8.2 is within ^8.1 range)

#### Scenario: Invalid version string

- GIVEN an unparseable version string `"abc"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return an error, not panic
