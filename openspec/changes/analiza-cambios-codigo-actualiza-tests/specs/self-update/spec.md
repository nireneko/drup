# Delta for self-update

## ADDED Requirements

### Requirement: Backup Location Regression

The test suite MUST verify that `BackupBinary` writes the backup to `~/.drup/backups/` (not the same directory as the running binary) and that `RestoreBinary` reads from that same location.

#### Scenario: Backup written to ~/.drup/backups/

- GIVEN a test with `HOME` set to `t.TempDir()`
- WHEN `BackupBinary` runs
- THEN the backup file SHALL exist at `<HOME>/.drup/backups/drup.bak`
- AND no backup file SHALL exist adjacent to the source binary

#### Scenario: Restore reads from ~/.drup/backups/

- GIVEN a backup at `<HOME>/.drup/backups/drup.bak`
- WHEN `RestoreBinary` runs
- THEN the file SHALL be copied back to the original binary path

### Requirement: Cross-Device Copy Regression

The test suite MUST verify that binary replacement uses `copyFile` (content copy) rather than `os.Rename`, ensuring cross-device compatibility.

#### Scenario: Replacement works across filesystem boundaries

- GIVEN source and destination on different mount points (simulated via `t.TempDir()` separation)
- WHEN `ReplaceBinary` runs
- THEN the destination SHALL contain the source content
- AND the operation SHALL NOT fail with "invalid cross-device link"

### Requirement: ETXTBSY Atomic Staging Regression

The test suite MUST verify that the new binary is staged via a temp file then atomically renamed, preventing ETXTBSY when the running binary is in use.

#### Scenario: Staged via temp file then renamed

- GIVEN a running binary path
- WHEN the replacement flow stages the new binary
- THEN the system SHALL write to a temporary file first
- AND THEN atomically rename the temp file to the destination
- AND the running binary SHALL NOT be opened for writing directly

### Requirement: Archive Extraction from .tar.gz Regression

The test suite MUST verify that `extractBinaryFromTarGz` correctly extracts a binary nested inside a subdirectory within a `.tar.gz` archive.

#### Scenario: Nested binary extracted from tar.gz

- GIVEN a `.tar.gz` archive containing `drup-linux-amd64/drup`
- WHEN `extractBinaryFromTarGz` runs
- THEN the extracted file SHALL be executable and match the original binary content
