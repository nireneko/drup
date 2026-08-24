# Exploration: fix-review-findings-guardrails

Date: 2026-08-21 · Source: PROJECT-REVIEW.md (full review, 2026-08-21) · Mode: read-only investigation
Verification performed: every finding re-checked against current code; S1 bypass reproduced empirically; `go build ./...` and `go test ./...` (20 pkgs) confirmed green.

---

## 1. Verified findings

All 13 minor/medium issues and all security findings were located and confirmed. Line numbers had **no drift** — the review matches current code exactly.

| Finding | Verified location | Verdict | Notes |
|---|---|---|---|
| S1/C1 allowlist substring bypass | `internal/patch/patch.go:35-42` (`defaultCheckAllowedURL`, `strings.Contains(url, domain)`) | **CONFIRMED** | Empirically reproduced: `https://evil.com/www.drupal.org/evil.patch`, `https://drupal.org.evil.com/evil.patch`, `https://notdrupal.org/?x=git.drupal.org` all pass. No regression tests exist for the allowlist shapes. Local-path branch (`localPatchPath`, patch.go:47-66) is sound. |
| S2 `allow_dirty` exposed via MCP | `internal/coreupgrade/apply.go:93` (`if !allowDirty` skips clean check); MCP handler `internal/app/mcp_tools.go:1733-1762` (param at :1738, passed at :1750) | **CONFIRMED** | CLI also exposes it (`drup upgrade-core --allow-dirty`, app.go:141). Note apply.go's rationale comment: without it the tool is unusable after pipeline stages that leave changes by design — removal needs a story for that flow. |
| S3/D3 drush blocklist gaps | `internal/app/mcp_tools.go:32-39` (blocklist), exact-string match at `:659` | **CONFIRMED** | Blocks only: `sql-drop`, `site-install`, `site:install`, `sql-sanitize`, `php-eval`, `core:execute-cli`. Missing destructive equivalents/aliases: `sql:query`/`sqlq`, `php:script`/`scr`, `ev`, `core:execute`, `exec`, plus `sqlc`/`sql-cli` family. Shell-metachar filter `[;|&$`]` (:42) misses newlines/backticks-in-args edge cases (S6). |
| D4/S4 no project-root validation | `internal/app/mcp_tools.go` (all handlers) | **CONFIRMED** | Only exceptions found: `composer_require` checks composer.json exists (:569-571); `core_upgrade_check` checks IsAbs + `..` (:1639-1644); `upgrade_scan` checks `..` (:791-793). `coreupgrade.ValidateProjectPath` (apply.go:28-39) is a ready-made helper but only used by core-upgrade. `envdetect.Detect` requires abs path but accepts *any* directory. |
| D1 scanner 64 KB limit | `internal/mcp/server.go:332` (`bufio.NewScanner(in)`) | **CONFIRMED** | No `scanner.Buffer(...)` anywhere in repo (grep verified). Oversized line → `Run()` returns error → server exits mid-session. |
| D2 no timeouts/context | `internal/mcp/server.go:419`; `internal/exec/exec.go` (whole file) | **CONFIRMED** | `Run`/`RunWithEnv`/`RunWithEnvInput` have no ctx parameter; no `context.` import in either package. Server is single-threaded synchronous: one hung drush/composer blocks all subsequent requests, not just the caller's. |
| M1 README/docs count drift | `README.md:8` ("498 passing" badge), `:103` ("26 tools"), `:295` ("25 tools"), `:421` ("163+" tests); actual = 29 tools in `WireMCPTools` (counted) | **CONFIRMED + EXTENDED** | `docs/mcp-tools.md:3` says "26 tools", `:5` says "27 total" — third inconsistent figure. Tests assert 25 default / 29 post-wire (`internal/mcp/mcp_test.go:161,430`). |
| M2 missing separator | `internal/app/mcp_tools.go:719` | **CONFIRMED** | `stderr = stderr + "warning: ..."` glues into previous line. |
| M3 nondeterministic tools/list | `internal/mcp/server.go:366` | **CONFIRMED** | Iterates `s.tools` Go map directly. |
| M4 stale TypeRegA comment | `internal/update/upgrade.go:153-156` (docblock) vs `:178` (code: only `tar.TypeReg`) | **CONFIRMED** | Behavior correct; comment stale. |
| M5 double Close in copyFile | `internal/update/upgrade.go:241` (deferred) + `:253` (explicit) | **CONFIRMED** | Harmless today; masks future write-error handling. |
| M6 notifications answered with error | `internal/mcp/server.go:350-362` | **CONFIRMED** | No `req.ID == nil` check; `notifications/initialized` falls to default → `-32601` response written to a notification (spec violation; can confuse strict clients). |
| M7 os.Stdout swap in cleanup handler | `internal/app/mcp_tools.go:1806-1816` | **CONFIRMED** | Root cause: `RunCleanup` prints via `fmt.Println` to global stdout (`internal/app/cleanup.go:36,48,91`). Handler swaps `os.Stdout` with an `os.Pipe` and reads only AFTER `RunCleanup` returns → deadlock if output > ~64 KB pipe buffer; global swap is not concurrency-safe. Fix requires giving `RunCleanup` an `io.Writer`. |

**Extra finding discovered during exploration** (not in review): `internal/app/cleanup.go:71` uses `git add -A` before its commit — exactly the pattern patch.Apply eliminated (patch.go:140-159 stages only declared paths). G8 must include cleanup.go or the scope-check guarantee has a hole.

Baseline health re-verified: `go build ./...` OK, `go test ./...` 20/20 packages OK.

---

## 2. Blast-radius map per guardrail

### G1 — Project-root pinning + session binding (highest value)
- **New package** `internal/session`: token generation/validation, pinned canonical root, expiry. No existing session concept anywhere (grep: no DRUP_* env vars, no token code).
- **Key gap**: there is **no preflight MCP tool** — preflight is CLI-only (`app.go:74-75`, `commands.go:894`). G1 says "at preflight", so the design must add either (a) a `session_open` MCP tool that runs root resolution + issues the token, or (b) server-side session state bound to process lifetime.
- **Root resolution**: reuse `composerutil.ReadWebRoot` + marker checks (composer.json, `<webroot>/core`) — same markers envdetect already stats; `filepath.EvalSymlinks` comparison is new.
- **Enforcement choke point**: `WireMCPTools` (`mcp_tools.go:51-82`) is the single registration site → wrap mutating handlers in a guard middleware there. Mutating set per review: `apply_patch`, `core_upgrade_apply`, `composer_require`, `create_patch`, `cleanup`, `patch_rollback`, `custom_compat_fix`, `contrib_compat_patch`, `contrib_allow_lenient`. **Exploration additions**: `test_backup_restore` (replaces whole tree, backup.go:520-543) and `test_backup_delete` are destructive too; `upgrade_scan` nests a real mutation (installs upgrade_status via `realHandleComposerRequire`, mcp_tools.go:817-822) inside a nominally read-only tool — middleware on `composer_require` alone does NOT cover this path.
- **Token transport decision**: tool-arg param (schema change on every mutating tool, visible to clients) vs server-side state (invisible; stdio server is single-client so binding is trivial). Server-side strongly preferred for compat.
- **Templates**: orchestrator SKILL.md ×3 platforms (claude/opencode/codex) must document the session flow; `TestSKILLMD_CrossPlatformIdentical` (packaging_test.go:115) forces identical SKILL.md across platforms.

### G2 — Backup-before-mutate in code
- Reuse `backup.Manager.List()` (backup.go:136-166): already returns manifests sorted `CreatedAt desc` — newest manifest timestamp IS the freshness signal. Manifests live in `<project>/.drup/backups/*/manifest.json`.
- Gate lives in the same G1 middleware; needs session-start time (G1 dependency) or "newer than N hours" fallback.
- `test_backup_create` MCP tool already exists (mcp_tools.go:96-106).

### G3 — Mutation ledger + caps
- **New package** `internal/audit`: JSONL append under `~/.config/drup/audit/<project>.jsonl` (state.go's atomic-write pattern reusable; configDir var seam at state.go:37 overridable in tests).
- Entry fields map to existing data: tool name, args hash (sha256 of RawMessage), result, commit hash (patch.ApplyResult.CommitHash, coreupgrade RollbackCheckpoint already returned).
- Caps enforcement in middleware; reset mechanism = human edits file or new `pipeline_status` arg.
- `pipeline_status` exposure: extend `generate_report` or new tool (registry + stub + doc mirror convention applies — see §3 wiring-symmetry tests).
- `internal/metrics` Collector (singleton, recover-guarded) is the in-process analog; ledger adds cross-run persistence.

### G4 — Kill switch
- `DRUP_DISABLE_MUTATIONS=1` checked in the same middleware. First DRUP_* env var in the codebase.
- `--locked` flag: `RunMCP` (commands.go:407-411) parses nothing today; add flag pass-through from `app.Run` case "mcp".
- Installer templates: `mcp.json` template is minimal (`{"command":"{{BINARY_PATH}}","args":["mcp"]}`); adding `"--locked"` touches packaging templates + installer render tests for 3 platforms.

### G5 — Dry-run-by-default outside session
- Same middleware as G1/G4 (this is why G1 should land first). Tools with native dry_run params: `core_upgrade_apply`, `contrib_compat_patch`, `contrib_allow_lenient`, `custom_compat_fix` — middleware forces the param true when no valid session. Tools WITHOUT a dry-run mode (`apply_patch`, `composer_require`, `patch_rollback`, `cleanup`, `create_patch`) need a refusal path instead — "dry-run" is not meaningful for them. Design must split the mutating set into force-dry-run vs refuse-only.

### G6 — Transport hardening (D1+D2+M3+M6+M7)
- `server.go`: `scanner.Buffer(make([]byte, 0, 64KB), 10MB)`; sorted tools/list (sort names before iterate); skip response when `req.ID == nil`; per-call `context.WithTimeout`.
- Timeout plumbing: handlers are `func(json.RawMessage) (json.RawMessage, error)` — changing the signature to accept ctx touches all 29 real handlers + 25 stubs. Lower-churn alternative: keep signature, add `RunWithContext` variants in exec.go and give long-running handlers a deadline via a package-level default + per-tool override table. Decision needed in design phase.
- `cleanup.go`: change `RunCleanup(args []string)` to accept/return an `io.Writer` (or return structured data instead of printing JSON) — kills the os.Stdout swap; update CLI call site (app.go:108-112) and cleanup_test.go.

### G7 — Drush canonical normalization (+ optional allowlist)
- `mcp_tools.go` only: alias→canonical table, trim/lowercase, blocklist (or allowlist) on canonical name. Internal drush calls (`upgrade_scan` runs `pm:list`, `config:delete`, `en`, `cr`, `analyze` via RunWithEnv directly) bypass drush_exec entirely, so tightening drush_exec cannot break internal flows.
- Allowlist mode needs a curated command list + escape hatch (env/config) — open question below.

### G8 — Scope-checked commits
- Extend gitops (add `CommitScoped(path, msg, allowedFiles)` verifying `git diff --cached --name-only ⊆ allowed`) or a shared helper.
- Call sites: `cleanup.go:71` (`git add -A` — replace with scoped paths), autofix/create_patch flows (rector writes files then handlers commit? — create_patch does NOT commit; autofix does not commit either; commits happen in patch.Apply [already scoped] and cleanup [not scoped]). Actual blast radius smaller than review implies: main offender is cleanup.go.

### G9 — Validator evidence integrity
- `scan` package: add SHA256 of normalized findings to ScanResult serialization; `validate` output includes `evidence_hash`.
- Enforcement is inherently prompt-side (orchestrator templates require the hash in gate decisions); optional mechanical part: `validate` accepts `expected_hash` and fails closed on mismatch.
- Templates ×3 platforms + validator agent docs.

---

## 3. Reusable patterns (do not invent new ones)

1. **Package-level var test seams** — uniform across repo: `drupexec.Run`/`RunWithEnv` (exec.go:113,125), `runCommand` (gitops.go:12, patch.go:22), `httpClient` (patch.go:19), `checkAllowedURL` (patch.go:33), `defaultEnvDetector` (mcp_tools.go:29), `configDir` (state.go:37), `detectEnv`/`run` (backup.go:48-50). New packages (session, audit) MUST follow this seam style.
2. **Table-driven tests + testdata fixtures** — e.g. scan/checkstyle_test, drupalorg XML fixtures.
3. **Declarative schema registry + wiring-symmetry invariant tests** — `toolRegistry` (server.go:64-310); `TestServer_WiringSymmetryEveryDefaultToolHasSchema`, `TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric` (mcp_test.go:304-346) mechanically enforce: new tool ⇒ stub in defaultTools() OR `test_backup_` prefix, schema with non-empty Properties, Required incl. project_path. Any new tool (session_open, pipeline_status) must satisfy these or extend the documented exception rules.
4. **Single registration choke point** — `WireMCPTools` is where guard middleware wraps handlers; no per-handler scattering.
5. **Atomic state persistence** — tmp-file + rename (state.go:103-125); legacy-key tolerance pattern (state.go:64-75) if state.json gains fields.
6. **Count-locking tests** — mcp_test.go locks 25/29 tool counts; any tool addition updates those assertions deliberately.
7. **Template parity enforcement** — `TestSKILLMD_CrossPlatformIdentical` + per-platform render tests; SKILL.md changes go to all 3 platform dirs in one commit.
8. **Existing validation helpers to generalize** — `coreupgrade.ValidateProjectPath` (abs + no `..`), backup.validateProject (abs + clean + stat dir), envdetect abs-path requirement. G1's canonical-root check should unify these into one helper rather than adding a fourth variant.

---

## 4. Proposed PR slices (800-line review budget, auto-chain)

Ordering principle: security fixes first and independently shippable; transport hardening before session infra (timeouts used by later slices); everything session-dependent clusters after G1.

| # | Slice | Contents | Files | Est. lines (code+tests) |
|---|---|---|---|---|
| PR1 | Patch allowlist fix | S1/C1: parse URL, exact host match, https-only, subdomain suffix rule; regression tests for the 3 bypass shapes + legit URLs | patch.go, patch_test.go | ~120 |
| PR2 | Transport hardening | G6 = D1 scanner buffer, D2 timeouts (exec ctx variants + per-call deadline), M3 sorted tools/list, M6 ignore notifications, M7 cleanup io.Writer refactor (+M4/M5 minors in update/upgrade.go while touching nearby concepts? no — keep separate) | server.go, exec.go(+test), cleanup.go(+test), mcp_tools.go (handler call sites) | ~450 |
| PR3 | Drush hardening + small fixes | S3/D3 canonical alias normalization + extended blocklist (or allowlist mode), S6 metachar filter completion/doc, M2 separator | mcp_tools.go(+test) | ~250 |
| PR4 | Session + root pinning | G1+S4/D4: internal/session pkg, canonical-root resolver (unify ValidateProjectPath variants), WireMCPTools guard middleware, session_open tool (or equivalent), registry entries, symmetry-test updates | new internal/session, mcp_tools.go, commands.go, server.go (registry), mcp_test.go | ~600 |
| PR5 | Kill switch + dry-run default | G4+G5 on top of PR4 middleware: env var + --locked flag + installer template args; force-dry-run vs refuse partition of mutating tools | app.go, commands.go, mcp_tools.go, installer/packaging templates+tests | ~300 |
| PR6 | Backup gate + ledger/caps | G2+G3: freshness check in middleware, internal/audit pkg, caps config surface, pipeline_status tool | mcp_tools.go, new internal/audit, state.go (if state-based config), docs | ~450 |
| PR7 | Commit scoping + evidence hash | G8 (cleanup.go `add -A` → scoped; gitops helper) + G9 (scan evidence_hash, validate expected_hash, template gate wording ×3 platforms) | cleanup.go, gitops.go, scan/, mcp_tools.go, templates | ~350 |
| PR8 | Docs accuracy | M1: README counts (26→29, 25→29, badge/test counts), docs/mcp-tools.md totals; M4/M5 minors in update/upgrade.go | README.md, docs/mcp-tools.md, update/upgrade.go(+test) | ~120 |

Total ≈ 2,600 lines — chained PRs mandatory. PR1–PR3 have no interdependencies beyond ordering comfort; PR4 is the keystone (PR5–PR6 depend on its middleware; PR7 independent of PR4–PR6). Each slice keeps build+tests green independently.

---

## 5. Risks & open questions the proposal MUST answer

**Risks**
- CRITICAL: Session-token requirement (G1/G5) breaks every currently installed agent workflow and shipped skill templates on upgrade — existing users' pipelines hard-fail after binary update until they re-run install/sync AND their orchestrator learns the session flow. Needs explicit migration story (grace mode? env opt-out? version-gated behavior?).
- WARNING: `allow_dirty` removal (S2) conflicts with apply.go's documented rationale — end-of-pipeline runs legitimately have dirty trees; removing the escape hatch may make `core_upgrade_apply` unusable in the exact scenario it was built for.
- WARNING: Changing `ToolHandler` signature for context (D2) touches ~54 functions (29 real + 25 stubs); if rejected for churn, the fallback (package-level deadline) is less clean but far smaller. Design must pick explicitly.
- WARNING: Guard middleware on `composer_require` alone leaves the `upgrade_scan` nested-install path unguarded (mcp_tools.go:817-822) — incomplete G1 coverage would be worse than none if it creates a false sense of safety.
- SUGGESTION: New MCP tools (session_open, pipeline_status) shift the locked tool counts (25/29) and require docs/mcp-tools.md + spec updates in the same slice, or the wiring-symmetry tests and M1-style drift come back immediately.

**Open questions**
1. Token transport: server-side session state (recommended; invisible to clients, stdio single-client) vs explicit tool argument (visible, auditable, but schema churn on every mutating tool)?
2. Should `drush_exec` become allowlist-mode by default? The pipeline needs ~a dozen commands (cr, updb, en, pm:list, state:get, config:get…); allowlists age better but any legitimate non-curated command now fails. Blocklist+normalization is the safe middle; allowlist behind config opt-in?
3. `allow_dirty`: remove from MCP surface (CLI keeps flag), or keep gated behind valid session + logged justification in the G3 ledger?
4. Config surface for caps/timeouts/allowlist-mode: state.json (global, user-level), `.drup/config.json` in-project, env vars, or flags? Caps are per-project; timeouts are per-tool-class; today NO config file exists besides state.json (model assignments only).
5. Does G5's "dry-run" apply to tools with no dry-run semantics (apply_patch, composer_require, patch_rollback, cleanup)? Proposal should define refuse vs force-dry-run partitions explicitly.
6. Is S5 (cosign/signature verification for self-update) in scope? Review marks LOW/follow-up; recommend deferring to a separate change.
7. G9 enforcement depth: embed-hash-only (prompt-side consumption) vs validate accepting `expected_hash` fail-closed param (mechanical)? The latter is testable; the former is cheaper.

## Ready for Proposal

Yes. All findings verified with exact locations, blast radius mapped, patterns identified, PR chain sized. The proposal phase must resolve open questions 1–5 (they shape spec requirements); 6–7 can be resolved during design.
