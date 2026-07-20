# Self-Update Specification

## Purpose

Binary upgrade via GitHub Releases with checksum verification and deferred sync pattern.

## Requirements

### Requirement: Version Check

The system SHALL check GitHub Releases for newer versions of the `drup` binary.

#### Scenario: Newer version available

- GIVEN the current binary is v0.1.0 and GitHub has v0.2.0
- WHEN `drup upgrade` runs
- THEN the system SHALL report the available version and prompt for confirmation

#### Scenario: Already up to date

- GIVEN the current binary matches the latest GitHub release
- WHEN `drup upgrade` runs
- THEN the system SHALL report "already up to date" and exit 0

#### Scenario: GitHub unreachable

- GIVEN GitHub API is unreachable
- WHEN `drup upgrade` runs
- THEN the system SHALL report the connection error and exit 1

### Requirement: Checksum Verification

The system SHALL verify the downloaded binary's checksum before replacing the current binary.

#### Scenario: Valid checksum

- GIVEN a downloaded binary with matching SHA256 checksum
- WHEN verification runs
- THEN the system SHALL proceed with binary replacement

#### Scenario: Invalid checksum

- GIVEN a downloaded binary with mismatched checksum
- WHEN verification runs
- THEN the system SHALL abort the update, delete the downloaded file, and report a security warning

### Requirement: Binary Replacement

The system SHALL replace the current binary with the new version atomically.

#### Scenario: Successful replacement

- GIVEN a verified new binary
- WHEN replacement runs
- THEN the system SHALL rename the current binary as backup, move the new binary into place, and make it executable

#### Scenario: Replacement fails

- GIVEN the binary replacement fails (permissions, disk full)
- WHEN replacement runs
- THEN the system SHALL restore the backup binary and report the error

### Requirement: Deferred Sync

The system SHALL trigger a deferred sync after binary upgrade to update agent skill files and MCP config.

#### Scenario: Post-upgrade sync

- GIVEN a successful binary upgrade from v0.1.0 to v0.2.0
- WHEN the new binary runs for the first time
- THEN the system SHALL detect the version mismatch in state.json and run the installer to sync assets

#### Scenario: Sync skipped when up to date

- GIVEN state.json matches the current binary version
- WHEN the binary starts
- THEN the system SHALL skip the sync and proceed normally
