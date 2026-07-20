# MCP Server Specification

## Purpose

MCP stdio server wrapping all internal packages as 7 tools for AI agent consumption.

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

The system SHALL expose a `scan` MCP tool that accepts `project_path` and returns classified error JSON.

#### Scenario: scan with valid path

- GIVEN a tool call `scan({project_path: "/path/to/drupal"})`
- WHEN the project has upgrade_status results
- THEN the system SHALL return JSON `{errors: {contrib: [...], custom: [...], theme: [...]}}` with error details

#### Scenario: scan with invalid path

- GIVEN a tool call `scan({project_path: "/nonexistent"})`
- THEN the system SHALL return a JSON-RPC error indicating path not found

### Requirement: autofix Tool

The system SHALL expose an `autofix` MCP tool that runs drupal-rector and returns a summary.

#### Scenario: autofix applies rector

- GIVEN a tool call `autofix({project_path: "/path/to/drupal"})`
- WHEN drupal-rector is available
- THEN the system SHALL return JSON `{rector_summary: string, remaining_errors: number}`

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

The system SHALL expose a `validate` MCP tool that re-runs scan and returns current error state.

#### Scenario: validate with zero errors

- GIVEN a tool call `validate({project_path: "/path"})`
- WHEN upgrade_status reports zero errors
- THEN the system SHALL return `{total_errors: 0, errors: []}`

#### Scenario: validate with remaining errors

- GIVEN a tool call `validate({project_path: "/path"})`
- WHEN errors remain
- THEN the system SHALL return `{total_errors: N, errors: [...]}` with full error details

### Requirement: create_patch Tool

The system SHALL expose a `create_patch` MCP tool for generating patches from deprecation details.

#### Scenario: create_patch generates diff

- GIVEN a tool call `create_patch({module_name: "mymodule", deprecation_details: "..."})`
- WHEN the system can generate a fix
- THEN the system SHALL return `{patch_path: "/path/to/patch", applied: true}`

### Requirement: Tool Schema Validation

The system SHALL validate all tool inputs against their JSON schemas before execution.

#### Scenario: Missing required parameter

- GIVEN a tool call missing a required parameter
- THEN the system SHALL return a JSON-RPC error with code -32602 (invalid params)
