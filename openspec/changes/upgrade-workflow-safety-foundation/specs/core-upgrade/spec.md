# Delta for Core Upgrade

## MODIFIED Requirements

### Requirement: Core Version Update

The system MUST accept only the current Drupal major or its immediate next major as a target. A target equal to the current major MUST return a successful no-op without modifying `composer.json` or executing commands. A lower or skipped major MUST fail before mutation. For the immediate next major, the system MUST update the `drupal/core-recommended` (or `drupal/core`) constraint in `composer.json`.

(Previously: accepted any target version and updated its core constraint.)

#### Scenario: Update to immediate next major

- GIVEN `composer.json` contains `"drupal/core-recommended": "^10.3"`
- WHEN `drup upgrade-core 11` runs
- THEN it MUST update the constraint to `^11` in `composer.json`

#### Scenario: Already at target version

- GIVEN `composer.json` contains `"drupal/core-recommended": "^11.0"`
- WHEN `drup upgrade-core 11` runs
- THEN it MUST return success with "already at target" and make no changes

#### Scenario: Lower target

- GIVEN the current Drupal major is 11
- WHEN `drup upgrade-core 10` runs
- THEN it MUST fail before mutation

#### Scenario: Skipped target

- GIVEN the current Drupal major is 10
- WHEN `drup upgrade-core 12` runs
- THEN it MUST fail before mutation
