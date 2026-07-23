# Proposal: drup-retrospective-fixes

## Intent

Fix the 6 critical issues documented in AGENT-RETROSPECTIVE.md that forced ~60% of the drup automated pipeline to be bypassed during a real Drupal 10.6 → 11.4 upgrade. These bugs made the CLI unusable in DDEV environments, blocked agent tool discovery, and produced false negatives in contrib compatibility checks.

## Scope

### In Scope
- Exit code 3 semantic handling in `drup scan` and `drup validate`
- DDEV-aware execution for all CLI commands (replace `-r` with `--root=`, use `RunWithEnv`)
- MCP tool parameter schema exposure (20 tools)
- Contrib compatibility parsing for compound constraints (`^10.3 || ^11.0`)
- PHP 8.4 deprecation suppression in preflight (auto-patch `settings.php`)
- Report data collection (replace hardcoded zeros with real scan data)

### Out of Scope
- Stage 6 (CUSTOM LOOP) implementation — no CLI command exists; deferred to separate change
- E2E test infrastructure (planned for later phase per `openspec/config.yaml`)
- Sub-agent fallback tools when MCP fails
- `drup fix` error handling when no custom modules exist (minor, not blocking)

## Capabilities

### New Capabilities
None

### Modified Capabilities
- `scan`: Exit code 3 must be treated as success-with-findings, not error
- `validation-gates`: Same exit code 3 semantic handling
- `mcp-server`: Tool schemas must expose parameter definitions (not empty objects)
- `contrib-check`: Parse `core_version_requirement` with compound constraint support
- `preflight`: Detect PHP 8.4+ and auto-patch `settings.php` to suppress `E_DEPRECATED`
- `report`: Collect real scan data instead of hardcoded zeros

## Approach

Fix in priority order (P0 → P1 → P2):

**P0 — Blockers**
1. **Exit code 3**: In `RunScan`, `DoValidate`, `realHandleScan`, `realHandleValidate` — parse stdout when exit code is 3, only treat exit codes 1, 2, >3 as errors. Distinguish "findings exist" (stdout has data) from "drush crashed" (stderr has error).
2. **DDEV support**: Replace all `drupexec.Run("drush", "-r", path, ...)` with `drupexec.RunWithEnv(prefix, "drush", "--root=" + path, ...)`. Wire `envdetect.Detect()` into CLI commands (`RunPreflight`, `RunScan`, `RunFix`, `RunValidate`, `RunUpgradeCore`).
3. **MCP schemas**: In `handleListTools` (`internal/mcp/server.go:102-117`), replace empty `inputSchema: {"type": "object"}` with actual JSON Schema definitions for each tool's parameters.

**P1 — Important**
4. **Contrib compatibility**: In `CheckRelease` (`internal/drupalorg/drupalorg.go:95-149`), fetch and parse `core_version_requirement` from module's `.info.yml` or `composer.json`. Support `||` operators in version constraints.
5. **PHP 8.4 patch**: In `RunPreflight`, detect PHP 8.4+ via `php -v`, then append `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` to `settings.php` after the DDEV include block.

**P2 — Nice to have**
6. **Report data**: In `RunReport` (`internal/app/commands.go:166-198`), replace hardcoded `TotalErrors: 0` with actual data from `scan.ParseCodeclimateJSON()` or similar.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modified | `RunScan`, `DoValidate`, `RunPreflight`, `RunUpgradeCore`, `RunReport` — exit code handling + env detection + report data |
| `internal/app/mcp_tools.go` | Modified | `realHandleScan`, `realHandleValidate` — exit code 3 handling |
| `internal/mcp/server.go` | Modified | `handleListTools` — expose parameter schemas for 20 tools |
| `internal/drupalorg/drupalorg.go` | Modified | `CheckRelease` — parse compound version constraints |
| `internal/envdetect/envdetect.go` | Unchanged | Already correct; just needs to be called by CLI commands |
| `internal/exec/exec.go` | Unchanged | Already supports `RunWithEnv`; just needs to be called |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Exit code 3 handling confuses "findings" with "crash" | Medium | Parse stdout first; if empty + exit 3, treat as error. Log stderr for debugging. |
| DDEV path mapping breaks `--root=` | Low | Test with `ddev exec drush --root=/var/www/html/...` — DDEV maps `/var/www/html` to project root. |
| MCP schema changes break existing agents | Low | Schemas are additive (adding properties, not removing). Agents that ignored schemas will continue working. |
| Contrib constraint parsing fails on edge cases | Medium | Use semver library for parsing; fallback to string match if parsing fails. |
| PHP 8.4 patch corrupts `settings.php` | Low | Append after DDEV include block; backup `settings.php` before patching. |

## Rollback Plan

All changes are isolated to internal packages. Rollback:
1. `git revert <commit>` — reverts the entire change
2. For individual fixes: each P0/P1/P2 fix should be a separate commit for granular rollback
3. No database schema changes, no state file changes — safe to revert at any point

## Dependencies

- None — all fixes are internal to the drup binary
- Existing `envdetect` and `exec` packages already provide the infrastructure needed

## Success Criteria

- [ ] `drup scan` and `drup validate` return exit 0 when `upgrade_status:analyze` returns exit 3 with findings
- [ ] `drup scan` works under DDEV without manual `ddev exec` workaround
- [ ] MCP tools expose parameter schemas (agents can discover `module`, `path`, `url` parameters)
- [ ] `drup contrib webform` returns `has_d11_release: true` for webform 6.3.0
- [ ] `drup preflight` auto-patches `settings.php` on PHP 8.4+ projects
- [ ] `drup report` outputs real scan data, not hardcoded zeros
- [ ] All existing tests pass (`go test ./...`)
- [ ] No regressions in non-DDEV environments
