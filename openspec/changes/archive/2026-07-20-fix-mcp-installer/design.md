# Design: Fix MCP Installer

## Technical Approach

Fix two broken MCP config writers. Claude Code needs a per-server JSON file at `~/.claude/mcp/drup.json` in flat format (matching existing `engram.json`/`context7.json`). OpenCode needs a read-merge-write into `~/.config/opencode/opencode.json` that adds a `"drup"` key under `"mcp"` while preserving all existing config.

## Architecture Decisions

### Decision: Claude per-server file location

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `~/.claude/.mcp.json` (current) | Wrong — Claude Code doesn't read this path | Rejected |
| `~/.claude/mcp/drup.json` | Matches existing per-server pattern (`engram.json`, `context7.json`) | **Chosen** |

### Decision: Claude flat format

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `{"mcpServers": {"drup": {...}}}` (current) | Wrapper format — Claude per-server files don't use it | Rejected |
| `{"command": "...", "args": ["mcp"]}` | Matches `engram.json` and `context7.json` exactly | **Chosen** |

### Decision: OpenCode merge strategy

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Write separate `mcp.json` (current) | OpenCode reads `opencode.json`, not `mcp.json` | Rejected |
| Read-merge-write into `opencode.json` | Preserves existing keys, single source of truth | **Chosen** |

The merge uses `encoding/json` with `map[string]any` — no struct, because `opencode.json` has many keys we don't model and must preserve verbatim.

### Decision: Atomic write for OpenCode

Write to a temp file in the same directory, then `os.Rename`. Prevents partial writes from corrupting the user's config on crash or disk-full.

## Data Flow

### Claude (simple — write-only)

```
Render("claude", binaryPath)
  → template: {"command": "{{BINARY_PATH}}", "args": ["mcp"]}
  → {{BINARY_PATH}} replaced
  → WriteMCPConfig writes to ~/.claude/mcp/drup.json
```

### OpenCode (read-merge-write)

```
Render("opencode", binaryPath)
  → template: {"type": "local", "command": ["{{BINARY_PATH}}", "mcp"]}
  → {{BINARY_PATH}} replaced
  → WriteMCPConfig:
      1. Read ~/.config/opencode/opencode.json
      2. Unmarshal into map[string]any
      3. Ensure config["mcp"] exists (create if nil)
      4. Set config["mcp"]["drup"] = rendered snippet
      5. Marshal with indent
      6. Write atomically (temp + rename)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/installer/installer.go` | Modify | Change `ClaudeAdapter.MCPConfigPath()` to `~/.claude/mcp/drup.json`. Change `OpenCodeAdapter.MCPConfigPath()` to `~/.config/opencode/opencode.json`. Rewrite `OpenCodeAdapter.WriteMCPConfig()` to read-merge-write. |
| `internal/packaging/templates/claude/.mcp.json` | Modify | Flat format: `{"command": "{{BINARY_PATH}}", "args": ["mcp"]}` |
| `internal/packaging/templates/opencode/.mcp.json` | Modify | Merge snippet: `{"type": "local", "command": ["{{BINARY_PATH}}", "mcp"]}` |
| `internal/installer/installer_test.go` | Modify | Update path assertions. Add tests for merge behavior, missing file, corrupt file. |

## Interfaces / Contracts

### Claude template output (after render)

```json
{
  "command": "/usr/local/bin/drup",
  "args": ["mcp"]
}
```

### OpenCode template output (after render — the snippet to merge)

```json
{
  "type": "local",
  "command": ["/usr/local/bin/drup", "mcp"]
}
```

### OpenCode merged result (opencode.json after install)

```json
{
  "mcp": {
    "context7": { ... existing ... },
    "engram": { ... existing ... },
    "drup": {
      "type": "local",
      "command": ["/usr/local/bin/drup", "mcp"]
    }
  },
  ... all other keys preserved ...
}
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `ClaudeAdapter.MCPConfigPath()` returns `~/.claude/mcp/drup.json` | Path assertion in temp home |
| Unit | `ClaudeAdapter.WriteMCPConfig()` writes flat format | Write + read back, verify JSON shape |
| Unit | `OpenCodeAdapter.WriteMCPConfig()` merges into existing `opencode.json` | Pre-populate file with known keys, write MCP, verify all keys preserved + `mcp.drup` added |
| Unit | `OpenCodeAdapter.WriteMCPConfig()` creates new `opencode.json` when missing | No pre-existing file, verify created with correct structure |
| Unit | `OpenCodeAdapter.WriteMCPConfig()` fails on corrupt JSON | Write garbage to file, verify explicit error |
| Unit | `OpenCodeAdapter.WriteMCPConfig()` preserves existing `mcp` entries | Pre-populate with `context7` and `engram`, verify they survive merge |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. The old paths were never read by any agent (that's the bug). Existing stale files (`.mcp.json`, `mcp.json`) are harmless leftovers — out of scope for this fix.

## Open Questions

_None_
