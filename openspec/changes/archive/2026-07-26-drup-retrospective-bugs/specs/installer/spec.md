# Delta for Installer

## ADDED Requirements

### Requirement: Sub-skill Path Resolution

The system SHALL resolve sub-skill template paths to correct agent-native directories, stripping redundant prefixes and preventing `SKILL.md` from being treated as a directory component.

For a path like `skills/skills/foo/SKILL.md`, the system SHALL map it to `{agent.SkillsDir()}/foo/SKILL.md`.

#### Scenario: Sub-skill with nested skills/ prefix

- GIVEN a template path `skills/skills/drupal-contrib-patch-writer/SKILL.md/SKILL.md`
- WHEN `resolveFilePath()` resolves the path for OpenCode
- THEN the output SHALL be `~/.config/opencode/skills/drupal-contrib-patch-writer/SKILL.md`
- AND the system SHALL NOT create intermediate `skills/skills/` nesting

#### Scenario: Sub-skill with SKILL.md suffix in directory

- GIVEN a template path where `SKILL.md` appears as a directory segment
- WHEN `resolveFilePath()` resolves the path
- THEN the system SHALL treat `SKILL.md` as a filename, not a directory
- AND the final path SHALL end with exactly one `SKILL.md` filename

#### Scenario: Path resolution across all adapters

- GIVEN template paths for sub-skills
- WHEN `resolveFilePath()` resolves for Claude Code, OpenCode, and Codex
- THEN each adapter SHALL produce paths under its native skills directory
- AND no adapter SHALL produce nested `skills/skills/` paths

### Requirement: Slash Command Template Writing

The system SHALL write an OpenCode slash command file (`commands/drup.md`) when the OpenCode adapter is detected during installation.

#### Scenario: OpenCode slash command written on install

- GIVEN OpenCode is detected
- WHEN the installer writes assets
- THEN the system SHALL write a slash command template to `~/.config/opencode/commands/drup.md`
- AND the file SHALL contain a valid OpenCode command definition

#### Scenario: Slash command not written for non-OpenCode agents

- GIVEN only Claude Code is detected (OpenCode is not)
- WHEN the installer writes assets
- THEN the system SHALL NOT write any slash command file
