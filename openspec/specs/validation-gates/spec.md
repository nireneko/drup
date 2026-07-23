# Validation Gates Specification

## Purpose

Hard gates between each pipeline stage enforcing external validation, no self-approval, retry loops, and phase gating.

## Requirements

### Requirement: External Validation

The system SHALL require a party other than the sub-agent under test to confirm its work: the orchestrator dispatches `drup-validator`, which executes `validate` independently of the sub-agent's self-report. The orchestrator MUST NOT execute `validate` directly — it only dispatches `drup-validator` and reads its report.

#### Scenario: Orchestrator delegates validation of sub-agent work

- GIVEN drup-contrib finishes processing module X
- WHEN the orchestrator needs confirmation
- THEN the orchestrator SHALL dispatch `drup-validator(scope=contrib, module=X)` and SHALL NOT call `validate` itself

#### Scenario: Sub-agent claims success but validator finds errors

- GIVEN a sub-agent reports "done" for its scope
- WHEN `drup-validator` runs and its report shows errors
- THEN the orchestrator SHALL treat the sub-agent's report as failed and re-enter the retry loop

### Requirement: No Self-Approval

The system SHALL NOT allow any sub-agent — including `drup-validator` — to validate its own remediation work. `drup-validator` MUST only be dispatched against work produced by a different sub-agent.

#### Scenario: Sub-agent cannot skip validation

- GIVEN a sub-agent completes its task
- WHEN the sub-agent attempts to proceed without confirmation
- THEN the orchestrator SHALL block progression and dispatch `drup-validator` before advancing

#### Scenario: Validator cannot validate its own output

- GIVEN `drup-validator` has no remediation capability
- WHEN the orchestrator plans dispatches
- THEN `drup-validator` SHALL never be dispatched to confirm its own prior report

### Requirement: Retry Loop

The system SHALL retry failed validations up to 2 times with the same sub-agent and model before escalating.

#### Scenario: First retry

- GIVEN `validate` returns errors for a scope
- WHEN the orchestrator re-launches the sub-agent
- THEN the system SHALL pass the validator output as feedback and retry with the same model

#### Scenario: Second retry

- GIVEN the first retry also fails validation
- WHEN the orchestrator re-launches again
- THEN the system SHALL pass updated validator output and retry a second time

#### Scenario: Escalation after retries exhausted

- GIVEN 2 retries have failed
- WHEN the orchestrator escalates
- THEN the system SHALL switch to a higher model tier (haiku → sonnet) and retry once more

### Requirement: Phase Gating

The system SHALL NOT advance to the next pipeline phase until all items in the current phase pass validation. Exit code 3 from `upgrade_status:analyze` SHALL be treated as success-with-findings (not error). The validator SHALL parse stdout on exit 3 and only advance when `total_errors == 0`.

#### Scenario: Phase complete

- GIVEN all contrib modules pass individual validation
- WHEN the orchestrator runs phase-level validation
- THEN the system SHALL execute `validate(global)` and proceed to the next phase only if `total_errors == 0`

#### Scenario: Phase incomplete

- GIVEN some contrib modules still have errors
- WHEN the orchestrator checks phase completion
- THEN the system SHALL NOT proceed to the custom loop and SHALL iterate remaining errors with the correct sub-agent

#### Scenario: Phase complete with exit code 3

- GIVEN `validate` returns exit code 3 with parseable findings
- WHEN the orchestrator checks phase completion
- THEN the system SHALL parse findings, count errors, and proceed only if `total_errors == 0`

#### Scenario: Validate scoped to module under DDEV

- GIVEN a DDEV project and `validate({module_name: "mymodule"})`
- WHEN validation runs
- THEN the system SHALL use `RunWithEnv` with `--root=` and handle exit code 3 correctly

### Requirement: Scope Blocking

The system SHALL block progression within a scope while any error remains in that scope.

#### Scenario: Sequential within scope

- GIVEN module X has a pending error in the contrib scope
- WHEN the orchestrator considers module Y
- THEN the system SHALL NOT process module Y until module X's error is resolved or escalated to human review

### Requirement: Gate Evidence

The system SHALL include validator output as evidence when retrying or escalating.

#### Scenario: Retry with evidence

- GIVEN a validation failure with specific error messages
- WHEN the sub-agent is re-launched
- THEN the system SHALL include the exact validator error output in the sub-agent's context

#### Scenario: Escalation with full history

- GIVEN 2 failed retries with their validator outputs
- WHEN the model is escalated
- THEN the system SHALL include all previous attempts and their validation results in the escalated agent's context
