# Orchestrator Skill Specification

## Purpose

SKILL.md encoding the complete 7-stage Drupal upgrade pipeline with validation gates for AI agents.

## Requirements

### Requirement: Pipeline Definition

The system SHALL define a 7-stage pipeline: preflight → dep check → rector → contrib loop → custom loop → final validation → report.

#### Scenario: Pipeline stages in order

- GIVEN the orchestrator skill is loaded
- WHEN the pipeline executes
- THEN stages SHALL execute in order: preflight, dep check, rector, contrib loop, custom loop, final validation, report

#### Scenario: Stage dependency

- GIVEN stage N has not completed
- WHEN the orchestrator considers stage N+1
- THEN the orchestrator SHALL NOT proceed to stage N+1 until stage N passes its validation gate

### Requirement: Contrib Loop

The system SHALL iterate over each contrib module, checking D11 release, applying patches, and validating per module.

#### Scenario: Module with D11 release

- GIVEN a contrib module with a D11 release available
- WHEN the contrib loop processes it
- THEN the system SHALL run `composer require`, commit, and validate before proceeding to the next module

#### Scenario: Module without release, patch available

- GIVEN a contrib module without D11 release but with an RTBC patch
- WHEN the contrib loop processes it
- THEN the system SHALL apply the patch, commit, and validate

#### Scenario: Module unresolvable

- GIVEN a contrib module with no release and no working patch after retries
- WHEN the contrib loop exhausts retries
- THEN the system SHALL add it to the pending human review list

### Requirement: Custom Loop

The system SHALL iterate over each custom file with errors, applying fixes and validating per file.

#### Scenario: Custom file fix succeeds

- GIVEN a custom file with deprecation errors
- WHEN the custom loop processes it
- THEN the system SHALL fix the file, validate, and commit

#### Scenario: Custom file fix fails after retries

- GIVEN a custom file that fails validation after 2 retries + model escalation
- WHEN the custom loop exhausts retries
- THEN the system SHALL add it to the pending human review list

### Requirement: Sequential Scope Execution

The system SHALL execute pipeline stages sequentially — no parallel sub-agent execution across stages.

#### Scenario: No parallel stages

- GIVEN the contrib loop is running
- WHEN the orchestrator manages execution
- THEN the custom loop SHALL NOT start until all contrib modules pass validation

### Requirement: Human Escalation

The system SHALL produce an actionable pending human list when automated resolution fails.

#### Scenario: Escalation list

- GIVEN modules/files that failed all retry attempts
- WHEN the pipeline completes
- THEN the system SHALL include each item with: path, error summary, attempted fixes, and suggested manual action
