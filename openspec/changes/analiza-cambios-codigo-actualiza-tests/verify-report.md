# Verification Report: analiza-cambios-codigo-actualiza-tests

## Change Summary

**Change**: Test coverage for recent fixes and MCP handlers  
**Mode**: Standard verification (not strict TDD)  
**Verdict**: **FAIL**

---

## Completeness

| Phase | Tasks | Status |
|-------|-------|--------|
| Phase 1: Testability hooks | 3/3 | ✅ Complete |
| Phase 2: Regression tests | 3/3 | ✅ Complete |
| Phase 3: RunUninstall tests | 4/4 | ✅ Complete |
| Phase 4: MCP handler tests | 0/7 | ❌ Not implemented |
| Phase 5: Helper function tests | 0/3 | ❌ Not implemented |
| Phase 6: Verification | 1/3 | ⚠️ Partial (tests run, vet clean, coverage < 70%) |

**Total**: 11/20 tasks complete (55%)

---

## Build & Test Evidence

### Commands Executed

```bash
# Test execution
go test ./... -short -v 2>&1
# Exit code: 1 (FAIL)

# Coverage
go test ./... -short -coverprofile=/tmp/verify-coverage.out
# Exit code: 1 (FAIL)
go tool cover -func=/tmp/verify-coverage.out | grep "total:"
# Output: total: 55.1%

# Vet
go vet ./...
# Exit code: 0 (clean)
```

### Test Results

**FAIL**: `internal/packaging` — `TestRender_OpenCode`  
**Error**: `missing commands/drup.md for opencode`  
**Root cause**: Test was modified to expect `commands/drup.md` in the opencode template, but the template file does not exist. This is a test introduced by this change that fails.

**All other packages**: PASS  
- `internal/app`: 31.8% coverage, all tests pass
- `internal/update`: 79.6% coverage, all tests pass (including new backup/cross-device tests)
- `internal/installer`: 69.4% coverage, all tests pass (including new WriteSkill/WriteCommand/RemoveMCPConfig tests)
- `internal/exec`: 100% coverage
- `internal/drupalorg`: 81.2% coverage

### Coverage

**Overall**: 55.1% (target: ≥ 70%)  
**Gap**: -14.9 percentage points

**Package coverage**:
- `cmd/drup`: 0.0%
- `internal/app`: 31.8% (MCP handlers not tested)
- `internal/drupalorg`: 81.2%
- `internal/envdetect`: 97.1%
- `internal/exec`: 100.0%
- `internal/gitops`: 73.3%
- `internal/installer`: 69.4%
- `internal/mcp`: 30.8%
- `internal/packaging`: 90.0%
- `internal/patch`: 62.7%
- `internal/report`: 93.5%
- `internal/scan`: 95.0%
- `internal/state`: 75.0%
- `internal/update`: 79.6%

---

## Spec Compliance Matrix

### self-update/spec.md

| Requirement | Scenarios | Status | Evidence |
|-------------|-----------|--------|----------|
| Backup Location Regression | 2 | ✅ PASS | `TestBackupBinary_WritesToHomeBackupsDir` verifies backup at `~/.drup/backups/drup.bak` and no adjacent backup. Restore scenario not explicitly tested but covered by existing `TestUpgrade_FullFlow`. |
| Cross-Device Copy Regression | 1 | ✅ PASS | `TestReplaceBinary_CrossDeviceCopy` verifies content copied correctly via separate TempDirs. |
| ETXTBSY Atomic Staging Regression | 1 | ⚠️ PARTIAL | `TestAtomicReplace` verifies temp-file-then-rename pattern. Not explicitly named as ETXTBSY regression test, but behavior is covered. |
| Archive Extraction from .tar.gz Regression | 1 | ✅ PASS | `TestExtractBinaryFromTarGz` (existing) covers nested binary extraction with 4 sub-tests. |

**Compliance**: 3.5/4 requirements fully compliant (87.5%)

### installer/spec.md

| Requirement | Scenarios | Status | Evidence |
|-------------|-----------|--------|----------|
| WriteSkill Directory Structure | 2 | ✅ PASS | `TestWriteSkill_CreatesDirectoryStructure` (table-driven, 3 adapters) verifies `<name>/SKILL.md` directory structure. Second scenario (Install with both agents) partially covered by `TestInstall_WritesFiles`. |
| WriteCommand Adapter Tests | 2 | ✅ PASS | `TestWriteCommand_OpenCode`, `TestWriteCommand_ClaudeIsNoop`, `TestWriteCommand_CodexIsNoop` verify adapter-specific behavior. |
| RunUninstall Command Tests | 4 | ✅ PASS | `TestRunUninstall_StateDrivenAdapterSelection`, `TestRunUninstall_DryRunOutput`, `TestRunUninstall_ForceWithMissingState`, `TestRunUninstall_SelfRemovalError` cover all 4 scenarios. |
| RemoveMCPConfig Per-Agent Path Regression | 2 | ✅ PASS | `TestClaudeAdapter_RemoveMCPConfig`, `TestOpenCodeAdapter_RemoveMCPConfig_PreservesOtherKeys`, `TestOpenCodeAdapter_RemoveMCPConfig_RemovesEmptyMCP`, `TestOpenCodeAdapter_RemoveMCPConfig_Idempotent`, `TestCodexAdapter_RemoveMCPConfig` cover both scenarios. |

**Compliance**: 4/4 requirements fully compliant (100%)

### mcp-server/spec.md

| Requirement | Scenarios | Status | Evidence |
|-------------|-----------|--------|----------|
| MCP Handler Happy-Path Tests | 10 | ❌ FAIL | **NOT IMPLEMENTED**. No tests for `composer_require`, `drush_exec`, `contrib_upgrade_path`, `upgrade_scan`, `patch_status`, `patch_rollback`, `generate_report`, `module_info`, `detect_env`, `drupal_version_matrix` happy paths. |
| Helper Function Tests | 5 | ❌ FAIL | **NOT IMPLEMENTED**. No tests for `parseInstalledVersion`, `hasPackage`, `extractZip`. |
| Invalid Input Tests for All New Handlers | 2 | ❌ FAIL | **NOT IMPLEMENTED**. No invalid-JSON or missing-params tests for the 10 new handlers. |

**Compliance**: 0/3 requirements compliant (0%)

---

## Design Coherence

| Design Decision | Implementation | Status |
|-----------------|----------------|--------|
| `exec.Run` and `exec.RunWithEnv` as function variables | ✅ Implemented in `internal/exec/exec.go` (lines 46, 55) | Aligned |
| `drupalorg.SetHTTPClientForTest` helper | ✅ Implemented in `internal/drupalorg/drupalorg.go` (lines 20-24) | Aligned |
| `RunUninstall` testability hooks (`stateLoadFn`, `osExecutableFn`, `osUserHomeDirFn`, `stateRemoveFn`) | ✅ Implemented in `internal/app/commands.go` (lines 275-278) | Aligned |
| `NewServer` signature change (add `version` parameter) | ✅ Implemented in `internal/mcp/server.go` (line 48), tests updated | Aligned (but not in design.md) |
| `AgentAdapter` extended with `CommandsDir`, `WriteCommand`, `MCPConfigPath` | ✅ Implemented in `internal/installer/installer.go` | Aligned (but not in design.md) |

**Coherence**: 5/5 design decisions implemented correctly. Two additional changes (NewServer signature, AgentAdapter extensions) were not in design.md but are reasonable and tested.

---

## Issues

### CRITICAL

1. **FAILING TEST**: `TestRender_OpenCode` in `internal/packaging/packaging_test.go`  
   - **What**: Test expects `commands/drup.md` in opencode template, but file does not exist.
   - **Impact**: `go test ./...` exits non-zero. CI will fail.
   - **Fix**: Either add `templates/opencode/commands/drup.md` template, or remove the assertion from the test.
   - **Spec reference**: Not in any spec — this is a test bug.

2. **MISSING IMPLEMENTATION**: Phase 4 — MCP handler happy-path tests (7 tasks)  
   - **What**: No tests for 10 new MCP handlers.
   - **Impact**: `mcp-server/spec.md` requirements 1 and 3 are untested. 17 scenarios untested.
   - **Spec reference**: `mcp-server/spec.md` lines 5-67, 103-116.

3. **MISSING IMPLEMENTATION**: Phase 5 — Helper function tests (3 tasks)  
   - **What**: No tests for `parseInstalledVersion`, `hasPackage`, `extractZip`.
   - **Impact**: `mcp-server/spec.md` requirement 2 is untested. 5 scenarios untested.
   - **Spec reference**: `mcp-server/spec.md` lines 69-101.

4. **COVERAGE GAP**: Overall coverage 55.1% < 70% target  
   - **What**: Coverage target not met.
   - **Impact**: Proposal success criterion not met.
   - **Spec reference**: `proposal.md` line 74.

### WARNING

1. **DESIGN DEVIATION**: `NewServer` signature change not in design.md  
   - **What**: `NewServer(out io.Writer, version string)` adds a `version` parameter. Design.md does not mention this change.
   - **Impact**: Minor — change is reasonable and tested, but not documented in design.
   - **Recommendation**: Update design.md to document this change.

2. **DESIGN DEVIATION**: `AgentAdapter` extensions not in design.md  
   - **What**: `CommandsDir()`, `WriteCommand()`, `MCPConfigPath()` methods added to adapters. Design.md does not mention these.
   - **Impact**: Minor — changes are reasonable and tested, but not documented in design.
   - **Recommendation**: Update design.md to document these methods.

3. **PARTIAL COVERAGE**: ETXTBSY regression test not explicitly named  
   - **What**: `TestAtomicReplace` covers the behavior, but is not named as an ETXTBSY regression test.
   - **Impact**: Minor — behavior is tested, but spec traceability is weaker.
   - **Recommendation**: Rename or add a comment linking `TestAtomicReplace` to the ETXTBSY spec requirement.

### SUGGESTION

1. **Add RestoreBinary test**: `self-update/spec.md` scenario "Restore reads from ~/.drup/backups/" is not explicitly tested. Add `TestRestoreBinary_ReadsFromHomeBackupsDir` to make this explicit.

2. **Add integration test for Install with multiple agents**: `installer/spec.md` scenario "WriteSkill for each detected agent" is only partially covered. Add a test that installs to both Claude and OpenCode and verifies both.

3. **Document testability hooks in design.md**: The design.md mentions testability hooks but does not list all of them. Update the "File Changes" table to include all hooks added.

---

## Testability Hooks Verification

| Hook | Location | Used by Tests | Status |
|------|----------|---------------|--------|
| `exec.Run` (var) | `internal/exec/exec.go:46` | Not used (Phase 4 not implemented) | ✅ Implemented, ❌ Not used |
| `exec.RunWithEnv` (var) | `internal/exec/exec.go:55` | Not used (Phase 4 not implemented) | ✅ Implemented, ❌ Not used |
| `drupalorg.SetHTTPClientForTest` | `internal/drupalorg/drupalorg.go:20` | Not used (Phase 4 not implemented) | ✅ Implemented, ❌ Not used |
| `stateLoadFn` | `internal/app/commands.go:275` | `commands_test.go` (4 tests) | ✅ Implemented, ✅ Used |
| `osExecutableFn` | `internal/app/commands.go:276` | `commands_test.go` (4 tests) | ✅ Implemented, ✅ Used |
| `osUserHomeDirFn` | `internal/app/commands.go:277` | `commands_test.go` (4 tests) | ✅ Implemented, ✅ Used |
| `stateRemoveFn` | `internal/app/commands.go:278` | `commands_test.go` (4 tests) | ✅ Implemented, ✅ Used |

**Summary**: 7/7 hooks implemented. 4/7 hooks used by tests. 3/7 hooks (exec.Run, exec.RunWithEnv, SetHTTPClientForTest) implemented but not used because Phase 4 was not implemented.

---

## Verdict

**FAIL**

### Blocking Issues

1. **Failing test**: `TestRender_OpenCode` must be fixed (either add template or remove assertion).
2. **Missing implementation**: Phase 4 (MCP handler tests) and Phase 5 (helper function tests) must be implemented to meet spec requirements.
3. **Coverage target not met**: 55.1% < 70% target.

### Recommendations

1. **Immediate**: Fix `TestRender_OpenCode` by adding `templates/opencode/commands/drup.md` or removing the assertion.
2. **Next**: Implement Phase 4 (MCP handler tests) using the testability hooks already in place.
3. **Next**: Implement Phase 5 (helper function tests).
4. **Optional**: Add explicit `TestRestoreBinary_ReadsFromHomeBackupsDir` for spec traceability.
5. **Optional**: Update design.md to document `NewServer` signature change and `AgentAdapter` extensions.

### What Works Well

- Phase 1-3 tests are well-structured, table-driven where appropriate, and use `t.TempDir()` correctly.
- Testability hooks follow the existing pattern (`execCommand`, `executableFn`, etc.) and are minimal.
- RunUninstall tests cover all 4 spec scenarios with proper state-driven mocking.
- Installer tests cover WriteSkill, WriteCommand, and RemoveMCPConfig for all 3 adapters.
- Update regression tests cover backup location, cross-device copy, and tar.gz extraction.
- `go vet` is clean.

### Effort to Pass

- **Fix failing test**: 5 minutes (add template or remove assertion).
- **Implement Phase 4**: 2-3 hours (10 handlers × ~20 lines each, using existing hooks).
- **Implement Phase 5**: 30 minutes (3 helpers × ~30 lines each).
- **Total**: ~3-4 hours to reach PASS.

---

## Appendix: Test Output Summary

```
?   	github.com/nireneko/drup/cmd/drup	[no test files]
ok  	github.com/nireneko/drup/internal/app	0.718s	coverage: 31.8%
ok  	github.com/nireneko/drup/internal/drupalorg	0.015s	coverage: 81.2%
ok  	github.com/nireneko/drup/internal/envdetect	0.020s	coverage: 97.1%
ok  	github.com/nireneko/drup/internal/exec	0.015s	coverage: 100.0%
ok  	github.com/nireneko/drup/internal/gitops	0.192s	coverage: 73.3%
ok  	github.com/nireneko/drup/internal/installer	0.034s	coverage: 69.4%
ok  	github.com/nireneko/drup/internal/mcp	0.006s	coverage: 30.8%
FAIL	github.com/nireneko/drup/internal/packaging	0.005s	coverage: 90.0%
ok  	github.com/nireneko/drup/internal/patch	0.061s	coverage: 62.7%
ok  	github.com/nireneko/drup/internal/report	0.005s	coverage: 93.5%
ok  	github.com/nireneko/drup/internal/scan	0.005s	coverage: 95.0%
ok  	github.com/nireneko/drup/internal/state	0.014s	coverage: 75.0%
ok  	github.com/nireneko/drup/internal/update	0.029s	coverage: 79.6%
FAIL
```

**Total test time**: ~1.1s (short mode)  
**Total packages**: 15 (1 no tests, 1 FAIL, 13 PASS)  
**Overall coverage**: 55.1%
