# E2E Test Scaffolding Specification

## Purpose

Mock-based integration tests for pipeline stage orchestration without requiring a real Drupal site.

## Requirements

### Requirement: Mock-Based Integration Tests

The system SHALL provide integration test scaffolding for pipeline stage orchestration using mocked external commands. Tests MUST NOT require a real Drupal site. The system SHALL mock `cliRun` / subprocess calls to test stage sequencing, gate conditions, and error paths.

| Req | Strength | Behavior |
|-----|----------|----------|
| Mock external commands | MUST | Intercept subprocess calls (composer, drush, git) |
| Stage sequencing | MUST | Verify stages execute in correct order |
| Gate conditions | MUST | Verify gates block progression on failure |
| Error paths | MUST | Verify retry and escalation behavior |
| No real Drupal | MUST NOT | Require a running Drupal site or database |

#### Scenario: Full pipeline stage sequence

- GIVEN mocked commands that all succeed
- WHEN the integration test runs the pipeline
- THEN stages SHALL execute in order: preflight → dep-check → rector → contrib → custom → core-upgrade → validate → cleanup → report

#### Scenario: Gate blocks on validate failure

- GIVEN mocked validate returns exit code 1
- WHEN the integration test runs
- THEN the pipeline SHALL halt before cleanup stage and report validation failure

#### Scenario: Cleanup skipped on validate failure

- GIVEN mocked validate returns non-zero
- WHEN the integration test runs
- THEN cleanup stage SHALL NOT execute
