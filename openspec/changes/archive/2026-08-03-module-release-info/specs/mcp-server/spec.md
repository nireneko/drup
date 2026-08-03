# Delta for MCP Server

## ADDED Requirements

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

## MODIFIED Requirements

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
