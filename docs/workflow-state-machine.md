# Upgrade run state machine

The run state machine is implemented in `internal/runstate`. It is the durable authority for an agent-coordinated upgrade; prompts and agent reports do not independently authorize mutations.

## Persisted run

Each run is stored under:

```text
<project>/.drup/runs/<run-id>.json
```

A run contains a schema version, canonical project path, target major, commit strategy, scope, status, phase, persisted allowed actions, append-only evidence, confirmations, pending-human entries, checkpoint plans/history, inventory snapshots, and a sanitized contrib plan.

Evidence contains summaries and hashes rather than raw command output, because command output can contain project-sensitive information.

## Phases

| Phase | Meaning |
|---|---|
| `git_safety` | Project identity and Git safety evidence. |
| `environment` | Runtime and immediate-major readiness evidence. |
| `tooling` | Upgrade tooling preparation evidence. |
| `initial_backup` | Initial backup evidence. |
| `baseline` | Initial inventory/scan evidence. |
| `custom_theme` | Custom code and theme remediation checkpoint. |
| `contrib_patch` | Patch-level contrib compatibility checkpoint. |
| `contrib_minor` | Minor contrib update checkpoint. |
| `contrib_major` | Major contrib checkpoint; bounded to one package target. |
| `core_loop` | Immediate core-major checkpoint. |
| `cleanup` | Post-validation temporary-tool cleanup. |
| `report` | Final report evidence. |
| `completed` | Terminal completed phase. |

Run status is `active`, `blocked`, `completed`, or `abandoned`. The `allowed_actions` field is written with the run and must be read back rather than reconstructed from conversational context.

## Transitions and confirmations

The run API exposes `run_create`, `run_status`, `run_record`, `run_confirm`, `run_block`, and `run_abandon`. A caller records only an action offered by the active run. Explicit confirmation actions exist for `core_upgrade` and `restore`.

Checkpoint plans hold fixed steps—backup, update, database update, cache rebuild, site status, validation, configuration export, and optional smoke tests—with each step’s pending/running/succeeded/failed/unavailable evidence. The executor maps those names to fixed argv adapters; callers cannot put arbitrary shell snippets into a plan.

A checkpoint commit is denied unless its candidate hash, changed paths, target/scope, and independent validation hash match the persisted evidence.

## Recovery

On a restart, query `run_status` for the canonical project. Do not open a new run just because an agent session ended. A blocked run must be resolved through its recorded action; an unresolved restore journal blocks run-authorized mutations. Read [Safety and recovery](safety-and-recovery.md) before any restore.
