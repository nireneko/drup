# Delta for scan

## MODIFIED Requirements

### Requirement: Drush Invocation

The system SHALL invoke `drush upgrade_status:analyze` with the `--all` flag to ensure complete analysis results.

#### Scenario: Full project scan

- GIVEN a Drupal project with multiple modules and themes
- WHEN `drup scan` runs
- THEN the system SHALL execute `drush upgrade_status:analyze --all --format=json` and parse the complete output

#### Scenario: Empty results without --all

- GIVEN a Drupal project where `upgrade_status:analyze` without `--all` returns empty results
- WHEN `drup scan` runs
- THEN the system SHALL still return full analysis by including the `--all` flag

(Previously: The system invoked `drush upgrade_status:analyze` without the `--all` flag, which could return empty results on projects with multiple modules/themes.)
