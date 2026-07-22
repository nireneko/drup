# Tasks: drup-post-test-fixes

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~570 (15 files: parser rewrite, 5 call sites, 2 CLI commands, error helper, 4 SKILL.md, test expansion) |
| 800-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR (size:exception accepted) |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Plain-text parser + fixtures + call sites (Groups A+C+E) | PR 1 | `go test ./internal/scan/ ./internal/app/ -v` | `drup scan <path>` against real Drupal project | `internal/scan/scan.go`, `internal/scan/testdata/*`, `internal/app/commands.go`, `internal/app/mcp_tools.go`, all test files |
| 2 | CLI commands + SKILL.md sync (Groups B+D) | PR 1 (same) | `go test ./internal/app/ -run 'TestRun$'` | `drup validate <path>`, `drup apply-patch <url> <path>` | `internal/app/app.go`, `internal/app/commands.go`, 4× SKILL.md |

## Phase 1: Plain-Text Scan Parser (Group A — CRITICAL)

- [x] **A-1** Rewrite `scan.Parse()` for plain-text output
  - **Files**: `internal/scan/scan.go`
  - **Acceptance**:
    - Remove `rawProject`, `rawError` JSON structs and `json.Unmarshal` path
    - Implement line-based parser: detect `Project: <name>` lines as block delimiters
    - Extract file:line from `- <path>:<line>` lines via regex
    - Extract message from next non-Rule line
    - Extract rule from `Rule: <rule>` lines
    - Skip `[warning]`, blank, and `------` separator lines
    - Return zero-error `ScanResult` when no `Project:` lines found
    - Preserve existing `classifyPath()` and `model.go` types unchanged
  - **Dependencies**: none

- [x] **A-2** Replace JSON fixtures with plain-text fixtures
  - **Files**: `internal/scan/testdata/upgrade_status_d10.txt`, `upgrade_status_d9.txt`, `upgrade_status_empty.txt`
  - **Acceptance**:
    - Create plain-text fixtures matching real `upgrade_status:analyze` v4.3.x output format
    - D10 fixture: 3 projects (token/contrib, mymodule/custom, mytheme/theme), 4 total errors — same data as current JSON
    - D9 fixture: 1 project (oldmodule/contrib), 1 error
    - Empty fixture: blank or `[warning]`-only content, zero project blocks
    - Delete old `.json` fixture files
  - **Dependencies**: A-1

- [x] **A-3** Update `scan_test.go` for plain-text fixtures
  - **Files**: `internal/scan/scan_test.go`
  - **Acceptance**:
    - Rename fixture references from `.json` → `.txt`
    - `TestParse_D10Fixture`: same assertions (TotalErrs=4, 3 modules, classification checks)
    - `TestParse_D9Fixture`: same assertions (TotalErrs=1, oldmodule, contrib)
    - `TestParse_EmptyFixture`: TotalErrs=0, len(Modules)=0
    - Replace `TestParse_MalformedJSON` with `TestParse_UnparseableInput`: verify graceful zero-result or error
    - Add table-driven `TestParse_PlainText` with subtests: multi-project, single-project, warnings-only, empty-input
  - **Dependencies**: A-2

## Phase 2: Remove `--format=json` From All Call Sites (Group A)

- [x] **A-4** Remove `--format=json` from `RunScan()` in `commands.go`
  - **Files**: `internal/app/commands.go`
  - **Acceptance**:
    - Line 61: change drush args from `"drush", "-r", path, "upgrade_status:analyze", "--all", "--format=json"` → `"drush", "-r", path, "upgrade_status:analyze", "--all"`
    - Existing `TestRunScan_PassesAllFlag` still passes
  - **Dependencies**: A-1

- [x] **A-5** Remove `--format=json` from 4 MCP tool call sites in `mcp_tools.go`
  - **Files**: `internal/app/mcp_tools.go`
  - **Acceptance**:
    - `realHandleScan` (line ~78): remove `"--format=json"` from drush args
    - `realHandleAutofix` (line ~122): remove `"--format=json"` from re-scan drush args
    - `realHandleValidate` (line ~210): remove `"--format=json"` from drush args
    - `realHandleUpgradeScan` (line ~629): remove `"--format=json"` from analyzeArgs
  - **Dependencies**: A-1

## Phase 3: Error Context Helper (Group C)

- [x] **C-1** Add `drushExecError` helper and apply to all drush call sites
  - **Files**: `internal/app/commands.go`, `internal/app/mcp_tools.go`
  - **Acceptance**:
    - Add `func drushExecError(cmd string, args []string, exitCode int, stderr, stdout string) error` in `commands.go`
    - Error message includes: full command string, exit code, stderr (full), stdout (truncated to 500 chars)
    - Replace `fmt.Errorf("exec drush: %w", err)` and `fmt.Errorf("drush exit %d: %s", ...)` patterns in `RunScan` (commands.go)
    - Replace same patterns in `realHandleScan`, `realHandleAutofix`, `realHandleValidate`, `realHandleUpgradeScan` (mcp_tools.go)
    - For parse failures: include command + truncated stdout (500 chars)
  - **Dependencies**: A-4, A-5

## Phase 4: Missing CLI Commands (Group B)

- [x] **B-1** Extract shared logic and add `validate` + `apply-patch` CLI commands
  - **Files**: `internal/app/commands.go`, `internal/app/app.go`
  - **Acceptance**:
    - Extract `DoValidate(projectPath, module string) (*scan.ScanResult, []scan.DepError, error)` from `realHandleValidate` logic in `commands.go`
    - Extract `DoApplyPatch(patchURL, projectPath string) (*patch.Result, error)` from `realHandleApplyPatch` logic in `commands.go`
    - Add `RunValidate(args []string) error` in `commands.go`: parse args, call `DoValidate`, output JSON `{total_errors, errors}`, exit 1 if errors > 0
    - Add `RunApplyPatch(args []string) error` in `commands.go`: parse args, call `DoApplyPatch`, output JSON result
    - Add `case "validate"` and `case "apply-patch"` to `app.go` switch with usage messages
    - Update `printUsage()` to list both new commands
    - Refactor `realHandleValidate` and `realHandleApplyPatch` to call the shared functions
  - **Dependencies**: A-1 (shared validate uses scan.Parse)

## Phase 5: SKILL.md Sync (Group D)

- [x] **D-1** Update all 4 SKILL.md copies
  - **Files**: `internal/packaging/templates/opencode/SKILL.md`, `internal/packaging/templates/claude/SKILL.md`, `internal/packaging/templates/codex/SKILL.md` (+ root if exists)
  - **Acceptance**:
    - Every `drup <cmd>` reference has a matching `case` in `app.go` dispatcher
    - `drup validate <path> [module]` and `drup apply-patch <url> <path>` documented in Stage 4/7
    - Stage numbering is sequential (1-8) matching actual CLI flow
    - All 3 template copies have identical command content
    - Remove any references to non-existent CLI commands
  - **Dependencies**: B-1

## Phase 6: Expand Test Coverage (Group E)

- [x] **E-1** Add `RunScan` CLI integration test with plain-text mock
  - **Files**: `internal/app/commands_test.go`
  - **Acceptance**:
    - `TestRunScan_PlainTextParsing`: mock `drupexec.Run` to return realistic plain-text output (multi-project), verify JSON stdout contains expected `total_errors` and module names
    - `TestRunScan_DrushExitNonZero`: mock exit code 1, verify error includes command + exit code + stderr (tests C-1 error helper)
    - `TestRunScan_ParseFailure`: mock exit 0 with garbage output, verify error includes truncated stdout
    - Update `TestRunScan_PassesAllFlag`: verify `--format=json` is NOT in captured args
  - **Dependencies**: A-3, A-4, C-1

- [x] **E-2** Update MCP tool tests with plain-text mock output
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Acceptance**:
    - `TestRealHandleScan_PlainText`: mock drupexec.Run returning plain text, verify parsed JSON response has correct modules/errors
    - `TestRealHandleValidate_PlainText`: same pattern, verify filtering by module works
    - `TestRealHandleAutofix_RemainingErrors`: mock rector + plain-text re-scan, verify `remaining_errors` count
    - `TestRealHandleUpgradeScan_PlainText`: mock plain-text analyze output, verify module list and total_errors
    - All mocks return plain text, NOT JSON
  - **Dependencies**: A-5, C-1

## Phase 7: Final Verification

- [x] **V-1** Run full test suite and verify all groups
  - **Files**: none (verification only)
  - **Acceptance**:
    - `go test ./...` passes — zero failures
    - `go vet ./...` clean
    - `gofmt -l .` no output
    - No `.json` files remain in `internal/scan/testdata/`
    - No `--format=json` appears in `upgrade_status:analyze` calls (grep verification)
    - `drup validate` and `drup apply-patch` appear in `printUsage()` output
  - **Dependencies**: all above

## Apply Order

Sequential — each phase depends on the parser rewrite (A-1):

```
A-1 → A-2 → A-3 → A-4, A-5 (parallel) → C-1 → B-1 → D-1 → E-1, E-2 (parallel) → V-1
```

Groups A+C are the critical path (parser + call sites + errors). Groups B+D are independent of A+C except for the shared validate logic. Group E validates everything.
