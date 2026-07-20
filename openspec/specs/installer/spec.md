# Installer Specification

## Purpose

Detect installed AI agents, write skill files and MCP configuration to native directories, manage backups and state tracking.

## Requirements

### Requirement: Agent Detection

The system SHALL detect which AI agents are installed on the system by checking known configuration paths.

#### Scenario: Detect Claude Code

- GIVEN Claude Code is installed with config at `~/.claude/`
- WHEN agent detection runs
- THEN the system SHALL report Claude Code as detected with its config directory

#### Scenario: Detect OpenCode

- GIVEN OpenCode is installed with config at `~/.config/opencode/`
- WHEN agent detection runs
- THEN the system SHALL report OpenCode as detected

#### Scenario: No agents detected

- GIVEN no known agent config directories exist
- WHEN agent detection runs
- THEN the system SHALL report "no agents detected" and suggest installing one

### Requirement: Asset Writing

The system SHALL write generated skill files and MCP config to each detected agent's native directory.

#### Scenario: Write to Claude Code

- GIVEN Claude Code is detected
- WHEN the installer writes assets
- THEN the system SHALL write SKILL.md and sub-agent defs to Claude Code's skills directory and MCP config to its config file

#### Scenario: Write to multiple agents

- GIVEN both Claude Code and OpenCode are detected
- WHEN the installer writes assets
- THEN the system SHALL write to both agents' native directories independently

### Requirement: Config Backup

The system SHALL backup existing configuration files before overwriting them.

#### Scenario: Backup before write

- GIVEN an existing MCP config file
- WHEN the installer is about to overwrite it
- THEN the system SHALL create a backup copy (tar.gz) before writing the new config

#### Scenario: Backup retention

- GIVEN multiple installer runs over time
- WHEN backups accumulate
- THEN the system SHALL retain the 5 most recent backups and prune older ones

### Requirement: State Tracking

The system SHALL maintain a `state.json` file tracking installation state.

#### Scenario: Track installed assets

- GIVEN a successful installation
- WHEN state.json is written
- THEN the system SHALL record: installed agents, file paths written, installation timestamp, and binary version

#### Scenario: State reset

- GIVEN state.json is deleted
- WHEN the installer runs
- THEN the system SHALL treat it as a fresh install with no prior state
