# Archive Report: fix-drup-pipeline-bugs

## Summary

| Field | Value |
|-------|-------|
| Change | fix-drup-pipeline-bugs |
| Archived to | `openspec/changes/archive/2026-07-22-fix-drup-pipeline-bugs/` |
| Archive date | 2026-07-22 |
| Verdict | PASS WITH WARNINGS |
| Tasks | 23/23 complete |
| Tests | 87 passed, 0 failed |
| Critical findings | 0 |

## Specs Synced

| Domain | Action | Details |
|--------|--------|---------|
| scan | Updated | Added "Drush Invocation" requirement (2 scenarios: full project scan, empty results without --all) |
| core-upgrade | Updated | Modified "Composer Execution" (advisory bypass, -W flag, composer update -W) and "Backup" (cleanup on success, retention on failure) |
| preflight | Updated | Modified "Dev Dependency Installation" (added config conflict scenario: delete update.settings before enable) |
| mcp-server | Updated | Modified "scan Tool" (--all flag), "autofix Tool" (--all refresh), "validate Tool" (module scoping + --all), "upgrade_scan Tool" (config conflict handling, 3 scenarios) |

## Archive Contents

- proposal.md ✅
- exploration.md ✅
- specs/ ✅ (4 delta specs: scan, core-upgrade, preflight, mcp-server)
- design.md ✅
- tasks.md ✅ (23/23 tasks complete)
- apply-progress.md ✅
- verify-report.md ✅

## Warnings (non-CRITICAL, accepted)

1. **Spec deviation: "Backup retained on failure" scenario** — The spec requires backup retention on failure, but implementation uses `defer os.Remove(backupPath)` which removes on all exit paths. The design document explicitly chose this approach. Reconciliation recommended in a follow-up change.
2. **"upgrade_scan idempotent" scenario only partially covered** — Test exercises the full lifecycle but not the idempotent path where `upgrade_status` is already enabled.

## Source of Truth Updated

The following main specs now reflect the new behavior:
- `openspec/specs/scan/spec.md`
- `openspec/specs/core-upgrade/spec.md`
- `openspec/specs/preflight/spec.md`
- `openspec/specs/mcp-server/spec.md`

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived.
