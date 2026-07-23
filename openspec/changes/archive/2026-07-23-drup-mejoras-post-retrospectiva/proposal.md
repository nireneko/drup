# Proposal: drup-mejoras-post-retrospectiva

## Intent

Address findings from a real Drupal 10→11 upgrade retrospective: harden pipeline gates, add cleanup automation, improve CLI robustness, and create two new skills for custom code fixes and contrib patching.

## Scope

### In Scope
- P1: Core readiness check in preflight, cleanup stage (Stage 8), post-D11 validation gates
- P2: Smart no-op bypass, structured `issue_patches` responses, `create_patch` web-root fix, semver `isPHPCompatible`, DDEV-aware composer calls, pipeline metrics, E2E test scaffolding
- P2/P3: Two new skills (`drupal-custom-d11-fixes`, `drupal-contrib-patch-writer`)

### Out of Scope
- Changes to pipeline stage ordering (Core stays after Contrib+Custom)
- Docker-based E2E with real Drupal site (deferred)
- golangci-lint setup
- Any changes to MCP server protocol or installer

## Capabilities

### New Capabilities
- `cleanup-stage`: Post-validation cleanup — uninstall upgrade_status, remove from composer.json, atomic commit. Runs only after validate passes.
- `pipeline-metrics`: Non-blocking telemetry collection (durations, commands, retries) output as JSON in report.

### Modified Capabilities
- `preflight`: Add core readiness check — verify composer.json constraints allow Drupal 11, list incompatible `core_version_requirement` modules, abort early with blockers report.
- `validation-gates`: Post-D11 gate swap — when core >= 11.x, replace upgrade_status:analyze gate with `drush updb -y` + `drush cr` + `drush status` success criteria.
- `issue-patches`: Structured JSON responses instead of empty arrays — include status, message, and suggestion fields.
- `apply-patch`: Fix `create_patch` to read web root from `composer.json` scaffold config instead of `os.Getwd()`.
- `core-upgrade`: DDEV-aware composer calls — use `ddev composer` when DDEV detected.
- `report`: Include pipeline metrics section in JSON and markdown output.
- `scan`: Smart no-op bypass — skip rector/custom-loop when no custom code exists.

## Approach

Single PR (user approved unlimited review budget). Implement in priority order: P1 first (pipeline correctness), P2 second (robustness), P2/P3 last (skills). Each item gets unit tests per strict_tdd. Semver comparison uses `github.com/Masterminds/semver/v3` (already indirect dep via composer tooling) or minimal stdlib implementation.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/app/preflight.go` | Modified | Add core readiness constraint check |
| `internal/app/validate.go` | Modified | Post-D11 gate logic (drush updb/cr/status) |
| `internal/app/scan.go` | Modified | Smart no-op detection for empty custom dirs |
| `internal/coreupgrade/coreupgrade.go` | Modified | DDEV-aware composer execution |
| `internal/drupalorg/issue_patches.go` | Modified | Structured JSON responses |
| `internal/patch/create.go` | Modified | Web root from composer scaffold config |
| `internal/exec/composer.go` | Modified | DDEV prefix for composer calls |
| `internal/report/report.go` | Modified | Metrics section in output |
| `internal/app/cleanup.go` | New | Cleanup stage (uninstall upgrade_status, commit) |
| `internal/metrics/` | New | Pipeline telemetry collection |
| `internal/semver/` | New or import | Semver comparison for PHP compatibility |
| `skills/drupal-custom-d11-fixes/` | New | Skill: D11 deprecation catalog |
| `skills/drupal-contrib-patch-writer/` | New | Skill: contrib patch guidelines |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Semver lib adds dependency weight | Low | Use stdlib `strconv` split if Masterminds not already direct |
| Cleanup stage runs on failed validate | Low | Gate on validate exit code; skip cleanup on failure |
| Post-D11 gate misses edge cases | Med | Keep upgrade_status as optional info (not gate) |
| Metrics overhead slows pipeline | Low | Non-blocking goroutine; drop metrics on timeout |
| Skills become stale with Drupal updates | Med | Version-catalog in skill; note last-updated date |

## Rollback Plan

`git revert` the single PR commit. No database or state changes — all modifications are in Go source and skill files. Cleanup stage is additive (new stage), removing it reverts to prior pipeline.

## Dependencies

- `github.com/Masterminds/semver/v3` (if not already direct) for semver comparison
- Existing `cliRun` / `envdetect` infrastructure for DDEV-aware calls

## Complexity Estimate

| Priority | Items | Estimate |
|----------|-------|----------|
| P1 | Core readiness, cleanup stage, post-D11 gates | 4–6h |
| P2 | No-op bypass, structured responses, web-root fix, semver, DDEV composer, metrics, E2E scaffold | 8–12h |
| P2/P3 | Two new skills | 4–6h |
| **Total** | **12 items** | **16–24h** |

## Success Criteria

- [ ] Preflight aborts with clear report when composer constraints block D11
- [ ] Cleanup stage removes upgrade_status only after validate passes
- [ ] Post-D11 validate uses drush updb/cr/status, not upgrade_status:analyze as gate
- [ ] Rector/custom-loop skipped with log message when no custom code exists
- [ ] `issue_patches` returns structured JSON with status/message/suggestion
- [ ] `create_patch` reads web root from composer scaffold config
- [ ] `isPHPCompatible` uses semver comparison
- [ ] Composer calls use `ddev composer` under DDEV
- [ ] Report includes pipeline metrics (duration, commands, retries)
- [ ] E2E test scaffolding exists (mock-based, not real Drupal)
- [ ] Both new skills load and trigger correctly
- [ ] `go test ./...` and `go vet ./...` pass
