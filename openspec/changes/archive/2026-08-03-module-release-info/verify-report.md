# Verification Report: Module Release Info MCP Tool

**Change**: module-release-info
**Version**: N/A (delta specs: `contrib-check`, `mcp-server`)
**Artifact store**: openspec (+ Engram mirror)
**Round**: 3 (final verification after implementation and corrective apply rounds)
**Date**: 2026-08-03

## Verdict Summary

**PASS WITH WARNINGS — ready for `sdd-archive`.**

**Status**: 0 CRITICAL, 9 WARNING, 9 SUGGESTION. Requirements 7/7, scenarios 19/19 by the envelope definition.

**Ready for archive: YES.** No blocker remains. All three CRITICAL issues from earlier rounds are independently verified closed.

## Evidence Summary

**Build & Tests Execution**

```text
gofmt -l .              → exit 0, 0 bytes output (empty)
go build ./...          → exit 0
go vet ./...            → exit 0
go test -count=1 ./... → exit 0, 20 packages ok, 0 failures
```

**Spec Compliance**: 7 requirements, 19 scenarios — all with passing tests.

**Toolchain State at Archive Time**
- `gofmt -l .` empty
- `go build ./...` clean
- `go vet ./...` clean
- `go test -count=1 ./...` all 20 packages passing, 0 failures
- Tool counts: `defaultTools()` = 25, `WireMCPTools` = 29 (confirmed by runtime probe)

## Round-2 CRITICAL Disposition (all resolved)

1. **Delta spec revert risk**: Delta's `New Tool Registration` and `Tool Schema Validation` requirements are now byte-identical to the corrected baseline. Archive merge is idempotent and cannot revert the corrected -32603 contract.

2. **Untested scenario**: *Maintenance Status Extraction → Unsupported project still lists releases* now has a passing `TestCurateReleases` subtest, mutation-verified to bind to production code.

3. **Delta schema validation error contract**: Confirmed byte-identical to baseline. The delta now correctly states handler-level validation surfaces as -32603, with -32602 reserved for malformed outer `params`.

## Delta vs Baseline Consistency

**Spec files**: Both `contrib-check` and `mcp-server` delta specs are verified against their baselines:

- Delta `New Tool Registration` = baseline (byte-identical)
- Delta `Tool Schema Validation` = baseline (byte-identical)
- Delta `contrib-check` adds 4 genuinely new requirements (no collision with baseline)
- Delta `mcp-server` adds 1 genuinely new tool requirement (not in baseline)
- Zero stale "-32602 pre-handler" claims remain in either delta
- Zero change-relative prose found in delta
- Zero "10 new tools" staleness in baseline (cleaned during apply)

## Test Coverage Summary

**Passing tests**: 13 specific change-relevant tests, all PASS on forced `-count=1` re-run:
- `TestCurateReleases` (7 subtests including "unsupported project still lists releases")
- `TestModuleReleaseInfo_*` (4 tests)
- `TestServer_*` (2 wiring tests)
- Guard tests (2 app-level parameter validation tests)

**Coverage note**: 16 of 19 scenarios are covered at their exact boundary; 3 scenarios (`module_release_info` tool success paths) are PARTIAL (covered one layer below, via library tests + runtime probe). Per the verify skill, scenarios are CRITICAL only when they have *no* passing covering test — these have passing tests, so not blockers.

## Non-Blocking Warnings (9)

1. Pre-existing `docs/mcp-tools.md` drift (26 vs 27 vs 29 tools in different lines)
2. Stale Go doc comment identifier `TestServer_PostWireUpCountIs28` (body was updated, comment name was not)
3. `security_covered` field has zero test assertions (mutation-proven to be untested)
4. Three tool-level scenarios remain PARTIAL coverage (handler happy path uncovered)
5. `TestModuleReleaseInfo_InvalidCoreVersion` can pass for wrong reason, would add live network call
6. `design.md` still refers to `majorFromVersion` (code exports `MajorFromVersion`)
7. `tasks.md` 8.2 states delta's `Tool Schema Validation` was "left untouched" as residual risk — now stale (fix was applied directly)
8. Baseline spec was edited during apply (merge will be no-op for MODIFIED requirements)
9. `design.md` open questions remain unchecked

All warnings are non-blocking per the skill definition: none contradict spec statements, none indicate incomplete work.

## Substantive Requirements Confirmed

| Requirement | Confirmed by |
|---|---|
| Unconditional published-status gate | Code re-read: `drupalorg.go:672-677` shows gate before any filter |
| Insecure derivation (exact, case-sensitive, Release type scoped) | Code re-read + passing tests |
| Maintenance status with "unknown" default | Code re-read + 5 passing test cases |
| Fail open on unrecognized Release type terms | Code path confirmed + test coverage |
| not-found vs zero-release distinction | Two passing tests distinguish on both `status` and `found` |
| Core-version filter via `constraintMatchesDrupal` | Code + test coverage (no string equality) |
| Handler guards mirror `contrib_upgrade_path` | Code + runtime probe + two guard tests |
| Pre-handler schema validation absent by design | Confirmed: handler-level validation only, no pre-handler validation |

## Mutation-Based Verification

**Production-side mutations** were applied to prove test binding:

1. Suppress all releases when `MaintenanceStatus == "Unsupported"` → Only one subtest fails (the new one for "Unsupported") — test binds to production code.
2. Invert `SecurityCovered` comparison → No test fails — confirms field has zero covering assertions.

## Runtime End-to-End Probe

Wired server over stdio (`drup mcp`):
- `tools/list` returns 29 tools (confirmed)
- `module_release_info` schema advertised correctly
- Missing required param → -32603 (handler error)
- Invalid `core_version` → -32603 (handler error)
- Malformed outer `params` → -32602 (wire-level error)

Confirms: handler-level validation surfaces as -32603; -32602 reserved for malformed outer `params`.

## Completeness

- Tasks: 27 total, 27 complete (`[x]`), 0 incomplete
- Phases: 8 (original 6 + 2 corrective rounds)
- All tasks map to passing code

## Recommendation

**Ready for `sdd-archive`.**

No blocker remains. Every WARNING is either pre-existing repository drift, documentation staleness, or a coverage gap that does not contradict any spec statement. The strongest warning (mutation-proven `security_covered` untested) is worth a two-line assertion in a follow-up, not a blocker for archive.

Suggested follow-ups (none blocking):
- Two `security_covered` assertions in tests
- Fix the `...Is28` doc comment identifier
- Assert error messages in guard tests
- Add handler happy-path test for full boundary coverage
- Reconcile `docs/mcp-tools.md` totals with runtime 29
