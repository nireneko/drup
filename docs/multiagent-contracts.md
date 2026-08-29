# Multi-agent contract harness

The coordinator and every specialist agent exchange versioned JSON envelopes.
The contract is enforced before a transcript harness allows an MCP call; it is
not a workflow state machine and does not replace the existing session, audit,
backup, operation, or Git authority.

## Envelopes

All envelopes use `schema_version: "v1"` and bind the same identity:
`root`, immutable `candidate`, `run_id`, and `phase`.

- **Dispatch**: `agent`, `scope`, and opaque `payload`.
- **AgentReport**: `agent`, `status` (`pass`, `fail`, or `blocked`), summary,
  artifacts, evidence, and risks. Domain outcomes such as `updated` belong in
  evidence, not in `status`.
- **ValidationEvidence**: named independently observed checks.
- **CheckpointEvidence**: backup, validation, confirmation, or recovery
  evidence. Recording it does not authorize a new transition.

Unknown fields, unknown enum values, invalid schema versions, and a report
whose identity differs from its dispatch fail closed with diagnostics that name
the contract, JSON pointer, invalid value, and allowed values when applicable.

## Transcript corpus

`internal/multiharness.Corpus` exercises the happy path, dirty tree, backup
failure, bounded retries, isolated contrib-major work, sequential core calls,
confirmation rejection, and ambiguous recovery. The fake MCP derives its only
tool catalog from `mcp.ToolSpecs`, so tool schema changes cannot silently drift
from the harness.

Each `packaging.Render` output includes `contracts/agent-contract.json`. Tests
decode that rendered artifact for Claude, Codex, and OpenCode, then compare the
normalized trace from the same corpus. The artifact is render-time metadata;
the installer deliberately does not write it as an agent skill.

## Semantic trace identity

Rendered transcript entries declare a canonical `tool` plus the effect-relevant
arguments that apply to it. The harness currently records `project_path` for
project-scoped calls, `target_major` for `core_upgrade_apply`,
`composer_package` for `apply_patch`, and Composer's full `package` constraint
for `composer_require`. Request IDs and descriptions are intentionally omitted
from the semantic identity. This makes reordered core major steps and a contrib
package/major substitution observable without duplicating MCP schemas or
creating a new transition authority.
