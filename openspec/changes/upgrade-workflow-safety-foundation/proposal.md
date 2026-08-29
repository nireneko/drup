# Proposal: Upgrade Workflow Safety Foundation

## Intent

Prevent currently executable unsafe upgrade decisions: skipping Drupal majors, selecting an incompatible matrix entry, analysis tools changing projects, and retrying mutations. Establish explicit, testable safety boundaries before durable workflow orchestration.

## Scope

### In Scope
- Enforce immediate-next-major or no-op behavior for core upgrade checks and applies.
- Use numeric Drupal/PHP matrix comparisons.
- Add explicit Upgrade Status preparation; keep scan-family analysis read-only.
- Retry only an explicit allowlist of read-only MCP tools.

### Out of Scope
- Persistent run state, run IDs, transition authorization, and evidence-gated commits.
- Checkpoint execution, inventory/reporting, Composer planning, restore recovery, supply-chain hardening, and generated documentation.

## Capabilities

### New Capabilities
- `upgrade-status-preparation`: Explicit mutating preparation of Upgrade Status, separate from analysis.

### Modified Capabilities
- `core-upgrade`: Limit target selection and application to the immediate next Drupal major or a no-op.
- `scan`: Remove implicit Upgrade Status enablement from scan execution.
- `mcp-server`: Make `scan`, `upgrade_scan`, and `validate` read-only; remove autofix rescan; restrict retry eligibility to read-only tools; compare matrix versions numerically.

## Approach

Make the smallest shared-boundary changes: validate target majors in `coreupgrade`, parse version components numerically, move installation/enablement to `prepare_upgrade_status`, and classify retryable handlers by declared read-only effect. Cover equal, lower, skipped, and PHP 8.4 cases with focused table-driven tests. Deliver as autonomous auto-chain slices within the 800-line review budget.

## Affected Areas

| Area | Impact | Description |
|---|---|---|
| `internal/coreupgrade/` | Modified | Immediate-major validation and no-op handling |
| `internal/app/mcp_tools.go` | Modified | Explicit preparation and read-only analysis contracts |
| `internal/mcp/server.go` | Modified | Effect-aware retry policy |
| `internal/{coreupgrade,app,mcp}/*_test.go` | Modified | Safety-boundary tests |

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Existing callers expect `upgrade_scan` to prepare projects | Medium | Provide named preparation tool and clear refusal guidance |
| Read-only validation currently runs mutating Drush commands on D11+ | High | Separate those commands before declaring validation read-only |

## Rollback Plan

Revert the safety-foundation slices as independent commits. No persistent data or migration is introduced; restore prior MCP contracts only if compatibility requires it.

## Dependencies

- Existing Upgrade Status installation and environment-aware command execution.

## Success Criteria

- [ ] Core upgrades cannot target a lower or skipped major.
- [ ] PHP 8.4 selects the correct numeric Drupal compatibility entry.
- [ ] Analysis and retry paths cannot cause project mutation.
- [ ] Focused Go tests prove the new boundaries.
