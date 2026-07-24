# Tasks: Sync Verbose Output

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~300 (additions + deletions across 3 files) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | N/A |
| Rollback boundary | Single commit revert |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: N/A
400-line budget risk: Low

## Phase 1: Foundation — Types and Interface

- [x] 1.1 Add `FileStatus` type (`string`) with constants `FileNew`, `FileModified`, `FileUnchanged` and `SyncFileResult` struct (`Path string`, `Status FileStatus`) in `internal/installer/installer.go`
- [x] 1.2 Add `RenderMCPConfig(snippet string) (string, error)` to `AgentAdapter` interface in `internal/installer/installer.go`
- [x] 1.3 Implement `RenderMCPConfig()` on `ClaudeAdapter` — extract merge logic from `WriteMCPConfig()` (read existing, merge snippet into `mcpServers.drup`, marshal), return merged bytes without writing
- [x] 1.4 Implement `RenderMCPConfig()` on `OpenCodeAdapter` — same pattern, merge into `mcp.drup`
- [x] 1.5 Implement `RenderMCPConfig()` on `CodexAdapter` — return snippet as-is (no merge; Codex writes flat)
- [x] 1.6 Refactor each adapter's `WriteMCPConfig()` to call its own `RenderMCPConfig()` + atomic write, removing duplicated merge logic

## Phase 2: Core — Install() Refactor

- [x] 2.1 Add `resolveFilePath(agent AgentAdapter, path string) string` helper in `installer.go` — maps logical paths (`SKILL.md`, `agents/*.md`, `commands/*.md`, `.mcp.json`, `CLAUDE.md`, `copilot-instructions.md`) to absolute paths per adapter
- [x] 2.2 Refactor `Install()` signature to `Install(agents []AgentAdapter, binaryPath string, files map[string]string) ([]SyncFileResult, error)` — for each agent, for each file: compute intended content (use `RenderMCPConfig()` for `.mcp.json`), resolve absolute path, read existing bytes, determine status (`new`/`modified`/`unchanged`), collect result
- [x] 2.3 Move backup inside `Install()` after status computation — call `BackupConfig()` only when at least one file for that agent is `new` or `modified`
- [x] 2.4 Skip write for files with status `unchanged`; write only `new`/`modified` files

## Phase 3: Wiring — Update Callers

- [x] 3.1 Update `RunSync()` in `internal/app/commands.go` — capture `([]SyncFileResult, error)` from `Install()`, print per-file status grouped by agent (e.g., `  new: path`, `  modified: path`, `  unchanged: path`)
- [x] 3.2 Update `RunInstall()` in `internal/app/commands.go` — capture `([]SyncFileResult, error)` from `Install()`, print summary or per-file status

## Phase 4: Tests

- [x] 4.1 Update `TestInstall_WritesFiles` in `installer_test.go` for new `([]SyncFileResult, error)` return signature; assert all results have status `new`
- [x] 4.2 Add `TestInstall_AllUnchanged` — install twice, second call returns all `unchanged`, no files rewritten (compare modtime or content hash)
- [x] 4.3 Add `TestInstall_MixedStatus` — pre-populate one file with different content, verify `modified` for that file and `unchanged` for others
- [x] 4.4 Add `TestInstall_MCPPostMergeComparison` — pre-populate OpenCode `opencode.json` with other MCP entries; verify status is `unchanged` when merged output matches existing file byte-for-byte
- [x] 4.5 Add `TestInstall_BackupSkippedWhenUnchanged` — install twice, verify `BackupConfig` not called on second run (use backup dir override to assert no new backup created)
