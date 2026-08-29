## Exploration: Upgrade Workflow Safety Foundation

### Current State

`drup` has useful session, backup, audit, Git, and canonical-path safeguards, but workflow authority remains in prompts. `coreupgrade.NextMajor` selects the latest published major rather than the immediate next major, and `coreupgrade.Apply` accepts any target. The Drupal/PHP matrix compares string keys, so PHP 8.4 can select Drupal 9 instead of 11. Analysis tools still mutate: `scan` enables Upgrade Status, `upgrade_scan` can install and enable it, and `autofix` rescans. MCP retries every handler, including mutators. Patch, cleanup, and core operations can create commits before independent validation.

The workspace is on `main` with no tracked implementation for the proposed run state; `docs/workflow-state-machine.md` and `MEJORAS-PROPUESTAS.md` are untracked design inputs. The existing `auto-update-release-workflow` OpenSpec change is unrelated and complete.

### Affected Areas
- `internal/coreupgrade/check.go` — chooses the latest release rather than the immediate major.
- `internal/coreupgrade/apply.go` — permits skipped/lower target majors and creates a checkpoint itself.
- `internal/app/mcp_tools.go` — implements matrix selection, hidden scan mutations, target-major defaults, core handlers, and report plumbing.
- `internal/mcp/server.go` — retries all MCP handlers and duplicates tool contracts.
- `internal/app/guard.go` — is the shared mutator boundary to extend later with run authorization.
- `internal/patch/patch.go` and `internal/app/cleanup.go` — couple mutation with committing.
- `internal/backup/backup.go` — restores database before replacing files and has no recovery journal.
- `internal/{coreupgrade,app,mcp,patch,backup}/*_test.go` — focused unit and handler coverage for each safety slice.
- `docs/workflow-state-machine.md` and `internal/packaging/templates/` — source workflow contract and agent-facing behavior that must follow implemented Go authority.

### Approaches
1. **Safety foundation first** — Deliver immediate-major enforcement, numeric matrix comparison, explicit Upgrade Status preparation, read-only scan-family tools, and read-only-only retries.
   - Pros: fixes the currently executable unsafe decisions with bounded scope; establishes contracts needed by later run state; fits an 800-line review slice when split into two small PRs.
   - Cons: does not yet persist workflow progress or gate commits with validation evidence.
   - Effort: Medium.

2. **Implement persistent run state first** — Add `internal/runstate`, `run_*` MCP tools, persistence, transition validation, and mutator guards before correcting individual handlers.
   - Pros: establishes the final authority model immediately.
   - Cons: high coupling with unfinished target-major, tool-effect, commit, and retry semantics; likely exceeds the review budget and risks encoding today's unsafe behavior in durable state.
   - Effort: High.

3. **Implement the entire P0/P1 roadmap together** — Build the state machine, evidence commits, checkpoint executor, inventory/reporting, Composer planning, recovery, and contract harness in one change.
   - Pros: reaches the target workflow sooner in calendar time.
   - Cons: too large to validate or review safely; mixes independent recovery, supply-chain, reporting, and orchestration concerns.
   - Effort: Very High.

### Recommendation

Choose **Safety foundation first**, split by autonomous reviewable behavior:

1. Immediate-major/no-op enforcement and numeric version-matrix selection, with table-driven tests for equal, lower, skipped, and PHP 8.4 cases.
2. Explicit `prepare_upgrade_status`; make `scan`, `upgrade_scan`, and `validate` read-only; remove the `autofix` rescan; restrict retry to an explicit read-only allowlist.
3. Only then propose `internal/runstate` plus run-scoped, idempotent mutation authorization. Evidence-gated commits and checkpoints must be designed on top of that persisted authority, not bolted onto current booleans.

This is the smallest valuable delivery: it prevents unsafe major decisions and validator-side mutations now, without prematurely building the large state-machine abstraction. Each slice should remain below the 800-line review budget; use the configured auto-chain strategy if a slice forecast exceeds it.

### Risks
- Requiring a future `run_id` on mutators is a breaking MCP contract; preserve compatibility only for read-only tools or version the contract explicitly.
- Drupal/PHP support data becomes stale unless it records its source/date and has an explicit offline policy.
- `validate` currently runs `updb` and `cr` for Drupal 11+, so making validator operations strictly read-only requires separating those commands rather than merely relabeling the tool.
- Cleanup, patch, and core commit behavior cannot be fixed safely until validation evidence has a persistent, candidate-specific identity.
- Database restoration cannot be made universally atomic across DDEV, Lando, docker4drupal, and direct environments; recovery state must disclose non-atomic windows.

### Ready for Proposal

Yes — propose the first two Safety Foundation slices only. Defer persistent run state, evidence-authorized commits, checkpoints, inventory, Composer planning, transactional restore, supply-chain hardening, and generated documentation to subsequent chained changes.
