# Proposal: drup-retrospective-fixes

**One-liner**: Fix the two P0 failures from the 2026-08-03 retrospective — `drup_scan` crashing when `upgrade_status` is not pre-enabled, and MCP tools returning tool-specific payloads with no uniform success/failure signal — plus selective retry for transient errors and a test that actually asserts the registered tool count.

## Why

The retrospective documents a failed Drupal 10.6 → 11 pipeline run where the orchestrator (mimo-v2.5) could not complete a single stage. Two P0 findings drove the cascade failure:

1. **`drup_detect_env` returned empty** → no environment detected → every subsequent `drush` call used the wrong wrapper → ~15 minutes of manual diagnosis. The codebase has since been audited: `drup_detect_env` works, and lazy detection in `cliRun()` / `realHandleDrushExec()` means it self-heals on first access. **This is already fixed.**

2. **MCP tools returned no output** → the orchestrator had no signal for success/failure → it assumed success and continued into a broken state. The codebase now returns structured JSON from all 28 tools, but each tool uses a **different shape** (`scan.ScanResult`, `{"success","output","stderr","exit_code"}`, `[]Manifest`, etc.). The orchestrator model must parse 28 different shapes to detect pass/fail. The retrospective's suggested uniform envelope (`{"status":"pass|fail","summary":"..."}`) is still missing.

3. **Sub-agents never attempted** → the orchestrator called MCP tools directly, violating the SKILL.md. Investigation confirms all 6 sub-agent templates exist and are well-defined (`templates/{opencode,claude,codex}/agents/drup-*.md`). The orchestrator model not using them is a **model behavior issue**, not a configuration issue. Prompt-level reinforcement is the appropriate lever.

The concrete incident: `drup_scan` (mcp_tools.go:152-179) runs `drush upgrade_status:analyze` directly without checking if the module is enabled. When `upgrade_status` is present in the filesystem but not enabled (the exact state from the retrospective), drush returns exit 1 with "There are no commands defined in the upgrade_status namespace." Meanwhile, `drup_upgrade_scan` (mcp_tools.go:825-856) **does** check and auto-enable. Two tools, same underlying command, different robustness.

## What Changes

### 1. [P0] `realHandleScan` auto-enables `upgrade_status`

**File**: `internal/app/mcp_tools.go:152-179`
**What**: Port the auto-enable logic from `realHandleUpgradeScan` (lines 825-856) into `realHandleScan`. Before running `drush upgrade_status:analyze`, check `pm:list --status=enabled`, and if `upgrade_status` is missing, run `config:delete update.settings` + `drush en upgrade_status -y` + `drush cr`.
**Verification**: New test `TestRealHandleScan_AutoEnablesUpgradeStatus` that mocks a project where `upgrade_status` is in filesystem but not enabled, invokes `realHandleScan`, and asserts the scan completes without error. The test must fail when the auto-enable block is removed.
**Lines**: ~40 (handler) + ~30 (test) = 70

### 2. [P0] Uniform response envelope at MCP server level

**File**: `internal/mcp/server.go:408-425` (`handleToolCall`)
**What**: After `handler(p.Arguments)` returns `(result, nil)`, wrap the result in a uniform envelope before sending:

```go
// Post-process successful results to add uniform envelope fields.
if err == nil && result != nil {
    var payload interface{}
    if json.Unmarshal(result, &payload) == nil {
        envelope := map[string]interface{}{
            "status":  "pass",
            "summary": deriveSummary(p.Name, payload),
            "payload": payload,
        }
        result, _ = json.Marshal(envelope)
    }
}
```

For errors (the `err != nil` branch), the JSON-RPC error already surfaces to the caller. Add a parallel envelope for the error case by converting the error into a structured payload before sending the JSON-RPC error:

```go
if err != nil {
    errEnvelope := map[string]interface{}{
        "status":  "fail",
        "summary": err.Error(),
    }
    errJSON, _ := json.Marshal(errEnvelope)
    // Send as success with fail status instead of JSON-RPC error,
    // so the orchestrator model always gets parseable output.
    return s.sendResult(id, errJSON)
}
```

**Key design decision**: Errors become `{"status":"fail","summary":"..."}` in the result channel, NOT JSON-RPC errors. This is deliberate — the retrospective's core complaint was that the orchestrator had no parseable signal. A JSON-RPC error is a transport-level signal that some MCP clients swallow silently. A `{"status":"fail"}` in the result is always visible.

**`deriveSummary`**: A small helper that returns a one-line summary based on tool name and payload. For 28 tools, this can be a simple switch on `p.Name` returning format strings, or a best-effort extraction (e.g., if payload has `"success":true`, summary = "Tool X succeeded"; if payload has `"total_errors":0`, summary = "Scan complete: 0 errors"). Start with a minimal implementation that covers the top-level fields common to most tools.

**Verification**: New test `TestHandleToolCall_EnvelopeWrap` in `internal/mcp/server_test.go` that registers a mock handler returning `{"foo":"bar"}`, calls `handleToolCall`, and asserts the response contains `{"status":"pass","summary":"...","payload":{"foo":"bar"}}`. Also test the error path: handler returns `(nil, error)` → response is `{"status":"fail","summary":"..."}`.

**Lines**: ~50 (server.go) + ~40 (test) = 90

### 3. [P1] Selective retry for transient MCP tool errors

**File**: `internal/mcp/server.go:408-425` (alongside envelope wrapper)
**What**: Wrap `handler(p.Arguments)` with retry logic for transient errors only. Reuse the `doWithRetry` pattern from `drupalorg.go:31-80` but with a different policy:

- **Retry**: errors containing "context deadline exceeded", "connection refused", "timeout", "i/o timeout", "broken pipe"
- **Do NOT retry**: errors containing "exit status", "command not found", "no commands defined", "not defined", "already exists", "does not exist"

Implementation: a `isTransientError(err error) bool` helper + a retry loop (2 attempts max, 1s base backoff) around the handler call. Record retries via `metrics.Collector.RecordRetry()`.

**Verification**: New test `TestHandleToolCall_RetryOnTransient` that registers a handler that fails with "context deadline exceeded" on first call and succeeds on second. Assert the response is `{"status":"pass",...}` and `metrics.Collector` shows 1 retry. Also test `TestHandleToolCall_NoRetryOnRealError` — handler fails with "command not found" → no retry, immediate `{"status":"fail",...}`.

**Lines**: ~40 (retry logic) + ~30 (test) = 70

### 4. [P2] `TestWireMCPTools_AllToolsRegistered` actually asserts count

**File**: `internal/app/mcp_tools_test.go:30-45`
**What**: Replace the no-op test with a real assertion. Access `server.tools` (the internal map) and assert `len(server.tools) == 28`. If the `tools` field is unexported, expose a `ToolCount() int` method on `Server` for testing.
**Verification**: The test fails if any tool is accidentally unregistered.
**Lines**: ~10

### 5. [P2] Prompt-level reinforcement for sub-agent dispatch

**Files**: `~/.config/opencode/skills/drup/SKILL.md`, `internal/packaging/templates/{opencode,claude,codex}/agents/drup-*.md`
**What**: Add an explicit "Dispatch Contract" section to the orchestrator SKILL.md:

```markdown
## Dispatch Contract (HARD RULE)
You NEVER call MCP tools directly. You dispatch sub-agents via task().
Each sub-agent has a defined envelope: {"agent","status","summary","artifacts","evidence","risks"}.
If a sub-agent returns status=failed, STOP and report. Do not retry via bash.
```

Also add a one-line reminder to each sub-agent template's header:

```markdown
> You are a sub-agent. You receive tasks via task() from the orchestrator. You return results via the envelope contract. You do NOT call other sub-agents.
```

**Verification**: Manual review. No automated test for prompt content.
**Lines**: ~30

## What Does NOT Change

- **The 28 tool-specific payload shapes stay intact.** The envelope wraps them in `{"status","summary","payload":{...}}`. Existing code that parses `scan.ScanResult` or `{"success","output"}` continues to work by reading `envelope.payload`.
- **Sub-agent templates** beyond the dispatch reinforcement above. Their role definitions, tool grants, and envelope contracts are already correct.
- **`drup_detect_env` itself.** It works. Lazy detection in `cliRun()` / `realHandleDrushExec()` handles the case where it's never explicitly called.
- **`realHandleUpgradeScan`** (mcp_tools.go:777-917). It already auto-enables `upgrade_status` correctly. Do not touch it.
- **The "External Validation Only" / "No Self-Approval" / "Validator Owns All Gates" rules** from the drup SKILL.md. These are correct and non-negotiable.
- **Drupal.org HTTP retry** (`doWithRetry` in drupalorg.go). Already correct.
- **In-memory env detection cache** (envdetect.go). Process-scoped cache with mtime invalidation is fine.

## Open Decisions — Recommendations

### Q1: Envelope approach
**Recommendation**: Server-level wrap in `handleToolCall`.
**Rationale**: 1 file, ~50 lines, no per-handler risk, all 28 tools covered. Modifying 28 handlers individually would be ~280 lines of repetitive code with high merge-conflict risk. The server-level approach is the lazy solution that works.

### Q2: Retry scope
**Recommendation**: Selective retry on timeout/transport errors only. No retry on drush/composer exit code ≠ 0.
**Rationale**: Blanket retry would turn "module not found" into 3× "module not found" over 3 seconds. The retrospective's timeout (`drup_test_backup_list` → "Request timed out") was a transport-level failure, not a logic error. Retry the transport, not the logic.

### Q3: `realHandleScan` vs `realHandleUpgradeScan`
**Recommendation**: Bring `realHandleScan` in line with `realHandleUpgradeScan` (auto-enable). Do NOT deprecate `realHandleScan`.
**Rationale**: Both tools exist and are called by different code paths. `drup_scan` is the simpler interface (no module filter, no scope); `drup_upgrade_scan` is the richer one. The user shouldn't have to know which tool to pick based on whether `upgrade_status` is pre-enabled. Auto-enable in both removes the footgun.

### Q4: Sub-agent dispatch
**Recommendation**: Prompt-level reinforcement in SKILL.md + sub-agent templates. Do NOT attempt code-level enforcement (removing MCP tool access from the orchestrator) — the orchestrator is a model, not a Go process, and the host platform's `task()` primitive is the dispatch mechanism. If the model ignores the prompt, the fix is better prompting or platform-level agent routing, not drup code changes.
**Rationale**: The sub-agent templates are correct. The SKILL.md is explicit. The failure was model behavior. Code changes to drup cannot fix model behavior; prompt changes can influence it.

## Review Workload Forecast

| Item | File | Lines |
|------|------|-------|
| `realHandleScan` auto-enable + test | `mcp_tools.go` + `mcp_tools_test.go` | 70 |
| Envelope wrapper + test | `mcp/server.go` + `mcp/server_test.go` | 90 |
| Selective retry + test | `mcp/server.go` + `mcp/server_test.go` | 70 |
| Tool count assert | `mcp_tools_test.go` | 10 |
| SKILL.md + sub-agent template updates | `SKILL.md` + `templates/*/agents/*.md` | 30 |
| **Total** | | **270** |

**Verdict**: 270 lines, well under the 400-line chained-PR threshold. **Single PR, no chain needed.**

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Envelope wrapper breaks sub-agent prompt parsing — if a sub-agent prompt expects bare `scan.ScanResult` and now gets `{"status","summary","payload":{...}}`, it may fail to extract fields. | Medium | Keep tool-specific payload as `envelope.payload` (nested field). Update sub-agent templates to read `result.payload` instead of `result` directly. This is a 1-line change per template. |
| Retry on MCP dispatch retries a drush command that already mutated state (e.g., partial `composer install`, partial `drush en`). | Medium | Only retry on transport/timeout errors ("context deadline exceeded", "connection refused", "timeout"). Never retry on exit code ≠ 0 or error strings containing "exit status". |
| Converting errors from JSON-RPC errors to `{"status":"fail"}` results changes the MCP protocol contract. Some MCP clients may expect JSON-RPC errors for failures. | Low | The retrospective's core complaint was that errors were invisible. A `{"status":"fail"}` in the result is always visible. Document this as an intentional protocol extension. If a client needs JSON-RPC errors, it can check for the absence of `status` field. |
| `realHandleScan` change touches the scan flow used by Stage 1 of the pipeline. The e2e test (`internal/e2e/pipeline_test.go`) must still pass. | Low | The e2e test is mock-based and does not invoke real drush. The auto-enable logic uses the same `drupexec.RunWithEnv` path that the e2e test already mocks. Verify by running `go test ./internal/e2e/...` after the change. |
| `deriveSummary` for 28 tools is a maintenance burden — every new tool needs a case in the switch. | Low | Start with a minimal implementation: extract `summary` or `success` fields if present, otherwise use a generic "Tool {name} completed" message. Do not attempt to generate meaningful summaries for all 28 tools upfront. |

## Out of Scope

- No new MCP tools
- No new sub-agents
- No changes to sub-agent templates beyond the dispatch reinforcement (1-line header reminder)
- No changes to Drupal.org HTTP retry (`doWithRetry` in drupalorg.go)
- No changes to env detection storage or cache invalidation
- No changes to `realHandleUpgradeScan` (already correct)
- No changes to `drup_detect_env` (already works)
- No changes to the "External Validation Only" / "No Self-Approval" rules
- No `PreExistingConfigException` handling beyond the existing `update.settings` fix in `realHandleUpgradeScan`
- No key-value table manipulation (the retrospective's manual hack is not a pattern to codify)

## Acceptance Criteria

- [ ] `realHandleScan` auto-enables `upgrade_status` when it is present in filesystem but not enabled. Verified by `TestRealHandleScan_AutoEnablesUpgradeStatus`.
- [ ] All 28 MCP tool responses are wrapped in `{"status":"pass|fail","summary":"...","payload":{...}}` at the server level. Verified by `TestHandleToolCall_EnvelopeWrap`.
- [ ] MCP tool errors return `{"status":"fail","summary":"..."}` in the result channel (not JSON-RPC errors). Verified by `TestHandleToolCall_ErrorEnvelope`.
- [ ] Transient errors (timeout, connection refused) are retried up to 2 times with 1s backoff. Verified by `TestHandleToolCall_RetryOnTransient`.
- [ ] Real errors (exit code ≠ 0, "command not found") are NOT retried. Verified by `TestHandleToolCall_NoRetryOnRealError`.
- [ ] `TestWireMCPTools_AllToolsRegistered` asserts `len(server.tools) == 28` and fails if a tool is unregistered.
- [ ] Orchestrator SKILL.md contains a "Dispatch Contract" section that explicitly forbids direct MCP tool calls.
- [ ] Each sub-agent template contains a 1-line header reminder about the envelope contract.
- [ ] `go test ./...` passes with no regressions.
- [ ] `go test ./internal/e2e/...` passes (pipeline stage ordering unchanged).
- [ ] Total diff is under 400 lines (single PR).

## Next Steps

On approval, run `sdd-spec` to write the requirements and scenarios for each of the 5 changes above, then `sdd-design` for the technical approach (envelope structure, retry policy, `deriveSummary` implementation), then `sdd-tasks` for the implementation breakdown.
