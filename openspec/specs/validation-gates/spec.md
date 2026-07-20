# Validation Gates Specification

## Purpose

Hard gates between each pipeline stage enforcing external validation, no self-approval, retry loops, and phase gating.

## Requirements

### Requirement: External Validation

The system SHALL require the orchestrator — not the sub-agent — to execute `validate` after each sub-agent completes.

#### Scenario: Orchestrator validates sub-agent work

- GIVEN drup-contrib finishes processing module X
- WHEN the orchestrator runs validation
- THEN the orchestrator SHALL call `validate(scope=contrib, module=X)` independently of the sub-agent's self-report

#### Scenario: Sub-agent claims success but validate finds errors

- GIVEN a sub-agent reports "done" for its scope
- WHEN the orchestrator runs `validate` and it returns errors
- THEN the orchestrator SHALL treat the sub-agent's report as failed and re-enter the retry loop

### Requirement: No Self-Approval

The system SHALL NOT allow any sub-agent to validate its own work.

#### Scenario: Sub-agent cannot skip validation

- GIVEN a sub-agent completes its task
- WHEN the sub-agent attempts to proceed without orchestrator validation
- THEN the orchestrator SHALL block progression and run its own `validate` call

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

The system SHALL NOT advance to the next pipeline phase until all items in the current phase pass validation.

#### Scenario: Phase complete

- GIVEN all contrib modules pass individual validation
- WHEN the orchestrator runs phase-level validation
- THEN the system SHALL execute `validate(global)` and proceed to the next phase only if `total_errors == 0`

#### Scenario: Phase incomplete

- GIVEN some contrib modules still have errors
- WHEN the orchestrator checks phase completion
- THEN the system SHALL NOT proceed to the custom loop and SHALL iterate remaining errors with the correct sub-agent

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
