# Orchestrator Skill Specification

## Purpose

SKILL.md encoding the complete 8-stage Drupal upgrade pipeline with validation gates for AI agents. The orchestrator is a pure coordinator with zero execute permissions, delegating all work to `drup <stage>` CLI commands.

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

### Requirement: Cross-Platform Portability

The SKILL.md MUST NOT contain any platform-specific primitives (no `task()`, no agent definitions, no MCP tool calls). It SHALL be a plain markdown file readable by any AI agent on any platform (OpenCode, Claude Code, Codex).

#### Scenario: No platform primitives in SKILL.md

- GIVEN the generated SKILL.md
- WHEN scanned for platform-specific syntax
- THEN it MUST contain zero references to `task()`, agent definitions, or MCP invocations

#### Scenario: Any AI can follow the skill

- GIVEN SKILL.md loaded by Claude Code, Codex, or OpenCode
- WHEN the AI reads it
- THEN it MUST be able to execute the pipeline using only shell/bash commands

### Requirement: Direct CLI Invocation

The AI MUST execute every pipeline stage by calling `drup <stage>` CLI commands via shell. It MUST NOT modify project files directly (no editing composer.json, no running composer/drush outside of `drup` commands).

#### Scenario: AI calls drup CLI for each stage

- GIVEN the AI is executing the pipeline
- WHEN it reaches any stage
- THEN it MUST invoke the corresponding `drup <stage>` command and check its exit code

#### Scenario: AI must not edit files directly

- GIVEN the AI needs to upgrade Drupal core
- WHEN it follows the skill
- THEN it MUST call `drup upgrade-core` and MUST NOT edit composer.json itself

### Requirement: Validation Delegation

The orchestrator SHALL delegate every validation check to `drup validate` and `drup scan` CLI commands. The orchestrator MUST NOT self-approve — it SHALL rely on CLI exit codes, not its own file inspection.

#### Scenario: Gate check between stages

- GIVEN a stage completes
- WHEN the AI needs to confirm the scope is clean before advancing
- THEN it SHALL run `drup validate` and check exit code before proceeding

#### Scenario: Attempted self-approval is a defect

- GIVEN the SKILL.md
- WHEN it contains instructions for the AI to inspect files directly for validation
- THEN this SHALL be treated as a specification violation

### Requirement: Pipeline Definition

The system SHALL define an 8-stage pipeline: preflight → dep check → rector → contrib loop → **core upgrade** → custom loop → final validation → report. Each stage maps to a `drup <stage>` CLI command. The AI SHALL check each command's exit code before advancing.

#### Scenario: Pipeline stages in order

- GIVEN the skill is loaded
- WHEN the pipeline executes
- THEN stages SHALL execute in order: preflight, dep check, rector, contrib loop, core upgrade, custom loop, final validation, report

#### Scenario: Stage gate via exit code

- GIVEN stage N's `drup` command exits non-zero
- WHEN the AI checks the result
- THEN the AI SHALL NOT proceed to stage N+1 and SHALL report the failure

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

The system SHALL produce an actionable pending-human list when automated resolution fails, compiled from `drup` CLI output only — never from the AI's own file inspection.

#### Scenario: Escalation list

- GIVEN commands that failed all retry attempts
- WHEN the pipeline completes
- THEN the system SHALL include each item with: path, error summary, attempted fixes, and suggested manual action, sourced from `drup` CLI output
