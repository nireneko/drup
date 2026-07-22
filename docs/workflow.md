# drup Upgrade Workflow — Complete Guide

This document describes the complete Drupal upgrade workflow orchestrated by `drup`, from preflight checks through final reporting.

---

## Overview

The `/drup <path>` command in Claude Code (or OpenCode/Codex) runs a **7-stage pipeline** that automates Drupal 8/9/10 → 11 migration:

1. **Preflight** — environment detection, dependency installation
2. **Dep Check** — validation that dependencies are ready
3. **Rector** — automatic code cleanup (80% of the work, zero tokens)
4. **Contrib Loop** — update contrib modules (releases, patches, core-upgrade)
5. **Custom/Theme Loop** — refactor custom code and themes
6. **Final Validation** — global error check and routing
7. **Report** — generate UPGRADE-REPORT.md with full summary

**Key principle**: The orchestrator (SKILL.md) is a pure coordinator with zero execute permissions. All work is delegated to specialized sub-agents, each with a single responsibility.

---

## The 7 Stages

### Stage 1: Preflight

**Sub-agent**: `drup-preflight`  
**Responsibility**: Verify environment and install dependencies

**What happens**:
- Detects environment: ddev, lando, docker4drupal, or direct
  - If **unsupported** → **TERMINAL STATE** — pipeline stops, reports to user, never proceeds
- Verifies clean git tree (no uncommitted changes)
- Checks composer and drush availability
- Detects current Drupal version
- Installs missing dev dependencies:
  - `upgrade_status` module (required for scanning)
  - `drupal-rector` (required for automatic fixes)
  - `phpstan-drupal` (optional, for static analysis)

**Tools used**:
- `detect_env` — identify the environment (ddev/lando/docker/direct)
- `composer_require` — safely require `upgrade_status`, `drupal-rector`, etc.
- `drush_exec` — enable modules and verify installation

**Output to user**:
- Environment type (ddev, lando, etc.)
- Current Drupal version
- Dependency install summary
- Any fatal issues (unsupported environment, no git, etc.)

**Validation**: Stage 2 (`drup-validator`) confirms all dependencies actually took effect before proceeding.

---

### Stage 2: Dep Check

**Sub-agent**: `drup-validator`  
**Responsibility**: Confirm Stage 1 is complete and valid

**What happens**:
- Re-scans environment to verify Stage 1's dependency installations took effect
- Confirms `upgrade_status` is installed and enabled
- Confirms `drupal-rector` is available
- Checks that git is in a clean state (no uncommitted changes)

**Tools used**:
- `detect_env` — re-verify environment
- `upgrade_scan` — confirm dependencies are ready
- `scan` — initial deprecation analysis (preview for validation)

**Why separate from Stage 1**: `drup-preflight` installs dependencies; it never confirms its own work. Only `drup-validator` can confirm, preserving the "no self-approval" guarantee.

**Output to user**:
- Confirmation that dependencies are ready
- Initial error count (preview of what's to come)

---

### Stage 3: Rector (0 tokens)

**Sub-agent**: `drup-rector`  
**Responsibility**: Run drupal-rector to auto-fix standard deprecations

**What happens**:
- Runs `drupal-rector` with Drupal 11 rule sets over custom modules and themes
- Automatically fixes ~80% of standard deprecations (loops, functions, hooks, etc.)
- Commits changes with message: "style: apply drupal-rector auto-fixes for D11 compatibility"

**Tools used**:
- `autofix` — run drupal-rector on custom code

**Why 0 tokens**: `drupal-rector` is deterministic — it applies pre-defined PHP transformation rules. No AI decision-making needed.

**Validation**: `drup-validator` confirms `total_errors == 0` (or identifies remaining errors by scope) before `drup-rector` commits.

**Output to user**:
- Number of files processed
- Number of deprecations auto-fixed
- Remaining errors (if any) routed back to Stage 4 or 5

---

### Stage 4: Contrib Loop

**Sub-agent**: `drup-contrib`  
**Responsibility**: Update contrib modules to D11-compatible versions

**Per module** (for each contrib module with errors):

1. **Check for a D11-compatible release**
   - Tool: `contrib_check` → fetch Drupal.org release history
   - If **D11 release exists** → `composer require module:^11` → commit
   - If **no release** → proceed to patch search

2. **Search for patches** (if no release available)
   - Tool: `issue_patches` → search Drupal.org issues for patches
   - Tool: `patch_reconcile` → analysis-only: is the patch still needed? is it obsolete?
   - Tool: `apply_patch` → download and apply the best patch
   - If **patch applies cleanly** → commit with patch reference
   - If **no patches available** → create a local patch via `create_patch`

3. **Check for core-version bump** (if target Drupal major version has changed)
   - Tool: `core_upgrade_check` → preview what `composer.json` will change
   - Tool: `core_upgrade_apply` → requires clean git; creates checkpoint, mutates `composer.json`, commits
   - (Dry-run mode available for preview-only)

4. **Validation gate**: `drup-validator` confirms `total_errors == 0` for this module before committing.

**Validation**:
- Each module is validated before commit
- If errors remain, module is added to the pending list for Stage 5 (custom code refactor)

**Output to user**:
- Per-module summary: upgraded, patched, or pending
- Commit hashes for each update
- Any modules requiring manual review

---

### Stage 5: Custom/Theme Loop

**Sub-agents**: `drup-custom` (custom modules), `drup-theme` (theme files)  
**Responsibility**: Refactor custom code and themes to D11 compatibility

**Per file** (for each custom PHP file or twig template with deprecations):

1. **Analyze the error**: Read the deprecation message and context
2. **Apply minimal fix**: Rewrite the code to use the modern API
3. **Validation gate**: `drup-validator` confirms the fix is valid (zero new errors introduced)
4. **Commit with message**: "fix: refactor {file} for D11 compatibility"

**Retry and escalation**:
- If validation fails (errors persist):
  - **Retry 1** (haiku with feedback from validator) → attempt fix again
  - **Retry 2** (escalate to sonnet model with full context) → attempt fix again
  - **After 2 retries**: mark as pending for human review

**Tools used**:
- `validate` — per-file scope validation
- `scan` — categorize remaining errors

**Why two sub-agents**: Isolates context per file type (PHP vs Twig) to avoid saturating the orchestrator's window.

**Output to user**:
- Per-file summary: fixed, pending, escalated
- Files requiring human review (with context for the developer)

---

### Stage 6: Final Validation

**Sub-agent**: `drup-validator`  
**Responsibility**: Global error check and re-routing

**What happens**:
- Runs a global `scan` to get the current error state
- If `total_errors == 0` → proceed to Stage 7 (Report)
- If `total_errors > 0` → classify remaining errors and re-route:
  - Module-level errors → back to Stage 4 (Contrib Loop)
  - File-level errors → back to Stage 5 (Custom/Theme Loop)
  - Otherwise → pending human list

**Tools used**:
- `scan` — global scan of the project
- `validate` — scoped validation per module/file

**Why final validation**: Ensures the upgrade is actually complete before generating the report.

**Output to user**:
- Current error count
- What needs to be addressed next (if anything)

---

### Stage 7: Report

**Sub-agent**: `drup-validator`  
**Responsibility**: Generate final upgrade report

**What happens**:
- Generates `UPGRADE-REPORT.md` in the project root with:
  - **Summary**: Total errors resolved, remaining, pending
  - **Per-module breakdown**: status (upgraded/patched/pending), commit hashes, issues
  - **Per-file breakdown** (custom code): status (fixed/pending/escalated), remaining errors
  - **Pending list**: Files/modules requiring human review with full context
  - **Next steps**: What the developer should do to complete the upgrade

- Also generates JSON report for machine parsing (CI/CD integration)

**Tools used**:
- `generate_report` — generate Markdown + JSON reports

**Output to user**:
- UPGRADE-REPORT.md (displayed in Claude Code)
- Confirmation of completion or pending items

---

## Validation Gates (Strict Rules)

The orchestrator enforces strict validation rules to prevent errors from propagating:

| Rule | What it does |
|------|-----------|
| **External Validation Only** | Only `drup-validator` calls `scan`, `validate`, `upgrade_scan`. No other sub-agent validates its own work |
| **No Self-Approval** | A sub-agent saying "done" is meaningless. Only `drup-validator` report counts |
| **Validator Owns All Gates** | Before ANY commit, `drup-validator` must confirm zero errors for that exact scope |
| **Retry with Feedback** | If a fix fails validation, the sub-agent receives the validator's output as feedback before retrying |
| **Max 2 Retries** | Per scope on haiku model. Then escalate model (haiku → sonnet). Then pending human list |
| **Phase Gates** | No stage advances until ALL items in that stage pass validation |
| **Commit Only Post-Gate** | Each commit ONLY happens after `drup-validator` reports 0 errors |

---

## Error Classification

When errors persist after Stages 3–5, they are classified:

| Classification | How it's handled |
|---|---|
| **Module-level** (contrib, core compatibility) | Re-route to Stage 4 (Contrib Loop) — look for newer release, patch, or local patch |
| **File-level** (custom code, theme) | Re-route to Stage 5 (Custom/Theme Loop) — refactor with retry and escalation |
| **Unresolvable** (after 2 retries) | Add to pending human list in UPGRADE-REPORT.md with full context |

---

## Git Workflow

Each stage commits atomically:

- **Stage 3**: "style: apply drupal-rector auto-fixes for D11 compatibility"
- **Stage 4** (per module): "feat: upgrade {module} to D11-compatible {version}" (or "fix: apply {issue-id} patch")
- **Stage 5** (per file): "fix: refactor {file} for D11 compatibility"
- **Stage 4** (core): "feat(core): upgrade to Drupal {major}.{minor} (composer.json update)"

**Rollback**: Each commit is a clean checkpoint. If anything fails, revert individual commits from the history.

---

## Configuration

Optional config at `~/.config/drup/config.yaml`:

```yaml
agents:
  claude-code:
    skills:
      drup:
        model: claude-sonnet-4  # Orchestrator model
    agents:
      drup-rector:
        model: claude-haiku-3-5  # Rector is cheap; haiku is fine
      drup-contrib:
        model: claude-haiku-3-5  # Contrib updates are mostly deterministic
      drup-custom:
        model: claude-sonnet-4   # Custom code refactoring is hard; use sonnet
      drup-validator:
        model: claude-haiku-3-5  # Validator just scans and reports; haiku is fine
```

If not configured, `drup` uses sensible defaults (cheap for mechanical work, strong for reasoning).

---

## Example Run

```
User: /drup /path/to/project

[Stage 1: Preflight]
✓ Environment: ddev
✓ Clean git
✓ Drupal 10.0.0 detected
✓ Installing upgrade_status...
✓ Installing drupal-rector...

[Stage 2: Dep Check]
✓ Dependencies confirmed
ℹ 42 total errors found

[Stage 3: Rector]
✓ Auto-fixed 34 deprecations
ℹ 8 errors remain (manual fixes needed)

[Stage 4: Contrib Loop]
  webform:
    ✓ D11 release available → upgraded to 6.2
  entity_reference_revisions:
    ✓ RTBC patch found → applied
  mymodule (custom, but in contrib folder):
    ℹ No D11 release, no patch. Routed to Stage 5.

[Stage 5: Custom/Theme Loop]
  modules/custom/mymodule/mymodule.module:
    ✓ Fixed deprecated hook_form_alter usage
  themes/custom/mytheme/mytheme.theme:
    ✓ Fixed deprecated theme_render_element() call
  modules/custom/mymodule/mymodule.install:
    ℹ Requires manual review (complex schema upgrade)

[Stage 6: Final Validation]
ℹ 1 error remains → pending human review

[Stage 7: Report]
✓ UPGRADE-REPORT.md generated
📄 Open it to see the full summary and next steps
```

---

## Specs and Architecture

For full technical requirements, see:
- `openspec/changes/archive/2026-07-22-drupal-upgrade-orchestrator/specs/` — all formal specifications
- `openspec/changes/archive/2026-07-22-drupal-upgrade-orchestrator/design.md` — architectural decisions

---

## Troubleshooting

| Issue | Cause | Solution |
|-------|-------|----------|
| "unsupported environment" | ddev/lando/docker/direct not found | Install one of the supported environments or use `--force` |
| "dirty git tree" | Uncommitted changes | Commit or stash your changes |
| "upgrade_status not found" | Preflight install failed | Check composer/drush output, run `drup preflight` again |
| Errors won't go away | Validator is rightfully blocking | Review the UPGRADE-REPORT.md pending list and manually fix those items |
| Haiku model keeps failing | The custom code is too complex for cheap models | Configure `drup-custom` to use `claude-sonnet-4` |
