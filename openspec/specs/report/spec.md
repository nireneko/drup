# Report Specification

## Purpose

Generate JSON and markdown reports summarizing the Drupal upgrade with resolved/pending items and token accounting.

## Requirements

### Requirement: JSON Report Generation

The system SHALL generate a JSON report containing all upgrade results.

#### Scenario: Full report with resolved and pending

- GIVEN an upgrade session with some resolved and some pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `{resolved: [...], pending: [...], total_errors: N, token_accounting: {...}}`

#### Scenario: All errors resolved

- GIVEN an upgrade session with zero pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `pending: []` and `total_errors: 0`

### Requirement: Markdown Report Generation

The system SHALL generate a human-readable markdown report.

#### Scenario: Markdown with sections

- GIVEN upgrade results data
- WHEN markdown report is generated
- THEN the system SHALL produce markdown with sections: Summary, Resolved, Pending Human Review, Token Usage

#### Scenario: Pending items table

- GIVEN pending items that could not be auto-resolved
- WHEN markdown report is generated
- THEN the system SHALL include a table with columns: Module/File, Error, Suggested Action

### Requirement: Token Accounting

The system SHALL track and report token usage across sub-agent invocations.

#### Scenario: Multi-agent token tracking

- GIVEN multiple sub-agent invocations during the upgrade
- WHEN the report is generated
- THEN the system SHALL include per-agent token counts and a total in `token_accounting`

### Requirement: Report File Output

The system SHALL write report files to the project directory.

#### Scenario: Write report files

- GIVEN a project path `/path/to/drupal`
- WHEN report generation completes
- THEN the system SHALL write `drup-report.json` and `drup-report.md` to the project root
