# Proposal: Drupal Upgrade Orchestrator (No-Execute Coordinator)

## Intent

Redesign the shipped Drupal-upgrade agent flow around a hard constraint: the orchestrator is a pure coordinator with ZERO execute permissions. It only dispatches sub-agents and talks to the user. Today the orchestrator itself calls `validate` (self-approval), which violates the constraint. We also close three capability gaps (core version bump, unsupported-PM detection, patch reconciliation) and align prompts with proven orchestrator/sub-agent patterns.

## Scope

### In Scope
- Rewrite orchestrator SKILL.md as pure coordinator: sub-agent dispatch + user comms only, no Bash/MCP calls.
- Add a `drup-validator` sub-agent owning all `scan`/`validate`/`upgrade_scan` calls (preserves external-validation independence).
- Define sub-agent dispatch/report contract aligned with gentle-ai.
- Add 3 deterministic MCP tools: `core_upgrade_check`/`core_upgrade_apply`, unsupported-PM terminal state, `patch_reconcile`.
- Refresh `openspec/config.yaml` context (remove non-existent cobra/llm/heal).
- Keep SKILL.md + agent templates in sync across claude/opencode/codex.

### Out of Scope
- Rewriting the existing 4 sub-agents (only validator is new).
- Changing existing MCP tool implementations (only add missing ones).
- CLI binary structure changes (agent-level work only).

## Capabilities

### New Capabilities
- `core-upgrade`: deterministic composer.json `drupal/core` major-version bump (check + apply).
- `patch-reconcile`: patch lifecycle loop — detect newer patch on applied one, verify still needed.

### Modified Capabilities
- `orchestrator-skill`: no-execute coordinator; remove orchestrator-owned `validate`.
- `sub-agents`: add `drup-validator` role + model routing.
- `preflight`: add explicit terminal "unsupported project manager" error state (envdetect).

## Approach

Follow the README rule: deterministic work in Go/MCP, flow in agent prompts. Introduce validator sub-agent so no-self-approval survives without orchestrator execution. New Go packages mirror existing patterns (package-level exec/http overrides, table-driven tests, testdata fixtures). Adopt proven orchestrator prompt style with structured sub-agent report envelope for clarity and consistency.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/packaging/templates/{claude,opencode,codex}/SKILL.md` + `agents/*.md` | Modified | Coordinator rewrite + validator agent |
| `internal/mcp/tools.go`, `internal/app/mcp_tools.go` | Modified | 3 new tool handlers |
| `internal/envdetect` | Modified | Unsupported-PM terminal state |
| new Go pkgs (core bump, patch reconcile) | New | Deterministic capabilities |
| `openspec/config.yaml` | Modified | Fix stale context |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| 3-platform SKILL.md drift | High | Single source template, sync check |
| `core_upgrade_apply` mutates composer.json | Med | Git commit before apply; dry-run check tool |
| Naming collision (`drup upgrade` = self-update) | High | Reserve new verb (see question round) |
| >400-line review budget | Med | Chain PRs: tools, then prompts |

## Rollback Plan

All Drupal mutations are git-committed per step; revert commits to roll back. Agent template changes are additive/replaceable from git. New MCP tools are opt-in; unused tools have no runtime effect.

## Dependencies

- Reference architectural patterns from gentle-ai for orchestrator prompt style and sub-agent contract definition.
- User confirmation on new command naming (see question round).

## Success Criteria

- [ ] Orchestrator SKILL.md contains zero direct tool/Bash calls; all execution delegated.
- [ ] `drup-validator` owns every scan/validate call; no self-approval path remains.
- [ ] `core_upgrade_check/apply`, unsupported-PM state, and `patch_reconcile` implemented with tests.
- [ ] Templates identical in intent across claude/opencode/codex.
- [ ] `openspec/config.yaml` context matches real codebase.

## Proposal question round

These product decisions are unresolved and would make the spec ambiguous or risky. Recommend confirming before sdd-spec:

1. **Command naming**: `drup upgrade` already = binary self-update. What verb for Drupal-core upgrade — `drup drupal-upgrade` (recommended, unambiguous) or `drup core-upgrade`?
2. **Sub-agent roster**: Is 5 agents (preflight, contrib, custom, theme, + new validator) the right split, or should validator absorb preflight's scan role?
3. **Orchestrator prompt depth**: Adopt full structured orchestrator prompt style with report envelope, or only the minimal sub-agent dispatch contract?
4. **core_upgrade_apply safety**: Should apply be gated behind an explicit user confirmation step given it mutates composer.json, or fully autonomous within the pipeline?
5. **Delivery slicing**: Accept chained PRs (Go tools first, then agent prompts) to stay under the 400-line review budget?

Assumptions taken if unanswered: command `drup drupal-upgrade`; 5-agent roster with dedicated validator; adopt full structured orchestrator prompt style with report envelope; `core_upgrade_apply` commits-then-applies autonomously with prior git checkpoint; deliver as chained PRs.
