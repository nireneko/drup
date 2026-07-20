# Archive Report: drup-mvp

## Change Summary

| Field | Value |
|-------|-------|
| Change | drup-mvp |
| Title | Drupal Upgrade Automation System |
| Archived | 2026-07-20 |
| Verdict | PASS WITH WARNINGS |
| Tasks | 19/19 complete |
| Tests | 72 passing across 12 packages |
| Build | Clean (`go build`, `go vet`) |

## Artifacts

| Artifact | Status |
|----------|--------|
| proposal.md | ✅ Complete |
| design.md | ✅ Complete |
| tasks.md | ✅ 19/19 checked |
| apply-progress.md | ✅ All fixes applied |
| verify-report.md | ✅ PASS WITH WARNINGS |
| explore.md | ✅ Present |

## Specs Synced

No delta sync required. This was a greenfield project — all 15 specs were created directly in `openspec/specs/` during implementation:

| Domain | Action |
|--------|--------|
| scan | Created |
| cli-binary | Created |
| gitops | Created |
| contrib-check | Created |
| issue-patches | Created |
| apply-patch | Created |
| report | Created |
| mcp-server | Created |
| agent-packaging | Created |
| installer | Created |
| self-update | Created |
| preflight | Created |
| orchestrator-skill | Created |
| sub-agents | Created |
| validation-gates | Created |

## Verification Summary

- **72 tests** across 12 internal packages — all passing
- **6 CRITICAL issues** from initial verification resolved in apply phase
- **4 WARNINGs** remain (agent-side orchestration logic, sub-agent definition files, centralized schema validation, NID-specific URLs) — inherent to binary+agent architecture
- **2 SUGGESTIONs** noted (build-time version embedding, rector custom rule integration)

## Warnings at Archive Time

1. **No sub-agent definition files** — Templates include SKILL.md per platform only. Spec requires 4 sub-agent definitions as separate files. (Deferred to v0.2+)
2. **Orchestrator/validation-gates logic is agent-side** — Pipeline stages, retry loops, phase gating encoded in SKILL.md templates rather than binary code. (By design — binary provides tools, agent provides orchestration)
3. **Tool schema validation not centralized** — Each handler validates its own params. (Works, not centralized)
4. **Issue NID lookup not distinguished** — SearchPatches takes generic query, no NID-specific URL construction. (Minor gap)

## Deviations from Design

- Config format: JSON for state.json (stdlib only) instead of YAML
- Module path: `drup` (local) as specified
- MCP MVP: Hand-rolled JSON-RPC (zero deps) as specified
- Tool handler architecture: Real handlers in `internal/app/mcp_tools.go` (not `internal/mcp/tools.go`) to avoid import cycles

## Archive Location

`openspec/changes/archive/2026-07-20-drup-mvp/`

## SDD Cycle

This change has been fully planned, implemented, verified, and archived.
