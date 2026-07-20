# Delta for Installer

## MODIFIED Requirements

### Requirement: Asset Writing

The system SHALL write generated skill files and MCP configuration to each detected agent's native directory using the agent-specific path and format.

#### Scenario: Write to Claude Code

- GIVEN Claude Code is detected
- WHEN the installer writes assets
- THEN the system SHALL write SKILL.md and sub-agent defs to Claude Code's skills directory
- AND the system SHALL write MCP config to `~/.claude/mcp/drup.json` in flat format: `{"command": "<binary-path>", "args": ["mcp"]}`

#### Scenario: Write to OpenCode

- GIVEN OpenCode is detected
- WHEN the installer writes assets
- THEN the system SHALL write SKILL.md and sub-agent defs to OpenCode's skills directory
- AND the system SHALL merge a `"drup"` entry into `~/.config/opencode/opencode.json` under the `"mcp"` key with `{"type": "local", "command": ["<binary-path>", "mcp"]}`

#### Scenario: Write to multiple agents

- GIVEN both Claude Code and OpenCode are detected
- WHEN the installer writes assets
- THEN the system SHALL write to both agents' native directories independently using each agent's correct path and format

### Requirement: OpenCode Config Merge

The system SHALL merge the drup MCP entry into an existing `opencode.json` without corrupting or removing other configuration keys.

#### Scenario: Existing opencode.json with other MCP servers

- GIVEN `~/.config/opencode/opencode.json` exists with an `"mcp"` key containing other server entries
- WHEN the installer merges the drup entry
- THEN the system SHALL preserve all existing keys in `opencode.json`
- AND the system SHALL add or update only the `"drup"` entry under `"mcp"`

#### Scenario: Existing opencode.json with existing drup key

- GIVEN `opencode.json` already contains a `"drup"` entry under `"mcp"`
- WHEN the installer merges the drup entry
- THEN the system SHALL overwrite the existing `"drup"` entry with the new values

#### Scenario: opencode.json does not exist

- GIVEN `~/.config/opencode/opencode.json` does not exist
- WHEN the installer runs
- THEN the system SHALL create `opencode.json` with a top-level `"mcp"` key containing only the `"drup"` entry

#### Scenario: opencode.json is corrupt (invalid JSON)

- GIVEN `~/.config/opencode/opencode.json` exists but contains invalid JSON
- WHEN the installer attempts to merge
- THEN the system SHALL abort the merge
- AND the system SHALL report a clear error indicating the file is corrupt
- AND the system SHALL NOT overwrite or truncate the existing file

#### Scenario: opencode.json is not writable

- GIVEN `~/.config/opencode/opencode.json` exists but the file or directory lacks write permission
- WHEN the installer attempts to merge
- THEN the system SHALL abort the merge
- AND the system SHALL report a permission error with the file path

### Requirement: Claude Code MCP Config Format

The system SHALL write Claude Code MCP config as a per-server file in flat format (no `mcpServers` wrapper).

#### Scenario: Claude template renders flat format

- GIVEN the Claude Code adapter renders the MCP template
- WHEN the binary path is `/usr/local/bin/drup`
- THEN the output SHALL be `{"command": "/usr/local/bin/drup", "args": ["mcp"]}`
- AND the output SHALL NOT contain an `mcpServers` wrapper key

### Requirement: OpenCode MCP Config Format

The system SHALL produce a merge-ready snippet for OpenCode with `type` and `command` as an array.

#### Scenario: OpenCode template renders merge-ready snippet

- GIVEN the OpenCode adapter renders the MCP template
- WHEN the binary path is `/usr/local/bin/drup`
- THEN the snippet SHALL be `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`

#### Scenario: OpenCode merge preserves JSON structure

- GIVEN a valid `opencode.json` with indentation
- WHEN the installer writes the merged result
- THEN the output SHALL be valid JSON with indentation preserved for existing keys

## ADDED Requirements

### Requirement: Template File Format

The system SHALL ship template files in `internal/packaging/templates/{claude,opencode}/.mcp.json` that match each agent's expected format.

#### Scenario: Claude template is flat format

- GIVEN the Claude template at `internal/packaging/templates/claude/.mcp.json`
- WHEN the template is read
- THEN it SHALL contain a flat JSON object with `command` and `args` keys (no `mcpServers` wrapper)

#### Scenario: OpenCode template is snippet format

- GIVEN the OpenCode template at `internal/packaging/templates/opencode/.mcp.json`
- WHEN the template is read
- THEN it SHALL contain a JSON object with `type: "local"` and `command` as an array

## REMOVED Requirements

_None_

## RENAMED Requirements

_None_
