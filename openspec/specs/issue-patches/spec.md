# Issue Patches Specification

## Purpose

Extract patch, diff, and merge request links from Drupal.org issues with RTBC prioritization.

## Requirements

### Requirement: Issue Lookup by Module Name

The system SHALL search Drupal.org for issues related to a module and extract patch/diff/MR links. The system SHALL return structured JSON responses with `status`, `module`, `searched`, `message`, and `suggestion` fields instead of empty arrays or null values.

| Req | Strength | Behavior |
|-----|----------|----------|
| Structured response | MUST | Return `{status, module, searched, message, suggestion, patches[]}` |
| Status field | MUST | One of: `patches_found`, `no_patches_found`, `error` |
| Suggestion field | MUST | Include actionable next step (e.g., "check manually at https://...") |
| No empty arrays | MUST NOT | Return bare `[]` or `null` without context |

(Previously: returned empty array `[]` when no issues found, with no context)

#### Scenario: Module with patch issues

- GIVEN a module machine name with issues containing patches
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "patches_found", module: "<name>", patches: [{url, status, date, is_patch}], message: "N patches found", suggestion: "Apply highest-date RTBC patch first"}`

#### Scenario: Module with no issues

- GIVEN a module machine name with no relevant issues
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "no_patches_found", module: "<name>", searched: "<url>", message: "No patches found on Drupal.org", suggestion: "Create a custom patch or check issue queue manually at <url>"}`

#### Scenario: API error during lookup

- GIVEN Drupal.org API returns an error or is unreachable
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "error", module: "<name>", message: "<error detail>", suggestion: "Retry later or check manually"}`

### Requirement: Issue Lookup by NID

The system SHALL extract patch/diff/MR links from a specific Drupal.org issue by NID. The system SHALL return structured JSON with the same fields as module lookup.

(Previously: returned empty array `[]` when no patches found)

#### Scenario: Issue with multiple patches

- GIVEN an issue NID with multiple file attachments
- WHEN the system scrapes the issue page
- THEN the system SHALL return all patch/diff/MR URLs with their upload dates and statuses in a structured response

#### Scenario: Issue with no patches

- GIVEN an issue NID with no file attachments
- WHEN the system scrapes the issue page
- THEN the system SHALL return `{status: "no_patches_found", module: "<from-nid>", searched: "https://www.drupal.org/node/<NID>", message: "No patches in this issue", suggestion: "Check related issues"}`

### Requirement: RTBC Prioritization

The system SHALL sort results with RTBC (Reviewed, Tested, and Committed) patches first.

#### Scenario: Mixed status issues

- GIVEN issues with statuses: "Needs review", "RTBC", "Needs work", "Fixed"
- WHEN results are returned
- THEN the system SHALL sort by priority: RTBC > Fixed > Needs review > Needs work > other

#### Scenario: All same status

- GIVEN all issues have the same status
- WHEN results are returned
- THEN the system SHALL sort by date descending (newest first)

### Requirement: api-d7 Primary Source

The system SHALL use api-d7 structured endpoints before falling back to HTML scraping.

#### Scenario: api-d7 returns issue data

- GIVEN api-d7 is available and returns issue data
- WHEN the system queries for patches
- THEN the system SHALL extract patch URLs from the structured response

#### Scenario: api-d7 unavailable

- GIVEN api-d7 returns an error or is unreachable
- WHEN the system queries for patches
- THEN the system SHALL fall back to HTML scraping of the issue page

### Requirement: Patch URL Detection

The system SHALL identify patch files by extension (.patch, .diff) and merge request links (git.drupal.org MR URLs).

#### Scenario: Mixed file types

- GIVEN an issue with .patch, .diff, .txt, and image attachments
- WHEN the system extracts links
- THEN the system SHALL include only .patch, .diff, and MR URLs; `is_patch: true` for .patch/.diff, `is_patch: false` for MR links

### Requirement: Fixture-Based Tests

The system SHALL have fixture-based tests for known Drupal.org issue HTML structures.

#### Scenario: Fixture for issue page with patches

- GIVEN a saved HTML fixture of a Drupal.org issue with patches
- WHEN the scraper processes it
- THEN the output SHALL match expected patch URLs and metadata
