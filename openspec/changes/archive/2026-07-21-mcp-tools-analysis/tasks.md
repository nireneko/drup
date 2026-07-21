# Tasks: MCP Tools Analysis — 10 New Tools for Drupal Upgrade Orchestrator

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~950 (10 tools × ~60 lines handler + ~30 lines test + ~80 lines new packages) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (Phase 1: foundation) → PR 2 (Phase 2+3: intelligence + patches) → PR 3 (Phase 4 + wiring: reporting + registration) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Foundation: envdetect package + RunWithEnv + detect_env/composer_require/drush_exec handlers | PR 1 | `go test ./internal/envdetect/... ./internal/exec/...` | N/A — unit tests with t.TempDir() and mock execCommand | `internal/envdetect/`, RunWithEnv in exec.go, 3 handlers in tools.go + mcp_tools.go |
| 2 | Intelligence + Patches: contrib_upgrade_path + upgrade_scan + patch_status + patch_rollback | PR 2 | `go test ./internal/drupalorg/... ./internal/app/...` | N/A — unit tests with httptest + temp git repos | drupalorg.go additions, 4 handlers in tools.go + mcp_tools.go |
| 3 | Reporting + Wiring: generate_report + module_info + drupal_version_matrix + full registration | PR 3 | `go test ./... && go vet ./... && go build ./...` | N/A — unit tests with httptest + static map tests | report handler, drupalorg ModuleInfo, version matrix, final tool registration |

## Phase 1: Foundation — Environment Detection & Safe Execution

- [x] 1.1 Create `internal/envdetect/envdetect.go`: `Environment` type (ddev/lando/docker4drupal/direct/unknown), `Detection` struct, `Detector` interface, `DefaultDetector` with `sync.Mutex` + `map[string]*Detection` cache. Detection order: `.ddev/` → `.lando.yml` → `docker-compose.yml` + `*drupal*` → `composer.json` → unknown.
  - **Files**: `internal/envdetect/envdetect.go` (new)
  - **Test**: `go vet ./internal/envdetect/...`
  - **Effort**: medium

- [x] 1.2 Create `internal/envdetect/envdetect_test.go`: table-driven tests — each env type with marker files in `t.TempDir()`, cache hit returns same pointer, `force_detect` bypasses cache, missing path returns unknown, ambiguous markers resolve by priority.
  - **Files**: `internal/envdetect/envdetect_test.go` (new)
  - **Test**: `go test ./internal/envdetect/...`
  - **Effort**: medium

- [x] 1.3 Add `RunWithEnv(prefix []string, cmd string, args ...string)` to `internal/exec/exec.go`. Prepends prefix tokens: `["ddev"] + ["composer", "require", "pkg"]` → executes `ddev composer require pkg`. Empty prefix falls through to existing `Run()` path.
  - **Files**: `internal/exec/exec.go` (modify, ~10 lines)
  - **Test**: `go vet ./internal/exec/...`
  - **Effort**: small

- [x] 1.4 Add `RunWithEnv` tests to `internal/exec/exec_test.go`: verify prefix prepending via mock `execCommand`, empty prefix matches `Run()` behavior, multi-token prefix (docker compose exec).
  - **Files**: `internal/exec/exec_test.go` (modify)
  - **Test**: `go test ./internal/exec/...`
  - **Effort**: small

- [x] 1.5 Add `detect_env` placeholder handler in `internal/mcp/tools.go` and real handler in `internal/app/mcp_tools.go`. Real handler: unmarshal `{project_path, force_detect?}`, call `envdetect.Detect()`, return `{environment, command_prefix, detected_at}`. Validate `project_path` is non-empty absolute path.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run DetectEnv`
  - **Effort**: small

- [x] 1.6 Add `composer_require` placeholder + real handler. Real handler: validate package format regex, call `detect_env` for prefix, run `composer require --dry-run` via `RunWithEnv`, if pass run actual `composer require` with 60s timeout, parse installed version from stdout.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run ComposerRequire`
  - **Effort**: medium

- [x] 1.7 Add `drush_exec` placeholder + real handler. Real handler: check blocklist (`sql-drop`, `site-install`, `sql-sanitize`, `php-eval`, `core:execute-cli`), reject shell metacharacters (`;`, `|`, `&&`, `||`, `$()`, backticks), call `detect_env` for prefix, build command with `--root=<path>`, execute via `RunWithEnv`, parse JSON output if `format=json`.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run DrushExec`
  - **Effort**: medium

- [x] 1.8 **RED test — threat: shell injection**: verify `composer_require` rejects package `"; rm -rf /"` and `drush_exec` blocks `sql-drop`. Add to existing test files.
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Test**: `go test ./internal/app/... -run "ShellInjection|Blocklist"`
  - **Effort**: small

## Phase 2: Upgrade Intelligence

- [x] 2.1 Add `FetchReleaseHistory()` and `UpgradePath()` to `internal/drupalorg/drupalorg.go`. `FetchReleaseHistory` fetches full XML from `/release-history/{module}/{drupal_version}`. `UpgradePath` parses all releases, filters by target Drupal compatibility, prefers latest stable, returns `UpgradeRecommendation` with recommended + alternatives (max 5).
  - **Files**: `internal/drupalorg/drupalorg.go` (modify)
  - **Test**: `go vet ./internal/drupalorg/...`
  - **Effort**: medium

- [x] 2.2 Add tests for `UpgradePath` in `internal/drupalorg/drupalorg_test.go`: httptest serving testdata XML, verify stable preference, verify target filtering, verify 404 fallback chain, verify no-compatible-releases returns nil recommendation.
  - **Files**: `internal/drupalorg/drupalorg_test.go` (modify), `internal/drupalorg/testdata/` (add XML fixture)
  - **Test**: `go test ./internal/drupalorg/... -run UpgradePath`
  - **Effort**: medium

- [x] 2.3 Add `contrib_upgrade_path` placeholder + real handler. Validates `module_machine_name` regex, calls `drupalorg.UpgradePath()`, returns structured result.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run ContribUpgradePath`
  - **Effort**: small

- [x] 2.4 Add `upgrade_scan` placeholder + real handler. Orchestrates: check `composer.json` for `drupal/upgrade_status` → `composer_require` if missing → `drush_exec pm:list` to check enabled → `drush_exec en upgrade_status` if disabled → `drush_exec upgrade_status:analyze --format=json` → parse + filter by scope/module.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run UpgradeScan`
  - **Effort**: large

- [x] 2.5 **RED test — threat: path traversal in upgrade_scan**: verify `upgrade_scan` rejects `project_path` containing `..` segments. Add to `mcp_tools_test.go`.
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Test**: `go test ./internal/app/... -run PathTraversal`
  - **Effort**: small

## Phase 3: Patch Lifecycle

- [x] 3.1 Add `patch_status` placeholder + real handler. Read `composer.json` `extra.patches`, search for matching URL/package. Check git log for patch-related commits via `git -C <path> log --oneline --grep=<url>`. Return `{is_applied, commit_hash, registered_in_composer, patch_info}`.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run PatchStatus`
  - **Effort**: medium

- [x] 3.2 Add `patch_rollback` placeholder + real handler. Verify patch applied via `patch_status`, check working tree clean (`git status --porcelain`), `git revert <commit> --no-edit`, remove from `composer.json` `extra.patches`, run `composer update <package>` via `RunWithEnv`. Atomic: revert first, modify config only on success.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run PatchRollback`
  - **Effort**: medium

- [x] 3.3 **RED test — threat: dirty working tree**: verify `patch_rollback` returns error when `git status --porcelain` shows uncommitted changes. Use temp git repo with uncommitted file.
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Test**: `go test ./internal/app/... -run DirtyWorkingTree`
  - **Effort**: small

- [x] 3.4 **RED test — threat: non-git directory**: verify `patch_rollback` returns error when `project_path` is not a git repository.
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Test**: `go test ./internal/app/... -run NonGitDir`
  - **Effort**: small

## Phase 4: Reporting & Info

- [x] 4.1 Add `generate_report` placeholder + real handler. Collect data from `composer.json` patches + scan state. Call `report.GenerateJSON()` → write `<path>/drup-report.json`. Call `report.GenerateMarkdown()` → write `<path>/drup-report.md`. Return paths + summary.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run GenerateReport`
  - **Effort**: medium

- [x] 4.2 Add `ModuleInfo()` to `internal/drupalorg/drupalorg.go`. Query `api-d7/node.json?name=<module>`, parse title/downloads/maintainers. Fetch latest release from release-history. Return `ModuleMetadata`.
  - **Files**: `internal/drupalorg/drupalorg.go` (modify)
  - **Test**: `go vet ./internal/drupalorg/...`
  - **Effort**: medium

- [x] 4.3 Add `ModuleInfo` tests in `internal/drupalorg/drupalorg_test.go`: httptest for api-d7 JSON + release-history XML, verify field extraction, verify 404 returns error.
  - **Files**: `internal/drupalorg/drupalorg_test.go` (modify)
  - **Test**: `go test ./internal/drupalorg/... -run ModuleInfo`
  - **Effort**: small

- [x] 4.4 Add `module_info` placeholder + real handler. Validate module name regex, call `drupalorg.ModuleInfo()`, return structured result.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run ModuleInfo`
  - **Effort**: small

- [x] 4.5 Add `drupal_version_matrix` placeholder + real handler. Static map: D9→PHP 7.3/8.1, D10→PHP 8.1/8.3, D11→PHP 8.3/8.4. Lookup by `drupal_version` or reverse-lookup by `php_version`. Return error for unknown version.
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go test ./internal/app/... -run VersionMatrix`
  - **Effort**: small

- [x] 4.6 Write table-driven tests for `drupal_version_matrix`: lookup by drupal version, lookup by php version, no filter returns full matrix, unknown version returns error.
  - **Files**: `internal/app/mcp_tools_test.go`
  - **Test**: `go test ./internal/app/... -run VersionMatrix`
  - **Effort**: small

## Phase 5: Final Wiring & Verification

- [x] 5.1 Verify all 10 new tools registered in `defaultTools()` (`internal/mcp/tools.go`) and `WireMCPTools()` (`internal/app/mcp_tools.go`). Total: 17 tools (7 existing + 10 new).
  - **Files**: `internal/mcp/tools.go`, `internal/app/mcp_tools.go`
  - **Test**: `go build ./...`
  - **Effort**: small

- [x] 5.2 Run full test suite + vet: `go test ./... && go vet ./...`. Verify existing 7 tools unchanged. Verify all new tools respond to `tools/list`.
  - **Files**: none (verification only)
  - **Test**: `go test ./... && go vet ./...`
  - **Effort**: small
