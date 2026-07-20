# Tasks: Fix MCP Installer

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~150–200 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | N/A |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: N/A
400-line budget risk: Low

## Phase 1: Templates

- [x] 1.1 Rewrite `internal/packaging/templates/claude/.mcp.json` to flat format: `{"command": "{{BINARY_PATH}}", "args": ["mcp"]}`
- [x] 1.2 Rewrite `internal/packaging/templates/opencode/.mcp.json` to merge snippet: `{"type": "local", "command": ["{{BINARY_PATH}}", "mcp"]}`

## Phase 2: Core Logic (installer.go)

- [x] 2.1 Change `ClaudeAdapter.MCPConfigPath()` to return `~/.claude/mcp/drup.json`
- [x] 2.2 Change `OpenCodeAdapter.MCPConfigPath()` to return `~/.config/opencode/opencode.json`
- [x] 2.3 Rewrite `OpenCodeAdapter.WriteMCPConfig()`: read existing `opencode.json` into `map[string]any`, ensure `"mcp"` key exists, set `config["mcp"]["drup"]` from rendered content, marshal with indent, write atomically via temp file + `os.Rename`
- [x] 2.4 Handle missing file: if `opencode.json` doesn't exist, create new `map[string]any` with `"mcp"` → `"drup"` entry
- [x] 2.5 Handle corrupt file: if `json.Unmarshal` fails, return explicit error and do NOT overwrite the file

## Phase 3: Tests

- [x] 3.1 Update `TestClaudeAdapter_Paths` to assert `MCPConfigPath()` returns `~/.claude/mcp/drup.json`
- [x] 3.2 Add `TestOpenCodeAdapter_WriteMCPConfig_MergesExisting` — pre-populate `opencode.json` with other `mcp` entries, verify all keys preserved + `drup` added
- [x] 3.3 Add `TestOpenCodeAdapter_WriteMCPConfig_CreatesNew` — no existing file, verify created with correct `mcp.drup` structure
- [x] 3.4 Add `TestOpenCodeAdapter_WriteMCPConfig_CorruptFile` — write garbage JSON, verify error returned and file not overwritten
- [x] 3.5 Add `TestOpenCodeAdapter_WriteMCPConfig_OverwritesExistingDrup` — existing `drup` entry, verify it gets replaced
- [x] 3.6 Update `TestInstall_WritesFiles` MCP content fixture to flat format for Claude

## Phase 4: Verify

- [x] 4.1 Run `go test ./internal/installer/...` — all tests pass
- [x] 4.2 Run `go build ./...` — compiles clean
