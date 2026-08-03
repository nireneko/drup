# Tasks: drup-retrospective-fixes

## Overview

This is a single-PR change (size:exception, user override) addressing the P0/P1 failures from the 2026-08-03 retrospective. Estimated diff: ~566 lines across 6 requirements. Implementation order: REQ-4 → REQ-1 → REQ-2 → REQ-3 → REQ-5. REQ-6 is the cumulative acceptance gate, not a separate commit. Each commit leaves the system in a passing state (`go test ./...` passes, no protocol breakage visible to MCP clients).

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~566 |
| 400-line budget risk | High (over budget) |
| Chained PRs recommended | No (user override) |
| Suggested split | Single PR (size:exception) |
| Delivery strategy | size:exception (user override) |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Tool count assertion | PR 1 (single PR) | `go test ./internal/app/... -run TestWireMCPTools_AllToolsRegistered` | N/A (unit test) | `ToolCount()` method + test assertion |
| 2 | `realHandleScan` auto-enable | PR 1 (single PR) | `go test ./internal/app/... -run TestRealHandleScan_AutoEnablesUpgradeStatus` | Mock drush calls via `drupexec.RunWithEnv` | `ensureUpgradeStatusEnabled` helper + `realHandleScan` modification |
| 3 | Uniform MCP envelope | PR 1 (single PR) | `go test ./internal/mcp/... -run TestHandleToolCall_EnvelopeWrap` | Mock handler via `server.RegisterTool` | `Envelope` struct + `wrapInEnvelope` + `deriveSummary` + `handleToolCall` rewrite |
| 4 | Selective retry | PR 1 (single PR) | `go test ./internal/mcp/... -run TestRetryLoop_TransientThenSuccess` | Mock handler with transient errors | `retryLoop` + `isTransientError` + `retryBaseDelay` var |
| 5 | Sub-agent template updates | PR 1 (single PR) | `go test ./internal/packaging/... -run TestSubAgentTemplates_ContainPayloadReference` | N/A (grep test) | 18 template files + SKILL.md + grep test |

## Commit Strategy

The PR is broken into 5 atomic commits (one per REQ except REQ-6). Each commit MUST leave the system in a passing state. The commit order is fixed by dependency: REQ-4 (isolated test infrastructure) → REQ-1 (handler change, no protocol impact) → REQ-2 (protocol change, envelope wrapper) → REQ-3 (retry on top of envelope) → REQ-5 (template updates to match new envelope contract). REQ-6 is the cumulative acceptance gate run after all commits.

- Commit 1: REQ-4 — Tool count assertion (smallest, isolated)
- Commit 2: REQ-1 — `realHandleScan` auto-enable (handler change, no protocol impact)
- Commit 3: REQ-2 — Uniform MCP envelope (protocol change)
- Commit 4: REQ-3 — Selective retry (builds on envelope)
- Commit 5: REQ-5 — Sub-agent template updates (depends on envelope contract)

---

## Commit 1: REQ-4 — Fix TestWireMCPTools_AllToolsRegistered

**Files touched**:
- `internal/mcp/server.go` (add `ToolCount()` method)
- `internal/app/mcp_tools_test.go` (modify `TestWireMCPTools_AllToolsRegistered`)

**Estimated lines**: ~30

**Depends on**: none

**What to implement**:
- Add `ToolCount() int` method to `Server` in `internal/mcp/server.go` that returns `len(s.tools)`
- Modify `TestWireMCPTools_AllToolsRegistered` in `internal/app/mcp_tools_test.go` to assert `server.ToolCount() == 29` (note: actual count is 29, not 28 as stated in design — verified from code)
- Add diagnostic output on failure: list registered tool names vs expected set

**Tests to add**:
- `TestServer_ToolCount` (in `internal/mcp/server_test.go` if it exists, or skip): create server, register 2 tools, assert `ToolCount() == 2`

**Acceptance**:
- [x] `go test ./internal/mcp/...` passes
- [x] `go test ./internal/app/... -run TestWireMCPTools_AllToolsRegistered` passes
- [x] Test fails if a tool is unregistered (manual verification: temporarily comment out one `RegisterTool` call, run test, confirm failure with diagnostic)

**Commit message**:
```
fix(mcp): assert tool count in TestWireMCPTools_AllToolsRegistered

The test was a no-op that logged but did not assert the registered tool
count. This allowed tools to be accidentally unregistered without test
failure. Add ToolCount() method and assert the exact count (29 tools).
On failure, list missing/extra tools for debuggability.

Refs: drup-retrospective-fixes, RETROSPECTIVE.md
```

---

## Commit 2: REQ-1 — `realHandleScan` auto-enables `upgrade_status`

**Files touched**:
- `internal/app/mcp_tools.go` (add `ensureUpgradeStatusEnabled` helper, modify `realHandleScan`, refactor `realHandleUpgradeScan` to use helper)
- `internal/app/mcp_tools_test.go` (add tests)

**Estimated lines**: ~140

**Depends on**: Commit 1

**What to implement**:
- Extract `ensureUpgradeStatusEnabled(projectPath string) error` helper in `internal/app/mcp_tools.go` (or new file `upgrade_status_helper.go` if preferred)
- Helper logic: check `composer.json` for `drupal/upgrade_status`, check `pm:list --status=enabled`, if not enabled run `config:delete update.settings` → `drush en upgrade_status -y` → `drush cr`
- Modify `realHandleScan` (line 152-179) to call `ensureUpgradeStatusEnabled(params.ProjectPath)` before running `upgrade_status:analyze`
- Refactor `realHandleUpgradeScan` (lines 824-856) to call the helper instead of duplicating the enable logic. Keep its auto-install block (lines 816-822) separate — the helper does NOT auto-install (per spec)
- Add tests

**Tests to add**:
- `TestEnsureUpgradeStatusEnabled_AlreadyEnabled`: mock `pm:list` to return `{"upgrade_status":{...}}`, assert helper returns nil without calling `drush en`
- `TestEnsureUpgradeStatusEnabled_AutoEnables`: mock `pm:list` to return `{}`, mock `drush en` to succeed, assert `drush cr` is called after
- `TestEnsureUpgradeStatusEnabled_NotInstalled`: create temp dir with `composer.json` lacking `drupal/upgrade_status`, assert error contains "not installed"
- `TestRealHandleScan_AutoEnablesUpgradeStatus`: integration test — mock project with `upgrade_status` in composer but not enabled, call `realHandleScan`, assert scan proceeds without "no commands defined" error

**Acceptance**:
- [x] `go test ./internal/app/...` passes
- [x] `TestRealHandleScan_AutoEnablesUpgradeStatus` passes
- [x] `TestEnsureUpgradeStatusEnabled_*` tests pass
- [x] Manual: `realHandleUpgradeScan` behavior unchanged (existing tests still pass)

**Commit message**:
```
fix(scan): auto-enable upgrade_status in realHandleScan

realHandleScan ran drush upgrade_status:analyze without checking if
upgrade_status was enabled. When the module was present but not enabled,
drush returned "no commands defined in the upgrade_status namespace".
Extract ensureUpgradeStatusEnabled helper from realHandleUpgradeScan
and call it from both realHandleScan and realHandleUpgradeScan. The
helper checks composer.json, checks pm:list, and auto-enables if needed.

Refs: drup-retrospective-fixes, RETROSPECTIVE.md
```

---

## Commit 3: REQ-2 — Uniform MCP envelope in `handleToolCall`

**Files touched**:
- `internal/mcp/server.go` (add `Envelope` struct, `wrapInEnvelope`, `deriveSummary`, rewrite `handleToolCall`)
- `internal/mcp/server_test.go` (new file or extend existing)

**Estimated lines**: ~180

**Depends on**: Commit 2

**What to implement**:
- Add exported `Envelope` struct in `internal/mcp/server.go`:
  ```go
  type Envelope struct {
      Status  string          `json:"status"`
      Summary string          `json:"summary"`
      Payload json.RawMessage `json:"payload,omitempty"`
  }
  ```
- Add `wrapInEnvelope(toolName string, result json.RawMessage, handlerErr error) Envelope` helper
- Add `deriveSummary(toolName string, payload json.RawMessage) string` helper — best-effort extraction of `total_errors`, `success`, `summary` fields; fallback to "Tool {name} completed"
- Rewrite `handleToolCall` (lines 408-425):
  - Keep `sendError` for protocol-level errors (malformed JSON, unknown tool, invalid params)
  - Route handler outcomes through `wrapInEnvelope`
  - Send ALL tool outcomes (success and error) via `sendResult` — errors become `{status:"fail"}` in result channel, NOT JSON-RPC errors
  - Envelope marshal failure is the only case where a tool-related error still goes through `sendError` (server bug, not tool failure)

**Tests to add**:
- `TestHandleToolCall_EnvelopeWrap_Success`: mock handler returns `{"foo":"bar"}`, assert response has `status:"pass"`, `summary` non-empty, `payload.foo:"bar"`
- `TestHandleToolCall_EnvelopeWrap_Error`: mock handler returns `(nil, error("test error"))`, assert response has `status:"fail"`, `summary:"test error"`, NO `error` field at JSON-RPC level
- `TestHandleToolCall_PayloadIntact`: mock handler returns complex nested JSON, assert `envelope.payload` matches original byte-for-byte
- `TestDeriveSummary_TotalErrors`: `deriveSummary("scan", {"total_errors":3})` → contains "3 errors"
- `TestDeriveSummary_Success`: `deriveSummary("drush_exec", {"success":true})` → contains "succeeded"
- `TestDeriveSummary_Fallback`: `deriveSummary("unknown_tool", {"custom":"data"})` → "Tool unknown_tool completed"
- `TestHandleToolCall_ProtocolErrors_StillJSONRPC`: unknown tool name → JSON-RPC error with code `-32601`; malformed params → `-32602`

**Acceptance**:
- [x] `go test ./internal/mcp/...` passes
- [x] All envelope tests pass
- [x] Manual: run a smoke test against the host MCP client (or mock) to confirm clients can parse the new `{status, summary, payload}` shape
- [x] Existing tests in `internal/app/...` still pass (handler behavior unchanged)

**Commit message**:
```
fix(mcp): wrap all tool responses in uniform envelope

All 28 MCP tool responses are now wrapped in {"status":"pass|fail",
"summary":"...","payload":{...}} at the server level. Tool errors are
sent as {"status":"fail"} in the result channel, NOT as JSON-RPC errors.
This ensures the orchestrator model always receives parseable output
with a uniform success/failure signal. The original tool-specific
payload is preserved intact inside envelope.payload.

Protocol extension: errors as {"status":"fail"} in result channel.
JSON-RPC errors are reserved for protocol-level failures (malformed
JSON, unknown tool, invalid params).

Refs: drup-retrospective-fixes, RETROSPECTIVE.md
```

---

## Commit 4: REQ-3 — Selective retry for transient errors

**Files touched**:
- `internal/mcp/server.go` (add `isTransientError`, `retryLoop`, modify `handleToolCall` to call `retryLoop`)
- `internal/mcp/server_test.go` (add tests)

**Estimated lines**: ~130

**Depends on**: Commit 3

**What to implement**:
- Add `isTransientError(err error) bool` helper in `internal/mcp/server.go` — matches: "context deadline exceeded", "connection refused", "i/o timeout", "broken pipe", "no such host"
- Add package-level var `retryBaseDelay = 1 * time.Second` (tests override to `1ms`)
- Add `retryLoop(toolName string, handler ToolHandler, args json.RawMessage) (json.RawMessage, error)` method on `Server`:
  - Max 3 attempts (2 retries)
  - 1s base exponential backoff (1s, 2s) — use `retryBaseDelay` var
  - Call `metrics.Default().RecordRetry()` on each retry (not on initial attempt)
  - Return immediately on non-transient error
  - On exhaustion, return error with "after 3 attempts" suffix
- Modify `handleToolCall` to call `s.retryLoop(p.Name, handler, p.Arguments)` instead of `handler(p.Arguments)` directly

**Tests to add**:
- `TestRetryLoop_TransientThenSuccess`: handler fails with "context deadline exceeded" twice, succeeds on third. Assert `status:"pass"`, `metrics.Default().Snapshot().Retries == 2`
- `TestRetryLoop_NoRetryOnRealError`: handler fails with "command not found". Assert `status:"fail"`, handler called exactly once, `Retries == 0`
- `TestRetryLoop_Exhausted`: handler fails with "i/o timeout" 3 times. Assert `status:"fail"`, summary contains "after 3 attempts", `Retries == 2`
- `TestIsTransientError`: table-driven test — "context deadline exceeded" → true, "connection refused" → true, "command not found" → false, "exit status 1" → false, nil → false

**Acceptance**:
- [x] `go test ./internal/mcp/...` passes
- [x] All retry tests pass
- [x] `metrics.Default().Snapshot().Retries` is correctly incremented
- [x] Manual: verify retry does not mask real errors (e.g., "module not found" is NOT retried)

**Commit message**:
```
fix(mcp): add selective retry for transient MCP tool errors

Wrap handler calls with retry logic for transient transport errors
only: "context deadline exceeded", "connection refused", "i/o timeout",
"broken pipe", "no such host". Max 2 retries (3 total attempts) with
1s base exponential backoff. Record retries via metrics.Collector.
Real errors (exit code ≠ 0, "command not found") are NOT retried.

Refs: drup-retrospective-fixes, RETROSPECTIVE.md
```

---

## Commit 5: REQ-5 — Sub-agent templates read `result.payload`

**Files touched**:
- `internal/packaging/templates/{opencode,claude,codex}/agents/drup-{preflight,rector,contrib,custom,theme,validator}.md` (18 files)
- `/home/borja/.config/opencode/skills/drup/SKILL.md` (add "Dispatch Contract" section)
- `internal/packaging/templates_test.go` (new file or extend existing)

**Estimated lines**: ~86

**Depends on**: Commit 3 (envelope contract must exist before templates are updated)

**What to implement**:
- Add "## MCP Response Contract" section to each of the 18 sub-agent templates:
  ```markdown
  ## MCP Response Contract

  Every MCP tool response is wrapped in a uniform envelope:

  ```json
  {"status": "pass|fail", "summary": "...", "payload": { ...tool-specific data... }}
  ```

  Read the tool-specific response from `result.payload`, NOT from `result` directly. Check `result.status` for "pass" or "fail" before parsing `result.payload`. On `status: "fail"`, `result.summary` contains the error message.
  ```
- Update any existing prose that says "the tool returns X" to "the tool returns `result.payload` containing X"
- Add "Dispatch Contract" section to `/home/borja/.config/opencode/skills/drup/SKILL.md`:
  ```markdown
  ## MCP Tool Response Envelope (server-level contract)

  Every MCP tool response is wrapped in a uniform envelope by the server:

  ```json
  {"status": "pass|fail", "summary": "one-liner", "payload": { ...tool-specific data... }}
  ```

  Sub-agents read the tool-specific response from `result.payload`. Check `result.status` before parsing. On `status: "fail"`, `result.summary` has the error — do NOT retry via bash; report back to the orchestrator.

  **Protocol extension**: tool errors are returned as `{"status":"fail"}` in the result channel, NOT as JSON-RPC errors. This ensures the orchestrator model always receives parseable output.
  ```
- Add grep-based test `TestSubAgentTemplates_ContainPayloadReference` in `internal/packaging/templates_test.go`: for each of the 18 template files, assert the file contains `result.payload`. Fail with the file path if missing.

**Tests to add**:
- `TestSubAgentTemplates_ContainPayloadReference`: grep-based test that fails if any template is missing `result.payload`

**Acceptance**:
- [x] `go test ./internal/packaging/...` passes
- [x] All 18 templates contain `result.payload`
- [x] SKILL.md contains "Dispatch Contract" section
- [x] Manual: review one template to confirm prose is clear and consistent

**Commit message**:
```
docs(templates): update sub-agent templates to read result.payload

The MCP envelope wrapper (REQ-2) changes every tool response shape
from {original} to {"status","summary","payload":{original}}. Update
all 18 sub-agent templates (6 agents × 3 platforms) to read from
result.payload instead of result directly. Add "MCP Response Contract"
section to each template. Add "Dispatch Contract" section to the
orchestrator SKILL.md documenting the protocol extension.

Refs: drup-retrospective-fixes, RETROSPECTIVE.md
```

---

## Acceptance (REQ-6 — PR merge gate)

REQ-6 is not a commit; it's the cumulative acceptance gate. After all 5 commits:

- [x] `go test ./...` passes with no regressions
- [x] `go test ./internal/e2e/...` passes (pipeline stage ordering unchanged)
- [x] `go test ./internal/mcp/...` passes (envelope + retry tests)
- [x] `go test ./internal/app/...` passes (auto-enable + tool count tests)
- [x] `go test ./internal/packaging/...` passes (template grep test)
- [x] Manual: run a smoke test of one MCP tool before and after the change to confirm the new envelope shape works with the host MCP client
- [x] Manual: verify `realHandleUpgradeScan` (lines 777-917) behavior is unchanged (only internal refactor to use shared helper)
- [x] Manual: verify all 29 tool payloads are preserved byte-for-byte inside `envelope.payload`
- [x] Manual: verify `sendError` is still used for protocol-level errors (malformed JSON, unknown tool, method not found)
- [x] Total diff is ~566 lines (single PR, size:exception)

---

## Risks for the Implementer

1. **Envelope breaks sub-agent parsing**: The envelope wrapper changes the MCP protocol contract. If the host MCP client (Claude Code, OpenCode, etc.) does its own JSON-RPC error handling, Commit 3 may need adjustment. The implementer must run an end-to-end smoke test after Commit 3 to confirm clients can still parse the new shape.

2. **Retry masks bugs**: The retry loop in Commit 4 can mask real errors. If `isTransientError` is too broad, real errors (e.g., "module not found") will be retried 3 times. The implementer should err on the side of NOT retrying unless the error message clearly indicates a transient failure. The test `TestRetryLoop_NoRetryOnRealError` is the guard.

3. **Template update is mechanical but extensive**: The sub-agent template update in Commit 5 touches 18 files. The implementer should use `sed` or a small script to do the find-replace, not manual editing. The grep test `TestSubAgentTemplates_ContainPayloadReference` makes this mechanically verifiable.

4. **Tool count discrepancy**: The design and spec say 28 tools, but the actual code has 29 tools (verified from `internal/app/mcp_tools.go` lines 52-81). The test should assert 29, not 28. If a tool is added or removed in the future, the test will fail and the implementer must update the expected count.

5. **`retryBaseDelay` package var is not test-safe**: Tests override it to `1ms` to avoid 3-second test runs. The implementer must use `defer` to restore the original value. Document in test comments.

---

## PR Description Draft

**Title**: fix(mcp): uniform envelope, selective retry, and scan robustness

**Body**:

Fixes the 2026-08-03 retrospective P0 findings: `drup_scan` no longer crashes when `upgrade_status` is not pre-enabled, and all 29 MCP tools now return a uniform `{status, summary, payload}` envelope so the orchestrator model can always detect success/failure.

**Commits**:
1. `fix(mcp): assert tool count in TestWireMCPTools_AllToolsRegistered` — replaces no-op test with real assertion
2. `fix(scan): auto-enable upgrade_status in realHandleScan` — brings `realHandleScan` in line with `realHandleUpgradeScan`
3. `fix(mcp): wrap all tool responses in uniform envelope` — server-level wrapper, errors as `{status:"fail"}` in result channel
4. `fix(mcp): add selective retry for transient MCP tool errors` — retry on timeout/transport errors only, max 2 retries
5. `docs(templates): update sub-agent templates to read result.payload` — 18 templates + SKILL.md dispatch contract

**Protocol extension**: tool errors are returned as `{"status":"fail"}` in the result channel, NOT as JSON-RPC errors. This ensures the orchestrator model always receives parseable output. JSON-RPC errors are reserved for protocol-level failures (malformed JSON, unknown tool, invalid params).

**Refs**:
- `RETROSPECTIVE.md`
- `openspec/changes/drup-retrospective-fixes/proposal.md`
- `openspec/changes/drup-retrospective-fixes/design.md`
- `openspec/changes/drup-retrospective-fixes/specs/mcp-server-and-scan-robustness.md`

---

## Time Estimate

- Commit 1 (REQ-4): ~30 min
- Commit 2 (REQ-1): ~2 hours (helper extraction + 4 tests)
- Commit 3 (REQ-2): ~3 hours (envelope + deriveSummary + 7 tests + smoke test)
- Commit 4 (REQ-3): ~1.5 hours (retry loop + 4 tests)
- Commit 5 (REQ-5): ~1.5 hours (mechanical template updates + grep test)
- **Total**: ~8.5 hours of focused work
