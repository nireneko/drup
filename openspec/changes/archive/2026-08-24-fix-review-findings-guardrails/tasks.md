# Tasks: fix-review-findings-guardrails

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~2,640 total (proposal estimate, confirmed by task density) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 → PR2 → PR3 → PR4 → PR5 → PR6 → PR7 → PR8 |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Patch URL allowlist (S1/C1) | PR1 | `go test ./internal/patch/...` | N/A — pure unit, no live network | `internal/patch/patch.go` revert |
| 2 | Transport hardening (D1/D2/M3/M6/M7) | PR2 | `go test ./internal/mcp/... ./internal/exec/... ./internal/app/... -run Cleanup\|Scanner\|Notif` | `go run ./cmd/drup mcp` + manual `tools/list` twice | mcp/exec/cleanup files revert |
| 3 | Drush hardening (S3/D3/S6/M2) | PR3 | `go test ./internal/app/... -run DrushExec` | N/A — subprocess mocked via var seam | `mcp_tools.go` drush section revert |
| 4 | Session + root pinning (G1/S4/D4, keystone) | PR4 | `go test ./internal/session/... ./internal/coreupgrade/... ./internal/backup/... ./internal/envdetect/...` | `session_open` then `core_upgrade_check` via MCP stdio | delete `internal/session`, unwire guard, restore prior `ValidateProjectPath` |
| 5 | Kill switch + dry-run partition (G4/G5/S2) | PR5 | `go test ./internal/app/... ./internal/packaging/... -run Locked\|KillSwitch` | `drup mcp --locked` refuses a mutating call | flag/env checks revert independently of PR4 |
| 6 | Backup gate + audit ledger (G2/G3) | PR6 | `go test ./internal/audit/... ./internal/session/... -run Backup\|Cap` | ledger file inspected after a live tool call | delete `internal/audit`, unwire cap/backup checks |
| 7 | Scoped commits + evidence hash (G8/G9) | PR7 | `go test ./internal/gitops/... ./internal/scan/... ./internal/app/... -run Cleanup\|Hash` | `drup` cleanup stage against a repo with an unrelated dirty file | `gitops.Commit`/`EvidenceHash` revert, independent of PR4-6 |
| 8 | Docs + drift fixes (M1/M4/M5) | PR8 | `go test ./internal/update/...` | N/A — docs/comments only | file revert only |

## Phase 1: PR1 — Patch Allowlist

- [x] 1.1 `internal/patch/patch.go`: replace `strings.Contains` host check with `url.Parse` + exact-host match or `*.drupal.org` suffix rule; reject non-`https`; reject on parse failure; leave local-path branch unchanged.
- [x] 1.2 `internal/patch/patch_test.go`: RED→GREEN table tests for accept (drupal.org), reject host-as-path (`evil.com/www.drupal.org/...`), reject host-as-subdomain-of-attacker (`drupal.org.evil.com`), reject domain-in-query (`notdrupal.org/?x=git.drupal.org`), reject `http://`, local path unaffected.

## Phase 2: PR2 — Transport Hardening

- [x] 2.1 `internal/mcp/server.go`: set explicit bounded scanner buffer above 64KB; oversized line yields a JSON-RPC parse error without stopping the read loop; skip writing a response when `req.ID==nil` (notifications); sort `tools/list` by name.
- [x] 2.2 `internal/exec/exec.go`: add `RunCtx`, `RunWithEnvCtx`, `RunWithEnvInputCtx` ctx-aware siblings using `exec.CommandContext`-style SIGTERM via existing process-group tracking; `cliRun` keeps its signature, calls the Ctx sibling internally.
- [x] 2.3 `internal/app/mcp_tools.go`: add `defaultExecTimeout` (5m) + `execTimeoutOverride map[string]time.Duration` (composer_require, core_upgrade_apply, upgrade_scan longer); switch composer/drush/upgrade_scan call sites from `RunWithEnv` to the Ctx sibling with resolved deadline.
- [x] 2.4 `internal/app/cleanup.go`: `RunCleanup` accepts an `io.Writer` param instead of `fmt.Println`/`os.Stdout`; update call sites.
- [x] 2.5 `internal/app/mcp_tools.go`: `realHandleCleanup` passes the injected writer instead of swapping global `os.Stdout`.
- [x] 2.6 `internal/mcp/mcp_test.go` (or `server_test.go`): oversized line doesn't kill the server; `notifications/initialized` produces no response, ordinary requests still get one; two consecutive `tools/list` calls match and are sorted.
- [x] 2.7 `internal/exec/exec_test.go`: `*Ctx` sibling terminates a subprocess past its deadline and returns a timeout error; completes normally within deadline.
- [x] 2.8 `internal/app/cleanup_test.go`: writer captures large output with no `os.Stdout` swap/deadlock risk.

## Phase 3: PR3 — Drush Hardening

- [x] 3.1 `internal/app/mcp_tools.go`: add alias→canonical map (`sqlq`/`sql-cli`/`sqlc`→`sql:query`, `scr`→`php:script`, `ev`→`php-eval`, `exec`→`core:execute-cli`, `core:execute`, etc.); normalize (trim, lowercase, resolve alias) before blocklist eval; extend blocklist to the full canonical set from `specs/mcp-server`.
- [x] 3.2 `internal/app/mcp_tools.go`: extend metachar filter to reject `;`, `|`, `&`, `$`, backtick, and newline in command/args.
- [x] 3.3 `internal/app/mcp_tools.go:719` (`drushExecError`): insert a newline separator when appending a supplementary warning to existing stderr.
- [x] 3.4 `internal/app/mcp_tools_test.go`: blocked canonical (`sql-drop`), blocked via alias (`sqlq`, `scr`, `ev`, `exec`), metachar rejection incl. newline/backtick, stderr warning separator.

## Phase 4: PR4 — Session + Root Pinning (keystone)

- [x] 4.1 Create `internal/session/session.go`: process-lifetime token/bind-replace, `ResolveSymlinks` (abs + no `..` + `EvalSymlinks`, no marker precondition), `CanonicalRoot` (calls `ResolveSymlinks` + composer.json/webroot marker check), `RequireInstallAllowed`, guard partition funcs (force-dry-run vs refuse vs kill-switch, per `specs/agent-session` + `specs/mcp-server`).
- [x] 4.2 `internal/coreupgrade/apply.go`: `ValidateProjectPath` delegates to `session.ResolveSymlinks`, returns `(string, error)`; `Apply` uses the resolved path.
- [x] 4.3 `internal/coreupgrade/rollback.go`: `Rollback` adapts to the new `(string, error)` signature.
- [x] 4.4 `internal/backup/backup.go`: `validateProject` calls `session.ResolveSymlinks` after its existing `os.Stat`/`IsDir` check, before returning; no marker check added; missing/non-dir error unchanged.
- [x] 4.5 `internal/envdetect/envdetect.go`: `Detect` calls `session.ResolveSymlinks` after the existing existence/`IsDir` check, before the cache lookup and `detect()` marker scan; **decision**: key the result cache on the resolved (symlink-following) path — a symlink and its real target then share one cache entry with correct mtime-invalidation semantics either way; document the choice in a code comment per design Open Question 2. `EnvUnsupported`/`EnvUnknown` early returns stay graceful.
- [x] 4.6 `internal/app/mcp_tools.go`: `core_upgrade_check` drops its inline IsAbs/`..` check (mcp_tools.go:1639-1644), calls `coreupgrade.ValidateProjectPath` instead.
- [x] 4.7 `internal/app/mcp_tools.go`: register `session_open` via the 3-file wiring pattern (schema/placeholder/real handler); wire guard middleware into `WireMCPTools` for the full guarded set (`apply_patch`, `core_upgrade_apply`, `composer_require`, `create_patch`, `cleanup`, `patch_rollback`, `custom_compat_fix`, `contrib_compat_patch`, `contrib_allow_lenient`, `test_backup_restore`, `test_backup_delete`); add inline `session.RequireInstallAllowed` in `realHandleUpgradeScan` before its nested `realHandleComposerRequire` call (mcp_tools.go:817-822).
- [x] 4.8 `internal/app/commands.go`: wire `session_open` tool handler; adapt `upgrade-core` CLI command to `ValidateProjectPath`'s new return signature.
- [x] 4.9 Create `internal/projectconfig/config.go`: `.drup/config.json` loader (caps, timeouts, allowlist-mode) with safe defaults when absent.
- [x] 4.10 `internal/session/session_test.go`: symlinked path resolves to real target; `..`/non-absolute rejected before any operation; reopening replaces prior session; guard partition per tool; nested `upgrade_scan` install path guarded.
- [x] 4.11 `internal/coreupgrade/apply_test.go`, `rollback_test.go`, `internal/backup/backup_test.go`, `internal/envdetect/envdetect_test.go`: update for the new `ValidateProjectPath` signature and shared `ResolveSymlinks` call; assert `EnvUnsupported`/`EnvUnknown` still returned gracefully.
- [x] 4.12 `internal/mcp/mcp_test.go` + `internal/app/mcp_tools_test.go`: update wiring-symmetry/count-locking (25→26 for `session_open`; `pipeline_status` lands in PR6) and assert full guarded-set coverage incl. `upgrade_scan`.
- [x] 4.13 Update `internal/packaging/templates/{claude,opencode,codex}/SKILL.md`: document the `session_open` flow, same release per migration story.

## Phase 5: PR5 — Kill Switch + Dry-Run Partition

- [x] 5.1 `internal/app/commands.go`: parse `--locked` in `RunMCP`, equivalent to `DRUP_DISABLE_MUTATIONS=1`.
- [x] 5.2 `internal/session` guard funcs: kill switch (`DRUP_DISABLE_MUTATIONS=1` or `--locked`) refuses every mutating call regardless of session, checked before session/backup; `DRUP_ALLOW_UNSAFE=1` bypasses all guards with a one-time stderr warning per bypassed call.
- [x] 5.3 `internal/app/mcp_tools.go`: remove `allow_dirty` from the `core_upgrade_apply` MCP schema; unguarded-session path now forces native `dry_run: true` instead of accepting an override; CLI `--allow-dirty` untouched.
- [x] 5.4 `internal/packaging/templates/{claude,opencode,codex}/mcp.json`: render `--locked` when locked mode is selected; omit by default; parity across all three.
- [x] 5.5 `internal/session/session_test.go` (or `mcp_tools_test.go`): kill switch refuses with a valid open session; force-dry-run set (`core_upgrade_apply`, `contrib_compat_patch`, `contrib_allow_lenient`, `custom_compat_fix`) vs refuse-only set (`apply_patch`, `composer_require`, `patch_rollback`, `cleanup`, `create_patch`, `test_backup_restore`, `test_backup_delete`); `DRUP_ALLOW_UNSAFE` bypass + warning.
- [x] 5.6 `internal/app/mcp_tools_test.go`: `tools/list` schema for `core_upgrade_apply` has no `allow_dirty`; `internal/app/commands_test.go`: CLI `--allow-dirty` behavior unchanged.
- [x] 5.7 `internal/packaging/packaging_test.go`: `--locked` present only when selected, parity across the 3 templates.
- [x] 5.8 (gap closure) `internal/app/commands.go`: `packaging.RenderLocked` had zero production call sites — `installAgents` always called `packaging.Render`, and `RunInstall`/`RunSync` took no args, so no real command could reach locked mode. Added `installLockedRequested(args []string) bool` (mirrors `mcpLockedRequested`); threaded `locked bool` through `installAgents` to call `RenderLocked`; `RunInstall(args []string)`/`RunSync(args []string)` now parse `--locked`; `app.go` dispatch passes `args[1:]` through for `install`/`sync`; usage help documents `--locked` for both. Tests: `internal/app/commands_test.go` — `TestInstallLockedRequested` (flag parsing), `TestInstallAgents_LockedRendersLockedArgIntoMcpConfig` (locked threads into rendered mcp.json vs. default-off regression), `TestRunInstall_LockedFlagRendersLockedArgForAllPlatforms`, `TestRunInstall_WithoutLockedFlagOmitsLockedArg`, `TestRunSync_LockedFlagRendersLockedArgForAllPlatforms` (end-to-end through the real CLI entry point across claude/opencode/codex).

## Phase 6: PR6 — Backup Gate + Audit Ledger

- [x] 6.1 Create `internal/audit/audit.go`: JSONL `Append` via atomic tmp+rename; record = tool name, SHA256 of raw args, result, commit hash (when applicable), timestamp; write failure logged, never blocks the tool response.
- [x] 6.2 `internal/audit/audit.go`: mutation cap check (per-session default; per-day when no session per opt-out); safe built-in default cap when `.drup/config.json` has no cap configured.
- [x] 6.3 `internal/session` guard flow: backup-freshness gate — **decision**: default freshness window 24h, gating the full guarded mutating set (same set as Guard Middleware Enforcement, including the `upgrade_scan` nested path) per design Open Question 1; manifest newer than session-open time or within the window passes; stale/absent blocks naming `test_backup_create`.
- [x] 6.4 `internal/app/mcp_tools.go`: implement `pipeline_status` real handler (3-file wiring already scaffolded PR4) — per-tool counts, total mutations, remaining cap; zero counts + full cap on empty ledger, no error.
- [x] 6.5 Wire `internal/audit.Append` into every guard branch (kill-switch/session/backup/cap outcome) and real-handler completion per design Data Flow.
- [x] 6.6 `internal/audit/audit_test.go`: success/failure record shape, write failure doesn't block the tool response, cap reached refuses with count, default cap applied when unconfigured.
- [x] 6.7 `internal/session/session_test.go`: fresh backup allows; stale/absent backup blocks naming `test_backup_create`.
- [x] 6.8 `internal/app/mcp_tools_test.go`: `pipeline_status` with prior mutations and on empty ledger; update count-locking (26→27 for `pipeline_status`).

## Phase 7: PR7 — Scoped Commits + Evidence Hash

- [x] 7.1 `internal/gitops/gitops.go`: `Commit` runs `git diff --cached --name-only` after `git add --`, verifies staged set ⊆ declared `files`; unexpected staged path triggers `git reset` + error naming the offending path(s); empty declared list reports "nothing to commit" without committing.
- [x] 7.2 `internal/app/cleanup.go`: replace `git add -A` with the scoped `gitops.Commit` call over declared paths (composer.json, composer.lock, drush-modified config).
- [x] 7.3 `internal/scan/model.go`: add `EvidenceHash` — SHA256 over normalized/serialized findings; deterministic for identical findings, differs on any error-entry diff, valid non-empty hash for zero-error scans.
- [x] 7.4 `internal/app/mcp_tools.go`: `validate` handler accepts optional `expected_hash`; mismatch fails closed regardless of `total_errors`, error includes both hashes; omitted preserves prior `total_errors`-only gating.
- [x] 7.5 `internal/gitops/gitops_test.go`: declared-subset commit succeeds; unexpected staged file aborts + reports path; empty list no-ops.
- [x] 7.6 `internal/app/cleanup_test.go`: unrelated uncommitted file outside declared paths excluded from the commit.
- [x] 7.7 `internal/scan/scan_test.go`: `evidence_hash` determinism, divergence on differing findings, empty-findings case.
- [x] 7.8 `internal/app/mcp_tools_test.go`: `validate` matching / mismatched / omitted `expected_hash` scenarios.

## Phase 8: PR8 — Docs + Drift Fixes

- [x] 8.1 `README.md`: fix tool-count drift ("26 tools"/"25 tools" → actual `WireMCPTools` count post PR1-7) and test-count drift ("498 passing" vs "163+").
- [x] 8.2 `docs/mcp-tools.md`: sync tool count/list with final registrations incl. `session_open`/`pipeline_status`.
- [x] 8.3 `internal/update/upgrade.go:153-156`: fix stale docblock claiming `TypeRegA` is accepted; code only accepts `tar.TypeReg`.
- [x] 8.4 `internal/update/upgrade.go:241-253` (`copyFile`): remove the redundant explicit `Close` call, keep the single deferred `Close` (or check-and-return its error) to fix the double-Close.
- [x] 8.5 `internal/update/upgrade_test.go`: `copyFile` closes the destination file exactly once (regression guard for M5).
