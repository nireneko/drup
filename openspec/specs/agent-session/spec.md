# Agent Session Specification

## Purpose

Session lifecycle binding an MCP client to a single canonical Drupal project root for the life of the server process, and the guard middleware that gates every mutating MCP tool on a valid session.

## Requirements

### Requirement: Session Opening

The system SHALL expose a `session_open` MCP tool that resolves the canonical project root for `project_path` and binds a session to the server process for that root.

#### Scenario: Successful session open

- GIVEN a tool call `session_open({project_path: "/path/to/drupal"})`
- WHEN the path resolves to a valid Drupal project root (composer.json + web root markers present)
- THEN the system SHALL bind a session to that canonical root and return `{session_active: true, root: "<canonical>"}`

#### Scenario: Reject non-Drupal directory

- GIVEN a tool call `session_open({project_path: "/tmp/not-drupal"})`
- WHEN no composer.json or web root markers are found
- THEN the system SHALL return an error and SHALL NOT open a session

#### Scenario: Reopening replaces the prior session

- GIVEN a session is already bound to root A
- WHEN `session_open` is called with a valid path resolving to root B
- THEN the system SHALL replace the bound session with root B for the remainder of the process lifetime

### Requirement: Canonical Root Resolution

The system SHALL resolve project roots through one shared canonical-root helper (absolute path, symlink-evaluated, no `..` traversal), replacing the separate `ValidateProjectPath`, `backup.validateProject`, and envdetect ad hoc checks.

#### Scenario: Symlinked path resolves to real target

- GIVEN `project_path` is a symlink to a Drupal project directory
- WHEN the canonical-root helper resolves it
- THEN the system SHALL use `filepath.EvalSymlinks` and bind/compare sessions against the resolved real path

#### Scenario: Path traversal rejected

- GIVEN `project_path` contains `..` segments or is not absolute
- WHEN any tool resolves the project root
- THEN the system SHALL reject the call with an error before performing any file or session operation

### Requirement: Guard Middleware Enforcement

The system SHALL wrap every mutating tool handler at the single `WireMCPTools` registration point with guard middleware that requires an open session whose canonical root matches the tool's `project_path`. The guarded set SHALL include `apply_patch`, `core_upgrade_apply`, `composer_require`, `create_patch`, `cleanup`, `patch_rollback`, `custom_compat_fix`, `contrib_compat_patch`, `contrib_allow_lenient`, `test_backup_restore`, `test_backup_delete`, and the nested composer-install path inside `upgrade_scan`.

#### Scenario: Mutating call with matching session proceeds

- GIVEN a valid session bound to canonical root R
- WHEN a mutating tool is called with `project_path` resolving to R
- THEN the guard SHALL allow the call to reach the real handler

#### Scenario: Mutating call with mismatched or absent session

- GIVEN no session is open, or the open session is bound to a different canonical root
- WHEN a mutating tool is called
- THEN the guard SHALL block the real handler and apply the force-dry-run/refuse partition defined by `mcp-server`, returning an error that names the `session_open` flow

#### Scenario: Nested mutation inside upgrade_scan is guarded

- GIVEN `upgrade_scan` reaches its internal composer-install step for `upgrade_status`
- WHEN no valid session is bound to the target root
- THEN the guard SHALL block that internal mutation the same as a direct `composer_require` call

### Requirement: Backup-Freshness Gate

The guard middleware SHALL verify a backup manifest newer than the session-open time (or within a configured freshness window) exists before allowing a mutating call to reach its handler.

#### Scenario: Fresh backup present

- GIVEN a backup manifest was created after the session opened
- WHEN a mutating tool is called
- THEN the guard SHALL allow the call to proceed

#### Scenario: No fresh backup

- GIVEN no backup manifest newer than the freshness window exists
- WHEN a mutating tool is called
- THEN the guard SHALL block the call with an error naming `test_backup_create` as the remediation

### Requirement: Runtime Opt-Out

The system SHALL support `DRUP_ALLOW_UNSAFE=1` to bypass session and backup-freshness guarding, logging a warning on every bypassed call.

#### Scenario: Opt-out set

- GIVEN `DRUP_ALLOW_UNSAFE=1` is set in the server environment
- WHEN a mutating tool is called without a valid session
- THEN the system SHALL allow the call to proceed and SHALL log a warning identifying the bypassed guard

#### Scenario: Opt-out unset

- GIVEN `DRUP_ALLOW_UNSAFE` is unset or not `1`
- WHEN a mutating tool is called without a valid session
- THEN the system SHALL apply the standard guard behavior with no bypass
