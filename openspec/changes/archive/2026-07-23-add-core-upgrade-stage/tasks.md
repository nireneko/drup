# Tasks: Reorder Pipeline — Core Upgrade After Custom Loop

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~20–30 (markdown edits across 5 files) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

## Phase 1: Template Reordering

- [x] 1.1 In `internal/packaging/templates/claude/SKILL.md`: swap Stage 5 (CORE UPGRADE) and Stage 6 (CUSTOM LOOP) blocks. Relabel CUSTOM LOOP → Stage 5, CORE UPGRADE → Stage 6.
- [x] 1.2 In same file: update cross-references — "proceed to Stage 6" → "Stage 7" (CORE UPGRADE exit), "re-enter Stage 4 or Stage 6" → "Stage 4 or Stage 5" (FINAL VALIDATION), "Stage 5 involves core version bump" → "Stage 6" (confirmation gate).
- [x] 1.3 Repeat 1.1–1.2 for `internal/packaging/templates/opencode/SKILL.md` (identical structure).
- [x] 1.4 Repeat 1.1–1.2 for `internal/packaging/templates/codex/SKILL.md` (identical structure).

## Phase 2: Installed SKILL.md

- [x] 2.1 Run `drup sync` to propagate template changes to `~/.config/opencode/skills/drup/SKILL.md`. Verify installed copy matches templates.

## Phase 3: Orchestrator Spec

- [x] 3.1 In `openspec/specs/orchestrator-skill/spec.md`: update Pipeline Definition sequence from `contrib loop → core upgrade → custom loop →` to `contrib loop → custom loop → core upgrade →`.
- [x] 3.2 Update the "Pipeline stages in order" scenario THEN clause to match new order.

## Phase 4: Verification

- [x] 4.1 Grep all templates for "Stage 5" — verify CUSTOM LOOP = Stage 5, no stale CORE UPGRADE references.
- [x] 4.2 Grep for "Stage 6" — verify CORE UPGRADE = Stage 6, no stale CUSTOM LOOP references.
- [x] 4.3 Verify FINAL VALIDATION re-entry references "Stage 4 or Stage 5".
- [x] 4.4 Verify confirmation gate references "Stage 6" for core version bump.
- [x] 4.5 Run `go test ./...` — must pass with zero Go code changes.
