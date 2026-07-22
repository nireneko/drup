# Delta for Platform Bootstrap (renamed from Agent Packaging)

## RENAMED Requirements

### Requirement: Platform Templates → Bootstrap File Generation

(Reason: Domain renamed from `agent-packaging` to `platform-bootstrap` to reflect shift from generating agent definitions to generating thin bootstrap files that load SKILL.md.)
(Migration: All references to `agent-packaging` spec, packaging templates, and sub-agent definition generation should update to `platform-bootstrap` and bootstrap file generation.)

## MODIFIED Requirements

### Requirement: Bootstrap File Generation

The system SHALL generate platform-specific bootstrap files for three platforms. Each bootstrap file's sole purpose is to instruct the AI to load and follow `SKILL.md`.

| Platform | File | Content |
|----------|------|---------|
| OpenCode | `opencode.json` skill entry | Registers SKILL.md path as a loadable skill |
| Claude Code | `CLAUDE.md` | References SKILL.md path, instructs AI to load it |
| Codex | `.github/copilot-instructions.md` | References SKILL.md path, instructs AI to load it |

#### Scenario: OpenCode bootstrap

- GIVEN `drup install` targets OpenCode
- WHEN bootstrap generation runs
- THEN it SHALL produce an `opencode.json` entry that registers SKILL.md as a skill the AI can load

#### Scenario: Claude Code bootstrap

- GIVEN `drup install` targets Claude Code
- WHEN bootstrap generation runs
- THEN it SHALL produce `CLAUDE.md` at project root instructing the AI to load SKILL.md

#### Scenario: Codex bootstrap

- GIVEN `drup install` targets Codex
- WHEN bootstrap generation runs
- THEN it SHALL produce `.github/copilot-instructions.md` instructing the AI to load SKILL.md

### Requirement: SKILL.md Generation

The system SHALL generate a single cross-platform SKILL.md encoding the full pipeline. This SKILL.md is the single source of truth — bootstrap files only reference it.

#### Scenario: Generate SKILL.md

- GIVEN the pipeline definition
- WHEN SKILL.md generation runs
- THEN it SHALL produce a platform-neutral SKILL.md with every stage mapped to a `drup <stage>` CLI command

### Requirement: Template Sources

The system SHALL maintain template sources in `internal/packaging/templates/` for all bootstrap files. `drup install` SHALL render these templates to produce platform-specific output.

#### Scenario: Templates render for all platforms

- GIVEN template sources exist
- WHEN `drup install` runs for a target platform
- THEN it SHALL render the correct bootstrap file from templates for that platform
