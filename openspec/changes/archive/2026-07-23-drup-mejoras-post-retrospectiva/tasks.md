# Tasks: drup-mejoras-post-retrospectiva

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1800–2200 |
| 400-line budget risk | High |
| Chained PRs recommended | No (size:exception approved) |
| Suggested split | Single PR |
| Delivery strategy | single-pr (unlimited budget approved) |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

## Phase 1: Foundation (stdlib packages)

- [x] 1.1 Create `internal/semver/semver.go` with `Version{Major,Minor,Patch}`, `Parse(s string)`, `(v) Compare(other) int`, `Satisfies(v, constraint string) bool`. Handle `>=`, `^`, `~`, `||` operators. Stdlib only — no external deps.
- [x] 1.2 Create `internal/semver/semver_test.go` — table-driven: parse valid/invalid, compare ordering, satisfies for each operator, edge cases (missing patch, pre-release ignored).
- [x] 1.3 Create `internal/composerutil/webroot.go` with `ReadWebRoot(projectPath string) string`. Reads `composer.json` → `extra.drupal-scaffold.locations.web-root`, falls back to `"web"`.
- [x] 1.4 Create `internal/composerutil/webroot_test.go` — scaffold config present with custom value, scaffold absent (fallback), malformed JSON (fallback).
- [x] 1.5 Create `internal/metrics/metrics.go` — `Collector` struct with `sync.Mutex`, `PipelineStart()`, `StageStart(name)`, `StageEnd(name)`, `RecordCommand()`, `RecordRetry()`, `RecordFileModification()`, `Snapshot() Metrics`. `Default()` singleton. Non-blocking: `defer`+`recover` on all public methods.
- [x] 1.6 Create `internal/metrics/metrics_test.go` — concurrent goroutine stress, panic recovery, snapshot correctness.

## Phase 2: Core Pipeline Changes

- [x] 2.1 Modify `internal/app/commands.go`: replace `isPHP84OrLater` body with `semver.Satisfies`. Replace `isPHPCompatible` in `mcp_tools.go` to use `semver.Satisfies`. Update `commands_test.go` table tests.
- [x] 2.2 Modify `internal/app/commands.go` `RunUpgradeCore`: replace `execRunFn("composer", ...)` calls (lines 865, 882, 892) with `cliRun(cwd, "composer", ...)`. Verify DDEV prefix applied via existing `cliRun` → `envdetect.Detect` → `RunWithEnv`.
- [x] 2.3 Add test in `commands_test.go` for DDEV composer: override `defaultEnvDetector` to return DDEV prefix, verify `cliRun` passes correct args.
- [x] 2.4 Modify `internal/patch/patch.go` `Apply`: replace web root resolution with `composerutil.ReadWebRoot(projectPath)`. Remove any `os.Getwd()` usage for path resolution.
- [x] 2.5 Add tests in `patch_test.go` for custom web root (`docroot`) and fallback (`web`).
- [x] 2.6 Modify `internal/drupalorg/drupalorg.go`: add `PatchSearchResult` struct with `Status`, `Module`, `Searched`, `Message`, `Suggestion`, `Patches []PatchInfo`. Change `SearchPatches` to return `*PatchSearchResult`. Update all callers in `mcp_tools.go`.
- [x] 2.7 Update `drupalorg_test.go` for all 3 statuses: `patches_found`, `no_patches_found`, `error`. Verify no bare `[]` returns.
- [x] 2.8 Modify `internal/app/commands.go` `RunScan`: add empty-dir check for `web/modules/custom/` and `web/themes/custom/`. If both empty, log skip message and return zero-error model.
- [x] 2.9 Add tests in `commands_test.go` for smart bypass: both empty (skip), one has content (proceed).

## Phase 3: New Features (cleanup + preflight + post-D11 gates)

- [x] 3.1 Create `internal/app/cleanup.go` with `RunCleanup(args []string) error`. Gate: check validate exit code (from state or flag). If failed → log skip + exit 0. Otherwise: `cliRun("drush", "pm:uninstall", "upgrade_status", "-y")`, `cliRun("composer", "remove", "drupal/upgrade_status")`, git add + commit. Idempotent: skip steps if module absent.
- [x] 3.2 Create `internal/app/cleanup_test.go` — validate-pass-runs-cleanup, validate-fail-skips, already-removed idempotent, drush-failure-halts.
- [x] 3.3 Add `case "cleanup"` to `Run()` switch in `app.go`. Update `printUsage()`.
- [x] 3.4 Add `cleanup` MCP tool in `mcp_tools.go`.
- [x] 3.5 Add `checkCoreReadiness(projectPath string) ([]PreflightResult, error)` in `commands.go`. Parse `composer.json` `require.drupal/core` constraint. Scan `web/modules/custom/*/*.info.yml` and `web/themes/custom/*/*.info.yml` for `core_version_requirement`. Return blockers list.
- [x] 3.6 Wire `checkCoreReadiness` into `RunPreflight()`. Add fixture-based tests in `preflight_test.go` or `commands_test.go`.
- [x] 3.7 Modify `DoValidate` in `commands.go`: detect core version from `composer.lock`. If >= 11.x: run `drush updb -y` + `drush cr` + `drush status` as gates; run `upgrade_status:analyze` as info-only. If < 11.x: keep existing behavior.
- [x] 3.8 Add tests for post-D11 gate swap: mock `cliRun` to return core 11.x, verify correct command sequence. Test drush status failure halts.

## Phase 4: Metrics + Report + E2E

- [x] 4.1 Wire `metrics.Default()` calls into `Run*` functions: `PipelineStart()` in `RunPreflight`, `StageStart/End` in each `Run*`, `RecordCommand()` in `cliRun`/`execRunFn` wrappers.
- [x] 4.2 Modify `internal/report/report.go`: add `PipelineMetrics *metrics.Metrics` field to `ReportData`. Render in JSON and markdown output.
- [x] 4.3 Update `report_test.go` for metrics section presence.
- [x] 4.4 Create `internal/e2e/pipeline_test.go` — mock-based stage ordering test. Override `cliRun`/`execRunFn` vars. Verify: preflight → scan → validate → cleanup sequence. Verify: cleanup skipped on validate failure.

## Phase 5: Skills

- [x] 5.1 Create `internal/packaging/templates/opencode/skills/drupal-custom-d11-fixes/SKILL.md` — ~50 D11 deprecation patterns with before/after, replacement API, complexity, edge cases.
- [x] 5.2 Create `internal/packaging/templates/opencode/skills/drupal-contrib-patch-writer/SKILL.md` — 4 categories (A: info.yml, B: simple replacements, C: API params, D: escalate).
- [x] 5.3 Duplicate both skills to `claude/` and `codex/` template dirs.
- [x] 5.4 Update `packaging_test.go` to verify skill files exist and contain trigger phrases.

## Phase 6: Verification

- [x] 6.1 Run `go test ./...` — all tests pass.
- [x] 6.2 Run `go vet ./...` — no issues.
- [x] 6.3 Run `go build ./...` — clean build.
