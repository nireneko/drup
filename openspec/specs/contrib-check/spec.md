# Contrib Check Specification

## Purpose

Query Drupal.org for D11 compatibility of contrib modules using release-history XML, api-d7, and issue scraper.

## Requirements

### Requirement: Release History Lookup

The system SHALL fetch and parse Drupal.org release-history XML to determine D11 compatibility. The system SHALL parse `core_version_requirement` from release metadata and support compound constraints with `||` operators (e.g. `^10.3 || ^11.0`).

| Req | Strength | Behavior |
|-----|----------|----------|
| Compound constraints | MUST | Parse `||` in `core_version_requirement` |
| Semver matching | MUST | Use semver comparison, not string comparison |
| Fallback on parse failure | SHOULD | Fall back to string match if semver parse fails |

#### Scenario: Module with D11 release

- GIVEN a module machine name with a D11-compatible release
- WHEN the system fetches `https://updates.drupal.org/release-history/<module>/current`
- THEN the system SHALL return `{has_d11_release: true, latest_version: string, compatible_branches: [string]}`

#### Scenario: Module without D11 release

- GIVEN a module machine name without D11 releases
- WHEN the system fetches the release history
- THEN the system SHALL return `{has_d11_release: false, compatible_branches: []}`

#### Scenario: Module not found on Drupal.org

- GIVEN a machine name that does not exist on Drupal.org
- WHEN the system fetches the release history
- THEN the system SHALL return a 404 error with `{found: false}`

#### Scenario: Module with compound constraint

- GIVEN webform 6.3.0 with `core_version_requirement: "^10.3 || ^11.0"`
- WHEN `contrib_check({module_machine_name: "webform"})` runs
- THEN the system SHALL return `{has_d11_release: true}` (matches `^11.0`)

#### Scenario: Module with single constraint

- GIVEN a module with `core_version_requirement: "^11.0"`
- WHEN contrib_check runs
- THEN the system SHALL return `{has_d11_release: true}`

#### Scenario: Module incompatible with target

- GIVEN a module with `core_version_requirement: "^9.0 || ^10.0"`
- WHEN checking D11 compatibility
- THEN the system SHALL return `{has_d11_release: false}`

### Requirement: XML Parsing

The system SHALL parse Drupal.org release-history XML using `encoding/xml` from stdlib.

#### Scenario: Valid release-history XML

- GIVEN well-formed release-history XML with release entries
- WHEN the parser processes it
- THEN the system SHALL extract version tags, branch names, and incompatibility flags

#### Scenario: Malformed XML

- GIVEN invalid XML response
- WHEN the parser processes it
- THEN the system SHALL return a parse error with the raw response snippet

### Requirement: api-d7 Integration

The system SHALL use api-d7 as the primary source for module metadata before falling back to scraping.

#### Scenario: api-d7 returns module info

- GIVEN a module machine name
- WHEN the system queries api-d7
- THEN the system SHALL return module metadata including supported Drupal versions

#### Scenario: api-d7 unavailable

- GIVEN api-d7 is unreachable or returns an error
- WHEN the system queries api-d7
- THEN the system SHALL fall back to release-history XML parsing

### Requirement: Issue Scraper Fallback

The system SHALL scrape Drupal.org issue pages as a fallback when structured APIs are insufficient.

#### Scenario: Scraper finds D11 issues

- GIVEN release-history shows no D11 release
- WHEN the system scrapes the module's issue queue
- THEN the system SHALL return issue NIDs and titles mentioning D11 compatibility

#### Scenario: Scraper with no relevant issues

- GIVEN no issues mention D11 compatibility
- WHEN the system scrapes the issue queue
- THEN the system SHALL return an empty issue list

### Requirement: HTTP Client with Timeout

The system SHALL use `net/http` with configurable timeouts for all Drupal.org requests.

#### Scenario: Request timeout

- GIVEN Drupal.org does not respond within the timeout period
- WHEN an HTTP request is made
- THEN the system SHALL return a timeout error without hanging
