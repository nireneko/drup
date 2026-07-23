# Proposal: Reorder Pipeline — Core Upgrade After Custom Loop

## Intent

Move the core upgrade stage from position 5 (before custom loop) to position 6 (after custom loop, before final validation). The user's rationale: custom code fixes should be applied against the current Drupal version first, then core is upgraded as the last mutation step before final validation.

## Scope

### In Scope
- Reorder pipeline stages in all 3 agent SKILL.md templates (claude, opencode, codex)
- Update orchestrator-skill spec pipeline definition
- Update stage cross-references (final validation re-entry, confirmation gates)
- Renumber: custom loop becomes Stage 5, core upgrade becomes Stage 6

### Out of Scope
- Changing core upgrade logic (`internal/coreupgrade/`) — same operation, different position
- Adding new CLI commands or MCP tools
- Modifying validation gates or report generation logic

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `orchestrator-skill`: Pipeline Definition requirement changes stage order from `...contrib → core upgrade → custom loop → ...` to `...contrib → custom loop → core upgrade → ...`

## Approach

Documentation-only reorder across template files and specs. No Go code changes — the CLI dispatch (`app.go`) is command-based, not stage-ordered. The pipeline sequence lives entirely in SKILL.md templates and the orchestrator spec.

Files to update:
| File | Change |
|------|--------|
| `internal/packaging/templates/{claude,opencode,codex}/SKILL.md` | Swap Stage 5/6 headers, renumber references |
| `openspec/specs/orchestrator-skill/spec.md` | Update Pipeline Definition requirement text and scenario |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/packaging/templates/*/SKILL.md` | Modified | Swap Stage 5 (CORE UPGRADE) ↔ Stage 6 (CUSTOM LOOP), update all cross-references |
| `openspec/specs/orchestrator-skill/spec.md` | Modified | Pipeline Definition requirement + scenario text |
| `~/.config/opencode/skills/drup/SKILL.md` | Modified | Installed copy — updated via `drup sync` after template change |

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Cross-reference missed (e.g., "Stage 5" in confirmation gates) | Low | Grep all templates for stage number references |
| Custom loop fixes break after core upgrade | Low | User's explicit choice — custom fixes target current version, core upgrade is last mutation |

## Rollback Plan

Revert the template and spec changes (git revert). The Go binary is unaffected — pipeline ordering is purely in documentation/templates.

## Dependencies

None

## Success Criteria

- [ ] All 3 SKILL.md templates show: Stage 5 = CUSTOM LOOP, Stage 6 = CORE UPGRADE
- [ ] Orchestrator spec pipeline definition matches new order
- [ ] No stale "Stage 5" references to core upgrade remain in templates
- [ ] `drup sync` propagates changes to installed agent configs
- [ ] Existing tests pass (`go test ./...`)
