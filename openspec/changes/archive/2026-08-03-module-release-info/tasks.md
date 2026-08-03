# Tasks: Module Release Info MCP Tool

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~350-450 (new fixture + struct fields + ModuleReleaseInfo/curateReleases + tests + handler + 3 doc/count fixes) |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|-----------------------|-----------------|-------------------|
| 1 | Full additive tool (types, fixture, tests, wiring, doc/count fixes) | PR 1 (single, size:exception if >400 lines) | `go test ./internal/drupalorg/... ./internal/mcp/... ./internal/app/...` | N/A — no live network/VCS call; httptest fixture covers integration | Revert the single commit; no schema/state-file change, older binary just lacks the tool |

## Phase 1: Foundation — Types & Parsing

- [x] 1.1 In `internal/drupalorg/drupalorg.go`, add `ReleaseInfoResult`, `ReleaseDetail`, `releaseSecurity` structs exactly per design.md Interfaces/Contracts.
- [x] 1.2 Add `Terms []term \`xml:"terms>term"\`` to `releaseHistoryFull` (project-level only) and `Security releaseSecurity \`xml:"security"\`` to `releaseFull`.
- [x] 1.3 Implement unexported `curateReleases(rh *releaseHistoryFull, coreVersion string) *ReleaseInfoResult`: gate every release on `status == "published"` always; when `coreVersion != ""` additionally require `constraintMatchesDrupal(CoreCompat, majorFromVersion(coreVersion))`; empty `core_compatibility` dropped only under an active filter.
- [x] 1.4 In `curateReleases`, derive `insecure` from a term with `Name == "Release type"` and `strings.TrimSpace(Value) == "Insecure"` (case-sensitive); copy every `Release type` value verbatim into `release_type[]` regardless of recognition (fail open, never error).
- [x] 1.5 Extract project-level `maintenance_status` from `rh.Terms` (`Name == "Maintenance status"`), defaulting to `"unknown"` when absent, independent of release/filter state.
- [x] 1.6 Implement exported `ModuleReleaseInfo(module, coreVersion string) (*ReleaseInfoResult, error)`: call `releaseHistoryBranch`/`FetchReleaseHistory`; on `rh == nil` return `{found:false, status:"not_found", releases:[]}`; on non-nil with zero matching releases return `{found:true, status:"no_releases_found", releases:[]}`; otherwise `{found:true, status:"releases_found", releases:[...]}` via `curateReleases`. Propagate real transport/parse errors as Go `error` (never swallow into `status:"error"`).
  - Deviation: `majorFromVersion` was renamed to the exported `MajorFromVersion` so the `internal/app` MCP handler (a different package) can validate `core_version` before calling `ModuleReleaseInfo`, per design.md's handler-guard description. Not listed in design.md's Interfaces/Contracts section but required for it to compile as described.

## Phase 2: Fixture & Unit/Integration Tests

- [x] 2.1 Create `internal/drupalorg/testdata/release_info_real.xml` (NEW file, additive — do not modify or replace `release_d11.xml`): project `<terms>` with a `Maintenance status` term, at least 2 releases with `Release type` terms (one including `Insecure`), `<security covered="1">`, a single `<core_compatibility>` range, and at least one release with `status="unpublished"` to exercise the always-on gate.
- [x] 2.2 In `internal/drupalorg/drupalorg_test.go`, add table-driven unit tests over `curateReleases` covering: insecure derivation, unknown Release-type term pass-through (no error), published-only gate, `"unknown"` maintenance default, empty `core_compatibility` dropped only under filter.
- [x] 2.3 Add `httptest`-backed integration tests for `ModuleReleaseInfo` (using `SetHTTPClientForTest` + `releaseHistoryVersionURL` override pattern) covering: maintenance status surfaced, filter `"11"` narrows `"^10.2 || ^11"` correctly, unfiltered returns all published releases (excluding unpublished), `<error>` body → `not_found`, zero-release project → `found:true, releases:[]`.
- [x] 2.4 Confirm `TestCheckRelease_HasD11` and all other existing tests referencing `release_d11.xml` remain unmodified and green (regression check only, no code change expected).

## Phase 3: MCP Wiring

- [x] 3.1 In `internal/mcp/server.go`, add `module_release_info` entry to `toolRegistry` with schema: `module_machine_name` (string, required), `core_version` (string, optional, "Drupal core major to filter by, e.g. 11"), `Required: []string{"module_machine_name"}`.
- [x] 3.2 In `internal/mcp/tools.go`, add a placeholder entry for `module_release_info` to `defaultTools()`.
- [x] 3.3 In `internal/app/mcp_tools.go`, implement `realHandleModuleReleaseInfo(args json.RawMessage) (json.RawMessage, error)` mirroring `realHandleContribUpgradePath`: unmarshal params, guard `module_machine_name` with `moduleNamePattern`, guard non-empty `core_version` by returning `fmt.Errorf("invalid core_version: %s", ...)` when `majorFromVersion` yields `0`, then call `drupalorg.ModuleReleaseInfo` and marshal the result.
- [x] 3.4 Register `"module_release_info"` in `WireMCPTools` alongside the other real handlers.

## Phase 4: Tool-Count & Handler Tests

- [x] 4.1 In `internal/mcp/mcp_test.go`, read the current asserted counts at the `len(tools) != N` checks (do not trust design.md's numbers blindly) and update the default-tools count and its error message, and the post-wire-up count and its error message/comment, to reflect the one added tool (currently 24→25 default, 28→29 post-wire, per file inspection).
  - Also renamed `TestServer_PostWireUpCountIs28` to `TestServer_PostWireUpCountIs29` so the test name matches its own assertion.
- [x] 4.2 In `internal/app/mcp_tools_test.go`, add `TestModuleReleaseInfo_InvalidName` and `TestModuleReleaseInfo_InvalidCoreVersion`, mirroring `TestContribUpgradePath_InvalidName`.

## Phase 5: Documentation

- [x] 5.1 In `openspec/specs/mcp-server/spec.md`, correct the pre-existing self-contradictory tool counts that this change's delta already tracks (the `defaultTools()`-based 24→25 baseline/total referenced by the "New Tool Registration" and "Tool Schema Validation" requirements). Do not attempt to reconcile the separate 28-registered-vs-24-placeholder discrepancy — out of scope.
- [x] 5.2 In `docs/mcp-tools.md`, add a new `### 5.23 module_release_info` Tool Dictionary entry (module, core_version params; found/maintenance_status/releases response shape) and update the totals line per the `tools.go:22` mirror convention.

## Phase 6: Full Verification

- [x] 6.1 Run `go test ./...` and `go vet ./...`; confirm all existing tests (including `TestCheckRelease_HasD11`, `UpgradePath`, `ModuleInfo` tests) stay green alongside the new tests.
  - `go build ./...` clean, `go vet ./...` clean, `go test ./...` all packages pass (including the full `internal/drupalorg`, `internal/mcp`, `internal/app` suites).

## Phase 7: Corrective Round (verify FAIL remediation)

- [x] 7.1 (Fixed directly before this round, no apply action needed) `gofmt -w internal/drupalorg/drupalorg.go` — CI formatting gate CRITICAL-1. `gofmt -l .` now empty.
- [x] 7.2 Rewrote `openspec/specs/mcp-server/spec.md` requirement *New Tool Registration*: removed the false "-32602 before executing the handler" claim (CRITICAL-2), removed change-relative "Before this change... (Previously stated ...)" prose in favor of a timeless statement (25 `defaultTools()` / 29 after `WireMCPTools`), renamed/reworded the stale "All 10 new tools are callable" and "Existing tools unchanged" scenarios to drop the hardcoded "10" and include `module_release_info` in the callable-tools list, and reworded the schema-validation scenario to describe the actual contract: handler-level validation error -> JSON-RPC -32603 (internal error); -32602 stays reserved for malformed outer `params` JSON only (confirmed from `Server.handleToolCall`, `internal/mcp/server.go:408-424` — no tool has pre-handler schema validation). Also fixed one more stale "all 10 new tools" reference under *Tool Handler Registration Points* (§ Schema definitions scenario) found while reading the section. Left the pre-existing, separately-tracked *Tool Schema Validation* requirement's own "-32602" scenario and its "(Previously stated 20 tools...)" line untouched — that is WARNING-1/WARNING-2 territory, explicitly out of this change's scope per task 5.1 and the corrective-round instructions.
- [x] 7.3 Added `TestServer_HandleRequest_ModuleReleaseInfoInvalidParamsReturns32603` to `internal/mcp/mcp_test.go`: registers a `module_release_info` handler returning a Go error and asserts the JSON-RPC response carries code -32603, proving the spec scenario's corrected claim in code. Confirmed no pre-existing test anywhere in the repo asserted a JSON-RPC error code for handler-level validation (existing `TestContribUpgradePath_InvalidName`-style tests only assert `err != nil` at the Go-handler level, not the wire-level code), so this closes that gap using the exact same in-package pattern as the neighboring `TestServer_HandleRequest_UnknownTool` (-32601) and `TestServer_HandleRequest_InvalidJSON` (-32700) tests.
- [x] 7.4 Re-ran full verification: `gofmt -l .` empty, `go build ./...` exit 0, `go vet ./...` exit 0, `go test -count=1 ./...` — all 20 packages `ok`, 0 failures.

## Phase 8: Corrective Round 2 (verify FAIL remediation — 3 CRITICAL)

- [x] 8.1 Rewrote `openspec/changes/module-release-info/specs/mcp-server/spec.md` *New Tool Registration* (delta, lines ~29-49) — the round-1 fix had landed only in the baseline (`openspec/specs/mcp-server/spec.md`); the change's own delta was byte-unchanged and still asserted the false "-32602 before executing the handler" claim, which would have reverted the baseline fix at archive (round-2 CRITICAL-1). Replaced the delta's stale 24→25 scenario set with the exact corrected baseline text (25 `defaultTools()` / 29 after `WireMCPTools`; handler-level validation surfaced as -32603; -32602 reserved for malformed outer `params` JSON only), so delta and baseline now agree byte-for-byte on this requirement.
- [x] 8.2 Rewrote the pre-existing baseline *Tool Schema Validation* requirement in `openspec/specs/mcp-server/spec.md` (~line 359-383), which verify found self-contradicting the just-corrected *New Tool Registration* section in the same file. Confirmed via `internal/mcp/server.go:408-425` (`handleToolCall`) that no pre-handler `Required`-field/JSON-Schema validation exists anywhere: malformed outer `params` → -32602, unknown tool → -32601, any handler error → -32603. Replaced the aspirational "SHALL validate all tool inputs against their JSON schemas before execution" / "-32602 for missing required parameter" text with the real, universal contract (handler-level validation, -32603), renaming the scenario to "Missing required parameter surfaces as -32603". Left the delta's own (separately scoped, out-of-scope) copy of *Tool Schema Validation* untouched per the corrective-round instructions — this is a known, flagged residual risk for a follow-up change, not an oversight (see apply-progress).
- [x] 8.3 Added a new `TestCurateReleases` table case ("unsupported project still lists releases normally") in `internal/drupalorg/drupalorg_test.go` covering the previously-UNTESTED *Maintenance Status Extraction → Unsupported project still lists releases* scenario: asserts `MaintenanceStatus == "Unsupported"` and that its one published release (of two, one `unpublished`) is still returned via the same unconditional published-gate as any other project. Followed strict TDD: RED confirmed first with an intentionally wrong `wantMaint` value (failed with `MaintenanceStatus = "Unsupported", want "THIS_WILL_FAIL_INTENTIONALLY_FOR_RED_STEP"`, proving the assertion is wired to real production code), then GREEN confirmed after correcting the expected value (all 7 `TestCurateReleases` subtests PASS). Implementation required no change — `curateReleases` was already value-agnostic; this closes a coverage gap, not a behavior gap.
- [x] 8.4 Re-ran full verification: `gofmt -l .` empty (exit 0), `go build ./...` exit 0, `go vet ./...` exit 0, `go test -count=1 ./...` — all 20 packages `ok`, 0 failures (forced, no cache).

## Phase 9: Corrective Round 3 (verify FAIL remediation — residual archive-revert risk)

- [x] 9.1 The delta's own copy of *Tool Schema Validation* (`openspec/changes/module-release-info/specs/mcp-server/spec.md`, ~lines 52-78), flagged as an accepted "residual risk for a follow-up change" in task 8.2, was in fact fixed directly in this same change round rather than deferred: rewritten to match the corrected baseline text byte-for-byte (confirmed via `diff` in verify round 3). It is no longer a follow-up item — the delta and baseline now agree on both `New Tool Registration` and `Tool Schema Validation`, closing the archive-time revert risk entirely.
