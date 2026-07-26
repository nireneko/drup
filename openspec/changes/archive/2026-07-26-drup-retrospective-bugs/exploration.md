## Exploration: Review of UPGRADE-RETROSPECTIVA.md and informe-drup-bugs.md

### Current State

Two documents report bugs from a real Drupal 10.6 → 11.4 upgrade (nekotuto project) and from `drup install`/`drup init` usage. Three previous SDD changes have already addressed many items:

- **`drup-retrospective-fixes`** (archived 2026-07-23): Fixed exit code 3 handling, DDEV support in CLI (`cliRun()`), MCP tool parameter schemas, contrib compound constraints, PHP 8.4 `settings.php` patch, report data collection.
- **`drup-mejoras-post-retrospectiva`** (archived 2026-07-23): Added core readiness check, cleanup stage, post-D11 validation gates, smart no-op bypass, structured `issue_patches`, web root resolution, semver-based `isPHPCompatible`, DDEV composer calls, pipeline metrics, E2E scaffolding, sub-skills.
- **`analiza-cambios-codigo-actualiza-tests`** (active): Test coverage for the above fixes.

### Issue-by-Issue Status

#### From `informe-drup-bugs.md`

| # | Bug | Status | Evidence |
|---|-----|--------|----------|
| 1 | `/drup` slash command not registered | **OPEN** | `OpenCodeAdapter.WriteCommand()` writes to `~/.config/opencode/commands/`, but `drup install` has no template file under `commands/` in `templates/opencode/`. No command is generated. The bug report says it was fixed manually in `opencode.jsonc`, but `drup install` does not reproduce this fix. |
| 2 | `name` mismatch in SKILL.md | **FIXED** | Template at `templates/opencode/SKILL.md` uses `name: drup` matching the directory. |
| 3 | `triggers` field not recognized | **FIXED** | No `triggers` field in current SKILL.md templates. |
| 4 | `drup install` nested path bug | **OPEN** | `resolveFilePath()` default case: `filepath.Join(agent.SkillsDir(), path, "SKILL.md")`. When `path` = `skills/drupal-contrib-patch-writer/SKILL.md` (from packaging templates), this produces `~/.config/opencode/skills/skills/drupal-contrib-patch-writer/SKILL.md/SKILL.md` — double `skills/` and `SKILL.md` treated as directory. The `default` case needs a `skills/` prefix handler. |
| 5 | `drup init` ignores `drupal/core-recommended` | **OPEN** | `RunInit()` at `commands.go:55` checks only `require["drupal/core"]`. Does not check `drupal/core-recommended`. Note: `checkCoreReadiness()` (line 904) and `RunUpgradeCore()` (line 1087) DO check both — only `RunInit` is buggy. |
| 6 | Agent model validity | **OPEN (external)** | Agent model config is in template `.md` files under `templates/*/agents/`. This is a configuration concern, not a code bug. |

#### From `UPGRADE-RETROSPECTIVA.md`

| Issue | Status | Evidence |
|-------|--------|----------|
| `drup_detect_env` no output | **FIXED** | `realHandleDetectEnv()` at `mcp_tools.go:378-401` returns JSON with `environment`, `command_prefix`, `detected_at`. `envdetect.Detect()` correctly identifies DDEV/Lando/Docker4Drupal/Direct. |
| `drup_drush_exec` no output | **FIXED** | `realHandleDrushExec()` at `mcp_tools.go:509-595` uses `defaultEnvDetector.Detect()` → `drupexec.RunWithEnv(prefix, "drush", ...)`. Returns structured JSON with `success`, `output`, `stderr`, `exit_code`. |
| DDEV not detected | **FIXED** | `envdetect.go` detects `.ddev/` directory. `cliRun()` at `commands.go:94-100` wires detection into ALL CLI commands. |
| Exit code 3 as error | **FIXED** | `isScanExitOK()` at `commands.go:65-67` treats 0 and 3 as OK. |
| MCP tool schemas empty | **FIXED** | `server.go` `toolRegistry` has full schemas for all 20 tools. |
| Contrib false negatives | **FIXED** | `constraintMatchesDrupal()` in `drupalorg.go` handles compound `\|\|` constraints. |
| PHP 8.4 deprecation flood | **FIXED** | `patchSettingsPHP()` at `commands.go:1291-1347` auto-patches `settings.php` in preflight. |
| Report placeholder | **FIXED** | `RunReport()` calls `doValidateFn()` for live data. |
| Security advisories blocking | **ADDRESSED** | `RunUpgradeCore()` at line 1153 runs `composer config policy.advisories.block false` before upgrade. |
| `--with-all-dependencies` needed | **FIXED** | `RunUpgradeCore()` uses `-W` flag (lines 1168, 1180). |
| Scaffold plugin TypeError | **NOT REPRODUCIBLE** | No evidence in current code that drup corrupts `allowed-packages`. `coreupgrade.Apply()` modifies version constraints, not scaffold config. |
| No rollback plan | **FIXED** | `coreupgrade.Apply()` creates rollback checkpoints. `RunUpgradeCore()` creates `composer.json.bak`. |
| Scan/report timeouts | **MITIGATED** | MCP server has configurable timeouts. Smart no-op bypass skips scan when no custom code. |
| `drup_upgrade_scan` no output | **FIXED** | `realHandleUpgradeScan()` at `mcp_tools.go:620-751` returns structured JSON with modules, errors, install status. |
| `drup_module_info` HTTP 412 | **OPEN (external)** | Depends on Drupal.org API availability. Code in `drupalorg.go` makes HTTP requests; no retry/backoff implemented. |
| `drup_contrib_check` silent | **FIXED** | `realHandleContribCheck()` returns JSON from `drupalorg.CheckRelease()`. |
| `drup_contrib_upgrade_path` parse error | **OPEN (external)** | Depends on Drupal.org XML format. Code in `drupalorg.UpgradePath()`. |
| `drup_core_upgrade_check` parse error | **FIXED** | `realHandleCoreUpgradeCheck()` uses `coreupgrade.NextMajor()` + `coreCurrentVersion()` — no XML parsing of Drupal.org. |

### Affected Areas

- `internal/app/commands.go:31-61` — `RunInit()` only checks `drupal/core`, not `drupal/core-recommended`
- `internal/installer/installer.go:899-921` — `resolveFilePath()` default case produces nested `skills/skills/.../SKILL.md/SKILL.md` paths for sub-skills
- `internal/packaging/templates/opencode/` — No `commands/drup` template file for OpenCode slash command registration
- `internal/packaging/templates/claude/` — No `commands/drup` template (Claude doesn't support commands, but OpenCode does)
- `internal/drupalorg/drupalorg.go` — HTTP calls to Drupal.org without retry/backoff (low priority, external dependency)

### Approaches

1. **Fix 3 remaining code bugs** — `RunInit` core-recommended detection, `resolveFilePath` nested paths, OpenCode slash command registration.
   - Pros: Fixes all actionable bugs from both reports. Small, focused diff.
   - Cons: Doesn't address external Drupal.org API fragility (out of scope for code bugs).
   - Effort: Low

2. **Fix 3 bugs + add Drupal.org retry/backoff** — Same as above plus HTTP resilience.
   - Pros: Also addresses `module_info` HTTP 412 and `contrib_upgrade_path` parse errors.
   - Cons: Larger scope, needs HTTP client refactoring.
   - Effort: Medium

3. **Fix only `RunInit`** — Smallest possible fix.
   - Pros: One-line change.
   - Cons: Leaves installer path bug and slash command registration broken.
   - Effort: Minimal

### Recommendation

**Approach 1** — Fix the 3 remaining code bugs. They are concrete, reproducible, and directly cited in both reports. Drupal.org API resilience (Approach 2) is a separate concern that should be its own change.

Specific fixes:
1. **`RunInit`**: Add `drupal/core-recommended` to the check at `commands.go:55` (same pattern as `checkCoreReadiness` line 904).
2. **`resolveFilePath`**: Add a `case strings.HasPrefix(path, "skills/"):` handler that strips the `skills/` prefix and maps to `agent.SkillsDir() + rest`. Also handle the `SKILL.md` suffix to avoid creating a directory named `SKILL.md`.
3. **Slash command**: Add a `commands/drup.md` template under `templates/opencode/` so `drup install` creates the slash command automatically.

### Risks

- The `resolveFilePath` fix must handle all three agent adapters (Claude, OpenCode, Codex). Codex and Claude also have sub-skills under `templates/*/skills/`.
- The slash command template must contain valid OpenCode command JSON format, not just markdown.
- `RunInit` fix is trivial but must update the error message to mention both `drupal/core` and `drupal/core-recommended`.

### Ready for Proposal

**Yes** — All affected code has been read, all bugs verified against current code, and previous SDD changes checked for overlap. Three concrete bugs remain open with clear fix locations. The orchestrator can proceed to proposal phase.
