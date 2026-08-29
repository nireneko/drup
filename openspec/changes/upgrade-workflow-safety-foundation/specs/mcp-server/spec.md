# Delta for MCP Server

## MODIFIED Requirements

### Requirement: scan Tool

The `scan` MCP tool SHALL require prepared Upgrade Status, run `drush upgrade_status:analyze --all`, and MUST NOT mutate the project.

(Previously: scan was not read-only.)

#### Scenario: scan with valid prepared path

- GIVEN prepared Upgrade Status and a valid project path
- WHEN `scan` is called
- THEN it MUST return classified analysis JSON without mutation

#### Scenario: scan with invalid path

- GIVEN `scan({project_path: "/nonexistent"})`
- WHEN the tool is called
- THEN it SHALL return a path-not-found error

### Requirement: autofix Tool

The `autofix` MCP tool SHALL run drupal-rector and return its summary. It MUST NOT refresh Upgrade Status analysis.

(Previously: autofix rescanned before rector.)

#### Scenario: autofix applies rector

- GIVEN `autofix` is called and drupal-rector is available
- WHEN the tool runs
- THEN it MUST execute rector without an analysis command

### Requirement: validate Tool

The read-only `validate` MCP tool SHALL analyze the full project or an optional module. It MUST require prepared Upgrade Status and MUST NOT run updates, cache rebuilds, installation, enablement, or configuration changes.

(Previously: validate lacked a read-only contract.)

#### Scenario: validate with zero errors

- GIVEN prepared Upgrade Status reports zero errors
- WHEN `validate` runs without `module_name`
- THEN it SHALL return `{total_errors: 0, errors: []}` without mutation

#### Scenario: validate with remaining errors

- GIVEN prepared Upgrade Status reports findings
- WHEN `validate` runs
- THEN it SHALL return all findings without mutation

#### Scenario: validate scoped to module

- GIVEN prepared Upgrade Status and `module_name: "mymodule"`
- WHEN `validate` runs
- THEN it SHALL analyze only that module without mutation

### Requirement: upgrade_scan Tool

The read-only `upgrade_scan` MCP tool SHALL require prepared Upgrade Status and return filtered analysis results. It MUST NOT install, enable, or configure it.

(Previously: upgrade_scan prepared Upgrade Status.)

#### Scenario: upgrade_scan prepared

- GIVEN Upgrade Status is prepared
- WHEN `upgrade_scan` runs
- THEN it SHALL return analysis results without mutation

#### Scenario: upgrade_scan unprepared

- GIVEN Upgrade Status is unprepared
- WHEN `upgrade_scan` runs
- THEN it MUST return preparation guidance without mutation

#### Scenario: upgrade_scan configuration conflict

- GIVEN Upgrade Status preparation would require changing `update.settings`
- WHEN `upgrade_scan` runs
- THEN it MUST return preparation guidance without changing configuration

### Requirement: drupal_version_matrix Tool

The Drupal/PHP matrix SHALL use numeric version-component comparison for selection and filtering.

(Previously: lookup used string comparison.)

#### Scenario: Lookup by Drupal version

- GIVEN `drupal_version: "11"`
- WHEN the version is in the matrix
- THEN it SHALL return its PHP requirements

#### Scenario: PHP 8.4 selection

- GIVEN `php_version: "8.4"`
- WHEN compatible Drupal versions are selected
- THEN it MUST select the numerically correct compatible major

#### Scenario: Unknown version

- GIVEN `drupal_version: "99"`
- WHEN it is absent from the matrix
- THEN it SHALL return `unknown Drupal version: 99`

### Requirement: Selective Retry for Transient Errors

System SHALL retry transient errors for read-only `scan`, `upgrade_scan`, and `validate`. It MUST NOT retry other tools. Eligible tools MAY retry twice, 1s backoff, and MUST record retries. Logic errors MUST NOT retry.

(Previously: every handler was retry-eligible.)

#### Scenario: Eligible retry succeeds

- GIVEN `scan` has two transient failures then succeeds
- WHEN the server processes the call
- THEN it SHALL return success and record two retries

#### Scenario: Mutator transient error

- GIVEN a mutating tool returns a transient error
- WHEN the server processes the call
- THEN it MUST invoke that handler exactly once

#### Scenario: Logic error

- GIVEN an eligible tool returns `command not found`
- WHEN the server processes the call
- THEN it MUST return failure without retrying

#### Scenario: Eligible retry exhausted

- GIVEN `validate` returns a transient error on all three attempts
- WHEN the server processes the call
- THEN it MUST return failure after three attempts and record two retries
