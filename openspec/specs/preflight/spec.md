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

The system SHALL install required dev dependencies if they are not already present: `drupal/upgrade_status`, `palantirnet/drupal-rector`, `mglaman/phpstan-drupal`.

#### Scenario: All dev deps already installed

- GIVEN all three dev dependencies are present in `composer.json` require-dev
- WHEN preflight checks run
- THEN the system SHALL report all dependencies as present and skip installation

#### Scenario: Missing dev deps

- GIVEN `drupal/upgrade_status` is not in require-dev
- WHEN preflight checks run
- THEN the system SHALL run `composer require --dev drupal/upgrade_status palantirnet/drupal-rector mglaman/phpstan-drupal` and report success

#### Scenario: Composer require fails

- GIVEN network issues or dependency conflicts prevent installation
- WHEN `composer require --dev` is executed
- THEN the system SHALL report the failure with the composer error output and halt
