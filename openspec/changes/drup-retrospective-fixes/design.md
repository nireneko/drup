# Design: drup-retrospective-fixes

## Technical Approach

Six targeted fixes to the drup CLI pipeline, ordered by severity (P0 → P2). Each fix is an isolated commit. The core pattern: extract shared helpers for exit-code semantics and env-aware execution, then wire them into both CLI and MCP code paths.

## Architecture Decisions

### Decision: Exit code 3 — shared helper vs inline checks

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Inline `if exitCode == 3` in each caller | 4 call sites, easy to miss one | **Rejected** |
| Shared `isScanExitOK(int) bool` helper | One function, all callers consistent | **Chosen** |

**Rationale**: 4 call sites (`RunScan`, `DoValidate`, `realHandleScan`, `realHandleAutofix` re-scan). A single helper prevents drift.

### Decision: DDEV support — per-command detection vs global flag

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `--ddev` CLI flag | User must remember, fragile | **Rejected** |
| Auto-detect via `envdetect.Detect()` in each CLI command | Same pattern as MCP handlers, zero config | **Chosen** |

**Rationale**: MCP handlers already auto-detect. CLI commands should match. The `envdetect` package exists and works — it just needs to be called.

### Decision: MCP schemas — inline map vs typed registry

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Big `map[string]ToolSchema` in `server.go` | Simple, all in one place | **Chosen** |
| Per-handler schema registration API | More flexible, but 20 tools with static params doesn't need it | Rejected |

**Rationale**: YAGNI. All 20 tools have static parameter shapes. A single map in `server.go` is the shortest path.

### Decision: Contrib constraint — info.yml fetch vs XML term parsing

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Parse `core_version_requirement` from `.info.yml` via git.drupal.org | Accurate, handles `||` | **Chosen** |
| Improve XML term parsing to handle compound strings | Fragile, terms are free-text like "Drupal 11" | Rejected as primary |

**Rationale**: The XML `<terms>` contain human-readable strings ("Drupal 11"), not machine-parseable constraints. The `.info.yml` has the actual `core_version_requirement: '^10.3 || ^11.0'`. Fetch from `https://git.drupalcode.org/project/<module>/-/raw/<branch>/<module>.info.yml`.

### Decision: PHP 8.4 patch — append vs inject

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Append `error_reporting()` after DDEV include block | Matches the manual workaround that worked | **Chosen** |
| Inject into php.ini or .user.ini | Doesn't work because DrupalKernel overrides with `E_ALL` | Rejected |

### Decision: Report data — run scan vs read state

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Call `DoValidate` to get live scan data | Simple, reuses existing code | **Chosen** |
| Read from state file or previous scan output | Stale data, adds state coupling | Rejected |

## Data Flow

### Fix 1+2: Scan/Validate with exit code 3 + DDEV

```
CLI: RunScan(path)
  │
  ├─ envdetect.Detect(path) → detection
  │
  ├─ drupexec.RunWithEnv(prefix, "drush", "upgrade_status:analyze", "--all", "--root="+path)
  │     │
  │     └─ [ddev] drush upgrade_status:analyze --all --root=/var/www/html
  │
  ├─ isScanExitOK(exitCode)?
  │     ├─ true (0 or 3) → scan.Parse(stdout) → JSON output
  │     └─ false (1,2,>3) → check stderr empty? → error or crash report
  │
  └─ exit 0
```

### Fix 3: MCP Tool Schema

```
tools/list request
  │
  └─ handleListTools → toolRegistry[name] → {name, description, inputSchema{properties, required}}
```

### Fix 4: Contrib compound constraint

```
CheckRelease(module)
  │
  ├─ Fetch release-history XML (existing)
  │
  ├─ Find latest release version → branch name (e.g. "6.3.0" → "6.x")
  │
  ├─ Fetch info.yml from git.drupalcode.org
  │     └─ GET https://git.drupalcode.org/project/<module>/-raw/<branch>/<module>.info.yml
  │
  ├─ Parse core_version_requirement: "^10.3 || ^11.0"
  │
  └─ constraintMatchesDrupal(constraint, 11) → true
```

### Fix 5: PHP 8.4 preflight patch

```
RunPreflight()
  │
  ├─ drupexec.RunWithEnv(prefix, "php", "-r", "echo PHP_VERSION;")
  │     └─ "8.4.2"
  │
  ├─ isPHP84OrLater("8.4.2") → true
  │
  ├─ Read settings.php
  │
  ├─ Check if suppression already present → skip if yes
  │
  ├─ Find DDEV include block end (or last line)
  │
  ├─ Append: error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED);
  │
  └─ Add preflight result: php84_compat → pass
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modify | Add `isScanExitOK` helper, `envAwareRun` helper; update `RunScan`, `DoValidate`, `RunPreflight`, `RunReport`, `RunFix`, `RunUpgradeCore` |
| `internal/app/mcp_tools.go` | Modify | Update `realHandleScan`, `realHandleAutofix` to use `isScanExitOK`; use `--root=` consistently |
| `internal/mcp/server.go` | Modify | Add `toolRegistry` map with schemas; update `handleListTools` to emit full schemas |
| `internal/drupalorg/drupalorg.go` | Modify | Add `fetchInfoYML`, `constraintMatchesDrupal`; update `parseReleaseXML` to enrich with constraint data |
| `internal/app/commands_test.go` | Modify | Add tests for `isScanExitOK`, exit-3 parsing, DDEV prefix propagation |
| `internal/mcp/server_test.go` | Modify | Add test that `tools/list` returns non-empty `inputSchema.properties` |
| `internal/drupalorg/drupalorg_test.go` | Modify | Add tests for compound constraint parsing |

## Interfaces / Contracts

### Exit code helper

```go
// isScanExitOK returns true for exit codes that carry valid scan data.
// 0 = no findings, 3 = findings exist. 1, 2, >3 = real errors.
func isScanExitOK(exitCode int) bool {
    return exitCode == 0 || exitCode == 3
}
```

### Env-aware CLI runner

```go
// cliRun detects the environment for projectPath and runs cmd with the
// appropriate prefix. Uses --root= instead of -r for drush commands.
// Returns the same (stdout, stderr, exitCode, err) as drupexec.Run.
func cliRun(projectPath string, cmd string, args ...string) (string, string, int, error) {
    detection, err := defaultEnvDetector.Detect(projectPath, false)
    if err != nil {
        return "", "", -1, fmt.Errorf("detect environment: %w", err)
    }
    return drupexec.RunWithEnv(detection.CommandPrefix, cmd, args...)
}
```

### MCP Tool schema registry

```go
// toolSchema defines a tool's metadata for the tools/list response.
type toolSchema struct {
    Description string
    Properties  map[string]jsonSchemaProperty
    Required    []string
}

type jsonSchemaProperty struct {
    Type        string `json:"type"`
    Description string `json:"description"`
}

// toolRegistry maps tool names to their schemas.
// Populated in init() or as a package-level var.
var toolRegistry = map[string]toolSchema{
    "scan": {
        Description: "Run upgrade_status:analyze on a Drupal project",
        Properties: map[string]jsonSchemaProperty{
            "project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
        },
        Required: []string{"project_path"},
    },
    // ... 19 more entries
}
```

### Contrib constraint parser

```go
// constraintMatchesDrupal checks if a core_version_requirement string
// (e.g. "^10.3 || ^11.0") is satisfied by the given Drupal major version.
func constraintMatchesDrupal(constraint string, drupalMajor int) bool
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `isScanExitOK` returns true for 0,3; false for 1,2,4+ | Table-driven test |
| Unit | `constraintMatchesDrupal` handles `^11`, `^10.3 \|\| ^11.0`, `>=10 <12` | Table-driven with edge cases |
| Unit | `cliRun` passes correct prefix for DDEV vs direct | Mock `envdetect.Detector` |
| Unit | `handleListTools` returns schemas with properties | Assert JSON structure |
| Unit | PHP 8.4 detection + settings.php patch idempotency | Temp dir with mock settings.php |
| Unit | `RunReport` populates real data from scan | Mock `DoValidate` via function var |
| Integration | Exit code 3 with valid stdout → parsed result | `httptest` + fake exec |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary changes. The fixes modify how existing command execution is invoked (prefix + flag changes), not the execution boundary itself.

## Migration / Rollout

No migration required. All changes are internal to the drup binary. Each fix is a separate commit for granular rollback.

## Open Questions

- [ ] Drupal.org git raw URL format: need to verify `https://git.drupalcode.org/project/<module>/-/raw/<branch>/<module>.info.yml` works for all contrib modules (some use different branch naming like `8.x-1.x` vs `6.x`)
- [ ] Should `cliRun` be a package-level var (like `execRunFn`) for testability, or is mocking `envdetect.Detector` sufficient?
