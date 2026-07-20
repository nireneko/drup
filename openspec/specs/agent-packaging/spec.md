# Agent Packaging Specification

## Purpose

Templates for generating agent skill files, sub-agent definitions, and MCP configuration per platform (Claude Code, OpenCode, Codex).

## Requirements

### Requirement: Platform Templates

The system SHALL maintain templates for each supported agent platform: Claude Code, OpenCode, and Codex.

#### Scenario: Claude Code template

- GIVEN the packaging system generates for Claude Code
- WHEN templates are rendered
- THEN the output SHALL include SKILL.md, sub-agent definitions, and MCP config in Claude Code's native format

#### Scenario: OpenCode template

- GIVEN the packaging system generates for OpenCode
- WHEN templates are rendered
- THEN the output SHALL include skill files, agent configs, and MCP server config in OpenCode's native format

#### Scenario: Codex template

- GIVEN the packaging system generates for Codex
- WHEN templates are rendered
- THEN the output SHALL include skill files and agent definitions in Codex's native format

### Requirement: Skill File Generation

The system SHALL generate the orchestrator SKILL.md from the pipeline template with platform-specific adaptations.

#### Scenario: Generate SKILL.md

- GIVEN a target platform
- WHEN skill file generation runs
- THEN the system SHALL produce a complete SKILL.md encoding the 7-stage pipeline for that platform

### Requirement: Sub-Agent Definition Generation

The system SHALL generate sub-agent definition files for all 4 agents (preflight, contrib, custom, theme).

#### Scenario: Generate all sub-agent defs

- GIVEN a target platform
- WHEN sub-agent generation runs
- THEN the system SHALL produce 4 definition files with correct model routing and MCP tool assignments

### Requirement: MCP Config Generation

The system SHALL generate MCP server configuration pointing to the `drup` binary.

#### Scenario: MCP config with binary path

- GIVEN the drup binary location
- WHEN MCP config is generated
- THEN the system SHALL produce config with `{command: "drup", args: ["mcp"]}` in the platform's config format
