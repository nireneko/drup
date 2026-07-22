```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:ca48c6566e2f6a137efc3464fb91ad4119388c116084a629002e8a5f39931e5e
verdict: pass with warnings
blockers: 0
critical_findings: 0
requirements: 8/8
scenarios: 17/19
test_command: go test ./...
test_exit_code: 0
test_output_hash: sha256:ca48c6566e2f6a137efc3464fb91ad4119388c116084a629002e8a5f39931e5e
build_command: go build ./... && go vet ./...
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Verification Report

**Change**: fix-drup-pipeline-bugs
**Version**: N/A
**Mode**: Strict TDD

### Completeness
| Metric | Value |
|--------|-------|
| Tasks total | 23 |
| Tasks complete | 23 |
| Tasks incomplete | 0 |

### Build & Tests Execution
**Build**: ✅ Passed
```text
$ go build ./...
(exit 0, no output)
$ go vet ./...
(exit 0, no output)
$ gofmt -l .
(no output — clean)
```

**Tests**: ✅ 87 passed / ❌ 0 failed / ⚠️ 0 skipped
```text
$ go test -count=1 ./...
ok  	github.com/nireneko/drup/internal/app		1.044s
ok  	github.com/nireneko/drup/internal/coreupgrade	0.224s
ok  	github.com/nireneko/drup/internal/drupalorg	0.018s
ok  	github.com/nireneko/drup/internal/envdetect	0.006s
ok  	github.com/nireneko/drup/internal/exec		0.013s
ok  	github.com/nireneko/drup/internal/gitops		0.188s
ok  	github.com/nireneko/drup/internal/installer	0.029s
ok  	github.com/nireneko/drup/internal/mcp		0.006s
ok  	github.com/nireneko/drup/internal/packaging	0.004s
ok  	github.com/nireneko/drup/internal/patch		0.067s
ok  	github.com/nireneko/drup/internal/patchreconcile	0.109s
ok  	github.com/nireneko/drup/internal/report		0.006s
ok  	github.com/nireneko/drup/internal/scan		0.005s
ok  	github.com/nireneko/drup/internal/state		0.007s
ok  	github.com/nireneko/drup/internal/update		0.036s
```

**Coverage**: 48.0% statements (internal/app) → ➖ No threshold configured

### Spec Compliance Matrix
| Requirement | Scenario | Test | Result |
|-------------|----------|------|--------|
| scan > Drush Invocation | Full project scan | `commands_test.go > TestRunScan_PassesAllFlag` | ✅ COMPLIANT |
| scan > Drush Invocation | Empty results without --all | `commands_test.go > TestRunScan_PassesAllFlag` | ✅ COMPLIANT |
| core-upgrade > Composer Execution | Composer update with advisory bypass | `commands_test.go > TestRunUpgradeCore_Integration` | ✅ COMPLIANT |
| core-upgrade > Composer Execution | Composer not available | `commands_test.go > TestRunUpgradeCore_ComposerNotFound` | ✅ COMPLIANT |
| core-upgrade > Backup | Backup created and cleaned on success | `commands_test.go > TestRunUpgradeCore_Integration` | ✅ COMPLIANT |
| core-upgrade > Backup | Backup retained on failure | (no test) | ❌ UNTESTED |
| preflight > Dev Dependency Installation | All dev deps already installed | `preflight_test.go > TestRunPreflight_*` (existing) | ✅ COMPLIANT |
| preflight > Dev Dependency Installation | Missing dev deps | `preflight_test.go > TestRunPreflight_*` (existing) | ✅ COMPLIANT |
| preflight > Dev Dependency Installation | Config conflict before enable | `preflight_test.go > TestRunPreflight_DeletesUpdateSettingsBeforeEnable` | ✅ COMPLIANT |
| preflight > Dev Dependency Installation | Composer require fails | `preflight_test.go` (existing) | ✅ COMPLIANT |
| mcp-server > scan Tool | scan with valid path | `mcp_tools_test.go > TestRealHandleScan_PassesAllFlag` | ✅ COMPLIANT |
| mcp-server > scan Tool | scan with invalid path | `mcp_tools_test.go` (existing path validation) | ✅ COMPLIANT |
| mcp-server > autofix Tool | autofix applies rector | `mcp_tools_test.go > TestRealHandleAutofix_PassesAllFlagInRescan` | ✅ COMPLIANT |
| mcp-server > validate Tool | validate with zero errors | `mcp_tools_test.go > TestRealHandleValidate_PassesAllFlagWhenNoModule` | ✅ COMPLIANT |
| mcp-server > validate Tool | validate with remaining errors | `mcp_tools_test.go > TestRealHandleValidate_PassesAllFlagWhenNoModule` | ✅ COMPLIANT |
| mcp-server > validate Tool | validate scoped to module | `mcp_tools_test.go > TestRealHandleValidate_PassesModuleNameWhenSet` | ✅ COMPLIANT |
| mcp-server > upgrade_scan Tool | upgrade_scan full lifecycle | `mcp_tools_test.go > TestRealHandleUpgradeScan_DeletesUpdateSettingsBeforeEnable` | ✅ COMPLIANT |
| mcp-server > upgrade_scan Tool | upgrade_scan idempotent | (partial — test does not exercise already-enabled path) | ⚠️ PARTIAL |
| mcp-server > upgrade_scan Tool | upgrade_scan with config conflict | `mcp_tools_test.go > TestRealHandleUpgradeScan_DeletesUpdateSettingsBeforeEnable` | ✅ COMPLIANT |

**Compliance summary**: 17/19 scenarios fully compliant, 1 untested, 1 partial

### Correctness (Static Evidence)
| Requirement | Status | Notes |
|------------|--------|-------|
| `--all` flag at 4 call sites | ✅ Implemented | `commands.go:61`, `mcp_tools.go:78,122,205` — all verified |
| Validate module scoping | ✅ Implemented | `mcp_tools.go:205-207` — conditional `params.Module` vs `--all` |
| Composer advisory bypass | ✅ Implemented | `commands.go:667` — `composer config policy.advisories.block false` |
| Composer `-W` + `--no-update` | ✅ Implemented | `commands.go:681-686` — correct flags |
| Composer `update -W` | ✅ Implemented | `commands.go:694` — full dependency resolution |
| Config conflict pre-enable | ✅ Implemented | `commands.go:472`, `mcp_tools.go:610` — `config:delete update.settings` |
| Backup cleanup on success | ✅ Implemented | `commands.go:664` — `defer os.Remove(backupPath)` |
| Checkpoint in error message | ✅ Implemented | `commands.go:647` — includes `RollbackCheckpoint` |

### Coherence (Design)
| Decision | Followed? | Notes |
|----------|-----------|-------|
| `--all` inline at each site | ✅ Yes | No helper function — matches design |
| Module name when `params.Module != ""` | ✅ Yes | `mcp_tools.go:205-207` |
| `composer config` before require | ✅ Yes | `commands.go:667` |
| Pre-emptive `config:delete` before `en` | ✅ Yes | `commands.go:472`, `mcp_tools.go:610` |
| `defer os.Remove` for backup | ✅ Yes | `commands.go:664` — but see WARNING below |

### TDD Compliance
| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Found in apply-progress |
| All tasks have tests | ✅ | 23/23 tasks have test files |
| RED confirmed (tests exist) | ✅ | 7/7 task groups have test files verified |
| GREEN confirmed (tests pass) | ✅ | 7/7 test groups pass on execution |
| Triangulation adequate | ✅ | 6 task groups triangulated, 1 single-case (acceptable) |
| Safety Net for modified files | ✅ | 3/3 modified files had safety net |

**TDD Compliance**: 6/6 checks passed

---

### Test Layer Distribution
| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 9 new + 78 existing | 3 (`commands_test.go`, `mcp_tools_test.go`, `preflight_test.go`) | `go test` |
| Integration | 2 (`TestRunUpgradeCore_Integration`, `TestRunUpgradeCore_ErrorMessageIncludesCheckpoint`) | 1 (`commands_test.go`) | `go test` |
| E2E | 0 | 0 | N/A |
| **Total** | **87** | **3** | |

---

### Changed File Coverage
| File | Line % | Rating |
|------|--------|--------|
| `internal/app/commands.go` (RunScan) | 71.4% | ⚠️ Acceptable |
| `internal/app/commands.go` (RunPreflight) | 90.0% | ✅ Excellent |
| `internal/app/commands.go` (RunUpgradeCore) | 84.0% | ✅ Excellent |
| `internal/app/mcp_tools.go` (realHandleScan) | 76.9% | ⚠️ Acceptable |
| `internal/app/mcp_tools.go` (realHandleAutofix) | 87.0% | ✅ Excellent |
| `internal/app/mcp_tools.go` (realHandleValidate) | 62.5% | ⚠️ Acceptable |
| `internal/app/mcp_tools.go` (realHandleUpgradeScan) | 65.6% | ⚠️ Acceptable |

Coverage analysis for changed functions only — all ≥ 62%, most ≥ 80%.

---

### Assertion Quality
✅ All assertions verify real behavior. No tautologies, ghost loops, or smoke-only tests found.

Specifically verified:
- `TestRunScan_PassesAllFlag`: asserts `--all` in captured drush args
- `TestRunUpgradeCore_Integration`: asserts 3 composer calls with correct args, backup cleanup, JSON output fields
- `TestRunUpgradeCore_ErrorMessageIncludesCheckpoint`: asserts error string contains "checkpoint"
- `TestRealHandleValidate_PassesModuleNameWhenSet`: asserts module name present AND `--all` absent
- `TestRealHandleUpgradeScan_DeletesUpdateSettingsBeforeEnable`: asserts call ordering (config:delete before en)
- `TestRunPreflight_DeletesUpdateSettingsBeforeEnable`: asserts call ordering + target (`update.settings`)

**Assertion quality**: 0 CRITICAL, 0 WARNING

---

### Quality Metrics
**Linter**: ✅ `go vet ./...` clean (exit 0)
**Formatter**: ✅ `gofmt -l .` clean (no output)
**Type Checker**: ✅ `go build ./...` clean (exit 0)

### Issues Found
**CRITICAL**: None

**WARNING**:
1. **Spec deviation: "Backup retained on failure" scenario** — The spec (`core-upgrade/spec.md`) requires that `composer.json.bak` MUST remain after a failed upgrade for rollback purposes. The implementation uses `defer os.Remove(backupPath)` at `commands.go:664`, which removes the backup on ALL exit paths including failure. The design document explicitly chose this approach ("defer guarantees cleanup on all exit paths"), but it contradicts the spec scenario. No test covers this scenario. The design decision should be reconciled with the spec — either update the spec to accept cleanup-on-failure, or change the implementation to only remove the backup on the success path.

2. **"upgrade_scan idempotent" scenario only partially covered** — `TestRealHandleUpgradeScan_DeletesUpdateSettingsBeforeEnable` tests the full lifecycle but does not exercise the idempotent path where `upgrade_status` is already installed and enabled (the `pm:list` check at `mcp_tools.go:597-607`). The test always goes through the install/enable path.

**SUGGESTION**:
1. Consider adding a test for `TestRunUpgradeCore_BackupRetainedOnFailure` that verifies backup file existence after a failed upgrade — this would resolve WARNING #1 regardless of which direction the spec/design reconciliation goes.
2. Consider adding a `TestRealHandleUpgradeScan_Idempotent` test where `pm:list` returns `upgrade_status` as enabled, verifying that `config:delete` and `en` are skipped.

### Verdict
**PASS WITH WARNINGS**

All 23 tasks complete, 87 tests pass, build/vet/fmt clean. 17/19 spec scenarios fully compliant. Two warnings: (1) design-spec conflict on backup retention during failure, (2) idempotent upgrade_scan path untested. No blockers.
