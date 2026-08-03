# Exploration: drup-retrospective-fixes

## 1. Context

The retrospective (RETROSPECTIVE.md, written by mimo-v2.5 on 2026-08-03) documents a failed Drupal 10.6 → 11 upgrade pipeline run on `/home/borja/sites/drupal/upgrade-test`. The primary failures were: `drup_detect_env` returning empty, the orchestrator never attempting sub-agent dispatch, MCP tools returning no output, and `upgrade_status` being in an inconsistent state. A prior exploration (`archive/2026-07-26-drup-retrospective-bugs/exploration.md`) already addressed many of the original bugs. This exploration verifies which retrospective claims still hold against the current codebase and which have been resolved.

## 2. Workspace Shape

```
/home/borja/sites/agents/drup/
├── cmd/drup/main.go                    # CLI entrypoint
├── internal/
│   ├── app/
│   │   ├── app.go                      # CLI dispatch (Run)
│   │   ├── commands.go                 # RunPreflight, RunScan, RunValidate, cliRun()
│   │   ├── mcp_tools.go               # Real MCP tool handlers (WireMCPTools)
│   │   ├── mcp_tools_test.go          # MCP tool tests
│   │   ├── contrib_patch.go           # Contrib patching logic
│   │   ├── compat.go                  # core_version_requirement widening
│   │   ├── lenient.go                 # drupal-lenient allow list
│   │   ├── cleanup.go                 # Post-pipeline cleanup
│   │   └── backup_commands.go         # CLI backup commands
│   ├── mcp/
│   │   ├── server.go                  # MCP JSON-RPC server + toolRegistry schemas
│   │   └── tools.go                   # Placeholder handlers (overridden by WireMCPTools)
│   ├── envdetect/envdetect.go         # Environment detection (ddev/lando/docker/direct)
│   ├── exec/exec.go                   # Subprocess runner (Run, RunWithEnv)
│   ├── backup/backup.go               # Testing backup create/list/restore/delete
│   ├── scan/                          # upgrade_status:analyze parser
│   ├── drupalorg/drupalorg.go         # Drupal.org HTTP (doWithRetry here)
│   ├── coreupgrade/                   # Core version bump + rollback
│   ├── patch/                         # Patch download + git apply
│   ├── patchreconcile/                # Patch lifecycle analysis
│   ├── report/                        # JSON + Markdown report generation
│   ├── packaging/templates/           # Agent/skill/MCP templates per platform
│   │   ├── opencode/agents/           # drup-preflight, drup-rector, drup-contrib,
│   │   │                              # drup-custom, drup-theme, drup-validator
│   │   ├── claude/agents/             # Same 6 agents
│   │   └── codex/agents/              # Same 6 agents
│   ├── state/state.go                 # ~/.config/drup/state.json
│   ├── metrics/metrics.go             # Pipeline telemetry (retries counter)
│   ├── e2e/pipeline_test.go           # Mock-based stage ordering test
│   └── ...
├── openspec/
│   ├── config.yaml
│   ├── specs/                         # 17 spec files
│   └── changes/
│       ├── drup-retrospective-fixes/  # ← THIS CHANGE
│       └── archive/                   # 12+ archived changes
└── RETROSPECTIVE.md
```

## 3. Sub-Agent Verification Table

All 6 sub-agents described in the retrospective ARE configured as agent templates. They are installed by `drup install` / `drup init` into the host platform's agent directory.

| Sub-agent | Configured? | Template locations | Tools granted | Envelope defined? |
|-----------|-------------|-------------------|---------------|-------------------|
| `drup-preflight` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-preflight.md` | Bash, MCP | ✅ `{"agent","status","summary","artifacts","evidence","risks"}` |
| `drup-rector` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-rector.md` | Bash, MCP | ✅ Same envelope, `status: completed\|failed` |
| `drup-contrib` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-contrib.md` | Bash, MCP | ✅ Same envelope, `status: updated\|patched\|created\|failed` |
| `drup-custom` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-custom.md` | Bash, Read, Edit, Grep, Glob, MCP | ✅ Same envelope, `status: fixed\|failed` |
| `drup-theme` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-theme.md` | Bash, Read, Edit, Grep, Glob, MCP | ✅ Same envelope, `status: fixed\|failed` |
| `drup-validator` | ✅ YES | `templates/{opencode,claude,codex}/agents/drup-validator.md` | MCP only | ✅ Same envelope, `status: pass\|fail\|blocked` |

**Key finding**: The sub-agents exist as *prompt templates* installed into the host agent platform. They are NOT Go code or running services — they are markdown files with frontmatter that define the agent's role, allowed tools, input/output contracts, and model routing. The orchestrator (the drup SKILL.md) dispatches them via the host platform's `task()` primitive. The retrospective's question "are they configured?" is answered: **yes, they are**. Whether the orchestrator model actually uses them is a model behavior issue, not a configuration issue.

## 4. MCP Tool Envelope Audit

The retrospective claims tools return "empty response". **This is no longer true for the current codebase.** All 28 tools return structured JSON. However, they do NOT use a uniform envelope — each tool returns a tool-specific shape. The retrospective suggests a uniform envelope `{"status":"pass|fail","summary":"...","evidence":{...},"risks":[...]}`.

### Audit Table

| Tool | Go file:line | Current return shape | Uniform envelope? | Severity |
|------|-------------|---------------------|-------------------|----------|
| `drup_detect_env` | `mcp_tools.go:513-536` | `{"environment","command_prefix","detected_at"}` | ❌ No status/summary | P2 |
| `drup_test_backup_create` | `mcp_tools.go:96-106` | Backup `Manifest` struct (JSON) | ❌ No status/summary | P2 |
| `drup_test_backup_list` | `mcp_tools.go:107-117` | `[]Manifest` (JSON array) | ❌ No status/summary | P2 |
| `drup_test_backup_restore` | `mcp_tools.go:118-134` | `{"backup_id","restored":true}` | ❌ No status/summary | P2 |
| `drup_test_backup_delete` | `mcp_tools.go:135-150` | `{"backup_id","deleted":true}` | ❌ No status/summary | P2 |
| `drup_scan` | `mcp_tools.go:152-179` | `scan.ScanResult` (total_errors, modules, etc.) | ❌ No status/summary | P2 |
| `drup_upgrade_scan` | `mcp_tools.go:777-917` | `{"total_errors","modules","upgrade_status_installed","upgrade_status_enabled"}` | ❌ No status/summary | P2 |
| `drup_validate` | `mcp_tools.go:311-359` | `{"total_errors","errors"}` | ❌ No status/summary | P2 |
| `drup_composer_require` | `mcp_tools.go:538-627` | `{"success","installed_version","stdout","stderr","exit_code"}` | ❌ Close but not uniform | P2 |
| `drup_drush_exec` | `mcp_tools.go:644-730` | `{"success","output","stderr","exit_code"}` | ❌ Close but not uniform | P2 |
| `drup_autofix` | `mcp_tools.go:181-239` | `{"rector_summary","remaining_errors"}` | ❌ No status/summary | P2 |
| `drup_create_patch` | `mcp_tools.go:404-509` | `{"patch_path","applied"}` | ❌ No status/summary | P2 |
| `drup_apply_patch` | `mcp_tools.go:280-296` | `DoApplyPatch` result | ❌ No status/summary | P2 |
| `drup_patch_status` | `mcp_tools.go:930-1045` | `{"is_applied","commit_hash","registered_in_composer","patch_info"}` | ❌ No status/summary | P2 |
| `drup_patch_rollback` | `mcp_tools.go:1094-1305` | `{"success","reverted_commit","removed_from_composer"}` | ❌ Close but not uniform | P2 |
| `drup_patch_reconcile` | `mcp_tools.go:1764-1784` | `patchreconcile.Reconcile` result | ❌ No status/summary | P2 |
| `drup_contrib_check` | `mcp_tools.go:241-254` | `drupalorg.CheckRelease` result | ❌ No status/summary | P2 |
| `drup_contrib_compat_patch` | `mcp_tools.go:1464-1488` | `PatchContribForCore` result | ❌ No status/summary | P2 |
| `drup_contrib_allow_lenient` | `mcp_tools.go:1444-1462` | `AllowLenient` result | ❌ No status/summary | P2 |
| `drup_contrib_upgrade_path` | `mcp_tools.go:732-753` | `drupalorg.UpgradePath` result | ❌ No status/summary | P2 |
| `drup_module_info` | `mcp_tools.go:1490-1513` | `drupalorg.ModuleInfo` result | ❌ No status/summary | P2 |
| `drup_module_release_info` | `mcp_tools.go:755-775` | `drupalorg.ModuleReleaseInfo` result | ❌ No status/summary | P2 |
| `drup_issue_patches` | `mcp_tools.go:256-278` | `drupalorg.SearchPatches` result | ❌ No status/summary | P2 |
| `drup_custom_compat_fix` | `mcp_tools.go:1420-1442` | `BumpCustomCoreCompat` result | ❌ No status/summary | P2 |
| `drup_core_upgrade_check` | `mcp_tools.go:1629-1690` | `{"current_version","next_version","composer_patch_preview","supported"}` | ❌ No status/summary | P2 |
| `drup_core_upgrade_apply` | `mcp_tools.go:1733-1762` | `{"success","report","rollback_checkpoint","stderr"}` | ❌ Close but not uniform | P2 |
| `drup_cleanup` | `mcp_tools.go:1786-1832` | Parsed JSON from `RunCleanup` or `{"success","output"}` | ❌ No status/summary | P2 |
| `drup_generate_report` | `mcp_tools.go:1307-1418` | `{"success","json_report_path","markdown_report_path","summary"}` | ❌ Close but not uniform | P2 |

**Summary**: All 28 tools return structured JSON (the retrospective's "empty response" claim is stale). However, none use the uniform `{"status","summary","evidence","risks"}` envelope the retrospective suggests. Tool-specific shapes are fine for machine consumption but make it harder for the orchestrator model to uniformly detect success/failure without parsing each tool's unique shape.

### Error handling pattern

When a tool fails, it returns `(nil, error)` → the MCP server sends a JSON-RPC error response (`mcp/server.go:420-421`). This means errors ARE surfaced to the caller, but as JSON-RPC errors, not as structured `{"status":"fail"}` payloads. The orchestrator model sees these as MCP error messages, not as tool output.

## 5. Env Detection & Drush Wrapper

### Detection storage

- **Where**: `envdetect.DefaultDetector.cache` — an in-memory `map[string]*Detection` protected by `sync.Mutex` (`envdetect.go:42-45`).
- **NOT persisted to disk.** Detection runs on first access per project path per process lifetime.
- **Cache invalidation**: Checks project root `mtime` against `DetectedAt`; re-detects if the directory is newer (`envdetect.go:86-93`).

### Detection → Drush/Composer consumption path

```
realHandleDrushExec (mcp_tools.go:694)
  → defaultEnvDetector.Detect(projectPath, false)
  → detection.CommandPrefix  (e.g., ["ddev", "exec"] for DDEV)
  → drupexec.RunWithEnv(projectPath, detection.CommandPrefix, "drush", cmdArgs...)
  → exec.CommandInDir(projectPath, "ddev", "exec", "drush", ...)
```

```
realHandleComposerRequire (mcp_tools.go:575)
  → defaultEnvDetector.Detect(projectPath, false)
  → drupexec.RunWithEnv(projectPath, detection.CommandPrefix, "composer", ...)
```

```
cliRun (commands.go:118-127) — used by RunScan, RunValidate, RunPreflight, etc.
  → defaultEnvDetector.Detect(projectPath, false)
  → drupexec.RunWithEnv(projectPath, detection.CommandPrefix, cmd, args...)
```

### What happens if `drup_detect_env` is never called?

The first call to ANY tool that uses `cliRun()` or calls `defaultEnvDetector.Detect()` directly will trigger detection. The cache starts empty but detection is lazy — it runs on first access. **There is no failure path from "never calling detect_env"** as long as the project path is valid and has a recognized marker file.

### What happens if detection returns `unsupported`?

- `realHandleCoreUpgradeCheck` (line 1650-1658): returns `{"supported": false}` — graceful.
- `cliRun()` and `realHandleDrushExec`: `CommandPrefix` is `[]string{}`, so commands run directly. If the project has no recognized env markers, commands like `drush` will fail with "command not found" because there's no wrapper. **This is the real risk**: an unsupported environment silently falls through to direct execution, which will fail with confusing errors.

### DDEV prefix

For DDEV, the prefix is `["ddev", "exec"]` (envdetect.go:110), NOT `["ddev"]`. The comment explains: `ddev drush` collapses exit codes, while `ddev exec drush` preserves them. This is intentional.

## 6. Retry/Backoff & Timeout Handling

### What exists

- **`doWithRetry`** in `internal/drupalorg/drupalorg.go:31-80`: 3 attempts, 500ms base exponential backoff. Retries on HTTP 412, 429, 500, 502, 503, 504, and transport errors. Used exclusively for Drupal.org HTTP calls.
- **`metrics.Collector.RecordRetry()`** in `internal/metrics/metrics.go:92-95`: Counter for pipeline-level retry tracking. Called externally, not by MCP tools.

### What is missing

- **No retry for MCP tool handlers**: `realHandleDrushExec`, `realHandleComposerRequire`, `realHandleScan`, `realHandleTestBackupCreate/List`, etc. have NO retry logic. A timeout or transient failure returns an error immediately.
- **No timeout configuration**: The MCP server (`mcp/server.go`) has no per-tool timeout. The host platform's MCP client sets timeouts (the retrospective saw `MCP error -32001: Request timed out` from the client side).
- **No retry for `upgrade_status` enable**: `realHandleUpgradeScan` (lines 838-856) tries to enable the module once. If `drush en` fails, it returns an error. No retry.
- **No retry for backup operations**: `backup.Create()` and `backup.List()` are single-attempt. The retrospective's `drup_test_backup_list` timeout had no retry.

## 7. Other Retrospective Findings

### PreExistingConfigException handling

- **Partially handled**: `realHandleUpgradeScan` (mcp_tools.go:840-841) runs `drush config:delete update.settings` before enabling `upgrade_status`. This addresses the specific `update.settings` conflict seen in the retrospective.
- **Not generally handled**: No tool catches `PreExistingConfigException` for other config conflicts during module install. If a different config key conflicts, the tool returns an error.
- **`drup_drush_exec` blocklist** (mcp_tools.go:32-39): Blocks `php-eval`, `sql-drop`, `site-install`, etc. The retrospective used `php-eval` to work around the issue — that path is now blocked.

### Module state verification before scan

- **`realHandleUpgradeScan`** (mcp_tools.go:825-856): ✅ DOES verify `upgrade_status` is enabled. Checks `pm:list --status=enabled --format=json`, and if not enabled, runs `config:delete update.settings` + `drush en upgrade_status -y` + `drush cr`.
- **`realHandleScan`** (mcp_tools.go:152-179): ❌ Does NOT verify. Runs `drush upgrade_status:analyze` directly. If the module is not enabled, it fails with "no commands defined in the upgrade_status namespace" — exactly the error from the retrospective.

### Smoke test for MCP tools

- **`TestWireMCPTools_NoPanic`** (mcp_tools_test.go:22-28): Verifies `WireMCPTools` registers without panic.
- **`TestWireMCPTools_AllToolsRegistered`** (mcp_tools_test.go:30-45): Claims to verify all tools but only logs — does NOT actually assert the count.
- **No smoke test that calls every tool** and verifies it returns valid JSON.
- **E2E test** (`e2e/pipeline_test.go`): Mock-based stage ordering test. Does NOT test real MCP tool responses.

### Key-value hack

The retrospective describes manually inserting into `key_value` table (line 296-298). **No evidence in current code** that any tool does this. The `realHandleUpgradeScan` approach is `drush en upgrade_status -y` + `drush cr`, which is the correct Drupal API path.

### Orchestrator executing bash

The retrospective notes the orchestrator ran ~30 bash commands instead of using sub-agents. This is a **model behavior issue**, not a code issue. The drup SKILL.md explicitly says "You NEVER call Bash" (SKILL.md:13). Whether the model obeys this is outside the scope of code changes.

## 8. Open Questions for the User

1. **Uniform envelope**: Should all 28 MCP tools be wrapped in a uniform `{"status","summary","evidence","risks"}` envelope? This would be a large change touching every handler. The alternative is to keep tool-specific shapes and document them clearly, since the sub-agent prompts already describe expected output formats.

2. **Retry scope**: Should retry/backoff be added to MCP tool handlers (drush, composer, backup), or is the existing Drupal.org HTTP retry sufficient? Retrying a `drush` command that failed due to a real error (e.g., module not found) would just waste time.

3. **`realHandleScan` vs `realHandleUpgradeScan`**: `realHandleScan` does NOT auto-enable `upgrade_status` while `realHandleUpgradeScan` does. Should `realHandleScan` be updated to match, or should it be deprecated in favor of `realHandleUpgradeScan`?

4. **Sub-agent testing**: The sub-agent templates exist and are well-defined. The retrospective's issue was that the orchestrator model didn't use them. Is there a code change that would make this more reliable (e.g., removing direct MCP tool access from the orchestrator skill), or is this purely a prompt/model training concern?

## 9. Risks & Unknowns

1. **Envelope unification is high-effort, low-clarity**: Wrapping 28 tools in a uniform envelope is a large diff with unclear benefit. The sub-agent prompts already describe expected shapes. The orchestrator model may not parse a uniform envelope any better than tool-specific shapes.

2. **Retry could mask real errors**: Adding retry to drush/composer calls could turn a 1-second "module not found" error into a 10-second "module not found × 3" delay. Retry needs to be selective (timeouts, transient errors) not blanket.

3. **`realHandleScan` is a footgun**: It silently assumes `upgrade_status` is enabled. The retrospective's failure came from this exact path. Either it needs the same auto-enable logic as `realHandleUpgradeScan`, or it should be clearly documented as "requires upgrade_status to be pre-enabled."

4. **Sub-agent dispatch is a model behavior issue**: The templates are correct. The SKILL.md is explicit. If the model ignores them, code changes to the templates may not help — the issue may be in how the orchestrator skill is loaded or how the model is prompted at runtime.

5. **In-memory env detection is process-scoped**: If the MCP server restarts, the detection cache is lost. This is fine for normal operation (lazy re-detection), but means each new MCP connection re-detects from scratch.

## 10. Recommended Next Step

**Go straight to `sdd-propose`** with a focused scope. The retrospective raises many issues, but the codebase has already been significantly improved since the original bugs were filed. The actionable items for THIS change are:

1. **P0**: Fix `realHandleScan` to auto-enable `upgrade_status` (same pattern as `realHandleUpgradeScan`), OR deprecate it in favor of `realHandleUpgradeScan`. This is the exact failure path from the retrospective.
2. **P1**: Add a uniform response envelope wrapper at the MCP server level (not per-handler) so every tool response includes `status` and `summary` fields. This can be done in `server.go:handleToolCall` by wrapping the handler result.
3. **P2**: Add selective retry for timeout-class errors in `drupexec.RunWithEnv` (distinguish "command not found" from "timed out").
4. **P3**: Fix `TestWireMCPTools_AllToolsRegistered` to actually assert the tool count.

The sub-agent configuration question is resolved: they exist and are well-defined. The orchestrator model not using them is a separate concern that may need prompt engineering or platform-level enforcement, not code changes to drup itself.
