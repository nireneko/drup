# Design: Reorder Pipeline — Core Upgrade After Custom Loop

## Technical Approach

Documentation-only reorder. Swap the position of Stage 5 (CORE UPGRADE) and Stage 6 (CUSTOM LOOP) in all SKILL.md templates and the orchestrator spec. No Go code changes — CLI dispatch in `internal/app/app.go` is command-based (`drup upgrade-core`, `drup scan`, etc.), not stage-ordered. The pipeline sequence exists only in markdown.

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| Go code changes | None needed | Refactor app.go to encode stage order | CLI dispatch is a `switch args[0]` — no ordering exists in code. Adding one would be over-engineering a doc change. |
| README.md / docs/workflow.md | Out of scope | Update to match | These use a 7-stage model (core upgrade is a sub-step of contrib loop). Different document, different model. Touching them risks scope creep. |
| Installed SKILL.md | Updated via `drup sync` | Manual edit | The installer copies from templates. Editing the installed copy directly would be overwritten on next sync. |

## Data Flow

No data flow changes. The pipeline is sequential documentation — the AI agent reads stages top-to-bottom and executes `drup <stage>` commands. Reordering the markdown sections reorders the execution.

```
Before:  Stage 4 (CONTRIB) → Stage 5 (CORE UPGRADE) → Stage 6 (CUSTOM) → Stage 7 (VALIDATION) → Stage 8 (REPORT)
After:   Stage 4 (CONTRIB) → Stage 5 (CUSTOM) → Stage 6 (CORE UPGRADE) → Stage 7 (VALIDATION) → Stage 8 (REPORT)
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/packaging/templates/claude/SKILL.md` | Modify | Swap Stage 5↔6 blocks, renumber cross-references |
| `internal/packaging/templates/opencode/SKILL.md` | Modify | Same changes (identical template) |
| `internal/packaging/templates/codex/SKILL.md` | Modify | Same changes (identical template) |
| `openspec/specs/orchestrator-skill/spec.md` | Modify | Update Pipeline Definition requirement text + scenario |
| `~/.config/opencode/skills/drup/SKILL.md` | Modify | Installed copy — updated via `drup sync` after template change |

### Exact changes per template (all 3 are identical)

Each template needs these edits:

1. **Swap stage blocks**: Move CUSTOM LOOP content (currently lines 92-104) before CORE UPGRADE content (currently lines 81-90). Relabel: CUSTOM LOOP → Stage 5, CORE UPGRADE → Stage 6.
2. **CORE UPGRADE section**: "proceed to Stage 6" → "proceed to Stage 7" (line 89).
3. **FINAL VALIDATION section**: "re-enter the matching loop (Stage 4 or Stage 6)" → "(Stage 4 or Stage 5)" (line 121). Custom loop is now Stage 5.
4. **User Confirmation Gates**: "Stage 5 involves a non-dry-run core version bump" → "Stage 6 involves..." (line 156).

### Exact changes to orchestrator spec

In `openspec/specs/orchestrator-skill/spec.md`:

1. **Pipeline Definition** (line 75): Change sequence from `contrib loop → core upgrade → custom loop →` to `contrib loop → custom loop → core upgrade →`.
2. **Scenario** (line 81): Same reorder in the THEN clause.

## Interfaces / Contracts

None affected. No Go interfaces, no MCP tool signatures, no CLI argument changes.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Go code unaffected | `go test ./...` — must pass with zero changes |
| Visual | Template consistency | Grep all templates for "Stage 5" and "Stage 6" — verify CUSTOM LOOP = 5, CORE UPGRADE = 6, no stale references |
| Visual | Cross-reference integrity | Verify FINAL VALIDATION re-entry points to Stage 4 or Stage 5; confirmation gate references Stage 6 |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. After template changes are committed, `drup sync` propagates to installed agent configs. Users on existing installs get the updated stage order on next sync.

## Open Questions

None.
