# Delta for Installer

## ADDED Requirements

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

## MODIFIED Requirements

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
