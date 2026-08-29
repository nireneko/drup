# Upgrade Run State Machine

This specification makes `docs/workflow.md` enforceable. Go owns the upgrade
run state and rejects invalid transitions; the LLM orchestrator only asks for
the next permitted action and dispatches the appropriate agent.

## Outcome

Every mutating MCP operation belongs to one persisted upgrade run. The MCP can
answer what completed, what failed, and what action is permitted next, even
when the agent session is restarted.

## Scope

This adds a project-local run record at `.drup/runs/<run-id>.json`, MCP tools
to create/read/advance runs, and transition guards around existing mutators.
It does not replace the existing session, backup, or mutation-audit guards;
those remain additional safety checks.

## Run Record

```json
{
  "id": "run-20260824-abc123",
  "project_path": "/absolute/project",
  "target_major": 11,
  "commit_strategy": "per-fix",
  "scope": ["all"],
  "status": "active",
  "phase": "git_safety",
  "original_branch": "main",
  "original_commit": "abc123",
  "upgrade_branch": "upgrade/drupal-11",
  "current_major": 10,
  "next_major": 11,
  "backups": [],
  "evidence": [],
  "pending_human": [],
  "created_at": "2026-08-24T12:00:00Z",
  "updated_at": "2026-08-24T12:00:00Z"
}
```

`evidence` is append-only. Every item records the phase, target, producing
agent/tool, outcome, timestamp, and structured payload or artifact paths.
Never store secrets or raw command output that may contain credentials.

## States And Transitions

| State | Required evidence to enter | Permitted next state |
|---|---|---|
| `git_safety` | canonical path, Git repository, clean tree, original branch/commit, upgrade branch | `environment` |
| `environment` | environment, Composer, PHP, Drush, DB, web root, immediate-major compatibility, upgrade-needed decision | `tooling` or `completed` |
| `tooling` | Drush, Upgrade Status, Rector, analysis dependencies; temporary packages recorded | `initial_backup` |
| `initial_backup` | bound session and backup ID/path | `baseline` |
| `baseline` | core/PHP, enabled custom extensions, exact contrib versions, patches, config state, categorized findings | `custom_theme` |
| `custom_theme` | scoped validation, config export, optional validated commit, checkpoint backup | `contrib_patch` |
| `contrib_patch` | backup, updates, `updb`, validation/smoke checks, config export, optional commit | `contrib_minor` |
| `contrib_minor` | same checkpoint evidence as `contrib_patch` | `contrib_major` |
| `contrib_major` | one package only per checkpoint; same checkpoint evidence | `core_loop` |
| `core_loop` | one immediate major only: backup, preview, user confirmation, upgrade, `updb`, cache/status, validation, config export, optional commit | `contrib_patch` for the next major or `cleanup` at target |
| `cleanup` | final validation, tests/smoke checks, explicit temporary-tool policy, config export if changed, optional commit | `report` |
| `report` | root Markdown report with versions, patches, backups, commits, exports, failures, and pending-human work | `completed` |
| `blocked` | failure or human decision required | same state after a recorded resolution, or `abandoned` |
| `completed` / `abandoned` | terminal | none |

`core_loop` returns to `contrib_patch` for the newly reached immediate major.
The transition function must reject a target that skips a major.

## MCP Contract

Add these tools. All responses use the existing `{status, summary, payload}`
envelope.

| Tool | Inputs | Result |
|---|---|---|
| `run_create` | `project_path`, `target_major`, `commit_strategy`, `scope` | Creates a run in `git_safety`; rejects another active run for the project. |
| `run_status` | `project_path`, optional `run_id` | Returns state, allowed actions, checkpoint evidence, pending-human list, and next target. |
| `run_record` | `project_path`, `run_id`, `action`, `target`, `evidence` | Records evidence and performs a valid transition only when its prerequisites are present. |
| `run_confirm` | `project_path`, `run_id`, `action` | Records explicit user confirmation; initially required only for real core mutations and destructive restore. |
| `run_block` | `project_path`, `run_id`, `reason`, `target` | Records a retryable blocker or a pending-human item. |
| `run_abandon` | `project_path`, `run_id`, `reason` | Ends a run without deleting backups or evidence. |

Every existing mutating tool gains a required `run_id`. Its guard verifies that
the run is active, belongs to the same canonical project root, and permits that
tool in the current state. A rejected call must not mutate and must explain the
expected state/action.

Read-only tools do not require a `run_id`. `session_open`, backup freshness,
the mutation cap, and the new run guard are evaluated together for mutations.

## Action Rules

The transition layer, not the agent, enforces these rules:

- `test_backup_create` is allowed only after Git/environment readiness and is
  required before every mutating checkpoint.
- Composer, patch, Rector, compatibility, database-update, config-export, and
  core mutation actions are allowed only in their matching state.
- A validator result is recorded separately from a fixer result. A commit is
  allowed only after the corresponding validator evidence passes.
- A major contrib checkpoint accepts exactly one package target.
- `core_upgrade_apply` requires `run_confirm(action="core_upgrade")` and a
  target equal to the recorded immediate next major.
- `test_backup_restore` requires `run_confirm(action="restore")`; automatic
  restore and automatic backup deletion remain forbidden.
- A failed validator, Composer conflict, patch conflict, or unavailable
  release moves the run to `blocked` or adds a pending-human item with the
  attempted action and evidence.

## Orchestrator Contract

The platform skills must use the run API as their source of truth:

1. Call `run_create` after the user agrees to scope and commit strategy.
2. Before every dispatch and after every report, call `run_status`.
3. Dispatch only the agent/action listed in `allowed_actions`.
4. Have the responsible agent call `run_record` with its own evidence.
5. Ask the user when `run_status` reports a confirmation or human decision is
   required. Do not infer approval from earlier conversation.
6. Generate the final report from `run_status` evidence, then record `report`.

The skills must not claim a phase passed from prose alone. A report without a
successful `run_record` is not a checkpoint.

## Persistence And Recovery

- Write records atomically: create a temporary file in `.drup/runs/`, fsync if
  supported by the existing persistence conventions, then rename it.
- Keep completed and abandoned runs for reporting and recovery; never remove a
  retained backup as part of state cleanup.
- On server restart, `run_status` reads the project record and returns the
  same next action.
- Reject concurrent active runs for the same canonical project root.
- Store a schema version in the record so future migrations can be explicit.

## Implementation Boundaries

Suggested packages:

- `internal/runstate`: record types, atomic persistence, transition validation.
- `internal/app`: MCP handlers and composition with `guardedCall`.
- `internal/mcp/server.go`: tool schemas and descriptions.
- `internal/packaging/templates`: orchestrator and agent contracts that call
  the run tools.

Do not put transition logic in prompts or duplicate it across mutating tool
handlers. One guard in Go is the authority.

## Acceptance Criteria

- [ ] A new run cannot mutate before Git safety, environment, tooling, and an
  initial backup are recorded.
- [ ] A mutating MCP tool without a valid `run_id` is refused without changes.
- [ ] A commit without independent validator evidence is refused.
- [ ] A core upgrade without confirmation or with a skipped major is refused.
- [ ] A major contrib checkpoint cannot update more than one package.
- [ ] Restarting the MCP server preserves the active run and its next action.
- [ ] The final report can be reconstructed from the persisted run evidence.
- [ ] Unit tests cover every valid transition and representative invalid
  transitions; handler tests prove guards prevent underlying mutations.
