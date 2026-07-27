# drup MCP Tools — Agent Reference

This document is an **agent-facing reference** for the 25 MCP tools exposed by the `drup` binary over stdio (JSON-RPC 2.0). It exists so the orchestrator and sub-agents pick the right tool fast, sequence calls correctly, and never trip a guardrail.

**Tooling totals at runtime:** 20 categorized tools in `defaultTools()` (see §5) + the `cleanup` post-pipeline utility (§5.21) + `custom_compat_fix` (§5.22) + 4 backup tools (§6) = **26 total**.

For tool **schemas** (JSON Schema, required fields, types) call `tools/list` — do not hardcode them here. For tool **internals** (Go package, test coverage) read `internal/app/mcp_tools.go`.

---

## 1. Core Directives

1. **Pick the MCP tool, never shell out.** Every operation in the upgrade pipeline has a deterministic MCP tool. If you find yourself about to run drush, composer, git-apply, curl, or any patch operation via Bash — STOP and pick a tool. The blocker isn't a guideline; the tools do things Bash cannot (env auto-prefix, dry-run pre-check, drush blocklist, git checkpoint).
2. **Use exact registry names.** Tools are registered as short names (`scan`, `validate`, `core_upgrade_apply`). Depending on host orchestrator they may appear prefixed (`drup_scan`, `drup_validate`). Call your named tool by whatever name it shows in your function-calling UI — never hardcode a prefix.
3. **Read the response, do not pattern-match its shape.** Every tool returns a unique envelope. The decision rules below tell you what `success`, `total_errors`, `is_applied`, etc. mean per tool.
4. **Errors are returned two ways.** Either as `"isError": true` (the tool itself failed — bad arg, blocked command, unreachable), or as a successful response body that has `"success": false` or `"supported": false` — meaning the tool ran but the project state is what it is. **Both are non-panics.** Read the body.
5. **Backup before mutations.** Any tool that mutates `composer.json`, the working tree, or drupal.org state (apply_patch, core_upgrade_apply, patch_rollback, composer_require without dry-run, create_patch) requires a `test_backup_create` recorded in run state. The orchestrator enforces this; sub-agents must read it before dispatching mutators.
6. **`tasks/list` advertises only what is in `s.tools`.** If a tool is in code but missing from `defaultTools()` AND missing from `toolRegistry`, clients see it with an empty `inputSchema` and may break. Verify the wiring is symmetric (defaultTools entry ↔ toolRegistry entry ↔ real handler ↔ doc entry) before dispatching.

---

## 2. Decision Matrix — Pick the Tool for the Intent

| Intent | Use this tool | Not this |
|---|---|---|
| Find out which Drupal/PHP versions are compatible | `drupal_version_matrix` | curl + grep |
| Detect dev environment (ddev, lando, docker, direct) | `detect_env` | shell markers / `which drush` |
| Project-wide deprecation scan, with all modules | `upgrade_scan` (atomic install→enable→analyze) | `scan` (assumes upgrade_status already installed) |
| Re-scan after a fix to count remaining errors | `scan` or `validate` | `upgrade_scan` (skips fresh install) |
| Validate just one module or one file | `validate` (with `module` or `file`) | `scan` (no filter) |
| "Will this contrib work on Drupal 11?" | `contrib_check` | `contrib_upgrade_path` (different question) |
| "Which exact version should I **install** for D11?" | `contrib_upgrade_path` | `contrib_check` (only reports has-d11-branch) |
| Find a patch for an issue or module | `issue_patches` | curl + grep d.o HTML |
| Download and apply a .patch + register in composer.json | `apply_patch` | curl + git apply manually |
| Reverse a failed patch cleanly | `patch_rollback` | git revert + manual composer edit |
| Was this patch already applied? Is it obsolete? | `patch_status`, then `patch_reconcile` | grep git log manually |
| Generate a patch from rector fixes for a contrib module | `create_patch` (writes to /tmp, does NOT auto-apply) | `autofix` (which is for **custom** code only) |
| Run rector on **/modules/custom** and **/themes** | `autofix` | `create_patch` (different purpose) |
| Bump Drupal core, with rollback checkpoint | `core_upgrade_apply` (`dry_run` first) | `composer_require drupal/core*` (bypasses checkpoint) |
| What's the next core major and a composer.json preview? | `core_upgrade_check` (read-only) | `core_upgrade_apply` (mutates) |
| Generate JSON + Markdown upgrade report | `generate_report` | manual report writing |
| After final validation, uninstall dev modules and revert temp patches | `cleanup` (with `validate_passed:true`) | shell `drush pm:uninstall` + `git revert` (no env awareness) |
| Declare the target Drupal major in your own modules and themes | `custom_compat_fix` | hand-editing every `.info.yml`, or missing them entirely |

---

## 3. Standard Pipelines / Sequencing

### 3.1 First-time upgrade of a project

```
1. test_backup_create           (mandatory before any mutation)
2. detect_env                    (set prefix, catch unsupported env early)
3. drupal_version_matrix         (does the PHP version even meet min?)
4. core_upgrade_check            (read-only next major preview)
5. core_upgrade_apply dry_run    (preview the composer.json diff)
[user gate — confirm]
6. core_upgrade_apply            (real; gets rollback_checkpoint)
7. upgrade_scan                  (install/enable upgrade_status, full analyze)
8. contrib_upgrade_path per module → composer_require → apply_patch / create_patch → patch_reconcile
9. autofix                       (custom + themes only)
10. validate                     (zero-errors gate)
11. composer update              (resolve any new conflicts)
12. drush updb
13. validate                     (final scope-global)
14. generate_report
15. custom_compat_fix project_path=... (declare D11 in custom modules/themes)
16. cleanup validate_passed=true (uninstall upgrade_status, revert temp patches)
16. test_backup cleanup (manual only — never automated)
```

### 3.2 Patch workflow per contrib module

```
patch_status     →  if NOT applied:
  issue_patches
  apply_patch     →  returns commit_hash
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

- **Purpose**: Static compatibility table D9/D10/D11 → min PHP, recommended PHP, supported-until, next major.
- **Returns**: `{ drupal_version, php_requirements: {minimum, recommended}, supported_until, upgrade_path: {next_major, migration_guide_url}, known_issues }`
- **Side-effects**: none (no network).
- **NOTE**: PHP comparison is lexicographic (`>=`). Works for `8.1, 8.3, 8.4` but NOT for `7.x` vs `8.x`. Treat as advisory, not strict semver.

### 5.8 `issue_patches`

- **Purpose**: Search Drupal.org issues for patches (RTBC prioritized). Use either `module_name` or `issue_nid`.
- **Returns**: `[{ url, status (RTBC|draft|...), date, is_patch, issue_nid }]`
- **Side-effects**: none.
- **Red flag**: passing both `module_name` AND `issue_nid`. Pick one — `issue_nid` wins.

### 5.9 `apply_patch`

- **Purpose**: Download a `.patch` from drupal.org, validate URL allowlist, `git apply`, commit, register under `composer.json → extra.patches`.
- **Returns**: `{ applied: <bool>, commit_hash: <str>, error: <str> }`
- **Prerequisites**: clean working tree; absolute `project_path`; `patch_url` from `*.drupal.org`.
- **Side-effects**: creates a git commit and a JSON edit on `composer.json`. Atomic — if any step fails, leaves no commit.
- **Error signals**: each failure mode (`URL not allowed`, `download fails`, `git apply conflict`, `composer.json malformed`) has a distinct message.
- **Red flag**: retrying on conflict without first running `patch_rollback`.

### 5.10 `patch_status`

- **Purpose**: Was a patch (by URL or by composer_package) already applied? Reads `composer.json` and `git log`.
- **Returns**: `{ is_applied, commit_hash, registered_in_composer, patch_info: {url, package, description}|null }`
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

- **Purpose**: Run `drupal-rector process` on `/modules/custom` and `/themes` only. Re-scans to report remaining errors.
- **Returns**: `{ rector_summary, remaining_errors }`
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

- **Purpose**: Read-only next-Drupal-major lookup. Returns preview of the composer.json diff. **Never mutates anything.**
- **Returns**: `{ current_version, next_version, composer_patch_preview, supported: <bool> }`
- **Side-effects**: none.
- **Red flag**: expecting it to apply the upgrade — use `core_upgrade_apply` for that.

### 5.19 `core_upgrade_apply`

- **Purpose**: Bump `drupal/core-recommended` and `drupal/core` to `target_version`. With `dry_run=true`, returns the diff preview only. With `dry_run=false`, refuses on dirty tree, commits a git checkpoint, then mutates `composer.json`.
- **Returns**: `{ success, report: <diff>, rollback_checkpoint: <sha>, stderr }`
- **Prerequisites**: clean working tree, git repo, absolute `project_path`, `target_version` string.
- **Side-effects**: on real run, creates two commits (checkpoint + bump).
- **Rollback**: `git reset --hard <rollback_checkpoint>` reverts everything cleanly.
- **Error signals**: distinct messages for dirty tree, invalid `target_version`, path containing `..`.
- **Red flag**: requesting a real (non-dry-run) apply without `test_backup_create` recorded in run state.

### 5.20 `generate_report`

- **Purpose**: Wrap `internal/report` to write `drup-report.json` and/or `drup-report.md` to the project root.
- **Returns**: `{ success, json_report_path, markdown_report_path, summary: {total_modules_checked, patches_applied, custom_files_fixed, errors_remaining, pending_human_review} }`
- **Side-effects**: writes files in `project_path` root.
- **Red flag**: assuming it's read-only — it creates artifacts.

### 5.21 `cleanup`

- **Purpose**: Post-pipeline cleanup. Uninstalls dev modules (`upgrade_status`, `drupal-rector`) and reverts any temporary patches created during rector. Only runs when `validate_passed=true`.
- **Returns**: `{ success, uninstalled: [<module>…], reverted_patches: [<url>…], stderr }`
- **Prerequisites**: absolute `project_path`; `validate_passed: true` (otherwise the tool refuses to run, intentionally — to preserve debugging state on failed validation).
- **Side-effects**: modifies the Drupal site (uninstalls modules) and the working tree (reverts temp patches).
- **Red flag**: calling it on a failing pipeline to "clean anyway" — it is designed to skip on failure. If you want to clean after a failed run, restore from `test_backup` instead.

> The wiring invariant above (cleanup is in both maps with the right properties) is enforced by `TestServer_WiringSymmetryCleanupToolIsSymmetric` in `internal/mcp/mcp_test.go`. If you add a schema property or shorten/remove `cleanup` from either `defaultTools()` or `toolRegistry`, that test will fail.

### 5.22 `custom_compat_fix`

- **Purpose**: Declares support for the target Drupal major in the project's own modules, themes and profiles by widening `core_version_requirement`. These declarations are what `preflight` reports as `core_module_compat` blockers, and no other stage rewrites them.
- **Returns**: `{ project_path, target_version, dry_run, updated, already_compatible, needs_attention, changes: [{ name, file, before, after, changed, note }] }`
- **Prerequisites**: absolute `project_path`. `target_version` defaults to `11`; `dry_run` reports the rewrites without writing them.
- **Side-effects**: rewrites `.info.yml` files under `modules/custom`, `themes/custom` and `profiles/custom`. The existing constraint is kept and the target major appended, so an extension does not silently lose the versions it already declared. The file's quoting style is preserved.
- **Never touches contrib**: composer owns `modules/contrib`, so an in-place edit there is discarded on the next `composer install`. Use `create_patch` and `apply_patch` for contrib.
- **Red flag**: an extension reported under `needs_attention` has no `core_version_requirement` at all — it still uses the removed `core:` key, and where to insert the replacement is a judgement call left to a human.

---

## 6. Backup Tools (additional)
> **Policy enforcement**: the reverse asymmetry described above (backup tools are intentionally NOT in `defaultTools()`) is enforced by `TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric` in `internal/mcp/mcp_test.go`. Adding a new tool that is not in `defaultTools()` without the `test_backup_` prefix fails that test. Adding a new backup-style tool fails it too if you forget to also register the schema in `toolRegistry` or skip the `project_path` requirement. Update both the code and this section in lockstep when changing the policy.

Not part of the categorized 21, but mandatory in the pipeline.

### `test_backup_create`, `test_backup_list`, `test_backup_restore`, `test_backup_delete`

- All four accept `project_path`. Restore also needs `backup_id` and `confirm: true`. Delete needs `backup_id`.
- Restore refuses without `confirm:true` — this is a second-line defense against accidental deletion.
- **NEVER DELETE A BACKUP AUTOMATICALLY.** Retain for the user to delete explicitly via `test_backup_delete` with `confirm`. Phase 0 backup is preserved across successful final stages and reused on failed runs.

---

## 7. Architecture & Implementation Reference

```
internal/mcp/                         (transport only — stubs + JSON-RPC)
├── server.go         transport, toolRegistry (schemas), handleToolCall dispatch
└── tools.go          placeholder stubs (overridden in production)

internal/app/mcp_tools.go              (real handlers + WireMCPTools)
└── WireMCPTools(s)   registers realHandle* into server.tools — must be called before Run
```

- In production: `app.go` calls `WireMCPTools(server)` before `server.Run()`. All tool calls go through real handlers.
- In tests where `WireMCPTools` is NOT called, the stubs in `internal/mcp/tools.go` keep the server crash-free so `tools/list` returns a valid catalog of names + schemas. Tool calls return shape-valid empty responses — never errors.
- **Wiring symmetry rule**: every tool that gets registered by `WireMCPTools` MUST also appear in both `defaultTools()` (for the stub) and `toolRegistry` (for the schema). If a name is in only one of those three places, MCP clients will receive it broken or not at all. The `cleanup` tool is one of the symmetric entries.

> **Forward symmetry** is enforced by `TestServer_WiringSymmetryEveryDefaultToolHasSchema`; reverse asymmetry for backup tools by `TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric`.

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
| `test_backup_restore` / `test_backup_delete` | Confirmation gate | Both require explicit fields; restore requires `confirm:true` |

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
