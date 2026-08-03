# MCP Server Specification

## Purpose

MCP stdio server wrapping all internal packages as 25 tools for AI agent consumption.

## Requirements

### Requirement: MCP Server Transport

The system SHALL implement an MCP server using stdio transport, accepting JSON-RPC requests on stdin and writing responses to stdout.

#### Scenario: Server startup

- GIVEN the binary is invoked with `drup mcp`
- WHEN the MCP server starts
- THEN the system SHALL listen on stdin for JSON-RPC messages and respond on stdout

#### Scenario: Invalid JSON input

- GIVEN the server receives malformed JSON on stdin
- THEN the system SHALL respond with a JSON-RPC error with code -32700 (parse error)

### Requirement: scan Tool

The system SHALL expose a `scan` MCP tool that accepts `project_path` and returns classified error JSON. The tool SHALL invoke `drush upgrade_status:analyze` with the `--all` flag to ensure complete analysis. The `--format=json` flag SHALL NOT be used; the parser handles plain-text output.

#### Scenario: scan with valid path

- GIVEN a tool call `scan({project_path: "/path/to/drupal"})`
- WHEN the project has upgrade_status results
- THEN the system SHALL execute `drush upgrade_status:analyze --all` and return JSON `{errors: {contrib: [...], custom: [...], theme: [...]}}` with error details

#### Scenario: scan with invalid path

- GIVEN a tool call `scan({project_path: "/nonexistent"})`
- THEN the system SHALL return a JSON-RPC error indicating path not found

### Requirement: autofix Tool

The system SHALL expose an `autofix` MCP tool that runs drupal-rector and returns a summary. Before running rector, the tool SHALL ensure upgrade_status analysis is current by invoking it with the `--all` flag.

#### Scenario: autofix applies rector

- GIVEN a tool call `autofix({project_path: "/path/to/drupal"})`
- WHEN drupal-rector is available
- THEN the system SHALL run `drush upgrade_status:analyze --all` to refresh analysis, execute drupal-rector, and return JSON `{rector_summary: string, remaining_errors: number}`

### Requirement: contrib_check Tool

The system SHALL expose a `contrib_check` MCP tool that queries Drupal.org for D11 compatibility.

#### Scenario: contrib_check for compatible module

- GIVEN a tool call `contrib_check({module_machine_name: "token"})`
- WHEN the module has a D11 release
- THEN the system SHALL return `{has_d11_release: true, latest_version: "8.x-1.x", compatible_branches: [...]}`

#### Scenario: contrib_check for incompatible module

- GIVEN a tool call `contrib_check({module_machine_name: "old_module"})`
- WHEN no D11 release exists
- THEN the system SHALL return `{has_d11_release: false, compatible_branches: []}`

### Requirement: issue_patches Tool

The system SHALL expose an `issue_patches` MCP tool accepting `issue_nid` or `module_name`.

#### Scenario: issue_patches by module

- GIVEN a tool call `issue_patches({module_name: "token"})`
- WHEN the module has issues with patches
- THEN the system SHALL return `[{url, status, date, is_patch}]` sorted by RTBC priority

#### Scenario: issue_patches by NID

- GIVEN a tool call `issue_patches({issue_nid: "1234567"})`
- THEN the system SHALL return patches for that specific issue

### Requirement: apply_patch Tool

The system SHALL expose an `apply_patch` MCP tool that downloads and applies a patch.

#### Scenario: apply_patch success

- GIVEN a tool call `apply_patch({patch_url: "https://...", project_path: "/path"})`
- WHEN the patch downloads and applies cleanly
- THEN the system SHALL return `{applied: true, commit_hash: "abc123", error: null}`

#### Scenario: apply_patch conflict

- GIVEN a tool call `apply_patch({patch_url: "https://...", project_path: "/path"})`
- WHEN `git apply` fails due to conflicts
- THEN the system SHALL return `{applied: false, commit_hash: null, error: "conflict description"}`

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

### Requirement: create_patch Tool

The system SHALL expose a `create_patch` MCP tool for generating patches from deprecation details.

#### Scenario: create_patch generates diff

- GIVEN a tool call `create_patch({module_name: "mymodule", deprecation_details: "..."})`
- WHEN the system can generate a fix
- THEN the system SHALL return `{patch_path: "/path/to/patch", applied: true}`

### Requirement: detect_env Tool

The system SHALL expose a `detect_env` MCP tool that detects the Drupal development environment (ddev, lando, docker4drupal, direct) for a given project path and caches the result for all subsequent tool calls.

#### Scenario: detect_env with ddev project

- GIVEN a tool call `detect_env({project_path: "/path/to/ddev/project"})`
- WHEN the project contains a `.ddev/` directory
- THEN the system SHALL return `{environment: "ddev", command_prefix: ["ddev"], detected_at: "<timestamp>"}`

#### Scenario: detect_env with lando project

- GIVEN a tool call `detect_env({project_path: "/path/to/lando/project"})`
- WHEN the project contains a `.lando.yml` file
- THEN the system SHALL return `{environment: "lando", command_prefix: ["lando"], detected_at: "<timestamp>"}`

#### Scenario: detect_env with direct installation

- GIVEN a tool call `detect_env({project_path: "/path/to/drupal"})`
- WHEN the project contains `composer.json` but no environment markers
- THEN the system SHALL return `{environment: "direct", command_prefix: [], detected_at: "<timestamp>"}`

#### Scenario: detect_env with unknown environment

- GIVEN a tool call `detect_env({project_path: "/nonexistent"})`
- WHEN the path does not exist or is not a directory
- THEN the system SHALL return `{environment: "unknown", command_prefix: []}` with an error message

#### Scenario: detect_env cache bypass

- GIVEN a tool call `detect_env({project_path: "/path", force_detect: true})`
- WHEN `force_detect` is true
- THEN the system SHALL bypass the cache and re-run detection

### Requirement: composer_require Tool

The system SHALL expose a `composer_require` MCP tool that safely wraps `composer require` with input validation, dry-run pre-check for conflicts, timeout handling, and structured output parsing.

#### Scenario: composer_require success

- GIVEN a tool call `composer_require({project_path: "/path", package: "drupal/token:^1.0"})`
- WHEN the package installs successfully
- THEN the system SHALL return `{success: true, installed_version: "1.0.0", stdout: "...", stderr: "", exit_code: 0}`

#### Scenario: composer_require conflict

- GIVEN a tool call `composer_require({project_path: "/path", package: "drupal/incompatible"})`
- WHEN the dry-run detects a conflict
- THEN the system SHALL return `{success: false, installed_version: "", stdout: "", stderr: "conflict details", exit_code: 1}`

#### Scenario: composer_require invalid package format

- GIVEN a tool call `composer_require({project_path: "/path", package: "invalid;package"})`
- WHEN the package name fails validation
- THEN the system SHALL return an error before executing any command

### Requirement: drush_exec Tool

The system SHALL expose a `drush_exec` MCP tool that safely wraps drush execution with command blocklist, automatic environment-aware prefixing, `--root` flag injection, and structured output parsing.

#### Scenario: drush_exec success

- GIVEN a tool call `drush_exec({project_path: "/path", command: "status", format: "json"})`
- WHEN the command executes successfully
- THEN the system SHALL return `{success: true, output: {...}, stderr: "", exit_code: 0}`

#### Scenario: drush_exec blocked command

- GIVEN a tool call `drush_exec({project_path: "/path", command: "sql-drop"})`
- WHEN the command is in the blocklist
- THEN the system SHALL return `{success: false, error: "command 'sql-drop' is blocked for safety", exit_code: -1}`

#### Scenario: drush_exec shell metacharacters rejected

- GIVEN a tool call `drush_exec({project_path: "/path", command: "status; rm -rf /"})`
- WHEN the command contains shell metacharacters
- THEN the system SHALL return an error before executing

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

### Requirement: contrib_upgrade_path Tool

The system SHALL expose a `contrib_upgrade_path` MCP tool that resolves the recommended contrib module version for a target Drupal major version.

#### Scenario: contrib_upgrade_path finds stable release

- GIVEN a tool call `contrib_upgrade_path({module_machine_name: "token", current_drupal_version: "10", target_drupal_version: "11"})`
- WHEN a stable release compatible with D11 exists
- THEN the system SHALL return `{module: "token", recommended_upgrade: {version: "...", is_stable: true, ...}, alternative_versions: [...]}`

#### Scenario: contrib_upgrade_path no compatible releases

- GIVEN a tool call `contrib_upgrade_path({module_machine_name: "old_module", current_drupal_version: "10", target_drupal_version: "11"})`
- WHEN no releases are compatible with D11
- THEN the system SHALL return `{module: "old_module", recommended_upgrade: null, alternative_versions: []}`

### Requirement: patch_status Tool

The system SHALL expose a `patch_status` MCP tool that checks whether a specific patch is already applied by inspecting `composer.json` `extra.patches` and git history.

#### Scenario: patch_status applied patch

- GIVEN a tool call `patch_status({project_path: "/path", patch_url: "https://..."})`
- WHEN the patch is registered in composer.json and found in git log
- THEN the system SHALL return `{is_applied: true, commit_hash: "abc123", registered_in_composer: true, patch_info: {...}}`

#### Scenario: patch_status not applied

- GIVEN a tool call `patch_status({project_path: "/path", patch_url: "https://..."})`
- WHEN the patch is not registered and not in git log
- THEN the system SHALL return `{is_applied: false, commit_hash: "", registered_in_composer: false, patch_info: null}`

### Requirement: patch_rollback Tool

The system SHALL expose a `patch_rollback` MCP tool that cleanly reverts a previously applied patch: git-revert the commit, remove from `composer.json`, and run `composer update`.

#### Scenario: patch_rollback success

- GIVEN a tool call `patch_rollback({project_path: "/path", patch_url: "https://...", composer_package: "drupal/token"})`
- WHEN the patch is applied and working tree is clean
- THEN the system SHALL revert the commit, remove from composer.json, run composer update, and return `{success: true, reverted_commit: "...", removed_from_composer: true}`

#### Scenario: patch_rollback dirty working tree

- GIVEN a tool call `patch_rollback({project_path: "/path", patch_url: "https://...", composer_package: "drupal/token"})`
- WHEN the working tree has uncommitted changes
- THEN the system SHALL return `{success: false, error: "working tree is dirty"}`

### Requirement: generate_report Tool

The system SHALL expose a `generate_report` MCP tool that generates upgrade reports in JSON and/or Markdown format by wrapping the existing `internal/report` package.

#### Scenario: generate_report both formats

- GIVEN a tool call `generate_report({project_path: "/path", report_type: "both"})`
- WHEN the report is generated
- THEN the system SHALL write `drup-report.json` and `drup-report.md` to project_path and return `{success: true, json_report_path: "...", markdown_report_path: "...", summary: {...}}`

### Requirement: module_info Tool

The system SHALL expose a `module_info` MCP tool that fetches module metadata and health indicators from Drupal.org for decision support.

#### Scenario: module_info fetches metadata

- GIVEN a tool call `module_info({module_machine_name: "token"})`
- WHEN the module exists on Drupal.org
- THEN the system SHALL return `{module: "token", title: "Token", maintainers: [...], downloads: N, last_release: "...", open_issues: N}`

#### Scenario: module_info not found

- GIVEN a tool call `module_info({module_machine_name: "nonexistent"})`
- WHEN the module does not exist on Drupal.org
- THEN the system SHALL return an error `"module 'nonexistent' not found"`

### Requirement: drupal_version_matrix Tool

The system SHALL expose a `drupal_version_matrix` MCP tool that provides a Drupal/PHP version compatibility matrix for preflight validation using static data.

#### Scenario: drupal_version_matrix lookup by Drupal version

- GIVEN a tool call `drupal_version_matrix({drupal_version: "11"})`
- WHEN the version is in the static map
- THEN the system SHALL return `{drupal_version: "11", php_requirements: {minimum: "8.3", recommended: "8.4"}, supported_until: "TBA", upgrade_path: {next_major: ""}}`

#### Scenario: drupal_version_matrix unknown version

- GIVEN a tool call `drupal_version_matrix({drupal_version: "99"})`
- WHEN the version is not in the static map
- THEN the system SHALL return an error `"unknown Drupal version: 99"`

### Requirement: module_release_info Tool

The system SHALL expose a `module_release_info` MCP tool that returns curated release information for a module, combining project-level maintenance status with per-release derived fields. The tool SHALL accept a required `module_machine_name` parameter and an optional `core_version` filter parameter, and SHALL be wired via the same 3-file pattern (schema in `server.go`, placeholder in `tools.go`, real handler + registration in `mcp_tools.go`) used by other tools.

#### Scenario: module_release_info returns maintenance status and releases

- GIVEN a tool call `module_release_info({module_machine_name: "pathauto"})`
- WHEN the module exists on Drupal.org
- THEN the system SHALL return `{module, found: true, maintenance_status, releases: [{version, tag, core_compatibility, release_type: [...], insecure, security_covered, date}, ...]}`

#### Scenario: module_release_info with core_version filter

- GIVEN a tool call `module_release_info({module_machine_name: "token", core_version: "11"})`
- WHEN releases exist with a range satisfying major 11
- THEN the system SHALL return only published releases matching that range

#### Scenario: module_release_info for unknown module

- GIVEN a tool call `module_release_info({module_machine_name: "nonexistent"})`
- WHEN the module does not exist on Drupal.org
- THEN the system SHALL return a not-found result via the `status`/`message`/`suggestion` shape, not a JSON-RPC error

### Requirement: New Tool Registration

The system SHALL register newly added MCP tool handlers. The system registers 25 tool handlers via `defaultTools()`; `WireMCPTools` wires real handlers for all of them, bringing the total to 29 registered tool handlers and schemas.

#### Scenario: New tools are callable

- GIVEN the MCP server starts with the new handlers registered
- WHEN an agent calls any of: `detect_env`, `upgrade_scan`, `composer_require`, `drush_exec`, `contrib_upgrade_path`, `patch_status`, `patch_rollback`, `generate_report`, `module_info`, `drupal_version_matrix`, `module_release_info`
- THEN the system SHALL route the call to the correct handler and return a valid JSON-RPC response

#### Scenario: New tools validate their own input and surface errors as -32603

- GIVEN a tool call to any new tool with missing or invalid required parameters
- WHEN the handler runs
- THEN the handler SHALL return a descriptive Go error, which the server surfaces as a JSON-RPC error with code -32603 (internal error)
- Code -32602 (invalid params) is reserved for malformed JSON in the request's `params` field itself; the server does not perform pre-handler JSON-Schema validation against `toolRegistry[...].Required` for any tool

#### Scenario: Existing tools unchanged

- GIVEN the new tools are registered
- WHEN an agent calls any of the tools that existed before them
- THEN the behavior SHALL be identical to before their addition

### Requirement: Tool Handler Registration Points

The system SHALL register new tool handlers in both `internal/mcp/tools.go` (placeholder/schema definitions) and `internal/app/mcp_tools.go` (real handler wiring).

#### Scenario: Schema definitions

- GIVEN the MCP server initialization
- WHEN tool schemas are built for the `tools/list` response
- THEN all new tools SHALL appear with correct input schemas as defined above

#### Scenario: Handler wiring

- GIVEN a tool call arrives for a new tool
- WHEN the handler dispatch runs
- THEN the system SHALL invoke the correct internal package function

### Requirement: Tool Schema Validation

The system SHALL expose complete JSON Schema `inputSchema` for all 25 tools in the `tools/list` response. Each tool's schema SHALL declare `properties` (with name, type, description) and `required` fields. The system does not perform pre-handler JSON-Schema validation against these `required` fields for any tool; each handler validates its own parameters and returns a descriptive Go error, which the server surfaces as a JSON-RPC error with code -32603 (internal error). Code -32602 (invalid params) is reserved for malformed JSON in the request's `params` field itself.

| Req | Strength | Behavior |
|-----|----------|----------|
| Schema properties | MUST | Each tool MUST declare `properties` with parameter definitions |
| Required fields | MUST | Each tool MUST declare `required` array for mandatory params |
| No empty schemas | MUST NOT | Return `{"type": "object"}` with no properties |

#### Scenario: Agent discovers scan parameters

- GIVEN an agent calls `tools/list`
- WHEN the response is received
- THEN the `scan` tool schema SHALL include `properties: {project_path: {type: "string", description: "..."}}` and `required: ["project_path"]`

#### Scenario: All 25 tools have schemas

- GIVEN the MCP server starts
- WHEN `tools/list` is called
- THEN all 25 tools SHALL have non-empty `inputSchema.properties`

#### Scenario: Missing required parameter surfaces as -32603

- GIVEN a tool call missing a required parameter
- WHEN the handler runs
- THEN the handler SHALL return a descriptive Go error, which the server surfaces as a JSON-RPC error with code -32603 (internal error), not -32602

### Requirement: Drush Error Context

The system SHALL wrap all drush execution failures with structured error context including the full command string, exit code, stderr (full), and stdout (truncated to 500 chars). This helper (`drushExecError`) SHALL be used by RunScan and all MCP tool handlers that invoke drush.

#### Scenario: Drush non-zero exit

- GIVEN drush exit 1 with stderr
- THEN error SHALL include command, exit code, stderr

#### Scenario: Parse failure

- GIVEN drush exit 0 but unparseable output
- THEN error SHALL include command and truncated stdout (500 chars)

### Requirement: Uniform Response Envelope

The system SHALL wrap every MCP tool response (success or error) in a uniform envelope: `{"status":"pass|fail","summary":"...","payload":{...}}`. The wrapper lives in `handleToolCall` (one location); per-handler code is NOT modified. The original tool-specific payload is preserved intact inside `envelope.payload`.

**MCP protocol extension (deliberate)**: Errors are sent as `{"status":"fail","summary":"..."}` in the result channel, NOT as JSON-RPC errors. This is a deliberate extension of the MCP protocol for this server. Rationale: the retrospective's core complaint was that errors were invisible. JSON-RPC errors are transport-level signals that some MCP clients swallow silently. A `{"status":"fail"}` in the result channel is always visible to the orchestrator model.

| Req | Strength | Behavior |
|-----|----------|----------|
| Envelope wrap (success) | MUST | `{"status":"pass","summary":"...","payload":{original}}` |
| Envelope wrap (error) | MUST | `{"status":"fail","summary":"<error message>"}` in result channel |
| Error transport | MUST NOT | Do NOT send errors as JSON-RPC errors; send as `{"status":"fail"}` in result |
| Payload preservation | MUST | Original tool payload byte-for-byte inside `envelope.payload` |
| Single wrapper location | MUST | Wrapper in `handleToolCall` only; per-handler code unchanged |
| `deriveSummary` helper | SHOULD | Best-effort summary: extract `summary`/`success`/`total_errors` fields if present, else generic "Tool {name} completed" |

#### Scenario: Success envelope wrap

- GIVEN a handler returns `{"foo":"bar"}` (no error)
- WHEN `handleToolCall` processes the response
- THEN the client SHALL receive `{"status":"pass","summary":"...","payload":{"foo":"bar"}}`
- AND `payload.foo` SHALL equal `"bar"`

#### Scenario: Error envelope — NOT a JSON-RPC error

- GIVEN a handler returns `(nil, error("command not found: xyz"))`
- WHEN `handleToolCall` processes the response
- THEN the client SHALL receive `{"status":"fail","summary":"command not found: xyz"}` in the result channel
- AND the response SHALL NOT be a JSON-RPC error (no `error` field at JSON-RPC level)
- AND the JSON-RPC `id` SHALL match the request `id`

#### Scenario: Payload preservation — complex object

- GIVEN a handler returns a complex `scan.ScanResult` with nested `errors.contrib`, `errors.custom`, `errors.theme` arrays
- WHEN `handleToolCall` wraps the response
- THEN `envelope.payload` SHALL contain the full `scan.ScanResult` byte-for-byte

### Requirement: Selective Retry for Transient Errors

The system SHALL wrap `handler(p.Arguments)` with retry logic for transient errors only. The system SHALL retry on transport/timeout errors, never on logic errors (exit code ≠ 0, "command not found", etc.). Max 2 retries (3 total attempts) with 1s base exponential backoff. Each retry SHALL be recorded via `metrics.Collector.RecordRetry()`.

| Req | Strength | Behavior |
|-----|----------|----------|
| Retry on transient errors | MUST | Retry on: "context deadline exceeded", "connection refused", "i/o timeout", "broken pipe", "timeout" |
| No retry on logic errors | MUST NOT | Do NOT retry on: "exit status", "command not found", "no commands defined", "already exists", "does not exist" |
| Max retries | MUST | 2 retries (3 total attempts) |
| Backoff | MUST | 1s base exponential backoff (1s, 2s) |
| Metrics recording | MUST | Record each retry via `metrics.Collector.RecordRetry()` |
| Retry exhausted | MUST | Return `{"status":"fail","summary":"... after 3 attempts ..."}` |

#### Scenario: Retry on transient error — succeeds after 2 failures

- GIVEN a handler fails with "context deadline exceeded" on attempts 1 and 2, succeeds on attempt 3
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"pass",...}`
- AND `metrics.Collector` SHALL show 2 retries recorded

#### Scenario: No retry on real error

- GIVEN a handler fails with "command not found: xyz"
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"fail","summary":"command not found: xyz"}` immediately
- AND `metrics.Collector` SHALL show 0 retries recorded
- AND the handler SHALL be invoked exactly once

#### Scenario: Retry exhausted

- GIVEN a handler fails with "context deadline exceeded" on all 3 attempts
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"fail","summary":"... after 3 attempts ..."}`
- AND `metrics.Collector` SHALL show 2 retries recorded

### Requirement: Tool Count Assertion

The test `TestWireMCPTools_AllToolsRegistered` SHALL assert the exact count of registered MCP tools (currently 29) and list missing/extra tools on failure for debuggability.

| Req | Strength | Behavior |
|-----|----------|----------|
| Assert tool count | MUST | Assert `len(server.tools) == 29` (or expose `ToolCount() int` if `tools` is unexported) |
| Diagnostic on failure | MUST | List missing/extra tools if count is wrong |

#### Scenario: All 29 tools registered — test passes

- GIVEN all 29 MCP tools are registered
- WHEN `TestWireMCPTools_AllToolsRegistered` runs
- THEN the test SHALL pass

#### Scenario: Tool accidentally unregistered — test fails with diagnostic

- GIVEN 27 tools registered (one missing)
- WHEN `TestWireMCPTools_AllToolsRegistered` runs
- THEN the test SHALL fail
- AND the failure message SHALL list the missing tool name(s)
