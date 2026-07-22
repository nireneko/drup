# Tasks: Test Coverage for Recent Fixes and MCP Handlers

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~680 (prod: ~20 hooks, test: ~660) |
| 400-line budget risk | Medium |
| 800-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR (within 800-line review budget) |
| Delivery strategy | auto-forecast |
| Chain strategy | pending |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Testability hooks + Phase 1 regression tests | PR 1 | `go test ./internal/update/ ./internal/installer/ ./internal/app/ -run "Backup|CrossDevice|WriteCommand|RunUninstall" -short` | N/A — unit tests with t.TempDir() | All test files + 3 prod hook files; revert removes tests and hooks cleanly |
| 2 | Phase 2 MCP handler happy-path + helper tests | PR 2 | `go test ./internal/app/ -run "ComposerRequire|DrushExec|ContribUpgrade|UpgradeScan|PatchStatus|PatchRollback|ModuleInfo|ParseInstalled|HasPackage|ExtractZip" -short` | N/A — mocked exec/HTTP, no external tools | mcp_tools_test.go additions only; independent of Phase 1 |

## Phase 1: Testability Hooks (Foundation)

- [ ] 1.1 Convert `Run` and `RunWithEnv` in `internal/exec/exec.go` from `func` to `var` (function variables). Zero caller changes; enables test override. ~4 lines.
  - Files: `internal/exec/exec.go`
  - Verify: `go build ./...`
  - Est: 4 lines

- [ ] 1.2 Add `SetHTTPClientForTest(*http.Client) func()` to `internal/drupalorg/drupalorg.go`. Saves/restores `httpClient` var. ~6 lines.
  - Files: `internal/drupalorg/drupalorg.go`
  - Verify: `go build ./...`
  - Est: 6 lines

- [ ] 1.3 Add override point vars to `internal/app/commands.go`: `stateLoadFn`, `osExecutableFn`, `osUserHomeDirFn`, `stateRemoveFn`. Update `RunUninstall` to use them. ~10 lines.
  - Files: `internal/app/commands.go`
  - Verify: `go build ./... && go test ./internal/app/ -short`
  - Est: 10 lines

## Phase 2: Regression Tests (Self-Update + Installer)

- [ ] 2.1 Add `TestBackupBinary_WritesToHomeBackupsDir` in `internal/update/upgrade_test.go`. Set HOME to `t.TempDir()`, call `BackupBinary`, verify backup at `<HOME>/.drup/backups/drup.bak` and NOT adjacent to source.
  - Files: `internal/update/upgrade_test.go`
  - Depends: 1.1 (for override if needed)
  - Verify: `go test ./internal/update/ -run TestBackupBinary_WritesToHomeBackupsDir -v`
  - Est: 35 lines

- [ ] 2.2 Add `TestReplaceBinary_CrossDeviceCopy` in `internal/update/upgrade_test.go`. Use separate `t.TempDir()` for src/dst to simulate cross-device. Verify content copied correctly via `copyFile` path (not rename).
  - Files: `internal/update/upgrade_test.go`
  - Verify: `go test ./internal/update/ -run TestReplaceBinary_CrossDeviceCopy -v`
  - Est: 40 lines

- [ ] 2.3 Add `TestWriteCommand_OpenCode` and `TestWriteCommand_Codex` in `internal/installer/installer_test.go`. Use `t.TempDir()` fixtures. Verify wrapper content and correct output path for each adapter.
  - Files: `internal/installer/installer_test.go`
  - Verify: `go test ./internal/installer/ -run TestWriteCommand -v`
  - Est: 80 lines

## Phase 3: RunUninstall Command Tests

- [ ] 3.1 Add `TestRunUninstall_StateDrivenAdapterSelection` in `internal/app/commands_test.go`. Override `stateLoadFn` to return state with `["claude", "opencode"]`. Verify both adapters' Remove methods called.
  - Files: `internal/app/commands_test.go`
  - Depends: 1.3
  - Verify: `go test ./internal/app/ -run TestRunUninstall_StateDriven -v`
  - Est: 40 lines

- [ ] 3.2 Add `TestRunUninstall_DryRunOutput` in `internal/app/commands_test.go`. Override `stateLoadFn`, pass `--dry-run`. Capture stdout, verify paths listed without removal.
  - Files: `internal/app/commands_test.go`
  - Depends: 1.3
  - Verify: `go test ./internal/app/ -run TestRunUninstall_DryRun -v`
  - Est: 35 lines

- [ ] 3.3 Add `TestRunUninstall_ForceWithMissingState` in `internal/app/commands_test.go`. Override `stateLoadFn` to return error, pass `--force`. Verify proceeds without error.
  - Files: `internal/app/commands_test.go`
  - Depends: 1.3
  - Verify: `go test ./internal/app/ -run TestRunUninstall_Force -v`
  - Est: 25 lines

- [ ] 3.4 Add `TestRunUninstall_SelfRemovalError` in `internal/app/commands_test.go`. Override `osExecutableFn` to return non-writable path. Verify error reported without panic.
  - Files: `internal/app/commands_test.go`
  - Depends: 1.3
  - Verify: `go test ./internal/app/ -run TestRunUninstall_SelfRemoval -v`
  - Est: 30 lines

## Phase 4: MCP Handler Happy-Path Tests

- [ ] 4.1 Add table-driven `TestRealHandleComposerRequire` in `internal/app/mcp_tools_test.go`. Cases: invalid JSON, missing package, happy path with mocked `drupexec.Run`. Verify `success: true` and parsed version.
  - Files: `internal/app/mcp_tools_test.go`
  - Depends: 1.1
  - Verify: `go test ./internal/app/ -run TestRealHandleComposerRequire -v -short`
  - Est: 40 lines

- [ ] 4.2 Add table-driven `TestRealHandleDrushExec` in `internal/app/mcp_tools_test.go`. Cases: invalid JSON, blocked command, happy path with mocked `drupexec.RunWithEnv`.
  - Files: `internal/app/mcp_tools_test.go`
  - Depends: 1.1
  - Verify: `go test ./internal/app/ -run TestRealHandleDrushExec_HappyPath -v -short`
  - Est: 35 lines

- [ ] 4.3 Add `TestRealHandleContribUpgradePath_HappyPath` in `internal/app/mcp_tools_test.go`. Use `drupalorg.SetHTTPClientForTest` + `httptest.Server` returning valid release XML. Verify `recommended_upgrade` in response.
  - Files: `internal/app/mcp_tools_test.go`
  - Depends: 1.2
  - Verify: `go test ./internal/app/ -run TestRealHandleContribUpgradePath_HappyPath -v -short`
  - Est: 35 lines

- [ ] 4.4 Add `TestRealHandleUpgradeScan_HappyPath` in `internal/app/mcp_tools_test.go`. Use `t.TempDir()` with minimal `composer.json`. Mock `drupexec.Run` for drush. Verify `total_errors` and `modules` in response.
  - Files: `internal/app/mcp_tools_test.go`
  - Depends: 1.1
  - Verify: `go test ./internal/app/ -run TestRealHandleUpgradeScan_HappyPath -v -short`
  - Est: 40 lines

- [ ] 4.5 Add `TestRealHandlePatchStatus_HappyPath` in `internal/app/mcp_tools_test.go`. Use `t.TempDir()` with git init + patch commit + `composer.json` extra.patches. Skip in `-short`. Verify `is_applied: true`.
  - Files: `internal/app/mcp_tools_test.go`
  - Verify: `go test ./internal/app/ -run TestRealHandlePatchStatus_HappyPath -v`
  - Est: 45 lines

- [ ] 4.6 Add `TestRealHandlePatchRollback_HappyPath` in `internal/app/mcp_tools_test.go`. Use `t.TempDir()` with git repo + patch commit. Skip in `-short`. Verify `success: true` and `reverted_commit`.
  - Files: `internal/app/mcp_tools_test.go`
  - Verify: `go test ./internal/app/ -run TestRealHandlePatchRollback_HappyPath -v`
  - Est: 40 lines

- [ ] 4.7 Add `TestRealHandleModuleInfo_HappyPath` in `internal/app/mcp_tools_test.go`. Use `drupalorg.SetHTTPClientForTest` + `httptest.Server` returning valid module JSON. Verify title, maintainers, download count.
  - Files: `internal/app/mcp_tools_test.go`
  - Depends: 1.2
  - Verify: `go test ./internal/app/ -run TestRealHandleModuleInfo_HappyPath -v -short`
  - Est: 35 lines

## Phase 5: Helper Function Tests

- [ ] 5.1 Add table-driven `TestParseInstalledVersion` in `internal/app/mcp_tools_test.go`. Cases: valid `composer.lock` with version, missing package, malformed JSON. Verify extracted version string.
  - Files: `internal/app/mcp_tools_test.go`
  - Verify: `go test ./internal/app/ -run TestParseInstalledVersion -v -short`
  - Est: 30 lines

- [ ] 5.2 Add table-driven `TestHasPackage` in `internal/app/mcp_tools_test.go`. Cases: `composer.json` with package in require, without package, empty require. Verify boolean result.
  - Files: `internal/app/mcp_tools_test.go`
  - Verify: `go test ./internal/app/ -run TestHasPackage -v -short`
  - Est: 25 lines

- [ ] 5.3 Add `TestExtractZip` in `internal/app/mcp_tools_test.go`. Create real `.zip` in `t.TempDir()`, call `extractZip`, verify extracted files exist with correct content.
  - Files: `internal/app/mcp_tools_test.go`
  - Verify: `go test ./internal/app/ -run TestExtractZip -v -short`
  - Est: 30 lines

## Phase 6: Verification

- [ ] 6.1 Run full test suite: `go test ./... -short`. Verify all existing + new tests pass.
- [ ] 6.2 Run `go vet ./...`. Verify clean.
- [ ] 6.3 Check coverage delta: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out`. Verify overall coverage >= 70%.
