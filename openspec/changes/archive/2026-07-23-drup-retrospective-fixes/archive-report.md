# Archive Report: drup-retrospective-fixes

## Summary

| Field | Value |
|-------|-------|
| Change | drup-retrospective-fixes |
| Archived to | `openspec/changes/archive/2026-07-23-drup-retrospective-fixes/` |
| Mode | openspec |
| Verdict | **PASS** |
| Tasks | 26/26 complete |
| Commits | 7 |
| Tests | All passing (`go test ./...` exit 0) |

## What Was Done

Six critical fixes to the drup CLI pipeline, identified during a real Drupal 10.6 → 11.4 upgrade where ~60% of the automated pipeline had to be bypassed manually.

### Fix 1 — Exit Code 3 Semantics (P0)
- Added `isScanExitOK()` shared helper — exit codes 0 and 3 treated as success, 1/2/>3 as errors
- Wired into 4 call sites: `RunScan`, `DoValidate`, `realHandleScan`, `realHandleAutofix`
- Empty stdout + exit 3 treated as error (drush crash, not findings)

### Fix 2 — DDEV-Aware Execution (P0)
- Added `cliRun()` helper that calls `envdetect.Detect()` + `drupexec.RunWithEnv()`
- Replaced all `drupexec.Run("drush", "-r", path, ...)` with `cliRun(path, "drush", ..., "--root="+path)`
- All CLI commands now auto-detect DDEV and prefix commands accordingly

### Fix 3 — MCP Tool Schema Exposure (P0)
- Added `toolRegistry` map with JSON Schema definitions for all 20 tools
- Each tool now exposes `properties` (name, type, description) and `required` fields
- Replaced empty `inputSchema: {"type": "object"}` with full schemas

### Fix 4 — Contrib Compound Constraint Parsing (P1)
- Added `constraintMatchesDrupal()` — splits on `||`, handles `^`, `>=/<` ranges
- Added `fetchInfoYML()` — fetches `.info.yml` from git.drupalcode.org
- `CheckRelease` now parses `core_version_requirement` for accurate D11 compatibility

### Fix 5 — PHP 8.4 Deprecation Suppression (P1)
- Added `isPHP84OrLater()` version detection
- Added `patchSettingsPHP()` — appends `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` after DDEV include block
- Idempotent (checks for existing line), creates `.bak` backup

### Fix 6 — Report Data Collection (P2)
- `RunReport` now calls `DoValidate` for live scan data
- `TotalErrors` populated from actual results, not hardcoded zeros
- `Resolved` and `Pending` arrays populated from scan output

## Key Metrics

| Metric | Value |
|--------|-------|
| Files changed | 7 (commands.go, mcp_tools.go, server.go, drupalorg.go + 3 test files) |
| New functions | `isScanExitOK`, `cliRun`, `constraintMatchesDrupal`, `fetchInfoYML`, `isPHP84OrLater`, `patchSettingsPHP` |
| Test coverage | 16 spec scenarios → 16+ passing tests |
| Packages tested | 15 packages, all pass |
| Spec compliance | 100% — all scenarios have corresponding tests |

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| scan | Updated | Replaced "Drush Invocation" — added `--root=`, env detection, exit code 3 handling (3 new scenarios) |
| validation-gates | Updated | Extended "Phase Gating" — added exit code 3 semantics (2 new scenarios) |
| mcp-server | Updated | Extended "Tool Schema Validation" — added schema exposure requirements (2 new scenarios) |
| contrib-check | Updated | Extended "Release History Lookup" — added compound constraint support (3 new scenarios) |
| preflight | Updated | Extended "Dev Dependency Installation" — added PHP 8.4 detection and auto-patch (3 new scenarios) |
| report | Updated | Extended "JSON Report Generation" — added real data collection requirements (2 new scenarios) |

## Archive Contents

- proposal.md ✅
- spec.md ✅
- design.md ✅
- tasks.md ✅ (26/26 tasks complete)
- verify-report.md ✅ (PASS, no CRITICAL issues)
- exploration.md ✅

## Deferred Items

Per the proposal's "Out of Scope":
- Stage 6 (CUSTOM LOOP) implementation — no CLI command exists; deferred to separate change
- E2E test infrastructure — planned for later phase
- Sub-agent fallback tools when MCP fails
- `drup fix` error handling when no custom modules exist (minor)

## Known Issues

None. All 8 success criteria met. No CRITICAL, WARNING, or SUGGESTION issues in verification.

## Recommendations

1. **E2E tests**: Add integration tests that exercise the full pipeline under DDEV to catch env-detection regressions early.
2. **git.drupalcode.org URL stability**: The contrib constraint fix depends on `https://git.drupalcode.org/project/<module>/-/raw/<branch>/<module>.info.yml`. Monitor for URL format changes from Drupal.org.
3. **PHP version handling**: The PHP 8.4 patch is append-only. Consider a more robust settings.php management approach if more PHP-version-specific patches are needed in the future.

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived.
All delta specs merged into main specs. Ready for the next change.
