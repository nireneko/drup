# Getting started

Use this guide to establish a safe baseline before an upgrade. Start with read-oriented commands; move to an agent-coordinated run only after you understand the project’s current state.

## Prerequisites

- A local `drup` binary (`drup version` succeeds).
- An absolute path to the target Drupal project.
- For scan and mutation-oriented operations: Composer, Drush, and the PHP/runtime environment used by that project. `drup` detects common container wrappers and invokes project tooling through that environment.
- Git when you intend to use preflight, checkpoints, or upgrade workflow commits.

`preflight` installs development analysis packages when they are missing, so it is not a read-only command. Begin with `scan` only when the site already has Upgrade Status and Drush available.

## Quick baseline

```bash
PROJECT=/absolute/path/to/drupal-project

drup preflight "$PROJECT"
drup scan "$PROJECT"
drup report "$PROJECT"
```

`scan` emits structured JSON from `drush upgrade_status:analyze --all --format=checkstyle`. `report` writes `drup-report.json` and `drup-report.md` at the project root based on a current validation scan.

Review both the scan output and the working tree before using a mutating command.

## First mechanical remediation

```bash
# Runs Rector against custom modules and themes, then scans again.
drup fix "$PROJECT"

# Inspect the exact change before accepting it.
git -C "$PROJECT" diff

# Preview Drupal 11 core-requirement widening in custom extensions.
drup compat-fix "$PROJECT" --dry-run
```

`fix` can create `rector.php` in the project root when the project has no Rector configuration. It intentionally avoids contributed code. A clean post-Rector scan is evidence about Rector’s effect, not proof that the upgrade is complete.

## Agent integration

```bash
# Detect installed agents and write drup assets.
drup install

# Re-render assets after changing model assignments or updating drup.
drup sync

# Install an MCP server configuration that rejects mutating tools.
drup install --locked
```

Restart the agent after installation. The installer integrates the following locations while preserving unrelated MCP configuration:

| Host | MCP configuration | Skills / agents |
|---|---|---|
| Claude Code | `~/.claude.json` | `~/.claude/skills/drup/`, `~/.claude/agents/` |
| OpenCode | `~/.config/opencode/opencode.json` | `~/.config/opencode/skills/drup/`, `~/.config/opencode/agents/` |
| Codex | `~/.codex/config.toml` | `~/.codex/skills/drup/`, `~/.codex/agents/` |

Codex also receives a prompt in `~/.codex/prompts/`; its prompt command is exposed by Codex as `/prompts:<name>`. The exact rendered prompt and agent files are owned by `internal/packaging`, not by a hand-edited installation.

## Choose the right interface

| Situation | Recommended interface |
|---|---|
| Inspect compatibility and scan output | CLI: `scan`, `contrib`, `issue`, `preflight` |
| Apply a clearly scoped mechanical Rector pass | CLI: `fix`, then review and scan |
| Coordinate backup, run state, validator evidence, checkpoints, and core changes | Installed agent + MCP |
| Integrate MCP in a controlled or read-only environment | `drup mcp --locked` |

Continue with [the upgrade workflow](upgrade-workflow.md) before initiating a durable run.
