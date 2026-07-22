# Proposal: Test Coverage for Recent Fixes and MCP Handlers

## Intent

Last 15 commits fixed self-update reliability (ETXTBSY, cross-device copy, backup paths, archive extraction), installer correctness (skill directories, MCP config paths), and added uninstall. Test coverage sits at 54.6% with large gaps in the code that was just changed. This change adds regression tests for the fixes and happy-path tests for the 10 new MCP handlers.

## Scope

### In Scope
- Phase 1: Regression tests for 6 self-update fixes, installer fixes, and uninstall command
- Phase 2: Happy-path tests for all 10 new MCP handlers in `mcp_tools.go`
- Helper tests for `parseInstalledVersion`, `hasPackage`, `extractZip`

### Out of Scope
- E2E testing infrastructure (not set up, separate effort)
- Full branch coverage of error paths in MCP handlers (only happy-path + invalid-input)
- Bubbletea/TUI tests
- `RunInstall` and `RunSync` full flow tests (deferred — require significant mocking infrastructure)

## Capabilities

### New Capabilities
None — this is a testing-only change.

### Modified Capabilities
- `self-update`: Add regression tests for backup location (`~/.drup/backups/`), cross-device copy (not rename), archive extraction from `.tar.gz`, ETXTBSY atomic staging
- `installer`: Add tests for `WriteSkill` directory creation, `RemoveMCPConfig` per-agent paths, `WriteCommand` for OpenCode/Codex adapters
- `mcp-server`: Add happy-path tests for 10 new handlers: `detect_env`, `composer_require`, `drush_exec`, `contrib_upgrade_path`, `upgrade_scan`, `patch_status`, `patch_rollback`, `generate_report`, `module_info`, `drupal_version_matrix`

## Approach

**Phase 1 — Fix regression tests (lower risk, immediate value):**
- `upgrade_test.go`: verify backup goes to `~/.drup/backups/`, `copyFile` used for cross-device, archive extraction from `.tar.gz` with nested binary
- `installer_test.go`: verify `WriteSkill` creates `<name>/SKILL.md` directory structure, MCP config paths per agent adapter
- `commands_test.go`: `RunUninstall` with mocked state, `--dry-run` output, `--force` with missing state, self-removal error paths

**Phase 2 — MCP handler happy-path tests:**
- Table-driven tests per handler: invalid JSON -> missing params -> happy path
- Mock `exec.Runner` interface for `composer_require`/`drush_exec` handlers
- Mock `drupalorg` HTTP client via `httptest` for `module_info`/`contrib_upgrade_path`
- `t.TempDir()` fixtures for filesystem-dependent handlers (`upgrade_scan`, `patch_status`)

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/update/upgrade_test.go` | Modified | Add regression tests for 6 fixes |
| `internal/installer/installer_test.go` | Modified | Add WriteSkill, RemoveMCPConfig, WriteCommand tests |
| `internal/app/commands_test.go` | Modified | Add RunUninstall state-driven tests |
| `internal/app/mcp_tools_test.go` | New | Happy-path tests for 10 handlers |
| `internal/mcp/tools_test.go` | Modified | Add placeholder handler tests |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| MCP handlers call real `exec.Command` — hard to test | High | Introduce `exec.Runner` interface or use `exec.LookPath` guards; skip in `-short` |
| `RunUninstall` calls `os.Executable()` and `statepkg.Load()` | Medium | Add override points (function variables) for test injection |
| Filesystem-dependent handlers need real git/composer state | Medium | Use `t.TempDir()` fixtures with minimal `composer.json`/git init |

## Rollback Plan

All changes are test-only files. Rollback = `git revert` of the test commits. No production code changes, zero runtime risk.

## Dependencies

- Phase 2 may require adding an `exec.Runner` interface to `internal/exec/` if one doesn't exist yet (exploration suggested dependency injection needed)

## Success Criteria

- [ ] All existing tests still pass (`go test ./...`)
- [ ] Phase 1: regression tests cover all 6 update fixes, installer skill-dir fix, uninstall state-driven flow
- [ ] Phase 2: each of the 10 new MCP handlers has at least invalid-input + happy-path test
- [ ] Overall coverage increases from 54.6% to >= 70%
- [ ] `go vet ./...` passes clean
