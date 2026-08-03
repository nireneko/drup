# Spec Delta: Configurable Per-Phase Models

## New Capability: model-config

### Requirement: REQ-001 Nested Model Assignment Shape

The system SHALL represent model assignments as `platform -> agent -> {default, escalation}`, matching Go type `map[string]map[string]ModelPhaseAssignment` (`ModelPhaseAssignment{Default, Escalation string}`).

#### Scenario: Full assignment resolves both slots

- GIVEN config `{"claude":{"drup-rector":{"default":"claude-haiku-4-5-20251001","escalation":"claude-sonnet-5"}}}`
- WHEN `packaging.Render("claude", binaryPath, assignments)` runs
- THEN `drup-rector.md` frontmatter `model:` SHALL equal the configured default

#### Scenario: Unknown platform/agent key rejected

- GIVEN config contains platform key `"chatgpt"` (not in `Platforms()`)
- WHEN `Render` resolves assignments
- THEN the system SHALL fail closed, return an error, and write zero files

### Requirement: REQ-002 Model Name Validation Policy

The system SHALL accept any non-empty string as a model identifier and SHALL NOT validate it against an allowlist (see Dependency Notes). Structural keys (platform, agent name) MUST be validated against known values. Per-platform naming examples MUST be documented: `claude-haiku-4-5-20251001` (claude), `openrouter/qwen/qwen3-30b-a3b:free` (opencode), `gpt-4o-mini` (codex).

#### Scenario: Arbitrary but valid string accepted

- GIVEN `escalation: "some-future-model-id"`
- WHEN `Render` runs
- THEN the value SHALL pass through unmodified into frontmatter, prose, and roster

### Requirement: REQ-003 Backward-Compatible Defaults

Empty or missing `ModelAssignments` SHALL resolve to per-platform, per-agent built-in defaults identical to the currently hardcoded values, including `drup-validator`'s distinct non-cheap default (never the fixer-agent default). Partial config SHALL fall back to built-ins only for the unconfigured platform/agent pairs.

This is **functional** byte-identity, not literal source-file byte-identity: REQ-004/REQ-005/REQ-007 intentionally rewrite template source (substituting hardcoded model literals for `{{MODEL_DEFAULT}}` / `{{MODEL_ESCALATION}}` placeholders across 18 agent templates and 3 SKILL.md rosters). An empty/nil `ModelAssignments` resolves those placeholders back to the same built-in values that were previously hardcoded, so the *rendered model assignment per agent* is unchanged — the agent behaves identically — even though the template source and, consequently, the rendered file bytes around the substituted fields differ from the pre-change output.

#### Scenario: Empty config resolves to unchanged built-in models

- GIVEN `ModelAssignments` is empty/nil
- WHEN `Render` runs for any platform
- THEN every agent's resolved `model` value (frontmatter/TOML field and "Default model:" prose) SHALL equal the pre-change hardcoded value for that agent, producing zero behavior change

#### Scenario: Partial config leaves other agents untouched

- GIVEN config sets only `claude.drup-rector`
- WHEN `Render("claude", ...)` runs
- THEN `drup-contrib`, `drup-validator`, etc. SHALL use built-in defaults

### Requirement: REQ-008 State Persistence

`internal/state/state.go` SHALL expose `ModelAssignments map[string]map[string]ModelPhaseAssignment` under JSON key `model_assignments`, replacing the dead `model_overrides` field. A legacy `model_overrides` key present in an existing `state.json` SHALL be ignored with a one-time warning, with no migration.

#### Scenario: Round-trip through state.json

- GIVEN a state written with `ModelAssignments` populated
- WHEN `state.Load()` reads it back
- THEN the decoded struct SHALL equal the written value

### Requirement: REQ-009 Sync Reads And Renders

`drup sync` SHALL load state, resolve assignments per platform/agent per REQ-003, and call `packaging.Render(platform, binaryPath, resolved)`, writing only files whose content changed (existing `SyncFileResult` contract unaffected).

#### Scenario: Sync applies configured models

- GIVEN state.json has a non-empty `model_assignments`
- WHEN `drup sync` runs
- THEN written agent files SHALL reflect the configured models and `SyncFileResult` status SHALL be `modified` for changed files only

## Modified Capability: installer

### Requirement: REQ-004 Model Placeholder Substitution (ADDED)

`packaging.Render(platform, binaryPath, modelOverrides)` SHALL accept a third parameter and substitute `{{MODEL_DEFAULT}}` / `{{MODEL_ESCALATION}}` in all 18 agent templates' frontmatter/TOML `model` field and in their "Default model:" prose, plus each platform's `SKILL.md` roster table.

#### Scenario: Zero placeholders survive render

- GIVEN any valid or empty `modelOverrides`
- WHEN `Render` completes for any platform
- THEN no rendered file SHALL contain the substring `{{MODEL_`

### Requirement: REQ-005 Codex Model Field Preservation (ADDED)

`renderCodexAgentConfig` SHALL emit the resolved `model` field in the output TOML, fixing the current silent drop. Substitution SHALL happen before Markdown→TOML conversion.

#### Scenario: Codex TOML retains model

- GIVEN codex platform render with any resolved model for `drup-rector`
- WHEN `renderCodexAgentConfig` converts the template
- THEN the emitted `.toml` SHALL contain `model = "<resolved>"`

### Requirement: REQ-006 OpenCode Reconciliation (ADDED)

For OpenCode, the rendered frontmatter `model:` value and the rendered "Default model:" prose sentence MUST reference the same resolved model, eliminating today's frontmatter/prose contradiction.

#### Scenario: Frontmatter and prose agree

- GIVEN any config (including empty, using built-ins)
- WHEN an OpenCode agent file renders
- THEN the frontmatter `model:` value SHALL be reachable from the prose's stated default/escalation names

## Modified Capability: sub-agents

### Requirement: Model Routing

The system SHALL route sub-agents to a model tier resolved from `ModelAssignments`, falling back to built-in per-agent defaults when unconfigured. `drup-validator` SHALL keep its distinct non-cheap default, never equal to the fixer-agent cheap default.
(Previously: model tier names — haiku/sonnet — were hardcoded literals with no configuration path.)

#### Scenario: Cheap model for mechanical work

- GIVEN preflight, contrib, custom, or theme tasks with no override configured
- WHEN the orchestrator selects a model
- THEN the system SHALL use the built-in cheap default

#### Scenario: Escalation for custom code

- GIVEN custom code tasks that fail on the default model after 2 retries
- WHEN the orchestrator escalates
- THEN the system SHALL switch to the configured (or built-in) escalation model for that file

#### Scenario: Configured override changes routing

- GIVEN `ModelAssignments["claude"]["drup-custom"] = {Default: "claude-opus-4", Escalation: "claude-opus-4"}`
- WHEN the orchestrator selects a model for drup-custom
- THEN the system SHALL use `claude-opus-4` instead of the built-in default

## Modified Capability: orchestrator-skill

### Requirement: REQ-007 Roster Table Reflects Resolved Models (ADDED)

Each platform `SKILL.md` roster table row for a config-driven agent MUST render the actual resolved default/escalation model names after `drup sync`, not template literals.

#### Scenario: Roster reflects override

- GIVEN `claude.drup-rector` is overridden to `{default:"claude-opus-4", escalation:"claude-opus-4"}`
- WHEN `drup sync` writes `claude/SKILL.md`
- THEN the `drup-rector` roster row SHALL read `claude-opus-4 (2 retries)` and not the built-in literal

## Test Scenarios (Inputs → Outputs)

| # | Scenario | Input | Expected Output |
|---|----------|-------|------------------|
| 1 | Config round-trip | Populated `ModelAssignments`, `Save` then `Load` | Decoded struct equals written value |
| 2 | Substitution correctness | `{claude.drup-rector:{default:X, escalation:Y}}` | Frontmatter `model: X`; prose names X then Y |
| 3 | Backward compatibility | Empty `ModelAssignments` | Every agent's resolved model equals the pre-change hardcoded value (functional byte-identity; zero behavior change) |
| 4 | Codex field preserved | Any config, platform `codex` | `.toml` contains `model = "<resolved>"` |
| 5 | OpenCode reconciliation | Any config, platform `opencode` | Frontmatter and prose model names agree |
| 6 | Unknown key rejected | Platform `"chatgpt"` | `Render` errors, zero files written |
| 7 | Zero leftover placeholders | Any config or empty | No `{{MODEL_` substring in any output file |

## Dependency Notes (gentle-ai)

Investigated for a dynamic model-name validation/discovery pattern; none exists locally. `gentle-ai` tooling available in this environment is a code-review lens orchestrator (`gentle-ai review ...`), unrelated to LLM model catalogs. Decision: free-form string validation, no allowlist — consistent with the proposal's own assumption.
