# Design: Upgrade Workflow Safety Foundation

## Technical Approach

Create guarded `prepare_upgrade_status`. Make `scan`, `upgrade_scan`, and `validate` analysis-only; validate core targets before `coreupgrade.Apply` writes or invokes Git; parse matrix components numerically; and retry only read-only handlers.

## Architecture Decisions

| Decision | Choice | Alternative | Rationale |
|---|---|---|---|
| Core-major gate | Read current core major and accept only equal or `current+1`; equal is a success no-op. | Trust caller target. | Rejecting lower/skipped targets before checkpoint, write, or subprocess prevents mutation. |
| Preparation boundary | Register guarded `prepare_upgrade_status`; it installs, removes `update.settings` conflict, enables, and rebuilds cache. | Keep preparation in scan handlers. | A named mutator separates setup from retryable analysis. |
| Read-only analysis | Check Composer presence and `pm:list` enabled state, then analyze; report `prepare_upgrade_status` guidance when unprepared. | Auto-enable or retain D11 `updb`/`cr`. | The forbidden commands mutate Drupal state. |
| Retry policy | `handleToolCall` uses `retryLoop` only for `scan`, `upgrade_scan`, and `validate`. | Retry all tools or add generic metadata. | A local allowlist prevents mutator replay without new abstraction. |

## Data Flow

```text
Client -> prepare_upgrade_status -> guarded Composer/config/en/cr -> prepared project
Client -> scan|upgrade_scan|validate -> presence + pm:list -> analyze -> parsed response
Client -> core_upgrade_apply -> canonical root -> major gate -> no-op | checkpoint/write
Client -> MCP dispatch -> read-only allowlist -> retryLoop -> handler/envelope
```

## Requirement Traceability

| Requirement/scenario | Design boundary | RED test boundary |
|---|---|---|
| Scan: prepared, disabled, and missing module | `realHandleScan` and `realHandleUpgradeScan` check absolute project path, Composer package, and enabled `pm:list`; only then issue `drush upgrade_status:analyze --all`. | Captured command tests prove prepared paths run only analysis; disabled/missing paths return preparation guidance and issue no Composer/config/en/cr command; invalid scan path returns path-not-found. |
| Validate: full result and optional module | `DoValidate` always uses read-only Upgrade Status analysis; `realHandleValidate` returns filtered findings and evidence hash. | Table-driven tests prove zero/all findings response and module target/filtering; assert no `updb`, `cr`, install, enable, or config commands. |
| Matrix: PHP 8.4 and unknown Drupal version | `realHandleDrupalVersionMatrix` parses Drupal/PHP components with `semver.Parse` before selecting the highest compatible entry. | Table tests cover PHP 8.4 numeric selection and `drupal_version: "99"` returning `unknown Drupal version: 99`. |
| Retry: eligibility, backoff, and recording | `handleToolCall` bypasses `retryLoop` for mutators; `retryLoop` retains three attempts, 1s exponential backoff, and `metrics.Default().RecordRetry()`. | Stubbed tests prove two transient retries are recorded before success/exhaustion, one mutator call, and no retry for `command not found`. |
| Preparation: enabled-module no-op | Preparation checks Composer and `pm:list` before mutations. | Captured command test proves an enabled module neither reinstalls nor runs config delete, enable, or cache rebuild. |

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/coreupgrade/{check,apply}.go` | Modify | Immediate-major/no-op gate before mutation. |
| `internal/coreupgrade/*_test.go` | Modify | Table-driven target and no-mutation cases. |
| `internal/app/mcp_tools.go` | Modify | Preparation handler, read-only analysis, numeric matrix, and no autofix rescan. |
| `internal/app/{commands,guard,mcp_tools}_test.go` | Modify | Command-sequence RED tests. |
| `internal/mcp/{server,tools}.go` | Modify | Preparation schema/stub and selective retry dispatch. |
| `internal/mcp/mcp_test.go` | Modify | Wiring, retry eligibility, backoff, and recording tests. |

## Interfaces / Contracts

`prepare_upgrade_status({project_path})` is the only domain operation that may install/enable Upgrade Status or modify `update.settings`; it uses existing guard middleware. Analysis operations return actionable preparation guidance without mutation. `validate` accepts optional module filtering and returns `{total_errors, errors, evidence_hash}`. Matrix lookup preserves the unknown-version error contract.

## Testing Strategy

| Layer | What to test | Approach |
|---|---|---|
| Unit | Gates, matrix, retry, filtered results | Table-driven Go tests and command seams. |
| Integration | Preparation and analysis command sequences | `t.TempDir()` fixtures with captured `RunWithEnv` calls. |
| E2E | N/A | No E2E harness exists. |

## Threat Matrix

| Boundary | Applicability | Safe / failure behavior | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A — no executable-file classification change. | N/A | N/A |
| Git repository selection | Applicable — core apply uses `git -C` after canonical root resolution. | Resolved absolute root only; relative/traversal fails before Git. | Canonical root succeeds; relative and `..` make no Git call. |
| Commit state | Applicable — target gate precedes checkpoint commit. | Equal/lower/skipped target makes no write/commit; immediate target retains checkpoint rules. | Equal, lower, skipped, and dirty cases assert no checkpoint/mutation. |
| Push state | N/A — no push/ref resolution. | N/A | N/A |
| PR commands | N/A — no PR command composition. | N/A | N/A |

## Migration / Rollout

No migration required. Deliver autonomous slices: core gate/matrix, preparation/read-only analysis, then retry/wiring tests; each remains within the 800-line review budget.

## Open Questions

- [ ] None.
