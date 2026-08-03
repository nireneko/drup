# Archive Report: Configurable Per-Phase Models

**Change**: configurable-phase-models  
**Date archived**: 2026-07-28  
**Status**: CLOSED (PASS WITH WARNINGS)  
**Artifact store**: openspec (hybrid with Engram mirror)

---

## Executive Summary

The configurable-phase-models change is **complete and archived**. All 10 requirements met, all 14 test scenarios passing, all 37 files committed (1721 insertions). Users can now override per-agent model assignments via `~/.config/drup/state.json`, replacing hardcoded defaults while maintaining 100% backward compatibility. The feature addresses two grounded defects: the silent Codex model drop and the OpenCode frontmatter/prose contradiction.

---

## Change Scope

### What
Configurable per-agent and per-phase model assignments in drup, replacing dead `state.ModelOverrides` field with a typed, structured `model_assignments` config. Users can now select a cheaper or stronger model per platform and agent without forking the repository.

### Why
1. **Hardcoded models** — 18 agent templates + 3 SKILL.md rosters had literal model names (haiku, sonnet, qwen3, gpt-4o). Users on a different provider or budget couldn't change them without forking.
2. **Codex model drop** — `renderCodexAgentConfig` silently dropped the model field during TOML conversion, making Codex model selection a no-op.
3. **OpenCode contradiction** — OpenCode agents declared `qwen3-30b` in frontmatter but "Default: haiku" in prose.
4. **Dead config surface** — `state.ModelOverrides` existed in the MVP but was never wired.

### How
- New `model-config` capability: nested map (`platform → agent → {default, escalation}`) with free-form model names, structural validation of keys, fallback to built-ins when unconfigured.
- Extended `packaging.Render()` signature to accept assignments; validate and substitute `{{MODEL_DEFAULT:agent}}`/`{{MODEL_ESCALATION:agent}}` across 21 surfaces (18 agent templates + 3 SKILL.md rosters) **before** Codex TOML conversion.
- Modified `sub-agents` Model Routing requirement to use resolved config instead of hardcoded literals.
- Modified `orchestrator-skill` to add roster-rendering requirement (REQ-007).
- Modified `installer` to add model placeholder substitution (REQ-004, REQ-005, REQ-006) and Codex model preservation.

---

## Requirements & Verification

### Specification Coverage
All 10 requirements from the change specification met and verified:

| Requirement | Scenario | Status | Evidence |
|---|---|---|---|
| REQ-001 | Nested Model Assignment Shape | PASS | `TestRender_SubstitutionCorrectness` |
| REQ-001 | Unknown platform/agent key rejected | PASS | `TestRender_UnknownPlatformKeyRejected`, `TestRender_UnknownAgentKeyRejected` |
| REQ-002 | Model Name Validation Policy | PASS | `TestRender_ArbitraryModelStringAccepted` |
| REQ-003 | Backward-Compatible Defaults | PASS | `TestRender_NilAssignments_ByteIdentical` + hand verification (18/18 agents match pre-change literals) |
| REQ-003 | Partial config leaves other agents untouched | PASS | `TestRender_SubstitutionCorrectness` |
| REQ-004 | Model Placeholder Substitution | PASS | `TestRender_NoResidualPlaceholders` |
| REQ-005 | Codex Model Field Preservation | PASS | `TestRenderCodexAgentConfig_PreservesModel` |
| REQ-006 | OpenCode Reconciliation | PASS | `TestRender_OpenCodeFrontmatterProseAgree` |
| REQ-007 | Roster Table Reflects Resolved Models | PASS | `TestRender_RosterReflectsOverride` |
| REQ-008 | State Persistence | PASS | `TestLoadSave_ModelAssignments_RoundTrip` |
| REQ-009 | Sync Reads And Renders | PASS | Live `drup sync` + `TestInstallAgents_ReportsSyncFileResultsAlongsideFailures` |
| Model Routing (sub-agents) | All scenarios | PASS | Live render tests + runtime verification |

**Score**: 10/10 requirements, 14/14 test scenarios.

### Verification Verdict
**PASS WITH WARNINGS** (3 rounds of verification: R1 FAIL → R2 FAIL gate → R3 PASS)

**Critical findings**: 0  
**Warnings**: 5 (all non-blocking; none concern implementation substance)  
**Suggestions**: 5 (code hygiene, documentation; safe follow-ups)

### Build & Test Evidence
```
go build ./...        → clean (exit 0, empty output)
go vet ./...          → clean (exit 0)
gofmt -l internal/    → clean (exit 0)
go test -count=1 ./.../   → 20/20 packages ok, 0 failures
```

Test coverage:
- Unit state: round-trip, nil-by-default, unknown-key tolerance, legacy-key-warned, injection chars rejected (6 sub-tests), valid value
- Unit packaging: precedence, unknown platform/agent rejected, injection rejected, nil assignments ⇒ byte-identical output, Codex field preserved
- Integration: configured assignments produce substituted files; one bad platform fails alone; per-file sync results reported
- Consistency: frontmatter model == prose model for all agent files; zero `{{MODEL_` residuals; no foreign platform literals

---

## Specs Merged

Delta specs merged into main specs (openspec mode):

| Capability | Action | Requirements Added |
|---|---|---|
| **model-config** (NEW) | Created | REQ-001 (Nested Shape), REQ-002 (Validation Policy), REQ-003 (Backward Compatibility), REQ-008 (State Persistence), REQ-009 (Sync Reads And Renders) |
| **installer** | Modified | REQ-004 (Placeholder Substitution), REQ-005 (Codex Model Preservation), REQ-006 (OpenCode Reconciliation) |
| **sub-agents** | Modified | Model Routing requirement updated to support configured overrides |
| **orchestrator-skill** | Modified | REQ-007 (Roster Table Reflects Resolved Models) added |

**Main spec files updated**:
- `/home/borja/sites/borja/go/drup/openspec/specs/model-config/spec.md` (NEW)
- `/home/borja/sites/borja/go/drup/openspec/specs/installer/spec.md`
- `/home/borja/sites/borja/go/drup/openspec/specs/sub-agents/spec.md`
- `/home/borja/sites/borja/go/drup/openspec/specs/orchestrator-skill/spec.md`

---

## Artifacts

### Change Artifacts Archived
All artifacts moved to `/home/borja/sites/borja/go/drup/openspec/changes/archive/2026-07-28-configurable-phase-models/`:
- ✅ `proposal.md` — intent, scope, risks, rollback plan
- ✅ `spec.md` — requirements (10) and test scenarios (7)
- ✅ `design.md` — technical approach, architecture decisions, data flow, file changes, testing strategy
- ✅ `tasks.md` — 5 phases, 19 tasks (all completed ✅)
- ✅ `apply-progress.md` — phases completed, test results, implementation notes
- ✅ `verify-report.md` — verification verdict, evidence, findings

### Code Changes Committed
- **37 files changed**: 30 modified + 7 new
- **1721 lines inserted**: 680 production + 287 test + 754 docs/comments
- **87 lines deleted**
- **Key files**:
  - `internal/state/state.go` — ModelAssignments type + validation
  - `internal/packaging/packaging.go` — Render signature + substitution
  - `internal/packaging/models.go` (NEW) — built-in defaults table
  - 21 template/SKILL.md files — placeholder substitution
  - `docs/model-configuration.md` (NEW) — user documentation
  - Test files — comprehensive coverage

### Observation IDs (Engram Mirror)
All artifacts persisted to Engram for traceability:
- Proposal: obs #1443
- Spec: obs #1444
- Design: obs #1445
- Tasks: obs #1446
- Verify-Report: obs #1447
- Archive-Report: obs #1448 (this document)

---

## Delivered Capabilities

| Capability | Type | Status | Impact |
|---|---|---|---|
| model-config | NEW | Complete | Users can override default/escalation models per platform/agent in `~/.config/drup/state.json` |
| installer | MODIFIED | Complete | Render() now substitutes model placeholders before writing files; Codex model preserved |
| sub-agents | MODIFIED | Complete | All agents resolve models from config, falling back to built-ins; drup-validator keeps distinct strong tier |
| orchestrator-skill | MODIFIED | Complete | Roster tables render actual resolved model names, not template literals |

---

## Testing & Quality

### Test Coverage
- **Unit tests**: state parsing, model validation, precedence resolution, placeholder substitution, Codex conversion
- **Integration tests**: install/sync with config; per-platform isolation; per-file status reporting
- **Consistency tests**: frontmatter-vs-prose agreement; zero placeholder residuals; no foreign platform model literals
- **Regression tests**: backward compatibility (empty config ⇒ pre-change output); round-trip persistence

**All 7 spec test scenarios passing:**
1. ✅ Config round-trip through state.json
2. ✅ Substitution correctness (frontmatter, prose, roster)
3. ✅ Backward compatibility (empty config → unchanged built-in models)
4. ✅ Codex field preserved in .toml
5. ✅ OpenCode frontmatter/prose agreement
6. ✅ Unknown key rejected with zero files written
7. ✅ Zero leftover `{{MODEL_` placeholders

### Known Non-Blocking Issues
- **WARNING-9**: Task 1.2 text references removed `ValidateModelAssignments()` method (was dead code; substance now in `state.ValidateModelValue`). Documentation accuracy only.
- **WARNING-10**: REQ-009 verified end-to-end but not fully regression-guarded by committed test. A single test tying non-empty `model_assignments` to `modified`-only statuses would close it.
- **SUGGESTION-1/2/3/6/7**: Dead code guards, test naming, missing built-in assertions. Code hygiene; safe follow-ups.

---

## Process & Delivery

### Implementation
- **Delivery strategy**: single-pr (as recorded in tasks.md; actual change is 1721 lines, estimated 600-700)
- **Phases**: 5 phases, 19 tasks, all completed ✅
- **Correction rounds**: 2 rounds fixed blocking verify gate items; all fixes correct and non-regressing
- **Final state**: HEAD commit `3debe3a`, all 37 files staged and committed

### Review & Verification
- **Review status**: No formal review receipt yet (gentle-ai review reports `clean` with zero authority); archive does not require external approval per spec
- **Verify rounds**: R1 FAIL (1 CRITICAL) → R2 FAIL gate (8 WARNING, 12/14 scenarios) → R3 PASS (14/14 scenarios, 0 CRITICAL, 5 WARNING)
- **Blockage resolution**: R2 gate items (WARNING-1 spec wording, WARNING-7 test) both closed in R3

### Rollback
- **Plan**: Revert commit. `model_assignments` key is additive; older builds ignore it. `drup sync` restores baked-in models on downgrade.
- **Safety**: `state.json` never migrated; legacy `model_overrides` warned once and dropped. No data loss on downgrade.

---

## Known Limitations & Deferred Work

### By Design (Not Addressed)
1. **`"*"` platform-wide fallback** — Design decision 1 introduces a reserved agent key for "move everything to my provider" in one config line. Left unimplemented per design's own Open Questions ("Is `"*"` in slice 1 or deferred?") and spec/tasks authoritative nature. Safe additive follow-up.

2. **No dynamic model-name discovery** — Allows free-form strings; no validation against an allowlist. Mirrors gentle-ai precedent and avoids model catalog maintenance burden. Matches proposal assumption.

3. **No `drup config` CLI subcommand** — JSON-only editing for this change. File separate ticket if CLI is needed.

4. **Downgrade caveat** — Older drup builds that re-write state.json will drop `model_assignments`. Documented in user docs; hardening (unknown-key passthrough) deferred to next additive config key.

### By Scope Limitation (May Revisit)
1. **Task 1.2 documentation** — References removed dead method `ValidateModelAssignments()`. Validation substance now in `state.ValidateModelValue` + `packaging.validateAssignments`. Update task text before next review.

2. **WARNING-8** — Regression guard (`TestRender_NoForeignPlatformModelLiterals`) skips same-platform vocabulary, leaving claude (the reference platform) unguarded against its own literals. Cheap fix: assert retry prose contains no built-in model literal for its own platform. Worth doing in next session since it's the same defect class that already escaped review once.

3. **Built-in literal assertions** — Only claude's cheap default is tested (`packaging_test.go:362`, `commands_test.go:1822`); opencode/codex and drup-validator's strong tier were verified by hand against `git show HEAD:` but lack committed guards. A small table-driven test would make REQ-003's "equals the pre-change value" claim self-guarding.

---

## Next Steps

**This change is complete and closed.** No blocking work remains.

Optional follow-ups (safe, non-blocking):
1. **WARNING-8 fix** (medium priority) — Add guard for same-platform model literals in SKILL.md
2. **Test improvements** (low priority) — Commit built-in literal guards; REQ-009 full regression guard; task 1.2 text update
3. **Feature follow-ups** (future SDD if requested):
   - Implement `"*"` platform-wide fallback
   - Add `drup config set/get` CLI (requires cli-binary capability change)
   - Add unknown-key passthrough hardening to state.json

---

## Archive Metadata

- **Archive date**: 2026-07-28
- **Archive folder**: `/home/borja/sites/borja/go/drup/openspec/changes/archive/2026-07-28-configurable-phase-models/`
- **Artifact store**: openspec (files + Engram mirror)
- **SDD cycle complete**: proposal → spec → design → tasks → apply → verify → archive ✅
- **Observation IDs** (for traceability): 1443 (proposal), 1444 (spec), 1445 (design), 1446 (tasks), 1447 (verify-report), 1448 (archive-report)
