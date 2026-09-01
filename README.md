# drup

**A safety-first operations layer for Drupal major-version upgrades.**

`drup` combines a Go CLI, a local MCP server, and installed agent roles to make Drupal upgrade work observable, checkpointed, and recoverable. It automates deterministic tasks—environment detection, Upgrade Status analysis, Rector execution, Drupal.org release and issue lookups, patch handling, backups, and evidence-bound checkpoints—while keeping material decisions and destructive recovery under operator control.

It is designed for moving a Composer-managed Drupal project through its **next** supported major version, not for treating a major upgrade as an unreviewed bulk rewrite.

## What drup solves

- Establishes a reproducible upgrade workspace with preflight checks and structured scan output.
- Runs Drupal Rector only where it belongs: custom modules and themes.
- Finds compatible contrib releases and Drupal.org patch candidates.
- Persists run state, inventories, checkpoint plans, operation records, and reports under the project’s `.drup/` directory.
- Creates local database-and-files backup manifests before guarded mutations.
- Exposes the same guarded operations to Claude Code, OpenCode, and Codex over MCP.

## What it does not promise

- It does **not** guarantee that every Upgrade Status finding is automatically fixable. Some findings require project-specific engineering or product decisions.
- It does **not** skip Drupal majors. Core planning and guarded execution are immediate-major operations.
- It does **not** make `drup fix` an end-to-end upgrade pipeline. `fix` runs Rector on custom modules/themes and then re-runs the scan; it does not update contrib, upgrade core, commit changes, or restore a backup.
- It does **not** restore or delete a backup automatically.

## Install and verify

Prerequisites: Go `1.25.10` or newer to build from source, plus a Drupal project with Composer when operating on a site.

```bash
# Install from the module.
go install github.com/nireneko/drup/cmd/drup@latest

# Or build a repository checkout.
git clone https://github.com/nireneko/drup.git
cd drup
go build -o drup ./cmd/drup

# Verify the binary and inspect the command surface.
drup version
drup help
```

The `version` command reports `dev-version` for a locally built binary unless build metadata supplies a release version.

## Start safely with the CLI

Run the read-oriented checks first from a Drupal project path:

```bash
PROJECT=/absolute/path/to/drupal-project

drup preflight "$PROJECT"
drup scan "$PROJECT"
drup contrib webform
drup issue 1234567
```

A typical controlled change sequence is:

```bash
# Rector only, then a fresh Upgrade Status scan.
drup fix "$PROJECT"

# Review the changed files and scan output before continuing.
git -C "$PROJECT" diff

drup compat-fix "$PROJECT" --dry-run
drup report "$PROJECT"
```

Use `drup help` before a mutating command. Some commands require a project-local session, a fresh backup, a durable run ID, or an explicit confirmation; the MCP workflow is the intended interface for coordinating those guards. See [the CLI reference](docs/cli-reference.md) and [the upgrade workflow](docs/upgrade-workflow.md).

## Use with an agent

Install the agent assets while the target agent applications are closed, then restart them:

```bash
drup install
# To register a server that refuses mutations:
drup install --locked
```

The installer detects available Claude Code, OpenCode, and Codex installations; it writes the `drup` skill and specialist agents, merges only drup’s MCP entry, and records installation state. `drup sync` reapplies the rendered assets after an upgrade. `drup uninstall --dry-run` shows removal candidates before uninstalling.

Once installed, describe the Drupal upgrade to the agent and provide the project path. The installed coordinator is responsible for sequencing the specialist roles; the MCP server is stdio-only and has no listening network port. See [Agent integration](docs/getting-started.md#agent-integration) and [the multi-agent contract](docs/multiagent-contracts.md).

## Safety model

`drup` treats a major upgrade as a sequence of durable checkpoints, not one shell command:

- **Canonical project identity:** sessions resolve and bind one absolute, symlink-resolved Drupal root.
- **Guarded mutation:** mutating operations can require an open session, a fresh backup, mutation-budget evidence, and run-state authority. Locked MCP mode refuses mutating calls.
- **Independent evidence:** workflow state records sanitized hashes and summaries rather than trusting agent prose or storing raw secret-bearing output.
- **Recovery without surprise:** backups are retained; restore requires explicit confirmation and uses a transactional filesystem recovery journal. An unresolved restore journal blocks new run-authorized mutations.
- **Scoped commits:** the checkpoint commit operation binds a candidate diff to independently recorded validation evidence.

These guarantees are operational guardrails, not a substitute for testing, reviewing the diff, or making a database recovery plan. Read [Safety and recovery](docs/safety-and-recovery.md) before enabling mutations.

## Architecture in brief

- `cmd/drup` is the process entry point and interruption boundary.
- `internal/app` composes the CLI and MCP handlers.
- `internal/mcp` publishes ToolSpec-defined MCP contracts; generated catalog documentation and JSON are checked for drift.
- `internal/runstate`, `internal/operation`, `internal/session`, and `internal/audit` provide durable workflow authority and mutation controls.
- `internal/backup`, `internal/inventory`, `internal/report`, and `internal/coreupgrade` provide recovery, evidence, reporting, and immediate-major core operations.
- `internal/packaging` renders platform-specific agent skills and configuration from repository templates.

See [Architecture](docs/architecture.md) for the boundaries and persistence model.

## Documentation

| Need | Read |
|---|---|
| First local or agent-backed run | [Getting started](docs/getting-started.md) |
| Exact CLI commands and side effects | [CLI reference](docs/cli-reference.md) |
| Controlled major-upgrade lifecycle | [Upgrade workflow](docs/upgrade-workflow.md) |
| Safety controls, backups, restore and recovery | [Safety and recovery](docs/safety-and-recovery.md) |
| Global and project configuration | [Configuration](docs/configuration.md) |
| Packages, persistence, MCP, and agent rendering | [Architecture](docs/architecture.md) |
| MCP tool catalog | [MCP tools](docs/mcp-tools.md) |
| Model assignment overrides | [Model configuration](docs/model-configuration.md) |
| Agent report contracts | [Multi-agent contracts](docs/multiagent-contracts.md) |
| Building, testing, and maintaining docs | [Development](docs/development.md) |

## Contributing and validating documentation

Documentation is part of the product contract. Verify claims against `internal/app/app.go`, command tests, generated MCP artifacts, and rendered packaging templates; do not document a proposed capability as shipped behavior.

```bash
# Markdown whitespace and patch integrity.
git diff --check

# Test the codebase and confirm generated MCP catalog bytes are current.
GOCACHE=/tmp/drup-go-build go test ./...
GOCACHE=/tmp/drup-go-build go run ./cmd/mcp-catalog-gen --check
```

See [Development](docs/development.md) for focused test commands and the generated-catalog workflow.

## MCP catalog

The following generated summary is a compact compatibility surface. For tool-by-tool operating guidance and schemas, use [`docs/mcp-tools.md`](docs/mcp-tools.md) and MCP `tools/list`.

<!-- BEGIN GENERATED MCP CATALOG -->

## MCP catalog (generated)

`ToolSpec` is the only source for these 45 implemented MCP contracts: schemas, effects, guards, and stub visibility. Regenerate with `go generate ./...`; CI rejects byte drift. 41 tools are available as transport stubs. Planned or obsolete tools are intentionally absent from this runtime catalog.

| Tool | Required input | Effect | Guard contract | Stub |
|---|---|---|---|---|
| `apply_patch` | `patch_url, project_path, composer_package, description, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `autofix` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `checkpoint_commit` | `project_path, run_id, commit_strategy, scope, paths, validation_hash, target, request_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `checkpoint_execute` | `project_path, run_id, phase, target_major, targets, paths, request_id` | `mutating` | session + mutation_cap + run | yes |
| `cleanup` | `project_path, validate_passed, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `composer_require` | `project_path, package, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_allow_lenient` | `project_path, packages, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_check` | `module_machine_name` | `read_only` | none | yes |
| `contrib_compat_patch` | `project_path, module_machine_name, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `contrib_plan` | `project_path, run_id` | `workflow` | none | yes |
| `contrib_upgrade_path` | `module_machine_name, current_drupal_version, target_drupal_version` | `read_only` | none | yes |
| `core_upgrade_apply` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `core_upgrade_check` | `project_path` | `read_only` | none | yes |
| `create_patch` | `module_name, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `custom_compat_fix` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `detect_env` | `project_path` | `read_only` | none | yes |
| `drupal_version_matrix` | `—` | `read_only` | none | yes |
| `drush_exec` | `project_path, command, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `generate_report` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `inventory_capture` | `project_path, run_id, stage` | `workflow` | none | yes |
| `issue_patches` | `—` | `read_only` | none | yes |
| `module_info` | `module_machine_name` | `read_only` | none | yes |
| `module_release_info` | `module_machine_name` | `read_only` | none | yes |
| `operation_reconcile` | `project_path, request_id, evidence_path, run_id` | `mutating` | run | yes |
| `patch_reconcile` | `module_machine_name, current_patch_url` | `read_only` | none | yes |
| `patch_rollback` | `project_path, patch_url, composer_package, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `patch_status` | `project_path, patch_url, composer_package` | `read_only` | none | yes |
| `pipeline_status` | `project_path` | `read_only` | none | yes |
| `prepare_upgrade_status` | `project_path, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `restore_check` | `project_path, backup_id` | `read_only` | none | yes |
| `restore_recover` | `project_path, journal_id, confirm, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | yes |
| `run_abandon` | `project_path, run_id, reason` | `workflow` | none | yes |
| `run_block` | `project_path, run_id, reason` | `workflow` | none | yes |
| `run_confirm` | `project_path, run_id, action` | `workflow` | none | yes |
| `run_create` | `project_path, target_major, commit_strategy, scope` | `workflow` | none | yes |
| `run_record` | `project_path, run_id, action, evidence` | `workflow` | none | yes |
| `run_status` | `project_path` | `workflow` | none | yes |
| `scan` | `project_path` | `read_only` | none | yes |
| `session_open` | `project_path` | `read_only` | none | yes |
| `test_backup_create` | `project_path, request_id, run_id` | `mutating` | session + mutation_cap + run | no |
| `test_backup_delete` | `project_path, backup_id, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | no |
| `test_backup_list` | `project_path` | `read_only` | none | no |
| `test_backup_restore` | `project_path, backup_id, confirm, plan_id, request_id, run_id` | `mutating` | session + backup + mutation_cap + run | no |
| `upgrade_scan` | `project_path` | `read_only` | none | yes |
| `validate` | `project_path` | `read_only` | none | yes |

**Side-effect assertions:** `read_only` changes no project or workflow state; `workflow` changes only persisted run authority; `mutating` requires the listed guard evidence before its handler runs. The manual tool dictionary below is explanatory and must not weaken this contract.

<!-- END GENERATED MCP CATALOG -->
