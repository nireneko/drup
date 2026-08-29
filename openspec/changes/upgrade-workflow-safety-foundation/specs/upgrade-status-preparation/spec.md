# Upgrade Status Preparation Specification

## Purpose

Provide an explicit mutating operation that prepares Upgrade Status before read-only analysis.

## Requirements

### Requirement: Explicit Upgrade Status Preparation

The system MUST expose `prepare_upgrade_status` as the only operation in this domain that installs or enables Upgrade Status. It MUST install the package when absent, resolve an `update.settings` conflict before enablement, enable the module, and rebuild caches.

#### Scenario: Prepare an uninstalled module

- GIVEN a Drupal project without Upgrade Status in Composer
- WHEN `prepare_upgrade_status` runs
- THEN it MUST install, enable, and cache-rebuild Upgrade Status

#### Scenario: Prepare a disabled installed module

- GIVEN Upgrade Status is installed but disabled and `update.settings` conflicts
- WHEN `prepare_upgrade_status` runs
- THEN it MUST remove the conflict before enabling the module

#### Scenario: Prepare an enabled module

- GIVEN Upgrade Status is installed and enabled
- WHEN `prepare_upgrade_status` runs
- THEN it MUST complete without reinstalling or re-enabling it
