# Design: Drupal Upgrade Orchestrator (No-Execute Coordinator)

## Technical Approach

Split responsibility along the README rule: deterministic work in Go/MCP, flow in agent prompts. The orchestrator SKILL.md becomes a pure coordinator state machine — zero Bash/MCP calls — that only dispatches sub-agents and talks to the user. A new `drup-validator` sub-agent owns every `scan`/`validate`/`upgrade_scan` call, preserving external-validation independence without orchestrator execution. Three deterministic MCP tools close the capability gaps. Prompt style and dispatch/report envelope follow the gentle-ai coordinator pattern.

## Architecture Decisions

| Decision | Choice | Alternatives rejected | Rationale |
|----------|--------|-----------------------|-----------|
| Self-approval vs no-execute conflict | Dedicated `drup-validator` sub-agent owns all validation calls | Orchestrator keeps `validate` (current); let each agent self-validate | Orchestrator must have zero execute perms; separate validator keeps the "no self-approval" invariant intact |
| Command verb | `drup drupal-upgrade` | `drup core-upgrade`; overload `drup upgrade` | `drup upgrade` already = binary self-update (`internal/update`); avoid vocabulary collision |
| Rector as agent | Promote rector to a dedicated `drup-rector` sub-agent | Keep rector inline as an orchestrator stage | Orchestrator can no longer call `autofix` itself; 6-agent roster (preflight, rector, contrib, custom, theme, validator) |
| Core-upgrade safety | `core_upgrade_apply` commits a git checkpoint before mutating, supports `dry_run` | Fully autonomous mutation; no checkpoint | composer.json mutation must be reversible; dry-run lets validator/user preview |
| gentle-ai depth | Adopt BOTH prompt style AND structured report envelope | Only dispatch contract | Consistency with reference; machine-parseable agent handoffs |
| New tool placement | Placeholder in `internal/mcp/tools.go`, real handler in `internal/app/mcp_tools.go`, logic in new packages | Inline logic in handlers | Mirrors existing 17-tool two-layer pattern |

## Data Flow

    User ──/drup drupal-upgrade <path>──▶ ORCHESTRATOR (no-execute)
                                              │ dispatch (structured msg)
        ┌───────────┬───────────┬────────────┼────────────┬───────────┐
        ▼           ▼           ▼             ▼            ▼           ▼
    preflight    rector     contrib       custom        theme    VALIDATOR
    (env/deps)  (autofix)  (per-module) (per-file)   (per-file)  (scan/validate)
        │           │           │             │            │           ▲
        └───────────┴───────────┴──────report envelope─────┴───────────┘
                                              │
                    orchestrator reads validator findings ──▶ decide next
                    (advance gate | retry agent | ask user | escalate model)

State lives in the orchestrator's conversation context (in-flight) and in git commits (durable, one commit per passed gate). No orchestrator-owned Go state. Every agent returns the gentle-ai report envelope: `status | summary | artifacts | evidence | risks`. The orchestrator NEVER trusts an agent's "done" — it dispatches `drup-validator` for the authoritative gate result before advancing or committing.

Decision flow: preflight → (validator gate) → rector → (gate) → contrib loop → custom loop → theme loop → final validator gate → report. User-confirmation gates fire on: unsupported project-manager terminal state, core-version bump apply, and any destructive/ambiguous action. On agent failure the orchestrator re-dispatches the SAME agent with validator evidence (max 2 retries), then escalates model, then adds to PENDING HUMAN LIST.

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/packaging/templates/{claude,opencode,codex}/SKILL.md` | Modify | Rewrite as no-execute coordinator state machine + report envelope |
| `.../templates/*/agents/drup-validator.md` | Create | New validator agent (owns scan/validate/upgrade_scan) |
| `.../templates/*/agents/drup-rector.md` | Create | Rector promoted to dedicated agent (owns autofix) |
| `internal/mcp/tools.go` | Modify | Register 3 placeholder handlers |
| `internal/app/mcp_tools.go` | Modify | 3 real handlers |
| `internal/coreupgrade/` | Create | `NextMajor`, `PreviewComposerPatch`, `Apply` (read + mutate) |
| `internal/patchreconcile/` | Create | `Reconcile` analysis over drupalorg |
| `internal/envdetect/envdetect.go` | Modify | Add `EnvUnsupported` terminal state |
| `openspec/config.yaml` | Modify | Remove stale cobra/llm/heal context |

## Interfaces / Contracts

MCP tool signatures (JSON in/out, matching existing handler style):

- `core_upgrade_check{project_path}` → `{current_version, next_version, composer_patch_preview, supported}` — read-only, no exec/git.
- `core_upgrade_apply{project_path, target_version, dry_run}` → `{success, report, rollback_checkpoint, stderr}` — requires clean tree; `dry_run=true` uses `composer require --dry-run` and returns preview only; on apply, commits checkpoint (returns SHA) then mutates.
- `patch_reconcile{module_machine_name, current_patch_url}` → `{newer_patches[], is_still_needed, recommendation}` — analysis-only via `drupalorg.SearchPatches` + `CheckRelease`; no mutation.

Sub-agent report envelope (all 6 agents): `{agent, status: pass|fail|blocked, summary, artifacts[], evidence, risks[]}`. Dispatch message (orchestrator→agent): `{scope, target (module/file), error_details, prior_evidence, expected_return}`. Agents receive the specific target + full error context, never partial snippets and never the whole project — matching gentle-ai context isolation.

Agent capability matrix:

| Agent | MCP tools | Role |
|-------|-----------|------|
| drup-preflight | detect_env, drush_exec | env detection, dep install, unsupported-PM terminal report |
| drup-rector | autofix | deterministic auto-fix |
| drup-contrib | contrib_check, contrib_upgrade_path, issue_patches, apply_patch, create_patch, patch_status, patch_rollback, patch_reconcile, core_upgrade_check, core_upgrade_apply | per-module resolution + core bump |
| drup-custom | (edits files) | per-file custom refactor |
| drup-theme | (edits files) | per-file twig/theme refactor |
| drup-validator | scan, validate, upgrade_scan, generate_report | authoritative gates + report |

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | `NextMajor`, `PreviewComposerPatch`, `Reconcile`, `EnvUnsupported` | Table-driven, testdata composer.json fixtures |
| Unit | tool handlers arg validation, path traversal, clean-tree guard | Table-driven, package-level exec override |
| Integration | drupalorg calls in reconcile | `httptest` (existing pattern) |
| Integration | `core_upgrade_apply` git checkpoint + revert | temp git repo, exec stub |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|----------|---------------|-----------------|-------------------|
| Documentation-like paths | N/A: no file-type classification introduced | — | — |
| Git repository selection | Applicable: `core_upgrade_apply` uses `git -C project_path` | Absolute project_path; reject `..` segments (match `upgrade_scan`) | Test rejects relative/traversal path; uses `-C` not cwd |
| Commit state | Applicable: checkpoint commit before mutation | Require clean tree (porcelain empty) before apply; abort if dirty | Test aborts on dirty tree; test checkpoint SHA returned |
| Push state | N/A: no push automation | — | — |
| PR commands | N/A: no PR automation | — | — |

Also: `core_upgrade_apply`/`patch_reconcile` inherit composer package-name validation and drush blocklist/metachar guards already in `internal/app`.

## Migration / Rollout

No data migration. Deliver as chained PRs to stay under the 400-line budget: PR#1 Go/MCP tools + envdetect state (with tests), PR#2 agent templates (validator, rector, coordinator rewrite) across 3 platforms, PR#3 config cleanup. All Drupal mutations are git-committed per step; revert commits to roll back. New tools are opt-in.

## Open Questions

- [ ] Confirm verb `drup drupal-upgrade` (assumed) vs `drup core-upgrade`.
- [ ] Confirm 6-agent roster (task context) vs 5-agent (proposal folded rector into orchestrator).
