# Design: drup-orchestrator-delegation-fix

## Technical Approach

Three coordinated changes: (1) wire `drup upgrade-core` CLI command around existing `internal/coreupgrade` logic, adding composer/drush execution; (2) rewrite SKILL.md template from sub-agent dispatch to sequential `drup <stage>` CLI commands; (3) replace per-platform agent definitions with thin bootstrap files that reference SKILL.md.

## Architecture Decisions

### Decision: CLI command wraps existing coreupgrade package

| Option | Tradeoff | Decision |
|--------|----------|----------|
| New binary `drup-upgrade-core` | Separate release cycle, but duplicates exec wiring | **Rejected** |
| Add `RunUpgradeCore` in `internal/app/commands.go` calling `coreupgrade.Apply` + exec steps | Follows existing pattern (RunPreflight, RunScan), reuses package | **Chosen** |

**Rationale**: `internal/coreupgrade` already has `Apply`, `NextMajor`, `PreviewComposerPatch`, `Rollback`. The CLI command adds the missing orchestration: backup → apply → `composer require` → `drush updb` → `drush status` verify.

### Decision: SKILL.md is a single cross-platform file

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Per-platform SKILL.md variants | More maintenance, platform drift | **Rejected** |
| Single SKILL.md with `drup <stage>` commands, bootstraps just reference it | One source of truth, any AI can follow | **Chosen** |

**Rationale**: The spec requires zero platform primitives. Each pipeline stage maps to a `drup` CLI command callable via shell. Bootstrap files (CLAUDE.md, copilot-instructions.md, opencode.json skill entry) only say "load SKILL.md".

### Decision: Bootstrap files replace agent definitions

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Keep per-platform agent/ directory with sub-agent .md files | OpenCode-specific `task()` pattern, violates cross-platform goal | **Rejected** |
| Remove agents/ from templates, add bootstrap references | Simpler, cross-platform, SKILL.md is authority | **Chosen** |

**Rationale**: Sub-agent dispatch is an OpenCode primitive. The new model: AI reads SKILL.md, calls `drup <stage>` via shell, checks exit codes. No agent definitions needed.

## Data Flow

```
AI reads SKILL.md (loaded via bootstrap)
  │
  ├── drup preflight          → exit 0/1
  ├── drup scan <path>        → JSON report
  ├── drup fix <path>         → rector auto-fix
  ├── drup contrib <module>   → D11 compat check
  ├── drup issue <nid>        → patch links
  ├── drup upgrade-core <ver> → composer.json patch + composer require + drush updb + verify
  ├── drup scan <path>        → re-validate
  └── drup report <path>      → final report
```

`drup upgrade-core` internal flow:
```
Read composer.json → detect current constraint
  → PreviewComposerPatch (diff preview)
  → backup composer.json → composer.json.bak
  → applyConstraint (rewrite drupal/core* entries)
  → git checkpoint commit
  → exec: composer require drupal/core-recommended:^<N> --update-with-dependencies
  → exec: drush updb -y
  → exec: drush status --format=json → verify version matches target
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/app/app.go` | Modify | Add `upgrade-core` case to Run() switch |
| `internal/app/commands.go` | Modify | Add `RunUpgradeCore(args)` — parses target version + `--dry-run`, calls coreupgrade.Apply, then runs composer/drush/verify steps |
| `internal/packaging/templates/opencode/SKILL.md` | Modify | Rewrite: remove sub-agent roster, replace with 8-stage CLI pipeline |
| `internal/packaging/templates/claude/SKILL.md` | Modify | Same rewrite (content identical to opencode) |
| `internal/packaging/templates/codex/SKILL.md` | Modify | Same rewrite (content identical to opencode) |
| `internal/packaging/templates/opencode/agents/` | Delete | Remove all 6 sub-agent .md files |
| `internal/packaging/templates/claude/agents/` | Delete | Remove all sub-agent .md files |
| `internal/packaging/templates/codex/agents/` | Delete | Remove all sub-agent .md files |
| `internal/packaging/templates/claude/CLAUDE.md` | Create | Bootstrap: "Load SKILL.md from this directory and follow the pipeline" |
| `internal/packaging/templates/codex/copilot-instructions.md` | Create | Bootstrap: "Load SKILL.md and follow the pipeline" |
| `internal/packaging/packaging.go` | Modify | Add `{{SKILL_PATH}}` placeholder substitution for bootstrap templates |
| `internal/installer/installer.go` | Modify | Add bootstrap file writing per adapter (CLAUDE.md at project root for Claude, copilot-instructions.md under .github/ for Codex, skill entry in opencode.json for OpenCode) |

## Interfaces / Contracts

### `drup upgrade-core` CLI

```
drup upgrade-core <target-version> [--dry-run]

Args:
  <target-version>  Major version number (e.g. "11")

Flags:
  --dry-run         Preview changes without executing

Exit codes:
  0  success (or dry-run preview printed)
  1  composer.json not found, already at target, or verification failed
  2  usage error (missing argument)
  3  composer/drush not found or execution failed

stdout (JSON):
  {
    "current_constraint": "^10.3",
    "target_constraint": "^11.0",
    "dry_run": false,
    "backup": "composer.json.bak",
    "checkpoint": "<git-sha>",
    "composer_exit": 0,
    "drush_updb_exit": 0,
    "verified_version": "11.0.0",
    "success": true
  }
```

### Cross-platform SKILL.md pipeline

```
Stage 1: drup preflight
Stage 2: drup scan <path>  (validate deps installed)
Stage 3: drup fix <path>   (rector)
Stage 4: drup contrib <module>  (per module loop)
Stage 5: drup upgrade-core <N>  (core version bump)
Stage 6: drup scan <path>  (custom/theme loop — AI-guided fixes)
Stage 7: drup scan <path>  (final validation)
Stage 8: drup report <path>
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `RunUpgradeCore` arg parsing, dry-run flag, missing composer.json | Table-driven tests in `commands_test.go` |
| Unit | Composer.json constraint rewrite (already in `coreupgrade/apply_test.go`) | Existing tests cover this |
| Integration | Full `drup upgrade-core` with scaffold Drupal project | Test with fixture composer.json + mocked exec |
| Unit | SKILL.md contains no platform primitives | Grep test for `task(`, `agent`, MCP tool names |
| Unit | Bootstrap templates render correctly per platform | Table-driven in `packaging_test.go` |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|----------|--------------|-----------------|-------------------|
| Documentation-like paths | N/A — no file classification | — | — |
| Git repository selection | Applicable — `git -C <projectPath>` for checkpoint/rollback | `validateProjectPath` enforces absolute path, no `..` traversal | Test: relative path rejected, `..` segment rejected |
| Commit state | Applicable — checkpoint commit before mutation | Requires clean working tree (`gitops.IsClean`), creates empty commit as rollback anchor | Test: dirty tree returns error with file list |
| Push state | N/A — no push operations | — | — |
| PR commands | N/A — no PR automation | — | — |

## Migration / Rollout

No data migration. `drup sync` re-generates bootstrap files from updated templates. Existing SKILL.md installations are overwritten on next `drup sync`.

## Open Questions

- [ ] Should `drup upgrade-core` support `--drush-skip` flag for environments without a running Drupal database? (Proposal mentions this as a testing concern)
