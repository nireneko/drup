# Delta Spec: drup-post-test-fixes

`upgrade_status:analyze` v4.3.x ignores `--format=json` and outputs plain text. This breaks the parser, all MCP tools, and the CLI pipeline.

## Group A — Plain-Text Scan Parser (CRITICAL)

### NEW: Plain-Text Scan Parser

| Req | Strength | Behavior |
|-----|----------|----------|
| Line-based parsing | MUST | Parse plain-text `upgrade_status:analyze` output into existing error model |
| Tolerant extraction | MUST | Regex field extraction; skip unrecognized lines |
| Project detection | MUST | `Project: <name>` lines delimit per-project blocks |
| Empty output | MUST | Return zero-error model when no project blocks found |

#### Scenario: Multi-project plain text

- GIVEN plain text with contrib + custom projects
- WHEN `scan.Parse()` runs
- THEN SHALL return classified errors with file/line/message/rule

#### Scenario: Tolerate warnings and blanks

- GIVEN `[warning]` and blank lines in output
- WHEN parsing runs
- THEN SHALL skip non-error lines, produce correct model

### MODIFIED: Scan — Drush Invocation

(Previously: `--format=json` on all drush calls)

| Req | Strength | Behavior |
|-----|----------|----------|
| Remove --format=json | MUST | All 5 call sites: no `--format=json` flag |

#### Scenario: CLI scan

- GIVEN `drup scan /path`
- THEN SHALL run `drush -r /path upgrade_status:analyze --all`

**Files**: `scan/scan.go`, `scan/testdata/*`, `app/commands.go`, `app/mcp_tools.go`

## Group B — Missing CLI Commands

### MODIFIED: CLI Binary — Dispatch

(Previously: init, scan, fix, contrib, issue, report only)

| Req | Strength | Behavior |
|-----|----------|----------|
| validate | MUST | `drup validate <path> [module]` — re-run scan, return error state |
| apply-patch | MUST | `drup apply-patch <url> <path>` — download and apply patch |
| Shared logic | SHOULD | CLI and MCP handlers reuse same functions |

#### Scenario: validate exit codes

- GIVEN clean project → exit 0, `{total_errors: 0}`
- GIVEN errors remain → exit 1, `{total_errors: N}`

#### Scenario: apply-patch conflict

- GIVEN git apply fails → exit 1 with error details

**Files**: `app/app.go`, `app/commands.go`

## Group C — Error Context

### MODIFIED: Error Wrapping (scan + mcp-server)

(Previously: `fmt.Errorf("parse scan output: %w", err)` — no command context)

| Req | Strength | Behavior |
|-----|----------|----------|
| Error helper | MUST | Wrap failures with: command, exit code, stderr, truncated stdout |
| Apply everywhere | MUST | RunScan + 4 MCP tools use the helper |

#### Scenario: Drush non-zero exit

- GIVEN drush exit 1 with stderr
- THEN error SHALL include command, exit code, stderr

#### Scenario: Parse failure

- GIVEN drush exit 0 but unparseable output
- THEN error SHALL include command and truncated stdout (500 chars)

**Files**: `app/commands.go`, `app/mcp_tools.go`

## Group D — SKILL.md Sync

### MODIFIED: Orchestrator Skill — Commands

(Previously: references non-existent CLI commands, wrong stage numbers)

| Req | Strength | Behavior |
|-----|----------|----------|
| Command accuracy | MUST | All 4 copies reference only existing CLI commands |
| Stage numbering | MUST | Sequential, matching actual CLI flow |
| Copy consistency | MUST | Root + 3 templates identical in command content |

#### Scenario: All commands exist

- GIVEN any SKILL.md → every `drup <cmd>` has matching `case` in `app.go`

**Files**: `SKILL.md`, `internal/packaging/templates/{claude,codex,opencode}/SKILL.md`

## Group E — Test Coverage

### MODIFIED: Test Verification

| Req | Strength | Behavior |
|-----|----------|----------|
| Plain-text fixtures | MUST | Replace JSON fixtures with real plain-text output |
| Parse tests | MUST | Table-driven: multi-project, single, empty, warnings |
| CLI integration | MUST | `RunScan` test with mocked plain-text drush output |
| MCP mock updates | MUST | Mocks return plain text for scan/validate/autofix/upgrade_scan |

#### Scenario: Fixture round-trip

- GIVEN plain-text fixture in `testdata/`
- WHEN `scan.Parse()` reads it
- THEN model SHALL match expected errors exactly

**Files**: `scan/scan_test.go`, `scan/testdata/*`, `app/commands_test.go`, `app/mcp_tools_test.go`
