# drup MCP tools — agent reference

`drup mcp` exposes a stdio JSON-RPC MCP server for an agent-coordinated Drupal upgrade. The server is local to the host process: it does not listen on a network port. Start it with `drup mcp --locked` when an integration must inspect tools without being able to mutate a project.

Tool schemas, effects, guards, and stub visibility are defined once in `internal/mcp.ToolSpec`. The catalog below is generated; agents should obtain exact JSON schemas from `tools/list` rather than copying examples from prose. For operating context, read [Upgrade workflow](upgrade-workflow.md), [Safety and recovery](safety-and-recovery.md), and [Architecture](architecture.md).

<!-- BEGIN GENERATED MCP CATALOG -->

## MCP catalog contract (generated)

`ToolSpec` is the only source for these 45 implemented MCP contracts: schemas, effects, guards, and stub visibility. Regenerate with `go generate ./...`; CI rejects byte drift. 41 tools are available as transport stubs. Planned or obsolete tools are intentionally absent from this runtime catalog.

| Tool | Required input | Effect | Guard contract | Stub |
|---|---|---|---|---|
| `apply_patch` | `patch_url, project_path, composer_package, description, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `autofix` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `checkpoint_commit` | `project_path, run_id, commit_strategy, scope, paths, validation_hash, target, request_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `checkpoint_execute` | `project_path, run_id, phase, target_major, targets, paths, request_id` | `mutating` | session + mutation_cap + run | yes |
| `cleanup` | `project_path, validate_passed, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `composer_require` | `project_path, package, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_allow_lenient` | `project_path, packages, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_check` | `module_machine_name` | `read_only` | none | yes |
| `contrib_compat_patch` | `project_path, module_machine_name, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_plan` | `project_path, run_id` | `workflow` | none | yes |
| `contrib_upgrade_path` | `module_machine_name, current_drupal_version, target_drupal_version` | `read_only` | none | yes |
| `core_upgrade_apply` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `core_upgrade_check` | `project_path` | `read_only` | none | yes |
| `create_patch` | `module_name, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `custom_compat_fix` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `detect_env` | `project_path` | `read_only` | none | yes |
| `drupal_version_matrix` | `—` | `read_only` | none | yes |
| `drush_exec` | `project_path, command, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `generate_report` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `inventory_capture` | `project_path, run_id, stage` | `workflow` | none | yes |
| `issue_patches` | `—` | `read_only` | none | yes |
| `module_info` | `module_machine_name` | `read_only` | none | yes |
| `module_release_info` | `module_machine_name` | `read_only` | none | yes |
| `operation_reconcile` | `project_path, request_id, evidence_path, run_id` | `mutating` | run | yes |
| `patch_reconcile` | `module_machine_name, current_patch_url` | `read_only` | none | yes |
| `patch_rollback` | `project_path, patch_url, composer_package, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `patch_status` | `project_path, patch_url, composer_package` | `read_only` | none | yes |
| `pipeline_status` | `project_path` | `read_only` | none | yes |
| `prepare_upgrade_status` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `restore_check` | `project_path, backup_id` | `read_only` | none | yes |
| `restore_recover` | `project_path, journal_id, confirm, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `run_abandon` | `project_path, run_id, reason` | `workflow` | none | yes |
| `run_block` | `project_path, run_id, reason` | `workflow` | none | yes |
| `run_confirm` | `project_path, run_id, action` | `workflow` | none | yes |
| `run_create` | `project_path, target_major, commit_strategy, scope` | `workflow` | none | yes |
| `run_record` | `project_path, run_id, action, evidence` | `workflow` | none | yes |
| `run_status` | `project_path` | `workflow` | none | yes |
| `scan` | `project_path` | `read_only` | none | yes |
| `session_open` | `project_path` | `read_only` | none | yes |
| `test_backup_create` | `project_path, request_id, run_id` | `mutating` | session + mutation_cap + run | no |
| `test_backup_delete` | `project_path, backup_id, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | no |
| `test_backup_list` | `project_path` | `read_only` | none | no |
| `test_backup_restore` | `project_path, backup_id, confirm, plan_id, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | no |
| `upgrade_scan` | `project_path` | `read_only` | none | yes |
| `validate` | `project_path` | `read_only` | none | yes |

**Side-effect assertions:** `read_only` changes no project or workflow state; `workflow` changes only persisted run authority; `mutating` requires the listed guard evidence before its handler runs. The manual tool dictionary below is explanatory and must not weaken this contract.

<!-- END GENERATED MCP CATALOG -->

## Updating the catalog

1. Define or change the tool once in `internal/mcp.ToolSpec`, including its schema, effect, guard policy, and stub visibility.
2. Keep the owning stub and production handler, then run its focused behavior tests. A descriptor without either handler fails fast during wiring.
3. Run `go generate ./...`, inspect `internal/mcp/tool-catalog.generated.json` and these generated tables, then run `go test ./internal/mcp ./internal/packaging`.
4. CI can run `go run ./cmd/mcp-catalog-gen --check` to compare fresh bytes without writing. Do not hand-edit content between the generated markers; explain behavior and caveats only in the manual tool dictionary.

## 1. Core Directives

1. **Pick the MCP tool, never shell out.** Every operation in the upgrade pipeline has a deterministic MCP tool. If you find yourself about to run drush, composer, git-apply, curl, or any patch operation via Bash — STOP and pick a tool. The blocker isn't a guideline; the tools do things Bash cannot (env auto-prefix, dry-run pre-check, drush blocklist, git checkpoint).
2. **Use exact registry names.** Tools are registered as short names (`scan`, `validate`, `core_upgrade_apply`). Depending on host orchestrator they may appear prefixed (`drup_scan`, `drup_validate`). Call your named tool by whatever name it shows in your function-calling UI — never hardcode a prefix.
3. **Read the response, do not pattern-match its shape.** Every tool returns the same uniform envelope (`{status, summary, payload, operation_state?}` — see [§4.1](#11-response-envelope-uniform-contract)). The payload field is the tool-specific response shape; read it for `success`, `total_errors`, `is_applied`, etc.
4. **Errors are returned two ways.** (a) As a uniform envelope with `"status": "fail"` or `"status": "unknown"` and `"summary": "<error message>"` in the **result** channel — the tool failed or its mutating outcome cannot be proven. `unknown` is a hard stop: do not retry the mutation; reconcile observable evidence first. (b) As a JSON-RPC error response (`"code": -32601`, `-32602`, `-32603`) — this is a **protocol-level** failure (malformed request, unknown tool name, marshal failure), NOT a tool failure. Always inspect `status` first; treat JSON-RPC errors as dispatch failures and stop.
5. **Backup before mutations.** Any tool that mutates `composer.json`, the working tree, or drupal.org state (apply_patch, core_upgrade_apply, patch_rollback, composer_require without dry-run, create_patch) requires a `test_backup_create` recorded in run state. The orchestrator enforces this; sub-agents must read it before dispatching mutators.
6. **Give every mutator a stable `request_id`.** The server persists that identity before the effect. Repeating a completed ID returns its stored response without executing again; reusing it for a different operation is refused.
7. **Use a persisted run for every project mutation.** Call `run_create`, then advance only via the action returned by `run_status`. A mutator needs that `run_id`, and Go refuses a missing, foreign, blocked, or out-of-phase run before its handler executes.
8. **`tools/list` advertises only what is in `s.tools`.** `internal/mcp.ToolSpec` is the canonical catalog for each name, schema, effect class, timeout, role, preconditions, and stub visibility. Stubs and production wiring are derived from it; a missing handler fails fast during wiring.

### 1.1 Response Envelope (uniform contract)

Every MCP tool response (success OR tool-level error) is wrapped at the server level (`internal/mcp/server.go:handleToolCall`) in the same envelope:

```json
{
  "status": "pass | fail | unknown",
  "summary": "one-line human-readable summary",
  "payload": { /* original tool-specific response, only on pass */ },
  "operation_state": "unknown" /* present only for an ambiguous mutation */
}
```

- **`status: "pass"`** — the handler returned without error. `payload` contains the tool's original response shape.
- **`status: "fail"`** — the handler returned an error. `summary` is the error message; `payload` is empty. **The error is sent in the `result` channel, NOT as a JSON-RPC error.** This is a deliberate protocol extension so the orchestrator model always gets a parseable signal.
- **`status: "unknown"`** — a mutating handler timed out or was cancelled after its durable intent was recorded. The effect may have happened. Do not retry with this or a new equivalent request ID; use `operation_reconcile` after observing evidence below the project root.
- **JSON-RPC errors** are reserved for **protocol-level** failures (malformed request, unknown tool name, marshal failure). They are not tool failures; the tool never ran.

Sub-agents MUST read the tool-specific response from `payload`, not from the result directly. This is enforced by the grep test `TestSubAgentTemplates_ContainPayloadReference` over the 18 sub-agent templates.

Only descriptor-marked read-only tools retry transient errors, up to 2 times with 1s base exponential backoff. Mutators never auto-retry; their request IDs and durable outcomes control recovery. Retries are recorded via `metrics.Default().RecordRetry()`.

---

## 2. Decision Matrix — Pick the Tool for the Intent

| Intent | Use this tool | Not this |
|---|---|---|
| Find out which Drupal/PHP versions are compatible | `drupal_version_matrix` | curl + grep |
| Detect dev environment (ddev, lando, docker, direct) | `detect_env` | shell markers / `which drush` |
| Project-wide deprecation scan, with all modules | `upgrade_scan` (atomic install→enable→analyze) | `scan` (assumes upgrade_status already installed) |
| Re-scan after a fix to count remaining errors | `scan` or `validate` | `upgrade_scan` (skips fresh install) |
| Validate just one module or one file | `validate` (with `module` or `file`) | `scan` (no filter) |
| "Will this contrib work on the target Drupal major?" | `contrib_check` | `contrib_upgrade_path` (different question) |
| "Which exact version should I **install** for the target major?" | `contrib_upgrade_path` | `contrib_check` (only reports compatibility branch availability) |
| Build safe contrib update batches before touching Composer | `contrib_plan` | asking an agent to infer dependency order |
| Find a patch for an issue or module | `issue_patches` | curl + grep d.o HTML |
| Download and apply a .patch + register in composer.json | `apply_patch` | curl + git apply manually |
| Reverse a failed patch cleanly | `patch_rollback` | git revert + manual composer edit |
| Was this patch already applied? Is it obsolete? | `patch_status`, then `patch_reconcile` | grep git log manually |
| Generate a patch from rector fixes for a contrib module | `create_patch` (writes to /tmp, does NOT auto-apply) | `autofix` (which is for **custom** code only) |
| Run rector on **/modules/custom** and **/themes** | `autofix` | `create_patch` (different purpose) |
| Bump Drupal core, with rollback checkpoint | `core_upgrade_apply` (`dry_run` first) | `composer_require drupal/core*` (bypasses checkpoint) |
| What's the next core major and a composer.json preview? | `core_upgrade_check` (read-only) | `core_upgrade_apply` (mutates) |
| Generate JSON + Markdown upgrade report | `generate_report` | manual report writing |
| Run a resumable operational checkpoint after a scoped change | `checkpoint_execute`, then independent validator evidence and `checkpoint_commit` | shelling out to Composer/Drush in a prompt |
| After final validation, uninstall dev modules and revert temp patches | `cleanup` (with `validate_passed:true`) | shell `drush pm:uninstall` + `git revert` (no env awareness) |
| Declare the target Drupal major in your own modules and themes | `custom_compat_fix` | hand-editing every `.info.yml`, or missing them entirely |

---

## 3. Standard Pipelines / Sequencing

### 3.1 Target first-time upgrade pipeline

The authoritative process is documented in [`workflow.md`](workflow.md). The compact
tool sequence below preserves its ordering; it is intentionally not a shortcut around the
phase gates.

```
1. git clean check, record current branch/commit, create upgrade branch
2. detect_env, verify PHP/core/Composer/Drush/database access
3. decide whether an upgrade is needed and select the immediate next major
4. test_backup_create (database + selected filesystem snapshot)
5. install Drush if missing, then install/enable upgrade_status and Rector tooling
6. upgrade_scan (baseline findings and inventory)
7. custom_compat_fix dry_run → apply → checkpoint_execute → independent validation evidence → checkpoint_commit
8. autofix existing custom modules/themes only → manual fixes → checkpoint_execute → independent validation evidence → checkpoint_commit
9. contrib patch-level phase: batch scoped targets in checkpoint_execute → independent validation evidence → checkpoint_commit
10. contrib minor-level phase: batch scoped targets in checkpoint_execute → independent validation evidence → checkpoint_commit
11. contrib major-level phase: exactly one package target in checkpoint_execute → independent validation evidence → checkpoint_commit
12. core_upgrade_check for the immediate next major
13. core_upgrade_apply dry_run → user gate → apply → checkpoint_execute → independent validation evidence → checkpoint_commit
14. repeat steps 9–13 for every remaining major; never skip a major
15. final upgrade_status validation, tests, cache rebuild, and status checks
16. remove temporary tooling when explicitly configured → validate → config export → checkpoint_commit
17. generate_report with exact versions, patches, commits, backups, and pending work
18. retain the backup; restore only after explicit user confirmation; never delete automatically
```

### 3.2 Patch workflow per contrib module

```
patch_status     →  if NOT applied:
  issue_patches
  apply_patch     →  returns changed_files; validate the candidate before checkpoint_commit
patch_reconcile  →  is_still_needed?
if false: patch_rollback → composer update
if true:  apply_patch with newer URL
```

### 3.3 Validation gate — every time something claims success

```
sub-agent dispatcher: "I fixed X"
                      ↓
       drup-validator runs scan / validate / upgrade_scan
                      ↓
       if evidence.total_errors != 0 → re-dispatch with prior_evidence
       if evidence.total_errors == 0 → allowed to commit
```

**Validator is the only agent that calls scan-family tools.** If you are a fixer agent calling `validate` to confirm your own work, you have violated the rule.

---

## 4. Anti-Patterns — DO NOT

These are mistakes observed in real agent runs. Each row links to the tool section that handles the safe path.

| # | Mistake | Replace with |
|---|---|---|
| 1 | Calling `create_patch` and assuming the patch was applied to disk | `create_patch` only writes to `/tmp`; follow with `apply_patch` |
| 2 | Manually prefixing `ddev ` / `lando ` to bare `composer` / `drush` in shell | `composer_require`, `drush_exec` — they handle `detect_env` internally |
| 3 | Using `contrib_check` to pick an upgrade version | `contrib_check` answers "has D11 branch?"; use `contrib_upgrade_path` for "what to install" |
| 4 | HTML-scraping Drupal.org issue queues via curl | `issue_patches` and `patch_reconcile` use the JSON `api-d7` endpoint |
| 5 | Running `core_upgrade_apply` or `patch_rollback` on a dirty working tree | Both require a clean tree; `git status --porcelain` must be empty first |
| 6 | Downloading a patch with `curl` and running `git apply` by hand | Bypasses composer registration. Use `apply_patch` |
| 7 | Retrying `apply_patch` blindly on conflict | Use `patch_rollback` first to clear state, then retry with a corrected URL |
| 8 | Running `autofix` on `modules/contrib` | `autofix` is rector on `/modules/custom` and `/themes` only. For contrib, use `create_patch` |
| 9 | Passing relative paths or `.` for `project_path` | All tool args require **absolute paths**. Paths containing `..` are rejected |
| 10 | Using `composer_require drupal/core*` to bump core | Use `core_upgrade_apply` — it manages the git checkpoint and `composer.json` atomically |
| 11 | Assuming `upgrade_scan` fixes anything | `upgrade_scan` only installs/enables `upgrade_status` and returns the analysis JSON; it never mutates code |
| 12 | Calling blocked drush commands via shell because `drush_exec` refused | `sql-drop`, `site-install`, `sql-sanitize`, `php-eval`, `core:execute-cli` are blocked for safety. Ask the human |
| 13 | Skipping `detect_env` and passing `command_prefix` to other tools | `detect_env` is cached per process; the wrapper tools call it themselves. Do not pre-compute prefixes |
| 14 | Calling `issue_patches` with both fields filled | Use `issue_nid` for a single issue, or `module_name` for a search. Pick one |
| 15 | Using `validate` with no `module` filter when you want a module view | It will run `--all`; pass `module: "mymodule"` to scope it |
| 16 | Calling `cleanup` with `validate_passed:false` to "clean anyway" | The tool refuses when the validator gate has not passed — this is intentional, to preserve debugging state on failed runs |
| 17 | Dispatching an MCP tool that is in code but not in `defaultTools()` or `toolRegistry` | Such tools have empty `inputSchema` for clients; fix the wiring first (or remove the registration) |

---

## 5. Tool Dictionary

Every tool below documents: **Purpose · Returns · Prerequisites · Side-effects · Error signals · Red flags.** Schemas are intentionally omitted — get them from `tools/list` at runtime.

### 5.1 `scan`

- **Purpose**: Run `drush upgrade_status:analyze` and classify errors per module. Re-scan after fixes.
- **Returns**: `{ project_path, total_errors, modules: [{name, type, errors: [{file, line, message, rule, severity, source}]}] }`
- **Prerequisites**: `upgrade_status` module must already be installed and enabled. If unsure, use `upgrade_scan` instead.
- **Side-effects**: none.
- **Error signals**:
  - `exit_code: 3` with empty stdout means drush crashed, **not** that you have findings — re-run after fixing drush first.
  - Parse failure with `--all` argument included in drush args (no `--format=json`; plain text is parsed backend-side).
- **Red flag**: calling `scan` when `upgrade_status` is not installed — use `upgrade_scan`.

### 5.2 `upgrade_scan`

- **Purpose**: One-shot analysis when `upgrade_status` may NOT be installed: automatically installs via `composer require`, deletes conflicting `update.settings`, enables the module, then runs the analyze command.
- **Returns**: `{ total_errors, modules: [...], upgrade_status_installed, upgrade_status_enabled }`
- **Prerequisites**: absolute `project_path` without `..`.
- **Side-effects**: may install `drupal/upgrade_status --dev` and write a config delete; an existing version is unaffected.
- **Error signals**: failure stops at the first failing sub-step (install / enable / analyze) and reports exactly which one — read carefully.
- **Red flag**: calling it twice in a row without changes — it re-walks composer. Use `scan` for re-checks.

### 5.3 `validate`

- **Purpose**: Re-run analysis with optional scope/module/file filtering.
- **Returns**: `{ total_errors: <int>, errors: [<scan.DepError>…] }`
- **Prerequisites**: same as `scan`.
- **Side-effects**: none; pure read.
- **Filter precedence**: `module` ⇒ analyze only that module. `file` ⇒ post-filter errors by filename substring. `scope` ⇒ one of `env | contrib | custom | theme | global | rector`.
- **Red flag**: omitting `module` when you wanted a per-module view — without it, the tool runs `--all`.

### 5.4 `contrib_check`

- **Purpose**: Compatibility check — does this module have a Drupal 11 release branch on drupal.org?
- **Returns**: `{ module, has_d11_release: <bool>, latest_version, compatible_branches: [<str>…] }`
- **Side-effects**: none.
- **Error signals**: module not found → empty `latest_version` + empty `compatible_branches` (NOT a JSON-RPC error).

### 5.5 `contrib_upgrade_path`

- **Purpose**: Answer the practical question: "Which version of X should I install to move from D{current} to D{target}?" Uses full release-history XML parsing, prefers latest stable.
- **Returns**: `{ module, current_version, recommended_upgrade: {version, drupal_compatibility, release_date, is_stable}, alternative_versions: […], upgrade_notes }`
- **Side-effects**: none.
- **Red flag**: using it in place of `contrib_check` — they answer different questions.

### 5.6 `module_info`

- **Purpose**: Module metadata: maintainers, release dates, downloads, open issues, dependencies.
- **Returns**: `{ module, title, description, maintainers, project_url, downloads, last_release, open_issues, dependencies: {required, optional} }`
- **Side-effects**: none.
- **Error signals**: invalid `module_machine_name` (uppercase, starts with digit) — returns error WITHOUT calling Drupal.org.

### 5.7 `drupal_version_matrix`

- **Purpose**: Static compatibility table for the supported Drupal majors → min PHP, recommended PHP, supported-until, next major. The current table must evolve as new Drupal majors are released.
- **Returns**: `{ drupal_version, php_requirements: {minimum, recommended}, supported_until, upgrade_path: {next_major, migration_guide_url}, known_issues }`
- **Side-effects**: none (no network).
- **NOTE**: PHP and Drupal major selection use numeric semver comparison. Treat this static matrix as advisory until its matching target-major catalog is available.

### 5.8 `issue_patches`

- **Purpose**: Search Drupal.org issues for patches (RTBC prioritized). Use either `module_name` or `issue_nid`.
- **Returns**: `[{ url, status (RTBC|draft|...), date, is_patch, issue_nid }]`
- **Side-effects**: none.
- **Red flag**: passing both `module_name` AND `issue_nid`. Pick one — `issue_nid` wins.

### 5.9 `apply_patch`

- **Purpose**: Download a `.patch` from drupal.org with the URL allowlist enforced on the initial request and every redirect, cap the body at 10 MiB, preflight it with `git apply --check`, then register it under `composer.json → extra.patches`.
- **Returns**: `{ applied: <bool>, changed_files: [<path>…], patch_evidence: { sha256, size, initial_url, final_url, recorded_at }, error: <str> }`; it never commits. Successful provenance is also retained at `composer.json → extra.drup.patch_evidence`.
- **Prerequisites**: clean working tree; absolute `project_path`; `patch_url` from `*.drupal.org`.
- **Side-effects**: mutates the working tree and `composer.json`; it never stages or commits. If registration fails, it reverts the applied patch.
- **Error signals**: each failure mode (`URL/redirect not allowed`, oversized body, unsafe patch path/type, `git apply --check` conflict, `composer.json` malformed) has a distinct message. Download failures remove their temporary file.
- **Red flag**: retrying on conflict without first running `patch_rollback`.

### 5.10 `patch_status`

- **Purpose**: Was a patch (by URL or by composer_package) already applied? Reads `composer.json`; legacy records may also expose a commit hash.
- **Returns**: `{ is_applied, commit_hash, registered_in_composer, patch_info: {url, package, description}|null }`; new commit-free patch mutations may have an empty `commit_hash` until their run-bound checkpoint is recorded.
- **Side-effects**: none.
- **NOTE**: `is_applied=false` may simply mean rejected/closed — see `patch_reconcile` for upstream status.

### 5.11 `patch_rollback`

- **Purpose**: Revert an applied patch atomically: `git revert` first, then remove the composer.json entry.
- **Returns**: `{ success, reverted_commit, removed_from_composer, error }`
- **Prerequisites**: clean working tree, valid git repo, the patch is currently applied, you have its URL + composer_package.
- **Side-effects**: creates a git revert commit; modifies `composer.json`.
- **Failure modes** (distinct errors):
  - `not a git repository`
  - `working tree is dirty; commit or stash changes first`
  - `patch is not applied`
  - `cannot find patch commit to revert`
  - `revert conflict: <git stderr>`
- **Red flag**: running it on a project without git — it will refuse cleanly.

### 5.12 `patch_reconcile`

- **Purpose**: Analysis-only: is the currently-applied patch still needed? Are newer RTBC patches available upstream? Uses JSON `api-d7` (no HTML scraping).
- **Returns**: `{ newer_patches: [...], is_still_needed: <bool>, recommendation: <str> }`
- **Side-effects**: none.
- **Red flag**: thinking this reverts anything — it never mutates.

### 5.13 `create_patch`

- **Purpose**: Run rector on a specific contrib module, generate a `git diff`, write patch to `/tmp/drup-<module>-*.patch`. **It does NOT apply.**
- **Returns**: `{ patch_path, applied: <bool> }`
- **Prerequisites**: absolute `project_path`, `module_name` exists at `<web-root>/modules/contrib/<module_name>`. Web root comes from `composer.json → extra.drupal-scaffold.locations.web-root`.
- **Side-effects**: runs rector on the module dir; writes a temp `.patch` file.
- **Red flag**: assuming `applied:true` means the patch is in composer.json. It only writes the file — you must follow with `apply_patch`.

### 5.14 `autofix`

- **Purpose**: Run `drupal-rector process` on `/modules/custom` and `/themes` only. It does not re-scan; an independent validator owns the next read-only check.
- **Returns**: `{ rector_summary }`
- **Prerequisites**: project must contain at least one of `modules/custom` or `themes`. `drupal-rector` must be installed in `vendor/bin/`.
- **Side-effects**: **mutates files** in `modules/custom` and `themes`. Requires a backup.
- **Red flag**: running it on the whole project — rector on contrib without a patch to capture changes is a maintenance hazard.

### 5.15 `composer_require`

- **Purpose**: Safe `composer require` with regex package-name validation, **`--dry-run` pre-check**, dev/no-update flag support, version parsing from output.
- **Returns**: `{ success, installed_version, stdout, stderr, exit_code }`
- **Prerequisites**: `composer.json` in `project_path`. `detect_env` resolves the prefix; empty prefix = direct.
- **Side-effects**: on success, modifies `composer.json` + `composer.lock` and downloads the package.
- **Error signals**: distinct messages for invalid package regex, blocked shell metacharacters, missing composer.json, dry-run failure (conflict), exec error.
- **Red flag**: not specifying `package` — required field, returns error.

### 5.16 `drush_exec`

- **Purpose**: Generic drush runner with environment prefix, blocklist, and shell-metacharacter guard.
- **Returns**: `{ success, output, stderr, exit_code }`
- **Prerequisites**: absolute `project_path`. `command` (string), `args` (array of strings), optional `format` (`json|table|...`).
- **Side-effects**: runs whatever drush does — **including mutating drupal state**.
- **Hard blocklist** (rejected with `command '<x>' is blocked for safety`):
  - `sql-drop`
  - `site-install`
  - `site:install`
  - `sql-sanitize`
  - `php-eval`
  - `core:execute-cli`
- **Metacharacter guard**: rejects `;`, `|`, `&`, `$`, backtick in `command` and every `args` entry.
- **NOTE**: when `format=="json"` and the drush output is valid JSON, `output` is parsed; otherwise a warning is appended to `stderr`.
- **Red flag**: trying to bypass the blocklist by running shell directly. Blocked commands are off-limits for autonomous agents — escalate.

### 5.17 `detect_env`

- **Purpose**: Detect the dev environment and cache the result per `project_path`. Returns the command prefix that all other env-aware tools use.
- **Returns**: `{ environment: "ddev"|"lando"|"docker4drupal"|"direct"|"unknown", command_prefix: [<str>...], detected_at }`
- **Algorithm**: `.ddev/` → `ddev` · `.lando.yml` → `lando` · `docker-compose.yml` mentioning drupal → `docker4drupal` · `composer.json` only → `direct` · otherwise → `unknown`.
- **Side-effects**: in-memory cache only. Cache invalidates if `force_detect:true` OR project directory mtime changes.
- **Red flag**: calling it on every tool call. It is cached — call once at the top of the pipeline.

### 5.18 `core_upgrade_check`

- **Purpose**: Read-only next-Drupal-major lookup. An optional `target_major` is accepted only when an exact compatibility catalog exists for every required immediate step. Returns preview of the composer.json diff. **Never mutates anything.**
- **Returns**: `{ current_version, next_version, composer_patch_preview, supported: <bool> }`
- **Side-effects**: none.
- **Red flag**: expecting it to apply the upgrade — use `core_upgrade_apply` for that.

### 5.19 `core_upgrade_apply`

- **Purpose**: Apply one validated immediate step to `target_major`; `target_version` remains a compatibility alias. With `dry_run=true`, returns the diff preview only. With `dry_run=false`, refuses on dirty tree, records the existing rollback revision, then mutates `composer.json`.
- **Returns**: `{ success, report: <diff>, rollback_checkpoint: <sha>, stderr }`
- **Prerequisites**: clean working tree, git repo, absolute `project_path`, and exact metadata for the requested major jump. Missing metadata fails closed; a `10-to-11` catalog is never reused for `11-to-12`.
- **Side-effects**: mutates the working tree only. Use `checkpoint_commit` after independent validation to publish the resulting diff.
- **Rollback**: `git reset --hard <rollback_checkpoint>` reverts everything cleanly.
- **Error signals**: distinct messages for dirty tree, invalid `target_version`, path containing `..`.
- **Red flag**: requesting a real (non-dry-run) apply without `test_backup_create` recorded in run state.

### 5.20 `inventory_capture`

- **Purpose**: Read-only capture of a versioned inventory (`baseline` or `final`) into a persisted run. It records exact core/PHP/package/extension versions, patches, config, and tests without subprocesses or project writes.
- **Inputs**: `project_path`, `run_id`, `stage` (`baseline` or `final`).
- **Safety**: refuses unknown stages and incomplete captures. Reports with `run_id` use only these persisted snapshots and fail closed when either snapshot or typed evidence is absent.

### 5.20 `contrib_plan`

- **Purpose**: Read `composer.lock`, query Composer's read-only direct-outdated and prohibitions views, and persist a deterministic contrib ledger for the active run.
- **Returns**: `{ success, project_path, run_id, plan: {items, groups} }`; blockers include their causal root and the conflicting constraint.
- **Safety**: never changes `composer.json` or `composer.lock`. Patch/minor groups may batch targets; every major group has exactly one target. Invalid JSON, an unavailable lock, or an unresolved Composer query fails closed before replacing the prior plan.
- **Red flag**: treating a Drupal.org release recommendation as installable without this plan — root and transitive Composer constraints can still forbid it.

### 5.21 `generate_report`

- **Purpose**: Wrap `internal/report` to write deterministic JSON/Markdown from a persisted run snapshot when `run_id` is supplied; legacy no-run calls retain live report behavior.
- **Returns**: `{ success, json_report_path, markdown_report_path, summary: {total_modules_checked, patches_applied, custom_files_fixed, errors_remaining, pending_human_review} }`
- **Side-effects**: writes files in `project_path` root.
- **Red flag**: assuming it's read-only — it creates artifacts.

### 5.22 `cleanup`

- **Purpose**: Post-pipeline cleanup. Uninstalls dev modules (`upgrade_status`, `drupal-rector`) and reverts any temporary patches created during rector. Only runs when `validate_passed=true`.
- **Returns**: `{ success, uninstalled: [<module>…], reverted_patches: [<url>…], stderr }`
- **Prerequisites**: absolute `project_path`; `validate_passed: true` (otherwise the tool refuses to run, intentionally — to preserve debugging state on failed validation).
- **Side-effects**: modifies the Drupal site (uninstalls modules) and the working tree (reverts temp patches).
- **Red flag**: calling it on a failing pipeline to "clean anyway" — it is designed to skip on failure. If you want to clean after a failed run, restore from `test_backup` instead.

### 5.23 `checkpoint_commit`

- **Purpose**: Publish exactly the current candidate previously approved by independent validation evidence.
- **Inputs**: `project_path`, `run_id`, `commit_strategy`, `scope`, complete `paths`, `validation_hash`, and `target`; optional `commit_message`.
- **Returns**: `{ success, skipped, commit_strategy, commit_hash?, candidate_hash?, changed_files }`.
- **Safety**: `none` always skips; `single` is accepted only at final report; `per-fix` requires matching validation evidence. The live path set and candidate hash must exactly match that evidence, otherwise the request is rejected as stale.
- **Red flag**: staging or committing from `apply_patch`, `core_upgrade_apply`, or `cleanup` bypasses this evidence binding.

> The wiring invariant above is enforced by MCP catalog tests. Every tool must remain present in `defaultTools()`, `toolRegistry`, and `WireMCPTools`.

### 5.24 `checkpoint_execute`

- **Purpose**: Persist and execute one operational checkpoint with explicit argv-only adapters: fresh backup, `drush updatedb`, cache rebuild, status, independent validation, and `drush config:export` when `config/sync` is managed. Composer package update is included only for contrib patch/minor/major phases; custom/theme, core-loop, and cleanup never repeat it.
- **Inputs**: `project_path`, `run_id`, active `phase`, exact `target_major`, scoped `targets`, and complete candidate `paths`.
- **Safety**: phase and target identity must match the durable run; `contrib_major` accepts exactly one target while patch/minor targets may be batched. Every step persists sanitized hashes, exit code, duration, and paths. A failed step is terminal. An unavailable step may be explicitly retried with `resume:true` and a fresh `request_id`; the executor never blindly resumes either state.
- **Backup and validation**: the backup ID is bound to this plan rather than inferred from the newest manifest. Validation is re-run and candidate identity is recomputed from Git after it succeeds; no fixer-provided success assertion is accepted.
- **Publication**: this tool never commits. Pass independently recorded validation evidence and the recomputed candidate to `checkpoint_commit` as the only publication boundary.

### 5.25 `custom_compat_fix`

- **Purpose**: Declares support for the target Drupal major in the project's own modules, themes and profiles by widening `core_version_requirement`. These declarations are what `preflight` reports as `core_module_compat` blockers, and no other stage rewrites them.
- **Returns**: `{ project_path, target_version, dry_run, updated, already_compatible, needs_attention, changes: [{ name, file, before, after, changed, note }] }`
- **Prerequisites**: absolute `project_path`. `target_version` defaults to `11`; `dry_run` reports the rewrites without writing them.
- **Side-effects**: rewrites `.info.yml` files under `modules/custom`, `themes/custom` and `profiles/custom`. The existing constraint is kept and the target major appended, so an extension does not silently lose the versions it already declared. The file's quoting style is preserved.
- **Never touches contrib**: composer owns `modules/contrib`, so an in-place edit there is discarded on the next `composer install`. Use `create_patch` and `apply_patch` for contrib.
- **Red flag**: an extension reported under `needs_attention` has no `core_version_requirement` at all — it still uses the removed `core:` key, and where to insert the replacement is a judgement call left to a human.

### 5.23 `module_release_info`

- **Purpose**: Curated release list and maintenance status for a contrib module — combines project-level `maintenance_status` with per-release derived fields (`insecure`, `security_covered`) from the release-history feed. Complements `contrib_check` (has-D11-branch?) and `contrib_upgrade_path` (which version to install?) with the full curated picture.
- **Returns**: `{ status, module, found, maintenance_status, core_version_filter, message, suggestion, releases: [{version, tag, core_compatibility, release_type: [...], insecure, security_covered, date}, ...] }`
- **Prerequisites**: none beyond `module_machine_name`. `core_version` is optional (e.g. `"11"`).
- **Side-effects**: none.
- **Always-on gate**: `releases[]` is always restricted to `status == "published"` — retracted/unpublished releases never appear, whether or not `core_version` is supplied. `core_version` only narrows further.
- **Error signals**: unknown module → `status: "not_found"` (NOT a JSON-RPC error, same convention as `contrib_check`); invalid `module_machine_name` or an unparseable `core_version` → JSON-RPC error before the Drupal.org call.
- **Red flag**: assuming an empty `releases[]` always means "unknown module" — check `status`; `no_releases_found` means the project exists but has nothing published (or nothing matching the filter).

### 5.24 `session_open`

- **Purpose**: Resolve `project_path` to its canonical Drupal project root (symlinks followed, no `..`, marker-checked) and bind a session to it for the rest of the server process's lifetime. Call this once before any guarded mutating tool.
- **Returns**: `{ session_active: true, root: <resolved absolute path> }`
- **Prerequisites**: `project_path` required; must resolve to a valid Drupal project root.
- **Side-effects**: replaces any session bound earlier in this same server process — reopening rebinds, it does not stack.
- **Error signals**: symlink resolution or marker-check failure returns an error before any session is bound.
- **Red flag**: calling a guarded mutating tool (`apply_patch`, `core_upgrade_apply`, `composer_require`, `create_patch`, `cleanup`, `patch_rollback`, `custom_compat_fix`, `contrib_compat_patch`, `contrib_allow_lenient`, `test_backup_restore`, `test_backup_delete`, `restore_recover`, or `upgrade_scan`'s nested install) without an open, matching session — it is refused or forced into dry-run depending on the tool's guard partition.

### 5.25 `pipeline_status`

- **Purpose**: Read-only summary of the project's mutation audit ledger: per-tool call counts, total mutations, and remaining mutation-cap headroom for whichever window currently applies (per-session if a matching session is bound, otherwise per-day).
- **Returns**: `{ per_tool_counts: {<tool>: <int>, ...}, total_mutations: <int>, remaining_cap: <int> }`
- **Prerequisites**: `project_path` required.
- **Side-effects**: none — reads the audit ledger, never writes.
- **Error signals**: a project with no ledger yet still returns zero counts and the full cap, never an error.
- **Red flag**: assuming `remaining_cap` reflects a global cap — it is scoped to a session window when one is bound, otherwise to the current day.

### 5.26 `operation_reconcile`

- **Purpose**: Resolve a mutation with durable `unknown` outcome using an observable file already present below `project_path`.
- **Inputs**: `project_path`, an active matching `run_id`, the original mutation's `request_id`, and a project-relative `evidence_path` to an existing regular file. A client-supplied success boolean is never accepted as evidence.
- **Returns**: `{ success, request_id, operation_state: "completed", evidence }` when the unknown operation is reconciled.
- **Side-effects**: records the observed evidence and confirmed response in `.drup/operations.v1.json`; it does not rerun the original mutation.
- **Red flag**: guessing that a timeout failed. A timeout or cancellation can occur after the effect; inspect a real artifact first, then reconcile it. If evidence is absent, preserve `unknown` and stop.

### 5.27 Run-state workflow tools

- **`run_create`** creates one active run for the canonical root at `git_safety`; it rejects a second active or blocked run for that root.
- **`run_status`** returns the persisted phase, `allowed_actions`, safe evidence hashes/summaries, and pending-human recovery state. It is the only source of truth after restart.
- **`run_record`** accepts one currently allowed checkpoint action plus `{kind, summary, payload?}` evidence. The raw payload is never persisted; only its hash and a sanitized summary are retained.
- **`run_confirm`** records the explicit `core_upgrade` or `restore` confirmation required by the corresponding guarded mutation.
- **`run_block`** persists a safe reason and recovery target, leaving only `resolve_block` as the next action; **`run_abandon`** finishes the run without deleting evidence or backups.
- **Red flag**: advancing from agent prose or stdout. If `run_status` does not advertise an action, the tool must not be called.

---

## 6. Backup Tools (additional)
> **Policy enforcement**: the reverse asymmetry described above (backup tools are intentionally NOT in `defaultTools()`) is enforced by `TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric` in `internal/mcp/mcp_test.go`. Adding a new tool that is not in `defaultTools()` without the `test_backup_` prefix fails that test. Adding a new backup-style tool fails it too if you forget to also register the schema in `toolRegistry` or skip the `project_path` requirement. Update both the code and this section in lockstep when changing the policy.

Not part of the 28 categorized tools in §5, but mandatory in the pipeline.

### `test_backup_create`, `test_backup_list`, `test_backup_restore`, `test_backup_delete`, `restore_recover`

- All four accept `project_path`. Restore also needs `backup_id`, `plan_id` from `restore_check`, and `confirm: true`. Delete needs `backup_id`.
- Restore refuses without `confirm:true` — this is a second-line defense against accidental deletion.
- Restore writes an atomic journal in `.drup/restores/` before each destructive boundary, creates an independent rescue backup, and preserves the pre-swap filesystem tree. Any non-completed journal blocks later run mutations until an operator explicitly reconciles it.
- Database imports are declared **non-atomic**: after an import failure or interrupted filesystem swap, inspect the journal's `rescue_backup_id` and `previous_path`, reconcile the database deliberately, and only then resume. Never blind-retry a journaled restore.
- **NEVER DELETE A BACKUP AUTOMATICALLY.** Retain the original and rescue backups for the user to delete explicitly via `test_backup_delete` with `confirm`. Phase 0 backup is preserved across successful final stages and reused on failed runs.

---

## 7. Architecture & Implementation Reference

```
internal/mcp/                         (transport only — descriptors + JSON-RPC)
├── server.go         transport, ToolSpec catalog, handleToolCall dispatch
└── tools.go          placeholder stubs (overridden in production)

internal/app/mcp_tools.go              (real handlers + descriptor-driven WireMCPTools)
└── WireMCPTools(s)   resolves real handlers from ToolSpec and applies the effect guard before Run
```

- In production: `app.go` calls `WireMCPTools(server)` before `server.Run()`. All tool calls go through real handlers.
- In tests where `WireMCPTools` is NOT called, the stubs in `internal/mcp/tools.go` keep the server crash-free so `tools/list` returns a valid catalog of names + schemas. Tool calls return shape-valid empty responses — never errors.
- **Wiring rule**: every descriptor must resolve to a real handler; stub-visible descriptors must also resolve to a stub handler. The four `test_backup_*` descriptors intentionally omit stubs but remain visible once production wiring runs.

> Descriptor completeness and stub parity are enforced by `TestToolSpecsAreTheSingleCatalogForSchemasAndStubs`; reverse asymmetry for backup tools remains enforced by `TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric`.

## 8. Security Model (one-screen summary)

| Tool | Protection | Mechanism |
|---|---|---|
| `apply_patch` | URL allowlist | `*.drupal.org` only |
| `drush_exec` | Blocklist + injection guard | 6 dangerous commands rejected; `;|&\$`` chars rejected in command + args |
| `composer_require` | Conflict prevention | `--dry-run` first; reactor regex on package name |
| `core_upgrade_apply` / `patch_rollback` | Dirty-tree guard | `git status --porcelain` must be empty |
| `upgrade_scan` / `core_upgrade_*` | Path-traversal guard | Rejects `..` in `project_path` |
| `composer_require` | Shell injection | Package name must match `vendor/package[:constraint]` |
| `cleanup` | Failure-mode preservation | Refuses when `validate_passed=false` so debugging state is kept |
| `test_backup_restore` / `test_backup_delete` | Confirmation gate | Both require explicit fields; restore requires `confirm:true` and a matching `plan_id` |

## 9. Self-test Checklist for Sub-Agents

Each fixer sub-agent should self-verify before declaring done:

- [ ] I called the right MCP tool — not shell.
- [ ] I checked the response `success` / `error` field, not just exit code.
- [ ] For mutating tools: `test_backup_create` ran and `backup_id` recorded.
- [ ] For env-aware tools: I let the tool call `detect_env` itself; I did not pre-compute the prefix.
- [ ] For drush commands: not in the blocklist.
- [ ] For composer mutations: package name matches the regex; if dry-run failed, I did not retry without fixing the conflict.
- [ ] For paths: absolute, no `..`.
- [ ] I did NOT call `scan`/`validate`/`upgrade_scan` myself — those belong to `drup-validator`.

A sub-agent that fails this checklist must report `status: blocked` and surface the violated item in `risks[]`.

### `restore_recover`

MCP wrapper for the explicit persisted filesystem recovery procedure. It requires `project_path`, `journal_id`, and `confirm:true`; it fails closed on ambiguous evidence and keeps DB reconciliation blocked.

### `restore_check`

Read-only preflight for a confirmed `RestorePlan`. It validates backup integrity, canonical paths, environment detection, conservative permissions, and same-volume staging space before any rescue backup, database import, or filesystem change. Its `database_mode: "non_atomic"` requires the journal recovery procedure if a restore later fails.

### Restore recovery

A `recovery_required` journal can execute filesystem reconciliation only through `backup.Manager.Recover(project, journal_id, true)`. It fails closed unless the preserved tree is present under `.drup/restores/`; it leaves `filesystem_recovered` blocked until the operator restores the rescue database and verifies it.
