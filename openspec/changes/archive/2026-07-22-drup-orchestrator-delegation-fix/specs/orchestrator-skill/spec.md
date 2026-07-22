# Delta for Orchestrator Skill

## ADDED Requirements

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

## MODIFIED Requirements

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

### Requirement: Human Escalation

The system SHALL produce an actionable pending-human list when automated resolution fails, compiled from `drup` CLI output only — never from the AI's own file inspection.

#### Scenario: Escalation list

- GIVEN commands that failed all retry attempts
- WHEN the pipeline completes
- THEN the system SHALL include each item with: path, error summary, attempted fixes, and suggested manual action, sourced from `drup` CLI output
