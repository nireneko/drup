# Proposal: Configurable Per-Agent Models

## Intent

Model choice is baked into 18 embedded agent templates plus 3 `SKILL.md` roster tables. Users on a different provider, budget, or model generation must fork drup. `state.ModelOverrides` was declared in the MVP and never wired, so the config surface exists on disk but does nothing.

Two grounded defects raise urgency:
- `renderCodexAgentConfig` emits only `description` + `developer_instructions`, silently **dropping** `model = "gpt-4o-mini"`. Codex model selection is already a no-op.
- Opencode agents declare `model: openrouter/qwen/...` in frontmatter while their prose says "Default model: haiku". Generated assets contradict themselves.

## Scope

### In Scope
- Typed model-assignment config in `~/.config/drup/state.json`, replacing the dead `ModelOverrides` field.
- `packaging.Render` accepts assignments; substitutes `{{MODEL_DEFAULT}}` / `{{MODEL_ESCALATION}}`.
- Substitution across **21** surfaces: 18 agent frontmatter *and* their "Default model:" prose, plus each platform `SKILL.md` roster table.
- Codex TOML output carries the resolved model (ordering: substitute, then TOML convert).
- Per-platform built-in defaults when config is absent or partial.
- `docs/model-configuration.md`; tests for load, resolve, substitute, Codex conversion.

### Out of Scope
- Runtime model switching, cost tracking, provider credentials.
- Changing the two-attempt escalation *rule* (only which models it names).
- A `drup config` subcommand (see question 3).

## Capabilities

### New Capabilities
- `model-config`: resolution of effective default/escalation model per platform+agent, precedence, validation.

### Modified Capabilities
- `installer`: `Render` signature and model substitution during install/sync.
- `sub-agents`: agent models sourced from config, not template literals.
- `orchestrator-skill`: roster table model names become rendered, not fixed.

## Approach

`state` exposes assignments; `packaging` resolves `config → platform default → built-in`, substitutes placeholders before any Codex TOML conversion, and fails closed on unknown platform/agent keys. `installAgents` passes the resolved map. Unset config renders byte-identical output to today.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/state/state.go` | Modified | Replace dead `ModelOverrides` |
| `internal/packaging/packaging.go` | Modified | Param + substitution + Codex model |
| `internal/packaging/templates/**` | Modified | 21 files placeholdered |
| `internal/app/commands.go` | Modified | `installAgents` passes assignments |
| `docs/model-configuration.md` | New | Semantics + examples |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Typo'd model breaks all agents | Med | Validate keys; reject unknown agent/platform |
| Partial substitution → contradictory assets | High | Test asserting zero `{{MODEL_` remain |
| Codex TOML corruption | Med | Substitute before convert; golden tests |
| Silent config-shape drift from MVP field | Low | No migration; ignore legacy key, warn once |

## Rollback Plan

Revert the commit. `state.json` gains only an additive key that older builds ignore; `drup sync` restores baked-in models.

## Dependencies

None.

## Success Criteria

- [ ] Empty config renders output identical to pre-change
- [ ] Configured model appears in frontmatter, prose, and roster for all 3 platforms
- [ ] Codex `.toml` contains the resolved model
- [ ] No `{{MODEL_*}}` placeholder survives render
- [ ] `go test ./...` and `go vet ./...` pass

## Proposal question round

I could not prompt interactively. These need your answer before spec/design:

1. **Config shape** — nested (`claude → drup-rector → {default, escalation}`) or flat dotted keys? Nested matches the two-level semantics and is the recommended assumption.
2. **Escalation as data** — templates have *no* escalation slot today; it is prose only. Do you want escalation configurable (touches prose + roster in 21 files), or default-only in slice 1 (much smaller, keeps escalation hardcoded)?
3. **Editing surface** — hand-edited JSON only, or a `drup config set/get/list` subcommand? A subcommand changes the `cli-binary` capability and adds scope.
4. **Codex model drop** — fix `renderCodexAgentConfig` to emit the model in this change, or file it separately? It is a prerequisite for Codex config to have any effect.
5. **Validation strictness** — reject unknown model strings against an allowlist, or accept any string and let the platform fail? Allowlists age badly against new model releases.

Assumptions pending review: nested shape; escalation configurable; JSON editing only; Codex fix included; no model-name allowlist.
