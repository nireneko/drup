# Delta for Validation Gates

## MODIFIED Requirements

### Requirement: External Validation

The system SHALL require a party other than the sub-agent under test to confirm its work: the orchestrator dispatches `drup-validator`, which executes `validate` independently of the sub-agent's self-report. The orchestrator MUST NOT execute `validate` directly — it only dispatches `drup-validator` and reads its report.
(Previously: required "the orchestrator — not the sub-agent" to execute `validate` directly, which conflicts with the orchestrator's zero-execute constraint.)

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
(Previously: stated only that a sub-agent cannot skip validation, without addressing who performs the delegated check.)

#### Scenario: Sub-agent cannot skip validation

- GIVEN a sub-agent completes its task
- WHEN the sub-agent attempts to proceed without confirmation
- THEN the orchestrator SHALL block progression and dispatch `drup-validator` before advancing

#### Scenario: Validator cannot validate its own output

- GIVEN `drup-validator` has no remediation capability
- WHEN the orchestrator plans dispatches
- THEN `drup-validator` SHALL never be dispatched to confirm its own prior report
