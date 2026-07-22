# Delta for Sub-Agents

## ADDED Requirements

### Requirement: drup-validator Agent

The system SHALL define a `drup-validator` sub-agent with model routing to haiku/cheap, owning `scan`, `upgrade_scan`, `validate`, and report generation. This agent MUST NOT execute rector, patches, or any remediation action — it performs analysis and reporting only.

#### Scenario: Validator called between stages

- GIVEN the orchestrator dispatches a validation request for a scope (module, file, or global)
- WHEN `drup-validator` runs
- THEN the agent SHALL run the relevant scan/validate tool for that scope and return a structured report with error count and details, without modifying any file

#### Scenario: Validator receives no remediation instructions

- GIVEN `drup-validator` is dispatched
- WHEN its task context is built
- THEN the context SHALL contain only the scope to validate — no fix instructions, since the agent has no write/execute capability

## MODIFIED Requirements

### Requirement: drup-preflight Agent

The system SHALL define a `drup-preflight` sub-agent with model routing to haiku/cheap, using detection-only tools (git/composer/drush/environment checks). Scan and validate calls SHALL be delegated to `drup-validator`, not executed by `drup-preflight`.
(Previously: `drup-preflight` used `scan` and `validate` MCP tools directly.)

#### Scenario: Preflight agent execution

- GIVEN the orchestrator dispatches preflight work
- WHEN drup-preflight runs
- THEN the agent SHALL detect Drupal version, check git/composer/drush/environment, install dev deps, and report results — without calling `scan` or `validate` itself

### Requirement: drup-contrib Agent

The system SHALL define a `drup-contrib` sub-agent with model routing to haiku/cheap, using `contrib_check`, `issue_patches`, and `apply_patch` MCP tools. Validation of the result SHALL be delegated to `drup-validator`, not executed by `drup-contrib`.
(Previously: `drup-contrib` also used the `validate` MCP tool directly.)

#### Scenario: Contrib agent processes one module

- GIVEN the orchestrator dispatches a contrib module to drup-contrib
- WHEN the agent runs
- THEN the agent SHALL check D11 release, search/apply/create patches, and report results for that module only — the orchestrator SHALL separately dispatch `drup-validator` to confirm the result

#### Scenario: Contrib agent context isolation

- GIVEN drup-contrib is processing module X
- WHEN the agent runs
- THEN the agent's context SHALL contain only module X's data — no other module context

### Requirement: drup-custom Agent

The system SHALL define a `drup-custom` sub-agent with model routing haiku → sonnet escalation, applying fixes only. Scan and validate calls SHALL be delegated to `drup-validator`.
(Previously: `drup-custom` used `validate` and `scan` MCP tools directly.)

#### Scenario: Custom agent fixes file

- GIVEN the orchestrator dispatches a custom file, with error context supplied by a prior `drup-validator` scan, to drup-custom
- WHEN the agent runs on haiku
- THEN the agent SHALL apply fixes and report results — without calling `scan` or `validate` itself

#### Scenario: Custom agent model escalation

- GIVEN drup-custom fails validation twice on haiku, per `drup-validator` reports
- WHEN the orchestrator escalates
- THEN the system SHALL re-dispatch the same file to drup-custom running on sonnet

### Requirement: drup-theme Agent

The system SHALL define a `drup-theme` sub-agent with model routing to haiku, applying twig/theme fixes only. Scan and validate calls SHALL be delegated to `drup-validator`.
(Previously: `drup-theme` used `validate` and `scan` MCP tools directly.)

#### Scenario: Theme agent fixes twig file

- GIVEN the orchestrator dispatches a theme file, with error context from `drup-validator`, to drup-theme
- WHEN the agent runs
- THEN the agent SHALL fix twig/theme deprecations and report results — without calling `scan` or `validate` itself
