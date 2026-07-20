# Sub-Agents Specification

## Purpose

Four specialized sub-agent definitions with model routing for isolated context per upgrade domain.

## Requirements

### Requirement: drup-preflight Agent

The system SHALL define a `drup-preflight` sub-agent with model routing to haiku/cheap, using `scan` and `validate` MCP tools.

#### Scenario: Preflight agent execution

- GIVEN the orchestrator dispatches preflight work
- WHEN drup-preflight runs
- THEN the agent SHALL detect Drupal version, check git/composer/drush, install dev deps, and report results

### Requirement: drup-contrib Agent

The system SHALL define a `drup-contrib` sub-agent with model routing to haiku/cheap, using `contrib_check`, `issue_patches`, `apply_patch`, and `validate` MCP tools.

#### Scenario: Contrib agent processes one module

- GIVEN the orchestrator dispatches a contrib module to drup-contrib
- WHEN the agent runs
- THEN the agent SHALL check D11 release, search/apply/create patches, and report results for that module only

#### Scenario: Contrib agent context isolation

- GIVEN drup-contrib is processing module X
- WHEN the agent runs
- THEN the agent's context SHALL contain only module X's data — no other module context

### Requirement: drup-custom Agent

The system SHALL define a `drup-custom` sub-agent with model routing haiku → sonnet escalation, using `validate` and `scan` MCP tools.

#### Scenario: Custom agent fixes file

- GIVEN the orchestrator dispatches a custom file to drup-custom
- WHEN the agent runs on haiku
- THEN the agent SHALL read the file + errors, apply fixes, and report results

#### Scenario: Custom agent model escalation

- GIVEN drup-custom fails validation twice on haiku
- WHEN the orchestrator escalates
- THEN the system SHALL re-dispatch the same file to drup-custom running on sonnet

### Requirement: drup-theme Agent

The system SHALL define a `drup-theme` sub-agent with model routing to haiku, using `validate` and `scan` MCP tools.

#### Scenario: Theme agent fixes twig file

- GIVEN the orchestrator dispatches a theme file to drup-theme
- WHEN the agent runs
- THEN the agent SHALL fix twig/theme deprecations and report results

### Requirement: Model Routing

The system SHALL route sub-agents to the appropriate model tier based on task complexity.

#### Scenario: Cheap model for mechanical work

- GIVEN preflight, contrib, or theme tasks
- WHEN the orchestrator selects a model
- THEN the system SHALL use haiku/cheap model by default

#### Scenario: Escalation for custom code

- GIVEN custom code tasks that fail on cheap model
- WHEN the orchestrator escalates after 2 retries
- THEN the system SHALL switch to sonnet for that specific file
