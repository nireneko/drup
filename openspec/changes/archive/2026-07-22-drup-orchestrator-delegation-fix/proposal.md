# Proposal: drup-orchestrator-delegation-fix

## Intent

The drup orchestrator violates its "coordinator only, zero execute" rule: it either executes work directly (modifying composer.json, running composer) or tells users to do it manually. Three root causes: permission/agent model is OpenCode-specific, tools contradict the skill's zero-execute rule, and no explicit pipeline stage for core upgrade.

Fix must work across OpenCode, Claude Code, and Codex — not tied to any single platform's agent model.

## Scope

### In Scope
- Cross-platform SKILL.md — no OpenCode-specific primitives (`task()`, agent definitions). Pipeline described in natural language; each stage is a `drup <stage>` CLI command.
- New `drup upgrade-core` CLI command — full pipeline: update composer.json target version, `composer require drupal/core`, `drush updb`, verify result.
- Ensure every pipeline stage has a deterministic `drup` CLI command (preflight, rector, contrib, upgrade-core, custom, theme, validate).
- Platform bootstrap files: `opencode.json` (load SKILL.md as skill), `CLAUDE.md` (load SKILL.md), `.github/copilot-instructions.md` (load SKILL.md).
- Template sources updated so `drup install` regenerates cross-platform config.

### Out of Scope
- MCP server (future concern)
- Platform-specific agent definitions (no `drup-*` sub-agents in opencode.json)
- Changes to `gentle-orchestrator` or SDD workflow

## Capabilities

### New Capabilities
- `core-upgrade`: New spec covering the `drup upgrade-core` CLI command — composer.json manipulation, composer require, drush updb, result verification.

### Modified Capabilities
- `orchestrator-skill`: Rewrite for cross-platform — no opencode-specific primitives, every stage is a `drup` CLI command, AI orchestrates by calling CLI tools only.
- `agent-packaging` (renamed to `platform-bootstrap`): Generate bootstrap files for OpenCode (opencode.json skill entry), Claude Code (CLAUDE.md), and Codex (copilot-instructions.md).

## Approach

1. **Cross-platform SKILL.md**: Rewrite the pipeline so each stage corresponds to `drup <stage>`. The AI reads the skill and calls CLI commands — never modifies files directly.
2. **New CLI command `drup upgrade-core`**: Implements the full core upgrade pipeline (composer.json change → composer require → drush updb → verify). What was a manual step becomes a deterministic CLI command.
3. **Platform bootstraps**: Thin files per platform that tell the AI "load drup/SKILL.md and follow it." SKILL.md is the single source of truth.
4. **Template sync**: Update packaging templates so `drup install` generates cross-platform bootstrap files.
5. **Go code**: Add `drup upgrade-core` command in `cmd/` and `internal/upgrade/`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `SKILL.md` (template + installed) | Rewritten | Cross-platform pipeline, no opencode-specific primitives, every stage is `drup <stage>` |
| `cmd/` + `internal/upgrade/` | New | `drup upgrade-core` CLI command implementation |
| `~/.config/opencode/opencode.json` | Modified | Remove `drup-orchestrator` agent, add SKILL.md as skill entry |
| `CLAUDE.md` (project root) | New | Bootstrap for Claude Code — load SKILL.md |
| `.github/copilot-instructions.md` | New | Bootstrap for Codex — load SKILL.md |
| `internal/packaging/templates/` | Modified | Templates for all platform bootstrap files |
| `openspec/specs/orchestrator-skill/spec.md` | Modified | Cross-platform pipeline spec |
| `openspec/specs/agent-packaging/spec.md` | Modified | Rename to platform-bootstrap, cover all 3 platforms |
| `openspec/specs/core-upgrade/spec.md` | New | Spec for drup upgrade-core command |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|-------------|
| `drup upgrade-core` needs Drupal site to test | Med | Integration test with a minimal Drupal scaffold; flag to skip drush steps |
| Codex ignores copilot-instructions.md for non-IDE use | Low | Fallback: the SKILL.md works as plain instructions regardless of platform |
| AI still tries to modify composer.json directly instead of calling CLI | Low | SKILL.md explicitly says "NEVER modify files directly — call `drup upgrade-core`" |

## Rollback Plan

Revert SKILL.md to previous version. Revert opencode.json CLAUDE.md copilot-instructions.md. Remove `drup upgrade-core` command. Project data files are never modified by drup itself.

## Dependencies

- Drupal site with composer.json for testing upgrade-core
- Go standard library + `encoding/json` for composer.json manipulation

## Success Criteria

- [ ] SKILL.md contains zero OpenCode-specific primitives — readable by any AI
- [ ] Every pipeline stage is a `drup <stage>` CLI command, including core upgrade
- [ ] `drup upgrade-core` runs the full core upgrade pipeline without manual steps
- [ ] OpenCode, Claude Code, and Codex can all follow the skill
- [ ] `drup install` generates correct bootstrap files for all platforms
