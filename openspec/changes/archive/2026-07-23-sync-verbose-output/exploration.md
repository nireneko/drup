## Exploration: sync-verbose-output

### Current State

The `drup sync` command is implemented in `internal/app/commands.go:519-554` (`RunSync`). It:

1. Loads state from `~/.config/drup/state.json` to get the list of installed agents.
2. Re-detects agents via `installer.DetectAgents()` (checks for `~/.claude`, `~/.config/opencode`, `~/.codex`).
3. For each detected agent, calls `packaging.Render(agentID, binaryPath)` to get a `map[string]string` of relative path → content.
4. Calls `installer.Install([]AgentAdapter{agent}, binaryPath, files)` which writes each file to the agent's directories.
5. Prints `Synced drup to <agentID>` per agent.
6. Clears `PendingSync` flag in state.

The `files` map returned by `packaging.Render` contains the exact set of files that will be written. The keys are relative paths like `SKILL.md`, `agents/drup-preflight.md`, `.mcp.json`, etc. The `installer.Install` function iterates this map and dispatches each file to the correct Write method on the adapter.

**No information about which files changed is tracked or displayed.** The function writes unconditionally and reports only the agent name.

### Affected Areas

- `internal/app/commands.go` — `RunSync()` function (lines 519-554): needs to collect and display per-file change information.
- `internal/installer/installer.go` — `Install()` function (lines 843-902): currently returns only `error`. Needs to return information about what was written and whether each file was new, modified, or unchanged.
- `internal/packaging/packaging.go` — `Render()` is read-only and provides the full content map; no changes needed here.

### What Files Are Synced (per agent)

From `internal/packaging/templates/`:

**Claude** (`~/.claude/skills/`, project root):
- `SKILL.md` → `~/.claude/skills/drup/SKILL.md`
- `CLAUDE.md` → `<project>/CLAUDE.md`
- `.mcp.json` → `<project>/.mcp.json` (merged into existing)
- `skills/drupal-contrib-patch-writer/SKILL.md`
- `skills/drupal-custom-d11-fixes/SKILL.md`

**OpenCode** (`~/.config/opencode/skills/`):
- `SKILL.md` → `~/.config/opencode/skills/drup/SKILL.md`
- `.mcp.json` → `~/.config/opencode/opencode.json` (merged into existing)
- `skills/drupal-contrib-patch-writer/SKILL.md`
- `skills/drupal-custom-d11-fixes/SKILL.md`

**Codex** (`~/.codex/skills/`):
- `SKILL.md` → `~/.codex/skills/drup/SKILL.md`
- `.mcp.json` → `~/.codex/mcp.json` (overwritten)
- `copilot-instructions.md` → `<project>/.github/copilot-instructions.md`
- `skills/drupal-contrib-patch-writer/SKILL.md`
- `skills/drupal-custom-d11-fixes/SKILL.md`

### Approaches

1. **Pre-write comparison in `Install()`** — Before writing each file, read the existing content and compare. Return a list of `SyncResult` entries (path, status: new/modified/unchanged) from `Install()`. `RunSync` prints the results.
   - Pros: Single source of truth for change detection; callers get structured data; testable.
   - Cons: Adds I/O (read before write) for every file on every sync. Small cost given ~5 files per agent.
   - Effort: Low

2. **Content hash comparison** — Hash existing files and compare against rendered content hash. Only report changed files.
   - Pros: Efficient for large files.
   - Cons: Overkill for ~5 small template files; adds complexity without meaningful benefit over direct byte comparison.
   - Effort: Low-Medium

3. **Post-write git diff** — After sync, run `git diff` on the affected paths to show what changed.
   - Pros: Shows actual line-level diffs.
   - Cons: Sync writes to `~/.claude/`, `~/.config/opencode/`, etc. which are typically NOT inside a git repo. Only project-root files (`CLAUDE.md`, `.mcp.json`, `.github/copilot-instructions.md`) could be diffed. Unreliable and environment-dependent.
   - Effort: Medium

### Recommendation

**Approach 1: Pre-write comparison in `Install()`**. It's the simplest correct solution:

- Modify `Install()` to return `([]SyncFileResult, error)` where `SyncFileResult` has `Path string`, `Status string` (new/modified/unchanged).
- Before each write, read existing file content and compare bytes.
- For MCP configs (which merge into existing JSON), compare the final merged output against the existing file.
- `RunSync()` collects results per agent and prints them.

Expected output format:
```
Synced drup to claude:
  ~/.claude/skills/drup/SKILL.md (modified)
  ~/project/CLAUDE.md (unchanged)
  ~/project/.mcp.json (modified)
  ...
```

### Risks

- **MCP config merge comparison**: Claude and OpenCode merge into existing JSON files. The comparison must be against the final merged content, not the template snippet. This is naturally handled if comparison happens after the merge logic.
- **Idempotent writes**: Some Write methods (like `WriteMCPConfig`) use atomic temp-file + rename. The comparison must happen before the write, reading the current file state.
- **No existing sync tests**: There are no tests for `RunSync` in `internal/app/`. Tests will need to be added.
- **Backward compatibility**: Changing `Install()` return signature will break callers. Only `RunSync` and `RunInstall` call it — both need updating.

### Ready for Proposal

Yes. The change is well-scoped to 2-3 files, has clear input/output, and the approach is straightforward.
