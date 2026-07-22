## Exploration: analiza-cambios-codigo-actualiza-tests

### Current State

Recent commits (last 15) fixed MCP tools, installer paths, self-update reliability, and added uninstall. Overall test coverage is **54.6%** — all tests pass, but large gaps remain in the code that was just added/fixed.

### What Changed (commits grouped by area)

**1. MCP Tools** — `0e4e19e`
- Added 10 new tools: `detect_env`, `composer_require`, `drush_exec`, `contrib_upgrade_path`, `upgrade_scan`, `patch_status`, `patch_rollback`, `generate_report`, `module_info`, `drupal_version_matrix`
- Real handlers in `internal/app/mcp_tools.go` (1182 lines)
- Placeholders in `internal/mcp/tools.go` (330 lines)

**2. Installer Fixes** — `c374574`, `42ed681`
- MCP config written to correct agent paths with proper format
- Skills written as directories (`<name>/SKILL.md`) not flat files

**3. Self-Update Reliability** — `0babeba`, `26781ba`, `254f7c5`, `18a3007`, `2b521f2`, `0b7dcfa`
- ETXTBSY fix: stage new binary via temp file, then atomic rename
- Cross-device copy (not rename) for binary replacement
- Backup moved to `~/.drup/backups/` to avoid permission issues
- Binary extracted from `.tar.gz` archive before replacing
- Archive extension preserved in temp download file
- New `upgrade.go` (370 lines) with full download→verify→extract→replace flow

**4. Uninstall Command** — `2e9f1a1`
- `RunUninstall` with `--dry-run`, `--force` flags
- State-driven adapter selection, self-removal of binary

**5. Environment Detection** — part of MCP tools
- `internal/envdetect/` (146 lines) — detects DDEV/Lando/Docker/local

### Affected Areas

| File | Coverage | Status |
|------|----------|--------|
| `internal/app/mcp_tools.go` | 8–90% per func | 10 new handlers, most <50% |
| `internal/app/commands.go` (RunUninstall) | 61.4% | Only flag-parsing tests |
| `internal/app/commands.go` (RunInstall) | 0% | No tests |
| `internal/app/commands.go` (RunSync) | 0% | No tests |
| `internal/app/commands.go` (extractZip) | 0% | No tests |
| `internal/mcp/tools.go` (placeholders) | 0% for all new handlers | Only protocol tests |
| `internal/installer/installer.go` (RemoveMCPConfig) | 43.6% | Partial |
| `internal/installer/installer.go` (WriteCommand) | 0% | No tests |
| `internal/update/upgrade.go` | 66–100% per func | Well-tested (783 lines) |
| `internal/envdetect/envdetect.go` | 95%+ | Excellent |

### Test Coverage Gaps (by priority)

**HIGH — Real MCP handler logic (`mcp_tools.go`)**

| Handler | Coverage | What's missing |
|---------|----------|----------------|
| `realHandleComposerRequire` | 20.5% | Happy path with valid package, `--dev` flag, `no_update` |
| `realHandleDrushExec` | 26.5% | Happy path, format flag, args passing |
| `realHandleContribUpgradePath` | 36.4% | Happy path with valid module/version |
| `realHandleUpgradeScan` | 8.1% | Happy path, scope filtering, module filtering |
| `realHandlePatchStatus` | 0% | All paths |
| `realHandlePatchRollback` | 13.4% | Happy path (dirty tree & non-git tested) |
| `realHandleGenerateReport` | 62.9% | `json`-only, `markdown`-only, `include_scan_data` |
| `realHandleModuleInfo` | 36.4% | Happy path with valid module |
| `parseInstalledVersion` | 0% | All paths |
| `hasPackage` | 0% | All paths |

**HIGH — Uninstall command (`commands.go`)**
- State-driven adapter selection (only 2 trivial tests exist)
- Dry-run output verification
- `--force` with missing/empty state
- Self-removal error handling
- Confirmation prompt flow

**MEDIUM — Zero-coverage commands**
- `RunInstall` (0%) — entire install flow untested
- `RunSync` (0%) — entire sync flow untested
- `extractZip` (0%) — zip extraction untested

**MEDIUM — Installer gaps**
- `RemoveMCPConfig` (43.6% Claude, 65.7% OpenCode)
- `WriteCommand` (0% for OpenCode/Codex adapters)
- `CodexAdapter` methods mostly 0%

**LOW — Update/upgrade (already well-tested)**
- `writeExecutable` error paths (66.7%)
- `copyFile` error paths (75%)
- `BackupBinary`/`RestoreBinary` edge cases

### Approaches

1. **Fill gaps by area, highest coverage impact first**
   - Pros: Maximizes coverage delta per effort; MCP handlers are the biggest gap
   - Cons: Some handlers need real exec mocking (composer, drush)
   - Effort: Medium

2. **Test the fixes, not the handlers**
   - Focus on the bug-fix commits: ETXTBSY, cross-device, backup location, archive extraction, skill directories, MCP config paths
   - Pros: Directly validates what broke; smaller test surface
   - Cons: Leaves new feature handlers untested
   - Effort: Low

3. **Both — fixes first, then handler coverage**
   - Phase 1: Tests for the 6 update fixes + installer fixes + uninstall
   - Phase 2: MCP handler happy-path tests with dependency injection
   - Pros: Validates fixes immediately, then broadens coverage
   - Cons: Larger scope
   - Effort: High

### Recommendation

**Approach 3** — fixes first (Phase 1), then handler coverage (Phase 2).

Rationale: The user explicitly said "recent commits fixed problems" — those fixes need regression tests first. Then fill the MCP handler gaps which are the largest coverage hole.

**Phase 1 tests (fixes):**
- `upgrade_test.go`: verify backup goes to `~/.drup/backups/`, verify `copyFile` used (not rename) for cross-device, verify archive extraction from `.tar.gz`
- `installer_test.go`: verify `WriteSkill` creates directory + `SKILL.md`, verify MCP config paths per agent
- `commands_test.go`: `RunUninstall` with mocked state, dry-run, force mode

**Phase 2 tests (MCP handlers):**
- Table-driven tests per handler: invalid JSON → missing params → happy path
- Mock `exec.Runner` for composer/drush handlers
- Mock `drupalorg` HTTP client for module_info/contrib_upgrade_path

### Risks

- MCP handlers call real `exec.Command` — need dependency injection or `exec.LookPath` guards to make testable
- `RunUninstall` calls `os.Executable()` and `statepkg.Load()` — both need override points for testing
- Some handlers (`upgrade_scan`, `patch_status`) depend on filesystem state (composer.json, git) — need `t.TempDir()` fixtures

### Ready for Proposal

**Yes.** The orchestrator should tell the user: exploration identified 3 priority tiers of test gaps. Phase 1 covers the recent fixes (update reliability, installer paths, uninstall). Phase 2 covers MCP handler happy paths. Recommend proceeding to proposal.
