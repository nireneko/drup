# drup — Drupal Upgrade Automation

**CLI + MCP harness that automates Drupal 8/9/10 → 11 migration, combining deterministic analysis with AI agents.**

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/tests-72%2F72-brightgreen" alt="Tests">
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
| **Orchestrator skill** | `~/.claude/skills/drup/SKILL.md` | 7-phase pipeline. Invoked with `/drup <path>` |
| **Sub-agents** | `~/.claude/agents/drup-preflight.md` | Preflight: detects environment, installs dependencies |
| | `~/.claude/agents/drup-contrib.md` | Contrib modules: releases, patches, commits |
| | `~/.claude/agents/drup-custom.md` | Custom code: refactor with retry and escalation |
| | `~/.claude/agents/drup-theme.md` | Themes: twig/.theme deprecations |
| **MCP server** | `~/.claude/.mcp.json` | Registers `drup mcp` as an MCP server with 7 tools |

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

The MCP server communicates over **stdio** (JSON-RPC 2.0). No port needed, no network needed — the agent launches the `drup mcp` process and communicates via stdin/stdout. The 7 exposed tools are documented in [MCP Tools](#mcp-tools).

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

## The Pipeline (7 phases)

```
[0. Preflight]      [1. Static]           [2. Resolution]           [3. Self-healing]      [4. Output]
clean git           composer require      contrib:                  re-analyze + phpstan   branch + commits
drush status    →   upgrade_status    →   · D11 release?        →   · ok → next        →   final report
detect core         drupal-rector         · issue patch?            · fails → retry        human pending list
version             (autofix ~80%)        custom: agent edits       · ×2 → escalate model  (PR optional)
```

### Phase 0 — Preflight
Verifies clean git, composer/drush available, core version. Installs missing dependencies (`upgrade_status`, `drupal-rector`, `phpstan-drupal`).

### Phase 1 — Rector (0 tokens)
Runs `drupal-rector` with D11 rule sets over custom modules and themes. Resolves ~80% of standard deprecations deterministically. Atomic commit.

### Phase 2 — Contrib Modules
For each contrib module with errors:
1. `contrib_check` → queries `updates.drupal.org/release-history` (Drupal core's Update module canonical feed)
2. D11-compatible release available? → `composer require` → commit
3. No release? → searches Drupal.org issues (api-d7 + HTML scraper) → prioritizes RTBC patches → downloads and applies
4. No patches? → the agent generates a `.patch` with the fix
5. **Validation gate**: `validate(scope=contrib, module=X)` → 0 errors = commit, >0 = retry

### Phase 3 — Custom Code
For each custom file with deprecations:
1. Agent reads the file + error message (±30 lines)
2. Applies the minimal fix
3. `validate(scope=custom, file=Y)` → 0 errors? → commit
4. Fails? → retries with validator feedback (×2)
5. Still failing? → escalates to a more powerful model (×1)
6. Still failing? → pending list for human review

### Phase 4 — Final Validation
`validate(global)` → `total_errors == 0`? → final report. Errors remain → iterates with the appropriate sub-agent.

---

## Validation Gates (strict rules)

The orchestrator NEVER trusts a sub-agent's self-declaration:

| Rule | Description |
|---|---|
| **External validation** | The orchestrator runs `validate` — the sub-agent never validates its own work |
| **No self-approval** | A sub-agent saying "done" means nothing. Only `validate` == 0 counts |
| **Retry with evidence** | If it fails, the same sub-agent receives the validator's output as feedback |
| **Max 2 retries** | Per scope. Then escalates model (haiku → sonnet). Then human list |
| **Phase gate** | No phase advances until ALL items pass validation |
| **Commit only post-gate** | Each commit runs ONLY after `validate` == 0 |

---

## Architecture

### The binary (`drup`)

```
cmd/drup/main.go              # entrypoint
internal/
  app/          # CLI dispatch (11 commands) + MCP tool handlers
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

`drup`'s MCP server exposes 7 tools with JSON types and schemas. It's the standard protocol that connects the binary with any compatible agent:

```
Claude Code ───┐
OpenCode ──────┼── MCP (stdio) ── drup mcp ── deterministic tools
Codex ─────────┘
```

---

## MCP Tools

| Tool | Input | Output |
|---|---|---|
| `scan` | `project_path` | JSON: classified errors (contrib/custom/theme/core) |
| `autofix` | `project_path` | JSON: rector summary + remaining errors |
| `contrib_check` | `module_machine_name` | `{ has_d11_release, latest_version, compatible_branches }` |
| `issue_patches` | `issue_nid` or `module_name` | `[{ url, status (RTBC/NR), date, is_patch }]` |
| `apply_patch` | `patch_url, project_path` | `{ applied, commit_hash, error }` |
| `validate` | `project_path` | `{ total_errors, errors[] }` |
| `create_patch` | `module_name, deprecation_details` | `{ patch_path, applied }` |

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
go test ./...           # 72 tests
go vet ./...            # static analysis
```

Test structure: table-driven, fixtures in `testdata/`, package-level variables for mocking subprocesses.

---

## License

MIT
