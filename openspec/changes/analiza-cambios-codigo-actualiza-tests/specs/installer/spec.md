# Delta for installer

## ADDED Requirements

### Requirement: WriteSkill Directory Structure

The test suite MUST verify that `WriteSkill` creates skills as directories (`<name>/SKILL.md`) rather than flat files.

#### Scenario: WriteSkill creates directory with SKILL.md

- GIVEN a detected agent with skills directory at `<agent_dir>/skills/`
- WHEN `WriteSkill("drup", content)` runs
- THEN `<agent_dir>/skills/drup/SKILL.md` SHALL exist with the given content
- AND `<agent_dir>/skills/drup/` SHALL be a directory

#### Scenario: WriteSkill for each detected agent

- GIVEN both Claude Code and OpenCode detected
- WHEN `Install` runs
- THEN each agent's skills directory SHALL contain `drup/SKILL.md`

### Requirement: WriteCommand Adapter Tests

The test suite MUST verify that `WriteCommand` produces correct adapter-specific output for OpenCode and Codex.

#### Scenario: OpenCode WriteCommand creates wrapper

- GIVEN OpenCode detected with binary at `/usr/local/bin/drup`
- WHEN `WriteCommand` runs for OpenCode
- THEN the command wrapper SHALL be written to the OpenCode commands directory
- AND the wrapper SHALL invoke the drup binary with correct args

#### Scenario: Codex WriteCommand creates wrapper

- GIVEN Codex detected
- WHEN `WriteCommand` runs for Codex
- THEN the command adapter SHALL be written to the Codex commands directory

### Requirement: RunUninstall Command Tests

The test suite MUST verify the `RunUninstall` command's state-driven flow, flag handling, and error paths at the command level.

#### Scenario: State-driven adapter selection

- GIVEN state.json listing Claude Code and OpenCode as installed agents
- WHEN `RunUninstall` runs
- THEN the system SHALL call Remove methods for both detected adapters

#### Scenario: Dry-run output

- GIVEN a valid state.json
- WHEN `RunUninstall --dry-run` runs
- THEN the system SHALL print what would be removed WITHOUT executing any removal

#### Scenario: Force mode with missing state

- GIVEN no state.json exists
- WHEN `RunUninstall --force` runs
- THEN the system SHALL skip state loading and attempt direct cleanup

#### Scenario: Self-removal error handling

- GIVEN the binary path is not writable
- WHEN self-removal is attempted
- THEN the system SHALL report the error without panicking

### Requirement: RemoveMCPConfig Per-Agent Path Regression

The test suite MUST verify that `RemoveMCPConfig` targets the correct config path for each agent adapter.

#### Scenario: Claude RemoveMCPConfig removes correct file

- GIVEN Claude MCP config at `~/.claude/mcp/drup.json`
- WHEN `RemoveMCPConfig` runs
- THEN only `~/.claude/mcp/drup.json` SHALL be removed
- AND other files in `~/.claude/mcp/` SHALL remain untouched

#### Scenario: OpenCode RemoveMCPConfig removes only drup key

- GIVEN `opencode.json` with `drup` and other MCP entries
- WHEN `RemoveMCPConfig` runs
- THEN only the `drup` key SHALL be removed from `mcp`
- AND other MCP entries SHALL be preserved
