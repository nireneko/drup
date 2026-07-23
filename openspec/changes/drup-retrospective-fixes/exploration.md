## Exploration: drup retrospective fixes

### Current State

The drup CLI is a Go binary with 15 internal packages, 20 MCP tools, and an 8-stage pipeline described in SKILL.md. A real Drupal 10.6 → 11.4 upgrade (documented in AGENT-RETROSPECTIVE.md) revealed that ~60% of the automated tooling had to be bypassed due to critical bugs.

---

### Pipeline Stage Inventory

| # | Stage (SKILL.md) | CLI Command | Implementation Status |
|---|------------------|-------------|----------------------|
| 1 | PREFLIGHT | `drup preflight` | ✅ Implemented — checks Drupal version, git clean, composer, drush, dev deps, enables upgrade_status |
| 2 | DEP CHECK | `drup scan <path>` | ⚠️ Implemented but BROKEN — exit code 3 treated as error |
| 3 | RECTOR | `drup fix <path>` | ✅ Implemented — runs rector, re-scans |
| 4 | CONTRIB LOOP | `drup contrib` / `drup issue` / `drup apply-patch` | ⚠️ Implemented but `contrib` has false negatives |
| 5 | CORE UPGRADE | `drup upgrade-core <ver>` | ✅ Implemented — composer mutation, checkpoint, rollback, updb, verify |
| 6 | CUSTOM LOOP | (no dedicated command) | ❌ NOT IMPLEMENTED — no CLI command; relies on scan + manual fix |
| 7 | FINAL VALIDATION | `drup validate <path>` | ⚠️ Implemented but BROKEN — same exit code 3 issue |
| 8 | REPORT | `drup report <path>` | ⚠️ PLACEHOLDER — hardcoded zero values, no real data collection |

**Total**: 8 stages described, 6 have CLI commands, 2 are broken by exit code handling, 1 is a placeholder, 1 has no command at all.

---

### MCP Tools Inventory

20 tools registered in `internal/mcp/tools.go` (placeholders) and overridden in `internal/app/mcp_tools.go` (real handlers).

| Tool | Parameters (struct) | Real Handler | Issues |
|------|---------------------|--------------|--------|
| `scan` | `project_path` | ✅ | Uses `-r` flag (broken under DDEV) |
| `autofix` | `project_path` | ✅ | Returns MCP error when no custom modules (should be success) |
| `contrib_check` | `module_machine_name` | ✅ | False negatives on compound constraints |
| `issue_patches` | `issue_nid`, `module_name` | ✅ | — |
| `apply_patch` | `patch_url`, `project_path` | ✅ | — |
| `validate` | `project_path`, `scope`, `module`, `file` | ✅ | Same exit code 3 bug |
| `create_patch` | `module_name`, `deprecation_details` | ✅ | — |
| `detect_env` | `project_path`, `force_detect` | ✅ | — |
| `composer_require` | `project_path`, `package`, `dev`, `no_update` | ✅ | — |
| `drush_exec` | `project_path`, `command`, `args`, `format` | ✅ | — |
| `contrib_upgrade_path` | `module_machine_name`, `current_drupal_version`, `target_drupal_version` | ✅ | — |
| `upgrade_scan` | `project_path`, `scope`, `module` | ✅ | Uses `RunWithEnv` correctly |
| `patch_status` | `project_path`, `patch_url`, `composer_package` | ✅ | — |
| `patch_rollback` | `project_path`, `patch_url`, `composer_package` | ✅ | — |
| `generate_report` | `project_path`, `report_type`, `include_scan_data`, `include_patch_list` | ⚠️ | Partial — doesn't collect scan data |
| `module_info` | `module_machine_name`, `include_maintainers`, `include_deps` | ✅ | — |
| `drupal_version_matrix` | `drupal_version`, `php_version` | ✅ | Static data |
| `core_upgrade_check` | `project_path` | ✅ | — |
| `core_upgrade_apply` | `project_path`, `target_version`, `dry_run` | ✅ | — |
| `patch_reconcile` | `module_machine_name`, `current_patch_url` | ✅ | — |

**CRITICAL**: `handleListTools` in `internal/mcp/server.go:102-117` returns `inputSchema: {"type": "object"}` with NO properties and NO required fields. Agents cannot discover what parameters each tool expects. This caused -32603 errors during the real upgrade.

---

### Exit Code Handling Analysis

**Location**: `internal/app/commands.go:74-96` (RunScan), `internal/app/commands.go:207-238` (DoValidate), `internal/app/mcp_tools.go:69-91` (realHandleScan)

**Bug**: All three call `drupexec.Run("drush", "-r", path, "upgrade_status:analyze", ...)` and treat ANY non-zero exit code as an error:

```go
if exitCode != 0 {
    return drushExecError(...)  // returns error, aborts pipeline
}
```

**Reality**: `upgrade_status:analyze` returns:
- Exit 0: no findings
- Exit 3: findings exist (this is NORMAL — it means the scan worked)

**Impact**: The entire pipeline is blocked. Every scan/validate call fails when there are actual deprecations to fix.

**Fix needed**: Treat exit code 3 as success-with-findings. Parse stdout regardless of exit code 3. Only treat exit codes 1, 2, and >3 as real errors.

---

### DDEV Support Analysis

**Detection**: `internal/envdetect/envdetect.go` correctly detects DDEV (`.ddev/` directory) and returns `CommandPrefix: ["ddev"]`.

**Usage**: Only MCP tool handlers use `RunWithEnv(detection.CommandPrefix, ...)` — specifically: `realHandleComposerRequire`, `realHandleDrushExec`, `realHandleUpgradeScan`, `realHandlePatchRollback`.

**NOT used by CLI commands**: `RunScan`, `RunFix`, `RunValidate`, `DoValidate`, `RunPreflight`, `RunUpgradeCore` all call `drupexec.Run(...)` directly — they NEVER consult envdetect. This means:
- `drup scan /path` runs `drush -r /path ...` on the host
- Under DDEV, this passes a host path to the container where it doesn't exist
- The `-r` flag is fundamentally incompatible with DDEV's path mapping

**Fix needed**: CLI commands must detect the environment and use `RunWithEnv` + `--root=` instead of `-r`. Or better: always use `--root=` which works in both direct and DDEV contexts.

---

### Contrib Compatibility Check Analysis

**Location**: `internal/drupalorg/drupalorg.go:95-149` (CheckRelease, parseReleaseXML)

**How it works**: Fetches `https://updates.drupal.org/release-history/<module>/current`, parses XML, looks for `<term><name>Core compatibility</name><value>Drupal 11</value></term>`.

**Bug**: Only checks if ANY release has "Drupal 11" in its compatibility terms. Does NOT parse `core_version_requirement` from `.info.yml` or `composer.json`. Cannot handle compound constraints like `^10.3 || ^11.0`.

**Impact**: False negatives — modules that DO support D11 are reported as incompatible. The retrospective confirms this with webform 6.3.0.

**Fix needed**: Parse the `core_version_requirement` field from the module's info.yml or composer.json (available via Drupal.org's API). Support compound constraints with `||` operators.

---

### PHP 8.4 Compatibility Analysis

**Problem**: PHP 8.4 deprecates implicitly nullable parameters (`?Type $param = null`). DrupalKernel calls `error_reporting(E_ALL)`, overriding any php.ini settings. Modules using this pattern flood stderr with deprecation notices, causing drush to exit 3.

**Current state**: NO code in drup handles this. The retrospective describes a manual workaround: adding `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` to `settings.php` AFTER the DDEV include.

**Fix needed**: Add a preflight check that detects PHP 8.4+ and patches `settings.php` to suppress E_DEPRECATED. This should be automatic, not manual.

---

### Other Code Issues Found

1. **`RunReport` is a placeholder** (`internal/app/commands.go:166-198`): Hardcodes `TotalErrors: 0` and empty slices. Does not actually collect scan data.

2. **`RunFix` error handling** (`internal/app/commands.go:98-131`): If rector exits non-zero, it prints to stderr but still calls `RunScan` — which will fail due to the exit code 3 bug.

3. **`RunPreflight` installs dev deps without env detection** (`internal/app/commands.go:526-570`): Calls `drupexec.Run("composer", "require", "--dev", ...)` directly — won't work under DDEV.

4. **`isPHPCompatible` is a string comparison** (`internal/app/mcp_tools.go:1178-1181`): `phpVer >= phpMin` does lexicographic comparison, not semver. "8.10" < "8.3" lexicographically.

5. **`RunUpgradeCore` doesn't use env detection** (`internal/app/commands.go:652-848`): Calls `execRunFn("composer", ...)` and `execRunFn("drush", ...)` directly — won't work under DDEV.

6. **No `--ddev` flag or auto-detection in CLI**: The envdetect package exists but is only wired into MCP handlers, not CLI commands.

---

### Affected Areas

- `internal/app/commands.go` — RunScan, DoValidate, RunFix, RunPreflight, RunUpgradeCore all need env detection + exit code 3 handling
- `internal/app/mcp_tools.go` — realHandleScan, realHandleValidate need exit code 3 handling
- `internal/mcp/server.go` — handleListTools needs to expose actual parameter schemas
- `internal/drupalorg/drupalorg.go` — CheckRelease needs to parse core_version_requirement
- `internal/envdetect/envdetect.go` — Already correct, but not used by CLI
- `internal/exec/exec.go` — Already supports RunWithEnv, just needs to be called

---

### Approaches

1. **Fix exit code handling + DDEV support + MCP schemas** — Address the three critical blockers that made the pipeline unusable in the real upgrade.
   - Pros: Unblocks the pipeline for DDEV users, makes MCP tools discoverable, fixes the #1 showstopper
   - Cons: Doesn't address contrib false negatives or PHP 8.4
   - Effort: Medium

2. **Full retrospective fix** — Fix all 6 issues identified: exit codes, DDEV, MCP schemas, contrib checks, PHP 8.4, placeholder report.
   - Pros: Complete fix for everything found in the retrospective
   - Cons: Larger scope, more testing needed
   - Effort: High

3. **Minimal fix** — Only fix exit code 3 handling (the single most critical bug).
   - Pros: Smallest diff, fastest to ship
   - Cons: Leaves DDEV broken, MCP schemas empty, contrib false negatives
   - Effort: Low

---

### Recommendation

**Approach 2 (Full retrospective fix)** — The retrospective explicitly documents all these issues from a real upgrade. Fixing only exit codes leaves the pipeline broken for DDEV users (the most common local dev environment). The MCP schema issue makes the tools undiscoverable by agents. All three are blocking issues that were worked around manually.

Priority order:
1. Exit code 3 handling (unblocks scan/validate)
2. DDEV support in CLI commands (unblocks containerized environments)
3. MCP tool schemas (unblocks agent discovery)
4. Contrib compatibility parsing (fixes false negatives)
5. PHP 8.4 settings.php patch (prevents deprecation floods)
6. Report data collection (completes Stage 8)

---

### Risks

- Exit code 3 handling must distinguish between "findings exist" (exit 3, stdout has data) and "drush crashed" (exit 3, stderr has error). Need to parse stdout first, then decide.
- DDEV support in CLI requires changing `drupexec.Run` calls to `drupexec.RunWithEnv` — but preflight and upgrade-core also need to detect the environment before running commands.
- MCP schema changes require defining JSON Schema for each tool's parameters — 20 tools to document.
- Contrib compatibility parsing requires fetching and parsing `.info.yml` from Drupal.org, which may not be available via the current API endpoints.

---

### Ready for Proposal

**Yes** — The codebase has been fully investigated, all affected files identified, and the issues are well-scoped. The orchestrator can proceed to proposal phase with confidence that the fixes are grounded in real code and real failures from the retrospective.
