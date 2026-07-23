# Scan Specification

## Purpose

Parse `upgrade_status:analyze` plain-text output into a classified error model organized by contrib, custom, and theme categories. The parser uses line-based regex extraction and tolerates warnings, blank lines, and unrecognized input.

## Requirements

### Requirement: Plain-Text Parsing

The system SHALL parse the plain-text output from `drush upgrade_status:analyze` into an internal error model using line-based regex extraction.

| Req | Strength | Behavior |
|-----|----------|----------|
| Line-based parsing | MUST | Parse plain-text `upgrade_status:analyze` output into existing error model |
| Tolerant extraction | MUST | Regex field extraction; skip unrecognized lines |
| Project detection | MUST | `Project: <name>` lines delimit per-project blocks |
| Empty output | MUST | Return zero-error model when no project blocks found |

#### Scenario: Multi-project plain text

- GIVEN plain text with contrib + custom projects
- WHEN `scan.Parse()` runs
- THEN SHALL return classified errors with file/line/message/rule

#### Scenario: Tolerate warnings and blanks

- GIVEN `[warning]` and blank lines in output
- WHEN parsing runs
- THEN SHALL skip non-error lines, produce correct model

#### Scenario: Empty analysis (zero errors)

- GIVEN plain-text output with no project blocks
- WHEN the scan package processes it
- THEN the system SHALL return an empty error list with `total_errors: 0`

#### Scenario: Unparseable input

- GIVEN input that does not match expected plain-text patterns
- WHEN the scan package processes it
- THEN the system SHALL return a zero-result model gracefully

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

The system SHALL have fixture-based unit tests covering known `upgrade_status` plain-text formats.

#### Scenario: Fixture test for D10 output

- GIVEN a fixture file with known D10 upgrade_status plain text
- WHEN the test runs
- THEN the parsed output SHALL match the expected error model exactly

#### Scenario: Fixture test for D9 output

- GIVEN a fixture file with known D9 upgrade_status plain text
- WHEN the test runs
- THEN the parsed output SHALL match the expected error model exactly

#### Scenario: Fixture test for empty output

- GIVEN a fixture file with blank or warning-only content
- WHEN the test runs
- THEN the parsed output SHALL return zero errors

### Requirement: Drush Invocation

The system SHALL invoke `drush upgrade_status:analyze` with the `--all` flag. The system SHALL use `--root=<path>` instead of `-r <path>` for DDEV compatibility. The system SHALL use `RunWithEnv` with environment detection to prefix commands (e.g. `ddev`) when a container environment is detected. The `--format=json` flag SHALL NOT be used; the parser handles plain-text output.

| Req | Strength | Behavior |
|-----|----------|----------|
| Root flag | MUST | Use `--root=<path>` instead of `-r <path>` |
| Env detection | MUST | Call `envdetect.Detect()` and use `RunWithEnv(prefix, ...)` |
| Exit code 3 | MUST | Treat exit code 3 as success-with-findings, not error |
| Exit code 1,2,>3 | MUST | Treat as real errors and abort |
| Stdout on exit 3 | MUST | Parse stdout regardless of exit code 3 |
| Empty stdout + exit 3 | MUST | Treat as error (drush crashed, not findings) |

#### Scenario: Scan with findings under DDEV

- GIVEN a DDEV project with deprecations
- WHEN `drup scan /path` runs
- THEN the system SHALL detect DDEV, run `ddev exec drush --root=/var/www/html upgrade_status:analyze --all`, parse stdout, and return exit 0 with findings

#### Scenario: Scan with no findings

- GIVEN a clean project
- WHEN `drup scan` runs
- THEN the system SHALL return exit 0 with zero-error model

#### Scenario: Scan with drush crash

- GIVEN drush exits 3 with empty stdout and error on stderr
- WHEN `drup scan` runs
- THEN the system SHALL treat this as an error and report stderr content

#### Scenario: Full project scan

- GIVEN a Drupal project with multiple modules and themes
- WHEN `drup scan` runs
- THEN the system SHALL execute `drush upgrade_status:analyze --all` and parse the complete plain-text output

#### Scenario: Empty results without --all

- GIVEN a Drupal project where `upgrade_status:analyze` without `--all` returns empty results
- WHEN `drup scan` runs
- THEN the system SHALL still return full analysis by including the `--all` flag
