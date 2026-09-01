# Upgrade workflow

The durable workflow is for agent-coordinated Drupal major upgrades. It turns a broad upgrade into persisted, reviewable checkpoints. CLI commands remain useful for focused work, but they do not by themselves create the full run authority.

## Operating principles

- Upgrade one immediate Drupal major at a time.
- Start with a clean, identifiable project and record the initial state.
- Separate mutation from independent validation.
- Create a fresh backup before guarded mutation checkpoints.
- Keep failed or unresolved work visible as blocked/pending-human evidence; do not hide it behind retries.
- Require explicit confirmation for real core upgrades and destructive restore.

## Lifecycle

```text
git safety → environment → tooling → initial backup → baseline
  → custom/theme → contrib patch → contrib minor → contrib major
  → core loop → cleanup → report → completed
```

A blocked run can be resolved only through a recorded action; completed and abandoned runs are terminal. The exact allowed action is persisted in `.drup/runs/<run-id>.json` and returned by `run_status`.

## A realistic run

1. **Create context.** The coordinator agrees scope and commit strategy, then creates a run for the canonical project root.
2. **Record readiness.** Check Git status, Composer, Drush, PHP/runtime, Drupal version, and the immediate target major. `preflight` may install required analysis dependencies.
3. **Open a session and back up.** Bind the MCP session to the project, create a backup, and record its evidence.
4. **Capture a baseline.** Inventory the project and run Upgrade Status to provide before/after evidence.
5. **Work custom code first.** Run Rector only on custom modules/themes, then use scoped custom/theme remediation where required. Validate independently and record the result.
6. **Resolve contrib in order.** Use the contributor plan and release/patch evidence for the current immediate major. Major contrib changes are bounded to one package per checkpoint.
7. **Upgrade core deliberately.** Preview the immediate step, obtain explicit confirmation, apply it, and execute the required checkpoint steps: backup, update, database update, cache rebuild, status, validation, configuration export, and any configured smoke tests.
8. **Finish with evidence.** Run final validation, retain backups, generate the report, and mark the run complete only when the workflow state permits it.

## Roles

| Role | Responsibility |
|---|---|
| Coordinator skill | Reads durable run status, asks for operator decisions, and dispatches bounded work. |
| `drup-preflight` | Environment/tooling preparation and session/backup prerequisites. |
| `drup-rector` | Deterministic Rector pass on custom modules/themes. |
| `drup-contrib` | Release, patch, contrib-plan, and core-operation work in scope. |
| `drup-custom` / `drup-theme` | Scoped non-Rector remediation. |
| `drup-validator` | Independent scan, validation, and reporting evidence. |

A role’s report is not a transition by itself. The MCP run API and its guards determine what may happen next. See [MCP tools](mcp-tools.md) and [Multi-agent contracts](multiagent-contracts.md).

## Human decision points

The coordinator must stop for an explicit operator decision when the workflow needs a core-upgrade confirmation, a destructive restore confirmation, an unresolved patch/release choice, or an action outside the persisted run authority. A clean technical workflow does not eliminate product, compatibility, or production-risk decisions.

## If a run is interrupted

Do not recreate history from chat. Query run status, inspect the recorded checkpoint/evidence, and inspect backup/restore journals. Resume only with the action allowed by the persisted state. If recovery is needed, follow [Safety and recovery](safety-and-recovery.md).
