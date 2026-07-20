# Release Automation Specification

## Purpose

Goreleaser configuration and GitHub Actions workflows that produce versioned, checksummed, multi-platform archives on tag push, plus a CI workflow for PR validation.

## Requirements

### Requirement: GoReleaser Build Configuration

The system SHALL provide a `.goreleaser.yaml` that cross-compiles the `drup` binary for linux, darwin, and windows on amd64 and arm64 architectures.

The build MUST inject the version via ldflags: `-s -w -X github.com/nireneko/drup/internal/app.Version={{.Version}}`.

#### Scenario: All target platforms built

- GIVEN a tag `v0.2.0` is pushed
- WHEN goreleaser runs
- THEN binaries SHALL be produced for linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64, windows/arm64

#### Scenario: Version embedded in binary

- GIVEN a goreleaser build completes for any target
- WHEN the binary runs with `--version`
- THEN the output SHALL contain the tag version without the `v` prefix

### Requirement: Archive Naming

The system SHALL name release archives using the template `drup_{version}_{os}_{arch}.tar.gz` for non-windows targets and `drup_{version}_{os}_{arch}.zip` for windows targets.

#### Scenario: Linux archive name

- GIVEN a build for linux/amd64 at version 0.2.0
- WHEN the archive is created
- THEN the filename SHALL be `drup_0.2.0_linux_amd64.tar.gz`

#### Scenario: Windows archive name

- GIVEN a build for windows/amd64 at version 0.2.0
- WHEN the archive is created
- THEN the filename SHALL be `drup_0.2.0_windows_amd64.zip`

### Requirement: Checksum Generation

The system SHALL generate a `checksums.txt` file containing SHA256 hashes for every release archive. Each line MUST follow the format `{sha256}  {filename}` (two-space separated).

#### Scenario: Checksums file present

- GIVEN a release with 6 platform archives
- WHEN goreleaser completes
- THEN `checksums.txt` SHALL be uploaded as a release asset with exactly 6 lines

#### Scenario: Checksum format

- GIVEN any archive in the release
- WHEN its checksum line is parsed
- THEN the line SHALL match `{64-char-hex}  {archive-filename}`

### Requirement: Release Workflow

The system SHALL provide `.github/workflows/release.yml` that triggers goreleaser on version tag pushes.

The workflow MUST trigger on `push: tags: - "v*"` and pass `GITHUB_TOKEN` to goreleaser.

#### Scenario: Tag push triggers release

- GIVEN the release workflow is present
- WHEN a tag matching `v*` is pushed
- THEN the workflow SHALL run goreleaser and create a GitHub Release

#### Scenario: Non-tag push skipped

- GIVEN the release workflow is present
- WHEN a push to a branch (not a tag) occurs
- THEN the release workflow SHALL NOT trigger

### Requirement: CI Workflow

The system SHALL provide `.github/workflows/ci.yml` that runs on pull requests and pushes to validate code quality.

The workflow MUST run `gofmt`, `go vet ./...`, and `go test ./...`.

#### Scenario: CI on pull request

- GIVEN the CI workflow is present
- WHEN a pull request is opened
- THEN the workflow SHALL run gofmt, go vet, and go test

#### Scenario: CI failure blocks

- GIVEN a PR with a gofmt violation
- WHEN the CI workflow runs
- THEN the workflow SHALL report failure
