# Archive Report: drup-post-test-fixes

**Change**: drup-post-test-fixes
**Archived to**: `openspec/changes/archive/2026-07-23-drup-post-test-fixes/`
**Mode**: openspec
**Date**: 2026-07-23
**Verdict**: PASS (18/18 requirements, 294 tests, 0 critical issues)

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| scan | Updated | Replaced JSON parsing with plain-text parsing; removed `--format=json` from Drush Invocation; updated fixture requirement to plain-text |
| cli-binary | Updated | Added `validate` and `apply-patch` commands; updated dispatch to 8 commands; added shared logic requirement |
| mcp-server | Updated | Removed `--format=json` from scan tool; added Drush Error Context requirement |

### Scan Spec Changes
- **Replaced**: "JSON Parsing" requirement → "Plain-Text Parsing" with line-based regex extraction, project detection, tolerant extraction
- **Modified**: "Drush Invocation" — removed `--format=json` from all scenarios, added CLI scan scenario
- **Modified**: "Fixture-Based Parsing" — updated from JSON fixtures to plain-text fixtures, added empty fixture scenario
- **Preserved**: Error Classification, Error Model Structure (unchanged)

### CLI Binary Spec Changes
- **Added**: "Validate Command" requirement with exit code scenarios
- **Added**: "Apply-Patch Command" requirement with success/conflict scenarios
- **Added**: "Shared Logic" requirement (DoValidate, DoApplyPatch shared between CLI and MCP)
- **Modified**: Command Dispatch — expanded from 6 to 8 commands
- **Modified**: Purpose — "6 commands" → "8 commands"

### MCP Server Spec Changes
- **Modified**: scan Tool — removed `--format=json` from scenario
- **Added**: "Drush Error Context" requirement with non-zero exit and parse failure scenarios

## Archive Contents

- proposal.md ✅
- spec.md ✅
- tasks.md ✅ (13/13 tasks complete)
- verify-report.md ✅ (PASS, 18/18)
- exploration.md ✅
- archive-report.md ✅ (this file)

### Missing Artifacts
- **design.md**: Not present in the change folder. The change was implemented directly from the spec and tasks without a separate design document. This is noted for audit trail completeness.

## Task Completion

All 13 tasks marked `[x]` in tasks.md:
- A-1: Rewrite `scan.Parse()` for plain-text output ✅
- A-2: Replace JSON fixtures with plain-text fixtures ✅
- A-3: Update `scan_test.go` for plain-text fixtures ✅
- A-4: Remove `--format=json` from `RunScan()` ✅
- A-5: Remove `--format=json` from 4 MCP tool call sites ✅
- C-1: Add `drushExecError` helper and apply to all drush call sites ✅
- B-1: Extract shared logic and add `validate` + `apply-patch` CLI commands ✅
- D-1: Update all 4 SKILL.md copies ✅
- E-1: Add `RunScan` CLI integration test with plain-text mock ✅
- E-2: Update MCP tool tests with plain-text mock output ✅
- V-1: Run full test suite and verify all groups ✅

## Source of Truth Updated

The following main specs now reflect the new behavior:
- `openspec/specs/scan/spec.md`
- `openspec/specs/cli-binary/spec.md`
- `openspec/specs/mcp-server/spec.md`

## Warnings

- **design.md missing**: No design document was produced for this change. The implementation proceeded directly from spec + tasks. This is acceptable for a bug-fix change but should be noted for audit completeness.
- **Group D (SKILL.md sync) and Group E (test coverage)**: These groups affected code and documentation but did not produce delta spec sections that map to main specs. SKILL.md files are managed by the packaging system, not the spec system. Test coverage requirements are captured in the scan and mcp-server specs via the fixture and error context requirements.

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived.
Ready for the next change.
