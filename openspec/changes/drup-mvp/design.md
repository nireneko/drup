# Design: drup-mvp — Drupal Upgrade Automation System

## Technical Approach

Go binary (CLI + MCP server) that orchestrates deterministic Drupal 8/9/10→11 migration. The binary handles all deterministic work (scanning, patching, validation); agents handle reasoning (custom code fixes). Module path `drup` (local), Go 1.25.10, stdlib only, manual CLI dispatch. Mirrors gentle-ai patterns: package-level var overrides for testability, table-driven tests, adapter interface for agent packaging, state.json with pending_sync deferred sync.

Reference: PRD §7, proposal §Approach.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| CLI dispatch | Manual `switch args[0]` | cobra | Proposal says no cobra; 6 commands don't need it. Matches gentle-ai pattern. |
| Module path | `drup` (local) | `github.com/gentleman-programming/drup` | User constraint. Local-only until v0.2. |
| MCP (MVP) | Hand-rolled JSON-RPC over stdio | `modelcontextprotocol/go-sdk` | Zero deps until v0.2. 7 tools, simple protocol. SDK added in v0.2. |
| Error model | Classified enum (contrib/custom/theme/core) | flat error list | Enables scope-based validation gates and sub-agent routing. |
| Config format | `drup.yaml` (stdlib `gopkg.in/yaml.v3` — or JSON fallback) | TOML | PRD specifies YAML. MVP can use JSON to stay stdlib-only if needed. |
| State location | `~/.config/drup/state.json` | project-local | Matches gentle-ai. Per-user, survives project deletion. |

## Package Dependency Graph

```
cmd/drup/main.go
    └──→ internal/app
             ├──→ internal/exec      (subprocess runner)
             ├──→ internal/scan      (upgrade_status JSON parser)
             ├──→ internal/drupalorg (release-history + issue scraper)
             ├──→ internal/patch     (download + git apply)
             ├──→ internal/gitops    (git clean, commits, branches)
             ├──→ internal/report    (JSON + markdown output)
             ├──→ internal/state     (state.json R/W)
             ├──→ internal/installer (detect agents, write assets)
             ├──→ internal/packaging (templates per agent)
             └──→ internal/update    (self-update binary)

internal/mcp
    ├──→ internal/scan
    ├──→ internal/exec
    ├──→ internal/drupalorg
    ├──→ internal/patch
    ├──→ internal/gitops
    └──→ internal/report

NO CYCLES. Leaf packages (exec, state) import nothing internal.
```

Data flow: CLI/MCP → app dispatch → domain packages → exec (subprocess) → stdout/JSON → report.

## Data Flow

```
User/Agent
    │
    ▼
┌─────────┐     ┌──────────┐     ┌────────────┐
│ CLI/MCP  │────→│   app    │────→│  domain    │
│ (input)  │     │ (dispatch)│     │  packages  │
└─────────┘     └──────────┘     └─────┬──────┘
                                       │
                              ┌────────┼────────┐
                              ▼        ▼        ▼
                           exec    scan    drupalorg
                          (git,    (JSON   (HTTP, XML
                          composer, parse)  parse)
                          drush)
                              │        │        │
                              └────────┼────────┘
                                       ▼
                                   report
                                (JSON + .md)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/drup/main.go` | Create | 15-line shim: calls `app.Run(os.Args[1:])` |
| `internal/app/app.go` | Create | Manual dispatch: `switch args[0]` → command functions |
| `internal/app/commands.go` | Create | One func per command: `RunInit`, `RunScan`, `RunFix`, etc. |
| `internal/exec/exec.go` | Create | `Run(cmd string, args ...string) (stdout, stderr, error)` with package-level `var execCommand` |
| `internal/scan/scan.go` | Create | `Parse(r io.Reader) (*ScanResult, error)` — upgrade_status JSON → model |
| `internal/scan/model.go` | Create | `ScanResult`, `ModuleError`, `ErrorClass` types |
| `internal/drupalorg/releases.go` | Create | `CheckRelease(module string) (*ReleaseInfo, error)` — XML parser |
| `internal/drupalorg/issues.go` | Create | `SearchPatches(query string) ([]PatchInfo, error)` — api-d7 + scraper |
| `internal/patch/patch.go` | Create | `Apply(patchURL, projectPath string) (*ApplyResult, error)` |
| `internal/gitops/gitops.go` | Create | `IsClean()`, `Commit(msg)`, `EnsureBranch(name)` |
| `internal/report/report.go` | Create | `GenerateJSON()`, `GenerateMarkdown()` |
| `internal/mcp/server.go` | Create | JSON-RPC stdio server, tool registry, 7 handlers |
| `internal/mcp/tools.go` | Create | Tool schemas (input/output JSON objects) |
| `internal/state/state.go` | Create | `Load()`, `Save()`, `State` struct with PendingSync |
| `internal/packaging/templates.go` | Create | Embedded SKILL.md + MCP config templates per agent |
| `internal/installer/installer.go` | Create | Detect agents, write assets, backup configs |
| `internal/update/update.go` | Create | GitHub Releases check, download, checksum, atomic replace |

## Interfaces / Contracts

### ScanResult (core data model)

```go
type ScanResult struct {
    Modules    []ModuleStatus `json:"modules"`
    TotalErrs  int            `json:"total_errors"`
    ProjectPath string        `json:"project_path"`
}

type ModuleStatus struct {
    Name       string       `json:"name"`
    Type       ErrorClass   `json:"type"` // contrib|custom|theme|core
    Errors     []DepError   `json:"errors"`
    HasD11     *bool        `json:"has_d11_release,omitempty"`
}

type DepError struct {
    File    string `json:"file"`
    Line    int    `json:"line"`
    Message string `json:"message"`
    Rule    string `json:"rule"` // rector/phpstan rule ID
}

type ErrorClass string
const (
    ClassContrib ErrorClass = "contrib"
    ClassCustom  ErrorClass = "custom"
    ClassTheme   ErrorClass = "theme"
    ClassCore    ErrorClass = "core"
)
```

### Release / Patch models

```go
type ReleaseInfo struct {
    Module     string `json:"module"`
    HasD11     bool   `json:"has_d11_release"`
    Latest     string `json:"latest_version"`
    Branches   []string `json:"compatible_branches"`
}

type PatchInfo struct {
    URL     string `json:"url"`
    Status  string `json:"status"` // RTBC, Needs Review, etc.
    Date    string `json:"date"`
    IsPatch bool   `json:"is_patch"`
    IssueNID string `json:"issue_nid"`
}
```

### MCP Tool Schemas (7 tools)

| Tool | Input | Output |
|------|-------|--------|
| `scan` | `{project_path: string}` | `ScanResult` JSON |
| `autofix` | `{project_path: string}` | `{rector_summary: string, remaining_errors: int}` |
| `contrib_check` | `{module: string}` | `ReleaseInfo` JSON |
| `issue_patches` | `{issue_nid?: string, module?: string}` | `[]PatchInfo` JSON |
| `apply_patch` | `{patch_url: string, project_path: string}` | `{applied: bool, commit_hash: string, error: string}` |
| `validate` | `{project_path: string, scope?: string, module?: string, file?: string}` | `{total_errors: int, errors: []DepError}` |
| `create_patch` | `{module: string, deprecation_details: string}` | `{patch_path: string, applied: bool}` |

### State.json

```go
type State struct {
    Version         string            `json:"version"`
    InstalledAgents []string          `json:"installed_agents"`
    PendingSync     bool              `json:"pending_sync"`
    ModelOverrides  map[string]map[string]string `json:"model_overrides,omitempty"`
}
```

## CLI Design

```
drup init                       → RunInit()   → writes drup.yaml
drup scan <path> [--json]       → RunScan()   → scan + print result
drup fix <path> [--dry-run]     → RunFix()    → full pipeline (v0.1: scan+rector+report)
drup contrib check <module>     → RunContrib()→ release-history lookup
drup issue patches <nid>        → RunIssue()  → issue scraper
drup report <path>              → RunReport() → JSON + markdown
drup mcp                        → RunMCP()    → stdio JSON-RPC server
drup install                    → RunInstall()→ detect agents, write assets
drup sync                       → RunSync()   → re-apply assets
drup upgrade                    → RunUpgrade()→ self-update
drup version [--check]          → RunVersion()
drup help                       → RunHelp()
```

Exit codes: 0=success, 1=errors found (scan/validate), 2=usage error, 3=network/external tool failure.

## Validation Gates

```
Sub-agent done → orchestrator calls validate(scope=X)
                    ├─ 0 errors → commit + next
                    └─ N errors → re-launch sub-agent with errors (×2)
                                  → escalate model (×1)
                                  → human list
```

Scope filtering: `validate --scope=contrib --module=X` returns only errors for module X. `validate --scope=custom --file=Y` returns only errors in file Y. `validate` (no scope) returns all.

Gate state machine: `running → validating → pass | re-enter | escalate | human`.

## Self-Update Flow

```
drup upgrade
  → GET /repos/{owner}/{repo}/releases/latest (Accept: application/json)
  → find asset matching GOOS/GOARCH
  → download to os.TempDir()
  → SHA256 verify against checksums.txt
  → rename current binary → .bak
  → move download → current path
  → set pending_sync=true in state.json
  → re-exec new binary → detects pending_sync → runs sync → clears flag
```

## Agent Packaging

Adapter interface:

```go
type AgentAdapter interface {
    ID() string
    Detect() bool
    SkillsDir() string
    MCPConfigPath() string
    WriteSkill(name, content string) error
    WriteMCPConfig(cfg MCPConfig) error
    Backup() (string, error) // returns backup path
}
```

Implementations: `ClaudeAdapter`, `OpenCodeAdapter`, `CodexAdapter`. Templates embedded via `//go:embed templates/`.

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | Parsers (scan, drupalorg XML, issue HTML) | Table-driven, `testdata/` fixtures |
| Unit | exec wrapper | Package-level `var execCommand` override |
| Unit | state R/W | Temp dir, round-trip |
| Integration | Mock Drupal project | Fixture dir with composer.lock + known deprecations |
| Integration | MCP tools | `httptest.Server` for drupal.org, stdio pipe for MCP |
| MCP | Tool schemas | Validate JSON schema correctness, test each handler |

Fixtures: `internal/scan/testdata/upgrade_status_*.json`, `internal/drupalorg/testdata/release_*.xml`, `internal/drupalorg/testdata/issue_*.html`.

## Threat Matrix

N/A — no routing, shell subprocess to untrusted input, VCS/PR automation, executable-file classification, or process-integration boundary in MVP. The binary runs `composer`/`drush`/`git` on the user's own project (trusted context). Patch URLs are allowlisted to drupal.org domains only.

## Migration / Rollout

No migration required. Greenfield project.

## Open Questions

- [ ] Config format: YAML (needs `gopkg.in/yaml.v3`) or JSON (stdlib only)? Proposal says YAML, but zero-deps constraint suggests JSON for MVP.
- [ ] Module path: `drup` (local) vs `github.com/gentleman-programming/drup`? User said local; explore said GitHub. Need to confirm.
- [ ] MCP MVP: hand-rolled JSON-RPC (zero deps) vs add SDK now? Proposal says SDK in v0.2.
