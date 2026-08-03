# Archive Report: drup-retrospective-fixes

**Change**: drup-retrospective-fixes  
**Display name**: MCP Server and Scan Robustness (Retrospective Fixes)  
**Archived**: 2026-08-04  
**Archive Path**: `openspec/changes/archive/2026-08-04-drup-retrospective-fixes/`  
**Status**: CLOSED — PASS  
**Mode**: openspec  

## Executive Summary

The drup-retrospective-fixes change addresses the P0/P1 failures from the 2026-08-03 retrospective. 6 requirements implemented across 8 commits on `main`: uniform MCP response envelope, selective retry for transient errors, `realHandleScan` auto-enable of `upgrade_status`, tool count assertion, sub-agent template updates for `result.payload` parsing, and a full regression gate. All 34+ tests pass. The verify-report's single CRITICAL finding (missing metrics recording in retry loop) was fixed in commit `56b912d`. The change is archived with all delta specs merged into main specs.

## Final State Authority

This archive report describes the state of the change **at close** (2026-08-04), superseding intermediate snapshots. Per the Final-State Authority hierarchy:

1. **Orchestrator launch prompt** (highest-ranked for final-state facts): 8 commits on `main`, all 6 REQs pass, smoke test confirmed against real `drup` binary (29 tools, envelope on success, `{status:fail}` on tool error), all tests pass (`go test ./...` and `go test ./internal/e2e/...`), 0 "Co-Authored-By" lines, working tree clean except SDD workflow artifacts.
2. **Persisted tasks artifact**: all implementation tasks checked `[x]`.
3. **Verify-report** (intermediate snapshot): recorded 1 CRITICAL (metrics recording missing) and 2 WARNINGs at verification time. The CRITICAL was fixed in commit `56b912d` (subsequent work). The WARNINGs were addressed: spec deviation was intentional per tasks/design and the spec was corrected in `387991d`; protocol extension documentation was added in `387991d`.

## Final State — What Shipped

### Commits on `main` (8 total, ahead of `origin/main` by 8)

| # | SHA | Description | REQ |
|---|-----|-------------|-----|
| 1 | `229c9b9` | `fix(mcp): assert tool count in TestWireMCPTools_AllToolsRegistered` | REQ-4 |
| 2 | `5a61d56` | `fix(scan): auto-enable upgrade_status in realHandleScan` | REQ-1 |
| 3 | `c683c71` | `fix(mcp): wrap all tool responses in uniform envelope` | REQ-2 |
| 4 | `7a7df74` | `fix(mcp): add selective retry for transient MCP tool errors` | REQ-3 |
| 5 | `2258494` | `docs(templates): update sub-agent templates to read result.payload` | REQ-5 |
| 6 | `c4c7a92` | `test(packaging): add grep test for sub-agent templates reading result.payload` | REQ-5 (fixup) |
| 7 | `56b912d` | `fix(mcp): record retry metric in retryLoop (REQ-3 gap)` | REQ-3 (CRITICAL fix) |
| 8 | `387991d` | `docs(mcp): document uniform response envelope contract` | Housekeeping (docs + spec drift fix) |

### Verification State at Close

- **6/6 REQs pass** (5 on first verify, REQ-3 fixed in `56b912d` and re-tested)
- **All tests pass**: `go test ./...` (21 packages), `go test ./internal/e2e/...` (3 tests), `go test ./internal/mcp/...` (27 tests), `go test ./internal/app/...` (34 tests), `go test ./internal/packaging/...` (20 tests)
- **Smoke test**: real `drup` binary confirmed 29 tools registered, envelope on success, `{status:fail}` envelope on tool error (NOT JSON-RPC error)
- **0 commits with "Co-Authored-By" lines**
- **Working tree clean** except `RETROSPECTIVE.md` and `openspec/changes/drup-retrospective-fixes/` (SDD workflow artifacts)

### Resolution of Verify-Report Findings

| Finding (at verify time) | Severity | Resolution | Commit |
|---|---|---|---|
| `metrics.Default().RecordRetry()` not called in `retryLoop` | CRITICAL | Fixed: `metrics.Default().RecordRetry()` added inside retry loop | `56b912d` |
| Spec deviation: `realHandleUpgradeScan` was modified | WARNING | Intentional per tasks/design (DRY refactor); spec updated to reflect shared helper | `387991d` |
| Protocol extension not documented in user-facing docs | WARNING | Documented in `docs/mcp-tools.md` and SKILL.md dispatch contract | `387991d` |
| `deriveSummary` coverage is minimal | SUGGESTION | Accepted as-is; generic fallback is safe | N/A |
| Smoke test was manual, not automated | SUGGESTION | Accepted as-is; not blocking | N/A |

## Specs Synced

Delta spec `mcp-server-and-scan-robustness.md` contained 6 ADDED requirements. REQ-6 is an acceptance gate (not a spec requirement); the other 5 were merged into main specs.

| Main Spec | Action | Requirements Merged | Details |
|-----------|--------|-------------------|---------|
| `openspec/specs/scan/spec.md` | Updated | REQ-1 | Added "realHandleScan Auto-Enables upgrade_status" requirement (3 scenarios) |
| `openspec/specs/mcp-server/spec.md` | Updated | REQ-2, REQ-3, REQ-4 | Added "Uniform Response Envelope" (3 scenarios), "Selective Retry for Transient Errors" (3 scenarios), "Tool Count Assertion" (2 scenarios) |
| `openspec/specs/sub-agents/spec.md` | Updated | REQ-5 (part 1) | Added "MCP Response Envelope Parsing" requirement (2 scenarios) |
| `openspec/specs/orchestrator-skill/spec.md` | Updated | REQ-5 (part 2) | Added "MCP Tool Response Envelope (Dispatch Contract)" requirement (1 scenario) |

**Total**: 4 main specs updated, 5 requirements added, 11 scenarios added. All existing requirements preserved.

## Source of Truth Updated

The following main specs now reflect the new behavior:

- `openspec/specs/scan/spec.md` — auto-enable requirement for `realHandleScan`
- `openspec/specs/mcp-server/spec.md` — uniform envelope, selective retry, tool count assertion
- `openspec/specs/sub-agents/spec.md` — sub-agent template `result.payload` parsing
- `openspec/specs/orchestrator-skill/spec.md` — dispatch contract for MCP envelope

## Tasks Completed

All 5 commit-level tasks + 3 follow-up tasks are complete:

| Task | Commit | Status |
|------|--------|--------|
| REQ-4: Tool count assertion | `229c9b9` | ✅ Complete |
| REQ-1: `realHandleScan` auto-enable | `5a61d56` | ✅ Complete |
| REQ-2: Uniform MCP envelope | `c683c71` | ✅ Complete |
| REQ-3: Selective retry | `7a7df74` | ✅ Complete |
| REQ-5: Sub-agent template updates | `2258494` | ✅ Complete |
| REQ-5 fixup: grep test | `c4c7a92` | ✅ Complete |
| REQ-3 fix: metrics.RecordRetry wired | `56b912d` | ✅ Complete |
| Housekeeping: docs + spec drift fix | `387991d` | ✅ Complete |

**Total**: 8/8 tasks complete. All checkboxes in `tasks.md` are `[x]`.

## Deliberate Design Decisions

### MCP Protocol Extension: Errors as `{status:fail}` in Result Channel

Tool errors are returned as `{"status":"fail","summary":"..."}` in the JSON-RPC result channel, NOT as JSON-RPC errors. This is a deliberate protocol extension documented in:
- `openspec/specs/mcp-server/spec.md` (Uniform Response Envelope requirement)
- `openspec/specs/orchestrator-skill/spec.md` (Dispatch Contract requirement)
- `docs/mcp-tools.md` (envelope section, commit `387991d`)
- Sub-agent templates (MCP Response Contract section)

Rationale: JSON-RPC errors are transport-level signals that some MCP clients swallow silently. A `{status:fail}` in the result channel is always visible to the orchestrator model. This was the core complaint of the retrospective.

### `realHandleUpgradeScan` Refactored to Share Helper

The spec originally stated "MUST NOT modify `realHandleUpgradeScan`" but tasks/design explicitly called for a DRY refactor to share the `ensureUpgradeStatusEnabled` helper. The implementation followed tasks/design. The auto-install block (lines 816-822) was preserved verbatim. The pre-change behavior is identical; only the code path changed. This was recorded as a spec deviation in the verify-report and corrected in the main spec at archive time (commit `387991d`).

## Risks and Known Issues

**None.** All verify-report findings have been resolved. The spec drift has been corrected. The protocol extension is documented. No open issues remain.

## Archive Contents

The archive folder contains all 7 artifacts:

- `explore.md` ✅ — initial exploration and MCP envelope audit
- `proposal.md` ✅ — user intent, scope, dependencies, risks, rollback plan
- `design.md` ✅ — technical approach, architecture decisions, file changes
- `tasks.md` ✅ — 8 tasks across 5 commits + 3 follow-ups, all `[x]` complete
- `verify-report.md` ✅ — verification results (CRITICAL fixed in `56b912d`)
- `specs/mcp-server-and-scan-robustness.md` ✅ — delta spec (6 ADDED requirements)
- `archive-report.md` ✅ — this file

## Rollback Boundary

Reverting the 8 commits would restore the pre-change state:
- No uniform envelope (tools return raw payloads)
- No selective retry (transient errors fail immediately)
- `realHandleScan` does not auto-enable `upgrade_status`
- Tool count test is a no-op
- Sub-agent templates read `result` directly

Older binary would expose the same 29 tools with their original response shapes.

## Final Toolchain State (at archive time)

```
Project: drup (github.com/nireneko/drup)
Mode: openspec
Archived: 2026-08-04

go test ./...           → exit 0 (21 packages ok)
go test ./internal/e2e/... → exit 0 (3 tests)
go vet ./...            → exit 0

MCP tools (runtime)     → 29
Envelope wrapper        → active (handleToolCall)
Retry loop              → active (transient errors only)
Metrics recording       → active (RecordRetry wired)
```

---

**Archive closed by**: sdd-archive executor  
**Mode**: openspec (filesystem archive)  
**Final verdict**: PASS — no blockers, no open issues, ready for next change
