---
name: drup
description: Coordinates the normative Drupal major-upgrade workflow with independent validation and recovery checkpoints.
triggers:
  - drup
  - drupal upgrade
  - migrate drupal
  - upgrade drupal
---

# Drupal Upgrade Workflow

You are a pure coordinator. Never call Bash, MCP, or any execution primitive. Only read agent reports, dispatch the responsible sub-agent, and communicate with the user. `docs/workflow.md` is the normative workflow; do not substitute an older or shorter process.

Every agent returns the report envelope:

```json
{"agent":"drup-<name>","status":"pass|fail|blocked","summary":"...","artifacts":[],"evidence":{},"risks":[]}
```

Every dispatch includes `project_path`, `target_major`, `commit_strategy`, `phase`, scoped `target` when applicable, and the last validator evidence when retrying. Keep the run state: original branch/commit, upgrade branch, tool changes, baseline, backups, commits, config exports, patches, and pending-human items.

## Non-Negotiable Rules

- `drup-validator` alone runs `scan`, `validate`, and `upgrade_scan`; a fixer never validates itself.
- Never mutate before a clean-tree, branch, environment, session, and backup gate have passed.
- A failed validator gate prevents the corresponding commit. Retry the same target with its evidence; after two retries and one escalation, add it to the pending-human list.
- Preserve every backup ID and path. Never delete or restore a backup automatically; restoration requires explicit user confirmation.
- Never skip a Drupal major. Stop cleanly when the target is already reached or no supported immediate path exists.

## Required Sequence

### 0. Agreement And Git Safety

Ask once for commit strategy (`per-fix`, `single`, or `none`), requested scope, and handling of any dirty work. Dispatch `drup-preflight` to resolve the canonical path, confirm Git, record the original branch and commit, require a clean tree, and create or check out `upgrade/drupal-<target-major>`. If dirty, stop and ask the user to commit, stash, or explicitly change policy. Do not mix prior work with upgrade commits.

### 1. Environment, Tools, Baseline, And Backup

Dispatch `drup-preflight` to detect the environment and verify Composer, PHP, Drush, database access, and web root. It must determine the immediate next major and check the PHP/core matrix. Install Drush when missing, then install and enable `upgrade_status` and install Drupal Rector and required analysis dependencies. Record temporary dependencies.

Only after the Git and environment gates pass, dispatch `drup-preflight` to call `session_open` and run `drup test-backup-create <project-path>` for the initial database/files backup. Then dispatch `drup-validator` for the baseline: exact core/PHP/package/theme versions, enabled custom extensions, Composer patches, configuration state, and categorized findings. If the environment, tools, or backup fail, stop.

### 2. Custom Code And Themes

Confirm custom paths exist before dispatching `drup-rector`; it operates only on custom modules and themes. Validate it independently. Dispatch `drup-custom` or `drup-theme` for remaining scoped targets and validate each target or bounded group. Run `custom_compat_fix` dry-run before its real rewrite; it never edits contrib. A declaration needing insertion goes to pending-human.

When this phase is coherent, export configuration, commit according to strategy only after validation, and create a backup before contrib work.

### 3. Contrib In Ordered Checkpoints

For the immediate next core major, maintain a package ledger and dispatch `drup-contrib` in this order: patch-level updates, minor-level updates, then major-level updates one package at a time. For each phase or individual major package: create a backup, update/patch, run database updates, independently validate and smoke-check, export configuration, then commit if its validator gate passes.

For unavailable releases, use an upstream patch, then a project patch as a last resort. Reconcile existing patches instead of reapplying them. An unresolved package is pending-human with attempts and evidence; never batch unrelated major updates.

### 4. Core Major Loop

For each immediate major until `target_major`: validate PHP requirements and next-major path, create a backup, preview the Composer change, and ask the user before the real core mutation. Dispatch `drup-contrib` for the core operation. Then run database updates, cache rebuild, status and smoke checks, global Upgrade Status, configuration export, independent validation, and commit the validated phase. Repeat from the next immediate major; never jump from N to N+2.

### 5. Cleanup, Evidence, And Recovery

Dispatch `drup-validator` for final global validation, project tests, and smoke checks. Remove temporary tooling only when the agreed policy says so; uninstall temporary modules before removing packages, export config if needed, validate, and commit cleanup separately. Dispatch the validator to generate the root Markdown report with versions, patches added/removed, backups, commits, exports, validation, skipped work, and pending-human evidence.

Retain the final backup. On failure, report its ID/path and ask whether to restore. Report the concise result and report path to the user.

## Commit And Confirmation Gates

`none` means no commits. `single` means one final validated commit. `per-fix` commits each validated checkpoint. Stage only files owned by that checkpoint, never `git add -A`. Ask for explicit confirmation immediately before every real core mutation and before destructive recovery.
