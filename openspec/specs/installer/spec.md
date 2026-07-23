# Installer Specification

## Purpose

Detect installed AI agents, write skill files and MCP configuration to native directories, manage backups and state tracking.

## Requirements

### Requirement: Agent Detection

The system SHALL detect which AI agents are installed on the system by checking known configuration paths.

#### Scenario: Detect Claude Code

- GIVEN Claude Code is installed with config at `~/.claude/`
- WHEN agent detection runs
- THEN the system SHALL report Claude Code as detected with its config directory

#### Scenario: Detect OpenCode

- GIVEN OpenCode is installed with config at `~/.config/opencode/`
- WHEN agent detection runs
- THEN the system SHALL report OpenCode as detected

#### Scenario: No agents detected

- GIVEN no known agent config directories exist
- WHEN agent detection runs
- THEN the system SHALL report "no agents detected" and suggest installing one

### Requirement: Asset Writing

The system SHALL write generated skill files and MCP configuration to each detected agent's native directory using the agent-specific path and format.

The system SHALL return `[]SyncFileResult` from `Install()` with one entry per synced file (absolute path + status).

The system SHALL skip writing when a file's status is `unchanged`.

(Previously: `Install()` wrote all files unconditionally and returned only `error`.)

#### Scenario: Write to Claude Code

- GIVEN Claude Code is detected
- WHEN the installer writes assets
- THEN the system SHALL write SKILL.md and sub-agent defs to Claude Code's skills directory
- AND write MCP config to `~/.claude/mcp/drup.json` in flat format
- AND return a `SyncFileResult` per file with correct status

#### Scenario: Write to OpenCode

- GIVEN OpenCode is detected
- WHEN the installer writes assets
- THEN the system SHALL write SKILL.md and sub-agent defs to OpenCode's skills directory
- AND merge a `"drup"` entry into `~/.config/opencode/opencode.json`
- AND return a `SyncFileResult` per file with correct status

#### Scenario: Write to multiple agents

- GIVEN both Claude Code and OpenCode are detected
- WHEN the installer writes assets
- THEN the system SHALL write to both agents independently
- AND return `SyncFileResult` entries covering all files for all agents

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

### Requirement: Config Backup

The system SHALL backup existing configuration files before overwriting them.

The system SHALL create a backup only when a file's status is `new` or `modified`. The system SHALL NOT backup when status is `unchanged`.

(Previously: backup triggered unconditionally before every overwrite, with no change detection.)

#### Scenario: Backup before write

- GIVEN an existing MCP config file differing from new output
- WHEN the installer overwrites it
- THEN the system SHALL create a backup copy (tar.gz) before writing
- AND the `SyncFileResult` SHALL have status `modified`

#### Scenario: No backup when unchanged

- GIVEN an existing MCP config matching new output byte-for-byte
- WHEN the installer processes that file
- THEN the system SHALL NOT create a backup
- AND SHALL NOT write to the file
- AND `SyncFileResult` SHALL have status `unchanged`

#### Scenario: Backup retention

- GIVEN multiple installer runs over time
- WHEN backups accumulate
- THEN the system SHALL retain the 5 most recent backups and prune older ones

### Requirement: State Tracking

The system SHALL maintain a `state.json` file tracking installation state.

#### Scenario: Track installed assets

- GIVEN a successful installation
- WHEN state.json is written
- THEN the system SHALL record: installed agents, file paths written, installation timestamp, and binary version

#### Scenario: State reset

- GIVEN state.json is deleted
- WHEN the installer runs
- THEN the system SHALL treat it as a fresh install with no prior state

### Requirement: Sync File Result Reporting

The system SHALL return `[]SyncFileResult` from `Install()`, each entry containing the absolute file path and a status: `new`, `modified`, or `unchanged`.

The system SHALL determine status by comparing intended write content against existing file bytes before writing.

#### Scenario: First install — all files new

- GIVEN no agent files exist on disk
- WHEN `Install()` runs
- THEN every `SyncFileResult` SHALL have status `new`

#### Scenario: Re-run with no changes — all unchanged

- GIVEN agent files match current template output
- WHEN `Install()` runs again
- THEN every `SyncFileResult` SHALL have status `unchanged`
- AND the system SHALL NOT rewrite any files

#### Scenario: Template change — file modified

- GIVEN an agent file differs from current template output
- WHEN `Install()` runs
- THEN that file's `SyncFileResult` SHALL have status `modified`
- AND the system SHALL overwrite the file

#### Scenario: MCP config uses post-merge content for comparison

- GIVEN an OpenCode `opencode.json` with other MCP entries
- WHEN the installer computes the drup merge result
- THEN comparison SHALL use the fully merged JSON, not the raw template snippet
- AND status SHALL be `unchanged` if merged output matches existing file byte-for-byte
