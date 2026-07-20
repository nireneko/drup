# Scan Specification

## Purpose

Parse `upgrade_status:analyze` JSON output into a classified error model organized by contrib, custom, and theme categories.

## Requirements

### Requirement: JSON Parsing

The system SHALL parse the JSON output from `drush upgrade_status:analyze` into an internal error model.

#### Scenario: Valid upgrade_status output

- GIVEN JSON output from `drush upgrade_status:analyze --format=json`
- WHEN the scan package processes it
- THEN the system SHALL produce a structured model with errors classified by type

#### Scenario: Empty analysis (zero errors)

- GIVEN JSON output with no errors
- WHEN the scan package processes it
- THEN the system SHALL return an empty error list with `total_errors: 0`

#### Scenario: Malformed JSON

- GIVEN invalid or unexpected JSON structure
- WHEN the scan package processes it
- THEN the system SHALL return a parse error with details about the unexpected structure

### Requirement: Error Classification

The system SHALL classify each error into one of three categories: contrib, custom, or theme.

#### Scenario: Contrib module error

- GIVEN an error originating from `modules/contrib/<module_name>`
- WHEN classification runs
- THEN the system SHALL categorize it under `contrib` with the module machine name

#### Scenario: Custom module error

- GIVEN an error originating from `modules/custom/<module_name>`
- WHEN classification runs
- THEN the system SHALL categorize it under `custom` with the file path

#### Scenario: Theme error

- GIVEN an error originating from `themes/<theme_name>`
- WHEN classification runs
- THEN the system SHALL categorize it under `theme` with the file path

#### Scenario: Unclassifiable path

- GIVEN an error from a path not matching contrib/custom/theme patterns
- WHEN classification runs
- THEN the system SHALL categorize it under `custom` as a fallback

### Requirement: Error Model Structure

The system SHALL represent each error with file path, line number, message, severity, and the originating module or theme name.

#### Scenario: Full error details

- GIVEN a parsed error entry
- WHEN the error model is constructed
- THEN each error SHALL contain: `{file, line, message, severity, source}` where source is the module or theme machine name

### Requirement: Fixture-Based Parsing

The system SHALL have fixture-based unit tests covering known `upgrade_status` JSON formats.

#### Scenario: Fixture test for D10 output

- GIVEN a fixture file with known D10 upgrade_status JSON
- WHEN the test runs
- THEN the parsed output SHALL match the expected error model exactly

#### Scenario: Fixture test for D9 output

- GIVEN a fixture file with known D9 upgrade_status JSON
- WHEN the test runs
- THEN the parsed output SHALL match the expected error model exactly
