## Exploration: drup-mvp — Drupal 8/9/10 → 11 upgrade harness

### Gentle-ai Architecture Summary

- **Module path**: `github.com/gentleman-programming/gentle-ai` (go.mod line 1)
- **Go version**: `go 1.25.10` in go.mod (NOT 1.26.0 as openspec/config.yaml claims — config.yaml is wrong, reconcile before implementation)
- **CLI structure**: No cobra. `cmd/gentle-ai/main.go` is a 20-line shim that calls `app.Run()`. `internal/app/app.go` does manual `os.Args` dispatch via a `switch args[0]` — no cobra, no pflag. Commands: `install`, `sync`, `update`, `upgrade`, `restore`, `doctor`, `version`, `help`, `uninstall`, `skill-registry`, `sdd-status`, `sdd-continue`, `codegraph`, `review*`.
- **Package layout**: 25 packages under `internal/`. Top-level responsibilities:
  - `app/` — top-level dispatch, TUI wiring, self-update orchestration
  - `cli/` — command implementations (RunInstall, RunSync, RunRestore, etc.)
  - `agents/` — per-agent adapters (claude/, opencode/, codex/, pi/, kiro/, kimi/, kilocode/, cursor/, etc.) implementing `agents.Adapter` interface
  - `components/` — injectable capabilities (mcp/, skills/, permissions/, uninstall/, filemerge/, communitytool/)
  - `model/` — shared types (AgentID, ComponentID, Selection, etc.)
  - `state/` — JSON state persistence (~/.gentle-ai/state.json)
  - `update/` — read-only update checking; `update/upgrade/` — write-path executor (download, checksum, atomic replace)
  - `installcmd/` — resolver for agent install commands (npm, uv, etc.)
  - `pipeline/`, `planner/` — execution orchestration
  - `tui/` — Bubble Tea TUI (charmbracelet/bubbletea)
  - `system/` — platform detection, OS support guards
  - `backup/` — config snapshots with manifests
  - `verify/` — post-install verification scenarios
  - `assets/` — embedded skills/agents content
  - `skillregistry/`, `versions/` — metadata
- **MCP implementation**: gentle-ai is an MCP **consumer**, not a server. It injects MCP server configs (context7, engram) into agent configs via `internal/components/mcp/inject.go`. No `modelcontextprotocol/go-sdk` or `mark3labs/mcp-go` in go.mod. For drup's `drup mcp` server command, we need to ADD a Go MCP SDK dependency — gentle-ai offers no reference implementation.
- **Self-update mechanism**: `internal/update/upgrade/download.go` — Download() resolves binary path via LookPath, downloads tar.gz from GitHub Releases, verifies SHA256 against checksums.txt, extracts binary, does atomic rename. `executor.go` wraps it with backup snapshots, strategy selection (homebrew vs binary), and report rendering. `app.go` calls selfUpdate() on non-TUI, non-update launches.
- **Install/sync mechanism**: `internal/agents/<agent>/adapter.go` implements `agents.Adapter` interface with methods for Detect(), paths (GlobalConfigDir, SkillsDir, MCPConfigPath, etc.), and strategies (MCPStrategy, SystemPromptStrategy). `internal/cli/run.go` and `internal/cli/sync.go` iterate installed agents and call component injectors (mcp.Inject, skills.Inject, etc.) per adapter. Per-agent adapters are the extension point.
- **State format**: `internal/state/state.go` — `InstallState` struct with `InstalledAgents []string`, `PendingSync bool`, `LastUpdateCheck *time.Time`, plus per-agent model assignment maps (ClaudePhaseAssignments, CodexModelAssignments, ModelAssignments, etc.). Stored at `~/.gentle-ai/state.json`. Atomic writes via `filemerge.WriteFileAtomic`. `MergeAgents()` for incremental agent additions. `PendingSync` flag set after self-upgrade, consumed on next launch to auto-run sync.
- **Testing patterns**: Table-driven tests throughout (see `state_test.go` — `tests := []struct{...}` with `t.Run(tt.name, ...)`). Package-level vars for testability (`var execCommand = exec.Command`, `var lookPathFn = exec.LookPath`, `var osStat = os.Stat`). No testify — stdlib `testing` + `reflect.DeepEqual` + `t.Errorf`. `internal/testdata/` for fixtures.
- **Build tooling**: `.goreleaser.yaml` v2 — CGO_ENABLED=0, linux/darwin/windows × amd64/arm64, ldflags `-X main.version={{.Version}}`, Homebrew tap + Scoop bucket. `scripts/install.sh` priority: brew > binary > go install.

### Conventions to Mirror

1. **Manual CLI dispatch** — no cobra. `app.Run()` → `switch args[0]`. PRD says cobra but gentle-ai doesn't use it; mirror the reference project OR deliberately diverge with cobra (PRD explicitly says cobra — follow PRD for drup, it's a deliberate choice).
2. **Per-agent adapter interface** — `agents.Adapter` with Detect/paths/strategy methods. drup needs the same for claude-code, opencode, codex.
3. **Package-level vars for testability** — `var execCommand = exec.Command` pattern for mocking subprocess calls in tests.
4. **State + PendingSync** — `~/.drup/state.json` with `pending_sync` flag for deferred sync after self-upgrade.
5. **Self-update = download + checksum + atomic replace** — same flow as gentle-ai's `upgrade/download.go`.

### Proposed Module Path

`github.com/gentleman-programming/drup`

Matches the gentle-ai pattern (`github.com/gentleman-programming/gentle-ai`). Same GitHub org.

### Go Tooling Versions Detected

| Item | gentle-ai actual | drup openspec/config.yaml says | Reconciliation |
|------|-----------------|-------------------------------|----------------|
| Go version | 1.25.10 | 1.26.0 | **config.yaml is wrong** — Go 1.26 doesn't exist yet (current date: July 2026, Go 1.25 is latest stable). Use 1.25.10 or whatever is current at init time. |
| CLI framework | manual args dispatch | cobra | PRD says cobra — follow PRD. gentle-ai's manual dispatch works but cobra is better for drup's subcommand tree (`drup contrib check`, `drup issue patches`). |
| MCP SDK | none (consumer only) | not specified | Need to add `github.com/modelcontextprotocol/go-sdk` or `mark3labs/mcp-go` for `drup mcp`. |

### Key Implementation Patterns to Reuse

1. **Self-update flow** (`internal/update/upgrade/download.go`) — download tar.gz → verify SHA256 against checksums.txt → extract → atomic rename. Copy this pattern verbatim for `drup upgrade`.
2. **State + deferred sync** (`internal/state/state.go` + `app.go` PendingSync handling) — same `pending_sync` flag pattern for `drup upgrade` → auto-sync on next launch.
3. **Adapter interface for agents** (`internal/agents/interface.go` + per-agent packages) — drup's `internal/packaging` or `internal/installer` needs the same Adapter pattern for writing skills/MCP configs to claude-code, opencode, codex.

### Risks and Gaps

- **No cobra in gentle-ai**: PRD says cobra for drup. This is a deliberate divergence — gentle-ai uses manual dispatch. Cobra is the right call for drup's deeper subcommand tree. Add `github.com/spf13/cobra` as a dependency.
- **No MCP server in gentle-ai**: gentle-ai only injects MCP configs, it doesn't serve MCP. drup's `drup mcp` (stdio server exposing scan/validate/apply_patch/contrib_check/issue_patches) needs a Go MCP SDK. Gentle-ai provides no reference — must build from scratch using `modelcontextprotocol/go-sdk`.
- **openspec/config.yaml Go version is wrong**: says 1.26.0, actual gentle-ai uses 1.25.10. Fix config.yaml before implementation.
- **drup directory is empty**: no go.mod, no cmd/, no internal/. sdd-init created openspec/ and .atl/ but no Go scaffolding. First task must be `go mod init` + directory structure.
- **No testdata fixtures yet**: gentle-ai uses `internal/testdata/` and `testdata/` at root. drup needs fixtures for upgrade_status JSON, release-history XML, and drupal.org issue HTML scraping.

### Ready for Proposal

**Yes.** The reference architecture is clear. The PRD maps cleanly to gentle-ai's patterns with two deliberate divergences (cobra, MCP server SDK). The drup directory is ready for `go mod init github.com/gentleman-programming/drup` and scaffolding.

Key decisions to lock in the proposal:
1. Module path: `github.com/gentleman-programming/drup`
2. CLI: cobra (per PRD), not manual dispatch (per gentle-ai)
3. MCP SDK: `github.com/modelcontextprotocol/go-sdk` (official) or `mark3labs/mcp-go` (community)
4. Go version: match gentle-ai's 1.25.10 or use current stable at time of init
