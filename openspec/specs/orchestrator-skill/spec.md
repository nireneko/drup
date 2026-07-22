# Orchestrator Skill Specification

## Purpose

SKILL.md encoding the complete 7-stage Drupal upgrade pipeline with validation gates for AI agents. The orchestrator is a pure coordinator with zero execute permissions, delegating all validation, scanning, and remediation work to specialized sub-agents.

## Requirements

### Requirement: Zero Execute Permission

The orchestrator agent MUST NOT invoke Bash, MCP tool calls, or any other execution primitive directly. It SHALL only: (a) read prior sub-agent reports, (b) dispatch a sub-agent with a defined task and context, and (c) communicate status to the user.

#### Scenario: Orchestrator needs scan data

- GIVEN the orchestrator needs to know the current error count
- WHEN it requires that information
- THEN it SHALL dispatch `drup-validator` to obtain it and SHALL NOT call `scan` or `upgrade_scan` itself

#### Scenario: Attempted direct tool call is a defect

- GIVEN a version of the orchestrator skill
- WHEN its SKILL.md contains any direct Bash or MCP tool invocation instruction
- THEN this SHALL be treated as a specification violation requiring correction

### Requirement: Validation Delegation

The orchestrator SHALL delegate every `scan`, `upgrade_scan`, and `validate` call to the `drup-validator` sub-agent. The orchestrator MUST NOT call `validate` to approve its own dispatch decisions (no self-approval).

#### Scenario: Gate check between stages

- GIVEN a fixer sub-agent (contrib, custom, or theme) reports completion for its scope
- WHEN the orchestrator needs to confirm the scope is clean before advancing
- THEN it SHALL dispatch `drup-validator` with that scope and wait for its report before advancing

### Requirement: Pipeline Definition

The system SHALL define a 7-stage pipeline: preflight → dep check → rector → contrib loop → custom loop → final validation → report. Each gate between stages SHALL be confirmed by a `drup-validator` report, not by the orchestrator executing a tool itself.

#### Scenario: Pipeline stages in order

- GIVEN the orchestrator skill is loaded
- WHEN the pipeline executes
- THEN stages SHALL execute in order: preflight, dep check, rector, contrib loop, custom loop, final validation, report

#### Scenario: Stage dependency

- GIVEN stage N has not been confirmed clean
- WHEN the orchestrator considers stage N+1
- THEN the orchestrator SHALL dispatch `drup-validator` for stage N and SHALL NOT proceed to stage N+1 until that report confirms zero errors

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

The system SHALL produce an actionable pending-human list when automated resolution fails, compiled from sub-agent and `drup-validator` reports only — never from the orchestrator's own tool output, since it has none.

#### Scenario: Escalation list

- GIVEN modules/files that failed all retry attempts, as reported by fixer sub-agents and `drup-validator`
- WHEN the pipeline completes
- THEN the system SHALL include each item with: path, error summary, attempted fixes, and suggested manual action, sourced entirely from sub-agent reports
