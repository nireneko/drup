# Delta for Orchestrator Skill

## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: Pipeline Definition

The system SHALL define a 7-stage pipeline: preflight → dep check → rector → contrib loop → custom loop → final validation → report. Each gate between stages SHALL be confirmed by a `drup-validator` report, not by the orchestrator executing a tool itself.
(Previously: stage gating did not specify who executes the gate check, allowing the orchestrator to call `validate` directly.)

#### Scenario: Pipeline stages in order

- GIVEN the orchestrator skill is loaded
- WHEN the pipeline executes
- THEN stages SHALL execute in order: preflight, dep check, rector, contrib loop, custom loop, final validation, report

#### Scenario: Stage dependency

- GIVEN stage N has not been confirmed clean
- WHEN the orchestrator considers stage N+1
- THEN the orchestrator SHALL dispatch `drup-validator` for stage N and SHALL NOT proceed to stage N+1 until that report confirms zero errors

### Requirement: Human Escalation

The system SHALL produce an actionable pending-human list when automated resolution fails, compiled from sub-agent and `drup-validator` reports only — never from the orchestrator's own tool output, since it has none.
(Previously: did not specify the report's data source given the orchestrator's tool access.)

#### Scenario: Escalation list

- GIVEN modules/files that failed all retry attempts, as reported by fixer sub-agents and `drup-validator`
- WHEN the pipeline completes
- THEN the system SHALL include each item with: path, error summary, attempted fixes, and suggested manual action, sourced entirely from sub-agent reports
