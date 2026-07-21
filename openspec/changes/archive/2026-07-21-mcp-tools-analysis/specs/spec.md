# Spec: MCP Tools Analysis

## Overview

Adds 10 new MCP tools to the drup orchestrator across 4 phases: environment detection, safe command execution, upgrade intelligence, patch lifecycle, and reporting. All tools are additive — no existing tools are modified.

---

## 1. detect_env

### Purpose

Detect the Drupal development environment (ddev, lando, docker4drupal, direct) for a given project path and cache the result for all subsequent tool calls. This is the foundation tool — all command-execution tools depend on it for correct command prefixing.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "force_detect": {
      "type": "boolean",
      "description": "Bypass cache and re-detect environment",
      "default": false
    }
  },
  "required": ["project_path"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "environment": {
      "type": "string",
      "enum": ["ddev", "lando", "docker4drupal", "direct", "unknown"]
    },
    "command_prefix": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Command tokens to prepend (e.g. [\"ddev\"] or [])"
    },
    "detected_at": {
      "type": "string",
      "format": "date-time"
    }
  }
}
```

### Behavior

**Detection heuristics (priority order):**

1. `.ddev/` directory exists → `ddev`, prefix `["ddev"]`
2. `.lando.yml` file exists → `lando`, prefix `["lando"]`
3. `docker-compose.yml` exists AND contains `*drupal*` service reference → `docker4drupal`, prefix `["docker-compose", "exec", "drupal"]`
4. None of the above → `direct`, prefix `[]`
5. Path does not exist or is not a directory → `unknown`, prefix `[]`

**Caching strategy:**

- In-memory `map[string]EnvResult` keyed by `project_path`
- Cache entry stores `detected_at` timestamp
- `force_detect: true` bypasses cache and re-runs detection
- Cache invalidation on project_path mtime change: if the project root directory mtime is newer than `detected_at`, re-detect (handles environment config file changes)

**Error states:**

| Condition | Response |
|-----------|----------|
| `project_path` does not exist | Return `environment: "unknown"`, `command_prefix: []`, error message in output |
| `project_path` is not a directory | Return `environment: "unknown"` with error |
| Permission denied reading path | Return `environment: "unknown"` with error |
| Ambiguous markers (e.g. both `.ddev/` and `.lando.yml`) | First match wins per priority order above |

### Validation Rules

- `project_path` MUST be a non-empty absolute path
- `project_path` MUST exist and be a directory (return `unknown` if not, do not error fatally)

### Dependencies

- New package: `internal/envdetect/`
- No dependency on existing tools

---

## 2. upgrade_scan

### Purpose

One-call upgrade_status analysis that handles the full lifecycle: install upgrade_status via composer (if missing), enable it (if disabled), run analysis, and return filtered results. Eliminates the 3-4 bash calls agents currently need.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "scope": {
      "type": "string",
      "enum": ["env", "contrib", "custom", "theme"],
      "description": "Filter analysis to a specific category"
    },
    "module": {
      "type": "string",
      "description": "Analyze a single module by machine name"
    }
  },
  "required": ["project_path"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "total_errors": { "type": "number" },
    "modules": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "category": { "type": "string" },
          "errors": { "type": "number" },
          "warnings": { "type": "number" },
          "messages": { "type": "number" }
        }
      }
    },
    "upgrade_status_installed": { "type": "boolean" },
    "upgrade_status_enabled": { "type": "boolean" }
  }
}
```

### Behavior

**State machine:** `check_installed → install (if needed) → check_enabled → enable (if needed) → analyze → filter → return`

**Step-by-step flow:**

1. Read `composer.json` at `project_path` — check if `drupal/upgrade_status` is in `require` or `require-dev`
2. If NOT present: call `composer_require` with `package: "drupal/upgrade_status"`, `dev: true`
3. If composer_require fails → return error with stderr, abort
4. Run `drush_exec` with command `"pm:list --status=enabled --format=json"` — check if `upgrade_status` is in enabled modules
5. If NOT enabled: run `drush_exec` with command `"en upgrade_status -y"`
6. If enable fails → return error, abort
7. Build analyze command: `drush upgrade_status:analyze <module_or_all> --format=json`
8. If `scope` is set, filter output to matching category
9. Parse JSON output into structured result
10. Return aggregated result

**Idempotency:**

- If `upgrade_status` is already in `composer.json` → skip install step
- If `upgrade_status` is already enabled → skip enable step
- Repeated calls with same parameters produce same result without side effects

**Partial failure handling:**

| Failure point | Behavior |
|---------------|----------|
| composer_require fails (conflict/timeout) | Return error with `exit_code`, `stderr`; do NOT proceed to analyze |
| drush en fails | Return error with `exit_code`, `stderr`; do NOT proceed to analyze |
| analyze returns non-zero but has partial JSON | Parse available JSON, include warning about partial results |
| analyze returns empty (no errors) | Return `total_errors: 0`, empty modules list |

**Timeout:** Each sub-command (composer, drush) has a 60-second timeout. If exceeded, return error with timeout message.

### Validation Rules

- `project_path` MUST be a non-empty absolute path that exists
- `scope` if provided MUST be one of: `env`, `contrib`, `custom`, `theme`
- `module` and `scope` are mutually exclusive; if both provided, `module` takes precedence

### Dependencies

- `detect_env` (for command prefix resolution)
- `composer_require` (for installing upgrade_status)
- `drush_exec` (for enabling and analyzing)
- Existing `internal/scan` package (for JSON parsing and classification)

---

## 3. composer_require

### Purpose

Safe wrapper around `composer require` with input validation, dry-run pre-check for conflicts, timeout handling, and structured output parsing.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "package": {
      "type": "string",
      "description": "Composer package name with optional version constraint (e.g. 'drupal/token:^1.0')"
    },
    "dev": {
      "type": "boolean",
      "description": "Add as dev dependency (--dev flag)",
      "default": false
    },
    "no_update": {
      "type": "boolean",
      "description": "Skip composer update after require (--no-update flag)",
      "default": false
    }
  },
  "required": ["project_path", "package"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "success": { "type": "boolean" },
    "installed_version": { "type": "string" },
    "stdout": { "type": "string" },
    "stderr": { "type": "string" },
    "exit_code": { "type": "number" }
  }
}
```

### Behavior

**Flow:**

1. Validate `package` format (see Validation Rules)
2. Call `detect_env` for `project_path` to get command prefix
3. Run `composer require --dry-run <package>` with environment prefix
4. If dry-run fails (conflict detected) → return `{success: false}` with conflict details from stderr
5. Run actual `composer require <package>` with 60-second timeout
6. Parse stdout for installed version: look for pattern `Installing <vendor>/<package> (<version>)` or `Upgrading <vendor>/<package> (<version>)`
7. Return structured result

**Error states:**

| Condition | Behavior |
|-----------|----------|
| Package name format invalid | Return error before executing any command |
| Dry-run shows conflict | Return `{success: false}` with conflict details in stderr |
| Composer command timeout (>60s) | Kill process, return `{success: false}` with timeout error |
| Composer exits non-zero | Return `{success: false}` with exit_code and stderr |
| `composer.json` not found at project_path | Return error indicating not a composer project |
| Version already installed (no-op) | Return `{success: true}` with current version |

**Edge cases:**

- If `package` has no version constraint, composer resolves latest — parse installed version from output
- If package is already in `composer.json` at the same version, return success with current version (idempotent)

### Validation Rules

- `package` MUST match pattern: `^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9]([_.-]?[a-z0-9]+)*(:[a-zA-Z0-9^~<>=*. -]+)?$`
- `project_path` MUST contain a `composer.json` file
- `no_update` with `dev: true` is valid but unusual — allow it

### Dependencies

- `detect_env` (for command prefix)
- `internal/exec` (for OS command execution with timeout)

---

## 4. drush_exec

### Purpose

Safe drush execution wrapper with command blocklist, automatic environment-aware prefixing, `--root` flag injection, and structured output parsing.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "command": {
      "type": "string",
      "description": "Drush command to execute (e.g. 'pm:list', 'upgrade_status:analyze')"
    },
    "args": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Additional arguments for the command"
    },
    "format": {
      "type": "string",
      "enum": ["json", "table", "csv", "yaml"],
      "description": "Output format (--format flag); json triggers structured parsing"
    }
  },
  "required": ["project_path", "command"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "success": { "type": "boolean" },
    "output": {
      "description": "Parsed object if format=json, otherwise raw string",
      "type": ["object", "string"]
    },
    "stderr": { "type": "string" },
    "exit_code": { "type": "number" }
  }
}
```

### Behavior

**Command blocklist (MUST be rejected):**

| Command | Reason |
|---------|--------|
| `sql-drop` | Destructive — drops all database tables |
| `site-install` | Destructive — reinstalls site, loses data |
| `sql-sanitize` | Destructive — alters user data |
| `php-eval` | Arbitrary code execution risk |
| `core:execute-cli` | Arbitrary command execution |

If a blocked command is detected → return `{success: false}` with error `"command '<name>' is blocked for safety"`, exit_code: -1. Do NOT execute.

**Flow:**

1. Validate `command` against blocklist
2. Call `detect_env` for `project_path`
3. Build command: `[prefix...] drush <command> [args...] --root=<project_path>`
4. If `format` is specified, append `--format=<format>`
5. Execute with 60-second timeout
6. If `format == "json"`: attempt to parse stdout as JSON
   - If parse succeeds → `output` is the parsed object
   - If parse fails → `output` is raw string, include warning in stderr
7. Classify error:
   - exit_code 0 → success
   - exit_code 1 → command error (drush reported failure)
   - exit_code 126/127 → binary not found
   - exit_code 137 → timeout (killed)
   - Other → unexpected error

**Auto-prefix behavior:**

- If `detect_env` returns `ddev` → command becomes `ddev drush <cmd> --root=<path>`
- If `detect_env` returns `lando` → command becomes `lando drush <cmd> --root=<path>`
- If `detect_env` returns `direct` → command becomes `drush <cmd> --root=<path>` (or vendor/bin/drush if found)

### Validation Rules

- `command` MUST NOT be in the blocklist
- `command` MUST NOT contain shell metacharacters (`;`, `|`, `&&`, `||`, `$()`, backticks)
- `args` items MUST NOT contain shell metacharacters
- `project_path` MUST exist and contain a Drupal installation (has `web/` or `docroot/` or `composer.json` with `drupal/core`)

### Dependencies

- `detect_env` (for command prefix)
- `internal/exec` (for OS command execution with timeout)

---

## 5. contrib_upgrade_path

### Purpose

Resolve the recommended contrib module version for a target Drupal major version. Goes beyond `contrib_check` (which answers "is it compatible?") to answer "which version should I install for the upgrade?"

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "module_machine_name": {
      "type": "string",
      "description": "Drupal contrib module machine name"
    },
    "current_drupal_version": {
      "type": "string",
      "description": "Current Drupal major version (e.g. '10')"
    },
    "target_drupal_version": {
      "type": "string",
      "description": "Target Drupal major version (e.g. '11')"
    }
  },
  "required": ["module_machine_name", "current_drupal_version", "target_drupal_version"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "module": { "type": "string" },
    "current_version": { "type": "string" },
    "recommended_upgrade": {
      "type": "object",
      "properties": {
        "version": { "type": "string" },
        "drupal_compatibility": {
          "type": "array",
          "items": { "type": "string" }
        },
        "release_date": { "type": "string" },
        "is_stable": { "type": "boolean" }
      }
    },
    "alternative_versions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "version": { "type": "string" },
          "drupal_compatibility": {
            "type": "array",
            "items": { "type": "string" }
          },
          "is_stable": { "type": "boolean" }
        }
      }
    },
    "upgrade_notes": { "type": "string" }
  }
}
```

### Behavior

**Flow:**

1. Fetch release-history XML from `https://updates.drupal.org/release-history/<module_machine_name>/<target_drupal_version>`
2. If HTTP 404 → try fallback URL with `current_drupal_version` to find cross-compatible releases
3. Parse XML: extract all `<release>` elements with `<version>`, `<tag>`, `<status>`, `<release-date>`, `<terms>` (for Drupal version compatibility)
4. Filter releases:
   - MUST be compatible with `target_drupal_version` (check `<terms>` for Drupal version)
   - SHOULD prefer `status: "published"` (stable) releases
   - SHOULD prefer latest release by date
5. Select `recommended_upgrade`: latest stable release compatible with target
6. Populate `alternative_versions`: other compatible releases (stable and non-stable), sorted by date descending, max 5
7. Extract `upgrade_notes` from release notes if available

**Version comparison logic:**

- Parse Drupal contrib version format: `<drupal_major>.x-<module_version>` (e.g., `8.x-1.12`, `2.x-3.0`)
- Compare module versions semantically within the same Drupal major branch
- Cross-branch releases (e.g., `3.x-1.0` for D11 when D10 used `8.x-1.x`) are treated as new branches

**Stability filtering:**

- `status: "published"` → stable (`is_stable: true`)
- `status: "unstable"` or containing `-alpha`, `-beta`, `-rc` → `is_stable: false`
- Recommended upgrade SHOULD be stable; if no stable release exists for target, return latest unstable with `is_stable: false`

**Fallback chain:**

1. Target version release-history → found → use it
2. Target version returns 404 → try current version history, filter for releases also compatible with target
3. Both fail → return `{recommended_upgrade: null}` with error message

**Error states:**

| Condition | Behavior |
|-----------|----------|
| Module does not exist on Drupal.org | HTTP 404 on both URLs → return error `"module not found on Drupal.org"` |
| No releases compatible with target | Return `{recommended_upgrade: null, alternative_versions: []}` |
| Network timeout (>15s) | Return error with timeout message |
| Malformed XML | Return parse error with details |

### Validation Rules

- `module_machine_name` MUST match `^[a-z][a-z0-9_]*$`
- `current_drupal_version` and `target_drupal_version` MUST be valid Drupal major versions (numeric string, e.g. "9", "10", "11")
- `current_drupal_version` SHOULD be less than `target_drupal_version` (warn if not, but allow)

### Dependencies

- `internal/drupalorg` (extend `CheckRelease()` to fetch and parse full release-history XML)
- HTTP client (existing, with timeout configuration)

---

## 6. patch_status

### Purpose

Check whether a specific patch is already applied to the project, by inspecting `composer.json` `extra.patches` configuration and git history.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "patch_url": {
      "type": "string",
      "description": "URL of the patch to check"
    },
    "composer_package": {
      "type": "string",
      "description": "Composer package the patch applies to (e.g. 'drupal/token')"
    }
  },
  "required": ["project_path"],
  "oneOf": [
    { "required": ["patch_url"] },
    { "required": ["composer_package"] }
  ]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "is_applied": { "type": "boolean" },
    "commit_hash": { "type": "string" },
    "registered_in_composer": { "type": "boolean" },
    "patch_info": {
      "type": "object",
      "properties": {
        "url": { "type": "string" },
        "package": { "type": "string" },
        "description": { "type": "string" }
      }
    }
  }
}
```

### Behavior

**Flow:**

1. Read `composer.json` at `project_path`
2. Parse `extra.patches` (or `extra.patches-default`) section
3. If `composer_package` provided: look up all patches registered for that package
4. If `patch_url` provided: search all packages' patches for matching URL
5. URL matching: exact match first, then substring match (handles URL variations like `https://` vs `http://`, with/without trailing params)
6. If found in `composer.json` → set `registered_in_composer: true`
7. Search git log for patch-related commits:
   - Search for commit messages containing the patch URL or description
   - Search for commits that modified files in the package's directory
   - If found → set `commit_hash` to the matching commit SHA
8. Determine `is_applied`:
   - `true` if registered in composer AND found in git log
   - `true` if registered in composer (assumes composer patches were applied on install)
   - `false` if not registered and not in git log

**Edge cases:**

- `composer.json` has no `extra.patches` section → `registered_in_composer: false`
- Patch registered in composer but git log shows revert commit → `is_applied: false`
- Multiple patches for same package → return info for the matching one; if `patch_url` not specified, return first match for `composer_package`
- Git repository not initialized → skip git log check, rely on composer.json only

### Validation Rules

- `project_path` MUST contain a `composer.json` file
- At least one of `patch_url` or `composer_package` MUST be provided
- `patch_url` if provided MUST be a valid URL

### Dependencies

- None (reads `composer.json` directly, uses `git log` via `internal/exec`)

---

## 7. patch_rollback

### Purpose

Cleanly revert a previously applied patch: git-revert the patch commit, remove the entry from `composer.json` `extra.patches`, and run `composer update` for the affected package to restore consistency.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "patch_url": {
      "type": "string",
      "description": "URL of the patch to revert"
    },
    "composer_package": {
      "type": "string",
      "description": "Composer package the patch was applied to"
    }
  },
  "required": ["project_path", "patch_url", "composer_package"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "success": { "type": "boolean" },
    "reverted_commit": { "type": "string" },
    "removed_from_composer": { "type": "boolean" },
    "error": { "type": "string" }
  }
}
```

### Behavior

**Flow:**

1. Call `patch_status` to verify the patch is applied and get `commit_hash`
2. If `is_applied: false` → return `{success: false, error: "patch is not applied"}`
3. If `commit_hash` is empty → return `{success: false, error: "cannot find patch commit to revert"}`
4. Run `git revert <commit_hash> --no-edit` in `project_path`
5. If git revert fails (conflict) → return `{success: false, error: "revert conflict: <details>"}`, do NOT modify composer.json
6. If git revert succeeds → set `reverted_commit` to the new revert commit SHA
7. Read `composer.json`, remove the matching entry from `extra.patches.<composer_package>`
8. If the package has no remaining patches → remove the package key entirely from `extra.patches`
9. Write updated `composer.json`
10. Run `composer update <composer_package>` to restore package state
11. If composer update fails → return `{success: true, removed_from_composer: true, error: "warning: composer update failed, manual intervention needed"}`
12. Return `{success: true}`

**Safety checks:**

- MUST verify patch is applied before attempting revert (step 1-2)
- MUST NOT modify `composer.json` if git revert fails (atomic: revert first, then update config)
- MUST check for uncommitted changes before git revert — if working tree is dirty, return error asking user to commit/stash first

**Dependency consistency:**

- After removing patch from `composer.json`, the `composer update` call ensures the package is re-downloaded without the patch
- If other patches exist for the same package, they remain in `composer.json` and are re-applied by composer

### Validation Rules

- All three fields are required
- `project_path` MUST be a git repository
- `patch_url` MUST be a valid URL
- `composer_package` MUST exist in `composer.json` `require` or `require-dev`

### Dependencies

- `patch_status` (to verify patch state and find commit hash)
- `internal/exec` (for git and composer commands)

---

## 8. generate_report

### Purpose

Generate upgrade reports in JSON and/or Markdown format by wrapping the existing `internal/report` package.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "project_path": {
      "type": "string",
      "description": "Absolute path to the Drupal project root"
    },
    "report_type": {
      "type": "string",
      "enum": ["json", "markdown", "both"],
      "default": "both",
      "description": "Report format to generate"
    },
    "include_scan_data": {
      "type": "boolean",
      "default": true,
      "description": "Include upgrade_status scan results in report"
    },
    "include_patch_list": {
      "type": "boolean",
      "default": true,
      "description": "Include list of applied patches in report"
    }
  },
  "required": ["project_path"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "success": { "type": "boolean" },
    "json_report_path": { "type": "string" },
    "markdown_report_path": { "type": "string" },
    "summary": {
      "type": "object",
      "properties": {
        "total_modules_checked": { "type": "number" },
        "patches_applied": { "type": "number" },
        "errors_remaining": { "type": "number" }
      }
    }
  }
}
```

### Behavior

**Flow:**

1. Collect data from internal state (scan results, patch list, project metadata)
2. If `include_scan_data: true` → include classified error data from scan package
3. If `include_patch_list: true` → include patches from `composer.json` `extra.patches`
4. If `report_type` is `json` or `both`:
   - Call `report.GenerateJSON()` with collected data
   - Write to `<project_path>/drup-report.json`
   - Set `json_report_path`
5. If `report_type` is `markdown` or `both`:
   - Call `report.GenerateMarkdown()` with collected data
   - Write to `<project_path>/drup-report.md`
   - Set `markdown_report_path`
6. Compute summary statistics
7. Return result with file paths and summary

**File output locations:**

- JSON: `<project_path>/drup-report.json`
- Markdown: `<project_path>/drup-report.md`
- If files already exist → overwrite (reports are snapshots, not incremental)

### Validation Rules

- `project_path` MUST exist and be writable
- `report_type` MUST be one of: `json`, `markdown`, `both`

### Dependencies

- `internal/report` (existing package — `GenerateJSON()`, `GenerateMarkdown()`)
- `internal/scan` (for scan data if `include_scan_data: true`)

---

## 9. module_info

### Purpose

Fetch module metadata and health indicators from Drupal.org for decision support when evaluating problematic modules.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "module_machine_name": {
      "type": "string",
      "description": "Drupal module machine name"
    },
    "include_maintainers": {
      "type": "boolean",
      "default": false,
      "description": "Include maintainer usernames"
    },
    "include_dependencies": {
      "type": "boolean",
      "default": false,
      "description": "Include module dependency list"
    }
  },
  "required": ["module_machine_name"]
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "module": { "type": "string" },
    "title": { "type": "string" },
    "maintainers": {
      "type": "array",
      "items": { "type": "string" }
    },
    "downloads": { "type": "number" },
    "last_release": { "type": "string" },
    "open_issues": { "type": "number" },
    "dependencies": {
      "type": "object",
      "properties": {
        "required": {
          "type": "array",
          "items": { "type": "string" }
        }
      }
    }
  }
}
```

### Behavior

**Flow:**

1. Query Drupal.org node API: `https://www.drupal.org/api-d7/node.json?name=<module_machine_name>`
2. Parse response: extract `title`, `field_download_count` (downloads), `maintainers`
3. If `include_maintainers: false` → omit or return empty array
4. Fetch latest release from release-history XML (reuse logic from `contrib_upgrade_path`)
5. If `include_dependencies: true`:
   - Parse `.info.yml` from release or API response for dependency list
   - Separate required vs optional dependencies
6. Fetch open issue count from `https://www.drupal.org/project/issues/<module>?version=All&status=1` (parse count from response or use API)
7. Return structured result

**Caching considerations:**

- Module metadata changes infrequently; cache in-memory for 1 hour
- Cache key: `module_info:<module_machine_name>`
- No explicit invalidation needed (TTL-based expiry)

**Error states:**

| Condition | Behavior |
|-----------|----------|
| Module not found on Drupal.org | Return error `"module '<name>' not found"` |
| API timeout (>10s) | Return error with timeout message |
| Partial data (e.g. downloads unavailable) | Return available fields, omit unavailable ones |

### Validation Rules

- `module_machine_name` MUST match `^[a-z][a-z0-9_]*$`

### Dependencies

- `internal/drupalorg` (extend for node API queries)
- HTTP client with timeout

---

## 10. drupal_version_matrix

### Purpose

Provide a Drupal/PHP version compatibility matrix for preflight validation. Static data for fast lookup with no external dependencies.

### Input Schema

```json
{
  "type": "object",
  "properties": {
    "drupal_version": {
      "type": "string",
      "description": "Drupal major version to query (e.g. '10', '11')"
    },
    "php_version": {
      "type": "string",
      "description": "PHP version to check compatibility for (e.g. '8.2', '8.3')"
    }
  },
  "properties_note": "At least one of drupal_version or php_version SHOULD be provided; if neither, return full matrix"
}
```

### Output Schema

```json
{
  "type": "object",
  "properties": {
    "drupal_version": { "type": "string" },
    "php_requirements": {
      "type": "object",
      "properties": {
        "minimum": { "type": "string" },
        "recommended": { "type": "string" }
      }
    },
    "supported_until": { "type": "string" },
    "upgrade_path": {
      "type": "object",
      "properties": {
        "next_major": { "type": "string" }
      }
    }
  }
}
```

### Behavior

**Static data source:**

The system SHALL maintain an internal map of Drupal version data:

| Drupal | PHP Min | PHP Recommended | Supported Until | Next Major |
|--------|---------|-----------------|-----------------|------------|
| 9 | 7.3 | 8.1 | 2024-06 | 10 |
| 10 | 8.1 | 8.3 | 2026-06 | 11 |
| 11 | 8.3 | 8.4 | TBA | — |

**Flow:**

1. If `drupal_version` provided → look up in static map, return matching row
2. If `php_version` provided (without `drupal_version`) → find all Drupal versions compatible with that PHP version, return the latest
3. If neither provided → return full matrix as array
4. If `drupal_version` not in map → return error `"unknown Drupal version: <version>"`

**Update mechanism:**

- Static map is hardcoded in `internal/envdetect/version_matrix.go` (or similar)
- Updated via code change when new Drupal versions are released
- No runtime update mechanism needed (release cadence is low)

### Validation Rules

- `drupal_version` if provided MUST be a numeric string
- `php_version` if provided MUST match pattern `^\d+\.\d+$`

### Dependencies

- None (pure static data lookup)

---

## MODIFIED: mcp-server (Delta)

### Requirement: New Tool Registration

The system SHALL register 10 new MCP tool handlers in addition to the existing 7 tools.

(Previously: 7 tools registered — scan, autofix, contrib_check, issue_patches, apply_patch, validate, create_patch)

#### Scenario: All 10 new tools are callable

- GIVEN the MCP server starts with the new handlers registered
- WHEN an agent calls any of: `detect_env`, `upgrade_scan`, `composer_require`, `drush_exec`, `contrib_upgrade_path`, `patch_status`, `patch_rollback`, `generate_report`, `module_info`, `drupal_version_matrix`
- THEN the system SHALL route the call to the correct handler and return a valid JSON-RPC response

#### Scenario: New tools validate input schemas

- GIVEN a tool call to any new tool with missing required parameters
- THEN the system SHALL return a JSON-RPC error with code -32602 (invalid params) before executing the handler

#### Scenario: Existing tools unchanged

- GIVEN the 10 new tools are registered
- WHEN an agent calls any of the original 7 tools
- THEN the behavior SHALL be identical to before this change

### Requirement: Tool Handler Registration Points

The system SHALL register new tool handlers in both `internal/mcp/tools.go` (placeholder/schema definitions) and `internal/app/mcp_tools.go` (real handler wiring).

#### Scenario: Schema definitions

- GIVEN the MCP server initialization
- WHEN tool schemas are built for the `tools/list` response
- THEN all 10 new tools SHALL appear with correct input schemas as defined above

#### Scenario: Handler wiring

- GIVEN a tool call arrives for a new tool
- WHEN the handler dispatch runs
- THEN the system SHALL invoke the correct internal package function

---

## Coverage Summary

| Tool | Happy Path | Edge Cases | Error States |
|------|-----------|------------|--------------|
| detect_env | ✅ 4 env types | ✅ ambiguous markers | ✅ missing path, permissions |
| upgrade_scan | ✅ full lifecycle | ✅ idempotent re-runs | ✅ partial failure at each step |
| composer_require | ✅ install + version parse | ✅ already installed | ✅ conflict, timeout, format |
| drush_exec | ✅ execute + parse | ✅ json parse fallback | ✅ blocklist, timeout, shell injection |
| contrib_upgrade_path | ✅ version resolution | ✅ no stable release | ✅ 404, timeout, malformed XML |
| patch_status | ✅ applied + registered | ✅ revert detected | ✅ no patches section, no git |
| patch_rollback | ✅ revert + cleanup | ✅ dirty working tree | ✅ revert conflict, composer fail |
| generate_report | ✅ both formats | ✅ overwrite existing | ✅ unwritable path |
| module_info | ✅ full metadata | ✅ partial data | ✅ not found, timeout |
| drupal_version_matrix | ✅ lookup by version | ✅ no filter provided | ✅ unknown version |
