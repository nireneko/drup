# Report Specification

## Purpose

Generate JSON and markdown reports summarizing the Drupal upgrade with resolved/pending items and token accounting.

## Requirements

### Requirement: JSON Report Generation

The system SHALL generate a JSON report containing all upgrade results with real scan data. The system SHALL collect actual error counts from `scan.ParseCodeclimateJSON()` or equivalent instead of hardcoded zeros.

| Req | Strength | Behavior |
|-----|----------|----------|
| Real error data | MUST | Populate `total_errors` from actual scan results |
| Real error list | MUST | Populate `resolved` and `pending` from scan data |
| No hardcoded zeros | MUST NOT | Use `0` only when scan confirms zero findings |

#### Scenario: Full report with resolved and pending

- GIVEN an upgrade session with some resolved and some pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `{resolved: [...], pending: [...], total_errors: N, token_accounting: {...}}`

#### Scenario: All errors resolved

- GIVEN an upgrade session with zero pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `pending: []` and `total_errors: 0`

#### Scenario: Report after scan with findings

- GIVEN a project with 15 deprecation errors
- WHEN `drup report` runs
- THEN the system SHALL output JSON with `total_errors: 15` and populated error arrays

#### Scenario: Report with no scan data available

- GIVEN no prior scan has been run
- WHEN `drup report` runs
- THEN the system SHALL run a scan first, then generate the report with real data

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
