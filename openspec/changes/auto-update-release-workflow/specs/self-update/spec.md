# Delta for Self-Update

## MODIFIED Requirements

### Requirement: Version Check

The system SHALL check GitHub Releases for newer versions of the `drup` binary and select the asset matching the current OS and architecture.

`CheckLatest` MUST filter release assets by `{os}_{arch}` suffix instead of returning the first asset. If no matching asset exists for the current platform, the system SHALL report an error.

(Previously: `CheckLatest` returned `Assets[0].BrowserDownloadURL` regardless of platform.)

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

#### Scenario: Platform-specific asset selection

- GIVEN a release with assets for linux/amd64, darwin/arm64, and windows/amd64
- WHEN `drup upgrade` runs on darwin/arm64
- THEN `CheckLatest` SHALL return the URL for the `drup_{version}_darwin_arm64.tar.gz` asset

#### Scenario: No matching asset for platform

- GIVEN a release with no asset matching the current OS/arch
- WHEN `drup upgrade` runs
- THEN the system SHALL report "no release asset found for {os}/{arch}" and exit 1

## ADDED Requirements

### Requirement: Owner Resolution

The system SHALL use the correct GitHub repository owner when querying for releases. The owner string in `internal/app/commands.go` MUST match the actual repository owner (`nireneko`).

#### Scenario: Correct owner used

- GIVEN the binary runs `drup upgrade`
- WHEN the GitHub API is queried
- THEN the request SHALL target `repos/nireneko/drup/releases/latest`

#### Scenario: Owner mismatch prevented

- GIVEN the owner string is set to an incorrect value
- WHEN tests run
- THEN a test SHALL verify the owner constant matches the expected repository owner
