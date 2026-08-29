# Apply Progress: Upgrade Workflow Safety Foundation

## Work Unit 1: Core Gate and Numeric Matrix

**Delivery**: auto-chain; stacked-to-main; PR 1 — Core gate and numeric matrix.

## Cumulative Completed Tasks

- [x] 1.1 RED: Test equal, lower, skipped, dirty, and canonical-root safety boundaries.
- [x] 1.2 GREEN: Enforce immediate-next core-major targets and successful equal-major no-ops.
- [x] 1.3 RED: Test PHP 8.4 numeric matrix selection and the unknown Drupal 99 error.
- [x] 1.4 GREEN: Compare matrix Drupal versions numerically.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `internal/coreupgrade/apply_test.go` | Integration | `go test ./internal/coreupgrade` — pass | `TestApply_RejectsUnsafeTargetsAndNoOpsAtCurrentMajor` failed for equal, lower, and skipped targets before implementation. | `go test ./internal/coreupgrade -run TestApply_RejectsUnsafeTargetsAndNoOpsAtCurrentMajor -count=1` — pass. | Equal, lower, and skipped cases; existing dirty and canonical-root cases remain covered. | None needed; minimal guard is clear. |
| 1.2 | `internal/coreupgrade/apply_test.go` | Integration | `go test ./internal/coreupgrade` — pass | Covered by task 1.1 RED test. | `go test ./internal/coreupgrade -count=1` — pass. | Immediate next, no-op, lower, and skipped behavior covered. | None needed. |
| 1.3 | `internal/app/mcp_tools_test.go` | Unit | `go test ./internal/app` — pass | PHP 8.4 selected Drupal 9 rather than Drupal 11 before implementation. | `go test ./internal/app -run 'TestDrupalVersionMatrix_(SelectsHighestNumericMajorForPHP84|UnknownVersion)' -count=1` — pass. | PHP 8.2 selects 10; PHP 8.4 selects 11; unknown 99 preserves its error. | None needed; direct numeric comparison is clear. |
| 1.4 | `internal/app/mcp_tools_test.go` | Unit | `go test ./internal/app` — pass | Covered by task 1.3 RED test. | `go test ./internal/app -run 'TestDrupalVersionMatrix_(SelectsHighestNumericMajorForPHP84|UnknownVersion)' -count=1` — pass. | PHP 8.2 and PHP 8.4 force numeric major ordering. | None needed. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/coreupgrade ./internal/app -count=1` — both packages passed (`internal/coreupgrade`: 0.350s; `internal/app`: 1.252s). |
| Runtime harness command/scenario and exact result | N/A — this slice is pure Go boundary logic; `t.TempDir()` Git repositories and table-driven command-free tests exercise filesystem and mutation boundaries without a separate runtime service. |
| Rollback boundary | Revert `internal/coreupgrade/apply.go`, `internal/coreupgrade/apply_test.go`, `internal/app/mcp_tools.go`, and `internal/app/mcp_tools_test.go`; this removes only immediate-major gating and numeric matrix ordering. |

## Test Summary

- **Total tests written**: 5 scenarios (three core target cases, two matrix selection cases) plus the exact unknown-version assertion.
- **Total tests passing**: 5 new scenarios and all focused package tests.
- **Layers used**: Unit (2 matrix cases), Integration (3 core cases).
- **Approval tests**: None — behavior changes were specified.
- **Pure functions created**: 1 (`composerCoreMajor`).

## Deviations and Issues

None — implementation matches the assigned design and scope.

## Work Unit 2: Explicit Preparation and Read-only Analysis

**Delivery**: auto-chain; stacked-to-main; PR 2; maintainer-approved `size:exception` up to 800 changed lines.

## Cumulative Completed Tasks

- [x] 1.1 RED: Test equal, lower, skipped, dirty, and canonical-root safety boundaries.
- [x] 1.2 GREEN: Enforce immediate-next core-major targets and successful equal-major no-ops.
- [x] 1.3 RED: Test PHP 8.4 numeric matrix selection and the unknown Drupal 99 error.
- [x] 1.4 GREEN: Compare matrix Drupal versions numerically.
- [x] 2.1 RED: Capture Upgrade Status preparation command sequences.
- [x] 2.2 GREEN: Add guarded `prepare_upgrade_status`.
- [x] 2.3 RED: Cover read-only scan and upgrade-scan prerequisites.
- [x] 2.4 GREEN: Make scan-family handlers analysis-only and register preparation metadata.
- [x] 2.5 RED: Cover read-only validate responses and evidence hashes.
- [x] 2.6 GREEN: Require prepared Upgrade Status before validation.
- [x] 2.7 RED: Prove autofix does not rescan.
- [x] 2.8 GREEN: Remove the autofix rescan.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.1 | `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | `TestRealHandlePrepareUpgradeStatus` was written before the preparation handler in the prior blocked attempt. | `go test ./internal/app -run TestRealHandlePrepareUpgradeStatus -count=1 -v` — 3/3 subtests passed. | Uninstalled, disabled-with-conflict, and already-enabled paths. | Removed obsolete unreachable assertions; `go vet ./...` passes. |
| 2.2 | `internal/app/mcp_tools.go`, `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | Covered by task 2.1 RED. | Same focused command — pass. | Install, conflict removal, enablement, cache rebuild, and enabled no-op are covered. | None needed. |
| 2.3 | `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | `TestReadOnlyScansRequirePreparedUpgradeStatus` was written before prerequisite enforcement in the prior blocked attempt. | Focused read-only suite — 5/5 subtests passed. | Prepared scan/upgrade-scan; disabled/conflicting and missing-module refusal; invalid path assertion. | None needed. |
| 2.4 | `internal/app/mcp_tools_test.go`, `internal/mcp/mcp_test.go` | Integration | `go test ./internal/app ./internal/mcp -count=1` — pass | Covered by task 2.3 RED and `TestPrepareUpgradeStatusHasSchemaAndStub`. | Focused app and MCP commands — pass. | Prepared versus unprepared handlers and schema/stub registration. | None needed. |
| 2.5 | `internal/app/mcp_tools_test.go`, `internal/app/commands_test.go` | Integration | `go test ./internal/app -count=1` — pass | `TestValidateIsReadOnlyAndRequiresPreparedUpgradeStatus` was written before the read-only validation route in the prior blocked attempt. | Focused read-only suite — 4/4 subtests passed. | Zero findings, all findings, module filtering, and unprepared refusal; evidence hash asserted. | None needed. |
| 2.6 | `internal/app/commands.go`, `internal/app/mcp_tools.go` | Integration | `go test ./internal/app -count=1` — pass | Covered by task 2.5 RED. | Focused read-only suite — pass. | Drupal 11 fixture verifies analysis without `updb`, cache rebuild, enablement, or config change. | None needed. |
| 2.7 | `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | `TestAutofixDoesNotRescan` was written before removing the rescan in the prior blocked attempt. | `go test ./internal/app -run TestAutofixDoesNotRescan -count=1 -v` — 1/1 passed. | Rector summary is returned while the forbidden Drush path remains absent. | None needed. |
| 2.8 | `internal/app/mcp_tools.go`, `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | Covered by task 2.7 RED. | Same focused command — pass. | Result excludes stale `remaining_errors` and all Drush analysis calls. | None needed. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/app -run 'Test(RealHandlePrepareUpgradeStatus|ReadOnlyScansRequirePreparedUpgradeStatus|ValidateIsReadOnlyAndRequiresPreparedUpgradeStatus|AutofixDoesNotRescan|RealHandleUpgradeScan_RequiresPreparationBeforeConfigMutation|RealHandleScan_RequiresPreparationBeforeMutation)' -count=1 -v` — exit 0; 15 named scenarios passed. `go test ./internal/mcp -run TestPrepareUpgradeStatusHasSchemaAndStub -count=1 -v` — exit 0; 1/1 passed. |
| Runtime harness command/scenario and exact result | N/A — no external Drupal runtime is available for this Go unit. `t.TempDir()` projects plus captured `RunWithEnv` calls exercise the real handler boundaries and assert preparation command ordering or absence of Composer/config/enable/cache/update commands. |
| Independent verification | `go test ./... -count=1`, `go vet ./...`, `gofmt -w` on modified Go files, and `git diff --check` — all exit 0. |
| Rollback boundary | Revert only `internal/app/commands.go`, `internal/app/commands_test.go`, `internal/app/mcp_tools.go`, `internal/app/mcp_tools_test.go`, `internal/mcp/server.go`, `internal/mcp/tools.go`, and `internal/mcp/mcp_test.go`; this removes preparation/read-only analysis behavior without touching the Phase 3 retry policy. |

### Native Settlement

- Attempt token: `sha256:f4bf1094f6ffc2896c4e2d9ed5d57838c13b7b2056b474847b6fd92b7050f285`
- Request ID: `apply-readonly-analysis-approved-20260826-01-settle`
- Outcome: `passed`
- Evidence revision: `sha256:3b190740a44650956fe095a07ea13ea605072d7272c4f8b62455445a44498e12`
- Remediates failed evidence revision: `sha256:bd9aad593ae0ac91470865a814d4779ab0e78867e7701509d876aad465747cf6`
- Settlement changed lines: 66 of the maintainer-authorized 800-line exception.

## Test Summary

- **Total tests written**: 15 focused scenarios across preparation, scan-family, validation, and autofix behavior, plus one MCP schema/stub test.
- **Total tests passing**: 16 focused scenarios; all packages pass under `go test ./... -count=1`.
- **Layers used**: Integration (16); E2E (0 — no harness exists).
- **Approval tests**: None — behavior changes were specified.
- **Pure functions created**: 0.

## Deviations and Issues

None — implementation matches the assigned design and scope. Two obsolete test branches became unreachable after the read-only contract changed; they were removed before the final `go vet` evidence.

## Phase 2 Gate Correction: Preparation Registration Guard

`prepare_upgrade_status` is now in `session.RefuseOnlyTools`, so its existing `guardHandler` registration executes the complete mutating guard chain: kill switch, matching session, backup freshness, mutation cap, and audit recording.

### Cumulative Completed Tasks

- [x] 1.1 RED: Test equal, lower, skipped, dirty, and canonical-root safety boundaries.
- [x] 1.2 GREEN: Enforce immediate-next core-major targets and successful equal-major no-ops.
- [x] 1.3 RED: Test PHP 8.4 numeric matrix selection and the unknown Drupal 99 error.
- [x] 1.4 GREEN: Compare matrix Drupal versions numerically.
- [x] 2.1 RED: Capture Upgrade Status preparation command sequences.
- [x] 2.2 GREEN: Add guarded `prepare_upgrade_status`.
- [x] 2.3 RED: Cover read-only scan and upgrade-scan prerequisites.
- [x] 2.4 GREEN: Make scan-family handlers analysis-only and register preparation metadata.
- [x] 2.5 RED: Cover read-only validate responses and evidence hashes.
- [x] 2.6 GREEN: Require prepared Upgrade Status before validation.
- [x] 2.7 RED: Prove autofix does not rescan.
- [x] 2.8 GREEN: Remove the autofix rescan.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.2 gate correction | `internal/app/mcp_tools_test.go`, `internal/session/session.go` | Integration | `go test ./internal/app -count=1` and `go test ./internal/session -count=1` — pass. | `TestWireMCPTools_PrepareUpgradeStatusRefusesWithoutSession` failed before the partition update: `install upgrade_status failed` instead of `session_open` guidance. | `go test ./internal/app -run 'TestWireMCPTools_PrepareUpgradeStatus(RefusesWithoutSession|HonorsKillSwitch|RequiresBackupAndAuditsRefusal)' -count=1 -v` — exit 0; 3/3 passed. | Added kill-switch and matching-session-without-backup cases; the latter asserts a `prepare_upgrade_status` failure audit record. | `gofmt -w internal/session/session.go internal/app/mcp_tools_test.go` followed by focused tests — pass. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/app -run 'TestWireMCPTools_(RefuseOnlyToolsRefuseWithoutSession|PrepareUpgradeStatus(RefusesWithoutSession|HonorsKillSwitch|RequiresBackupAndAuditsRefusal))' -count=1 -v` — exit 0; 4 parent tests passed, including 8 refuse-only registrations. `go test ./internal/session -run 'Test(EvaluateGuard_RefuseOnlyPartitionWithoutSession|EvaluateBackupFreshness_FreshBackupAllows|GuardedTools_IsUnionOfBothPartitions)' -count=1 -v` — exit 0; 3 parent tests passed. |
| Runtime harness command/scenario and exact result | The focused `server.CallTool("prepare_upgrade_status", ...)` integration scenarios passed through real MCP registration and `guardHandler`: no session returns `session_open`, the kill switch returns `DRUP_DISABLE_MUTATIONS`, and a matching session without a backup returns `test_backup_create` while auditing the refusal. |
| Rollback boundary | Revert `internal/session/session.go` and the three registration-guard cases in `internal/app/mcp_tools_test.go`; this reverts only the Phase 2 preparation guard correction. |

### Native Settlement

- Attempt token: `sha256:0bcdcc1f776cc213938ee4e52a0c6f3faf2a4ddb2d6008187e468a341a8b03ae`
- Request ID: `apply-preparation-guard-correction-20260826-01-settle`
- Outcome: `passed`
- Quality checks: `go vet ./internal/app ./internal/session` and `git diff --check` — exit 0.

## Phase 2 Cap-Refusal Correction Promotion

The user-authorized correction adds the registration-level mutation-cap refusal proof for `prepare_upgrade_status`; no production source changed during promotion.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 2.2 cap-refusal correction | `internal/app/mcp_tools_test.go` | Integration | `go test ./internal/app -count=1` — pass | `TestWireMCPTools_PrepareUpgradeStatusRefusesAtMutationCap` failed before the registration guard correction. | `go test ./internal/app -run 'TestWireMCPTools_(RefuseOnlyToolsRefuseWithoutSession|PrepareUpgradeStatus(RefusesWithoutSession|HonorsKillSwitch|RequiresBackupAndAuditsRefusal|RefusesAtMutationCap))' -count=1 -v` — exit 0; 5 parent tests passed. | Session absence, kill switch, missing backup with audited refusal, and the one-of-one mutation-cap refusal cover distinct guard outcomes. | None needed; the test-only promotion did not alter production behavior. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/app -run 'TestWireMCPTools_(RefuseOnlyToolsRefuseWithoutSession|PrepareUpgradeStatus(RefusesWithoutSession|HonorsKillSwitch|RequiresBackupAndAuditsRefusal|RefusesAtMutationCap))' -count=1 -v` — exit 0; 5 parent tests passed. `go test ./internal/session -run 'Test(EvaluateGuard_RefuseOnlyPartitionWithoutSession|EvaluateBackupFreshness_FreshBackupAllows|GuardedTools_IsUnionOfBothPartitions)' -count=1 -v` — exit 0; 3 parent tests passed. |
| Runtime harness command/scenario and exact result | Real `server.CallTool("prepare_upgrade_status", ...)` registration path passed: a one-of-one consumed mutation cap returns `mutation cap reached (1/1)` and writes an audited failure record. |
| Package validation | `go test ./internal/app ./internal/session -count=1`, `go vet ./internal/app ./internal/session`, and `git diff --check` — all exit 0. |
| Rollback boundary | Revert only `internal/session/session.go` and `TestWireMCPTools_PrepareUpgradeStatusRefusesAtMutationCap` in `internal/app/mcp_tools_test.go`; this removes only the Phase 2 cap-refusal correction. |

### Promotion Boundary

- Authorized attempt token: `sha256:59b56ae15056512e0e386830fc6d9c5b3f678a49a5e87d7e8c931a677d941a7b`
- Changed-line budget: 108 lines
- Scope: Phase 2 cap-refusal correction only; Unit 3 remains pending.

## Work Unit 1 Correction: Immediate Next-Major Check

`NextMajor` now rejects a release feed whose latest major skips the required immediate major. The error explicitly directs the caller to upgrade through the missing major rather than returning an unsafe target.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1/1.2 correction | `internal/coreupgrade/check_test.go` | Unit | `go test ./internal/coreupgrade -count=1` — pass | `TestNextMajor_OnlyOffersImmediateNextMajor` failed: a Drupal 10 project was offered 12.0.0. | `go test ./internal/coreupgrade -run 'TestNextMajor_(Available|OnlyOffersImmediateNextMajor|AlreadyOnLatest|InvalidCurrentVersion)' -count=1` — pass. | Added Drupal 11 → 13 coverage; both skipped-major cases return actionable errors. | None needed; one immediate-major comparison is the minimal shared guard. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/coreupgrade -run 'TestNextMajor_(Available|OnlyOffersImmediateNextMajor|AlreadyOnLatest|InvalidCurrentVersion)' -count=1` — pass; `go test ./internal/coreupgrade -run TestApply_RejectsUnsafeTargetsAndNoOpsAtCurrentMajor -count=1` — pass. |
| Runtime harness command/scenario and exact result | N/A — `NextMajor` is a pure read-only release-selection boundary; the existing `t.TempDir()` Git harness proves equal/lower/skipped apply paths leave `composer.json` and checkpoint count unchanged. |
| Rollback boundary | Revert `internal/coreupgrade/check.go` and `internal/coreupgrade/check_test.go`; this removes only immediate-release feed refusal and its proof. |

## Work Unit 3: MCP Retry Allowlist

**Delivery**: auto-chain; stacked-to-main; PR 3 — MCP retry allowlist.

### Cumulative Completed Tasks

- [x] 1.1–1.4 Core gate and numeric matrix.
- [x] 2.1–2.8 Explicit preparation and read-only analysis.
- [x] 3.1 RED: Prove retry eligibility, exhaustion, mutator single-call, and command-not-found behavior.
- [x] 3.2 GREEN: Retry only `scan`, `upgrade_scan`, and `validate`, preserving retry-loop backoff and metrics.
- [x] 3.3 Run complete Go tests, vet, formatting, and diff validation.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 3.1 | `internal/mcp/mcp_test.go` | Integration | `go test ./internal/mcp -count=1` — pass before edits. | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` — failed before production change: `autofix` invoked three times and recorded two retries instead of one call and zero retries. | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` — exit 0; all four scenarios passed. | Scan succeeds after two transient failures; validate exhausts at three calls/two metrics; mutator and command-not-found paths each remain single-call. | Existing direct `retryLoop` tests were moved to the retry-loop boundary so they continue proving backoff and metrics independently of dispatch eligibility; focused tests passed. |
| 3.2 | `internal/mcp/server.go`, `internal/mcp/mcp_test.go` | Integration | Same MCP package baseline — pass. | Covered by task 3.1. | Same focused command — exit 0; the named allowlist dispatch passed all cases. | Read-only success/exhaustion and non-retry error paths cover distinct dispatch outcomes. | `switch` is the minimal local allowlist; no abstraction added. |
| 3.3 | `internal/mcp/mcp_test.go` | Verification | `go test ./internal/mcp -count=1` — pass. | N/A — verification-only task; task 3.1 supplied the required RED behavior test. | `go test ./... -count=1`, `go vet ./...`, `gofmt -w` on all modified Go files, and `git diff --check` — all exit 0. | Full suite includes the retry, scan, validate, mutator, command-not-found, preparation, and core threat tests. | None needed. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` — exit 0; 4/4 named scenarios passed. `go test ./internal/mcp -count=1` — exit 0. |
| Runtime harness command/scenario and exact result | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` drives real JSON-RPC `handleRequest` dispatch with stubbed handlers; scan retries twice and succeeds, validate retries twice then fails, `autofix` is invoked once, and `upgrade_scan` with `command not found` is invoked once. Exit 0. |
| Full checks | `gofmt -w` on all modified Go files; `go test ./... -count=1`; `go vet ./...`; and `git diff --check` — all exit 0. |
| Rollback boundary | Revert the retry dispatch `switch` in `internal/mcp/server.go` and the retry tests in `internal/mcp/mcp_test.go`; this restores the prior transport retry policy without altering prior preparation/schema work. |

### Test Summary

- **Total tests written**: 4 dispatch scenarios in one table-driven integration test.
- **Total tests passing**: 4 new scenarios; all packages pass under `go test ./... -count=1`.
- **Layers used**: Integration (4); E2E (0 — no harness exists).
- **Approval tests**: 3 direct `retryLoop` regression tests retained while dispatch policy changed.
- **Pure functions created**: 0.

### Deviations and Issues

Implementation matches the retry allowlist design. Native settlement recorded passing evidence but requires a maintainer decision because the measured work-unit diff is 183 changed lines, three above the 180-line budget. No commit, push, or pull request was created.

### Native Settlement

- Attempt token: `sha256:a3c36ee62736d86d3c7a78e1cfc9e452e81ab08cc4c9723f9025cb3d5cc794bd`
- Request ID: `apply-retry-policy-20260826-01-settle`
- Outcome: `passed` with evidence revision `sha256:c31d71b00101a0f741b3f7804b6a4387ad2fdc2b50f87ec4d8676728bc697238`.
- Native gate: `blocked: maintainer_decision`; measured 183 changed lines against the 180-line work-unit budget.
- Next native action: maintainer reset/authorization is required; settlement must not be retried.

## Work Unit 3 Promotion: User-Authorized Retry Policy

The maintainer authorized promotion of the already implemented retry-policy unit at its measured 183-line limit. This promotion performs no source changes, commits, pushes, or pull-request actions.

### Cumulative Completed Tasks

- [x] 1.1–1.4 Core gate and numeric matrix.
- [x] 2.1–2.8 Explicit preparation and read-only analysis.
- [x] 3.1 RED: Prove retry eligibility, exhaustion, mutator single-call, and command-not-found behavior.
- [x] 3.2 GREEN: Retry only `scan`, `upgrade_scan`, and `validate`, preserving retry-loop backoff and metrics.
- [x] 3.3 Run complete Go tests, vet, formatting, and diff validation.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 3.1 | `internal/mcp/mcp_test.go` | Integration | Existing MCP package baseline passed before the original edits. | The table-driven dispatch test failed before the allowlist: `autofix` retried three times. | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` — exit 0; 4/4 scenarios passed during promotion. | Scan success after two transient failures, validate exhaustion, mutator single-call, and command-not-found refusal cover distinct outcomes. | Existing retry-loop tests retain backoff and retry-metric coverage; no new refactor was needed. |
| 3.2 | `internal/mcp/server.go`, `internal/mcp/mcp_test.go` | Integration | Same MCP baseline. | Covered by task 3.1 RED. | Same focused command — exit 0. | The explicit read-only allowlist and three non-success paths remain covered. | The local `switch` remains the minimal implementation. |
| 3.3 | `internal/mcp/mcp_test.go` | Verification | `go test ./internal/mcp -count=1` passed in the completed unit. | N/A — verification-only task. | `go test ./... -count=1`, `go vet ./...`, formatting check, and `git diff --check` — exit 0 during promotion. | The full suite exercises the retry and adjacent safety boundaries. | None needed. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | `go test ./internal/mcp -run TestHandleToolCall_RetriesOnlyReadOnlyTools -count=1 -v` — exit 0; 4/4 named scenarios passed. |
| Runtime harness command/scenario and exact result | The focused test drives real JSON-RPC `handleRequest` dispatch using stubbed handlers: `scan` retries twice then succeeds, `validate` exhausts after three calls with two retry records, `autofix` runs once, and `upgrade_scan` returns `command not found` after one call. Exit 0. |
| Full validation | `go test ./... -count=1`, `go vet ./...`, `gofmt -d internal/mcp/server.go internal/mcp/mcp_test.go` (no output), and `git diff --check` — all exit 0. |
| Rollback boundary | Revert only the retry dispatch `switch` in `internal/mcp/server.go` and retry-policy tests in `internal/mcp/mcp_test.go`; this restores the prior retry policy without affecting the earlier work units. |

### Promotion Boundary

- Delivery: stacked-to-main PR 3, maintainer-authorized `size:exception`.
- Authorized work unit: `retry-policy` only.
- Source changes during promotion: 0.
- Authorized maximum: 183 changed lines; the completed unit measures exactly 183 changed lines.
- No commit, push, or pull request was created.

## Final Contract Corrections

**Delivery**: focused remediation, `final-contract-corrections`; maximum 80 changed lines.

The core apply path now writes the immediate-major constraint as `^11`. The validate MCP handler, schema, and stub accept and expose `module_name` only, matching the delta specification.

### TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| Core constraint correction | `internal/coreupgrade/apply_test.go` | Integration | `go test ./internal/coreupgrade -run 'TestApply_(ChecksClean_CreatesCheckpoint_AndMutates|ResolvesSymlinkedProjectPath)' -count=1 -v` — 2/2 passed before the RED edit. | ✅ Written; the two assertions failed because the output was `^11.0`, not `^11`. | ✅ Passed; the same focused command passed 2/2 after the minimal constraint change. | ✅ Two mutation paths: direct and symlink-resolved projects both write `^11`. | ➖ None needed; a one-token format correction is clear. |
| Validate module contract correction | `internal/app/mcp_tools_test.go`, `internal/mcp/mcp_test.go` | Integration | Focused app validation scenarios (6) and MCP schema scenarios (4) passed before the RED edits. | ✅ Written; `module_name` analyzed `--all`, and the schema lacked `module_name` while exposing `module`. | ✅ Passed; `go test ./internal/app -run 'Test(RealHandleValidate_AcceptsModuleNameWhenSet|ValidateIsReadOnlyAndRequiresPreparedUpgradeStatus)' -count=1 -v` passed 5 subtests and `go test ./internal/mcp -run TestValidateSchema_ExposesModuleName -count=1 -v` passed 1/1. | ✅ Full-project, scoped-module, and unprepared validation paths plus schema absence of legacy `module`. | ➖ None needed; JSON tags and schema use the specified name directly. |

### Work Unit Evidence

| Evidence | Result |
|----------|--------|
| Focused test command and exact result | Core focused command passed 2/2. App focused command passed one direct scoped-module test plus four read-only subtests. MCP schema command passed 1/1. |
| Runtime harness command/scenario and exact result | `t.TempDir()` Git fixtures exercised direct and symlink core writes; mocked prepared Drupal handlers verified `module_name` reaches `drush upgrade_status:analyze` without mutation. All scenarios passed. |
| Full checks | `go test ./... -count=1`, `go vet ./...`, `gofmt -w` on the seven touched Go files, and `git diff --check` — exit 0. |
| Rollback boundary | Revert only the `^%d` constraint format in `internal/coreupgrade/apply.go`, its two assertions, validate `module_name` tags/schema/stub, and the scoped-module/schema tests. |

```yaml
schema: gentle-ai.remediation-result/v1
lineage_id: sha256:193d6839aac19bf75c6e2da8bd34aef921b926240bddebf25a563c07431b5919
generation: 10
fix_batch: final-contract-corrections
failed_evidence_revision: sha256:0ca25dd950b381ee199ecb4c2e9bf1e6936346b9295a725763004de82b118950
evidence_revision: sha256:8ad0b57bed1830503cdf8e2e2bf6725dc49f79e0bd3d4be5e7d4afe76808ad18
outcome: passed
```
```json
{"schema":"gentle-ai.remediation-evidence/v1","lineage_id":"sha256:193d6839aac19bf75c6e2da8bd34aef921b926240bddebf25a563c07431b5919","generation":10,"fix_batch":"final-contract-corrections","failed_evidence_revision":"sha256:0ca25dd950b381ee199ecb4c2e9bf1e6936346b9295a725763004de82b118950","evidence_revision":"sha256:8ad0b57bed1830503cdf8e2e2bf6725dc49f79e0bd3d4be5e7d4afe76808ad18","focused_checks":"pass","full_checks":"pass"}
```
