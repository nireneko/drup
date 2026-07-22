# Delta for mcp-server

## MODIFIED Requirements

### Requirement: scan Tool

The system SHALL expose a `scan` MCP tool that accepts `project_path` and returns classified error JSON. The tool SHALL invoke `drush upgrade_status:analyze` with the `--all` flag to ensure complete analysis.

#### Scenario: scan with valid path

- GIVEN a tool call `scan({project_path: "/path/to/drupal"})`
- WHEN the project has upgrade_status results
- THEN the system SHALL execute `drush upgrade_status:analyze --all --format=json` and return JSON `{errors: {contrib: [...], custom: [...], theme: [...]}}` with error details

#### Scenario: scan with invalid path

- GIVEN a tool call `scan({project_path: "/nonexistent"})`
- THEN the system SHALL return a JSON-RPC error indicating path not found

(Previously: The scan tool invoked `upgrade_status:analyze` without `--all`, potentially returning incomplete results.)

### Requirement: autofix Tool

The system SHALL expose an `autofix` MCP tool that runs drupal-rector and returns a summary. Before running rector, the tool SHALL ensure upgrade_status analysis is current by invoking it with the `--all` flag.

#### Scenario: autofix applies rector

- GIVEN a tool call `autofix({project_path: "/path/to/drupal"})`
- WHEN drupal-rector is available
- THEN the system SHALL run `drush upgrade_status:analyze --all` to refresh analysis, execute drupal-rector, and return JSON `{rector_summary: string, remaining_errors: number}`

(Previously: The autofix tool did not refresh upgrade_status analysis with `--all` before running rector.)

### Requirement: validate Tool

The system SHALL expose a `validate` MCP tool that re-runs scan and returns current error state. The tool SHALL accept an optional `module_name` parameter; when provided, it SHALL analyze only that module, otherwise it SHALL use the `--all` flag for full project analysis.

#### Scenario: validate with zero errors

- GIVEN a tool call `validate({project_path: "/path"})`
- WHEN upgrade_status reports zero errors
- THEN the system SHALL execute `drush upgrade_status:analyze --all` and return `{total_errors: 0, errors: []}`

#### Scenario: validate with remaining errors

- GIVEN a tool call `validate({project_path: "/path"})`
- WHEN errors remain
- THEN the system SHALL execute `drush upgrade_status:analyze --all` and return `{total_errors: N, errors: [...]}` with full error details

#### Scenario: validate scoped to module

- GIVEN a tool call `validate({project_path: "/path", module_name: "mymodule"})`
- WHEN analyzing a specific module
- THEN the system SHALL execute `drush upgrade_status:analyze mymodule` and return results for that module only

(Previously: The validate tool invoked `upgrade_status:analyze` without `--all` or module scoping, potentially returning incomplete results.)

### Requirement: upgrade_scan Tool

The system SHALL expose an `upgrade_scan` MCP tool that performs one-call upgrade_status analysis: install upgrade_status (if missing), enable it (if disabled), run analysis, and return filtered results. Before enabling `upgrade_status`, the tool MUST check for and resolve any `update.settings` configuration conflicts.

#### Scenario: upgrade_scan full lifecycle

- GIVEN a tool call `upgrade_scan({project_path: "/path"})`
- WHEN upgrade_status is not installed
- THEN the system SHALL install it via composer, check for `update.settings` config conflicts, delete conflicting config if present, enable the module, run `drush upgrade_status:analyze --all`, and return `{total_errors: N, modules: [...], upgrade_status_installed: true, upgrade_status_enabled: true}`

#### Scenario: upgrade_scan idempotent

- GIVEN a tool call `upgrade_scan({project_path: "/path"})`
- WHEN upgrade_status is already installed and enabled
- THEN the system SHALL skip install/enable steps, run `drush upgrade_status:analyze --all`, and return analysis results

#### Scenario: upgrade_scan with config conflict

- GIVEN a tool call `upgrade_scan({project_path: "/path"})`
- WHEN `update.settings` configuration exists and blocks module enablement
- THEN the system SHALL delete `update.settings` via `drush config:delete update.settings`, log the action, enable the module, and proceed with analysis

(Previously: The upgrade_scan tool did not handle `update.settings` config conflicts before enabling upgrade_status, causing failures on projects with existing update settings.)
