# Delta for Orchestrator Skill

## MODIFIED Requirements

### Requirement: Pipeline Definition

The system SHALL define an 8-stage pipeline: preflight → dep check → rector → contrib loop → custom loop → **core upgrade** → final validation → report. Each stage maps to a `drup <stage>` CLI command. The AI SHALL check each command's exit code before advancing.

(Previously: core upgrade was Stage 5 between contrib loop and custom loop; now core upgrade is Stage 6 between custom loop and final validation.)

#### Scenario: Pipeline stages in order

- GIVEN the skill is loaded
- WHEN the pipeline executes
- THEN stages SHALL execute in order: preflight, dep check, rector, contrib loop, custom loop, core upgrade, final validation, report

#### Scenario: Stage gate via exit code

- GIVEN stage N's `drup` command exits non-zero
- WHEN the AI checks the result
- THEN the AI SHALL NOT proceed to stage N+1 and SHALL report the failure
