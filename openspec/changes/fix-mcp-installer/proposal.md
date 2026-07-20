# Proposal: Fix MCP Installer

## Intent

`drup install` writes MCP server config to wrong paths in wrong formats for both Claude Code and OpenCode. After install, the MCP server is never registered — the core value proposition of the tool is broken for every user.

## Scope

### In Scope
- Fix Claude Code: write per-server file at `~/.claude/mcp/drup.json` in flat format (`{"command": "...", "args": ["mcp"]}`)
- Fix OpenCode: merge `"drup"` entry into `~/.config/opencode/opencode.json` under `"mcp"` key with `"type": "local"` and `"command"` as array
- Update templates in `internal/packaging/templates/{claude,opencode}/.mcp.json`
- Update `ClaudeAdapter.MCPConfigPath()`, `OpenCodeAdapter.MCPConfigPath()`, `OpenCodeAdapter.WriteMCPConfig()` in `installer.go`
- Update existing installer spec to reflect correct paths/formats

### Out of Scope
- Codex adapter (no evidence it's broken or used; leave for when someone reports it)
- MCP config uninstall/cleanup (separate concern)
- Changes to backup/restore logic

## Capabilities

### New Capabilities
_None_

### Modified Capabilities
- `installer`: MCP config paths and formats change for Claude Code and OpenCode adapters

## Approach

**Claude Code**: Change `MCPConfigPath()` to return `~/.claude/mcp/drup.json`. Change template to flat format (remove `mcpServers` wrapper). `WriteMCPConfig()` stays the same (write file).

**OpenCode**: Change `WriteMCPConfig()` to read `opencode.json`, merge a `"drup"` key under `"mcp"` with `{"type": "local", "command": ["/path/to/binary", "mcp"]}`, and write back. `MCPConfigPath()` returns `opencode.json` path. Template changes to the merge-ready snippet format.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/installer/installer.go` | Modified | Fix `MCPConfigPath()` for both adapters; rewrite `OpenCodeAdapter.WriteMCPConfig()` to read-merge-write `opencode.json` |
| `internal/packaging/templates/claude/.mcp.json` | Modified | Flat format: `{"command": "...", "args": ["mcp"]}` |
| `internal/packaging/templates/opencode/.mcp.json` | Modified | Snippet format: `{"type": "local", "command": ["{{BINARY_PATH}}", "mcp"]}` |
| `openspec/specs/installer/spec.md` | Modified | Update scenarios to reflect correct paths and formats |
| `internal/installer/installer_test.go` | Modified | Update/add tests for new paths and merge behavior |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| OpenCode `opencode.json` merge corrupts existing config | Medium | Parse with `encoding/json`, preserve all existing keys, write with indentation. Add test with real config fixture. |
| Claude Code format changes again in future | Low | Keep template simple; the flat format matches all known per-server MCP configs |
| Existing broken installs leave stale `.mcp.json` files | Low | Not in scope for this fix; document in release notes |

## Rollback Plan

Revert the commit. The old (broken) behavior is restored. No data loss — the old paths were never read by any agent.

## Dependencies

_None_

## Success Criteria

- [ ] `drup install` creates `~/.claude/mcp/drup.json` with flat format containing correct binary path
- [ ] `drup install` merges `"drup"` entry into `~/.config/opencode/opencode.json` under `"mcp"` with `"type": "local"` and `"command"` as array
- [ ] Claude Code detects and loads the drup MCP server after install
- [ ] OpenCode detects and loads the drup MCP server after install
- [ ] Existing `opencode.json` keys are preserved after merge
- [ ] All installer tests pass with updated expectations
