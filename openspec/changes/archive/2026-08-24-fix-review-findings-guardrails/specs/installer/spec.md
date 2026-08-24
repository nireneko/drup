# Delta for Installer

## ADDED Requirements

### Requirement: Locked MCP Launch Flag

The system SHALL support rendering a `--locked` launch argument in each platform's MCP config template (claude/opencode/codex). When present, `drup mcp` SHALL start with mutations disabled, equivalent to `DRUP_DISABLE_MUTATIONS=1`.

#### Scenario: Default install does not lock

- GIVEN a default installer run with no locked-mode selection
- WHEN the MCP config template renders
- THEN the rendered args SHALL NOT include `--locked`, preserving existing session-guard-only behavior

#### Scenario: User opts into locked mode

- GIVEN the user selects locked mode during install (or via config)
- WHEN the MCP config template renders for any platform
- THEN the rendered args SHALL include `--locked`, and `drup mcp` startup SHALL disable mutations for that server process

#### Scenario: Locked flag parity across platforms

- GIVEN locked mode is selected
- WHEN templates render for claude, opencode, and codex
- THEN all three SHALL include the equivalent `--locked` argument in their respective config formats
