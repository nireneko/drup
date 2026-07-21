# Proposal: New MCP Tools for Drupal Upgrade Orchestrator

## Intent

The drup MCP server has 7 working tools covering scan→fix→patch→validate, but critical gaps force agents to use raw `bash` for composer/drush operations, environment detection, and upgrade intelligence. This change adds 10 new MCP tools that close those gaps, making the orchestrator self-sufficient without shell fallbacks.

The most urgent gap: `internal/exec/exec.go` runs raw OS commands with no environment awareness. Every existing tool hardcodes `"drush"` as the binary, which breaks in ddev/lando/docker4drupal environments where the prefix must be `ddev drush` or `lando drush`.

## Scope

### In Scope
- 10 new MCP tools across 3 priority tiers
- Environment detection and caching subsystem
- Safe composer/drush execution wrappers
- Upgrade path intelligence for contrib modules
- Patch lifecycle management (status + rollback)
- Report generation via existing `report` package

### Out of Scope
- Modifying existing 7 tools (separate change if needed)
- E2E testing infrastructure (planned separately)
- LLM-based patch generation (future)

## Capabilities

### New Capabilities
- `env-detection`: Detect and cache Drupal dev environment (ddev, lando, docker4drupal, direct)
- `command-execution`: Safe composer/drush wrappers with validation and structured output
- `upgrade-scan`: Atomic install+enable+scan+filter for upgrade_status in one call
- `upgrade-intelligence`: Contrib upgrade path resolution for next Drupal major
- `patch-lifecycle`: Patch status checking and rollback support

### Modified Capabilities
- `mcp-server`: Register 10 new tool handlers in `internal/mcp/tools.go` and `internal/app/mcp_tools.go`

## Approach

### Tier 1: Core Infrastructure (HIGH priority, implement first)

#### 1. `detect_env`
- **Purpose**: Detect Drupal dev environment, cache result for all subsequent tool calls
- **Input**: `{ "project_path": "string (required)", "force_detect": "boolean (optional)" }`
- **Output**: `{ "environment": "ddev|lando|docker4drupal|direct|unknown", "command_prefix": ["string"], "detected_at": "string" }`
- **Detection logic**: Check for `.ddev/` → ddev, `.lando.yml` → lando, `docker-compose.yml` + `*drupal*` → docker4drupal, else direct
- **Cache**: In-memory map keyed by project_path; `force_detect` bypasses cache
- **Fallback**: If ambiguous, return `"unknown"` with empty prefix; agent must ask user or set config
- **Dependencies**: None (new package `internal/envdetect`)
- **Why**: Every existing tool hardcodes `"drush"` — this is the foundation all other tools need

#### 2. `upgrade_scan`
- **Purpose**: One-call upgrade_status analysis: require (if missing) → enable (if disabled) → analyze → filter
- **Input**: `{ "project_path": "string (required)", "scope": "string (optional: env|contrib|custom|theme)", "module": "string (optional)" }`
- **Output**: `{ "total_errors": "number", "modules": [...], "upgrade_status_installed": "boolean", "upgrade_status_enabled": "boolean" }`
- **Flow**: Check if `upgrade_status` in composer.json → `composer require` if missing → `drush en` if disabled → `drush upgrade_status:analyze --format=json` → parse + filter
- **Dependencies**: `detect_env` (for command prefix), `composer_require`, `drush_exec`
- **Why**: Current `scan` tool fails if upgrade_status isn't pre-installed; agent needs 3-4 bash calls today

#### 3. `composer_require`
- **Purpose**: Safe `composer require` with validation and structured output
- **Input**: `{ "project_path": "string (required)", "package": "string (required)", "dev": "boolean (optional)", "no_update": "boolean (optional)" }`
- **Output**: `{ "success": "boolean", "installed_version": "string", "stdout": "string", "stderr": "string", "exit_code": "number" }`
- **Validation**: Package name format, version constraint syntax, conflict pre-check via `composer require --dry-run`
- **Dependencies**: `detect_env` (for command prefix)
- **Why**: Stage 4 (Contrib Loop) runs composer require frequently; manual bash is error-prone

#### 4. `drush_exec`
- **Purpose**: Safe drush execution with Drupal context validation
- **Input**: `{ "project_path": "string (required)", "command": "string (required)", "args": ["string (optional)"], "format": "string (optional: json|table|csv|yaml)" }`
- **Output**: `{ "success": "boolean", "output": "object|string", "stderr": "string", "exit_code": "number" }`
- **Safety**: Blocklist dangerous commands (`sql-drop`, `site-install`, `sql-sanitize`); auto-add `--root` flag; parse JSON output when format=json
- **Dependencies**: `detect_env` (for command prefix)
- **Why**: Used throughout pipeline for module management; needs consistent error handling

#### 5. `contrib_upgrade_path`
- **Purpose**: Find recommended contrib version for NEXT Drupal major (not just "is D11 supported?")
- **Input**: `{ "module_machine_name": "string (required)", "current_drupal_version": "string (required)", "target_drupal_version": "string (required)" }`
- **Output**: `{ "module": "string", "current_version": "string", "recommended_upgrade": { "version": "string", "drupal_compatibility": ["string"], "release_date": "string", "is_stable": "boolean" }, "alternative_versions": [...], "upgrade_notes": "string" }`
- **Implementation**: Extend `drupalorg.CheckRelease()` to fetch full release-history XML, parse all branches, filter by target compatibility, prefer latest stable
- **Dependencies**: None (extends existing `internal/drupalorg`)
- **Why**: `contrib_check` answers "is it compatible?" but not "which version should I install?"

### Tier 2: Patch Lifecycle (MEDIUM priority)

#### 6. `patch_status`
- **Purpose**: Check if a patch is already applied
- **Input**: `{ "project_path": "string (required)", "patch_url": "string (optional)", "composer_package": "string (optional)" }`
- **Output**: `{ "is_applied": "boolean", "commit_hash": "string", "registered_in_composer": "boolean", "patch_info": { "url": "string", "package": "string" } }`
- **Checks**: composer.json `extra.patches`, git log for patch-related commits
- **Dependencies**: None
- **Why**: Pipeline resumability — avoid re-applying patches on retry

#### 7. `patch_rollback`
- **Purpose**: Revert a previously applied patch cleanly
- **Input**: `{ "project_path": "string (required)", "patch_url": "string (required)", "composer_package": "string (required)" }`
- **Output**: `{ "success": "boolean", "reverted_commit": "string", "removed_from_composer": "boolean", "error": "string" }`
- **Implementation**: `git revert` the patch commit, remove from composer.json `extra.patches`, run `composer update` for that package
- **Dependencies**: None
- **Why**: When patches fail validation, need clean rollback before retry

### Tier 3: Reporting & Info (MEDIUM/LOW priority)

#### 8. `generate_report`
- **Purpose**: Generate upgrade reports (JSON + Markdown)
- **Input**: `{ "project_path": "string (required)", "report_type": "string (optional: json|markdown|both)", "include_scan_data": "boolean", "include_patch_list": "boolean" }`
- **Output**: `{ "success": "boolean", "json_report_path": "string", "markdown_report_path": "string", "summary": { "total_modules_checked": "number", "patches_applied": "number", "errors_remaining": "number" } }`
- **Implementation**: Wrap existing `report.GenerateJSON()` and `report.GenerateMarkdown()`
- **Dependencies**: None (uses existing `internal/report`)
- **Why**: Stage 7 needs report generation; package exists but no MCP tool exposes it

#### 9. `module_info`
- **Purpose**: Module metadata and health indicators from Drupal.org
- **Input**: `{ "module_machine_name": "string (required)", "include_maintainers": "boolean", "include_dependencies": "boolean" }`
- **Output**: `{ "module": "string", "title": "string", "maintainers": ["string"], "downloads": "number", "last_release": "string", "open_issues": "number", "dependencies": { "required": ["string"] } }`
- **Dependencies**: None (extends `internal/drupalorg`)
- **Why**: Decision support when evaluating problematic modules

#### 10. `drupal_version_matrix`
- **Purpose**: Drupal/PHP compatibility matrix
- **Input**: `{ "drupal_version": "string (optional)", "php_version": "string (optional)" }`
- **Output**: `{ "drupal_version": "string", "php_requirements": { "minimum": "string", "recommended": "string" }, "supported_until": "string", "upgrade_path": { "next_major": "string" } }`
- **Implementation**: Static data map (low maintenance, fast lookup)
- **Dependencies**: None
- **Why**: Preflight validation; rarely called but prevents wasted effort

## Implementation Order

1. **Phase 1**: `detect_env` → `composer_require` → `drush_exec` (foundation; all depend on env detection)
2. **Phase 2**: `upgrade_scan` → `contrib_upgrade_path` (upgrade intelligence)
3. **Phase 3**: `patch_status` → `patch_rollback` (patch lifecycle)
4. **Phase 4**: `generate_report` → `module_info` → `drupal_version_matrix` (reporting & info)

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/envdetect/` | New | Environment detection package with caching |
| `internal/mcp/tools.go` | Modified | Register 10 new placeholder handlers |
| `internal/app/mcp_tools.go` | Modified | Register 10 new real handlers |
| `internal/drupalorg/` | Modified | Extend for upgrade path and module info |
| `internal/exec/exec.go` | Modified | Add environment-aware command prefixing |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Environment detection false positives | Medium | Explicit marker files (.ddev/, .lando.yml); "unknown" fallback forces user decision |
| Composer require hangs on conflicts | Medium | Add timeout (60s); dry-run pre-check before actual install |
| Drush command blocklist too restrictive | Low | Start conservative, expand based on real pipeline needs |
| Cache staleness for detect_env | Low | `force_detect` parameter; cache keyed by project_path mtime |

## Rollback Plan

All new tools are additive — no existing tools are modified. Rollback = remove new handlers from `WireMCPTools()` and delete new packages. No data migration needed.

## Dependencies

- None external; all built on existing internal packages

## Success Criteria

- [ ] All 10 tools registered and callable via MCP protocol
- [ ] `detect_env` correctly identifies ddev, lando, direct environments in test fixtures
- [ ] `upgrade_scan` completes full install→enable→analyze flow in one call
- [ ] `composer_require` and `drush_exec` use environment-aware command prefixes
- [ ] `go test ./...` passes with tests for all new tools
- [ ] `go vet ./...` clean
