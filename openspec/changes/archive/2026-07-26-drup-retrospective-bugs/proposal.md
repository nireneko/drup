# Proposal: Fix Retrospective Bugs (drup-retrospective-bugs)

## Intent

Three code bugs from `UPGRADE-RETROSPECTIVA.md` and `informe-drup-bugs.md` remain open. They block `drup init` with `drupal/core-recommended`, sub-skill installation (nested `skills/skills/.../SKILL.md/SKILL.md` paths), and OpenCode `/drup` slash command registration. Plus basic HTTP retry for Drupal.org 412/timeout errors.

## Scope

### In Scope
- Fix `RunInit()` to accept `drupal/core-recommended` alongside `drupal/core`
- Fix `resolveFilePath()` nested paths and `SKILL.md`-as-directory
- Add `commands/drup.md` template for OpenCode slash command
- Add retry/backoff to Drupal.org HTTP calls
- Unit tests for all three code bugs

### Out of Scope
- Major HTTP client refactor
- Skill/agent template redesign
- Agent model config changes
- XML parse error fix for `contrib_upgrade_path`

## Capabilities

### New Capabilities
- `drupal-org-resilience`: HTTP retry/backoff for Drupal.org API calls

### Modified Capabilities
- `installer`: Fix `resolveFilePath()` sub-skill paths; add slash command template writing
- `platform-bootstrap`: Add OpenCode slash command template to bootstrap generation

## Approach

1. **`RunInit`** — Add `drupal/core-recommended` check at `commands.go:55`, matching `checkCoreReadiness()` pattern. Update error message.
2. **`resolveFilePath`** — Add `strings.HasPrefix(path, "skills/")` case stripping prefix, mapping to `agent.SkillsDir() + rest`. Handle `SKILL.md` suffix.
3. **Slash command** — Create `templates/opencode/commands/drup.md` with valid OpenCode command JSON. Wire into installer for OpenCode adapter.
4. **HTTP retry** — Wrap Drupal.org calls with 3-attempt exponential backoff (500ms base).

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modified | `RunInit()` core check |
| `internal/installer/installer.go` | Modified | `resolveFilePath()` fix |
| `internal/packaging/templates/opencode/commands/` | New | Slash command template |
| `internal/drupalorg/drupalorg.go` | Modified | HTTP retry wrapper |
| `internal/installer/installer_test.go` | Modified | Path resolution tests |
| `internal/app/commands_test.go` | Modified | `RunInit` tests |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `resolveFilePath` fix breaks Claude/Codex | Low | Table-driven test covering all three adapters |
| Slash command format mismatch | Low | Validate against OpenCode schema; manual test |
| Retry delays legitimate failures | Low | 3 attempts, 500ms base; log each retry |

## Rollback Plan

Revert the commit. All changes are localized to single functions with no data migration or schema changes. Re-run `drup install` after rollback.

## Dependencies

None external.

## Success Criteria

- [ ] `drup init` succeeds with `drupal/core-recommended` (no `drupal/core` in require)
- [ ] `drup install` writes sub-skills to `~/.config/opencode/skills/<name>/SKILL.md` (no nesting)
- [ ] `drup install` creates `/drup` slash command for OpenCode
- [ ] Drupal.org calls retry on 412/timeout up to 3 times
- [ ] Unit tests pass for all three code bugs
- [ ] Manual `drup install` produces correct file layout
