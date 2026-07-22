# Tasks: drup-orchestrator-delegation-fix

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~750 (3× SKILL.md rewrite ~240 lines, RunUpgradeCore ~120, tests ~80, bootstrap templates ~40, packaging/installer ~50, 18 agent file deletions ~220) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 |
| Delivery strategy | single-pr-default |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Cross-platform SKILL.md + delete agent defs + bootstrap templates | PR 1 | `go test ./internal/packaging/...` | N/A — template content, no runtime exec | Revert SKILL.md + restore agents/ dirs |
| 2 | `drup upgrade-core` CLI command | PR 2 | `go test ./internal/app/... -run UpgradeCore` | Scaffold composer.json fixture + mocked exec | Remove `upgrade-core` case from app.go + RunUpgradeCore |
| 3 | Packaging/installer wiring for bootstrap files | PR 3 | `go test ./internal/installer/... ./internal/packaging/...` | `drup install` against temp project dir | Revert packaging.go + installer.go changes |

## Phase 1: Cross-Platform SKILL.md and Bootstrap Templates

- [x] 1.1 Rewrite `internal/packaging/templates/opencode/SKILL.md`: replace sub-agent roster with 8-stage `drup <stage>` CLI pipeline; remove all `task()`, agent definitions, MCP tool references
- [x] 1.2 Copy identical SKILL.md content to `internal/packaging/templates/claude/SKILL.md` and `internal/packaging/templates/codex/SKILL.md`
- [x] 1.3 Delete all 6 agent files in `internal/packaging/templates/opencode/agents/` (drup-contrib.md, drup-custom.md, drup-preflight.md, drup-rector.md, drup-theme.md, drup-validator.md)
- [x] 1.4 Delete all 6 agent files in `internal/packaging/templates/claude/agents/`
- [x] 1.5 Delete all 6 agent files in `internal/packaging/templates/codex/agents/`
- [x] 1.6 Create `internal/packaging/templates/claude/CLAUDE.md`: bootstrap instructing AI to load SKILL.md from same directory
- [x] 1.7 Create `internal/packaging/templates/codex/copilot-instructions.md`: bootstrap instructing AI to load SKILL.md

## Phase 2: `drup upgrade-core` CLI Command

- [x] 2.1 Add `RunUpgradeCore(args []string) error` to `internal/app/commands.go`: parse target version + `--dry-run` flag, call `coreupgrade.Apply`, then exec `composer require`, `drush updb`, `drush status` verify; output JSON result
- [x] 2.2 Add `upgrade-core` case to `internal/app/app.go` Run() switch with usage guard (`len(args) < 2` → error)
- [x] 2.3 Add `upgrade-core` to `printUsage()` in `internal/app/app.go`
- [x] 2.4 RED test: relative path and `..` segment rejected by project path validation (threat matrix: git repository selection)
- [x] 2.5 RED test: dirty working tree returns error with file list (threat matrix: commit state)
- [x] 2.6 Unit tests in `internal/app/commands_test.go`: arg parsing, missing composer.json, already-at-target, dry-run output, composer-not-found, drush-not-found

## Phase 3: Packaging and Installer Wiring

- [x] 3.1 Update `internal/packaging/packaging.go`: add bootstrap template rendering for CLAUDE.md and copilot-instructions.md with `{{SKILL_PATH}}` substitution
- [x] 3.2 Update `internal/installer/installer.go`: ClaudeAdapter writes CLAUDE.md to project root; CodexAdapter writes copilot-instructions.md to `.github/`; OpenCodeAdapter adds SKILL.md skill entry to opencode.json
- [x] 3.3 Update `internal/packaging/packaging_test.go`: table-driven tests for bootstrap template rendering per platform
- [x] 3.4 Add SKILL.md content test: grep for `task(`, agent definition syntax, MCP tool names — must find zero matches

## Phase 4: Integration Verification

- [x] 4.1 Integration test: `drup upgrade-core` with fixture composer.json + mocked exec for composer/drush
- [x] 4.2 Verify `go test ./...` passes across all packages
- [x] 4.3 Verify `go build ./cmd/drup` produces working binary with `drup upgrade-core --help`
