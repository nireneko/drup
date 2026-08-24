# Drupal Upgrade Workflow

This document is the operational reference for the Drupal upgrade workflow exposed by `drup`.
The **Target Workflow** is normative: it is the process that must be implemented. The current
agent behavior is documented only to make implementation gaps visible and prevent accidental
assumptions that an existing prompt already provides a required safeguard.

## Executive Summary

The target process is a coordinator-driven workflow with specialized agents:

1. The orchestrator asks for the commit strategy, scope, and handling of dirty work.
2. `drup-preflight` opens the project session, creates a local database/files backup, detects the environment, and installs upgrade tooling.
3. `drup-validator` independently verifies readiness and later validates every fix.
4. `drup-rector` fixes custom modules and themes automatically.
5. `drup-contrib` resolves contributed-module releases and patches.
6. `drup-custom` and `drup-theme` handle remaining file-level fixes.
7. The core is upgraded, a final validation is run, and a Markdown report is generated.
8. The backup is retained; it is never deleted automatically.

The current prompts do **not** yet implement the full phased workflow described here. The
missing pieces are listed in [Implementation Gaps](#implementation-gaps). Until those gaps
are implemented, the current workflow must not be presented as equivalent to the target.

## Source Of Truth

The generated platform-specific files are rendered from:

- `internal/packaging/templates/opencode/SKILL.md`
- `internal/packaging/templates/opencode/agents/drup-*.md`
- Equivalent templates under `internal/packaging/templates/claude/` and `codex/`
- The Codex entry prompt: `~/.codex/prompts/drup.md`

The installed OpenCode agents are generated copies under `~/.config/opencode/agents/`.
Changes to generated copies are not the source of truth; update the repository templates
and run `drup sync` when implementation changes are intended.

## Current Effective Workflow

### Stage -1: Plan And User Agreement

Before mutation, the orchestrator asks for:

- Commit strategy: `per-fix`, `single`, or `none`.
- Scope: Rector, compatibility declarations, contrib, custom code, themes, and/or core.
- Handling of existing uncommitted changes.

The orchestrator then dispatches all work to sub-agents. It must not run Bash or MCP tools
itself.

### Stage 0: Safety Backup And Session Binding

`drup-preflight` must first call `session_open` and then `test_backup_create`.

The backup currently contains:

- A Drush database dump compressed as `database.sql.gz`.
- A filtered project filesystem archive as `files.tar.gz`.
- A manifest with checksums, the database command, exclusions, and the backup ID.

The backup is stored below `.drup/backups/`. The backup ID and path must remain in the run
state and in the final report.

Important: the current stage order creates this backup before the normal preflight git
cleanliness check. A clean-tree gate is still required before any code or dependency
mutation, but the prompts do not currently enforce the user's preferred order of
"git clean, then environment check, then backup".

### Stage 1: Preflight

`drup-preflight`:

- Detects `ddev`, `lando`, `docker4drupal`, or direct execution.
- Stops for an unsupported project.
- Reads the current Drupal core version from Composer metadata.
- Checks git status.
- Checks Composer and Drush availability.
- Installs `drupal/upgrade_status`, `palantirnet/drupal-rector`, and optionally `mglaman/phpstan-drupal`.
- Enables `upgrade_status` with Drush.

It does not install Drush itself. A missing Drush installation is reported as a blocker.

### Stage 2: Independent Readiness Validation

`drup-validator` verifies the preflight result. It is the only agent allowed to run
`scan`, `validate`, or `upgrade_scan`.

The validator separates environment failures from readiness findings such as core Composer
constraints and custom `core_version_requirement` declarations. Readiness findings are
resolved by later stages and must not block environment validation prematurely.

### Stage 3: Rector On Existing Custom Code

`drup-rector` runs only on custom modules and themes. It must not process contributed code.
The agent reports changed paths and never validates its own changes.

The orchestrator then asks `drup-validator` to check whether Rector still has fixable work.
The gate is not blindly `total_errors == 0`, because Upgrade Status also reports advisory
items that require human judgement and cannot be removed by Rector.

If the result is acceptable, the Rector changes may be committed. Failed targets are retried
with validator evidence and eventually placed on the pending-human list.

### Stage 3.5: Custom Compatibility Declarations

`custom_compat_fix` widens `core_version_requirement` in custom modules, themes, and profiles.
It never edits contributed extensions. A dry run is expected before the real rewrite.

Extensions with no existing declaration are reported for human review because inserting a
new declaration requires a file-specific judgement.

### Stage 4: Contributed Module Resolution

For each affected contributed module, `drup-contrib`:

1. Checks whether a compatible release exists.
2. Selects an exact upgrade path when one exists.
3. Searches Drupal.org issues for a patch when no compatible release exists.
4. Applies an upstream patch and registers it in Composer when appropriate.
5. Generates a local Rector patch as a last resort and then applies it.
6. Reconciles existing patches instead of blindly reapplying them.

`drup-validator` validates the module after the change. Only then may the contrib agent
commit. A failed module is retried with the validator's evidence and then added to the
pending-human list.

### Stage 5: Custom And Theme Manual Fixes

The orchestrator routes each remaining file to:

- `drup-custom` for custom PHP/module files.
- `drup-theme` for Twig and theme files.

Each agent applies the smallest fix, leaves validation to `drup-validator`, and commits only
after the exact file scope passes. Failed files are retried with concrete validator output.

### Stage 6: Core Upgrade

`core_upgrade_check` previews the next major version. `core_upgrade_apply` then updates the
Composer constraints, resolves dependencies, runs `drush updb`, and verifies the result.
The real core mutation requires a clean tree, an existing backup, and an explicit user gate.

The current prompt has a single target-version operation. It does not yet orchestrate a
complete `9 -> 10 -> 11` or `10 -> 11` loop with a full validation boundary for every major.

### Stage 7: Final Validation And Report

The validator performs the global validation and generates the Markdown report from the
accumulated stage evidence. The report is expected to include:

- Modules checked and their before/after versions.
- Contributed releases, patches applied, patches created, and patches removed.
- Custom modules and themes changed.
- Validation results and remaining findings.
- A complete pending-human list with attempted actions and evidence.

The user receives a short summary and the path to the full Markdown report.

### Stage 9: Backup Finalization

The backup is retained after a successful run. On failure, the orchestrator reports the
backup and asks the user whether to restore it. Restoration is destructive and must use
explicit confirmation. Backup deletion is always a separate manual operation.

## Agent Responsibilities And Safety Gates

| Agent | Can mutate | Main responsibility | Independent validator required |
|---|---:|---|---:|
| `drup-preflight` | Yes | Session, environment, dependencies, backup | Yes |
| `drup-rector` | Yes | Rector on custom modules/themes | Yes |
| `drup-contrib` | Yes | Releases, patches, contrib compatibility, core operation | Yes |
| `drup-custom` | Yes | Custom PHP fixes | Yes |
| `drup-theme` | Yes | Twig/theme fixes | Yes |
| `drup-validator` | No | Scan, classify, validate, report | N/A |

The non-negotiable gates are:

- No mutating tool before the session and backup guards pass.
- No fixer agent validates its own work.
- No commit before the corresponding validator gate passes.
- Validator failures are retried with evidence, not with blind repetition.
- After the retry budget, unresolved work is explicitly reported for humans.

## Target Workflow (Normative)

The following is the complete workflow that should be implemented and treated as the desired
operational sequence.

### 0. Preconditions And Git Safety

1. Resolve the absolute canonical project path.
2. Confirm the project is a git repository.
3. Require a clean working tree before mutation.
4. Record the current branch and commit.
5. Create or check out a dedicated branch such as `upgrade/drupal-<target-major>`.
6. Record the branch in the run state and report it to the user.

If the tree is dirty, stop and ask the user to commit, stash, or explicitly change the policy.
Do not mix existing work with upgrade commits.

### 1. Environment And Version Preflight

1. Detect `ddev`, `docker4drupal`, `lando`, or direct execution.
2. Verify Composer, PHP, Drush, database access, and the Drupal web root.
3. Verify the current Drupal and PHP versions against the compatibility matrix.
4. Determine the immediate next Drupal major. Never skip a major version.
5. Check whether an upgrade is actually needed.
6. Create the initial database/files backup after the clean-tree and environment gates.

Drush must either be available from the project or be installed explicitly with Composer;
the workflow must not silently proceed with only a reachability check.

### 2. Install Upgrade Tooling

1. Install Drush if missing.
2. Install and enable `upgrade_status`.
3. Install Rector for Drupal and any required analysis dependencies.
4. Run the dependency validator.

All Composer changes must be recorded so the final report can distinguish temporary tools
from project runtime dependencies.

### 3. Baseline Upgrade Status

Run `upgrade_status` through Drush and save a structured baseline containing:

- Current core version.
- PHP version.
- Enabled custom modules and themes.
- Contributed modules and themes with exact installed versions.
- Current findings by category.
- Existing Composer patches.

This baseline is required for an accurate before/after report.

### 4. Custom Code And Theme Compatibility

1. Confirm custom module/theme paths exist before invoking Rector.
2. Run Rector only on those existing paths.
3. Re-run Upgrade Status.
4. Apply remaining custom and theme changes manually, one scoped target at a time.
5. Validate after each target or bounded group.
6. Export configuration when the code/config state is coherent.
7. Commit the validated custom/theme phase.
8. Create a backup before entering the next mutation phase.

### 5. Contrib Updates In Three Ordered Phases

Process only the immediate next core major and keep a ledger of every package.

#### Phase A: Patch-Level Compatibility

Update packages requiring only a patch release. Then:

1. Back up the database/files.
2. Apply Composer updates.
3. Run database updates with Drush.
4. Run Upgrade Status and smoke checks.
5. Export configuration.
6. Commit the phase.

#### Phase B: Minor-Level Compatibility

Repeat the same backup, update, database-update, validation, configuration-export, and commit
sequence for packages requiring a minor release.

#### Phase C: Major-Level Compatibility

Update major-version packages one at a time. For every package:

1. Create a backup.
2. Update exactly one package and resolve Composer dependencies.
3. Run database updates.
4. Run Upgrade Status and application smoke checks.
5. Export configuration.
6. Commit or stop with a package-specific rollback point.

Never batch unrelated major-version package updates. If a package has no compatible release,
use an upstream patch, a project patch, or the pending-human list.

### 6. Core Major Upgrade Loop

For each immediate core major until the requested target is reached:

1. Confirm the next major and PHP requirements.
2. Create a backup.
3. Preview the Composer change.
4. Ask for confirmation before the real core mutation.
5. Upgrade core and resolve dependencies.
6. Run database updates.
7. Run cache rebuild and status checks.
8. Run Upgrade Status again.
9. Export configuration.
10. Commit the validated core phase.

For example, a project moving from major `N` to `N+2` follows
`N -> complete N-to-N+1 cycle -> N+1 -> complete N+1-to-N+2 cycle -> N+2`.
The workflow must never jump directly across a major version. The concrete values may be
Drupal 9/10/11 today or Drupal 12/13 in the future.

### 7. Cleanup And Final Evidence

1. Run final global Upgrade Status validation.
2. Run the project's available tests and smoke checks.
3. Remove temporary `upgrade_status` and Rector dependencies only if the user/project policy
   says they are temporary.
4. Uninstall temporary modules before removing their Composer packages.
5. Export configuration if cleanup changes active configuration.
6. Commit cleanup separately.
7. Generate the Markdown report in the project root.
8. Retain the final backup and report its ID and path.
9. Show the user a concise summary and point them to the complete report.

## Implementation Gaps

These items are not fully represented by the current prompts/tools and must be resolved before
claiming that the target workflow is implemented:

| Gap | Current state | Required behavior |
|---|---|---|
| Git order | Backup stage runs before the preflight clean-tree check | Check git cleanliness before any backup/mutation, then create the branch |
| Upgrade branch | `upgrade/drupal-<target-major>` is only documented; no orchestrator stage/tool creates it | Add an explicit branch creation/check-out step and record the original branch |
| Drush installation | Drush is checked but not installed by preflight | Install or clearly block with an actionable instruction |
| Upgrade-needed decision | Core next-version preview exists, but no explicit no-op gate is documented | Stop cleanly when the project is already at the target or no supported path exists |
| Backup semantics | `test_backup_create` does database plus selected files | Keep it, but document exclusions and require an external production backup policy when needed |
| Baseline inventory | Validator scans findings but does not define a complete before-state inventory | Persist exact package/theme versions, patches, config state, and findings |
| Contrib ordering | Modules are processed in a generic per-module loop | Add patch, minor, and one-at-a-time major phases |
| Per-phase safety | No explicit backup, `updb`, smoke test, config export, and commit boundary between contrib phases | Make each phase a transaction-like checkpoint |
| Core major loop | Core stage accepts one target version | Iterate through immediate major versions and complete the full cycle at each one |
| Configuration export | No orchestrator stage explicitly runs `config:export` | Export and commit configuration after each validated phase |
| Cleanup scope | Implemented cleanup removes `upgrade_status`; Rector cleanup is not guaranteed by the current app cleanup path | Define whether Rector is temporary and remove its exact Composer packages/config when appropriate |
| Final runtime checks | Upgrade Status is the primary gate | Add project tests, cache rebuild, `drush status`, and smoke checks where available |
| Report source | Report generation consumes accumulated agent evidence | Extend evidence to include exact versions, patches added/removed, config exports, commits, backups, and skipped work |

## Failure And Recovery Rules

- A failed environment or backup stage stops the workflow.
- A failed validation never authorizes a commit.
- A Composer conflict is resolved before retrying; do not blindly repeat it.
- A patch conflict is rolled back or isolated before trying another patch.
- A failed phase leaves its backup and checkpoint available.
- Restoration is destructive and requires explicit user confirmation.
- A retained backup is never deleted automatically.
- Unresolved work goes into the report with the exact target, error, attempts, and next action.

## Final Checklist

- [ ] Git was clean before mutation.
- [ ] Original branch and upgrade branch were recorded.
- [ ] Environment and PHP/core compatibility were verified.
- [ ] The immediate next major was selected without skipping a major.
- [ ] Drush, Upgrade Status, and Rector availability were verified.
- [ ] A baseline scan and package inventory were saved.
- [ ] Custom modules and themes were checked for existence before Rector.
- [ ] Custom/theme changes passed independent validation.
- [ ] Contrib updates ran in patch, minor, then one-at-a-time major phases.
- [ ] Each mutation phase had a backup, database update, validation, config export, and commit.
- [ ] Core was upgraded one major at a time.
- [ ] Final validation, tests, cache rebuild, and status checks passed or were reported.
- [ ] Temporary tooling cleanup was explicit and validated.
- [ ] The root Markdown report contains exact versions, patches, commits, backups, and pending work.
- [ ] The user received the summary and report path.
