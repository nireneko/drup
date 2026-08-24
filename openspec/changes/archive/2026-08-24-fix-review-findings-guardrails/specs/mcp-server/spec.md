# Delta for MCP Server

## MODIFIED Requirements

### Requirement: drush_exec Tool

The system SHALL expose a `drush_exec` MCP tool that safely wraps drush execution with alias-to-canonical command normalization, an extended command blocklist evaluated against the canonical name, automatic environment-aware prefixing, `--root` flag injection, and structured output parsing. The system MUST normalize the command (trim, lowercase, resolve known aliases to their canonical drush command) before blocklist evaluation, so an alias of a blocked command is blocked identically to its canonical form.

| Req | Strength | Behavior |
|-----|----------|----------|
| Canonical normalization | MUST | Resolve aliases to canonical form before blocklist check |
| Extended blocklist | MUST | Block canonical forms of: `sql-drop`, `site-install`/`site:install`, `sql-sanitize`, `php-eval`/`php:script`/`ev`/`scr`, `core:execute-cli`/`core:execute`/`exec`, `sql:query`/`sqlq`/`sql-cli`/`sqlc` |
| Metacharacter filter | MUST | Reject commands or arguments containing `;`, `|`, `&`, `$`, `` ` ``, or newline characters |

(Previously: exact-string blocklist matched only 6 literal command names with no alias resolution; metacharacter filter did not cover newlines or backticks.)

#### Scenario: drush_exec success

- GIVEN a tool call `drush_exec({project_path: "/path", command: "status", format: "json"})`
- WHEN the command executes successfully
- THEN the system SHALL return `{success: true, output: {...}, stderr: "", exit_code: 0}`

#### Scenario: drush_exec blocked command (canonical form)

- GIVEN a tool call `drush_exec({project_path: "/path", command: "sql-drop"})`
- WHEN the command is in the blocklist
- THEN the system SHALL return `{success: false, error: "command 'sql-drop' is blocked for safety", exit_code: -1}`

#### Scenario: drush_exec blocked via alias — sqlq

- GIVEN a tool call `drush_exec({project_path: "/path", command: "sqlq"})`
- WHEN the command normalizes to the canonical `sql:query`, which is blocked
- THEN the system SHALL reject the call identically to the canonical form

#### Scenario: drush_exec blocked via alias — scr / ev

- GIVEN a tool call `drush_exec({project_path: "/path", command: "scr myscript.php"})` or `drush_exec({project_path: "/path", command: "ev 'phpinfo();'"})`
- WHEN the command normalizes to the canonical `php:script`/`php-eval`, which is blocked
- THEN the system SHALL reject the call identically to the canonical form

#### Scenario: drush_exec blocked via alias — exec / core:execute

- GIVEN a tool call `drush_exec({project_path: "/path", command: "exec 'rm -rf /'"})`
- WHEN the command normalizes to the canonical `core:execute-cli`, which is blocked
- THEN the system SHALL reject the call identically to the canonical form

#### Scenario: drush_exec shell metacharacters rejected

- GIVEN a tool call `drush_exec({project_path: "/path", command: "status; rm -rf /"})`
- WHEN the command contains shell metacharacters
- THEN the system SHALL return an error before executing

#### Scenario: drush_exec rejects newline and backtick injection

- GIVEN a tool call with a command argument containing a newline or a backtick character
- WHEN the metacharacter filter runs
- THEN the system SHALL reject the call before executing, matching the semicolon/pipe/ampersand/dollar behavior

### Requirement: Drush Error Context

The system SHALL wrap all drush execution failures with structured error context including the full command string, exit code, stderr (full), and stdout (truncated to 500 chars). This helper (`drushExecError`) SHALL be used by RunScan and all MCP tool handlers that invoke drush. When appending a supplementary warning to existing stderr content, the system MUST insert a newline separator so the warning does not merge into the prior line.

(Previously: a warning string was concatenated directly onto stderr with no separator, producing a glued, unreadable line.)

#### Scenario: Drush non-zero exit

- GIVEN drush exit 1 with stderr
- THEN error SHALL include command, exit code, stderr

#### Scenario: Parse failure

- GIVEN drush exit 0 but unparseable output
- THEN error SHALL include command and truncated stdout (500 chars)

#### Scenario: Warning appended with separator

- GIVEN existing stderr content and a supplementary warning to append
- WHEN the warning is appended
- THEN the system SHALL insert a newline between the existing content and the warning, keeping both readable as separate lines

## ADDED Requirements

### Requirement: Scanner Input Buffer Limit

The system MUST configure the stdin JSON-RPC line scanner with an explicit, bounded maximum buffer size larger than the default 64 KB, and MUST NOT allow an oversized single line to terminate the server process. Lines exceeding the configured maximum MUST produce a JSON-RPC parse error for that request without stopping the read loop.

#### Scenario: Large but within-bound request

- GIVEN a single JSON-RPC request line larger than 64 KB but within the configured maximum
- WHEN the server reads it
- THEN the system SHALL parse and process it normally

#### Scenario: Oversized request does not kill the server

- GIVEN a single line exceeding the configured maximum buffer size
- WHEN the server reads it
- THEN the system SHALL return a JSON-RPC parse error for that request and SHALL continue serving subsequent requests

### Requirement: Bounded Subprocess Execution

Every subprocess invocation (composer, drush, git, rector) made on behalf of a tool call MUST run under a bounded execution deadline. When the deadline elapses, the system MUST terminate the subprocess and return an error rather than blocking indefinitely.

#### Scenario: Subprocess completes within deadline

- GIVEN a subprocess that finishes before its deadline
- WHEN the tool handler awaits it
- THEN the system SHALL return the subprocess's normal result

#### Scenario: Subprocess exceeds deadline

- GIVEN a subprocess that does not exit before its configured deadline
- WHEN the deadline elapses
- THEN the system SHALL terminate the subprocess and return a timeout error, allowing the server to continue serving subsequent requests

### Requirement: Deterministic Tool Listing

The `tools/list` response MUST list tools in a deterministic order (sorted by tool name) across repeated calls within the same server run.

#### Scenario: Repeated tools/list calls match

- GIVEN two consecutive `tools/list` calls in the same server session
- WHEN their responses are compared
- THEN the tool ordering SHALL be identical and sorted by name

### Requirement: JSON-RPC Notification Handling

The system MUST distinguish JSON-RPC notifications (requests with no `id` field) from requests, and MUST NOT write a JSON-RPC response for a notification.

#### Scenario: notifications/initialized produces no response

- GIVEN the server receives a `notifications/initialized` message with no `id`
- WHEN the server processes it
- THEN the system SHALL perform any associated internal action but SHALL NOT write any response to stdout

#### Scenario: Ordinary requests still receive responses

- GIVEN a request with a non-null `id`
- WHEN the server processes it
- THEN the system SHALL write exactly one JSON-RPC response for that `id`

### Requirement: Kill Switch and Dry-Run Partition

The system MUST refuse every mutating tool call immediately when `DRUP_DISABLE_MUTATIONS=1` is set, regardless of session state. Independently, for mutating calls without a valid `agent-session`, the guard middleware MUST partition tools into force-dry-run (tools with a native `dry_run` parameter: `core_upgrade_apply`, `contrib_compat_patch`, `contrib_allow_lenient`, `custom_compat_fix`) and refuse-only (tools with no dry-run semantics: `apply_patch`, `composer_require`, `patch_rollback`, `cleanup`, `create_patch`, `test_backup_restore`, `test_backup_delete`).

#### Scenario: Kill switch refuses regardless of session

- GIVEN `DRUP_DISABLE_MUTATIONS=1` and a valid open session
- WHEN any mutating tool is called
- THEN the system SHALL refuse the call with an error naming the kill switch

#### Scenario: Force-dry-run tool without session

- GIVEN no valid session and `DRUP_DISABLE_MUTATIONS` unset
- WHEN `core_upgrade_apply` is called without an explicit `dry_run` value
- THEN the system SHALL force `dry_run: true` and proceed in preview mode

#### Scenario: Refuse-only tool without session

- GIVEN no valid session and `DRUP_DISABLE_MUTATIONS` unset
- WHEN `apply_patch` is called
- THEN the system SHALL refuse the call with an actionable error naming the `session_open` flow, since `apply_patch` has no dry-run mode

### Requirement: session_open and pipeline_status Registration

The system SHALL register `session_open` and `pipeline_status` as new MCP tool handlers, wired via the same 3-file pattern (schema, placeholder, real handler) used by other tools, satisfying the existing wiring-symmetry invariant tests.

#### Scenario: New tools are callable and schema-complete

- GIVEN the MCP server starts with `session_open` and `pipeline_status` registered
- WHEN `tools/list` is called and then each tool is invoked
- THEN both SHALL appear with non-empty `inputSchema.properties` and SHALL route to their real handlers

#### Scenario: Existing tool count reflects the two additions

- GIVEN the server registers its full default tool set
- WHEN the total registered tool count is checked
- THEN it SHALL equal the prior total plus exactly two (`session_open`, `pipeline_status`), and documentation/tests SHALL be updated to match in the same slice
