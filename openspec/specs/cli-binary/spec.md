# CLI Binary Specification

## Purpose

Go CLI binary (`drup`) providing 6 commands for Drupal upgrade automation. Manual dispatch via `os.Args`, stdlib only (no cobra).

## Requirements

### Requirement: Command Dispatch

The system SHALL dispatch CLI commands using manual `os.Args` parsing without third-party CLI frameworks.

#### Scenario: Valid command invocation

- GIVEN the binary is invoked with `drup <command> [args]`
- WHEN `<command>` matches one of: init, scan, fix, contrib, issue, report
- THEN the system SHALL execute the corresponding handler and exit 0

#### Scenario: Unknown command

- GIVEN the binary is invoked with `drup <unknown>`
- WHEN `<unknown>` does not match any registered command
- THEN the system SHALL print usage information to stderr and exit 1

#### Scenario: No arguments

- GIVEN the binary is invoked with `drup` and no arguments
- THEN the system SHALL print usage information to stdout and exit 0

### Requirement: Init Command

The system SHALL provide a `drup init` command that initializes a Drupal project for upgrade automation.

#### Scenario: Init in valid Drupal project

- GIVEN the current directory contains a `composer.json` with `drupal/core` dependency
- WHEN `drup init` is executed
- THEN the system SHALL verify project structure and output initialization confirmation

#### Scenario: Init outside Drupal project

- GIVEN the current directory does not contain a `composer.json`
- WHEN `drup init` is executed
- THEN the system SHALL print an error to stderr and exit 1

### Requirement: Scan Command

The system SHALL provide a `drup scan <path>` command that runs `upgrade_status:analyze` and outputs structured JSON.

#### Scenario: Scan valid project

- GIVEN a Drupal project path with `upgrade_status` installed
- WHEN `drup scan /path/to/project` is executed
- THEN the system SHALL output JSON with errors classified by type (contrib/custom/theme)

#### Scenario: Scan with missing path argument

- GIVEN `drup scan` is invoked without a path
- THEN the system SHALL print "usage: drup scan <path>" to stderr and exit 1

### Requirement: Fix Command

The system SHALL provide a `drup fix <path>` command that runs drupal-rector on the target project.

#### Scenario: Fix applies rector rules

- GIVEN a Drupal project path with `drupal-rector` installed
- WHEN `drup fix /path/to/project` is executed
- THEN the system SHALL run rector and output a summary of changes and remaining errors

### Requirement: Contrib Command

The system SHALL provide a `drup contrib <module>` command that checks Drupal.org for D11 compatibility.

#### Scenario: Check module with D11 release

- GIVEN a module machine name with a D11-compatible release on Drupal.org
- WHEN `drup contrib <module>` is executed
- THEN the system SHALL output JSON with `{has_d11_release: true, latest_version, compatible_branches}`

#### Scenario: Check module without D11 release

- GIVEN a module machine name without a D11-compatible release
- WHEN `drup contrib <module>` is executed
- THEN the system SHALL output JSON with `{has_d11_release: false}` and list available issue patches

### Requirement: Issue Command

The system SHALL provide a `drup issue <module_or_nid>` command that extracts patch/diff/MR links from Drupal.org issues.

#### Scenario: Issue lookup by module name

- GIVEN a module machine name with open issues containing patches
- WHEN `drup issue <module>` is executed
- THEN the system SHALL output JSON array of `[{url, status, date, is_patch}]` sorted by RTBC priority

#### Scenario: Issue lookup by NID

- GIVEN a Drupal.org issue NID
- WHEN `drup issue <nid>` is executed
- THEN the system SHALL output patch/diff/MR links for that specific issue

### Requirement: Report Command

The system SHALL provide a `drup report <path>` command that generates JSON and markdown reports.

#### Scenario: Generate report

- GIVEN a Drupal project path with scan results available
- WHEN `drup report /path/to/project` is executed
- THEN the system SHALL output a markdown summary and JSON with resolved/pending/token accounting

### Requirement: Stdlib Only

The system SHALL use only Go standard library packages for CLI dispatch and command execution. No third-party CLI frameworks (cobra, urfave/cli) SHALL be used.

#### Scenario: Build with stdlib

- GIVEN the Go source tree
- WHEN `go build ./cmd/drup` is executed
- THEN the binary SHALL compile with zero external dependencies for CLI functionality
