# Delta for preflight

## MODIFIED Requirements

### Requirement: Dev Dependency Installation

The system SHALL install required dev dependencies if they are not already present: `drupal/upgrade_status`, `palantirnet/drupal-rector`, `mglaman/phpstan-drupal`. Before enabling `upgrade_status`, the system MUST check for and resolve any `update.settings` configuration conflicts by deleting the conflicting configuration.

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

(Previously: The system enabled `upgrade_status` without checking for `update.settings` config conflicts, causing preflight to crash on projects with existing update settings.)
