# Proposal: Sync Verbose Output

## Intent

`drup sync` writes ~5 files per agent unconditionally and prints only `Synced drup to <agent>`. Users cannot tell which files actually changed. Add per-file status reporting (new / modified / unchanged) so sync output is informative and idempotent runs are visibly no-ops.

## Scope

### In Scope
- Pre-write byte comparison in `Install()` for every synced file
- Structured `[]SyncFileResult` return from `Install()` with path + status
- Per-file status output in `RunSync()` grouped by agent
- Comparison against post-merge content for MCP config files
- Update `RunInstall()` caller for new `Install()` signature
- Tests for change detection logic

### Out of Scope
- Line-level diffs (git diff not viable — most targets are outside repos)
- Content hash / caching layer (overkill for ~5 small templates)
- Dry-run or `--check` mode
- Changes to `packaging.Render()` or template content

## Capabilities

### New Capabilities
_None_

### Modified Capabilities
- `installer`: `Install()` returns `([]SyncFileResult, error)` instead of `error`. Each result entry carries absolute path and status (`new`, `modified`, `unchanged`). Comparison happens post-merge for MCP configs.

## Approach

Modify `Install()` in `installer/installer.go` to read existing content before each write, compare bytes, and collect a `SyncFileResult` per file. For MCP configs, compare post-merge output against current file. `RunSync()` prints per-file status per agent. `RunInstall()` adapts to new signature.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/installer/installer.go` | Modified | `Install()` returns `[]SyncFileResult`; pre-write read + compare per file |
| `internal/installer/installer_test.go` | Modified | Add tests for new/modified/unchanged detection |
| `internal/app/commands.go` | Modified | `RunSync()` prints per-file status; `RunInstall()` adapts to new signature |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| MCP merge comparison reads pre-merge file instead of post-merge | Low | Compare after merge logic produces final content, before write |
| Extra disk reads slow sync | Low | ~5 small files per agent; negligible I/O cost |
| `Install()` signature change breaks callers | Low | Only 2 callers (`RunSync`, `RunInstall`); both updated in same change |

## Rollback Plan

Revert the single commit. No data migration, no state format changes. `Install()` returns to `error`-only signature.

## Dependencies

_None_

## Success Criteria

- [ ] `drup sync` prints per-file status (new / modified / unchanged) for each agent
- [ ] Re-running sync with no template changes shows all files as `unchanged`
- [ ] MCP config comparison uses post-merge content, not template snippet
- [ ] Existing `RunInstall` flow continues to work with updated signature
- [ ] Tests cover new, modified, and unchanged scenarios
