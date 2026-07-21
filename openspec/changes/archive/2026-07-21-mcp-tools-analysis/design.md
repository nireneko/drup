# Design: MCP Tools Analysis — 10 New Tools for Drupal Upgrade Orchestrator

## Technical Approach

Add 10 new MCP tools in 4 phases. All tools are additive — no existing tools are modified. A new `internal/envdetect/` package provides environment detection with caching. The existing `internal/exec/exec.go` gains a `RunWithEnv` function for command prefixing. The existing `internal/drupalorg/` package gains two new functions for upgrade path resolution and module info.

## Architecture Decisions

### Decision: Environment Detection as Separate Package

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Inline detection in each tool handler | Fast, but duplicated logic; cache per-handler | Rejected |
| New `internal/envdetect/` package | One more package, but single source of truth with shared cache | **Chosen** |
| Add to `internal/exec/` | Couples detection to execution; exec should be dumb | Rejected |

**Rationale**: Multiple tools need the same detection result. A dedicated package with a `Detector` interface allows test doubles and a shared cache.

### Decision: Command Prefixing via RunWithEnv

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Modify `Run()` signature to accept prefix | Breaks all existing callers | Rejected |
| New `RunWithEnv(prefix, cmd, args...)` function | Additive; existing `Run()` unchanged | **Chosen** |
| Wrapper function in envdetect package | Splits responsibility across packages | Rejected |

**Rationale**: `RunWithEnv` prepends prefix tokens to the command. For ddev: `RunWithEnv([]string{"ddev"}, "composer", "require", "drupal/token")` → executes `ddev composer require drupal/token`. Existing `Run()` stays untouched.

### Decision: Drupal.org API Extensions In-Place

| Option | Tradeoff | Decision |
|--------|----------|----------|
| New `internal/drupalorg/upgrade.go` file | More files, but logical separation | Rejected (ponytail: same package, one file is enough until >400 lines) |
| Add functions to existing `drupalorg.go` | Keeps package small; follows existing pattern | **Chosen** |

**Rationale**: The package is 319 lines. Adding `UpgradePath()` and `ModuleInfo()` keeps it under 500 lines. The existing test pattern (httptest + package-level var override) extends naturally.

### Decision: Tool Registration Pattern

Follow the existing two-layer pattern exactly:
1. `internal/mcp/tools.go` — add 10 placeholder handlers in `defaultTools()`
2. `internal/app/mcp_tools.go` — add 10 real handlers, register in `WireMCPTools()`

No new registration mechanism needed.

## Data Flow

### upgrade_scan (most complex tool)

```
Agent
  │
  ▼
upgrade_scan handler
  │
  ├─→ detect_env(project_path) ──→ Environment{prefix: ["ddev"]}
  │
  ├─→ Check composer.json for "drupal/upgrade_status"
  │     │
  │     ├─ missing → composer_require(project_path, "drupal/upgrade_status")
  │     │              └─→ RunWithEnv(prefix, "composer", "require", "drupal/upgrade_status")
  │     │
  │     └─ present → skip
  │
  ├─→ drush_exec(project_path, "pm:enable", ["upgrade_status"])
  │     └─→ RunWithEnv(prefix, "drush", "pm:enable", "upgrade_status")
  │
  ├─→ drush_exec(project_path, "upgrade_status:analyze", ["--format=json"])
  │     └─→ RunWithEnv(prefix, "drush", "upgrade_status:analyze", "--format=json")
  │
  └─→ scan.Parse(stdout) → filter by scope → return result
```

### contrib_upgrade_path

```
Agent
  │
  ▼
contrib_upgrade_path handler
  │
  ├─→ drupalorg.FetchReleaseHistory(module) ──→ full XML
  │     └─→ HTTP GET https://updates.drupal.org/release-history/{module}/current
  │
  ├─→ parseReleaseXML → all releases with versions + compatibility terms
  │
  ├─→ Filter: releases compatible with target_drupal_version
  │
  └─→ Sort: prefer latest stable → return recommended + alternatives
```

### patch_rollback

```
Agent
  │
  ▼
patch_rollback handler
  │
  ├─→ Read composer.json → find patch in extra.patches
  │
  ├─→ git log --oneline --grep="{patch_url}" → find commit hash
  │
  ├─→ git revert {commit_hash} --no-edit
  │
  ├─→ Remove entry from composer.json extra.patches
  │
  └─→ RunWithEnv(prefix, "composer", "update", package_name)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/envdetect/envdetect.go` | Create | Detector interface, Environment type, marker-file detection, in-memory cache |
| `internal/envdetect/envdetect_test.go` | Create | Table-driven tests with temp dirs containing marker files |
| `internal/exec/exec.go` | Modify | Add `RunWithEnv(prefix []string, cmd string, args ...string)` function (~10 lines) |
| `internal/exec/exec_test.go` | Modify | Add test for `RunWithEnv` with mock prefix |
| `internal/drupalorg/drupalorg.go` | Modify | Add `FetchReleaseHistory()`, `UpgradePath()`, `ModuleInfo()` functions |
| `internal/drupalorg/drupalorg_test.go` | Modify | Add tests with httptest for new functions |
| `internal/mcp/tools.go` | Modify | Add 10 placeholder handlers in `defaultTools()` map |
| `internal/app/mcp_tools.go` | Modify | Add 10 real handler functions + register in `WireMCPTools()` |

## Interfaces / Contracts

### internal/envdetect

```go
type Environment string

const (
    EnvDdev           Environment = "ddev"
    EnvLando          Environment = "lando"
    EnvDocker4Drupal  Environment = "docker4drupal"
    EnvDirect         Environment = "direct"
    EnvUnknown        Environment = "unknown"
)

type Detection struct {
    Environment   Environment `json:"environment"`
    CommandPrefix []string    `json:"command_prefix"`
    DetectedAt    time.Time   `json:"detected_at"`
}

type Detector interface {
    Detect(projectPath string) (*Detection, error)
}

// DefaultDetector checks marker files with in-memory cache.
type DefaultDetector struct {
    mu    sync.Mutex
    cache map[string]*Detection
}
```

Detection order:
1. `.ddev/` directory exists → `ddev`, prefix `["ddev"]`
2. `.lando.yml` file exists → `lando`, prefix `["lando"]`
3. `docker-compose.yml` exists AND contains `*drupal*` → `docker4drupal`, prefix `["docker", "compose", "exec", "php"]`
4. `composer.json` exists → `direct`, prefix `[]`
5. None match → `unknown`, prefix `[]`

### internal/exec — RunWithEnv

```go
// RunWithEnv prepends prefix tokens to cmd.
// Example: RunWithEnv([]string{"ddev"}, "composer", "require", "pkg")
// executes: ddev composer require pkg
func RunWithEnv(prefix []string, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
    fullArgs := append(prefix, cmd)
    fullArgs = append(fullArgs, args...)
    if len(prefix) > 0 {
        return execCommand(fullArgs[0], fullArgs[1:]...)
    }
    return execCommand(cmd, args...).Output()
}
```

### internal/drupalorg — New Functions

```go
// UpgradeRecommendation is the recommended version for a target Drupal major.
type UpgradeRecommendation struct {
    Module       string    `json:"module"`
    Recommended  *Release  `json:"recommended_upgrade"`
    Alternatives []Release `json:"alternative_versions"`
}

type Release struct {
    Version           string   `json:"version"`
    DrupalCompat      []string `json:"drupal_compatibility"`
    ReleaseDate       string   `json:"release_date"`
    IsStable          bool     `json:"is_stable"`
}

// UpgradePath finds the recommended version for target Drupal major.
func UpgradePath(module, currentDrupal, targetDrupal string) (*UpgradeRecommendation, error)

// ModuleInfo fetches module metadata from Drupal.org.
type ModuleMetadata struct {
    Module      string   `json:"module"`
    Title       string   `json:"title"`
    Maintainers []string `json:"maintainers"`
    Downloads   int      `json:"downloads"`
    LastRelease string   `json:"last_release"`
    OpenIssues  int      `json:"open_issues"`
}

func ModuleInfo(module string) (*ModuleMetadata, error)
```

### Tool Handler Input/Output Contracts

Each tool follows the existing pattern: unmarshal `json.RawMessage` → validate → execute → marshal response.

**composer_require** input: `{project_path, package, dev?, no_update?}` → output: `{success, installed_version, stdout, stderr, exit_code}`

**drush_exec** input: `{project_path, command, args?, format?}` → output: `{success, output, stderr, exit_code}`. Blocklist: `sql-drop`, `site-install`, `site:install`, `sql-sanitize`.

**patch_status** input: `{project_path, patch_url?, composer_package?}` → output: `{is_applied, commit_hash, registered_in_composer, patch_info}`

**generate_report** input: `{project_path, report_type?, include_scan_data?, include_patch_list?}` → output: `{success, json_report_path, markdown_report_path, summary}`

**drupal_version_matrix** input: `{drupal_version?, php_version?}` → output: static lookup from hardcoded map.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit: envdetect | Detection logic for each marker file, cache hit/miss, force_detect bypass | Table-driven tests with `t.TempDir()` creating marker files |
| Unit: exec.RunWithEnv | Prefix prepending, empty prefix fallback | Mock `execCommand` override (existing pattern) |
| Unit: drupalorg.UpgradePath | XML parsing, version filtering, stable preference | httptest server + testdata XML fixtures (existing pattern) |
| Unit: drupalorg.ModuleInfo | JSON parsing from api-d7 | httptest server (existing pattern) |
| Unit: drush_exec blocklist | Dangerous commands rejected | Direct function call with blocklisted commands |
| Unit: patch_status/rollback | composer.json parsing, git log parsing | Mock filesystem + mock exec |
| Unit: drupal_version_matrix | Static lookup correctness | Table-driven tests |
| Integration: upgrade_scan | Full flow with mocked exec + detect | Mock Detector + mock execCommand |

## Threat Matrix

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | composer.json read for patch detection | Applicable | Only read `composer.json` at project root; reject paths containing `..` or absolute paths outside project | Test: path traversal attempt returns error |
| Git repository selection | `git -C {project_path}` for patch_rollback | Applicable | Validate project_path is a git repo before git operations; use `-C` flag exclusively | Test: non-git directory returns error |
| Commit state | git revert in patch_rollback | Applicable | Check working tree clean before revert; refuse if uncommitted changes | Test: dirty working tree returns error |
| Push state | N/A | N/A — no push operations | N/A | N/A |
| PR commands | N/A | N/A — no PR operations | N/A | N/A |
| Shell command composition | RunWithEnv prefix + cmd concatenation | Applicable | Validate package names (alphanumeric + `/` + `-` + `_`); validate drush commands against blocklist; never pass unsanitized input to shell | Test: package name with `; rm -rf /` rejected; drush `sql-drop` blocked |

## Migration / Rollout

No migration required. All 10 tools are additive. Existing 7 tools are untouched. Rollback = remove new entries from `defaultTools()` and `WireMCPTools()`.

## Implementation Phases

### Phase 1: detect_env + composer_require + drush_exec

**Files** (in order):
1. `internal/envdetect/envdetect.go` — Detector, Environment, Detection types, Detect()
2. `internal/envdetect/envdetect_test.go` — table-driven detection tests
3. `internal/exec/exec.go` — add RunWithEnv (~10 lines)
4. `internal/exec/exec_test.go` — add RunWithEnv test
5. `internal/mcp/tools.go` — add 3 placeholder handlers
6. `internal/app/mcp_tools.go` — add 3 real handlers with env-aware execution

**Test approach**: Run `go test ./internal/envdetect/... ./internal/exec/...` after each file.

### Phase 2: upgrade_scan + contrib_upgrade_path

**Files** (in order):
1. `internal/drupalorg/drupalorg.go` — add UpgradePath(), FetchReleaseHistory()
2. `internal/drupalorg/drupalorg_test.go` — add httptest-based tests
3. `internal/mcp/tools.go` — add 2 placeholder handlers
4. `internal/app/mcp_tools.go` — add 2 real handlers (upgrade_scan orchestrates detect_env + composer_require + drush_exec)

**Test approach**: upgrade_scan test mocks Detector and execCommand. contrib_upgrade_path test uses testdata XML.

### Phase 3: patch_status + patch_rollback

**Files** (in order):
1. `internal/mcp/tools.go` — add 2 placeholder handlers
2. `internal/app/mcp_tools.go` — add 2 real handlers (read composer.json, git operations)

**Test approach**: Create temp git repos with `t.TempDir()`, apply/revert patches, verify state.

### Phase 4: generate_report + module_info + drupal_version_matrix

**Files** (in order):
1. `internal/drupalorg/drupalorg.go` — add ModuleInfo()
2. `internal/drupalorg/drupalorg_test.go` — add ModuleInfo test
3. `internal/mcp/tools.go` — add 3 placeholder handlers
4. `internal/app/mcp_tools.go` — add 3 real handlers (generate_report wraps existing report package; drupal_version_matrix uses static map)

**Test approach**: generate_report test verifies file output. module_info test uses httptest. version_matrix is table-driven.

## Open Questions

- [ ] docker4drupal command prefix: `docker compose exec php` vs `docker-compose exec php` — depends on whether the project uses v1 or v2. Default to `docker compose` (v2) with fallback detection.
- [ ] Should `drush_exec` blocklist be configurable or hardcoded? Proposal says hardcoded; keeping it hardcoded for now.
