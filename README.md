# drup — Drupal Upgrade Automation

**CLI + MCP harness that automates Drupal 8/9/10 → 11 migration, combining deterministic analysis with AI agents.**

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/tests-163%20passing-brightgreen" alt="Tests">
</p>

---

## What it does

Migrating a Drupal site to the next major version is a **mechanical but manual** process that repeats project after project:

1. Install `upgrade_status` and `drupal-rector`
2. Run deprecation analysis
3. Check releases for each contrib module on Drupal.org
4. Look for patches in issues for modules without a compatible release
5. Refactor custom code (modules and themes)
6. Validate that everything compiles
7. Generate a report

`drup` automates all of this. **80% of the work is deterministic** (rector, releases, patches) and is resolved without spending a single AI token. The remaining 20% (complex custom code) is handled by an AI agent with validation and retry tooling.

```bash
# Full pipeline with a single command:
drup fix /path/to/drupal-project

# Or step by step from Claude Code:
/drup /path/to/drupal-project
```

---

## Quick Start

### Installation

```bash
# Option 1: Go install
go install github.com/nireneko/drup/cmd/drup@latest

# Option 2: from source
git clone git@github.com:nireneko/drup.git
cd drup && go build -o /usr/local/bin/drup ./cmd/drup

# Option 3: prebuilt binary (direct download)
# Available at https://github.com/nireneko/drup/releases (coming soon)
```

### Set up AI agents

```bash
drup install
```

This detects which agents you have installed (Claude Code, OpenCode, Codex) and writes the skills, sub-agents, and MCP configuration into their native directories.

### Update

```bash
drup upgrade      # updates the binary
drup sync         # re-applies skills to agents (after upgrade or template changes)
```

---

## Integration with AI agents

When running `drup install`, the binary detects which agents you have installed and writes the necessary files into their native directories.

### Claude Code

| What gets installed | Path | Purpose |
|---|---|---|
| **Orchestrator skill** | `~/.claude/skills/drup/SKILL.md` | 7-stage pipeline. Invoked with `/drup <path>`. Pure coordinator: zero Bash/MCP calls, only dispatches sub-agents and talks to the user |
| **Sub-agents** | `~/.claude/agents/drup-preflight.md` | Preflight: detects environment, installs dependencies |
| | `~/.claude/agents/drup-rector.md` | Rector: deterministic auto-fix (`autofix`) on custom modules/themes |
| | `~/.claude/agents/drup-contrib.md` | Contrib modules: releases, patches, core-version bump, commits |
| | `~/.claude/agents/drup-custom.md` | Custom code: refactor with retry and escalation |
| | `~/.claude/agents/drup-theme.md` | Themes: twig/.theme deprecations |
| | `~/.claude/agents/drup-validator.md` | Validator: owns every `scan`/`validate`/`upgrade_scan` call — the only agent allowed to confirm a gate; generates the final report |
| **MCP server** | `~/.claude/.mcp.json` | Registers `drup mcp` as an MCP server with 20 tools |

**Usage**: open Claude Code in the Drupal project and run:

```
/drup /path/to/project
```

Claude Code loads SKILL.md, connects to the MCP server, and runs the 7 pipeline phases. Sub-agents isolate work per module/file to avoid saturating the context.

**Default model**: the skill uses the session's active model. To force a specific model, set it in `~/.config/drup/config.yaml` (see [Configuration](#configuration)).

### OpenCode

| What gets installed | Path |
|---|---|
| **Orchestrator skill** | `~/.config/opencode/skills/drup/SKILL.md` |
| **Sub-agents** | `~/.config/opencode/agents/drup-*.md` |
| **MCP server** | `~/.config/opencode/mcp.json` |

**Usage**: in OpenCode, run `/drup <path>` or let the skill load automatically when you mention "Drupal upgrade" or "migrate Drupal".

### Codex

| What gets installed | Path |
|---|---|
| **Orchestrator skill** | `~/.codex/skills/drup/SKILL.md` |
| **Sub-agents** | `~/.codex/agents/drup-*.md` |
| **MCP server** | `~/.codex/mcp.json` |

**Usage**: in Codex CLI, run `/drup <path>`.

### The MCP server

All 3 agents share the same MCP configuration. The `.mcp.json` (or `mcp.json`) file registers the server:

```json
{
  "mcpServers": {
    "drup": {
      "command": "/path/to/binary/drup",
      "args": ["mcp"]
    }
  }
}
```

The MCP server communicates over **stdio** (JSON-RPC 2.0). No port needed, no network needed — the agent launches the `drup mcp` process and communicates via stdin/stdout. The 20 exposed tools are documented in [MCP Tools](#mcp-tools) and in [`docs/mcp-tools.md`](docs/mcp-tools.md).

### Verifying the installation

```bash
# See which agents drup detected
drup install
# Output: Installing drup to claude... done
#         Installing drup to opencode... done

# Force re-sync (after a binary update)
drup sync
```

---

## Full workflow: from Drupal 10 to Drupal 11

### Step by step with the CLI

```bash
# 1. Preflight: detects version, installs dependencies
drup preflight /path/to/project

# 2. Scan: initial deprecation analysis
drup scan /path/to/project

# 3. Fix: full pipeline
drup fix /path/to/project
#    ├── runs drupal-rector (autofix ~80%)
#    ├── for each contrib module: looks for D11 release or RTBC patch → applies → commit
#    ├── for each custom file: shows errors for the agent to resolve
#    └── final validation → report

# 4. Report: summary of everything done
drup report /path/to/project
```

### From Claude Code (orchestrated by skill)

```
/drup /path/to/project
```

The `/drup` skill runs the full pipeline in 7 phases with **validation gates**: each phase is validated before moving on. If something fails, it retries with a more powerful model. If it keeps failing, it goes to the pending list for human review.

### Individual commands

```bash
drup contrib check webform       # does it have a D11-compatible release?
drup issue patches 3412345       # patches from a Drupal.org issue (clean JSON)
drup mcp                         # MCP server (for AI agents)
```

---

## The Pipeline (7 stages)

The orchestrator itself never executes anything — it only reads sub-agent reports, dispatches the next sub-agent, and talks to the user. All the steps below are carried out by dedicated sub-agents calling MCP tools; see [Deterministic work vs. orchestration](#deterministic-work-vs-orchestration).

```
1. PREFLIGHT   2. DEP CHECK    3. RECTOR       4. CONTRIB LOOP    5. CUSTOM/THEME LOOP   6. FINAL VALIDATION   7. REPORT
drup-preflight drup-validator  drup-rector     drup-contrib       drup-custom/           drup-validator         drup-validator
env + core ver  confirms deps  autofix (0 tok)  per module:         drup-theme             total_errors == 0?    generate_report
                                                 release/patch/      per file:              else re-enter          → UPGRADE-REPORT.md
                                                 core-upgrade         validate → commit       loop 4/5
```

### Stage 1 — Preflight
`drup-preflight` verifies clean git, composer/drush availability, and core version, and installs missing dev dependencies (`upgrade_status`, `drupal-rector`, `phpstan-drupal`). An `unsupported` environment result is a **terminal state** — the pipeline stops and reports to the user; it never proceeds to later stages.

### Stage 2 — Dep Check
`drup-validator` confirms Stage 1's dependency install actually took effect. `drup-preflight` never confirms its own work.

### Stage 3 — Rector (0 tokens)
`drup-rector` runs `drupal-rector` with D11 rule sets over custom modules and themes, resolving ~80% of standard deprecations deterministically. `drup-validator` confirms `total_errors == 0` before `drup-rector` commits.

### Stage 4 — Contrib Loop
For each contrib module with errors, `drup-contrib`:
1. `contrib_check` / `contrib_upgrade_path` → is there a D11-compatible release?
2. No release? → `issue_patches` + `patch_reconcile` (analysis-only, JSON api-d7) → apply the best patch, or `create_patch` if none exists
3. Core-version bump needed? → `core_upgrade_check` (preview) then `core_upgrade_apply` (requires clean tree, creates a git checkpoint before mutating `composer.json`; supports `dry_run`)
4. `drup-validator` confirms `total_errors == 0` for that module before `drup-contrib` commits

### Stage 5 — Custom/Theme Loop
For each custom PHP file or twig/theme file with deprecations, `drup-custom` / `drup-theme` applies the minimal fix; `drup-validator` confirms per-file before a commit. Failures retry with validator feedback (max 2), then escalate model (haiku → sonnet), then go to the pending human list.

### Stage 6 — Final Validation
`drup-validator` re-runs a global scan. `total_errors == 0` → Stage 7. Otherwise the remaining errors are classified and routed back into Stage 4 or 5 for just those items.

### Stage 7 — Report
`drup-validator` calls `generate_report`, producing `UPGRADE-REPORT.md` with a summary, per-module/per-file results, and the pending human list.

---

## Validation Gates (strict rules)

The orchestrator NEVER trusts a sub-agent's self-declaration, and it never validates anything itself — only `drup-validator` does:

| Rule | Description |
|---|---|
| **External validation** | Only `drup-validator` calls `scan`/`validate`/`upgrade_scan`. No other sub-agent — and never the orchestrator — validates a sub-agent's own work |
| **No self-approval** | A sub-agent saying "done" means nothing. Only a `drup-validator` report showing `total_errors == 0` counts |
| **Retry with evidence** | If it fails, the same sub-agent receives the validator's output as feedback |
| **Max 2 retries** | Per scope on haiku. Then escalates model (haiku → sonnet). Then pending human list |
| **Phase gate** | No stage advances until ALL items pass validation |
| **Commit only post-gate** | Each commit runs ONLY after `drup-validator` reports 0 errors for that exact scope/target |

See [`openspec/changes/drupal-upgrade-orchestrator/specs/`](openspec/changes/drupal-upgrade-orchestrator/specs/) for the full spec-driven requirements behind this flow, and [`openspec/changes/drupal-upgrade-orchestrator/design.md`](openspec/changes/drupal-upgrade-orchestrator/design.md) for the architecture decisions.

---

## Architecture

### The binary (`drup`)

```
cmd/drup/main.go              # entrypoint
internal/
  app/          # CLI dispatch (11 commands) + MCP tool handlers
  envdetect/    # Drupal dev environment detection (ddev, lando, docker, direct)
  exec/         # subprocess runner (composer, drush, rector, phpstan, git)
  scan/         # upgrade_status:analyze JSON parser
  drupalorg/    # release-history XML + api-d7 + issue scraper
  patch/        # .patch download, git apply, composer.json registration
  gitops/       # git clean check, atomic commits, branches
  report/       # JSON + Markdown report generation
  mcp/          # MCP server (JSON-RPC 2.0, stdio)
  packaging/    # skill/agent/MCP templates (go:embed)
  installer/    # agent detection, asset writing, backup
  state/        # state.json with installed agents, pending_sync, models
  update/       # self-upgrade with checksum + atomic replacement
```

### The orchestrator (agent skills)

The binary only does deterministic work. The full flow is executed by an **AI agent** (Claude Code, OpenCode, Codex) following the instructions of a `SKILL.md`:

- **`/drup` skill**: 7-phase pipeline with validation gates
- **Sub-agents**: `drup-preflight`, `drup-contrib`, `drup-custom`, `drup-theme` — isolate context per module/file to avoid saturating the orchestrator's window

### The bridge (MCP)

`drup`'s MCP server exposes 20 tools with JSON types and schemas. It's the standard protocol that connects the binary with any compatible agent:

```
Claude Code ───┐
OpenCode ──────┼── MCP (stdio) ── drup mcp ── deterministic tools
Codex ─────────┘
```

---

## MCP Tools

### Core Tools (7 original)

| Tool | Input | Output | Purpose |
|---|---|---|---|
| `scan` | `project_path` | Classified errors | Initial deprecation analysis via `upgrade_status:analyze` |
| `autofix` | `project_path` | Rector summary + remaining errors | Runs `drupal-rector` on custom code |
| `contrib_check` | `module_machine_name` | `{ has_d11_release, latest_version, compatible_branches }` | Checks Drupal.org releases for a module |
| `issue_patches` | `issue_nid` or `module_name` | `[{ url, status, date, is_patch }]` | Searches Drupal.org issues for patches |
| `apply_patch` | `patch_url, project_path` | `{ applied, commit_hash, error }` | Downloads and applies a .patch safely |
| `validate` | `project_path, scope, module, file` | `{ total_errors, errors[] }` | Re-runs analysis with scope filtering |
| `create_patch` | `module_name, deprecation_details` | `{ patch_path, applied }` | Generates a .patch from deprecation analysis |

### New Tools (10 added)

| Tool | Input | Output | Purpose |
|---|---|---|---|
| `detect_env` | `project_path, force_detect?` | `{ environment, command_prefix, detected_at }` | Detects ddev/lando/docker/direct env |
| `composer_require` | `project_path, package, dev?, no_update?` | `{ success, installed_version, stdout, stderr }` | Safe `composer require` with dry-run |
| `drush_exec` | `project_path, command, args?, format?` | `{ success, output, stderr, exit_code }` | Safe drush execution with blocklist |
| `upgrade_scan` | `project_path, scope?, module?` | `{ total_errors, modules[], install_status }` | Atomic install→enable→analyze→filter |
| `contrib_upgrade_path` | `module, current_drupal, target_drupal` | `{ recommended_upgrade, alternative_versions[] }` | Finds recommended version for next major |
| `patch_status` | `project_path, patch_url?, package?` | `{ is_applied, commit_hash, registered_in_composer }` | Checks if a patch is already applied |
| `patch_rollback` | `project_path, patch_url, package` | `{ success, reverted_commit, error }` | Reverts a patch cleanly |
| `generate_report` | `project_path, report_type?, include_*?` | `{ json_path, markdown_path, summary }` | Generates JSON + Markdown upgrade report |
| `module_info` | `module, include_maintainers?, include_deps?` | `{ title, downloads, last_release, maintainers, deps }` | Module metadata from Drupal.org |
| `drupal_version_matrix` | `drupal_version?, php_version?` | `{ php_requirements, supported_until, upgrade_path }` | Drupal/PHP compatibility lookup |

### Core Upgrade / Patch Reconcile (3 added)

| Tool | Input | Output | Purpose |
|---|---|---|---|
| `core_upgrade_check` | `project_path` | `{ current_version, next_version, composer_patch_preview, supported }` | Read-only: next major version + composer.json patch preview |
| `core_upgrade_apply` | `project_path, target_version, dry_run` | `{ success, report, rollback_checkpoint, stderr }` | Requires a clean git tree; `dry_run` previews only; on apply, commits a git checkpoint before mutating `composer.json` |
| `patch_reconcile` | `module_machine_name, current_patch_url` | `{ newer_patches[], is_still_needed, recommendation }` | Analysis-only: is an already-applied patch obsolete, still needed, or superseded? |

---

## Deterministic work vs. orchestration

`drup` splits responsibility strictly along one rule: **all deterministic work happens in the Go binary/MCP tools; the AI agent only orchestrates and talks to the user.**

- **Go/MCP tools** (this binary): version checking, composer.json mutation, patch analysis, git operations, drush/composer execution, report generation — all 20 tools above run without spending a single AI token.
- **Agent orchestration** (`SKILL.md` + sub-agents, installed via `drup install`): the `/drup` skill is a pure coordinator with zero execute permission — it never calls Bash or an MCP tool directly. It only dispatches sub-agents (which do call the MCP tools) and relays their structured reports to the user.

This means `/drup <path>` is **not a shell command** — it is a slash command that loads an AI agent skill in Claude Code, OpenCode, or Codex. See [`openspec/changes/drupal-upgrade-orchestrator/specs/`](openspec/changes/drupal-upgrade-orchestrator/specs/) for the requirements this split is built on.

---

## Configuration

`~/.config/drup/config.yaml` (optional):

```yaml
agents:
  claude-code:
    skills:
      drup:
        model: claude-sonnet-4
    agents:
      drup-contrib:
        model: claude-haiku-3-5
      drup-custom:
        model: claude-haiku-3-5
  opencode:
    profiles:
      drup-orchestrator:
        default: openrouter/qwen/qwen3-30b-a3b:free
```

If you don't configure anything, `drup` uses sensible defaults (cheap for mechanical work, strong for reasoning).

---

## Commands

| Command | Description |
|---|---|
| `drup init` | Generates `drup.yaml` in the current directory |
| `drup scan <path>` | Initial deprecation analysis (JSON) |
| `drup fix <path>` | Full pipeline: preflight + rector + contrib + custom + validation |
| `drup preflight <path>` | Detects version, checks git/composer/drush, installs dependencies |
| `drup contrib check <module>` | D11 release or patch available? |
| `drup issue patches <nid>` | Patches from a Drupal.org issue |
| `drup report <path>` | Current status report vs D11 |
| `drup mcp` | MCP server over stdio (for AI agents) |
| `drup install` | Detects agents and writes skills + MCP config |
| `drup sync` | Re-applies skills to installed agents |
| `drup upgrade` | Updates the binary + syncs skills |
| `drup version` | Current version |

Global flags: `--json`, `--force` (dirty git), `--dry-run`.

---

## Roadmap

| Version | Scope |
|---|---|
| **v0.1** ✅ | Go binary: preflight + scan + fix + contrib + report. 72 tests. |
| v0.2 | Full pipeline with agent skills. Sub-agents with isolation. Working self-upgrade. |
| v0.3 | Standalone mode with LLM (no external agent). RAG over Drupal change records. |
| v0.4 | Chained 8→9→10→11. PR creation. CI mode. |

---

## Development

```bash
git clone git@github.com:nireneko/drup.git
cd drup

go build ./cmd/drup     # build
go test ./...           # all tests (163+)
go vet ./...            # static analysis
```

Test structure: table-driven, fixtures in `testdata/`, package-level variables for mocking subprocesses.

---

## License

MIT
