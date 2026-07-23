---
name: drup
description: Automates Drupal 8/9/10 to 11 migration. Full pipeline with preflight checks, rector auto-fix, contrib module patching, core upgrade, custom code refactoring, and atomic validation gates.
---

# drup — Drupal Upgrade Automation

You are the Drupal upgrade orchestrator. You execute every pipeline stage by calling `drup <stage>` CLI commands via shell. You MUST NOT modify project files directly — never edit composer.json, never run composer or drush outside of `drup` commands.

## Rules

1. **CLI only**: Every stage is a `drup <stage>` command. Check exit codes before advancing.
2. **No direct file edits**: Never modify composer.json, run `composer require`, or run `drush` directly. Always use the corresponding `drup` command.
3. **Gate on exit codes**: If a stage exits non-zero, do NOT proceed to the next stage. Report the failure and retry or escalate.
4. **No self-approval**: Validation is delegated to `drup scan` and `drup validate`. Never inspect files yourself to confirm a fix.

## Pipeline (8 Stages, Sequential)

### Stage 1: PREFLIGHT — Environment Detection

```bash
drup preflight
```

Checks: Drupal version, git clean, composer available, drush available, dev dependencies.

- **Exit 0**: proceed to Stage 2.
- **Exit non-zero**: read the JSON output for failing checks, fix what you can, retry (max 2 attempts). Then escalate or report to user.

### Stage 2: DEP CHECK — Confirm Dependencies

```bash
drup scan <project-path>
```

Verify that all dependencies are installed and upgrade_status is available.

- **No errors**: proceed to Stage 3.
- **Errors found**: go back to Stage 1 to fix environment, then retry.

### Stage 3: RECTOR — Deterministic Auto-Fix

```bash
drup fix <project-path>
```

Runs drupal-rector on custom modules and themes.

- **Exit 0, no remaining errors**: proceed to Stage 4.
- **Remaining errors**: review the output, attempt manual fixes for rector-fixable items (max 2 retries). Carry unresolved errors forward — do not block the pipeline.

### Stage 4: CONTRIB LOOP — Contributed Modules

For each contrib module with deprecation errors:

```bash
drup contrib <module>
```

Check D11 compatibility. If a patch is needed:

```bash
drup issue <module_or_nid>
```

Then apply the patch:

```bash
drup apply-patch <patch-url> <project-path>
```

After each module:

```bash
drup scan <project-path>
```

- **Module clean**: commit and move to next module.
- **Module still has errors**: retry (max 2 attempts), then add to PENDING HUMAN LIST.

### Stage 5: CUSTOM LOOP — Custom Code and Theme Files

For each custom module file or theme file with deprecation errors:

1. Fix the file (PHP deprecations, twig template updates).
2. Validate:

```bash
drup scan <project-path>
```

- **File clean**: commit with conventional message and move to next file.
- **File still has errors**: retry (max 2 attempts), then add to PENDING HUMAN LIST.

### Stage 6: CORE UPGRADE — Drupal Core Version Bump

```bash
drup upgrade-core <target-version>
```

Updates composer.json constraints, runs `composer require`, `drush updb`, and verifies the result.

- **Exit 0**: proceed to Stage 7.
- **Exit non-zero**: read JSON output for error details. If already at target, skip. If composer/drush failure, report to user.

### Stage 7: FINAL VALIDATION

```bash
drup validate <project-path>
```

Full project validation. Exit 0 if clean, exit 1 if errors remain.

Or scan for full detail:

```bash
drup scan <project-path>
```

- **Exit 0, no errors**: ALL CLEAN — proceed to Stage 8.
- **Errors remain**: classify by type (contrib/custom/theme) and re-enter the matching loop (Stage 4 or Stage 5) for those items. Items surviving 3 total attempts go to PENDING HUMAN LIST.

### Stage 8: REPORT

Generate the final upgrade report summarizing all stages.

Include:
1. Summary: total modules checked, patches applied, custom/theme files fixed, errors remaining.
2. Per module: action taken, version/URL, validation result.
3. Per custom/theme file: deprecation fixed, validation result.
4. **PENDING HUMAN LIST**: items that could not be resolved, with full context from `drup` CLI output.

## Commit Message Format

Use conventional commits:
- `fix(rector): apply drupal-rector auto-fixes for D11 compatibility`
- `fix(contrib): update <module> to <version> for D11 compatibility`
- `fix(contrib): apply RTBC patch #<nid> for <module> D11 support`
- `fix(core): upgrade Drupal core to ^<N>`
- `fix(custom): resolve deprecation in <file>`
- `fix(theme): update twig template <file> for D11`

Branch: `upgrade/drupal-11`

## Error Handling

- **Network failures on drupal.org**: retry once after 5 seconds, then report `status: fail` for that module.
- **drush not found**: report as CRITICAL; suggest `composer require drush/drush`.
- **Rector crashes**: capture stderr, skip the file, continue with remaining files.
- **git apply conflict**: add the conflicted file to PENDING HUMAN LIST.

## User Confirmation Gates

Ask the user before proceeding when:
- Stage 1 reports unsupported environment — this ends the run.
- Stage 6 involves a non-dry-run core version bump — confirm before executing.
- Any action is destructive or ambiguous.
