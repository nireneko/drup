# Tasks: drup-retrospective-fixes

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~530 (6 fixes + tests) |
| 400-line budget risk | Medium |
| Chained PRs recommended | No |
| Suggested split | Single PR with `size:exception` |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Medium

## Phase 1: Foundation — Exit Code Semantics (P0)

- [x] 1.1 Add `isScanExitOK(exitCode int) bool` helper in `internal/app/commands.go` — returns true for 0 and 3, false otherwise
- [x] 1.2 **RED test**: table-driven test for `isScanExitOK` — assert true for {0,3}, false for {1,2,4,-1} in `internal/app/commands_test.go`
- [x] 1.3 Wire `isScanExitOK` into `RunScan` (`internal/app/commands.go:74-96`) — replace `if exitCode != 0` with `if !isScanExitOK(exitCode)`; parse stdout on exit 3; treat empty stdout + exit 3 as error
- [x] 1.4 Wire `isScanExitOK` into `DoValidate` (`internal/app/commands.go:209-238`) — same pattern as RunScan
- [x] 1.5 Wire `isScanExitOK` into `realHandleScan` (`internal/app/mcp_tools.go:69-91`) — replace exit code check, parse stdout on exit 3
- [x] 1.6 Wire `isScanExitOK` into `realHandleAutofix` re-scan (`internal/app/mcp_tools.go:121-128`) — accept exit 3 for remaining error count
- [x] 1.7 **Integration test**: exit code 3 with valid stdout → parsed result; exit code 3 with empty stdout → error. Use fake exec in `internal/app/commands_test.go`

**Commit**: `fix: handle exit code 3 as success-with-findings in scan/validate`
**Files**: `internal/app/commands.go`, `internal/app/mcp_tools.go`, `internal/app/commands_test.go`
**Risk**: Medium — must distinguish "findings" from "drush crash"

## Phase 2: Foundation — DDEV-Aware Execution (P0)

- [x] 2.1 Add `cliRun(projectPath, cmd, args...)` helper in `internal/app/commands.go` — calls `defaultEnvDetector.Detect()` then `drupexec.RunWithEnv(prefix, cmd, args...)`
- [x] 2.2 **RED test**: `cliRun` passes correct prefix for DDEV vs direct. Mock `envdetect.Detector` in `internal/app/commands_test.go`
- [x] 2.3 Replace `drupexec.Run("drush", "-r", path, ...)` with `cliRun(path, "drush", "upgrade_status:analyze", "--all", "--root="+path)` in `RunScan`
- [x] 2.4 Replace drush invocation in `DoValidate` with `cliRun` using `--root=`
- [x] 2.5 Replace drush invocation in `realHandleScan` with env-aware call using `--root=`
- [x] 2.6 Replace drush invocations in `RunPreflight` (`internal/app/commands.go:437-601`) — use `cliRun` for `drush en`, `drush config:delete`, dev dep installs
- [x] 2.7 Replace drush invocation in `RunFix` re-scan path (calls `RunScan` which is already fixed — verify)
- [x] 2.8 Replace drush invocations in `RunUpgradeCore` (`internal/app/commands.go:804-821`) — `drush updb`, `drush status` via `cliRun`

**Commit**: `fix: use DDEV-aware execution with --root= in all CLI commands`
**Files**: `internal/app/commands.go`, `internal/app/mcp_tools.go`, `internal/app/commands_test.go`
**Dependencies**: Phase 1 (uses isScanExitOK in RunScan/DoValidate)
**Risk**: Low — `envdetect` and `RunWithEnv` already exist and work

## Phase 3: Core Implementation — MCP Tool Schemas (P0)

- [x] 3.1 Add `toolSchema` struct and `jsonSchemaProperty` struct in `internal/mcp/server.go`
- [x] 3.2 Add `toolRegistry` map with schemas for all 20 tools in `internal/mcp/server.go` — each entry has Description, Properties (name, type, description), Required fields
- [x] 3.3 Update `handleListTools` (`internal/mcp/server.go:102-117`) — replace empty `inputSchema: {"type": "object"}` with `toolRegistry[name]` lookup; emit full JSON Schema with properties and required
- [x] 3.4 **RED test**: assert `tools/list` returns non-empty `inputSchema.properties` for all 20 tools in `internal/mcp/server_test.go`
- [x] 3.5 **RED test**: assert `scan` tool schema includes `project_path` property with type "string" and required array

**Commit**: `feat: expose JSON Schema parameter definitions for all 20 MCP tools`
**Files**: `internal/mcp/server.go`, `internal/mcp/server_test.go`
**Risk**: Low — additive change, no existing behavior removed

## Phase 4: Core Implementation — Contrib Compound Constraint (P1)

- [x] 4.1 Add `fetchInfoYML(module, branch string) (string, error)` in `internal/drupalorg/drupalorg.go` — fetches `https://git.drupalcode.org/project/<module>/-/raw/<branch>/<module>.info.yml`
- [x] 4.2 Add `constraintMatchesDrupal(constraint string, drupalMajor int) bool` in `internal/drupalorg/drupalorg.go` — splits on `||`, parses each sub-constraint with semver logic, returns true if any matches
- [x] 4.3 **RED test**: `constraintMatchesDrupal` table-driven — `"^10.3 || ^11.0"` with 11 → true; `"^11.0"` with 11 → true; `"^9.0 || ^10.0"` with 11 → false; `">=10 <12"` with 11 → true
- [x] 4.4 Update `CheckRelease` / `parseReleaseXML` (`internal/drupalorg/drupalorg.go:95-149`) — after finding latest release version, derive branch name, call `fetchInfoYML`, parse `core_version_requirement`, use `constraintMatchesDrupal` to set `HasD11`
- [x] 4.5 **Integration test**: `CheckRelease` with mocked HTTP returning info.yml with compound constraint → `has_d11_release: true` in `internal/drupalorg/drupalorg_test.go`

**Commit**: `fix: parse compound core_version_requirement constraints for contrib D11 check`
**Files**: `internal/drupalorg/drupalorg.go`, `internal/drupalorg/drupalorg_test.go`
**Risk**: Medium — git.drupalcode.org URL format may vary per module branch naming

## Phase 5: Core Implementation — PHP 8.4 Deprecation Suppression (P1)

- [x] 5.1 Add `detectPHPVersion(projectPath string) (string, error)` in `internal/app/commands.go` — uses `cliRun` to execute `php -r "echo PHP_VERSION;"`, parses output
- [x] 5.2 Add `isPHP84OrLater(version string) bool` in `internal/app/commands.go` — parses major.minor, returns true if >= 8.4
- [x] 5.3 Add `patchSettingsPHP(projectPath string) error` in `internal/app/commands.go` — reads `web/sites/default/settings.php`, checks if suppression line already present (idempotent), finds DDEV include block end (or EOF), appends `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED);`, creates `.bak` backup before write
- [x] 5.4 **RED test**: `isPHP84OrLater` — "8.4.2" → true, "8.3.0" → false, "8.4.0" → true
- [x] 5.5 **RED test**: `patchSettingsPHP` idempotency — patch twice, second call is no-op; verify `.bak` created; verify line placed after DDEV include block
- [x] 5.6 Wire into `RunPreflight` — after dev dep installs, call `detectPHPVersion`, if >= 8.4 call `patchSettingsPHP`, add `PreflightResult{Check: "php84_compat", Pass: true}`

**Commit**: `fix: auto-patch settings.php to suppress E_DEPRECATED on PHP 8.4+`
**Files**: `internal/app/commands.go`, `internal/app/commands_test.go`
**Dependencies**: Phase 2 (uses cliRun)
**Risk**: Low — append-only, idempotent, backup created

## Phase 6: Integration — Report Data Collection (P2)

- [x] 6.1 Update `RunReport` (`internal/app/commands.go:166-198`) — call `DoValidate(path, "")` to get live scan data, populate `TotalErrors` from `len(filtered)`, populate `Resolved` and `Pending` from scan results
- [x] 6.2 Update `realHandleGenerateReport` (`internal/app/mcp_tools.go:968-1047`) — same pattern: call `DoValidate` when `IncludeScanData` is true, populate `data.TotalErrors` and error arrays
- [x] 6.3 **RED test**: `RunReport` with mocked `DoValidate` returning 15 errors → report JSON has `total_errors: 15` and populated arrays in `internal/app/commands_test.go`

**Commit**: `fix: populate report with real scan data instead of hardcoded zeros`
**Files**: `internal/app/commands.go`, `internal/app/mcp_tools.go`, `internal/app/commands_test.go`
**Dependencies**: Phase 1 (DoValidate uses isScanExitOK)
**Risk**: Low — reuses existing DoValidate

## Phase 7: Verification

- [ ] 7.1 Run `go test ./...` — all tests pass
- [ ] 7.2 Run `go vet ./...` — no warnings
- [ ] 7.3 Run `gofmt -l .` — no formatting issues
- [ ] 7.4 Verify each spec scenario has corresponding test coverage
- [ ] 7.5 Manual smoke test: `drup scan` under DDEV returns exit 0 with findings
- [ ] 7.6 Manual smoke test: `drup contrib webform` returns `has_d11_release: true`
