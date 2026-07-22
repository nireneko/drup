# Delta for Preflight

## ADDED Requirements

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
