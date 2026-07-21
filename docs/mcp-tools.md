# drup MCP Tools

The `drup` binary exposes **17 MCP tools** over stdio (JSON-RPC 2.0). These tools cover the full Drupal upgrade pipeline: environment detection, deprecation scanning, automatic fixing, contrib module management, patching, custom code refactoring, validation, and reporting.

## Tool Index

### Core Pipeline Tools

| # | Tool | Phase | Purpose |
|---|------|-------|---------|
| 1 | `scan` | Analysis | Run `upgrade_status:analyze` |
| 2 | `autofix` | Fix | Run `drupal-rector` on custom code |
| 3 | `contrib_check` | Research | Check Drupal.org for releases |
| 4 | `issue_patches` | Research | Search Drupal.org issues for patches |
| 5 | `apply_patch` | Fix | Download and apply a .patch file |
| 6 | `validate` | Validation | Re-run analysis with scope filtering |
| 7 | `create_patch` | Fix | Generate a .patch from deprecation analysis |

### Foundation Tools

| # | Tool | Phase | Purpose |
|---|------|-------|---------|
| 8 | `detect_env` | Preflight | Detect ddev/lando/docker4drupal/direct |
| 9 | `composer_require` | Execution | Safe `composer require` with dry-run |
| 10 | `drush_exec` | Execution | Safe drush execution with blocklist |

### Upgrade Intelligence

| # | Tool | Phase | Purpose |
|---|------|-------|---------|
| 11 | `upgrade_scan` | Analysis | Atomic install→enable→analyze→filter |
| 12 | `contrib_upgrade_path` | Research | Recommended version for next Drupal major |

### Patch Lifecycle

| # | Tool | Phase | Purpose |
|---|------|-------|---------|
| 13 | `patch_status` | Validation | Check if a patch is already applied |
| 14 | `patch_rollback` | Rollback | Revert a failed patch cleanly |

### Reporting & Info

| # | Tool | Phase | Purpose |
|---|------|-------|---------|
| 15 | `generate_report` | Report | Generate JSON + Markdown reports |
| 16 | `module_info` | Research | Module metadata from Drupal.org |
| 17 | `drupal_version_matrix` | Preflight | Drupal/PHP compatibility lookup |

---

## Tool Details

### 1. `scan`

Run `drush upgrade_status:analyze --format=json` and return classified errors.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project"
}
```

**Output:**
```json
{
  "project_path": "/path/to/drupal-project",
  "total_errors": 42,
  "modules": [
    {
      "name": "webform",
      "type": "contrib",
      "errors": [
        {
          "file": "modules/contrib/webform/webform.module",
          "line": 123,
          "message": "Call to deprecated function...",
          "rule": "DrupalDeprecation",
          "severity": "error",
          "source": "webform"
        }
      ]
    }
  ]
}
```

**Error states:** `upgrade_status` not installed, drush not found, parse failure.

---

### 2. `autofix`

Run `drupal-rector` with D11 rule sets on custom modules and themes.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project"
}
```

**Output:**
```json
{
  "rector_summary": "Applied 3 rector rules to 2 files",
  "remaining_errors": 0
}
```

**Error states:** rector not installed, file permission denied, process timeout.

---

### 3. `contrib_check`

Check if a module has a Drupal 11 compatible release.

**Input:**
```json
{
  "module_machine_name": "webform"
}
```

**Output:**
```json
{
  "module": "webform",
  "has_d11_release": true,
  "latest_version": "6.2.0",
  "compatible_branches": ["6.x"]
}
```

**Error states:** module not found on Drupal.org, release-history XML unavailable.

---

### 4. `issue_patches`

Search Drupal.org issues for patches, prioritizing RTBC (Reviewed & Tested by the Community).

**Input:**
```json
{
  "issue_nid": "3412345",
  "module_name": "webform"
}
```

**Output:**
```json
[
  {
    "url": "https://www.drupal.org/files/issues/2025-01-15/webform-d11.patch",
    "status": "RTBC",
    "date": "2025-01-15",
    "is_patch": true,
    "issue_nid": "3412345"
  }
]
```

**Error states:** no patches found, Drupal.org API unavailable, invalid issue NID.

---

### 5. `apply_patch`

Download a .patch from Drupal.org, apply it via `git apply`, register in `composer.json` under `extra.patches`.

**Input:**
```json
{
  "patch_url": "https://www.drupal.org/files/issues/patch.patch",
  "project_path": "/path/to/drupal-project"
}
```

**Output:**
```json
{
  "applied": true,
  "commit_hash": "abc123def456",
  "error": ""
}
```

**Security:** URL restricted to `drupal.org` domains. Atomic operation with rollback on failure.

**Error states:** URL not allowed, download fails, git apply conflict, composer.json malformed.

---

### 6. `validate`

Re-run `upgrade_status:analyze` with optional scope and module/file filtering.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "scope": "contrib",
  "module": "webform",
  "file": ""
}
```

**Output:**
```json
{
  "total_errors": 0,
  "errors": []
}
```

**Scopes:** `env`, `contrib`, `custom`, `theme`, `global`, `rector`.

---

### 7. `create_patch`

Run rector on a specific module, generate a git diff, and write it to a temporary .patch file.

**Input:**
```json
{
  "module_name": "webform",
  "deprecation_details": "Drupal\\Core\\Architecture\\SomeClass is deprecated"
}
```

**Output:**
```json
{
  "patch_path": "/tmp/drup-patch-12345.patch",
  "applied": true
}
```

**Limitations:** Currently generates patch via rector run + git diff, does not apply the patch automatically. Does not intelligently use `deprecation_details`.

---

### 8. `detect_env`

Detect the local Drupal development environment and cache the result. This is the **foundation tool** — all command-execution tools depend on it for correct command prefixing.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "force_detect": false
}
```

**Output:**
```json
{
  "environment": "ddev",
  "command_prefix": ["ddev"],
  "detected_at": "2025-07-21T12:00:00Z"
}
```

**Detection order:**

1. `.ddev/` directory exists → `ddev`
2. `.lando.yml` file exists → `lando`
3. `docker-compose.yml` mentions drupal → `docker4drupal`
4. `composer.json` exists → `direct`
5. None of the above → `unknown`

**Caching:** In-memory map keyed by `project_path`. Cache is invalidated by:
- `force_detect: true` in input
- Project directory mtime changes

**When `unknown`:** The tool returns with empty prefix. The orchestrator agent should ask the developer or raise a config prompt.

---

### 9. `composer_require`

Execute `composer require` safely with validation, dry-run pre-check, and structured output.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "package": "drupal/webform:^6.2",
  "dev": false,
  "no_update": false
}
```

**Output:**
```json
{
  "success": true,
  "installed_version": "6.2.0",
  "stdout": "  - Installing drupal/webform (6.2.0): Loading from cache",
  "stderr": "",
  "exit_code": 0
}
```

**Validation:**
- Package name format (`vendor/package` or `vendor/package:^version`)
- Dry-run first (`composer require --dry-run`) to detect conflicts
- 60-second timeout

**Dependencies:** Uses `detect_env` for command prefix (e.g., `ddev composer require`).

**Error states:** Invalid package name, dependency conflict (caught by dry-run), timeout, composer not found.

---

### 10. `drush_exec`

Execute drush commands safely with a blocklist of dangerous operations.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "command": "pm:list",
  "args": ["--type=module"],
  "format": "json"
}
```

**Output:**
```json
{
  "success": true,
  "output": [
    {"name": "webform", "status": "enabled"}
  ],
  "stderr": "",
  "exit_code": 0
}
```

**Blocklist (rejected commands):**
- `sql-drop`
- `site-install`
- `sql-sanitize`
- `php-eval`
- `core:execute-cli`

**Security:** Rejects arguments containing shell metacharacters (`;`, `|`, `` ` ``, `$()`).

**Dependencies:** Uses `detect_env` for command prefix (e.g., `ddev drush`).

**Error states:** Command blocked, shell metacharacters detected, drush not found, non-zero exit.

---

### 11. `upgrade_scan`

Atomic one-call upgrade_status analysis: install if missing → enable if disabled → analyze → filter.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "scope": "contrib",
  "module": "webform"
}
```

**Output:**
```json
{
  "total_errors": 3,
  "modules": [
    {
      "name": "webform",
      "type": "contrib",
      "errors": [...]
    }
  ],
  "upgrade_status_installed": true,
  "upgrade_status_enabled": true
}
```

**Flow:**

1. Check if `drupal/upgrade_status` is in `composer.json`
2. If missing → `composer require drupal/upgrade_status` (via `composer_require`)
3. Check if module is enabled → `drush pm:list --type=module --format=json`
4. If disabled → `drush en upgrade_status -y` (via `drush_exec`)
5. Run `drush upgrade_status:analyze --format=json`
6. Filter results by scope/module if specified

**Per-step isolation:** If any step fails (install, enable), the tool reports the failure without proceeding. Each step's failure has a distinct error message.

**Dependencies:** `detect_env`, `composer_require`, `drush_exec`.

**Error states:** Composer install fails, drush enable fails, analyze crashes, filtered result is empty.

---

### 12. `contrib_upgrade_path`

Find the recommended contrib module version for the NEXT Drupal major version. Unlike `contrib_check` (which answers "is it compatible?"), this answers "which version should I install?"

**Input:**
```json
{
  "module_machine_name": "webform",
  "current_drupal_version": "10.3.0",
  "target_drupal_version": "11.0.0"
}
```

**Output:**
```json
{
  "module": "webform",
  "current_version": "6.1.0",
  "recommended_upgrade": {
    "version": "6.2.0",
    "drupal_compatibility": ["11.0.0"],
    "release_date": "2025-06-15",
    "is_stable": true
  },
  "alternative_versions": [
    {
      "version": "6.3.0-dev",
      "drupal_compatibility": ["11.0.0"],
      "release_date": "2025-07-01",
      "is_stable": false
    }
  ],
  "upgrade_notes": ""
}
```

**Implementation:** Fetches full release-history XML from `updates.drupal.org/release-history/{module}`, parses all branches, filters by Drupal compatibility, prefers latest stable.

**Fallback chain:** Target URL → current URL → null (all failed = "no compatible version").

---

### 13. `patch_status`

Check if a specific patch is already applied to the project.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "patch_url": "https://www.drupal.org/files/issues/patch.patch",
  "composer_package": "drupal/webform"
}
```

**Output:**
```json
{
  "is_applied": true,
  "commit_hash": "abc123def456",
  "registered_in_composer": true,
  "patch_info": {
    "url": "https://www.drupal.org/files/issues/patch.patch",
    "package": "drupal/webform"
  }
}
```

**Checks:**
1. `composer.json` `extra.patches` section (URL matching)
2. Git log for patch-related commits

---

### 14. `patch_rollback`

Revert a previously applied patch cleanly.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "patch_url": "https://www.drupal.org/files/issues/patch.patch",
  "composer_package": "drupal/webform"
}
```

**Output:**
```json
{
  "success": true,
  "reverted_commit": "abc123def456",
  "removed_from_composer": true,
  "error": ""
}
```

**Atomic order:**
1. Revert the patch commit via `git revert`
2. Remove patch entry from `composer.json` `extra.patches`
3. Run `composer update` for that package to restore original code

**Safety:** Fails early if git working tree is dirty or directory is not a git repository.

---

### 15. `generate_report`

Generate structured upgrade reports in JSON and/or Markdown format.

**Input:**
```json
{
  "project_path": "/path/to/drupal-project",
  "report_type": "both",
  "include_scan_data": true,
  "include_patch_list": true,
  "include_pending_items": true
}
```

**Output:**
```json
{
  "success": true,
  "json_report_path": "/path/to/drupal-project/drup-report.json",
  "markdown_report_path": "/path/to/drupal-project/drup-report.md",
  "summary": {
    "total_modules_checked": 15,
    "patches_applied": 3,
    "custom_files_fixed": 5,
    "errors_remaining": 2,
    "pending_human_review": 1
  }
}
```

**Implementation:** Wraps the existing `internal/report` package functions `GenerateJSON()` and `GenerateMarkdown()`.

---

### 16. `module_info`

Get detailed metadata about a Drupal module from Drupal.org.

**Input:**
```json
{
  "module_machine_name": "webform",
  "include_maintainers": true,
  "include_dependencies": false
}
```

**Output:**
```json
{
  "module": "webform",
  "title": "Webform",
  "description": "Enables the creation of forms and questionnaires.",
  "maintainers": ["jrockowitz", "laryn"],
  "project_url": "https://www.drupal.org/project/webform",
  "downloads": 2500000,
  "last_release": "6.2.0",
  "open_issues": 342,
  "dependencies": {
    "required": [],
    "optional": []
  }
}
```

**Data sources:**
- `api-d7/node.json?name={module}` for metadata and maintainers
- Release-history XML for latest release info

---

### 17. `drupal_version_matrix`

Look up Drupal and PHP version compatibility.

**Input:**
```json
{
  "drupal_version": "10",
  "php_version": ""
}
```

**Output:**
```json
{
  "drupal_version": "10",
  "php_requirements": {
    "minimum": "8.1",
    "recommended": "8.3"
  },
  "supported_until": "2026",
  "upgrade_path": {
    "next_major": "11",
    "migration_guide_url": "https://www.drupal.org/docs/upgrading-drupal"
  },
  "known_issues": []
}
```

**Data source:** Static map (fast, no external calls):

| Drupal | PHP Min | PHP Rec | Supported Until | Next Major |
|--------|---------|---------|----------------|------------|
| 9 | 7.3 | 8.1 | 2023-11 | 10 |
| 10 | 8.1 | 8.3 | 2026 | 11 |
| 11 | 8.3 | 8.4 | 2028 | 12 |

---

## Tool Dependencies

```
detect_env (foundation)
  ├── composer_require
  │     └── upgrade_scan
  ├── drush_exec
  │     └── upgrade_scan
  └── (all env-aware commands)

drupalorg (package)
  ├── contrib_check (existing)
  ├── contrib_upgrade_path (new)
  └── module_info (new)

report (package)
  └── generate_report
```

## Security Model

| Tool | Protection | Mechanism |
|------|-----------|-----------|
| `apply_patch` | URL allowlist | Only `drupal.org` domains |
| `drush_exec` | Command blocklist | 5 dangerous commands rejected |
| `drush_exec` | Shell injection | Metacharacter detection (`;`, `\|`, `` ` ``, `$()`) |
| `composer_require` | Conflict detection | `--dry-run` pre-check |
| `composer_require` | Package validation | `vendor/package` format check |
| `patch_rollback` | Dirty tree guard | Rejects if uncommitted changes exist |
| `upgrade_scan` | Path traversal | Rejects `..` segments in project_path |

## Testing

Each tool has table-driven unit tests. Mock external dependencies:
- **HTTP:** `httptest.Server` for Drupal.org API calls
- **Commands:** Package-level `execCommand` variable for subprocess mocking
- **Filesystem:** `t.TempDir()` for environment detection and patch tests
- **Git:** `git init` + `git add` + `git commit` in temp dirs for patch lifecycle tests

Run all tests:

```bash
go test ./... -count=1
```

Run security (RED) tests specifically:

```bash
go test ./internal/app/... -run "RED|Shell|Blocklist|Traversal"
```
