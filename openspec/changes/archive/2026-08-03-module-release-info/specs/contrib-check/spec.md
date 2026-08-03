# Delta for Contrib Check

## ADDED Requirements

### Requirement: Curated Release Info Fetch

The system SHALL provide a curated release-info lookup for a module machine name, reusing the existing release-history fetch, retry, and constraint-matching logic. The result SHALL include `module`, `found`, `maintenance_status`, and `releases[]`, where each release entry includes `version`, `tag`, `core_compatibility` (raw range string), `release_type[]` (raw terms), `insecure` (derived boolean), `security_covered`, and `date`. Returned releases SHALL always be restricted to `status == "published"`: retracted/unpublished releases SHALL NOT appear in `releases[]`, independent of whether a core-version filter is applied.

#### Scenario: Successful fetch for a maintained module

- GIVEN a module with a valid release-history feed, including at least one retracted/unpublished release
- WHEN the curated release-info lookup runs with no filter
- THEN the system SHALL return `{module, found: true, maintenance_status, releases: [...]}` containing every published release from the feed, excluding the retracted/unpublished release

#### Scenario: Zero-release published project

- GIVEN a module that exists on Drupal.org but has no releases
- WHEN the curated release-info lookup runs
- THEN the system SHALL return `{found: true, releases: []}`, distinct from the unknown-module case

#### Scenario: Unknown module

- GIVEN a module machine name with no project on Drupal.org (release-history responds with an `<error>` body)
- WHEN the curated release-info lookup runs
- THEN the system SHALL return a not-found result using the `status`/`message`/`suggestion` shape, not a bare Go error

### Requirement: Maintenance Status Extraction

The system SHALL extract the project-level maintenance status from the release-history feed's project `terms` (the `Maintenance status` term) and expose it as `maintenance_status` in the curated result, regardless of the project's release or filter state.

#### Scenario: Actively maintained project

- GIVEN a project whose `terms` include `Maintenance status: Actively maintained`
- WHEN the curated release-info lookup runs
- THEN the response SHALL include `maintenance_status: "Actively maintained"`

#### Scenario: Unsupported project still lists releases

- GIVEN a project whose `terms` include `Maintenance status: Unsupported`
- WHEN the curated release-info lookup runs
- THEN the system SHALL still return the project's releases normally, with `maintenance_status: "Unsupported"` as the only warning signal (no separate refusal shape)

### Requirement: Release Term Derivation

The system SHALL derive a per-release `insecure` boolean from the release's `Release type` terms, treating the presence of an `Insecure` term as `true`. The system SHALL pass through all other `Release type` term values unchanged in `release_type[]` and SHALL NOT error when it encounters a term value it does not otherwise interpret.

#### Scenario: Release carrying the Insecure term

- GIVEN a release whose `Release type` terms include `Insecure`
- WHEN the curated release-info lookup processes that release
- THEN the release entry SHALL have `insecure: true` and SHALL include `Insecure` in `release_type[]`

#### Scenario: Release without the Insecure term

- GIVEN a release whose `Release type` terms are `["Bug fixes"]`
- WHEN the curated release-info lookup processes that release
- THEN the release entry SHALL have `insecure: false`

#### Scenario: Unrecognized Release type term value

- GIVEN a release whose `Release type` terms include a value not previously seen (editorial vocabulary drift)
- WHEN the curated release-info lookup processes that release
- THEN the system SHALL pass the unrecognized value through unchanged in `release_type[]` and SHALL NOT return an error

### Requirement: Core Version Filter

The system SHALL accept an optional core-version filter parameter for the curated release-info lookup. Every returned release is already restricted to `status == "published"` per the Curated Release Info Fetch requirement; when a core-version filter is supplied, the system SHALL additionally require that the release's `core_compatibility` range satisfies the requested core version, evaluated via the existing composer-range constraint matcher (never string equality). No filter changes the published-status gate — it only adds the compatibility constraint on top of it.

#### Scenario: Filter narrows to matching major version

- GIVEN a module with releases spanning multiple core-compatibility ranges, including one published release with `core_compatibility: "^10.2 || ^11"`
- WHEN the curated release-info lookup runs with `core_version: "11"`
- THEN the system SHALL return only published releases whose range satisfies major 11

#### Scenario: No filter returns every matching release

- GIVEN a module with multiple releases, including at least one retracted/unpublished release
- WHEN the curated release-info lookup runs without a core-version filter
- THEN the system SHALL return every published release from the feed, excluding the retracted/unpublished release, with no cap on count
