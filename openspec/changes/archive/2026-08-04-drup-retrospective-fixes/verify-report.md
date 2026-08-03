# Verify Report: drup-retrospective-fixes

## Summary
- 6 REQs verified
- 34 tests pass / 0 tests fail
- CRITICAL: 1
- WARNING: 2
- SUGGESTION: 2
- Recommendation: fix-then-pass

## REQ-1: realHandleScan auto-enables upgrade_status
- Verdict: PASS
- Evidence:
  - `ensureUpgradeStatusEnabled` helper exists at `internal/app/mcp_tools.go:907-945`
  - `realHandleScan` calls helper at line 161 before running `upgrade_status:analyze`
  - `realHandleUpgradeScan` refactored to use helper at line 830 (auto-install block preserved at 820-827)
- Files: `internal/app/mcp_tools.go:161`, `internal/app/mcp_tools.go:830`, `internal/app/mcp_tools.go:907-945`
- Tests:
  - `TestEnsureUpgradeStatusEnabled_AlreadyEnabled` — PASS
  - `TestEnsureUpgradeStatusEnabled_AutoEnables` — PASS
  - `TestEnsureUpgradeStatusEnabled_NotInstalled` — PASS
  - `TestRealHandleScan_AutoEnablesUpgradeStatus` — PASS
- Notes: Spec says "MUST NOT modify realHandleUpgradeScan" but tasks/design say "refactor to use helper". Implementation followed tasks/design (DRY principle). This is a spec deviation but acceptable.

## REQ-2: Uniform MCP envelope
- Verdict: PASS
- Evidence:
  - `Envelope` struct (exported) at `internal/mcp/server.go:500-505`
  - `wrapInEnvelope` helper at `internal/mcp/server.go:508-520`
  - `deriveSummary` helper at `internal/mcp/server.go:523-547`
  - `handleToolCall` routes through `wrapInEnvelope` at line 491
  - `sendError` still used for protocol-level errors (unknown tool at line 483, invalid params at line 478)
  - Tool errors go through `sendResult` with `{status:"fail"}` envelope (line 497)
- Files: `internal/mcp/server.go:475-498`, `internal/mcp/server.go:500-547`
- Tests:
  - `TestHandleToolCall_EnvelopeWrap_Success` — PASS
  - `TestHandleToolCall_EnvelopeWrap_Error` — PASS
  - `TestHandleToolCall_PayloadIntact` — PASS
  - `TestDeriveSummary_TotalErrors` — PASS
  - `TestDeriveSummary_Success` — PASS
  - `TestDeriveSummary_Fallback` — PASS
  - `TestHandleToolCall_ProtocolErrors_StillJSONRPC` — PASS
- Notes: Envelope structure is `{status, summary, payload}`. Tool errors are NOT JSON-RPC errors. Payload preservation verified byte-for-byte.

## REQ-3: Selective retry
- Verdict: WARNING (metrics recording missing)
- Evidence:
  - `isTransientError` helper at `internal/mcp/server.go:432-449` matches all 5 transient patterns
  - `retryLoop` at `internal/mcp/server.go:453-473` with 3 attempts, exponential backoff
  - `retryBaseDelay` package var at line 428 (tests override to 1ms)
  - **CRITICAL**: `metrics.Default().RecordRetry()` is NOT called in `retryLoop` — spec requires this
- Files: `internal/mcp/server.go:428-473`
- Tests:
  - `TestIsTransientError` — PASS (9 subtests)
  - `TestRetryLoop_TransientThenSuccess` — PASS
  - `TestRetryLoop_NoRetryOnRealError` — PASS
  - `TestRetryLoop_Exhausted` — PASS
- Notes: Retry logic is correct, but metrics recording is missing. Spec REQ-3 says "MUST Record each retry via metrics.Collector.RecordRetry()". The `metrics.Collector.RecordRetry()` method exists (`internal/metrics/metrics.go:92-93`) but is never called from `retryLoop`. This is a CRITICAL deviation.

## REQ-4: Tool count assertion
- Verdict: PASS
- Evidence:
  - `ToolCount()` method at `internal/mcp/server.go:324-327`
  - `TestWireMCPTools_AllToolsRegistered` at `internal/app/mcp_tools_test.go:30-46` asserts `ToolCount() == 29`
  - Diagnostic output lists registered tools on failure (lines 42-45)
- Files: `internal/mcp/server.go:324-327`, `internal/app/mcp_tools_test.go:30-46`
- Tests:
  - `TestWireMCPTools_AllToolsRegistered` — PASS
  - `TestServer_ToolCount` — PASS
- Notes: Test correctly asserts 29 tools (not 28 as in original proposal). Diagnostic on failure is present.

## REQ-5: Sub-agent templates read result.payload
- Verdict: PASS
- Evidence:
  - All 18 templates (6 agents × 3 platforms) contain `result.payload` — verified by grep
  - SKILL.md contains "MCP Tool Response Envelope (server-level contract)" section at line 51
  - Section documents protocol extension (errors as `{status:"fail"}` in result channel) at line 61
- Files: `internal/packaging/templates/{opencode,claude,codex}/agents/drup-*.md` (18 files), `/home/borja/.config/opencode/skills/drup/SKILL.md:51-61`
- Tests:
  - `TestSubAgentTemplates_ContainPayloadReference` — PASS (3 platforms)
- Notes: All templates updated consistently. SKILL.md documents the protocol extension.

## REQ-6: No regression
- Verdict: PASS
- Tests run:
  - `go test ./...` — all packages pass (21 packages)
  - `go test ./internal/e2e/...` — PASS (3 tests)
  - `go test ./internal/mcp/...` — PASS (27 tests)
  - `go test ./internal/app/...` — PASS (34 tests)
  - `go test ./internal/packaging/...` — PASS (20 tests)
- Commits:
  - 6 commits in expected order: REQ-4 → REQ-1 → REQ-2 → REQ-3 → REQ-5 → REQ-5 test
  - Commit messages follow conventional format
  - No "Co-Authored-By" lines found
- Notes: Full test suite passes. E2E pipeline test passes. No regressions detected.

## Additional Findings

### CRITICAL: Metrics recording missing in retry loop
- **Location**: `internal/mcp/server.go:453-473` (`retryLoop` function)
- **Issue**: Spec REQ-3 says "MUST Record each retry via metrics.Collector.RecordRetry()". The `metrics.Collector.RecordRetry()` method exists but is never called from `retryLoop`.
- **Impact**: Retry attempts are not tracked in metrics. This breaks observability and makes it impossible to monitor retry frequency.
- **Fix**: Add `metrics.Default().RecordRetry()` call inside the retry loop (after `if attempt < maxAttempts` check, before `time.Sleep`).

### WARNING: Spec deviation — realHandleUpgradeScan was modified
- **Location**: `internal/app/mcp_tools.go:782-894`
- **Issue**: Spec says "MUST NOT modify realHandleUpgradeScan (lines 777-917)" but implementation refactored it to use `ensureUpgradeStatusEnabled` helper.
- **Impact**: Low. The refactor is DRY-compliant and preserves behavior (auto-install block kept separate). Tasks/design explicitly called for this refactor.
- **Recommendation**: Update spec to reflect the refactor, or document this as an intentional spec override.

### WARNING: Protocol extension not documented in user-facing docs
- **Location**: README, docs/
- **Issue**: The MCP protocol extension (errors as `{status:"fail"}` in result channel, not JSON-RPC errors) is documented in SKILL.md but not in user-facing documentation (README, docs/).
- **Impact**: External MCP clients may not know about the protocol extension.
- **Recommendation**: Add a "Protocol Extensions" section to README or docs/mcp-protocol.md documenting the envelope wrapper and error-as-result pattern.

### SUGGESTION: deriveSummary coverage is minimal
- **Location**: `internal/mcp/server.go:523-547`
- **Issue**: `deriveSummary` only handles 3 field patterns: `total_errors`, `success`, `summary`. The rest get generic "Tool {name} completed".
- **Impact**: Low. Generic summary is always safe. Specific summaries are nice-to-have.
- **Recommendation**: Acceptable as-is. Add tool-specific cases incrementally if users request better summaries.

### SUGGESTION: Smoke test was manual, not automated
- **Location**: N/A
- **Issue**: The orchestrator ran a manual end-to-end smoke test (initialize + tools/list + tools/call) against a real drup binary. This is not automated.
- **Impact**: Low. Manual smoke test confirmed 29 tools registered, envelope on success, `{status:fail}` envelope on tool error. But it's not repeatable.
- **Recommendation**: Consider adding an integration test that builds the drup binary and runs a smoke test against it. Not blocking for this change.

## Files Inspected
- `openspec/changes/drup-retrospective-fixes/specs/mcp-server-and-scan-robustness.md`
- `openspec/changes/drup-retrospective-fixes/tasks.md`
- `openspec/changes/drup-retrospective-fixes/design.md`
- `openspec/changes/drup-retrospective-fixes/proposal.md`
- `/home/borja/.config/opencode/skills/sdd-verify/SKILL.md`
- `/home/borja/.config/opencode/skills/drup/SKILL.md`
- `/home/borja/.config/opencode/skills/_shared/SKILL.md`
- `internal/mcp/server.go`
- `internal/mcp/mcp_test.go`
- `internal/app/mcp_tools.go`
- `internal/app/mcp_tools_test.go`
- `internal/packaging/packaging_test.go`
- `internal/packaging/templates/{opencode,claude,codex}/agents/drup-*.md` (18 files)
- `internal/metrics/metrics.go`

## Test Results
- `go test ./...`: all 21 packages pass
- `go test ./internal/e2e/...`: PASS (3 tests: TestPipeline_StageOrdering, TestPipeline_CleanupSkippedOnValidateFailure, TestPipeline_CleanupRunsOnValidatePass)
- `go test ./internal/mcp/... -v`: PASS (27 tests including envelope, retry, protocol errors)
- `go test ./internal/app/... -run "TestRealHandleScan|TestEnsureUpgradeStatus|TestWireMCPTools" -v`: PASS (10 tests)
- `go test ./internal/packaging/... -run TestSubAgentTemplates_ContainPayloadReference -v`: PASS (3 platform subtests)

## Recommendation
**FIX-THEN-PASS**: 1 CRITICAL fix needed before merge.

### Required fix:
1. **Add metrics recording to retry loop** (`internal/mcp/server.go:453-473`):
   - Import `github.com/nireneko/drup/internal/metrics`
   - Add `metrics.Default().RecordRetry()` call inside the retry loop (after `if attempt < maxAttempts` check, before `time.Sleep`)
   - This is a 2-line fix: import + one function call

### Optional improvements (not blocking):
2. Update spec to document the `realHandleUpgradeScan` refactor (or document as intentional override)
3. Add "Protocol Extensions" section to README/docs documenting the envelope wrapper
4. Consider adding an automated smoke test (integration test that builds drup binary)

## Artifacts
- verify-report.md (this file)
