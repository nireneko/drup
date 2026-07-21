# Explore: MCP Tools Analysis

## Part A: Catalog of Current Agent Tools

### Built-in OpenCode Tools
The agent has access to these OpenCode runtime tools (from opencode.json):

- **bash**: Execute shell commands with timeout and working directory support
- **read**: Read files and directories with line numbers, offset/limit support
- **write**: Write files to filesystem (requires prior read for existing files)
- **edit**: Perform exact string replacements in files
- **glob**: Fast file pattern matching (e.g., "**/*.ts")
- **grep**: Fast content search using regex patterns
- **question**: Ask the user a question and wait for response
- **todowrite**: Manage todo lists for task tracking
- **task**: Delegate work to sub-agents
- **skill**: Load specialized skills/instructions
- **webfetch**: Fetch and convert web content (markdown/text/html)

### MCP Server Tools

#### Context7 Server (Remote)
- **context7_resolve-library-id**: Resolve package name to Context7-compatible library ID
- **context7_query-docs**: Retrieve up-to-date documentation and code examples for libraries/frameworks

#### Engram Server (Local)
Persistent memory system with these tools:
- **mem_save**: Save observations (decisions, bugs, discoveries, patterns)
- **mem_search**: Search persistent memory across sessions
- **mem_context**: Get recent session history
- **mem_session_summary**: Save end-of-session summary
- **mem_get_observation**: Get full untruncated content of an observation by ID
- **mem_save_prompt**: Save user prompt for context
- **mem_current_project**: Detect current project from working directory
- **mem_update**: Update existing observation by ID
- **mem_review**: Review observation lifecycle state
- **mem_pin/unpin**: Pin/unpin observations for priority
- **mem_suggest_topic_key**: Suggest stable topic key for upserts
- **mem_session_start/end**: Register session lifecycle
- **mem_judge**: Record verdict on memory conflicts
- **mem_compare**: Persist semantic verdicts between memories
- **mem_doctor**: Run operational diagnostics
- **mem_capture_passive**: Extract learnings from text output

#### Slidev Server (Local)
- **slide tools**: Create and manage web-based presentations (specific tools depend on slidev MCP implementation)

### Drup Project MCP Tools

The drup project exposes 7 MCP tools via stdio server (defined in `internal/mcp/tools.go`, wired with real implementations in `internal/app/mcp_tools.go`):

#### 1. scan
- **Purpose**: Run upgrade_status:analyze and return classified errors
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "project_path": "string",
    "total_errors": "number",
    "modules": [
      {
        "name": "string",
        "type": "contrib|custom|theme|core",
        "errors": [
          {
            "file": "string",
            "line": "number",
            "message": "string",
            "rule": "string",
            "severity": "string",
            "source": "string"
          }
        ]
      }
    ]
  }
  ```
- **Status**: ✅ **Implemented** - Executes `drush upgrade_status:analyze --format=json` and parses output via `scan.Parse()`
- **Implementation**: `realHandleScan()` in `internal/app/mcp_tools.go:28-50`

#### 2. autofix
- **Purpose**: Run drupal-rector on custom modules and themes
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "rector_summary": "string",
    "remaining_errors": "number"
  }
  ```
- **Status**: ✅ **Implemented** - Runs `vendor/bin/rector process` on modules/custom and themes, then re-scans
- **Implementation**: `realHandleAutofix()` in `internal/app/mcp_tools.go:52-94`

#### 3. contrib_check
- **Purpose**: Check if a contrib module has a D11-compatible release
- **Input Schema**:
  ```json
  {
    "module_machine_name": "string (required)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "module": "string",
    "has_d11_release": "boolean",
    "latest_version": "string",
    "compatible_branches": ["string"]
  }
  ```
- **Status**: ✅ **Implemented** - Fetches release-history XML from Drupal.org and parses compatibility
- **Implementation**: `realHandleContribCheck()` in `internal/app/mcp_tools.go:96-109`, uses `drupalorg.CheckRelease()`
- **Limitation**: Only checks current D11 compatibility, not upgrade path to next major

#### 4. issue_patches
- **Purpose**: Search Drupal.org issues for RTBC patches
- **Input Schema**:
  ```json
  {
    "issue_nid": "string (optional)",
    "module_name": "string (optional)"
  }
  ```
  At least one parameter required.
- **Output Schema**:
  ```json
  [
    {
      "url": "string",
      "status": "string (RTBC|Fixed|Needs review|Needs work|Unknown)",
      "date": "string",
      "is_patch": "boolean",
      "issue_nid": "string"
    }
  ]
  ```
- **Status**: ✅ **Implemented** - Tries api-d7 endpoint first, falls back to HTML scraping, sorts by RTBC priority
- **Implementation**: `realHandleIssuePatches()` in `internal/app/mcp_tools.go:111-133`, uses `drupalorg.SearchPatches()`

#### 5. apply_patch
- **Purpose**: Download and apply a .patch file, register in composer.json
- **Input Schema**:
  ```json
  {
    "patch_url": "string (required)",
    "project_path": "string (required)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "applied": "boolean",
    "commit_hash": "string (if successful)",
    "error": "string (if failed)"
  }
  ```
- **Status**: ✅ **Implemented** - Downloads patch, validates URL allowlist, applies via `git apply`, commits, registers in composer.json extra.patches
- **Implementation**: `realHandleApplyPatch()` in `internal/app/mcp_tools.go:135-149`, uses `patch.Apply()`
- **Safety**: URL allowlist restricts to drupal.org domains, atomic operation with rollback on failure

#### 6. validate
- **Purpose**: Re-run upgrade_status:analyze with scope filtering
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "scope": "string (optional: env|contrib|custom|theme|global|rector)",
    "module": "string (optional)",
    "file": "string (optional)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "total_errors": "number",
    "errors": [
      {
        "file": "string",
        "line": "number",
        "message": "string",
        "rule": "string",
        "severity": "string",
        "source": "string"
      }
    ]
  }
  ```
- **Status**: ✅ **Implemented** - Runs scan and filters by module/file if specified
- **Implementation**: `realHandleValidate()` in `internal/app/mcp_tools.go:151-194`
- **Note**: Scope parameter is accepted but filtering is done by module/file, not by scope classification

#### 7. create_patch
- **Purpose**: Generate a .patch file from deprecation analysis
- **Input Schema**:
  ```json
  {
    "module_name": "string (required)",
    "deprecation_details": "string (optional)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "patch_path": "string (temp file path)",
    "applied": "boolean"
  }
  ```
- **Status**: ⚠️ **Simplified Implementation** - Runs rector on specific module, generates git diff, writes to temp file. Does NOT actually apply the patch or use deprecation_details.
- **Implementation**: `realHandleCreatePatch()` in `internal/app/mcp_tools.go:196-253`
- **Limitation**: Doesn't intelligently generate patches from deprecation messages, just runs rector and diffs

## Part B: Proposed New MCP Tools for Drup

### 1. composer_require
- **Purpose**: Execute composer require commands safely with validation
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "package": "string (required, e.g., 'drupal/module_name:^2.0')",
    "dev": "boolean (optional, default false)",
    "no_update": "boolean (optional, default false)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "success": "boolean",
    "installed_version": "string",
    "stdout": "string",
    "stderr": "string",
    "exit_code": "number"
  }
  ```
- **Why needed**: The drup skill currently instructs the agent to run `composer require` manually via bash. This is error-prone and bypasses validation. A dedicated tool can:
  - Validate package names and version constraints
  - Check for conflicts before installing
  - Capture structured output for error handling
  - Prevent unsafe composer operations
  - Track what was installed for reporting
- **Priority**: **HIGH** - Frequently used in Stage 4 (Contrib Loop) of the pipeline
- **Implementation notes**: Wrap `drupexec.Run("composer", "require", ...)` with validation

### 2. drush_exec
- **Purpose**: Execute drush commands safely with Drupal context validation
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "command": "string (required, e.g., 'cr', 'pm:enable', 'config:import')",
    "args": ["string (optional)"],
    "format": "string (optional: json|table|csv|yaml, default json)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "success": "boolean",
    "output": "object|string (parsed if format=json)",
    "stderr": "string",
    "exit_code": "number"
  }
  ```
- **Why needed**: The skill mentions running drush commands manually (e.g., `drush en upgrade_status -y`). A dedicated tool can:
  - Validate drush is available before execution
  - Ensure commands run in correct Drupal root
  - Parse structured output (especially JSON format)
  - Prevent dangerous commands (sql-drop, site-install, etc.)
  - Provide consistent error handling
- **Priority**: **HIGH** - Used throughout the pipeline for module management
- **Implementation notes**: Wrap `drupexec.Run("drush", "-r", projectPath, command, ...)` with safety checks

### 3. contrib_upgrade_path
- **Purpose**: Find the latest compatible contrib module version for the NEXT Drupal major version
- **Input Schema**:
  ```json
  {
    "module_machine_name": "string (required)",
    "current_drupal_version": "string (required, e.g., '10.3.0')",
    "target_drupal_version": "string (required, e.g., '11.0.0')"
  }
  ```
- **Output Schema**:
  ```json
  {
    "module": "string",
    "current_version": "string",
    "recommended_upgrade": {
      "version": "string",
      "drupal_compatibility": ["string"],
      "release_date": "string",
      "is_stable": "boolean"
    },
    "alternative_versions": [
      {
        "version": "string",
        "drupal_compatibility": ["string"],
        "release_date": "string"
      }
    ],
    "upgrade_notes": "string (optional)"
  }
  ```
- **Why needed**: Current `contrib_check` only tells if a module has D11 support, but doesn't provide:
  - The recommended upgrade path (which version to install)
  - Release dates (to prefer newer releases)
  - Stability information (dev vs stable)
  - Alternative versions if recommended fails
  - Upgrade notes from maintainers
- **Priority**: **HIGH** - Critical for Stage 4 (Contrib Loop) decision-making
- **Implementation notes**: Extend `drupalorg.CheckRelease()` to fetch full release history, parse version constraints, and determine upgrade path

### 4. patch_status
- **Purpose**: Check if a specific patch is already applied to the project
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "patch_url": "string (optional)",
    "patch_description": "string (optional)",
    "composer_package": "string (optional)"
  }
  ```
  At least one parameter required.
- **Output Schema**:
  ```json
  {
    "is_applied": "boolean",
    "commit_hash": "string (if applied)",
    "applied_date": "string (if available)",
    "registered_in_composer": "boolean",
    "patch_info": {
      "url": "string",
      "description": "string",
      "package": "string"
    }
  }
  ```
- **Why needed**: The pipeline may re-run or resume after failure. Need to:
  - Avoid re-applying patches (prevents conflicts)
  - Verify patch state before validation
  - Track what was applied for reporting
  - Detect if patches were manually removed
- **Priority**: **MEDIUM** - Improves robustness of Stage 4 and Stage 6
- **Implementation notes**: Check composer.json extra.patches, git log for patch commits, and optionally verify patch content

### 5. patch_rollback
- **Purpose**: Rollback a previously applied patch
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "patch_url": "string (required)",
    "composer_package": "string (required)",
    "force": "boolean (optional, default false)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "success": "boolean",
    "reverted_commit": "string",
    "removed_from_composer": "boolean",
    "error": "string (if failed)"
  }
  ```
- **Why needed**: When patches fail validation or conflict with other changes, need to:
  - cleanly revert the patch
  - Remove from composer.json
  - Optionally revert the commit
  - Allow retry with different patch
- **Priority**: **MEDIUM** - Supports retry logic in Stage 4
- **Implementation notes**: Use `git revert` or `git apply -R`, update composer.json, handle conflicts

### 6. generate_report
- **Purpose**: Generate structured upgrade reports (JSON and Markdown)
- **Input Schema**:
  ```json
  {
    "project_path": "string (required)",
    "report_type": "string (optional: json|markdown|both, default both)",
    "include_scan_data": "boolean (optional, default true)",
    "include_patch_list": "boolean (optional, default true)",
    "include_pending_items": "boolean (optional, default true)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "success": "boolean",
    "json_report_path": "string (if generated)",
    "markdown_report_path": "string (if generated)",
    "summary": {
      "total_modules_checked": "number",
      "patches_applied": "number",
      "custom_files_fixed": "number",
      "errors_remaining": "number",
      "pending_human_review": "number"
    }
  }
  ```
- **Why needed**: The `report` package exists but there's no MCP tool to invoke it. Stage 7 needs to:
  - Generate comprehensive reports
  - Track what was done during the upgrade
  - List pending items for human review
  - Provide structured data for analysis
- **Priority**: **MEDIUM** - Required for Stage 7 (Report generation)
- **Implementation notes**: Wrap `report.GenerateJSON()` and `report.GenerateMarkdown()` with data collection

### 7. module_info
- **Purpose**: Get detailed information about a Drupal module
- **Input Schema**:
  ```json
  {
    "module_machine_name": "string (required)",
    "include_maintainers": "boolean (optional, default false)",
    "include_dependencies": "boolean (optional, default false)",
    "include_issue_stats": "boolean (optional, default false)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "module": "string",
    "title": "string",
    "description": "string",
    "maintainers": ["string"],
    "project_url": "string",
    "downloads": "number",
    "last_release": "string",
    "open_issues": "number",
    "dependencies": {
      "required": ["string"],
      "optional": ["string"]
    },
    "issue_stats": {
      "bug_count": "number",
      "task_count": "number",
      "feature_count": "number"
    }
  }
  ```
- **Why needed**: When deciding how to handle a problematic module, need context:
  - Is it actively maintained?
  - How many open issues (indicates health)?
  - What are its dependencies (may need upgrade too)?
  - Who maintains it (for contact/escalation)?
- **Priority**: **LOW** - Helpful for decision-making but not critical
- **Implementation notes**: Query Drupal.org API for module metadata, parse project page

### 8. drupal_version_matrix
- **Purpose**: Get compatibility matrix for Drupal versions
- **Input Schema**:
  ```json
  {
    "drupal_version": "string (optional)",
    "php_version": "string (optional)"
  }
  ```
- **Output Schema**:
  ```json
  {
    "drupal_version": "string",
    "php_requirements": {
      "minimum": "string",
      "recommended": "string"
    },
    "supported_until": "string",
    "upgrade_path": {
      "next_major": "string",
      "migration_guide_url": "string"
    },
    "known_issues": ["string"]
  }
  ```
- **Why needed**: Before starting an upgrade, need to verify:
  - PHP version compatibility
  - Support timeline (is target version still supported?)
  - Known issues with the upgrade path
  - Migration guide references
- **Priority**: **LOW** - Useful for preflight but not frequently used
- **Implementation notes**: Could be static data or fetched from Drupal.org release history

## Key Insights

### Current Tool Landscape
1. **Strong foundation**: The 7 existing MCP tools cover the core pipeline stages (scan, fix, check, patch, validate)
2. **Real implementations**: All tools have working implementations, not just placeholders
3. **Drupal.org integration**: Solid drupalorg package with release checking and patch searching
4. **Safety measures**: Patch application has URL allowlist and atomic operations
5. **Gap in execution tools**: No dedicated tools for composer or drush, forcing manual bash usage

### Critical Gaps Identified
1. **Composer execution**: The skill instructs manual `composer require` commands, which is error-prone and untracked
2. **Drush execution**: Manual drush commands bypass validation and structured output
3. **Upgrade path intelligence**: `contrib_check` only answers "is D11 supported?" but not "what version should I install?"
4. **Patch lifecycle management**: No way to check patch status or rollback failed patches
5. **Reporting**: Report package exists but no MCP tool to generate reports

### Recommendations

#### High Priority (Implement First)
1. **composer_require** - Most frequently needed, reduces errors in Stage 4
2. **drush_exec** - Used throughout pipeline, provides safety and structure
3. **contrib_upgrade_path** - Critical for intelligent upgrade decisions

#### Medium Priority (Implement Second)
4. **patch_status** - Improves robustness and resumability
5. **patch_rollback** - Enables retry logic
6. **generate_report** - Required for Stage 7 completion

#### Low Priority (Implement If Time)
7. **module_info** - Helpful context but not critical
8. **drupal_version_matrix** - Useful for preflight but rarely needed

### Implementation Strategy
1. Start with **composer_require** and **drush_exec** - they're straightforward wrappers with validation
2. Extend **drupalorg** package to support **contrib_upgrade_path** (fetch full release history)
3. Add **patch_status** and **patch_rollback** to improve patch lifecycle management
4. Wire **generate_report** to existing report package
5. Consider **module_info** and **drupal_version_matrix** as optional enhancements

### Testing Considerations
- All new tools should follow existing patterns (package-level vars for testability)
- Mock HTTP clients for Drupal.org API calls
- Test composer/drush execution with various exit codes
- Verify patch operations are truly atomic (rollback on failure)

## Ready for Proposal
**Yes** - The exploration is complete. The orchestrator should:
1. Review the 8 proposed tools and prioritize based on user needs
2. Decide which tools to implement in the first iteration
3. Create a proposal for the selected tools with implementation details
4. Consider splitting into multiple changes if scope is too large (e.g., "execution-tools" for composer_require + drush_exec, "upgrade-intelligence" for contrib_upgrade_path)
