# Delta for mcp-server

## ADDED Requirements

### Requirement: MCP Handler Happy-Path Tests

The test suite MUST include happy-path tests for all 10 new MCP handlers, each verifying correct output for valid input.

#### Scenario: composer_require happy path

- GIVEN a mocked `exec.Runner` returning success for `composer require drupal/token`
- WHEN `realHandleComposerRequire` is called with valid package
- THEN the response SHALL contain `success: true` and parsed version

#### Scenario: drush_exec happy path

- GIVEN a mocked `exec.Runner` returning JSON for `drush status`
- WHEN `realHandleDrushExec` is called with `command: "status"`
- THEN the response SHALL contain `success: true` and parsed output

#### Scenario: contrib_upgrade_path happy path

- GIVEN an `httptest.Server` returning valid release XML
- WHEN `realHandleContribUpgradePath` is called with valid module name
- THEN the response SHALL contain `recommended_upgrade` with version info

#### Scenario: upgrade_scan happy path

- GIVEN a `t.TempDir()` with minimal `composer.json` and mocked drush
- WHEN `realHandleUpgradeScan` is called
- THEN the response SHALL contain `total_errors` and `modules` list

#### Scenario: patch_status happy path

- GIVEN a `t.TempDir()` git repo with a patch commit and `composer.json` extra.patches entry
- WHEN `realHandlePatchStatus` is called
- THEN the response SHALL contain `is_applied: true` and `commit_hash`

#### Scenario: patch_rollback happy path

- GIVEN a clean git repo with a patch commit
- WHEN `realHandlePatchRollback` is called
- THEN the response SHALL contain `success: true` and `reverted_commit`

#### Scenario: generate_report happy path

- GIVEN a `t.TempDir()` with scan data
- WHEN `realHandleGenerateReport` is called with `report_type: "both"`
- THEN both `drup-report.json` and `drup-report.md` SHALL be written

#### Scenario: module_info happy path

- GIVEN an `httptest.Server` returning valid module JSON
- WHEN `realHandleModuleInfo` is called
- THEN the response SHALL contain module title, maintainers, and download count

#### Scenario: detect_env happy path

- GIVEN a `t.TempDir()` with `.ddev/` directory
- WHEN `realHandleDetectEnv` is called
- THEN the response SHALL contain `environment: "ddev"` and `command_prefix: ["ddev"]`

#### Scenario: drupal_version_matrix happy path

- GIVEN no external dependencies (static data)
- WHEN `realHandleDrupalVersionMatrix` is called with `drupal_version: "11"`
- THEN the response SHALL contain PHP requirements and support timeline

### Requirement: Helper Function Tests

The test suite MUST test `parseInstalledVersion`, `hasPackage`, and `extractZip` independently.

#### Scenario: parseInstalledVersion extracts version

- GIVEN a `composer.lock` containing `"version": "3.2.1"` for a package
- WHEN `parseInstalledVersion` is called
- THEN it SHALL return `"3.2.1"`

#### Scenario: parseInstalledVersion missing package

- GIVEN a `composer.lock` without the target package
- WHEN `parseInstalledVersion` is called
- THEN it SHALL return empty string

#### Scenario: hasPackage detects installed package

- GIVEN a `composer.json` with `"drupal/token"` in require
- WHEN `hasPackage` is called
- THEN it SHALL return `true`

#### Scenario: hasPackage missing package

- GIVEN a `composer.json` without the target package
- WHEN `hasPackage` is called
- THEN it SHALL return `false`

#### Scenario: extractZip extracts files

- GIVEN a valid `.zip` file in `t.TempDir()`
- WHEN `extractZip` is called
- THEN the destination directory SHALL contain the extracted files

### Requirement: Invalid Input Tests for All New Handlers

The test suite MUST verify that each of the 10 new handlers returns proper errors for invalid input.

#### Scenario: Each handler rejects invalid JSON

- GIVEN a tool call with malformed JSON payload
- WHEN dispatched to any new handler
- THEN the handler SHALL return a parse error without panicking

#### Scenario: Each handler rejects missing required params

- GIVEN a tool call with empty or missing required parameters
- WHEN dispatched to any new handler
- THEN the handler SHALL return an invalid-params error
