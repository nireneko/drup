# Delta for Core Upgrade

## ADDED Requirements

### Requirement: MCP allow_dirty Parameter Removal

The `core_upgrade_apply` MCP tool schema MUST NOT expose an `allow_dirty` (or equivalent) parameter. The CLI `drup upgrade-core --allow-dirty` flag SHALL remain unchanged and unaffected by this removal.

#### Scenario: MCP schema has no allow_dirty

- GIVEN an agent calls `tools/list`
- WHEN the `core_upgrade_apply` schema is inspected
- THEN it SHALL NOT declare an `allow_dirty` property

#### Scenario: Dirty tree via MCP without session

- GIVEN a dirty working tree and no valid `agent-session`
- WHEN `core_upgrade_apply` is called via MCP
- THEN the system SHALL apply the force-dry-run partition (native `dry_run` param forced true) rather than accepting an `allow_dirty` override

#### Scenario: CLI flag unaffected

- GIVEN a dirty working tree
- WHEN `drup upgrade-core --allow-dirty` runs via CLI
- THEN the system SHALL behave exactly as before this change

### Requirement: Unified Canonical Root Validation

All `internal/coreupgrade` entry points (CLI and MCP) SHALL use the single canonical-root validation helper shared with `agent-session`, replacing the package's standalone `ValidateProjectPath` checks.

#### Scenario: Consistent symlink resolution

- GIVEN a project path reached through a symlink
- WHEN both the CLI command and the `core_upgrade_check`/`core_upgrade_apply` MCP tools resolve it
- THEN both SHALL resolve to the same canonical real path

#### Scenario: Path traversal rejected uniformly

- GIVEN a `project_path` containing `..` segments
- WHEN any core-upgrade CLI or MCP entry point resolves it
- THEN the system SHALL reject the call with the same validation error used elsewhere in the codebase
