## Exploration: drup-post-test-fixes

### Current State

The drup CLI is a hand-rolled Go binary with manual `switch args[0]` dispatch in `internal/app/app.go`. It has no third-party CLI framework. Commands that interact with `upgrade_status:analyze` share a common pattern: invoke drush, capture stdout, pass it to `scan.Parse()` which calls `json.Unmarshal`.

#### How `RunScan()` works (commands.go:60-82)

```go
drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--all", "--format=json")
```

1. Calls `drush -r <path> upgrade_status:analyze --all --format=json`
2. Passes stdout to `scan.Parse()` which does `json.Unmarshal` into `map[string]rawProject`
3. Returns structured JSON to stdout

The `-r <path>` flag is correct — it sets the Drupal root. The `--all` flag is correct. The `--format=json` flag is the problem: `upgrade_status:analyze` v4.3.x **ignores it** and always outputs human-readable plain text.

#### How MCP scan/validate tools work

Three MCP tools call `upgrade_status:analyze` with the same broken pattern:

| Tool | File:Line | Drush call |
|------|-----------|------------|
| `realHandleScan` | mcp_tools.go:78 | `drush -r <path> upgrade_status:analyze --all --format=json` |
| `realHandleValidate` | mcp_tools.go:210 | `drush -r <path> upgrade_status:analyze <target> --format=json` |
| `realHandleAutofix` | mcp_tools.go:122 | `drush -r <path> upgrade_status:analyze --all --format=json` (re-scan) |
| `realHandleUpgradeScan` | mcp_tools.go:629 | `drush upgrade_status:analyze <target> --format=json --root=<path>` |

All four fail identically: `scan.Parse()` gets plain text, `json.Unmarshal` fails.

#### What `upgrade_status:analyze` actually outputs (plain text)

```
 [warning] Some modules have deprecation errors.

 Project: token (8.x-1.13)
  ------
  - modules/contrib/token/token.module:42
    Call to deprecated function token_get_tree().
    Rule: deprecation

 Project: mymodule
  ------
  - modules/custom/mymodule/src/Service.php:15
    Call to deprecated method getDefinition().
    Rule: drupal.entity_type_manager
```

This is the format `scan.Parse()` must handle. The current `scan.Parse()` only handles JSON.

#### Current SKILL.md pipeline description

The SKILL.md (both at `~/.config/opencode/skills/drup/SKILL.md` and the packaged templates at `internal/packaging/templates/*/SKILL.md`) describes an 8-stage pipeline:

| Stage | SKILL.md Command | Actual CLI Command | Exists? |
|-------|-----------------|-------------------|---------|
| 1 | `drup preflight` | `drup preflight` | Yes |
| 2 | `drup scan <path>` | `drup scan <path>` | Yes (broken) |
| 3 | `drup fix <path>` | `drup fix <path>` | Yes |
| 4 | `drup contrib`, `drup issue`, `drup apply-patch` | `drup contrib`, `drup issue`, **NO `apply-patch`** | Partial |
| 5 | `drup upgrade-core <ver>` | `drup upgrade-core <ver>` | Yes |
| 6 | `drup scan <path>` | `drup scan <path>` | Yes (broken) |
| 7 | `drup scan <path>` | `drup scan <path>` | Yes (broken) |
| 8 | Report generation | `drup report <path>` | Yes |

Rule 4 says "Validation is delegated to `drup scan` and `drup validate`" — but `drup validate` **does not exist as a CLI command**. It only exists as MCP tool `validate`.

The orchestrator-skill spec also references `drup validate` as a CLI command for gate checks between stages.

#### Current test coverage for CLI commands

| File | Lines | What's tested |
|------|-------|---------------|
| `commands_test.go` | 833 | RunUpgrade (2), RunUninstall (4), RunUpgradeCore (10), RunScan (1 — flag only) |
| `mcp_tools_test.go` | 818 | Most MCP tools — invalid JSON, security, flag passing |
| `preflight_test.go` | exists | Not read |
| `app_test.go` | exists | Not read |

**Not tested**: `RunScan` (parsing, error paths), `RunFix`, `RunContrib`, `RunIssue`, `RunReport`, `RunInit`, `RunPreflight` (in commands_test.go).

#### Current MCP tool naming convention

Tools are registered as short names (`scan`, `validate`, `contrib_check`, etc.) in `WireMCPTools()`. The MCP server exposes them with the `drup_` prefix to agents (e.g., `drup_scan`, `drup_validate`). This creates a naming duality:

- CLI: `drup scan` (space-separated)
- MCP: `drup_scan` (underscore-separated)

#### Error handling approach

Errors are wrapped with `fmt.Errorf("context: %w", err)` but never include:
- The full command that was executed
- The exit code
- The stderr output
- The stdout that failed to parse

Example from `RunScan`:
```go
return fmt.Errorf("parse scan output: %w", err)
// → "parse scan output: parse upgrade_status JSON: unexpected end of JSON input"
```

No indication of what drush command ran, what it returned, or what its exit code was.

---

### Root Causes

#### Bug 1: `drup scan` fails with JSON parse error

**Root cause**: `scan.Parse()` (scan/scan.go:26-58) only handles JSON input via `json.Unmarshal`. But `upgrade_status:analyze` v4.3.x does NOT support `--format=json` — it always outputs plain text. The `--format=json` flag is silently ignored.

- **File**: `internal/scan/scan.go:33` — `json.Unmarshal(data, &raw)` fails on plain text
- **File**: `internal/app/commands.go:61` — passes `--format=json` which is ignored
- **Files**: `internal/app/mcp_tools.go:78,122,210,629` — same pattern in 4 MCP tools

The test fixtures (`testdata/upgrade_status_d10.json`) are JSON, so unit tests pass — but they test against a format that `upgrade_status:analyze` never produces in reality.

#### Bug 2: `drup validate` CLI command does not exist

**Root cause**: `internal/app/app.go:15-68` — the switch statement has no `case "validate"`. The validate logic exists only in `realHandleValidate` (mcp_tools.go:193-242).

#### Bug 3: `drup apply-patch` CLI command does not exist

**Root cause**: Same as above — `app.go` has no `case "apply-patch"`. The logic exists only in `realHandleApplyPatch` (mcp_tools.go:177-191).

#### Bug 4: SKILL.md references non-existent CLI commands

**Root cause**: SKILL.md (and all 3 packaged templates) reference `drup validate` and `drup apply-patch` as CLI commands, but they don't exist in the CLI dispatcher.

#### Bug 5: Cryptic error messages

**Root cause**: Error wrapping in `commands.go` and `mcp_tools.go` uses `fmt.Errorf("context: %w", err)` without including the drush command, exit code, stderr, or raw stdout.

---

### Affected Areas

- `internal/scan/scan.go` — Parse() only handles JSON, must handle plain text
- `internal/scan/scan_test.go` — Tests use JSON fixtures that don't match real output
- `internal/scan/testdata/` — All 3 fixtures are JSON format
- `internal/app/commands.go` — RunScan() drush invocation + error messages
- `internal/app/mcp_tools.go` — 4 tools with same broken drush invocation
- `internal/app/app.go` — Missing `validate` and `apply-patch` CLI commands
- `internal/app/commands_test.go` — No coverage for RunScan parsing, RunFix, RunContrib, etc.
- `internal/app/mcp_tools_test.go` — Tests mock drush output as JSON (unrealistic)
- `SKILL.md` (4 copies) — References non-existent CLI commands
- `openspec/specs/scan/spec.md` — Spec assumes `--format=json` works
- `openspec/specs/orchestrator-skill/spec.md` — References `drup validate` as CLI

---

### Approaches

#### 1. Fix scan parser to handle plain text output (RECOMMENDED)

Replace `scan.Parse()` to handle the actual plain-text output format from `upgrade_status:analyze`. Drop the `--format=json` flag from all drush invocations. Add a plain-text parser with regex/line-based parsing.

- **Pros**: Fixes the CRITICAL bug. Makes the tool actually work. Aligns code with reality.
- **Cons**: Requires capturing real `upgrade_status:analyze` output to build accurate parser. Must update all test fixtures.
- **Effort**: Medium — parser rewrite + fixture updates + 4 drush call sites
- **Files**: `scan/scan.go`, `scan/scan_test.go`, `scan/testdata/*`, `app/commands.go`, `app/mcp_tools.go`

#### 2. Use `drush php:eval` to get JSON from Drupal API

Bypass `upgrade_status:analyze` entirely. Use `drush php:eval` to call the UpgradeStatus API directly and get structured data.

- **Pros**: Full control over output format. Could be more reliable.
- **Cons**: Depends on Drupal internal API which may change. More complex. Over-engineering for the problem.
- **Effort**: High — requires Drupal API knowledge, new integration tests
- **Files**: `scan/scan.go`, `app/commands.go`, `app/mcp_tools.go`

#### 3. Add `drup validate` and `drup apply-patch` CLI commands

Wire existing MCP handlers into the CLI dispatcher. Extract shared logic into reusable functions.

- **Pros**: SKILL.md becomes accurate. Pipeline works end-to-end via CLI.
- **Cons**: Small change, but needs to be done alongside the scan fix.
- **Effort**: Low — add 2 switch cases in app.go, extract handler logic
- **Files**: `app/app.go`, `app/commands.go`

#### 4. Improve error messages to include command, exit code, stderr

Wrap all drush/composer errors with full context.

- **Pros**: Makes debugging possible. Agents can report useful errors.
- **Cons**: Touches many call sites.
- **Effort**: Low-Medium — add a helper function, update error wrapping
- **Files**: `app/commands.go`, `app/mcp_tools.go`

#### 5. Sync SKILL.md with actual commands

Update all 4 SKILL.md copies to match real CLI commands. Fix stage numbering.

- **Pros**: Documentation matches reality.
- **Cons**: Must be kept in sync going forward.
- **Effort**: Low — text edits only
- **Files**: `SKILL.md` (root), `internal/packaging/templates/{claude,codex,opencode}/SKILL.md`

---

### Recommendation

**Combine approaches 1 + 3 + 4 + 5** in a single change:

1. **Fix the scan parser** (approach 1) — this is the CRITICAL blocker. Rewrite `scan.Parse()` to handle plain text. Update all fixtures. Remove `--format=json` from all 5 drush call sites.

2. **Add missing CLI commands** (approach 3) — wire `validate` and `apply-patch` into `app.go`. Extract shared logic from MCP handlers.

3. **Improve error messages** (approach 4) — add a `drushError` helper that includes command, exit code, stderr.

4. **Sync SKILL.md** (approach 5) — update all 4 copies to match real commands.

Approach 2 (php:eval) is over-engineering — skip it.

The `--json` global flag (issue 7) and MCP naming (issue 3) are lower priority — defer to a follow-up change.

---

### Risks

- **Plain-text format may vary across upgrade_status versions** — need to capture output from v4.3.x specifically and make the parser tolerant of minor format changes.
- **Existing JSON fixtures are wrong** — all 3 testdata files must be replaced with plain-text fixtures. Tests will break during the transition (TDD RED → GREEN).
- **MCP tool tests mock JSON output** — `mcp_tools_test.go` tests that override `drupexec.Run` to return `{}` will need updating to return plain text.
- **SKILL.md has 4 copies** — must update all 4 in lockstep or they'll drift.

---

### Ready for Proposal

**Yes.** The exploration has identified:
- 5 drush call sites that pass `--format=json` (silently ignored)
- 1 parser that only handles JSON (must handle plain text)
- 2 missing CLI commands (`validate`, `apply-patch`)
- 4 SKILL.md files referencing non-existent commands
- 0 error messages including command context

The fix is well-scoped: rewrite the parser, fix the call sites, add the CLI commands, improve errors, sync docs.
