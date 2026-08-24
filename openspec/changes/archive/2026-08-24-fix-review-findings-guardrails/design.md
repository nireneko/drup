# Design: fix-review-findings-guardrails

## Technical Approach

Eight independently-green PR slices (proposal order) closing G1–G9 + S1/C1. No new
abstraction layers beyond two small packages (`internal/session`, `internal/audit`)
and one config loader (`internal/projectconfig`). Every guardrail reuses an existing
choke point: `WireMCPTools` for registration-time wrapping, `cliRun` for the shared
exec path, `gitops.Commit` (extended to verify the staged set post-`git add`) for
commits, `state.go`'s atomic-write/var-seam pattern for new persistence.

## Architecture Decisions

| Decision | Choice | Rejected | Rationale |
|---|---|---|---|
| D2 timeout plumbing | New ctx-aware siblings in `internal/exec` (`RunCtx`, `RunWithEnvCtx`, `RunWithEnvInputCtx`, `exec.CommandContext`-style SIGTERM via existing process-group tracking); `cliRun` keeps its exact signature and calls the Ctx sibling internally with a resolved deadline; direct `drupexec.RunWithEnv` call sites in composer/drush/upgrade_scan handlers switch to the Ctx sibling | Add `context.Context` to `ToolHandler` | Threading ctx through the type touches 29 real + 25 stub handlers (54 functions) for a fix that only needs the exec layer to stop hanging. `ToolHandler` and `handleToolCall` stay untouched. |
| Deadlines | Package-level `defaultExecTimeout` (5 min) + `execTimeoutOverride map[string]time.Duration` in `internal/app` keyed by tool name (composer_require, core_upgrade_apply, upgrade_scan longer) | Per-call caller-supplied timeout param | Matches proposal's explicit lower-churn recommendation; table lives next to the handlers that need it, not in exec. |
| Token transport | Server-side session bound to process lifetime; no TTL (decision 1 confirmed) | Tool-arg token | stdio = single client; schema-invisible, zero churn on 12 mutating schemas. |
| Root validation | New `session.ResolveSymlinks` (abs + no `..` + `filepath.EvalSymlinks`, no marker precondition) is the single shared helper called from all three sites `specs/agent-session`'s "Canonical Root Resolution" requirement names: `coreupgrade.ValidateProjectPath` (rewritten to call it, now returns `(string, error)`), `backup.validateProject` (calls it after its existing `os.Stat` dir check, before returning), and `envdetect.Detect` (calls it after its existing existence/`IsDir` check, before the cache lookup and marker scan). Each site keeps its own existing marker/stat checks and graceful early-return behavior untouched — `envdetect.Detect` still returns `EnvUnsupported` for non-Drupal dirs (a path `core_upgrade_check` itself depends on) and `EnvUnknown` for a nonexistent path, `backup.validateProject` still returns a plain error on a missing/non-dir path. `session.CanonicalRoot` calls the same `ResolveSymlinks` and adds the marker check (composer.json or `<webroot>/core`) on top, strictly for session binding. `core_upgrade_check`'s inline IsAbs/`..` check (mcp_tools.go:1639-1644) is replaced with a call to `coreupgrade.ValidateProjectPath` so CLI and MCP share identical symlink resolution | Full replacement of all 3 existing validators with `CanonicalRoot` | `specs/agent-session`'s "Canonical Root Resolution" requirement literally names `ValidateProjectPath`, `backup.validateProject`, and `envdetect` as call sites that must resolve through the shared helper. `EvalSymlinks` on an already-existing, already-absolute path has no composer.json precondition, so extracting it into `ResolveSymlinks` satisfies that requirement at all three sites without forcing the marker check onto backup/envdetect, whose pre-composer.json tolerance (`envdetect.Detect`'s graceful `EnvUnsupported`/`EnvUnknown`) is a real blocker for full unification into `CanonicalRoot` itself (see Open Questions) |
| Guard placement | Registration-time wrapper in `WireMCPTools` for every dispatch-table tool call, **plus** an inline `session.RequireInstallAllowed` call inside `realHandleUpgradeScan` immediately before its nested `realHandleComposerRequire` call (mcp_tools.go:817-822) | Middleware-only | The nested call bypasses `s.tools` entirely (direct Go call), so wrapping the registered `composer_require` handler alone cannot see it. Guard *logic* stays centralized in `internal/session`; only the call site is duplicated. |
| G9 depth | `validate` gains optional `expected_hash`; mismatch fails closed with a re-scan instruction (Q7 resolved) | Prompt-side only | Testable, mechanical, matches recommended default. |
| Q6 (S5 cosign) | Deferred — separate change | In scope | Confirmed out of scope per proposal. |

## Guarded Tool Set (complete)

Force-dry-run (native `dry_run`): `core_upgrade_apply`, `contrib_compat_patch`, `contrib_allow_lenient`, `custom_compat_fix`.
Refuse-only: `apply_patch`, `composer_require`, `patch_rollback`, `cleanup`, `create_patch`, `test_backup_restore`, `test_backup_delete`.
Refuse-nested-only: `upgrade_scan` (guarded solely at its internal install path, never dry-run — it has no such param).
Kill switch (`DRUP_DISABLE_MUTATIONS=1` / `--locked`) collapses to the same partition as "no session." `DRUP_ALLOW_UNSAFE=1` bypasses all of the above with a one-time stderr warning.

## Data Flow

    tools/call ──▶ guard(name,args) ──▶ kill-switch? ──▶ session bound to root?
                       │no                   │no                  │yes
                       ▼                     ▼                    ▼
                   refuse/force-dry     refuse/force-dry     backup-fresh? ──▶ cap ok? ──▶ real handler
                                                                   │no            │no
                                                                   ▼              ▼
                                                               refuse         refuse
    every branch ──▶ internal/audit.Append(entry) ──▶ JSONL ledger

`upgrade_scan`'s nested install path re-enters this flow via `session.RequireInstallAllowed`, not through `s.tools`.

## File Changes

| File | Action | Notes |
|---|---|---|
| `internal/patch/patch.go` | Modify | PR1: `url.Parse` + exact host / suffix match, https-only |
| `internal/mcp/server.go` | Modify | PR2: scanner buffer, sorted `tools/list`, `req.ID==nil` skip |
| `internal/exec/exec.go` | Modify | PR2: add `*Ctx` siblings |
| `internal/app/cleanup.go` | Modify | PR2 (io.Writer) + PR7 (gitops.Commit instead of `add -A`) |
| `internal/app/mcp_tools.go` | Modify | PR3 drush aliases/blocklist; PR4 `core_upgrade_check` drops its inline IsAbs/`..` check for `coreupgrade.ValidateProjectPath`, guard wiring; PR6 pipeline_status |
| `internal/session/session.go` | Create | PR4: token, `ValidateAbsolutePath`, `ResolveSymlinks`, `CanonicalRoot`, guard funcs |
| `internal/coreupgrade/apply.go` | Modify | PR4: `ValidateProjectPath` delegates to `session.ResolveSymlinks`, returns `(string, error)`; `Apply` uses the resolved path |
| `internal/coreupgrade/rollback.go` | Modify | PR4: `Rollback` adapts to `ValidateProjectPath`'s new return signature |
| `internal/backup/backup.go` | Modify | PR4: `validateProject` calls `session.ResolveSymlinks` right after its existing `os.Stat`/`IsDir` check (path existence already confirmed) and before returning the resolved path — no marker check added, so the plain "invalid project" error on a missing/non-dir path is unaffected |
| `internal/envdetect/envdetect.go` | Modify | PR4: `Detect` calls `session.ResolveSymlinks` right after its existing `os.Stat`/`IsDir` existence check and before the cache lookup / `detect()` marker scan — sits before both the `EnvUnknown` (nonexistent path) and `EnvUnsupported` (no marker found) early returns, so both stay graceful |
| `internal/app/commands.go` | Modify | PR4/5: `session_open` wiring, `--locked` parse in `RunMCP`, `upgrade-core` CLI adapts to `ValidateProjectPath`'s new return signature |
| `internal/audit/audit.go` | Create | PR6: JSONL append, caps check |
| `internal/projectconfig/config.go` | Create | PR4/6: `.drup/config.json` loader, safe defaults |
| `internal/gitops/gitops.go` | Modify | PR7: `Commit` runs `git diff --cached --name-only` after `git add --`, verifies the staged set is a subset of the declared `files`; on an unexpected staged path it `git reset`s and returns an error naming the offending path(s) instead of committing |
| `internal/scan/model.go`, `validate` handler | Modify | PR7: `EvidenceHash`, `expected_hash` |
| `internal/packaging/templates/*/mcp.json` (×3) | Modify | PR5: `--locked` arg |
| `README.md`, `docs/mcp-tools.md`, `internal/update/upgrade.go` | Modify | PR8 |

## Threat Matrix

| Boundary | Applicability | Response |
|---|---|---|
| Git repo selection | Applicable | `-C projectPath` already used everywhere; `session.CanonicalRoot` fixes the root before any git/drush call in the guarded set |
| Commit state | Applicable | `gitops.Commit` scoped-file staging replaces `add -A` in cleanup.go; `Commit` itself now verifies staged files ⊆ declared list post-`git add` and aborts (`git reset` + error) otherwise, per `specs/gitops` Scoped Commit Helper |
| Documentation-like paths | N/A — no script/doc execution boundary changes |
| Push/PR commands | N/A — no VCS-host automation in scope |

## Testing Strategy

| Layer | Focus |
|---|---|
| Unit | Allowlist bypass shapes (S1), alias normalization table, `CanonicalRoot`/`ResolveSymlinks` symlink/marker cases, `execTimeoutOverride` resolution, evidence-hash mismatch, `gitops.Commit` unexpected-staged-file abort |
| Integration | Guard partition (force-dry vs refuse) per tool via `s.tools` map, nested upgrade_scan install path, ledger cap enforcement, CLI vs MCP symlinked-path resolution parity for `core_upgrade_check`/`core_upgrade_apply` |
| Symmetry/count | Existing `TestServer_WiringSymmetry*` + count-locking updated per slice (25→27 default after PR4/PR6, 29→31 post-wire) |

## Migration / Rollout

Per proposal: safe degradation (dry-run/refuse), `DRUP_ALLOW_UNSAFE=1` escape hatch, SKILL.md ×3 platforms updated same-release as PR4.

## Open Questions

- [ ] Backup-freshness window default (24h) and which subset of the mutating set it gates — confirm during PR6 tasks, not blocking earlier slices.
- [ ] `backup.validateProject` and `envdetect.Detect` now share `session.ResolveSymlinks` (symlink resolution only, no marker precondition) per `specs/agent-session`'s literal requirement, but neither is folded into the marker-checking `session.CanonicalRoot` itself: `envdetect.Detect` must still tolerate a directory with no composer.json yet (graceful `EnvUnsupported`, a path `core_upgrade_check` relies on) and `backup.validateProject`'s callers, while already expecting an existing Drupal project, are out of scope for folding into `CanonicalRoot` in this corrective pass — flagging for a spec amendment or explicit confirmation before tasks, rather than silently deviating.
- [ ] `envdetect.Detect`'s result cache (envdetect.go:87,96) is keyed on the raw caller-supplied `projectPath`, and `ResolveSymlinks` now runs before that cache lookup but its resolved path is not itself used as the cache key. Tasks must decide: key the cache on the resolved path (a symlink and its real target then share one cache entry — mtime-invalidation semantics stay correct either way since both point at the same underlying directory) or leave the key on the raw path (current behavior; simpler, but a symlinked and non-symlinked call for the same real project never hit the same cache entry). No functional bug either way — purely a cache-identity/efficiency choice.
