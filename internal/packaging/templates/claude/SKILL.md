---
name: drup
description: Drupal Upgrade Automation — orchestrates D8/9/10→11 migration with validation gates
triggers:
  - drup
  - drupal upgrade
  - migrate drupal
---

# drup — Drupal Upgrade Orchestrator

## Pipeline (7 stages, sequential)

1. **PREFLIGHT**: detect Drupal version, check git clean, verify composer/drush
2. **DEP CHECK**: install upgrade_status, drupal-rector, phpstan-drupal → `validate(scope=env)`
3. **RECTOR**: run drupal-rector on custom modules + themes → commit → `validate(scope=rector)`
4. **CONTRIB LOOP**: for each contrib module:
   - Check D11 release → `composer require` → commit → `validate(scope=contrib,module=X)`
   - No release? → search RTBC patch → apply → commit → gate
   - Gate fail? → retry ×2 → escalate model → human list
5. **CUSTOM LOOP**: for each custom file:
   - Fix → `validate(scope=custom,file=Y)` → commit
   - Fail? → retry ×2 → escalate → human list
6. **FINAL VALIDATION**: `validate(global)` → total_errors == 0?
7. **REPORT**: markdown + JSON

## Validation Gates (HARD enforcement)

- Orchestrator validates — sub-agents NEVER self-approve
- Max 2 retries per scope, then escalate model (haiku → sonnet)
- No phase advancement until all items pass

## MCP Tools

- `scan` — classify errors by type
- `validate` — re-run analysis with scope filtering
- `contrib_check` — D11 release lookup
- `issue_patches` — find RTBC patches
- `apply_patch` — download + git apply + composer-patches
- `autofix` — run drupal-rector
- `create_patch` — generate patch from deprecation details

## Sub-Agents

- `drup-preflight` (haiku) — env detection
- `drup-contrib` (haiku) — contrib module loop
- `drup-custom` (haiku → sonnet) — custom code fixes
- `drup-theme` (haiku) — theme fixes
