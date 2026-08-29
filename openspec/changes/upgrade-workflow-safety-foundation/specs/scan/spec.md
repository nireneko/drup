# Delta for Scan

## MODIFIED Requirements

### Requirement: realHandleScan Requires Prepared Upgrade Status

The `realHandleScan` handler MUST NOT install, enable, disable, configure, or cache-rebuild Upgrade Status. It MUST verify that Upgrade Status is installed and enabled before analysis, and MUST return actionable guidance to run `prepare_upgrade_status` when that prerequisite is unmet. The `realHandleUpgradeScan` handler MUST follow the same read-only prerequisite contract.

(Previously: scan and upgrade_scan auto-enabled an installed but disabled module, including configuration deletion and cache rebuild.)

#### Scenario: Prepared module

- GIVEN Upgrade Status is installed and enabled
- WHEN `drup_scan` runs
- THEN it MUST run analysis without preparation commands

#### Scenario: Disabled module

- GIVEN Upgrade Status is installed but disabled
- WHEN `drup_scan` runs
- THEN it MUST return preparation guidance without mutation

#### Scenario: Missing module

- GIVEN Upgrade Status is absent from Composer
- WHEN `drup_scan` runs
- THEN it MUST return preparation guidance without attempting installation
