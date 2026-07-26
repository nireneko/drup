# Delta for Platform Bootstrap

## ADDED Requirements

### Requirement: Slash Command Template Generation

The system SHALL generate an OpenCode slash command template as part of bootstrap file generation. The slash command template registers the `/drup` command for OpenCode.

#### Scenario: OpenCode bootstrap includes slash command

- GIVEN `drup install` targets OpenCode
- WHEN bootstrap generation runs
- THEN it SHALL produce a slash command template at `internal/packaging/templates/opencode/commands/drup.md`
- AND the template SHALL contain a valid OpenCode command JSON definition

#### Scenario: Slash command template is rendered during install

- GIVEN the slash command template exists in `internal/packaging/templates/opencode/commands/`
- WHEN `drup install` runs for OpenCode
- THEN it SHALL render the template to `~/.config/opencode/commands/drup.md`

#### Scenario: Slash command template has correct format

- GIVEN the slash command template is read
- WHEN the template content is validated
- THEN it SHALL contain a valid OpenCode command definition with the drup MCP invocation
