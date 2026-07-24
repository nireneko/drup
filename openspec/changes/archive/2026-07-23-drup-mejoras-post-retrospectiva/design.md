# Design: drup-mejoras-post-retrospectiva

## Technical Approach

12 improvements organized in 3 priority tiers. The pipeline is agent-orchestrated (SKILL.md drives stage sequencing via CLI commands) — there is no Go-level stage runner. Each improvement maps to a CLI command or MCP tool modification, preserving the existing pattern of package-level var overrides for testability.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| Cleanup stage integration | New `drup cleanup` CLI command + MCP tool | Auto-run after validate in Go | Matches existing pattern — agent orchestrates, Go provides commands. No stage runner exists in Go. |
| Semver implementation | New `internal/semver/` package, stdlib only | `github.com/Masterminds/semver/v3` | go.mod has zero deps. Adding a dep for one function is overkill. Stdlib `strconv.Split` handles `>=`, `^`, `~`, `\|\|`. |
| Pipeline metrics | Singleton `internal/metrics/` with `sync.Mutex` | Per-stage passing, goroutine collector | No central orchestrator in Go — commands are independent. Singleton with `Start/Stop/Record` called from each `Run*` function. Non-blocking: `defer` with recover. |
| Web root resolution | New `internal/composerutil/` helper | Inline in patch.go | Reused by cleanup, preflight, and patch. Single function: `ReadWebRoot(projectPath) string`. |
| DDEV composer | Modify `RunUpgradeCore` to use `cliRun` instead of `execRunFn` | New wrapper function | `cliRun` already does env detection + `RunWithEnv`. Just replace `execRunFn("composer", ...)` calls with `cliRun(cwd, "composer", ...)`. |
| E2E scaffolding | `internal/e2e/` with interface-based mock for `cliRun` | Docker-based real tests | Spec says "no real Drupal". Mock the `cliRun`/`execRun` boundary. Test stage sequencing by driving `Run*` functions with injected mocks. |
| Skills location | `internal/packaging/templates/{agent}/skills/` | Separate `skills/` dir at root | Skills are agent-installed assets. Existing `packaging/templates/` already handles per-agent rendering. Add sub-dirs for each skill. |

## Data Flow

### Cleanup Stage (new)

```
Agent (SKILL.md Stage 8)
  │
  ▼
drup cleanup <project-path>
  │
  ├─ Check validate exit code (from state/flag)
  │   └─ If failed → log + exit 0 (skip)
  ├─ cliRun("drush", "pm:uninstall", "upgrade_status", "-y")
  ├─ cliRun("composer", "remove", "drupal/upgrade_status")
  ├─ execRun("git", "-C", path, "add", "-A")
  └─ execRun("git", "-C", path, "commit", "-m", "chore(cleanup): ...")
```

### Post-D11 Validation Gate Swap

```
DoValidate(projectPath, module)
  │
  ├─ Detect core version (composer.lock)
  │
  ├─ IF core >= 11.x:
  │   ├─ cliRun("drush", "updb", "-y")      ← primary gate
  │   ├─ cliRun("drush", "cr")               ← primary gate
  │   ├─ cliRun("drush", "status")           ← primary gate (exit 0 = pass)
  │   └─ cliRun("drush", "upgrade_status:analyze", "--all")  ← info only
  │
  └─ IF core < 11.x:
      └─ existing upgrade_status:analyze gate (unchanged)
```

### Pipeline Metrics

```
metrics.Collector (singleton)
  │
  ├─ PipelineStart()        ← called from RunPreflight or first command
  ├─ StageStart(name)       ← called at entry of each Run* function
  ├─ StageEnd(name)         ← called at exit of each Run* function
  ├─ RecordCommand()        ← called from cliRun/execRun wrappers
  ├─ RecordRetry()          ← called on retry paths
  ├─ RecordFileModification() ← called on git commit paths
  └─ Snapshot() → Metrics   ← called from RunReport for JSON output
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/app/cleanup.go` | Create | `RunCleanup(args)` — cleanup stage command |
| `internal/app/cleanup_test.go` | Create | Tests for cleanup gate, idempotency, error paths |
| `internal/semver/semver.go` | Create | `Parse`, `Compare`, `Satisfies` — stdlib semver |
| `internal/semver/semver_test.go` | Create | Table-driven tests for constraint operators |
| `internal/metrics/metrics.go` | Create | Singleton collector with `sync.Mutex` |
| `internal/metrics/metrics_test.go` | Create | Non-blocking behavior, concurrent safety |
| `internal/composerutil/webroot.go` | Create | `ReadWebRoot(projectPath) string` |
| `internal/composerutil/webroot_test.go` | Create | Scaffold config + fallback tests |
| `internal/e2e/pipeline_test.go` | Create | Mock-based stage sequencing tests |
| `internal/app/commands.go` | Modify | Add `cleanup` case to `Run()` switch; add core readiness to `RunPreflight()`; modify `DoValidate()` for post-D11 gates; replace `isPHPCompatible`; replace `isPHP84OrLater` with semver |
| `internal/app/mcp_tools.go` | Modify | Add `cleanup` tool; modify `issue_patches` response; modify `create_patch` web root; wire metrics into `generate_report` |
| `internal/app/preflight.go` | Modify | (preflight logic is in commands.go — add core readiness check function) |
| `internal/drupalorg/drupalorg.go` | Modify | `SearchPatches` returns `PatchSearchResult` struct instead of `[]PatchInfo` |
| `internal/patch/patch.go` | Modify | `Apply` accepts `webRoot` param; uses `composerutil.ReadWebRoot` |
| `internal/report/report.go` | Modify | Add `PipelineMetrics` field to `ReportData`; render in JSON + markdown |
| `internal/coreupgrade/apply.go` | Modify | Replace `execRunFn("composer", ...)` with `cliRun(cwd, "composer", ...)` in caller (commands.go) |
| `internal/app/commands.go` | Modify | Replace `execRunFn` composer calls with `cliRun` for DDEV awareness |
| `internal/packaging/templates/opencode/skills/d11-fixes/SKILL.md` | Create | D11 deprecation catalog skill |
| `internal/packaging/templates/opencode/skills/contrib-patch/SKILL.md` | Create | Contrib patch writer skill |
| (same for claude/ and codex/ templates) | Create | Duplicate skills for each agent template |

## Interfaces / Contracts

### New Types

```go
// internal/semver/semver.go
type Version struct { Major, Minor, Patch int }
func Parse(s string) (Version, error)
func (v Version) Compare(other Version) int
func Satisfies(version Version, constraint string) bool

// internal/metrics/metrics.go
type Metrics struct {
    TotalDurationMS  int64             `json:"total_duration_ms"`
    StageDurations   map[string]int64  `json:"stage_durations"`
    CommandsExecuted int64             `json:"commands_executed"`
    FilesModified    int64             `json:"files_modified"`
    Retries          int64             `json:"retries"`
    Interventions    int64             `json:"human_interventions"`
}
type Collector struct { /* sync.Mutex, start times, counters */ }
func Default() *Collector
func (c *Collector) PipelineStart()
func (c *Collector) StageStart(name string)
func (c *Collector) StageEnd(name string)
func (c *Collector) RecordCommand()
func (c *Collector) RecordRetry()
func (c *Collector) Snapshot() Metrics

// internal/composerutil/webroot.go
func ReadWebRoot(projectPath string) string  // reads composer.json scaffold config, falls back to "web"

// internal/drupalorg/drupalorg.go — modified return type
type PatchSearchResult struct {
    Status     string      `json:"status"`     // "patches_found" | "no_patches_found" | "error"
    Module     string      `json:"module"`
    Searched   string      `json:"searched"`
    Message    string      `json:"message"`
    Suggestion string      `json:"suggestion"`
    Patches    []PatchInfo `json:"patches"`
}

// internal/report/report.go — added field
type ReportData struct {
    // ... existing fields ...
    PipelineMetrics *metrics.Metrics `json:"pipeline_metrics,omitempty"`
}
```

### Modified Function Signatures

```go
// internal/app/commands.go
func RunCleanup(args []string) error
func checkCoreReadiness(projectPath string) ([]PreflightResult, error)
func DoValidate(projectPath, module string) (*scan.ScanResult, []scan.DepError, error)
  // now detects core version and swaps gates for >= 11.x

// internal/patch/patch.go
func Apply(patchURL, projectPath, composerPackage, description string) (*ApplyResult, error)
  // internally uses composerutil.ReadWebRoot(projectPath) instead of os.Getwd()

// internal/app/mcp_tools.go — isPHPCompatible replaced
func isPHPCompatible(phpVer, phpMin, phpRecommended string) bool
  // now uses semver.Satisfies instead of string comparison
```

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit — semver | Parse, Compare, Satisfies for `>=`, `^`, `~`, `\|\|`, invalid input | Table-driven tests, no mocks |
| Unit — cleanup | Gate on validate exit, idempotent skip, drush failure halts | Override `cliRun`/`execRunFn` vars (existing pattern) |
| Unit — core readiness | Composer constraint parsing, .info.yml scanning, blockers report | `t.TempDir()` with fixture files |
| Unit — post-D11 gates | Core version detection, gate swap logic | Mock `cliRun` to return version, verify correct commands called |
| Unit — structured issue_patches | PatchSearchResult fields for all 3 statuses | Mock HTTP client (existing `SetHTTPClientForTest`) |
| Unit — web root | Scaffold config present/absent, custom values | `t.TempDir()` with composer.json fixtures |
| Unit — metrics | Concurrent safety, non-blocking (panic recovery), snapshot | Goroutine stress test + defer/recover test |
| Unit — DDEV composer | Verify `cliRun` used instead of `execRunFn` for composer | Mock envdetect to return DDEV, capture command args |
| E2E — pipeline | Stage ordering, gate blocking, cleanup skip on failure | `internal/e2e/` with mock `cliRun` interface |
| Skills | File exists, frontmatter valid, trigger phrases present | Static file checks in packaging_test.go |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary changes. All changes are within existing CLI command patterns.

## Migration / Rollout

No migration required. All changes are additive (new commands, new fields) or backward-compatible (modified responses add fields, never remove). The `issue_patches` response changes from `[]PatchInfo` to `PatchSearchResult` — this is a breaking change for consumers expecting a bare array. Mitigate by including the patches array in the struct and documenting the change.

## Open Questions

- [ ] Should `drup cleanup` accept a `--skip-commit` flag for CI environments?
- [ ] Should pipeline metrics be opt-in via flag or always-on?
- [ ] For the D11 deprecation catalog skill: should patterns be machine-parseable (YAML) or human-readable (markdown)?
