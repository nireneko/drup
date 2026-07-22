# Proposal: drup-post-test-fixes

Fix the drup pipeline so a real Drupal 10→11 upgrade works end-to-end. A live execution revealed that `drup scan` and all MCP scan/validate tools fail silently because `upgrade_status:analyze` v4.3.x ignores `--format=json` and always outputs plain text.

## Verified Issues

| # | Issue | Root Cause | Impact |
|---|-------|-----------|--------|
| 1 | `drup scan` fails | `scan.Parse()` uses `json.Unmarshal`; drush outputs plain text | Pipeline broken at stage 2 |
| 2 | MCP scan/validate/autofix/upgrade_scan fail | Same `--format=json` + JSON parse at 4 call sites | All MCP validation broken |
| 3 | `drup validate` missing from CLI | No `case "validate"` in `app.go` dispatcher | SKILL.md references phantom command |
| 4 | `drup apply-patch` missing from CLI | No `case "apply-patch"` in `app.go` dispatcher | SKILL.md references phantom command |
| 5 | SKILL.md mismatch (4 copies) | References non-existent CLI commands, wrong stage numbers | Orchestrator sends invalid commands |
| 6 | Cryptic errors | `fmt.Errorf("parse scan output: %w", err)` — no command, exit code, or stderr | Impossible to debug failures |
| 7 | No scan test coverage | `commands_test.go` has 1 flag-check test; fixtures are JSON (unrealistic) | Bugs ship undetected |

**Correction**: The original agent report claimed a `-r --help` bug. Exploration confirmed `commands.go:61` correctly uses `-r <path>`. No action needed for that claim.

## Scope

### In Scope
- Rewrite `scan.Parse()` for plain-text `upgrade_status:analyze` output
- Remove `--format=json` from all 5 drush call sites
- Wire `validate` and `apply-patch` into CLI dispatcher
- Add error context helper (command, exit code, stderr)
- Update all 4 SKILL.md copies
- Replace JSON fixtures with plain-text fixtures; expand test coverage

### Out of Scope
- MCP tool naming (`drup_` prefix) — defer
- Global `--json` output flag — defer
- `drush php:eval` approach — over-engineering
- New pipeline stages or features

## Capabilities

### New Capabilities
- `plain-text-scan-parser`: Line-based parser for `upgrade_status:analyze` plain-text output, replacing JSON-only parsing

### Modified Capabilities
- `scan`: Parse plain text instead of JSON; remove `--format=json` from drush invocation
- `cli-binary`: Add `validate` and `apply-patch` commands to dispatcher
- `mcp-server`: Update 4 tool handlers to remove `--format=json` and use plain-text parser
- `orchestrator-skill`: Sync SKILL.md command references and stage numbering with actual CLI

## Approach

**Group A — Fix scan parser (CRITICAL)**
Rewrite `scan.Parse()` with line-based/regex parsing for the actual plain-text format. Remove `--format=json` from `commands.go:61` and `mcp_tools.go:78,122,210,629`. Replace all 3 JSON fixtures with plain-text fixtures.

**Group B — Add missing CLI commands**
Add `case "validate"` and `case "apply-patch"` to `app.go` dispatcher. Extract shared logic from `realHandleValidate` and `realHandleApplyPatch` into reusable functions in `commands.go`.

**Group C — Better error messages**
Add a `drushExecError` helper wrapping command, exit code, stderr, and truncated stdout. Apply at scan and MCP call sites.

**Group D — Sync SKILL.md**
Update root `SKILL.md` and 3 packaged templates (`internal/packaging/templates/{claude,codex,opencode}/SKILL.md`). Fix stage numbering and remove references to non-existent commands.

**Group E — Expand test coverage**
Add plain-text parsing tests in `scan_test.go`. Add `RunScan` CLI integration test. Update `mcp_tools_test.go` mocks to return plain text.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/scan/scan.go` | Modified | Rewrite Parse() for plain text |
| `internal/scan/testdata/` | Modified | Replace 3 JSON fixtures with plain text |
| `internal/app/commands.go` | Modified | Remove `--format=json`, add error context |
| `internal/app/mcp_tools.go` | Modified | Remove `--format=json` at 4 sites, add error context |
| `internal/app/app.go` | Modified | Add `validate` and `apply-patch` cases |
| `SKILL.md` + 3 templates | Modified | Sync commands and stage numbers |
| `internal/scan/scan_test.go` | Modified | Plain-text parsing tests |
| `internal/app/commands_test.go` | Modified | RunScan integration test |
| `internal/app/mcp_tools_test.go` | Modified | Plain-text mock output |

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Plain-text format varies across upgrade_status versions | Medium | Tolerant parser: regex-based field extraction, skip unknown lines |
| JSON fixtures and MCP mocks all break during transition | High | TDD: write plain-text fixtures first, then update parser |
| SKILL.md copies drift apart | Low | Update all 4 in same commit; add sync check in CI later |

## Rollback Plan

Revert the single PR. All changes are in internal packages + docs — no data migration, no config changes. The pre-change code already fails on real drush output, so rollback restores the known-broken state (no regression).

## Dependencies

- None. All fixes are self-contained within the drup codebase.

## Success Criteria

- [ ] `drup scan <path>` succeeds against a real Drupal project with upgrade_status installed
- [ ] All 4 MCP tools (scan, validate, autofix, upgrade_scan) parse plain-text output correctly
- [ ] `drup validate <path>` and `drup apply-patch <url> <path>` work as CLI commands
- [ ] Error messages include drush command, exit code, and stderr
- [ ] All 4 SKILL.md copies match actual CLI command set
- [ ] `go test ./...` passes with plain-text fixtures (zero JSON fixtures remain)
