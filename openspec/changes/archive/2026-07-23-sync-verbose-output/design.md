# Design: Sync Verbose Output

## Technical Approach

Add per-file change detection to `Install()` by reading existing bytes before each write, comparing, and returning structured `[]SyncFileResult`. For MCP configs, add `RenderMCPConfig()` to `AgentAdapter` so the merge logic runs before comparison — status reflects post-merge content, not the raw template snippet. `RunSync()` prints per-file status grouped by agent. Backup moves inside `Install()` and only fires when at least one file is `new` or `modified`.

## Architecture Decisions

| Decision | Option A | Option B | Choice | Rationale |
|----------|----------|----------|--------|-----------|
| MCP comparison timing | Compare post-merge via `RenderMCPConfig()` on adapter | Compare raw template snippet | A | Spec requires post-merge; raw snippet would always differ from existing merged file |
| Backup trigger | Inside `Install()`, after status computation, only if any file is new/modified | Keep unconditional backup at top of `Install()` | A | Spec: "SHALL NOT backup when status is unchanged" |
| Adapter interface change | Add `RenderMCPConfig(snippet string) (string, error)` to `AgentAdapter` | Inline merge logic per adapter type-switch in `Install()` | A | Type-switch breaks open/closed; interface method keeps adapter encapsulation |
| Path resolution | Helper `resolveFilePath(agent, path)` in installer package | Each caller resolves paths | Helper | Single place maps logical paths (SKILL.md, agents/*.md, .mcp.json) to absolute paths |

## Data Flow

```
RunSync()
  │
  ├─ packaging.Render(agentID, binaryPath) → map[string]string
  │
  └─ installer.Install(agents, binaryPath, files)
       │
       ├─ For each agent:
       │    ├─ For each file: compute intended content
       │    │    ├─ .mcp.json → agent.RenderMCPConfig(snippet) → merged JSON
       │    │    └─ other files → content as-is
       │    │
       │    ├─ resolveFilePath(agent, path) → absolute path
       │    ├─ os.ReadFile(absPath) → existing bytes (or not-found)
       │    ├─ Compare: new | modified | unchanged
       │    │
       │    ├─ If ANY file is new/modified → BackupConfig(agent.SkillsDir())
       │    │
       │    └─ For each file with status != unchanged → write
       │
       └─ Return []SyncFileResult
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/installer/installer.go` | Modify | Add `SyncFileResult` type, `FileStatus` type, `RenderMCPConfig()` to `AgentAdapter` + all 3 adapters. Refactor `Install()` to compare-before-write, return `[]SyncFileResult`, conditional backup. Add `resolveFilePath()` helper. |
| `internal/installer/installer_test.go` | Modify | Add tests: all-new, all-unchanged, mixed, MCP post-merge comparison, backup skipped when unchanged. Update existing `TestInstall_WritesFiles` for new signature. |
| `internal/app/commands.go` | Modify | `RunSync()`: adapt to `([]SyncFileResult, error)` return, print per-file status grouped by agent. `RunInstall()`: same signature adaptation, discard results or print summary. |

## Interfaces / Contracts

```go
// FileStatus represents the change detection result for a single file.
type FileStatus string

const (
    FileNew       FileStatus = "new"
    FileModified  FileStatus = "modified"
    FileUnchanged FileStatus = "unchanged"
)

// SyncFileResult holds the outcome for one synced file.
type SyncFileResult struct {
    Path   string     // Absolute path of the file
    Status FileStatus // new, modified, or unchanged
}

// AgentAdapter — ADD one method:
type AgentAdapter interface {
    // ... existing methods ...
    RenderMCPConfig(snippet string) (string, error) // Returns fully merged config content without writing
}
```

`RenderMCPConfig` extracts the merge logic already inside each adapter's `WriteMCPConfig` — read existing, merge snippet, marshal — but returns the bytes instead of writing. `WriteMCPConfig` can be refactored to call `RenderMCPConfig` + write.

## Testing Strategy

| Layer | What | Approach |
|-------|------|----------|
| Unit | `Install()` returns correct status for new/modified/unchanged | Table-driven tests with temp dirs; pre-populate files for modified/unchanged scenarios |
| Unit | MCP post-merge comparison | Pre-populate `opencode.json` with other MCP entries; verify status is `unchanged` when merged output matches |
| Unit | Backup skipped when all unchanged | Assert `BackupConfig` not called (or backup dir empty) when all files unchanged |
| Unit | `resolveFilePath()` | Verify each logical path maps to correct absolute path per adapter |
| Integration | `RunSync()` output format | Capture stdout; verify per-file status lines grouped by agent |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. `Install()` signature change affects only `RunSync()` and `RunInstall()` — both updated in the same commit. No state format changes, no data migration.

## Open Questions

None.
