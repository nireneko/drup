# Delta Spec: drup-retrospective-fixes

Six fixes for bugs found during a real Drupal 10→11 upgrade. Priority order P0→P2.

---

## MODIFIED Requirements — scan

### Requirement: Drush Invocation

The system SHALL invoke `drush upgrade_status:analyze` with the `--all` flag. The system SHALL use `--root=<path>` instead of `-r <path>` for DDEV compatibility. The system SHALL use `RunWithEnv` with environment detection to prefix commands (e.g. `ddev`) when a container environment is detected.

| Req | Strength | Behavior |
|-----|----------|----------|
| Root flag | MUST | Use `--root=<path>` instead of `-r <path>` |
| Env detection | MUST | Call `envdetect.Detect()` and use `RunWithEnv(prefix, ...)` |
| Exit code 3 | MUST | Treat exit code 3 as success-with-findings, not error |
| Exit code 1,2,>3 | MUST | Treat as real errors and abort |
| Stdout on exit 3 | MUST | Parse stdout regardless of exit code 3 |
| Empty stdout + exit 3 | MUST | Treat as error (drush crashed, not findings) |

(Previously: used `-r <path>`, no env detection, any non-zero exit was error)

#### Scenario: Scan with findings under DDEV

- GIVEN a DDEV project with deprecations
- WHEN `drup scan /path` runs
- THEN the system SHALL detect DDEV, run `ddev exec drush --root=/var/www/html upgrade_status:analyze --all`, parse stdout, and return exit 0 with findings

#### Scenario: Scan with no findings

- GIVEN a clean project
- WHEN `drup scan` runs
- THEN the system SHALL return exit 0 with zero-error model

#### Scenario: Scan with drush crash

- GIVEN drush exits 3 with empty stdout and error on stderr
- WHEN `drup scan` runs
- THEN the system SHALL treat this as an error and report stderr content

---

## MODIFIED Requirements — validation-gates

### Requirement: Phase Gating

The system SHALL NOT advance to the next pipeline phase until all items in the current phase pass validation. Exit code 3 from `upgrade_status:analyze` SHALL be treated as success-with-findings (not error). The validator SHALL parse stdout on exit 3 and only advance when `total_errors == 0`.

(Previously: exit code 3 from validate was treated as failure, blocking pipeline advancement)

#### Scenario: Phase complete with exit code 3

- GIVEN `validate` returns exit code 3 with parseable findings
- WHEN the orchestrator checks phase completion
- THEN the system SHALL parse findings, count errors, and proceed only if `total_errors == 0`

#### Scenario: Validate scoped to module under DDEV

- GIVEN a DDEV project and `validate({module_name: "mymodule"})`
- WHEN validation runs
- THEN the system SHALL use `RunWithEnv` with `--root=` and handle exit code 3 correctly

---

## MODIFIED Requirements — mcp-server

### Requirement: Tool Schema Validation

The system SHALL expose complete JSON Schema `inputSchema` for all 20 tools in the `tools/list` response. Each tool's schema SHALL declare `properties` (with name, type, description) and `required` fields. The system SHALL validate all tool inputs against their JSON schemas before execution.

| Req | Strength | Behavior |
|-----|----------|----------|
| Schema properties | MUST | Each tool MUST declare `properties` with parameter definitions |
| Required fields | MUST | Each tool MUST declare `required` array for mandatory params |
| No empty schemas | MUST NOT | Return `{"type": "object"}` with no properties |

(Previously: all 20 tools returned `inputSchema: {"type": "object"}` with empty properties)

#### Scenario: Agent discovers scan parameters

- GIVEN an agent calls `tools/list`
- WHEN the response is received
- THEN the `scan` tool schema SHALL include `properties: {project_path: {type: "string", description: "..."}}` and `required: ["project_path"]`

#### Scenario: All 20 tools have schemas

- GIVEN the MCP server starts
- WHEN `tools/list` is called
- THEN all 20 tools SHALL have non-empty `inputSchema.properties`

---

## MODIFIED Requirements — contrib-check

### Requirement: Release History Lookup

The system SHALL fetch and parse Drupal.org release-history XML to determine D11 compatibility. The system SHALL parse `core_version_requirement` from release metadata and support compound constraints with `||` operators (e.g. `^10.3 || ^11.0`).

| Req | Strength | Behavior |
|-----|----------|----------|
| Compound constraints | MUST | Parse `||` in `core_version_requirement` |
| Semver matching | MUST | Use semver comparison, not string comparison |
| Fallback on parse failure | SHOULD | Fall back to string match if semver parse fails |

(Previously: only checked for literal "Drupal 11" in compatibility terms; no compound constraint support)

#### Scenario: Module with compound constraint

- GIVEN webform 6.3.0 with `core_version_requirement: "^10.3 || ^11.0"`
- WHEN `contrib_check({module_machine_name: "webform"})` runs
- THEN the system SHALL return `{has_d11_release: true}` (matches `^11.0`)

#### Scenario: Module with single constraint

- GIVEN a module with `core_version_requirement: "^11.0"`
- WHEN contrib_check runs
- THEN the system SHALL return `{has_d11_release: true}`

#### Scenario: Module incompatible with target

- GIVEN a module with `core_version_requirement: "^9.0 || ^10.0"`
- WHEN checking D11 compatibility
- THEN the system SHALL return `{has_d11_release: false}`

---

## MODIFIED Requirements — preflight

### Requirement: Dev Dependency Installation

The system SHALL install required dev dependencies if not present. The system SHALL detect PHP 8.4+ and auto-patch `settings.php` to suppress `E_DEPRECATED` after the DDEV include block.

| Req | Strength | Behavior |
|-----|----------|----------|
| PHP 8.4 detection | MUST | Run `php -v` (or env-equivalent) and parse version |
| Auto-patch settings.php | MUST | Append `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` |
| Patch placement | MUST | Insert after DDEV include block (or at end if no DDEV) |
| Backup before patch | SHOULD | Create `settings.php.bak` before modifying |
| Idempotent | MUST | Skip patch if suppression line already present |

(Previously: no PHP version-specific handling; no settings.php patching)

#### Scenario: PHP 8.4 project under DDEV

- GIVEN a DDEV project running PHP 8.4
- WHEN `drup preflight` runs
- THEN the system SHALL detect PHP 8.4, append deprecation suppression to `settings.php` after the DDEV include, and proceed

#### Scenario: PHP 8.3 project

- GIVEN a project running PHP 8.3
- WHEN preflight runs
- THEN the system SHALL skip the settings.php patch and proceed normally

#### Scenario: Patch already applied

- GIVEN `settings.php` already contains the deprecation suppression line
- WHEN preflight runs
- THEN the system SHALL detect the existing line and skip patching

---

## MODIFIED Requirements — report

### Requirement: JSON Report Generation

The system SHALL generate a JSON report containing all upgrade results with real scan data. The system SHALL collect actual error counts from `scan.ParseCodeclimateJSON()` or equivalent instead of hardcoded zeros.

| Req | Strength | Behavior |
|-----|----------|----------|
| Real error data | MUST | Populate `total_errors` from actual scan results |
| Real error list | MUST | Populate `resolved` and `pending` from scan data |
| No hardcoded zeros | MUST NOT | Use `0` only when scan confirms zero findings |

(Previously: `TotalErrors: 0` hardcoded, empty slices for resolved/pending)

#### Scenario: Report after scan with findings

- GIVEN a project with 15 deprecation errors
- WHEN `drup report` runs
- THEN the system SHALL output JSON with `total_errors: 15` and populated error arrays

#### Scenario: Report with no scan data available

- GIVEN no prior scan has been run
- WHEN `drup report` runs
- THEN the system SHALL run a scan first, then generate the report with real data
