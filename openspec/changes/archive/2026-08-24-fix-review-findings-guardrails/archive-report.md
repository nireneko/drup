# Archive Report: fix-review-findings-guardrails

**Change**: fix-review-findings-guardrails  
**Archived to**: `openspec/changes/archive/2026-08-24-fix-review-findings-guardrails/`  
**Archive Date**: 2026-08-24  
**Status**: Complete and Closed

## Executive Summary

The fix-review-findings-guardrails change has been successfully planned, implemented, verified, and archived. All 8 implementation phases are complete with passing test coverage (23 packages, all green). Delta specs have been merged into main specs. The change is ready for production.

## Final State Authority

This archive report describes the **FINAL STATE** of the change at close, superseding all intermediate snapshots. The Final-State Authority hierarchy (per sdd-archive SKILL.md) is:

1. **Native review authority** — (not applicable; no review was conducted)
2. **Persisted tasks artifact** — all 8 phases marked complete (all checkboxes [x])
3. **Explicit final-state facts from orchestrator launch prompt**:
   - WARNING resolved: `session.RequireInstallAllowed` dead code removed; full test suite re-confirmed green
   - 2 SUGGESTIONS left intentionally out-of-scope (docs drift, untracked file)
   - All tests pass: 23 packages, all ok
4. **Verify-report & apply-progress** — intermediate snapshots

Per this hierarchy, stale checkpoint claims from intermediate snapshots are **not** reported as current state; final facts are authoritative.

## Artifacts and Completion

### Task Completion Gate

All 8 implementation phases are complete:
- [x] Phase 1: PR1 — Patch Allowlist (1.1, 1.2)
- [x] Phase 2: PR2 — Transport Hardening (2.1-2.8)
- [x] Phase 3: PR3 — Drush Hardening (3.1-3.4)
- [x] Phase 4: PR4 — Session + Root Pinning (4.1-4.13)
- [x] Phase 5: PR5 — Kill Switch + Dry-Run Partition (5.1-5.8)
- [x] Phase 6: PR6 — Backup Gate + Audit Ledger (6.1-6.8)
- [x] Phase 7: PR7 — Scoped Commits + Evidence Hash (7.1-7.8)
- [x] Phase 8: PR8 — Docs + Drift Fixes (8.1-8.5)

**Task Completion Gate Status**: ✅ PASSED — No unchecked implementation tasks remain.

### Artifacts Present in Archive

- ✅ `proposal.md` — Scope, approach, and rollback plan
- ✅ `explore.md` — Research and discovery
- ✅ `design.md` — Detailed design decisions
- ✅ `specs/` (10 domain specs) — See "Specs Synced" below
- ✅ `tasks.md` — Complete task breakdown with all checkboxes marked
- ✅ `verify-report.md` — Verification findings and test coverage

### Specs Synced to Main

The following delta specs have been merged into `openspec/specs/`:

| Domain | Action | Details |
|--------|--------|---------|
| agent-session | Created | Full new domain spec (session lifecycle, guard middleware) |
| mutation-audit | Created | Full new domain spec (ledger, caps, pipeline_status tool) |
| apply-patch | Updated | +1 ADDED requirement (Patch URL Allowlist Validation) |
| cleanup-stage | Updated | Modified requirement (writer-injected output, scoped commit) |
| core-upgrade | Updated | +2 ADDED requirements (MCP allow_dirty removal, unified canonical root) |
| installer | Updated | +1 ADDED requirement (Locked MCP Launch Flag) |
| mcp-server | Updated | Modified 2 requirements + 5 ADDED requirements (buffer limit, subprocess timeout, deterministic tool listing, JSON-RPC notifications, kill switch/dry-run partition, session_open/pipeline_status registration) |
| gitops | Updated | +1 ADDED requirement (Scoped Commit Helper) |
| scan | Updated | +1 ADDED requirement (Evidence Hash) |
| validation-gates | Updated | +1 ADDED requirement (Evidence Hash Verification) |

**Spec Sync Status**: ✅ All delta specs merged successfully. No conflicts or truncations detected.

## Verification Status

Per the launch prompt (explicit final-state facts):

### WARNING Resolution

**Original finding** (verify-report.md): `session.RequireInstallAllowed` was flagged as dead code (unused in production, superseded by `guardedCall` covering the same nested upgrade_scan path with additional backup-freshness/cap/audit coverage).

**Final status**: ✅ RESOLVED
- Function removed from `internal/session/session.go`
- Dedicated test `TestRequireInstallAllowed_NestedUpgradeScanInstallPathGuarded` removed from `internal/session/session_test.go`
- Full build + vet + gofmt + test suite (23 packages) re-confirmed green after removal
- No other warnings remain

### Suggestions (Intentional)

Two suggestions from verify-report.md were left intentionally out-of-scope:
1. **docs/mcp-tools.md lacking a single canonical tool-count total string** — Deferred to a follow-up documentation polish pass
2. **PROJECT-REVIEW.md sitting untracked at repo root** — Out-of-scope for this change, left as user decision

Neither is blocking. Both are noted for future work.

### Test Coverage

**Final state**: ✅ All pass
- Go test suite: 23 packages tested
- Build: ✅ green
- Vet: ✅ green
- Gofmt: ✅ green
- Test coverage: ✅ all ok

Verification status per `sdd-verify` flow: 0 CRITICAL, 0 WARNINGS (resolved), 2 SUGGESTIONS (intentional, out-of-scope).

## Archive Operations

### New Domain Specs (Mechanical Copy)

Two new domain specs were created (not merged from existing):
- `openspec/specs/agent-session/spec.md` — Copied from delta, no truncation detected
- `openspec/specs/mutation-audit/spec.md` — Copied from delta, no truncation detected

**Verification**: Empty `diff -r` output confirmed byte-identity after copy.

### Change Folder Move to Archive

Source folder: `/home/borja/sites/borja/go/drup/openspec/changes/fix-review-findings-guardrails`  
Archive destination: `/home/borja/sites/borja/go/drup/openspec/changes/archive/2026-08-24-fix-review-findings-guardrails`

**Move method**: Plain `mv` (not `git mv`)  
**Pre-move snapshot**: Created and verified  
**Post-move verification**: Empty `diff -r` output — archive contents match pre-move snapshot exactly

**Verification**: ✅ Archive move successful; source removed; contents verified byte-identical.

## Delivery Summary

| Item | Status | Evidence |
|------|--------|----------|
| All 8 phases complete | ✅ | All checkboxes [x] in tasks.md |
| Test suite green | ✅ | 23 packages, all ok |
| Verify-report issues resolved | ✅ | WARNING fixed; 2 SUGGESTIONS intentional |
| Task Completion Gate | ✅ | PASSED |
| Delta specs merged | ✅ | 8 modified + 2 new domains in openspec/specs/ |
| Change folder archived | ✅ | Moved to `2026-08-24-fix-review-findings-guardrails` |
| Archive verification | ✅ | Byte-identity confirmed by diff -r |
| Archive report written | ✅ | This report |

## Cycle Closed

The SDD cycle for **fix-review-findings-guardrails** is **COMPLETE**. 

- **Proposal**: Defined scope, approach, and rollback
- **Spec**: Froze requirements across 10 domain specs
- **Design**: Detailed 8 phases with 50+ implementation tasks
- **Apply**: Implemented all phases, all tests green
- **Verify**: Resolved all critical/warning issues, 2 suggestions left intentional
- **Archive**: Merged deltas into main, moved change to archive, wrote audit trail

Ready for the next change or production deployment.

## Key Learnings

1. Mechanical copy contracts must use shell operations only; model-based Read/Write bypasses are dangerous and truncate silently.
2. Final-State Authority hierarchy requires explicit ranking of sources to avoid stale snapshot claims masquerading as current facts.
3. Scoped-commit helpers and evidence hashes improve auditability by preventing `git add -A` sprawl and enabling deterministic validation re-runs.
4. Session-based guards with mutation caps and backup-freshness gates significantly reduce operational blast radius in automated upgrade scenarios.
5. Multi-phase work benefits from canonical root resolution shared across all entry points (CLI and MCP) to eliminate symlink/path-traversal edge cases.
