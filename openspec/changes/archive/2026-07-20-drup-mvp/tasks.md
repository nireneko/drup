# Tasks: drup-mvp — Drupal Upgrade Automation System

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~3000 (15 new packages, 30+ files, fixtures) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 → PR 5 |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Foundation: exec, gitops, scan + fixtures | PR 1 | `go test ./internal/exec/... ./internal/gitops/... ./internal/scan/...` | N/A — pure unit tests with fixtures | `rm -rf internal/exec internal/gitops internal/scan` |
| 2 | Domain: drupalorg, patch, report + fixtures | PR 2 | `go test ./internal/drupalorg/... ./internal/patch/... ./internal/report/...` | `httptest.Server` for drupal.org XML/HTML fixtures | `rm -rf internal/drupalorg internal/patch internal/report` |
| 3 | CLI wiring: app dispatch + 6 commands + smoke | PR 3 | `go build ./cmd/drup && go test ./internal/app/...` | `drup scan ./testdata/fixture` end-to-end | `rm -rf internal/app cmd/drup` |
| 4 | MCP server: 7 tools, JSON-RPC stdio | PR 4 | `go test ./internal/mcp/...` | `echo '{"jsonrpc":"2.0",...}' | drup mcp` pipe test | `rm -rf internal/mcp` |
| 5 | Agent infra: state, packaging, installer, update | PR 5 | `go test ./internal/state/... ./internal/installer/... ./internal/update/... ./internal/packaging/...` | `drup install` against temp `~/.config/` | `rm -rf internal/state internal/packaging internal/installer internal/update` |

## Phase v0.1: Go Binary — Deterministic Core

### Phase 1: Scaffolding

- [x] 1.1 Create `go.mod` (module `drup`, go 1.25.10), `cmd/drup/main.go` (15-line shim calling `app.Run`), and empty `internal/app/app.go` with `Run(args []string) error` stub. **Verify**: `go build ./cmd/drup` compiles.
- [x] 1.2 Scaffold `internal/exec/exec.go` with `Run(cmd string, args ...string) (stdout, stderr string, exitCode int, err error)`. Use package-level `var execCommand = exec.Command` for testability. **Verify**: unit test with overridden var captures stdout/stderr.

### Phase 2: Leaf Domain Packages

- [x] 1.3 Create `internal/gitops/gitops.go`: `IsClean(path) (bool, []string, error)` via `git status --porcelain`, `EnsureBranch(path, name)` creating `upgrade/drupal-11`, `Commit(path, msg, files[]) (hash string, err error)` with conventional format. **Verify**: tests with temp git repos for clean/dirty/branch/commit scenarios.
- [x] 1.4 Create `internal/scan/model.go` with `ScanResult`, `ModuleStatus`, `DepError`, `ErrorClass` types per design §Interfaces. Create `internal/scan/scan.go`: `Parse(r io.Reader) (*ScanResult, error)` classifying paths by `modules/contrib/`, `modules/custom/`, `themes/` prefix. **Verify**: fixture tests with `testdata/upgrade_status_d10.json` and `testdata/upgrade_status_d9.json`.

### Phase 3: External Integration Packages

- [x] 1.5 Create `internal/drupalorg/releases.go`: `CheckRelease(module string) (*ReleaseInfo, error)` fetching `https://updates.drupal.org/release-history/<module>/current`, parsing XML with `encoding/xml`. Include `var httpClient` for test override. **Verify**: fixture test with `testdata/release_d11.xml` and `testdata/release_no_d11.xml`.
- [x] 1.6 Create `internal/drupalorg/issues.go`: `SearchPatches(query string) ([]PatchInfo, error)` using api-d7 JSON endpoint first, HTML scraper fallback. Extract `.patch`/`.diff`/MR URLs. Sort by RTBC priority (RTBC > Fixed > Needs review > Needs work). **Verify**: fixture test with `testdata/issue_with_patches.html`.
- [x] 1.7 Create `internal/patch/patch.go`: `Apply(patchURL, projectPath string) (*ApplyResult, error)`. Download via `net/http` (allowlist drupal.org domains), `git apply` via exec package (with `--whitespace=nowarn` fallback), register in `composer.json` under `extra.patches.<vendor>/<project>`. Atomic: revert on failure. **Verify**: tests with mock HTTP server + temp git repo.

### Phase 4: Report + CLI Wiring

- [x] 1.8 Create `internal/report/report.go`: `GenerateJSON(result *ReportData) ([]byte, error)` and `GenerateMarkdown(result *ReportData) (string, error)` with sections: Summary, Resolved, Pending Human Review, Token Usage. Write `drup-report.json` + `drup-report.md` to project root. **Verify**: golden file tests for JSON and markdown output.
- [x] 1.9 Create `internal/app/app.go` dispatch: `switch args[0]` routing to command functions. Unknown command → stderr usage + exit 1. No args → stdout usage + exit 0. Exit codes: 0=success, 1=errors, 2=usage, 3=network. **Verify**: table-driven tests for each command routing.
- [x] 1.10 Create `internal/app/commands.go`: `RunInit` (verify composer.json + drupal/core), `RunScan` (exec drush upgrade_status:analyze → scan.Parse → JSON stdout), `RunFix` (exec drupal-rector → summary), `RunContrib` (drupalorg.CheckRelease → JSON), `RunIssue` (drupalorg.SearchPatches → JSON), `RunReport` (report.Generate). **Verify**: integration test per command with mocked exec.
- [x] 1.11 Integration smoke test: build binary, run `drup help`, `drup scan <testdata>`, `drup contrib token` against fixture. **Verify**: `go build ./cmd/drup && go test ./...` passes clean.

## Phase v0.2: MCP + Agents

### Phase 5: State + MCP Server

- [x] 2.1 Create `internal/state/state.go`: `State` struct (Version, InstalledAgents, PendingSync, ModelOverrides), `Load() (*State, error)` from `~/.config/drup/state.json`, `Save(s *State) error` with atomic write. **Verify**: round-trip test in temp dir.
- [x] 2.2 Create `internal/mcp/server.go`: hand-rolled JSON-RPC 2.0 over stdio. Read stdin line-by-line, parse `{jsonrpc, method, params, id}`, dispatch to tool handler, write `{jsonrpc, result, id}` to stdout. Error codes: -32700 (parse), -32602 (invalid params), -32601 (method not found). **Verify**: pipe test with raw JSON on stdin.
- [x] 2.3 Create `internal/mcp/tools.go`: register 7 tools (scan, autofix, contrib_check, issue_patches, apply_patch, validate, create_patch) with JSON input schemas. Each handler delegates to the corresponding internal package. **Verify**: test each tool handler with mock dependencies.

### Phase 6: Agent Packaging + Installation

- [x] 2.4 Create `internal/packaging/templates.go`: `//go:embed templates/*` for SKILL.md, sub-agent defs, MCP config per platform (claude/, opencode/, codex/). `Render(platform, binaryPath string) (map[string]string, error)` returns filename→content. **Verify**: test that each platform produces correct file set.
- [x] 2.5 Create agent template files under `internal/packaging/templates/`: orchestrator SKILL.md encoding 7-stage pipeline with validation gates, 4 sub-agent definitions (drup-preflight, drup-contrib, drup-custom, drup-theme) with model routing and MCP tool assignments. One set per platform (claude/, opencode/, codex/).
- [x] 2.6 Create `internal/installer/installer.go`: `AgentAdapter` interface (ID, Detect, SkillsDir, MCPConfigPath, WriteSkill, WriteMCPConfig, Backup). Implementations: `ClaudeAdapter` (`~/.claude/`), `OpenCodeAdapter` (`~/.config/opencode/`), `CodexAdapter`. `DetectAgents() []AgentAdapter`, `Install(adapters, binaryPath)`, backup as tar.gz (retain 5 latest). **Verify**: test with temp home dirs.
- [x] 2.7 Create `internal/update/update.go`: `CheckLatest(owner, repo) (version, assetURL, error)` via GitHub Releases API, `Download(url) (path, error)` to temp dir, SHA256 verify against `checksums.txt`, atomic replace (rename current → `.bak`, move new → current path, chmod +x). Set `PendingSync=true` in state.json. **Verify**: test with `httptest.Server` serving mock release.
- [x] 2.8 Wire `drup mcp`, `drup install`, `drup sync`, `drup upgrade`, `drup version` commands in `internal/app/commands.go`. Add deferred sync check at binary startup: if `state.PendingSync`, run installer sync, clear flag. **Verify**: `go build ./cmd/drup && go test ./... && go vet ./...` all pass clean.
