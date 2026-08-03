# Delta Spec: drup-retrospective-fixes

## Purpose

Delta spec addressing the P0/P1 failures from the 2026-08-03 retrospective. Modifies two existing spec domains (`mcp-server`, `scan`) and adds cross-cutting requirements for sub-agent prompt contracts and test assertions.

**Priority adjustments from approved proposal** (user-mandated):
1. **MCP contract extension**: tool errors are sent as `{"status":"fail","summary":"..."}` in the result channel, NOT as JSON-RPC errors. This is a deliberate protocol extension — documented explicitly in REQ-2.
2. **REQ-5 promoted to P1**: sub-agent templates reading `result.payload` is required for the envelope wrapper to not break sub-agent parsing. Moved from P2 (proposal §What Changes / 5) to P1 in this spec.

Implements: `proposal.md` (approved), `explore.md` §4 (MCP envelope audit), `RETROSPECTIVE.md`.

---

## ADDED Requirements

### REQ-1: `realHandleScan` auto-enables `upgrade_status`

**Priority**: P0
**Implements**: proposal.md §What Changes / 1
**Modifies domain**: `openspec/specs/scan/spec.md` (Drush Invocation requirement)

`realHandleScan` (`internal/app/mcp_tools.go:152-179`) currently runs `drush upgrade_status:analyze` without checking whether `upgrade_status` is enabled. When the module is present in the filesystem but not enabled, drush returns exit 1 with "There are no commands defined in the upgrade_status namespace." `realHandleUpgradeScan` (lines 777-917) already handles this correctly. `realHandleScan` MUST be brought in line.

The system SHALL check `pm:list --status=enabled` for `upgrade_status` before running `upgrade_status:analyze`. If not enabled, it SHALL run `config:delete update.settings` (to clear any pre-existing config conflict), then `drush en upgrade_status -y`, then `drush cr`, then proceed with the scan. If `upgrade_status` is not in `composer.json`, the tool SHALL fail with a clear error and SHALL NOT silently install it.

| Req | Strength | Behavior |
|-----|----------|----------|
| Pre-scan enable check | MUST | Check `pm:list --status=enabled` for `upgrade_status` before `upgrade_status:analyze` |
| Auto-enable sequence | MUST | `config:delete update.settings` → `drush en upgrade_status -y` → `drush cr` |
| Missing from composer.json | MUST | Fail with clear error; do NOT silently install |
| Already enabled | MUST | Skip auto-enable, run `upgrade_status:analyze` directly |
| `realHandleUpgradeScan` unchanged | MUST NOT | Do NOT modify `realHandleUpgradeScan` (lines 777-917); it already handles this correctly |

#### Scenario: Auto-enable when installed but disabled (happy path)

- GIVEN a project where `upgrade_status` is in `composer.json` and filesystem but NOT in `pm:list --status=enabled`
- WHEN `drup_scan` is invoked
- THEN the system SHALL run the auto-enable sequence and complete `upgrade_status:analyze` without "no commands defined" error
- AND the response SHALL contain the scan results

#### Scenario: Already enabled — skip auto-enable

- GIVEN a project where `upgrade_status` is already enabled
- WHEN `drup_scan` is invoked
- THEN the system SHALL run `upgrade_status:analyze` directly without re-running enable steps

#### Scenario: Not in composer.json — clear error

- GIVEN a project where `upgrade_status` is NOT in `composer.json`
- WHEN `drup_scan` is invoked
- THEN the system SHALL return `{"status":"fail","summary":"upgrade_status is not installed; run composer require drupal/upgrade_status first"}` and SHALL NOT attempt installation

#### Scenario: TestRealHandleScan_AutoEnablesUpgradeStatus

- GIVEN a mock project in the "installed but disabled" state
- WHEN `TestRealHandleScan_AutoEnablesUpgradeStatus` runs
- THEN the test SHALL assert the scan completes without error
- AND the test SHALL fail when the auto-enable block is removed (regression guard)

---

### REQ-2: Uniform response envelope at MCP server level

**Priority**: P0
**Implements**: proposal.md §What Changes / 2
**Modifies domain**: `openspec/specs/mcp-server/spec.md` (MCP Server Transport + all tool requirements)

This is the centerpiece of the retrospective fix. The orchestrator model (mimo-v2.5) had no parseable signal for tool success/failure because each of the 29 tools returns a different JSON shape. The fix is a single server-level wrapper in `handleToolCall` (`internal/mcp/server.go:408-425`) that adds a uniform envelope to every response.

The system SHALL wrap every MCP tool response (success or error) in a uniform envelope: `{"status":"pass|fail","summary":"...","payload":{...}}`. The wrapper lives in `handleToolCall` (one location); per-handler code is NOT modified. The original tool-specific payload is preserved intact inside `envelope.payload`.

**MCP protocol extension (deliberate)**: Errors are sent as `{"status":"fail","summary":"..."}` in the result channel, NOT as JSON-RPC errors. This is a deliberate extension of the MCP protocol for this server. Rationale: the retrospective's core complaint was that errors were invisible. JSON-RPC errors are transport-level signals that some MCP clients swallow silently. A `{"status":"fail"}` in the result channel is always visible to the orchestrator model. Clients that need JSON-RPC error semantics can check for the absence of the `status` field.

| Req | Strength | Behavior |
|-----|----------|----------|
| Envelope wrap (success) | MUST | `{"status":"pass","summary":"...","payload":{original}}` |
| Envelope wrap (error) | MUST | `{"status":"fail","summary":"<error message>"}` in result channel |
| Error transport | MUST NOT | Do NOT send errors as JSON-RPC errors; send as `{"status":"fail"}` in result |
| Payload preservation | MUST | Original tool payload byte-for-byte inside `envelope.payload` |
| Single wrapper location | MUST | Wrapper in `handleToolCall` only; per-handler code unchanged |
| `deriveSummary` helper | SHOULD | Best-effort summary: extract `summary`/`success` fields if present, else generic "Tool {name} completed" |
| Protocol extension documented | MUST | SKILL.md and sub-agent templates document this as an intentional MCP protocol extension |

#### Scenario: Success envelope wrap

- GIVEN a handler returns `{"foo":"bar"}` (no error)
- WHEN `handleToolCall` processes the response
- THEN the client SHALL receive `{"status":"pass","summary":"...","payload":{"foo":"bar"}}`
- AND `payload.foo` SHALL equal `"bar"`

#### Scenario: Error envelope — NOT a JSON-RPC error

- GIVEN a handler returns `(nil, error("command not found: xyz"))`
- WHEN `handleToolCall` processes the response
- THEN the client SHALL receive `{"status":"fail","summary":"command not found: xyz"}` in the result channel
- AND the response SHALL NOT be a JSON-RPC error (no `error` field at JSON-RPC level)
- AND the JSON-RPC `id` SHALL match the request `id`

#### Scenario: Payload preservation — complex object

- GIVEN a handler returns a complex `scan.ScanResult` with nested `errors.contrib`, `errors.custom`, `errors.theme` arrays
- WHEN `handleToolCall` wraps the response
- THEN `envelope.payload` SHALL contain the full `scan.ScanResult` byte-for-byte
- AND `envelope.payload.errors.contrib` SHALL be accessible without additional parsing

#### Scenario: `deriveSummary` with `success` field

- GIVEN a handler returns `{"success":true,"output":"..."}`
- WHEN `deriveSummary` runs
- THEN the summary SHALL be "Tool {name} succeeded" or equivalent

#### Scenario: `deriveSummary` with `total_errors` field

- GIVEN a handler returns `{"total_errors":0,"errors":[]}`
- WHEN `deriveSummary` runs
- THEN the summary SHALL be "Scan complete: 0 errors" or equivalent

#### Scenario: `deriveSummary` fallback

- GIVEN a handler returns a payload with no recognized fields
- WHEN `deriveSummary` runs
- THEN the summary SHALL be "Tool {name} completed"

#### Scenario: TestHandleToolCall_EnvelopeWrap_Success

- GIVEN a mock handler returning `{"foo":"bar"}`
- WHEN `TestHandleToolCall_EnvelopeWrap_Success` runs
- THEN the test SHALL assert the response contains `status:"pass"`, `summary`, and `payload.foo:"bar"`

#### Scenario: TestHandleToolCall_EnvelopeWrap_Error

- GIVEN a mock handler returning `(nil, error("test error"))`
- WHEN `TestHandleToolCall_EnvelopeWrap_Error` runs
- THEN the test SHALL assert the response is `{"status":"fail","summary":"test error"}` in the result channel
- AND the test SHALL assert the response is NOT a JSON-RPC error

#### Scenario: TestHandleToolCall_PayloadIntact

- GIVEN a mock handler returning a complex nested payload
- WHEN `TestHandleToolCall_PayloadIntact` runs
- THEN the test SHALL assert `envelope.payload` matches the original payload byte-for-byte

---

### REQ-3: Selective retry for transient MCP tool errors

**Priority**: P1
**Implements**: proposal.md §What Changes / 3
**Modifies domain**: `openspec/specs/mcp-server/spec.md` (alongside REQ-2)

The retrospective documents `drup_test_backup_list` timing out with "Request timed out". Blanket retry would turn "module not found" into 3× "module not found" over 3 seconds. The system SHALL retry only on transport/timeout errors, never on logic errors (exit code ≠ 0, "command not found", etc.).

The system SHALL wrap `handler(p.Arguments)` with retry logic for transient errors only. Reuse the `doWithRetry` pattern from `internal/drupalorg/drupalorg.go:31-80` but with a different policy: max 2 retries, 1s base exponential backoff.

| Req | Strength | Behavior |
|-----|----------|----------|
| Retry on transient errors | MUST | Retry on: "context deadline exceeded", "connection refused", "i/o timeout", "broken pipe", "timeout" |
| No retry on logic errors | MUST NOT | Do NOT retry on: "exit status", "command not found", "no commands defined", "already exists", "does not exist" |
| Max retries | MUST | 2 retries (3 total attempts) |
| Backoff | MUST | 1s base exponential backoff (1s, 2s) |
| Metrics recording | MUST | Record each retry via `metrics.Collector.RecordRetry()` |
| `isTransientError` helper | MUST | Helper function classifying errors by string matching |
| Retry exhausted | MUST | Return `{"status":"fail","summary":"... after 3 attempts ..."}` |

#### Scenario: Retry on transient error — succeeds after 2 failures

- GIVEN a handler fails with "context deadline exceeded" on attempts 1 and 2, succeeds on attempt 3
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"pass",...}`
- AND `metrics.Collector` SHALL show 2 retries recorded

#### Scenario: No retry on real error

- GIVEN a handler fails with "command not found: xyz"
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"fail","summary":"command not found: xyz"}` immediately
- AND `metrics.Collector` SHALL show 0 retries recorded
- AND the handler SHALL be invoked exactly once

#### Scenario: Retry exhausted

- GIVEN a handler fails with "context deadline exceeded" on all 3 attempts
- WHEN `handleToolCall` processes the call
- THEN the response SHALL be `{"status":"fail","summary":"... after 3 attempts ..."}`
- AND `metrics.Collector` SHALL show 2 retries recorded

#### Scenario: TestHandleToolCall_RetryOnTransient

- GIVEN a mock handler that fails twice then succeeds
- WHEN `TestHandleToolCall_RetryOnTransient` runs
- THEN the test SHALL assert `status:"pass"` and `retries == 2`

#### Scenario: TestHandleToolCall_NoRetryOnRealError

- GIVEN a mock handler that fails with "command not found"
- WHEN `TestHandleToolCall_NoRetryOnRealError` runs
- THEN the test SHALL assert `status:"fail"` and `retries == 0`

#### Scenario: TestHandleToolCall_RetryExhausted

- GIVEN a mock handler that fails with "timeout" 3 times
- WHEN `TestHandleToolCall_RetryExhausted` runs
- THEN the test SHALL assert `status:"fail"` with summary mentioning "3 attempts" and `retries == 2`

---

### REQ-4: `TestWireMCPTools_AllToolsRegistered` asserts the count

**Priority**: P2
**Implements**: proposal.md §What Changes / 4
**Modifies domain**: `openspec/specs/mcp-server/spec.md` (Tool Handler Registration)

The existing test `TestWireMCPTools_AllToolsRegistered` (`internal/app/mcp_tools_test.go:30-45`) is a no-op — it does not assert the tool count. This allowed tools to be accidentally unregistered without test failure. The test MUST assert the exact count and list missing/extra tools on failure for debuggability.

| Req | Strength | Behavior |
|-----|----------|----------|
| Assert tool count | MUST | Assert `len(server.tools) == 29` (or expose `ToolCount() int` if `tools` is unexported) |
| Diagnostic on failure | MUST | List missing/extra tools if count is wrong |
| Test name | MUST | `TestWireMCPTools_AllToolsRegistered` |

#### Scenario: All 29 tools registered — test passes

- GIVEN all 29 MCP tools are registered
- WHEN `TestWireMCPTools_AllToolsRegistered` runs
- THEN the test SHALL pass

#### Scenario: Tool accidentally unregistered — test fails with diagnostic

- GIVEN 27 tools registered (one missing)
- WHEN `TestWireMCPTools_AllToolsRegistered` runs
- THEN the test SHALL fail
- AND the failure message SHALL list the missing tool name(s)

#### Scenario: Extra tool added — test fails with diagnostic

- GIVEN 29 tools registered (one extra)
- WHEN `TestWireMCPTools_AllToolsRegistered` runs
- THEN the test SHALL fail
- AND the failure message SHALL list the extra tool name(s)

---

### REQ-5: Sub-agent templates read `result.payload`

**Priority**: P1 (promoted from P2 in proposal)
**Implements**: proposal.md §What Changes / 5 + user-mandated priority adjustment
**Modifies domain**: `openspec/specs/sub-agents/spec.md`, `openspec/specs/orchestrator-skill/spec.md`

**Why P1**: The envelope wrapper (REQ-2) changes the shape of every MCP tool response from `{original}` to `{"status","summary","payload":{original}}`. If sub-agent templates continue reading `result` directly, they will parse the envelope instead of the original payload, breaking field extraction. This is a blocking dependency for REQ-2.

Each of the 6 sub-agent templates (`drup-preflight`, `drup-rector`, `drup-contrib`, `drup-custom`, `drup-theme`, `drup-validator`) MUST be updated to read the original tool response from `result.payload` instead of `result` directly. The orchestrator SKILL.md (`~/.config/opencode/skills/drup/SKILL.md`) MUST contain a "Dispatch Contract" section that explicitly states: sub-agents always read from `result.payload`.

| Req | Strength | Behavior |
|-----|----------|----------|
| Sub-agent template update | MUST | Each of the 6 templates reads `result.payload` instead of `result` |
| Orchestrator SKILL.md | MUST | Contains "Dispatch Contract" section stating sub-agents read from `result.payload` |
| Protocol extension documented | MUST | SKILL.md documents the MCP protocol extension (errors as `{"status":"fail"}` in result, not JSON-RPC errors) |
| Role definitions unchanged | MUST NOT | Do NOT change sub-agent role definitions, tool grants, or envelope contracts (other than `result.payload` reading) |

#### Scenario: Each sub-agent template reads `result.payload`

- GIVEN the 6 sub-agent templates (`drup-preflight`, `drup-rector`, `drup-contrib`, `drup-custom`, `drup-theme`, `drup-validator`)
- WHEN a manual review checks each template
- THEN each template SHALL contain the text `result.payload` in its response-handling instructions

#### Scenario: Orchestrator SKILL.md contains Dispatch Contract

- GIVEN the orchestrator SKILL.md
- WHEN a manual review checks it
- THEN it SHALL contain a "Dispatch Contract" section
- AND the section SHALL explicitly state sub-agents read from `result.payload`
- AND the section SHALL document the MCP protocol extension (errors as `{"status":"fail"}` in result channel)

#### Scenario: Sub-agent parses envelope correctly

- GIVEN a sub-agent receives an MCP tool response `{"status":"pass","summary":"...","payload":{"total_errors":0}}`
- WHEN the sub-agent parses the response
- THEN the sub-agent SHALL read `result.payload.total_errors` (not `result.total_errors`)
- AND the sub-agent SHALL correctly extract `total_errors == 0`

---

### REQ-6: No regression in existing pipeline

**Priority**: Acceptance criterion (blocking)
**Implements**: proposal.md §Acceptance Criteria

This is the acceptance gate. All existing tests MUST pass, all 29 MCP tools MUST continue to return their original payload structure (now wrapped in `envelope.payload`), and the 6 sub-agent templates' role definitions MUST NOT change (other than the `result.payload` reading update in REQ-5).

| Req | Strength | Behavior |
|-----|----------|----------|
| `go test ./...` passes | MUST | All unit tests pass |
| `go test ./internal/e2e/...` passes | MUST | E2E test (mock-based) passes; pipeline stage ordering unchanged |
| 29 MCP tools return original payload | MUST | Original payload structure preserved inside `envelope.payload` |
| Sub-agent role definitions unchanged | MUST NOT | Role definitions, tool grants unchanged (except REQ-5's `result.payload` reading) |
| `realHandleUpgradeScan` refactored to share helper | MAY | Refactored to call the shared `ensureUpgradeStatusEnabled` helper (REQ-1); install block (lines 816-822) preserved verbatim. The pre-change auto-enable behavior is identical, but the code now routes through the shared helper. |
| Total diff under 400 lines | MUST | Single PR, no chain needed |

#### Scenario: Full test suite passes

- GIVEN the implementation is complete
- WHEN `go test ./...` runs
- THEN all tests SHALL pass with no regressions

#### Scenario: E2E pipeline test passes

- GIVEN the implementation is complete
- WHEN `go test ./internal/e2e/...` runs
- THEN the E2E test SHALL pass (pipeline stage ordering unchanged)

#### Scenario: All 29 tools return original payload

- GIVEN the envelope wrapper is in place
- WHEN any of the 29 MCP tools is called
- THEN the response SHALL contain `envelope.payload` with the original tool-specific structure
- AND existing code parsing `scan.ScanResult` or `{"success","output"}` SHALL continue to work by reading `envelope.payload`

#### Scenario: `realHandleUpgradeScan` refactored to share the auto-enable helper

- GIVEN the implementation is complete
- WHEN a code review checks `realHandleUpgradeScan` (lines 777-917)
- THEN the function SHALL call the shared `ensureUpgradeStatusEnabled` helper
- AND the install block (lines 816-822) SHALL be preserved verbatim
- AND the pre-change auto-enable behavior SHALL be identical (same sequence: `pm:list` check → `config:delete update.settings` → `drush en upgrade_status -y` → `drush cr`)

---

## Coverage Summary

| Requirement | Happy Paths | Edge Cases | Error States |
|-------------|-------------|------------|--------------|
| REQ-1 (auto-enable) | ✅ 2 scenarios | ✅ 1 (already enabled) | ✅ 1 (not in composer.json) |
| REQ-2 (envelope) | ✅ 3 scenarios | ✅ 3 (deriveSummary variants) | ✅ 2 (error envelope, NOT JSON-RPC) |
| REQ-3 (retry) | ✅ 2 scenarios | ✅ 1 (retry exhausted) | ✅ 1 (no retry on real error) |
| REQ-4 (tool count) | ✅ 1 scenario | ✅ 2 (missing/extra tools) | N/A |
| REQ-5 (sub-agent templates) | ✅ 2 scenarios | ✅ 1 (envelope parsing) | N/A (manual review) |
| REQ-6 (no regression) | ✅ 3 scenarios | ✅ 1 (realHandleUpgradeScan unchanged) | N/A |

**Total**: 6 requirements, 24 scenarios (16 happy path, 8 edge/error).

---

## Out of Scope (from proposal)

- No new MCP tools
- No new sub-agents
- No changes to `drup_detect_env` (already works)
- No changes to `realHandleUpgradeScan` (already correct)
- No changes to Drupal.org HTTP retry (`doWithRetry` in `drupalorg.go`)
- No changes to env detection cache invalidation
- No `PreExistingConfigException` handling beyond the existing `update.settings` fix
- No key-value table manipulation

---

## Verification Strategy

All scenarios are testable in Go except REQ-5's manual review checks (prompt content). The test names in the scenarios match the proposal's test names for traceability:
- `TestRealHandleScan_AutoEnablesUpgradeStatus`
- `TestHandleToolCall_EnvelopeWrap_Success`
- `TestHandleToolCall_EnvelopeWrap_Error`
- `TestHandleToolCall_PayloadIntact`
- `TestHandleToolCall_RetryOnTransient`
- `TestHandleToolCall_NoRetryOnRealError`
- `TestHandleToolCall_RetryExhausted`
- `TestWireMCPTools_AllToolsRegistered` (existing test, now with real assertions)

REQ-5's manual review checks can be automated via a simple grep-based test if desired, but the proposal marks them as "manual review" — left to implementer discretion.
