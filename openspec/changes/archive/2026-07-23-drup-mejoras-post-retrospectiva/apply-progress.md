# Apply Progress: drup-mejoras-post-retrospectiva

## Status: ALL TASKS COMPLETE ✅

**Mode**: Strict TDD
**Total tasks**: 34/34 complete

## Completed Phases

### Phase 1: Foundation ✅
- [x] 1.1-1.2: `internal/semver/` — Version struct, Parse, Compare, Satisfies (stdlib only)
- [x] 1.3-1.4: `internal/composerutil/` — ReadWebRoot from composer.json scaffold config
- [x] 1.5-1.6: `internal/metrics/` — Singleton Collector with sync.Mutex, non-blocking

### Phase 2: Core Pipeline Changes ✅
- [x] 2.1: Replaced isPHP84OrLater/isPHPCompatible with semver.Satisfies
- [x] 2.2-2.3: Replaced execRunFn("composer") with cliRun for DDEV awareness
- [x] 2.4-2.5: Fixed create_patch to use composerutil.ReadWebRoot + project_path
- [x] 2.6-2.7: Added PatchSearchResult struct, changed SearchPatches return type
- [x] 2.8-2.9: Smart no-op bypass in RunScan for empty custom dirs

### Phase 3: New Features ✅
- [x] 3.1-3.2: Created cleanup.go with RunCleanup (gate on validate, idempotent)
- [x] 3.3: Added "cleanup" case to Run() switch + printUsage
- [x] 3.4: Added cleanup MCP tool
- [x] 3.5-3.6: Added checkCoreReadiness in preflight (composer + info.yml scanning)
- [x] 3.7-3.8: Modified DoValidate for post-D11 gate swap (drush updb/cr/status)

### Phase 4: Metrics + Report + E2E ✅
- [x] 4.1: Wired metrics.Default() snapshot into RunReport
- [x] 4.2: Added PipelineMetrics field to ReportData (JSON + markdown)
- [x] 4.3: Updated report tests for metrics section
- [x] 4.4: Created internal/e2e/pipeline_test.go (mock-based stage tests)

### Phase 5: Skills ✅
- [x] 5.1: Created drupal-custom-d11-fixes SKILL.md (~50 patterns)
- [x] 5.2: Created drupal-contrib-patch-writer SKILL.md (4 categories)
- [x] 5.3: Duplicated to claude/ and codex/ template dirs
- [x] 5.4: Updated packaging_test.go for skill verification

### Phase 6: Verification ✅
- [x] 6.1: go test ./... — 19 packages pass
- [x] 6.2: go vet ./... — no issues
- [x] 6.3: go build ./... — clean build

## Files Created
| File | Description |
|------|-------------|
| `internal/semver/semver.go` | Semver parsing and constraint evaluation |
| `internal/semver/semver_test.go` | Table-driven tests for all operators |
| `internal/composerutil/webroot.go` | ReadWebRoot from composer.json scaffold |
| `internal/composerutil/webroot_test.go` | Scaffold config + fallback tests |
| `internal/metrics/metrics.go` | Non-blocking pipeline metrics collector |
| `internal/metrics/metrics_test.go` | Concurrent safety + panic recovery tests |
| `internal/app/cleanup.go` | RunCleanup stage (uninstall upgrade_status) |
| `internal/app/cleanup_test.go` | Gate, idempotency, error path tests |
| `internal/e2e/pipeline_test.go` | Mock-based stage ordering tests |
| `internal/packaging/templates/*/skills/drupal-custom-d11-fixes/SKILL.md` | 50 D11 deprecation patterns |
| `internal/packaging/templates/*/skills/drupal-contrib-patch-writer/SKILL.md` | 4-category patch guidelines |

## Files Modified
| File | Changes |
|------|---------|
| `internal/app/commands.go` | semver import, isPHP84OrLater→semver, cliRun for composer, hasNoCustomCode, checkCoreReadiness, DoValidate split (pre/post D11), metrics wiring |
| `internal/app/mcp_tools.go` | semver+composerutil imports, isPHPCompatible→semver, create_patch uses project_path, PatchSearchResult callers, cleanup MCP tool |
| `internal/app/app.go` | cleanup case in Run() switch + printUsage |
| `internal/app/commands_test.go` | Updated mocks for cliRun (composer via RunWithEnv), DDEV composer test, smart bypass tests, post-D11 gate tests |
| `internal/app/preflight_test.go` | Core readiness check tests |
| `internal/drupalorg/drupalorg.go` | PatchSearchResult struct, SearchPatches returns *PatchSearchResult |
| `internal/drupalorg/drupalorg_test.go` | Updated for PatchSearchResult, added 3 status tests |
| `internal/report/report.go` | PipelineMetrics field, markdown metrics section |
| `internal/report/report_test.go` | Metrics section tests |
| `internal/packaging/packaging_test.go` | Skill file existence + trigger phrase tests |
