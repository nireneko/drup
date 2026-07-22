# Design: Test Coverage for Recent Fixes and MCP Handlers

## Technical Approach

Add test infrastructure using **function-variable injection** (the pattern already established in `exec.execCommand`, `update.executableFn`, `installer.homeDir`, `state.configDir`). Convert `drupexec.Run`/`RunWithEnv` from functions to function variables — zero caller changes, but tests can now override them. Add a `SetHTTPClientForTest` helper to `drupalorg`. Add function-variable override points to `RunUninstall`. All production changes are testability hooks only.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|--------------|-----------|
| Exec mocking | Convert `exec.Run` and `exec.RunWithEnv` from `func` to `var` | Interface injection, `export_test.go` | Zero caller changes; tests override `drupexec.Run = mockRun`; follows existing `execCommand` var pattern |
| HTTP mocking (drupalorg) | Add `SetHTTPClientForTest(*http.Client) func()` to `drupalorg` | Export vars, refactor handlers to accept client | One function, cleanup via defer; vars stay unexported; handlers unchanged |
| RunUninstall testability | Add `stateLoadFn`, `osExecutableFn`, `osUserHomeDirFn`, `stateRemoveFn` vars | Refactor to accept dependencies | Follows `checkLatestFn`/`upgradeFn` pattern already in `commands.go` |
| EnvDetector mocking | Override `defaultEnvDetector` var in `mcp_tools.go` | Interface injection | Already a package-level var; one-line override in tests |
| Filesystem isolation | `t.TempDir()` fixtures | Mock filesystem | Existing pattern; real filesystem is faster and more reliable for these tests |

## Data Flow

```
Test Override Point          Production Code              External Dependency
─────────────────           ─────────────────             ───────────────────
drupexec.Run = mock    →    mcp_tools.go handlers    →   composer/drush (mocked)
drupexec.RunWithEnv = mock → mcp_tools.go handlers    →   drush via env (mocked)
drupalorg.SetHTTPClient →   drupalorg.ModuleInfo()   →   drupal.org API (httptest)
defaultEnvDetector = mock →   mcp_tools.go handlers  →   envdetect (mocked)
stateLoadFn = mock     →    RunUninstall()           →   state.json (mocked)
osExecutableFn = mock  →    RunUninstall()           →   os.Executable (mocked)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/exec/exec.go` | Modify | Convert `Run` and `RunWithEnv` from `func` to `var` (testability hook, ~4 lines changed) |
| `internal/drupalorg/drupalorg.go` | Modify | Add `SetHTTPClientForTest` function (~6 lines) |
| `internal/app/commands.go` | Modify | Add `stateLoadFn`, `osExecutableFn`, `osUserHomeDirFn`, `stateRemoveFn` vars; update `RunUninstall` to use them (~10 lines) |
| `internal/update/upgrade_test.go` | Extend | Add regression tests: backup location, cross-device copy, ETXTBSY staging, tar.gz extraction |
| `internal/installer/installer_test.go` | Extend | Add `WriteSkill` directory structure, `RemoveMCPConfig` per-agent, `WriteCommand` adapter tests |
| `internal/app/commands_test.go` | Extend | Add `RunUninstall` state-driven tests: adapter selection, dry-run, force mode, self-removal error |
| `internal/app/mcp_tools_test.go` | Extend | Add happy-path + invalid-input tests for all 10 new handlers using table-driven pattern |

## Interfaces / Contracts

### exec package — function variables

```go
// internal/exec/exec.go — change from func to var
var Run = func(cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
    return execCommand(cmd, args...).Output()
}

var RunWithEnv = func(prefix []string, cmd string, args ...string) (stdout, stderr string, exitCode int, err error) {
    // existing implementation, unchanged
}
```

### drupalorg package — test helper

```go
// internal/drupalorg/drupalorg.go
func SetHTTPClientForTest(c *http.Client) func() {
    orig := httpClient
    httpClient = c
    return func() { httpClient = orig }
}
```

### app package — RunUninstall override points

```go
// internal/app/commands.go
var stateLoadFn = statepkg.Load
var osExecutableFn = os.Executable
var osUserHomeDirFn = os.UserHomeDir
var stateRemoveFn = statepkg.Remove
```

### Mock exec pattern for MCP handler tests

```go
// internal/app/mcp_tools_test.go
func mockExecResponse(stdout, stderr string, exitCode int) {
    origRun := drupexec.Run
    drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
        return stdout, stderr, exitCode, nil
    }
    // restore in t.Cleanup
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `parseInstalledVersion`, `hasPackage`, `extractZip` | Direct function calls with table-driven inputs |
| Unit | MCP handler input validation (invalid JSON, missing params) | Call handler with bad input, assert error |
| Unit | MCP handler happy path | Override `drupexec.Run`, `drupalorg` HTTP, call handler, assert JSON response |
| Integration | `RunUninstall` state-driven flow | Override `stateLoadFn`, `osExecutableFn`; assert adapter calls and output |
| Integration | `WriteSkill`, `RemoveMCPConfig`, `WriteCommand` | `t.TempDir()` fixtures; assert filesystem state |
| Regression | Backup location, cross-device copy, ETXTBSY, tar.gz extraction | `t.TempDir()`, `httptest` for download; assert file locations and content |

### Table-driven test structure for MCP handlers

```go
func TestRealHandleComposerRequire(t *testing.T) {
    tests := []struct {
        name       string
        input      string
        mockStdout string
        mockExit   int
        wantErr    bool
        wantField  map[string]interface{}
    }{
        {"invalid JSON", `{invalid`, "", 0, true, nil},
        {"missing package", `{}`, "", 0, true, nil},
        {"happy path", `{"project_path":"/tmp","package":"drupal/token"}`, "Installing drupal/token (1.2.3)", 0, false, map[string]interface{}{"success": true}},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // override drupexec.Run if needed
            // call realHandleComposerRequire
            // assert response
        })
    }
}
```

### `-short` skip pattern

Tests requiring real `git` or external tools skip in `-short` mode:

```go
func TestPatchRollback_HappyPath(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping test requiring git")
    }
    // ...
}
```

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary. (This is a test-only change; the security-sensitive code is already covered by existing RED tests in `mcp_tools_test.go`.)

## Migration / Rollout

No migration required. All changes are additive (test files) or testability hooks (function variables). Production behavior is unchanged.

## Open Questions

- [ ] Should we add a `SetEnvDetectorForTest` helper to `envdetect` package, or is overriding `defaultEnvDetector` in `mcp_tools.go` sufficient? (Leaning: override `defaultEnvDetector` is sufficient — it's already a package-level var in the `app` package.)
- [ ] Should `extractZip` tests use real `.zip` fixtures or in-memory archives? (Leaning: real `.zip` in `t.TempDir()` — simpler and matches the function's expected input.)
