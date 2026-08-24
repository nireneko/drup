# Verification Report: fix-review-findings-guardrails

**Change**: fix-review-findings-guardrails
**Artifact store**: openspec
**Round**: 1 (independent post-apply verification)
**Date**: 2026-08-24

## Verdict Summary

**PASS WITH WARNINGS — ready for `sdd-archive`.**

**Status**: 0 CRITICAL, 1 WARNING, 2 SUGGESTION. Tasks 44/44 complete. Requirements 24/24 across 10 spec files (70 scenarios), all high-risk requirements spot-checked end-to-end against passing tests. No blocker for archive.

## Evidence Summary

**Build & Tests Execution (independently re-run, not reused from apply-phase claims)**

```text
gofmt -l .              → exit 0, empty output
go build ./...          → exit 0
go vet ./...            → exit 0
go test -count=1 ./...  → exit 0, 22 packages ok, 0 failures (cmd/drup has no test files)
```

**Working tree state**: `git diff --cached --stat` empty; `git status --porcelain` shows only modified/untracked paths, nothing staged. `git log --oneline -5` shows no new commits since `c21f63c` — confirms no `git add`/`git commit` was ever run against the real repo during apply, per the explicit user constraint.

## Task Completeness

All 44 tasks across Phases 1–8 marked `[x]` in `tasks.md`. No unchecked tasks.

## High-Risk Requirement Spot-Checks (source + runtime test evidence)

| Requirement | Verified by | Result |
|---|---|---|
| 3 documented patch-allowlist bypass URLs rejected | `internal/patch/patch.go` `defaultCheckAllowedURL` (url.Parse + Hostname exact/suffix match, https-only) + `TestDefaultCheckAllowedURL` all 7 subtests re-run | PASS — all 3 bypass shapes (`evil.com/www.drupal.org/...`, `drupal.org.evil.com`, `notdrupal.org/?x=git.drupal.org`) rejected |
| Drush alias bypasses blocked | `drushAliasMap` resolves aliases to canonical form before `drushBlocklist` lookup in `normalizeDrushCommand`; `TestRealHandleDrushExec` alias/blocklist subtests re-run | PASS |
| Full guarded-tool set wired into `WireMCPTools` | 11 `guardHandler(...)` call sites == `len(ForceDryRunTools)+len(RefuseOnlyTools)` (4+7=11); `TestGuardedTools_IsUnionOfBothPartitions` and `TestWireMCPTools_UpgradeScanItselfIsNotRegistrationGuarded` re-run | PASS |
| Kill switch overrides an open session | `session.EvaluateGuard`: `killSwitchActive()` checked before `Current()` session lookup; `TestEvaluateGuard_KillSwitchRefusesForceDryRunToolsWithValidSession` / `...RefuseOnlyToolsWithValidSession` re-run, both green with a valid bound session present | PASS |
| `DRUP_ALLOW_UNSAFE` bypasses everything | `allowUnsafeActive()` checked first in `EvaluateGuard`, short-circuits kill switch and session; `TestEvaluateGuard_AllowUnsafeBypassesKillSwitchAndSession` re-run | PASS |
| Backup-freshness gate covers nested `upgrade_scan` install path | `realHandleUpgradeScan` routes its nested install through `guardedCall("composer_require", ...)` (not the narrower `session.RequireInstallAllowed`), so `EvaluateBackupFreshness` applies identically to a direct call; `TestRealHandleUpgradeScan_NestedInstallPathAllowedWithMatchingSession` (requires `writeFreshBackupManifest`) and `...GuardedWithoutSession` re-run | PASS |
| Audit ledger write failure never blocks a response | `audit.Append` has no return value — a write failure only calls `logFn` (stderr) and returns nothing to the caller | PASS (structural — `Append`'s signature makes blocking impossible) |
| Cap enforcement refuses before the handler runs | `guardedCall` calls `audit.CheckCap(...)` and returns the refusal *before* invoking `handler(args)` | PASS (source-confirmed ordering) |
| `gitops.Commit` aborts on unexpected staged file | `git diff --cached --name-only` checked against declared `files`; unexpected path triggers `git reset` + error naming the path, before any commit; `TestCommit_UnexpectedStagedFile_Aborts` re-run | PASS |
| `validate`'s `expected_hash` fails closed regardless of `total_errors` | Mismatch check runs unconditionally before any `total_errors` gating; `TestRealHandleValidate_MismatchedExpectedHash_FailsClosedRegardlessOfTotalErrors` re-run (zero-finding scan still fails on stale hash) | PASS |
| README/docs tool-count drift fix | `grep -c "s.RegisterTool("` = 31, matches README's "31 tools" and `TestServer_PostWireUpCountIs31` (27 default + 4 backup) | PASS |
| `--locked` CLI wiring gap closure (PR5.8) | `installLockedRequested`, `installAgents` threading, `RunInstall`/`RunSync` parsing re-run: `TestInstallLockedRequested`, `TestInstallAgents_LockedRendersLockedArgIntoMcpConfig`, `TestRunInstall_LockedFlagRendersLockedArgForAllPlatforms`, `TestRunInstall_WithoutLockedFlagOmitsLockedArg`, `TestRunSync_LockedFlagRendersLockedArgForAllPlatforms` | PASS |
| Stale `TypeRegA` docblock / `copyFile` double-Close (PR8/M3/M5) | `grep TypeRegA` → no match; `copyFile` has exactly one `defer out.Close()`, no explicit second `Close()` call | PASS |

## Design Coherence

Both design contradictions the apply-phase validators flagged and resolved before tasks (canonical-root symlink unification, gitops scoped-commit helper) are correctly reflected in the final code: `session.ResolveSymlinks` is the shared helper called from `coreupgrade.ValidateProjectPath`, `backup.validateProject`, and `envdetect.Detect` exactly as `design.md`'s Open Questions describe (no marker check forced onto backup/envdetect); `gitops.Commit` verifies the staged set post-`git add` per the Scoped Commit Helper decision.

Guard partition (`ForceDryRunTools`/`RefuseOnlyTools`/`GuardedTools()`) in `internal/session/session.go` is byte-for-byte consistent with design.md's "Guarded Tool Set (complete)" table.

## Non-Blocking Issues

**WARNING (1)**

1. `session.RequireInstallAllowed` (task 4.7's literal description: "add inline `session.RequireInstallAllowed` in `realHandleUpgradeScan`") exists, is exported, and has its own direct unit test (`TestRequireInstallAllowed_NestedUpgradeScanInstallPathGuarded`), but the actual production call site in `realHandleUpgradeScan` calls `guardedCall("composer_require", ...)` directly instead — a strict superset that also adds backup-freshness, mutation-cap, and audit-trail coverage `RequireInstallAllowed` alone would not provide. Functionally this exceeds the task/design requirement (confirmed by the passing "AllowedWithMatchingSession" test which requires a fresh backup manifest), but `RequireInstallAllowed` itself is now dead code outside its own test — worth a doc-comment note or removal in a follow-up, not a spec violation.

**SUGGESTION (2)**

1. `docs/mcp-tools.md` documents the wiring-symmetry rule narratively but does not carry an explicit numeric "31 tools" total anywhere the way `README.md` does twice — a future drift is easier to miss there since there's no single number to grep for.
2. `PROJECT-REVIEW.md` remains untracked at repo root; if this file is meant to travel with the change (source-of-truth findings list), consider whether it should be committed alongside the eventual PR chain or intentionally left local-only.

## Recommendation

**Ready for `sdd-archive`.**

No CRITICAL issue found. The single WARNING documents a beneficial deviation (stronger guarding than literally specified) rather than a regression, and both SUGGESTIONs are documentation polish with no functional impact. Build, vet, gofmt, and the full test suite (22 packages) are all green on an independent re-run, and every explicitly-flagged high-risk security/guard requirement was traced from source through to a passing runtime test.
