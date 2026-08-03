# Model Configuration Specification

## Purpose

Allow users to override per-phase and per-agent model assignments through configuration, replacing hardcoded defaults while maintaining backward compatibility.

## Requirements

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

This is **functional** byte-identity, not literal source-file byte-identity: REQ-004/REQ-005/REQ-007 (in the installer and orchestrator-skill specs) intentionally rewrite template source (substituting hardcoded model literals for `{{MODEL_DEFAULT}}` / `{{MODEL_ESCALATION}}` placeholders across 18 agent templates and 3 SKILL.md rosters). An empty/nil `ModelAssignments` resolves those placeholders back to the same built-in values that were previously hardcoded, so the *rendered model assignment per agent* is unchanged — the agent behaves identically — even though the template source and, consequently, the rendered file bytes around the substituted fields differ from the pre-change output.

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

## Dependency Notes (gentle-ai)

Investigated for a dynamic model-name validation/discovery pattern; none exists locally. `gentle-ai` tooling available in this environment is a code-review lens orchestrator (`gentle-ai review ...`), unrelated to LLM model catalogs. Decision: free-form string validation, no allowlist — consistent with the proposal's own assumption.
