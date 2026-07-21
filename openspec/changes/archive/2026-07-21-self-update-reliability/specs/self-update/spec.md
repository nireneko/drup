# Delta for Self-Update

## ADDED Requirements

### Requirement: Platform Detection Correctness

The system MUST use the runtime's actual operating system and architecture (`runtime.GOOS`, `runtime.GOARCH`) to select the upgrade release asset. The system MUST NOT honor environment-variable overrides (e.g. `GOOS`/`GOARCH`) for asset selection.

#### Scenario: Correct asset selected for host platform

- GIVEN `drup upgrade` runs on a linux/amd64 host
- WHEN the system selects the release asset to download
- THEN it SHALL select the linux/amd64 archive based on `runtime.GOOS`/`runtime.GOARCH`

#### Scenario: Environment override ignored

- GIVEN `GOOS` and `GOARCH` environment variables are set to values different from the host platform
- WHEN `drup upgrade` runs
- THEN the system SHALL still select the asset matching the actual host `runtime.GOOS`/`runtime.GOARCH`, ignoring the environment variables

### Requirement: Archive Extraction Robustness

The system MUST locate the target binary inside a downloaded tar.gz archive by matching the archive entry's base filename, regardless of the directory depth at which the entry is stored, and MUST extract only that entry directly to its destination path in a single pass.

#### Scenario: Binary at archive root

- GIVEN a tar.gz archive containing the binary at its root (e.g. `drup`)
- WHEN extraction runs
- THEN the system SHALL locate and extract the binary to the destination path with executable permissions

#### Scenario: Binary nested in a subdirectory

- GIVEN a tar.gz archive containing the binary inside a nested subdirectory (e.g. `drup-linux-amd64/drup`)
- WHEN extraction runs
- THEN the system SHALL locate and extract the binary by basename match, regardless of its path depth, to the destination path with executable permissions

#### Scenario: Binary not found in archive

- GIVEN a tar.gz archive that does not contain an entry matching the expected binary basename
- WHEN extraction runs
- THEN the system SHALL abort with an error and leave no partially written destination file

## MODIFIED Requirements

### Requirement: Binary Replacement

The system SHALL replace the current binary with the new version atomically. If replacement fails after the current binary has been backed up, the system SHALL restore the backup so the previous binary remains usable.
(Previously: only the staged `.new` file was removed on rename failure; the backup was never restored, leaving the system without a working binary on failure.)

#### Scenario: Successful replacement

- GIVEN a verified new binary
- WHEN replacement runs
- THEN the system SHALL copy the current binary as backup, move the new binary into place, and make it executable

#### Scenario: Replacement fails, backup restored

- GIVEN the current binary has been copied to a backup path
- WHEN moving the new binary into the final destination fails (e.g. permissions, disk full)
- THEN the system SHALL restore the backup binary to the original path and report the replacement error
- AND the previous binary SHALL remain executable and in place afterward

#### Scenario: Replacement fails and restore also fails

- GIVEN the current binary has been copied to a backup path and moving the new binary into place has failed
- WHEN the system attempts to restore the backup and that restore also fails
- THEN the system SHALL report both the original replacement error and the restore failure
- AND the system SHALL NOT report success or silently leave the binary path in an ambiguous state

### Requirement: Checksum Verification

The system SHALL verify the downloaded binary's checksum before replacing the current binary. This requirement is unchanged by the extraction rewrite and MUST continue to hold.
(Previously: identical behavior; restated here as a regression guard since the extraction path is being rewritten in this change.)

#### Scenario: Valid checksum

- GIVEN a downloaded binary with matching SHA256 checksum
- WHEN verification runs
- THEN the system SHALL proceed with binary replacement

#### Scenario: Invalid checksum

- GIVEN a downloaded binary with mismatched checksum
- WHEN verification runs
- THEN the system SHALL abort the update, delete the downloaded file, and report a security warning
