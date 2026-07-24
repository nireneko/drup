# Delta Spec: drup-mejoras-post-retrospectiva

## Overview

12 improvements from a real D10→D11 upgrade retrospective. Organized by priority: P1 (pipeline correctness), P2 (robustness), P2/P3 (skills).

---

## Domain: preflight

### ADDED Requirement: Core Readiness Check

The system SHALL verify that `composer.json` constraints allow Drupal 11 before proceeding with the upgrade pipeline. The system MUST parse all `core_version_requirement` values in `web/modules/custom/*/` and `web/themes/custom/*/*.info.yml` files and compare them against the target Drupal version.

| Req | Strength | Behavior |
|-----|----------|----------|
| Constraint check | MUST | Parse `composer.json` `require.drupal/core` constraint and verify it permits target major version |
| Module scan | MUST | Scan all custom module/theme `.info.yml` files for `core_version_requirement` |
| Blockers report | MUST | List every module/theme with incompatible `core_version_requirement` |
| Early abort | MUST | Halt pipeline with exit code and blockers report if any incompatibility found |

#### Scenario: All constraints allow Drupal 11

- GIVEN `composer.json` with `"drupal/core": "^10.3 || ^11"` and all `.info.yml` files with `core_version_requirement: ">=10.0 || ^11"`
- WHEN preflight runs core readiness check
- THEN the system SHALL report "core readiness: OK" and proceed

#### Scenario: Composer constraint blocks Drupal 11

- GIVEN `composer.json` with `"drupal/core": "^10.3"` (no `^11` allowance)
- WHEN preflight runs core readiness check
- THEN the system SHALL halt with message "composer.json constraint ^10.3 does not permit Drupal 11" and list the constraint

#### Scenario: Custom modules with incompatible core_version_requirement

- GIVEN 3 custom modules where 2 have `core_version_requirement: "<11"` and 1 has `">=10.0"`
- WHEN preflight runs core readiness check
- THEN the system SHALL halt and report: `blockers: [{module: "mod_a", constraint: "<11"}, {module: "mod_b", constraint: "<11"}]`

#### Scenario: No custom modules exist

- GIVEN a project with empty `web/modules/custom/` and `web/themes/custom/`
- WHEN preflight runs core readiness check
- THEN the system SHALL skip the module scan and report "no custom code to check"

---

## Domain: cleanup-stage (NEW)

### ADDED Requirement: Post-Validation Cleanup

The system SHALL execute a cleanup stage (Stage 8) ONLY after the validation stage (Stage 7) exits with code 0. The cleanup stage MUST uninstall `upgrade_status` via drush, remove `drupal/upgrade_status` from `composer.json`, and create an atomic commit.

| Req | Strength | Behavior |
|-----|----------|----------|
| Gate on validate | MUST | Run only if validate exit code == 0 |
| Skip on failure | MUST | Skip entirely with log message if validate fails |
| Drush uninstall | MUST | Run `drush pm:uninstall upgrade_status -y` |
| Composer remove | MUST | Run `composer remove drupal/upgrade_status` |
| Atomic commit | MUST | Commit with message `chore(cleanup): remove upgrade_status post D11 migration` |
| Idempotent | SHOULD | Skip steps for already-removed components |

#### Scenario: Validate passes, cleanup runs

- GIVEN validation stage exited with code 0
- WHEN Stage 8 begins
- THEN the system SHALL run `drush pm:uninstall upgrade_status -y`, then `composer remove drupal/upgrade_status`, then commit with the specified message

#### Scenario: Validate fails, cleanup skipped

- GIVEN validation stage exited with non-zero code
- WHEN Stage 8 is reached
- THEN the system SHALL log "cleanup skipped: validation failed" and exit without modifications

#### Scenario: upgrade_status already removed

- GIVEN `upgrade_status` is not in `composer.json` and not enabled
- WHEN cleanup stage runs
- THEN the system SHALL detect the absent module, skip uninstall/remove steps, and log "cleanup: nothing to do"

#### Scenario: Drush uninstall fails

- GIVEN `drush pm:uninstall upgrade_status -y` returns non-zero
- WHEN cleanup stage runs
- THEN the system SHALL halt cleanup, report the error, and NOT proceed to composer remove or commit

---

## Domain: validation-gates

### MODIFIED Requirement: Phase Gating

The system SHALL NOT advance to the next pipeline phase until all items in the current phase pass validation. Exit code 3 from `upgrade_status:analyze` SHALL be treated as success-with-findings (not error). The validator SHALL parse stdout on exit 3 and only advance when `total_errors == 0`.

**Post-D11 behavior (core >= 11.x):** When the detected Drupal core version is 11.x or higher, the system SHALL use `drush updb -y` + `drush cr` + `drush status` as the primary success gates. The `upgrade_status:analyze` command SHALL be run as optional informational output only and MUST NOT block pipeline progression. Success criteria: site bootstraps (`drush status` returns exit 0), no pending updates (`drush updb -y` completes cleanly), no fatal log errors.

(Previously: upgrade_status:analyze was the sole gate for all Drupal versions)

#### Scenario: Phase complete

- GIVEN all contrib modules pass individual validation
- WHEN the orchestrator runs phase-level validation
- THEN the system SHALL execute `validate(global)` and proceed to the next phase only if `total_errors == 0`

#### Scenario: Phase incomplete

- GIVEN some contrib modules still have errors
- WHEN the orchestrator checks phase completion
- THEN the system SHALL NOT proceed to the custom loop and SHALL iterate remaining errors with the correct sub-agent

#### Scenario: Phase complete with exit code 3

- GIVEN `validate` returns exit code 3 with parseable findings
- WHEN the orchestrator checks phase completion
- THEN the system SHALL parse findings, count errors, and proceed only if `total_errors == 0`

#### Scenario: Validate scoped to module under DDEV

- GIVEN a DDEV project and `validate({module_name: "mymodule"})`
- WHEN validation runs
- THEN the system SHALL use `RunWithEnv` with `--root=` and handle exit code 3 correctly

#### Scenario: Post-D11 validation gates (core >= 11.x)

- GIVEN Drupal core version is 11.0.0 or higher
- WHEN the validation stage runs
- THEN the system SHALL execute `drush updb -y`, then `drush cr`, then `drush status` as success gates, and MAY run `upgrade_status:analyze` for informational output only

#### Scenario: Post-D11 drush status fails

- GIVEN Drupal core >= 11.x and `drush status` returns non-zero
- WHEN post-D11 validation runs
- THEN the system SHALL report "site bootstrap failed" with drush stderr and halt

#### Scenario: Post-D11 pending updates remain

- GIVEN Drupal core >= 11.x and `drush updb -y` reports pending updates that fail
- WHEN post-D11 validation runs
- THEN the system SHALL report the failed update and halt

---

## Domain: issue-patches

### MODIFIED Requirement: Issue Lookup by Module Name

The system SHALL search Drupal.org for issues related to a module and extract patch/diff/MR links. The system SHALL return structured JSON responses with `status`, `module`, `searched`, `message`, and `suggestion` fields instead of empty arrays or null values.

| Req | Strength | Behavior |
|-----|----------|----------|
| Structured response | MUST | Return `{status, module, searched, message, suggestion, patches[]}` |
| Status field | MUST | One of: `patches_found`, `no_patches_found`, `error` |
| Suggestion field | MUST | Include actionable next step (e.g., "check manually at https://...") |
| No empty arrays | MUST NOT | Return bare `[]` or `null` without context |

(Previously: returned empty array `[]` when no issues found, with no context)

#### Scenario: Module with patch issues

- GIVEN a module machine name with issues containing patches
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "patches_found", module: "<name>", patches: [{url, status, date, is_patch}], message: "N patches found", suggestion: "Apply highest-date RTBC patch first"}`

#### Scenario: Module with no issues

- GIVEN a module machine name with no relevant issues
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "no_patches_found", module: "<name>", searched: "<url>", message: "No patches found on Drupal.org", suggestion: "Create a custom patch or check issue queue manually at <url>"}`

#### Scenario: API error during lookup

- GIVEN Drupal.org API returns an error or is unreachable
- WHEN the system queries for issues
- THEN the system SHALL return `{status: "error", module: "<name>", message: "<error detail>", suggestion: "Retry later or check manually"}`

### MODIFIED Requirement: Issue Lookup by NID

The system SHALL extract patch/diff/MR links from a specific Drupal.org issue by NID. The system SHALL return structured JSON with the same fields as module lookup.

(Previously: returned empty array `[]` when no patches found)

#### Scenario: Issue with multiple patches

- GIVEN an issue NID with multiple file attachments
- WHEN the system scrapes the issue page
- THEN the system SHALL return all patch/diff/MR URLs with their upload dates and statuses in a structured response

#### Scenario: Issue with no patches

- GIVEN an issue NID with no file attachments
- WHEN the system scrapes the issue page
- THEN the system SHALL return `{status: "no_patches_found", module: "<from-nid>", searched: "https://www.drupal.org/node/<NID>", message: "No patches in this issue", suggestion: "Check related issues"}`

---

## Domain: apply-patch

### MODIFIED Requirement: Git Apply

The system SHALL apply downloaded patches using `git apply`. The system SHALL determine the project web root by reading `composer.json` → `extra.drupal-scaffold.locations.web-root` and use it as the base path for patch operations. If the scaffold config is not present, the system SHALL fall back to `web/` as the default web root. The system MUST NOT use `os.Getwd()` to determine the web root.

| Req | Strength | Behavior |
|-----|----------|----------|
| Web root from composer | MUST | Read `extra.drupal-scaffold.locations.web-root` from `composer.json` |
| Fallback | MUST | Default to `web/` if scaffold config absent |
| No os.Getwd() | MUST NOT | Use `os.Getwd()` for web root determination |
| Project path based | MUST | Resolve web root relative to `project_path` parameter |

(Previously: used `os.Getwd()` which fails when drup runs from a different working directory than the Drupal project root)

#### Scenario: Clean apply

- GIVEN a valid .patch file and a clean git working tree
- WHEN the system runs `git apply <patch_file>`
- THEN the system SHALL report `{applied: true}` with the list of modified files

#### Scenario: Apply conflict

- GIVEN a patch that conflicts with current code
- WHEN the system runs `git apply <patch_file>`
- THEN the system SHALL report `{applied: false}` with the conflict details from git stderr

#### Scenario: Apply with whitespace issues

- GIVEN a patch with whitespace differences
- WHEN the system runs `git apply --whitespace=nowarn <patch_file>`
- THEN the system SHALL attempt apply with whitespace tolerance before reporting failure

#### Scenario: Custom web root from composer scaffold

- GIVEN `composer.json` with `extra.drupal-scaffold.locations.web-root: "docroot"`
- WHEN create_patch resolves the web root
- THEN the system SHALL use `<project_path>/docroot` as the base path

#### Scenario: No scaffold config present

- GIVEN `composer.json` without `extra.drupal-scaffold`
- WHEN create_patch resolves the web root
- THEN the system SHALL fall back to `<project_path>/web`

---

## Domain: core-upgrade

### MODIFIED Requirement: Composer Execution

The system MUST run `composer config policy.advisories.block false` before the require command to disable advisory blocking, then run `composer require drupal/core-recommended:^<target> drupal/core:^<target> --with-all-dependencies`, followed by `composer update --with-all-dependencies` to ensure full dependency resolution. When DDEV is detected via `envdetect.Detect()`, the system SHALL prefix all composer commands with `ddev composer` instead of bare `composer`.

(Previously: always used bare `composer` regardless of environment)

#### Scenario: Composer update with advisory bypass

- GIVEN a valid composer.json with updated constraint
- WHEN composer execution runs
- THEN it MUST disable advisory blocking, invoke `composer require` with `--with-all-dependencies`, run `composer update --with-all-dependencies`, and propagate the final exit code

#### Scenario: Composer not available

- GIVEN `composer` is not in PATH
- WHEN composer execution runs
- THEN it MUST exit non-zero with "composer not found" error

#### Scenario: DDEV environment detected

- GIVEN a DDEV project (`.ddev/` directory present)
- WHEN composer execution runs
- THEN the system SHALL execute `ddev composer config policy.advisories.block false`, `ddev composer require ...`, and `ddev composer update ...`

#### Scenario: Non-DDEV environment

- GIVEN a direct composer project (no DDEV)
- WHEN composer execution runs
- THEN the system SHALL use bare `composer` commands as before

---

## Domain: scan

### MODIFIED Requirement: Drush Invocation

The system SHALL invoke `drush upgrade_status:analyze` with the `--all` flag. The system SHALL use `--root=<path>` instead of `-r <path>` for DDEV compatibility. The system SHALL use `RunWithEnv` with environment detection to prefix commands (e.g. `ddev`) when a container environment is detected. The `--format=json` flag SHALL NOT be used; the parser handles plain-text output.

**Smart no-op bypass:** Before invoking drush, the system SHALL check if both `web/modules/custom/` and `web/themes/custom/` are empty (no subdirectories). If both are empty, the system SHALL skip the rector stage and the custom analysis loop, logging "scan: no custom code found, skipping rector and custom analysis" and returning a zero-error model immediately.

| Req | Strength | Behavior |
|-----|----------|----------|
| Root flag | MUST | Use `--root=<path>` instead of `-r <path>` |
| Env detection | MUST | Call `envdetect.Detect()` and use `RunWithEnv(prefix, ...)` |
| Exit code 3 | MUST | Treat exit code 3 as success-with-findings, not error |
| Exit code 1,2,>3 | MUST | Treat as real errors and abort |
| Stdout on exit 3 | MUST | Parse stdout regardless of exit code 3 |
| Empty stdout + exit 3 | MUST | Treat as error (drush crashed, not findings) |
| Empty custom dirs | MUST | Skip rector + custom analysis with informative log |

(Previously: always ran full scan regardless of whether custom code existed)

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

#### Scenario: Full project scan

- GIVEN a Drupal project with multiple modules and themes
- WHEN `drup scan` runs
- THEN the system SHALL execute `drush upgrade_status:analyze --all` and parse the complete plain-text output

#### Scenario: Empty results without --all

- GIVEN a Drupal project where `upgrade_status:analyze` without `--all` returns empty results
- WHEN `drup scan` runs
- THEN the system SHALL still return full analysis by including the `--all` flag

#### Scenario: No custom code — smart bypass

- GIVEN `web/modules/custom/` and `web/themes/custom/` are both empty (no subdirectories)
- WHEN `drup scan` runs
- THEN the system SHALL skip rector stage and custom analysis, log "scan: no custom code found, skipping rector and custom analysis", and return zero-error model

#### Scenario: Custom modules exist but themes empty

- GIVEN `web/modules/custom/mymodule/` exists but `web/themes/custom/` is empty
- WHEN `drup scan` runs
- THEN the system SHALL proceed with full scan (custom code is present)

---

## Domain: report

### MODIFIED Requirement: JSON Report Generation

The system SHALL generate a JSON report containing all upgrade results with real scan data. The system SHALL collect actual error counts from `scan.ParseCodeclimateJSON()` or equivalent instead of hardcoded zeros. The system SHALL include a `pipeline_metrics` section with timing, command counts, and retry data.

| Req | Strength | Behavior |
|-----|----------|----------|
| Real error data | MUST | Populate `total_errors` from actual scan results |
| Real error list | MUST | Populate `resolved` and `pending` from scan data |
| No hardcoded zeros | MUST NOT | Use `0` only when scan confirms zero findings |
| Pipeline metrics | MUST | Include `pipeline_metrics` section in JSON output |

(Previously: no pipeline metrics in report)

#### Scenario: Full report with resolved and pending

- GIVEN an upgrade session with some resolved and some pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `{resolved: [...], pending: [...], total_errors: N, token_accounting: {...}, pipeline_metrics: {...}}`

#### Scenario: All errors resolved

- GIVEN an upgrade session with zero pending errors
- WHEN report generation runs
- THEN the system SHALL output JSON with `pending: []` and `total_errors: 0`

#### Scenario: Report after scan with findings

- GIVEN a project with 15 deprecation errors
- WHEN `drup report` runs
- THEN the system SHALL output JSON with `total_errors: 15` and populated error arrays

#### Scenario: Report with no scan data available

- GIVEN no prior scan has been run
- WHEN `drup report` runs
- THEN the system SHALL run a scan first, then generate the report with real data

#### Scenario: Report with pipeline metrics

- GIVEN a completed pipeline run
- WHEN report generation runs
- THEN the system SHALL include `pipeline_metrics` with `total_duration_ms`, `stage_durations`, `commands_executed`, `files_modified`, `retries`, `human_interventions`

---

## Domain: pipeline-metrics (NEW)

### ADDED Requirement: Non-Blocking Metrics Collection

The system SHALL collect pipeline execution metrics throughout the upgrade process. Metrics collection MUST be non-blocking — a metrics failure SHALL NOT halt or affect the pipeline. The system SHALL output metrics as a JSON section in the final report.

| Req | Strength | Behavior |
|-----|----------|----------|
| Total duration | MUST | Track `total_duration_ms` from pipeline start to end |
| Stage durations | MUST | Track per-stage `duration_ms` in `stage_durations` map |
| Commands executed | MUST | Count total shell commands in `commands_executed` |
| Files modified | MUST | Count files changed in `files_modified` |
| Retries | MUST | Count retry attempts in `retries` |
| Human interventions | MUST | Count human escalations in `human_interventions` |
| Non-blocking | MUST | Metrics collection failure SHALL NOT block pipeline |
| JSON output | MUST | Output as `pipeline_metrics` section in report |

#### Scenario: Full pipeline metrics collection

- GIVEN a complete pipeline run with all stages
- WHEN the pipeline finishes
- THEN the system SHALL produce `{total_duration_ms: N, stage_durations: {preflight: N, scan: N, ...}, commands_executed: N, files_modified: N, retries: N, human_interventions: N}`

#### Scenario: Metrics collection error

- GIVEN a metrics tracking error (e.g., clock skew, counter overflow)
- WHEN the pipeline runs
- THEN the system SHALL log the metrics error and continue pipeline execution normally

#### Scenario: Partial pipeline run

- GIVEN a pipeline that fails at Stage 3 (contrib)
- WHEN metrics are collected
- THEN the system SHALL report durations only for completed stages and include the failure point

---

## Domain: isPHPCompatible (semver)

### ADDED Requirement: Semver-Based PHP Compatibility Check

The system SHALL replace string-based PHP version comparison with proper semantic version comparison. The system MUST parse PHP version constraints (e.g., `">=8.1"`, `"^8.2"`) and compare them against the detected PHP version using semver rules.

| Req | Strength | Behavior |
|-----|----------|----------|
| Semver parsing | MUST | Parse version strings into comparable semver components |
| Constraint evaluation | MUST | Evaluate constraint operators (`>=`, `^`, `~`, `||`) correctly |
| No string comparison | MUST NOT | Use lexicographic string comparison for versions |
| Implementation | SHOULD | Use `github.com/Masterminds/semver/v3` if already direct dep; otherwise minimal stdlib implementation |

#### Scenario: Compatible PHP version

- GIVEN PHP 8.3 detected and constraint `">=8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `true`

#### Scenario: Incompatible PHP version

- GIVEN PHP 8.0 detected and constraint `">=8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `false`

#### Scenario: Caret constraint

- GIVEN PHP 8.2 detected and constraint `"^8.1"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return `true` (8.2 is within ^8.1 range)

#### Scenario: Invalid version string

- GIVEN an unparseable version string `"abc"`
- WHEN `isPHPCompatible` runs
- THEN the system SHALL return an error, not panic

---

## Domain: E2E Test Scaffolding

### ADDED Requirement: Mock-Based Integration Tests

The system SHALL provide integration test scaffolding for pipeline stage orchestration using mocked external commands. Tests MUST NOT require a real Drupal site. The system SHALL mock `cliRun` / subprocess calls to test stage sequencing, gate conditions, and error paths.

| Req | Strength | Behavior |
|-----|----------|----------|
| Mock external commands | MUST | Intercept subprocess calls (composer, drush, git) |
| Stage sequencing | MUST | Verify stages execute in correct order |
| Gate conditions | MUST | Verify gates block progression on failure |
| Error paths | MUST | Verify retry and escalation behavior |
| No real Drupal | MUST NOT | Require a running Drupal site or database |

#### Scenario: Full pipeline stage sequence

- GIVEN mocked commands that all succeed
- WHEN the integration test runs the pipeline
- THEN stages SHALL execute in order: preflight → dep-check → rector → contrib → custom → core-upgrade → validate → cleanup → report

#### Scenario: Gate blocks on validate failure

- GIVEN mocked validate returns exit code 1
- WHEN the integration test runs
- THEN the pipeline SHALL halt before cleanup stage and report validation failure

#### Scenario: Cleanup skipped on validate failure

- GIVEN mocked validate returns non-zero
- WHEN the integration test runs
- THEN cleanup stage SHALL NOT execute

---

## Domain: skill — drupal-custom-d11-fixes (NEW)

### ADDED Requirement: D11 Deprecation Catalog Skill

A skill file SHALL exist at `skills/drupal-custom-d11-fixes/SKILL.md` containing a catalog of approximately 50 Drupal 11 deprecation patterns. Each pattern MUST include: the deprecation description, replacement API, before/after code examples, complexity rating, and edge cases.

| Req | Strength | Behavior |
|-----|----------|----------|
| Location | MUST | `skills/drupal-custom-d11-fixes/SKILL.md` |
| Pattern count | SHOULD | ~50 patterns covering common D11 deprecations |
| Pattern structure | MUST | Each entry: deprecation, replacement, before/after, complexity, edge cases |
| Trigger | MUST | Activate when drup fix/validate finds custom module deprecations |

#### Scenario: Skill loads correctly

- GIVEN the skill file exists at the expected path
- WHEN the agent loads the skill
- THEN the skill SHALL provide deprecation patterns with actionable fix guidance

#### Scenario: Skill triggers on custom deprecation

- GIVEN `drup validate` reports deprecations in `web/modules/custom/mymodule/`
- WHEN the agent needs fix guidance
- THEN the agent SHALL load this skill and match the deprecation to a catalog entry

---

## Domain: skill — drupal-contrib-patch-writer (NEW)

### ADDED Requirement: Contrib Patch Writer Skill

A skill file SHALL exist at `skills/drupal-contrib-patch-writer/SKILL.md` containing guidelines for writing minimal contrib patches organized by error category. Categories MUST include: (A) info.yml fixes, (B) simple replacements, (C) API parameter changes, (D) architecture changes (escalate to human).

| Req | Strength | Behavior |
|-----|----------|----------|
| Location | MUST | `skills/drupal-contrib-patch-writer/SKILL.md` |
| Category A | MUST | info.yml fixes (core_version_requirement, etc.) |
| Category B | MUST | Simple text replacements (renamed functions, constants) |
| Category C | MUST | API parameter changes (new/removed params) |
| Category D | MUST | Architecture changes — escalate, do not auto-patch |

#### Scenario: Skill guides patch for info.yml fix

- GIVEN a contrib module with outdated `core_version_requirement`
- WHEN the agent writes a patch
- THEN the skill SHALL guide a Category A fix with minimal diff

#### Scenario: Skill escalates architecture changes

- GIVEN a contrib module requiring service container restructuring
- WHEN the agent evaluates the fix
- THEN the skill SHALL direct escalation to human review (Category D)

---

## Out of Scope

- Pipeline stage reordering (Core stays after Contrib+Custom)
- Docker-based E2E with real Drupal site
- golangci-lint configuration
- MCP server protocol changes
- Installer changes

## Verification Summary

| ID | Requirement | Verification |
|----|-------------|--------------|
| 1 | Core readiness check | `go test ./internal/app/...` — preflight aborts on incompatible constraints |
| 2 | Cleanup stage | `go test ./internal/app/...` — cleanup runs only after validate exit 0 |
| 3 | Post-D11 gates | `go test ./internal/app/...` — drush updb/cr/status used when core >= 11 |
| 4 | Smart no-op bypass | `go test ./internal/app/...` — rector/custom skipped on empty dirs |
| 5 | Structured issue_patches | `go test ./internal/drupalorg/...` — JSON has status/message/suggestion |
| 6 | create_patch web root | `go test ./internal/patch/...` — reads from composer scaffold config |
| 7 | Semver isPHPCompatible | `go test ./internal/...` — semver comparison, not string |
| 8 | DDEV composer calls | `go test ./internal/coreupgrade/...` — ddev prefix when detected |
| 9 | Pipeline metrics | `go test ./internal/metrics/...` — non-blocking collection, JSON output |
| 10 | E2E scaffolding | `go test ./internal/e2e/...` — mock-based stage orchestration tests |
| 11 | D11 fixes skill | File exists at `skills/drupal-custom-d11-fixes/SKILL.md` with ~50 patterns |
| 12 | Contrib patch skill | File exists at `skills/drupal-contrib-patch-writer/SKILL.md` with 4 categories |
