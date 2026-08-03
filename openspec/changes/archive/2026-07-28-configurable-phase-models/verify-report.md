```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:dd409cc7636afab18090faa493ff0409e2fea49f5eb4d557f61753c1433d7a0f
verdict: pass
blockers: 0
critical_findings: 0
requirements: 10/10
scenarios: 14/14
test_command: "go test -count=1 ./..."
test_exit_code: 0
test_output_hash: sha256:8acd669bfe263a8917fe648934aa5ad8127adc2cfc269d02a24c84e3ebe2a460
build_command: "go build ./..."
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report (Round 3 — final gate after correction round 2)

**Change**: configurable-phase-models
**Artifact store**: openspec (+ Engram mirror)
**Mode**: Standard verification (no `strict_tdd` config or TDD runner in this repo)
**Date**: 2026-07-28
**Verdict**: **PASS WITH WARNINGS** — 0 CRITICAL, 5 WARNING, 5 SUGGESTION. Requirements 10/10, scenarios **14/14** (round 2: 12/14).
**Round history**: R1 FAIL (1 CRITICAL, 7 WARNING) → R2 FAIL gate (0 CRITICAL, 8 WARNING, 12/14 scenarios) → R3 PASS (14/14).

```yaml
authority_only_failure: false
missing_review_authority: false
substantive_failure: false
command_failed: false
test_command: "go test -count=1 ./..."
test_exit_code: 0
test_output_hash: sha256:8acd669bfe263a8917fe648934aa5ad8127adc2cfc269d02a24c84e3ebe2a460
build_command: "go build ./..."
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

Authoritative counts re-read from `spec.md`: **10 requirements** (`grep -c '^### Requirement:'`), **14 scenarios** (`grep -c '^#### Scenario:'`). Unchanged totals; the WARNING-1 correction reworded REQ-003 without adding or removing a scenario.

## Scope of this round

Bounded gate on the two items that blocked the round-2 gate, plus a no-regression sweep. This is not a full re-verify of unchanged dimensions.

## Build / Test / Static Evidence

| Check | Command | Exit | Result |
|---|---|---|---|
| Build | `go build ./...` | 0 | clean (empty output) |
| Tests | `go test -count=1 ./...` | 0 | 20/20 packages `ok`, 0 failures, 0 skips |
| Vet | `go vet ./...` | 0 | clean |
| Format | `gofmt -l internal/ cmd/` | 0 | no output |

Forced `-count=1`; no cached package result accepted as evidence.

## Gate Item 1 — WARNING-1 (REQ-003 byte-identity contradiction): CLOSED

`spec.md` REQ-003 no longer claims literal byte-identical output.

- Scenario heading renamed `Empty config is byte-identical` → **`Empty config resolves to unchanged built-in models`**.
- THEN clause now reads: "every agent's resolved `model` value (frontmatter/TOML field and 'Default model:' prose) SHALL equal the pre-change hardcoded value for that agent, producing zero behavior change".
- New REQ-003 paragraph (`spec.md:35`) states explicitly that this is **functional** byte-identity, not literal source-file byte-identity, and names REQ-004/005/007 as the intentional source rewrite.
- "Test Scenarios" table row 3 updated to match ("functional byte-identity; zero behavior change").
- `grep -n 'byte-identical\|byte-identity' spec.md` → only the two reworded, self-consistent passages remain. The contradiction with REQ-004/005/007 is gone.

Substance independently re-proven, not just the wording. Built the binary and rendered all three platforms into an isolated `$HOME` with **no** `model_assignments`, then compared every resolved default against the pre-change literal in `git show HEAD:` for the same template:

| Platform | Fixer agents (5) | `drup-validator` | Matches HEAD literal |
|---|---|---|---|
| claude | `claude-haiku-4-5-20251001` | `claude-sonnet-5` | 6/6 |
| opencode | `openrouter/qwen/qwen3-30b-a3b:free` | `openrouter/qwen/qwen3-235b-a22b` | 6/6 |
| codex | `gpt-4o-mini` | `gpt-4o` | 6/6 |

**18/18 agents resolve to exactly the pre-change hardcoded value.** REQ-003 as reworded is met, and the reworded scenario is now the one the implementation actually satisfies.

## Gate Item 2 — WARNING-7 (`SyncFileResult` clause): CLOSED on substance, weak as a guard

Test added: `TestInstallAgents_ReportsSyncFileResultsAlongsideFailures` (`internal/app/commands_test.go:1855`) — PASS. It captures stdout from `installAgents(..., "sync", nil)` with a corrupt `.claude.json`, asserts `succeeded == ["codex"]`, exactly one claude failure, and that codex's per-file statuses (`new:`) are still printed rather than dropped.

Honest scoping: that test is **narrower than WARNING-7 asked for**. It passes `nil` assignments and asserts only `new:`; it never asserts `modified` and never exercises a non-empty `model_assignments`. On its own it would not have closed the scenario.

The clause is closed because the specified behaviour was executed end-to-end with the real binary:

```
drup install                      → 32 files, all "new"
state.json += model_assignments   → {"claude":{"drup-rector":{default:"claude-opus-4",escalation:"claude-opus-4-max"}}}
drup sync                         → modified: ~/.claude/agents/drup-rector.md
                                    modified: ~/.claude/skills/drup/SKILL.md
                                    unchanged: (30 others)
drup sync (again)                 → 32 unchanged, 0 modified
```

- `modified` appeared for **exactly** the two files whose content genuinely changed; every other file reported `unchanged` and was not rewritten.
- Written content reflects the configured models: frontmatter `model: claude-opus-4`, prose "Default model: claude-opus-4 … escalate … to claude-opus-4-max", roster row `| drup-rector | claude-opus-4 → claude-opus-4-max (2 retries) |`.
- No leakage: `drup-custom` kept `claude-haiku-4-5-20251001`; `claude-opus-4` appears nowhere under opencode or codex.
- The `modified`/`unchanged` change-detection contract itself is separately regression-guarded by pre-existing `installer.TestInstall_AllUnchanged` (asserts modtimes unchanged) and `TestInstall_MixedStatus` (asserts `FileModified` for the touched file only).

Residual: no committed test pins the *combination* (non-empty `model_assignments` → `modified` only for the changed files). Tracked as WARNING-10, non-blocking.

## Regression Sweep — CRITICAL-1 and neighbours still clean

| Check | Result |
|---|---|
| `grep -rn "haiku\|sonnet\|qwen3\|gpt-4o" internal/packaging/templates/*/SKILL.md` | no match (exit 1) |
| Foreign literals in rendered output (`haiku`/`sonnet` under opencode+codex; `qwen3`/`gpt-4o` under claude) | no match |
| `grep -rl "{{MODEL_"` over all 32 rendered files | no match |
| `TestRender_NoForeignPlatformModelLiterals`, `TestRender_NoResidualPlaceholders` | PASS |
| OpenCode frontmatter vs prose (`drup-rector`, `drup-validator`, `drup-custom`) | identical model string on both surfaces |
| Codex `.toml` `model = "…"` present on all 6 agents | yes |
| `ValidateModelAssignments` (removed dead code) | absent from all `*.go` |

## Task Completion

19/19 checkboxes `[x]` in `tasks.md`; no unchecked task. Task 1.2 text is still stale (WARNING-9, carried).

## Spec Compliance Matrix

`PASS` = requirement met and covering evidence passed at runtime.

| Req | Scenario | R2 | R3 | Runtime evidence |
|---|---|---|---|---|
| REQ-001 | Full assignment resolves both slots | PASS | PASS | `TestRender_SubstitutionCorrectness` |
| REQ-001 | Unknown platform/agent key rejected, zero files | PASS | PASS | `TestRender_UnknownPlatformKeyRejected`, `TestRender_UnknownAgentKeyRejected` |
| REQ-002 | Arbitrary but valid string accepted | PASS | PASS | `TestRender_ArbitraryModelStringAccepted` |
| REQ-003 | **Empty config resolves to unchanged built-in models** | PARTIAL | **PASS** | **WARNING-1 closed.** `TestRender_NilAssignments_ByteIdentical`, `TestRender_SubstitutionCorrectness`; plus 18/18 resolved defaults compared against `git show HEAD:` literals in a live 3-platform render |
| REQ-003 | Partial config leaves other agents untouched | PASS | PASS | `TestRender_SubstitutionCorrectness`, `TestInstallAgents_AppliesConfiguredModelAssignments` |
| REQ-004 | Zero placeholders survive render | PASS | PASS | `TestRender_NoResidualPlaceholders` + scan of 32 installed files |
| REQ-005 | Codex TOML retains model | PASS | PASS | `TestRenderCodexAgentConfig_PreservesModel`, `…_RejectsUnquotedModel`; all 6 `.toml` carry `model` |
| REQ-006 | OpenCode frontmatter and prose agree | PARTIAL | **PASS** | `TestRender_OpenCodeFrontmatterProseAgree` PASS; direct inspection shows byte-equal model strings in frontmatter and prose on all opencode agents. The PARTIAL was test hygiene (dead guard, SUGGESTION-1), not missing coverage |
| REQ-007 | Roster reflects override | PASS | PASS | `TestRender_RosterReflectsOverride`; live roster row rendered `claude-opus-4 → claude-opus-4-max (2 retries)` |
| REQ-008 | Round-trip through state.json | PASS | PASS | `TestLoadSave_ModelAssignments_RoundTrip`, `TestLoad_LegacyModelOverrides_WarnedAndDropped`, `TestLoad_ModelAssignments_NilByDefault` |
| REQ-009 | **Sync applies configured models** | PARTIAL | **PASS** | **WARNING-7 closed.** `TestInstallAgents_AppliesConfiguredModelAssignments`, new `TestInstallAgents_ReportsSyncFileResultsAlongsideFailures`, `installer.TestInstall_AllUnchanged`/`TestInstall_MixedStatus`; plus live `drup sync` reporting `modified` for exactly the 2 changed files and `unchanged` for 30 |
| Model Routing | Cheap model for mechanical work | PASS | PASS | Live render: 5 fixer agents per platform on the cheap built-in tier |
| Model Routing | Escalation for custom code | PASS | PASS | CRITICAL-1 stays closed; every agent's prose names its own resolved escalation model |
| Model Routing | Configured override changes routing | PASS | PASS | `TestRender_SubstitutionCorrectness`; live override reached frontmatter, prose and roster |

**Requirements: 10/10. Scenarios: 14/14.** Spec "Test Scenarios" table (7 rows): all 7 PASS — row 3 now passes under the reworded functional-byte-identity wording.

## Design Coherence (delta only)

Unchanged from round 2 except that decision 6 (one resolved string feeds frontmatter, prose and roster) is re-confirmed on rendered output. Decision 1 (`"*"` fallback) is still unimplemented — `grep '"\*"'` finds nothing in `models.go`/`packaging.go`/`state.go` (WARNING-2, accepted deferral). Decision 8's text still contradicts the spec-compliant implementation (WARNING-5).

## Issues

### CRITICAL

**None.**

### WARNING (none block this verify gate; items 6 and 4 are archive prerequisites)

**WARNING-6 — No commits exist.** `git log --oneline -1` is still `c153980` (the pre-change merge). The tree is fully staged with nothing unstaged or untracked (`git status --porcelain` shows an index letter for all 37 entries). apply-progress records that `git commit` is denied to the apply executor. **Archive must not proceed until a human or the orchestrator creates the commits.**

**WARNING-4 — Authored size exceeds budget and still needs an explicit decision.** Staged, excluding `openspec/`: **30 files, 927 insertions + 93 deletions = 1020 authored lines** (was 983; correction round 2 added ~37 test lines), of which **558 are `*_test.go`**, leaving ~462 production lines. `tasks.md` forecast 600–700 against a recorded 800-line project budget with `Decision needed before apply: No`; the shared default is 400. `delivery_strategy: single-pr` permits one PR but does not itself grant a size exception — a maintainer must accept `size:exception` or request a split.

**WARNING-8 — Regression guard does not protect `claude` against its own literals** (carried, unaddressed by design of round 2's scope). `TestRender_NoForeignPlatformModelLiterals` skips same-platform vocabulary (`if otherPlatform == platform { continue }`, `packaging_test.go:461`). Mutation-proven in round 2: re-introducing `2 per scope on haiku, then 1 escalation attempt on sonnet` into `templates/claude/SKILL.md` leaves the whole `internal/packaging` suite `ok`. Claude is the reference platform and the origin of CRITICAL-1, so the exact defect class can silently return there. Cheap fix: assert the retry/`MAX RETRIES` prose lines contain no built-in model literal for their own platform.

**WARNING-9 — `tasks.md` task 1.2 is stale.** It reads "Add `ValidateModelAssignments()`…" and is checked `[x]`, but that method was deliberately removed (correctly — it was dead code). The validation substance lives in `state.ValidateModelValue` + `packaging.validateAssignments`. Reword before archive so the task record matches the code.

**WARNING-10 (NEW, replaces WARNING-7) — The REQ-009 status clause is verified but not regression-guarded.** The behaviour was proven by a live `drup sync`, and the change-detection contract is guarded at the installer layer, but no committed test ties a non-empty `model_assignments` to `modified`-only statuses. If placeholder resolution ever stopped changing file content, no test would fail. One test — install, mutate `model_assignments`, sync, assert exactly the rector + SKILL.md files report `modified` — would close it.

**WARNING-2 (carried) and WARNING-5 (carried)** — stale `design.md` passages: four references to the unimplemented `"*"` fallback (decision 1, Interfaces, validation contract, Testing Strategy, Open Question) and decision 8's "keep `ModelOverrides` as deprecated read-and-drop" versus the spec-compliant removal. Documentation accuracy only; amend or close the Open Question at archive.

### SUGGESTION

- **SUGGESTION-1** — dead guard in `TestRender_OpenCodeFrontmatterProseAgree`: `if !strings.Contains(content, frontmatterModel) { continue }` is unreachable because `frontmatterModel` was extracted from `content`. Remove it.
- **SUGGESTION-2** — `TestRender_DrupValidatorProseMatchesFrontmatter` does not test its name: it only asserts absence of `Default model: haiku`, never comparing frontmatter to prose, and 2 of its 3 sub-tests are vacuous on opencode/codex. Substance verified manually on all 3 platforms.
- **SUGGESTION-3** — `TestSKILLMD_CrossPlatformIdentical` is a misnomer now that rosters legitimately differ per platform. Rename to `TestSKILLMD_SharedLifecycleRules`.
- **SUGGESTION-6** — `drup-validator` prose escalates to its own default ("escalate the same scope to `claude-sonnet-5`" where that is already its default). Faithful to design decision 7 but reads as a no-op; prefer "retry the same scope on the same model".
- **SUGGESTION-7 (NEW)** — no committed test pins the built-in literals for opencode/codex or for `drup-validator`'s strong tier; only claude's cheap default is asserted (`packaging_test.go:362`, `commands_test.go:1822`). Round-3 verification did this by hand against `git show HEAD:`. A small table-driven test over `builtinModels` would make REQ-003's "equals the pre-change value" claim self-guarding.

## Positive Findings

- Correction round 2 hit exactly its bounded scope (spec wording + one test) and touched no production code; nothing regressed anywhere — build, vet, gofmt and all 20 packages clean.
- The REQ-003 rewording is the honest fix: it names the contradiction, explains why the source rewrite is intentional, and restates the requirement as the property the implementation actually guarantees.
- End-to-end runtime behaviour matches the spec exactly, including the subtle part: a single overridden agent changes precisely two files (its own definition plus the roster) and nothing else, with no cross-platform leakage.
- apply-progress again disclosed its own limits accurately, including that the WARNING-7 test is about status reporting rather than the full `modified`-only clause.

## Verdict

**PASS WITH WARNINGS.** 0 CRITICAL, 5 WARNING, 5 SUGGESTION. Requirements 10/10, scenarios 14/14.

Both gate items closed. WARNING-1 was a one-line-class spec correction and the reworded requirement is now the one the code satisfies — verified against the pre-change literals for all 18 agents, not taken on trust. WARNING-7's delivered test is narrower than requested, but the clause itself is now proven at runtime end-to-end and the underlying change-detection contract is guarded at the installer layer, so the coverage gap has moved from "unproven behaviour" (blocking) to "unguarded regression" (WARNING-10, not blocking).

Nothing in the implementation blocks archive. Two **process** prerequisites remain, both outside verification's authority:

| # | Prerequisite | Owner |
|---|---|---|
| 1 | Create the commits — `HEAD` is still the pre-change merge `c153980` (WARNING-6) | human / orchestrator |
| 2 | Accept `size:exception` for the 1020-line authored change, or split it (WARNING-4) | maintainer decision |

Also note for the archive gate: `gentle-ai review status` reports `clean` with zero authority entries, so no review receipt exists yet for this candidate. Archive requires `reviewGate.result: allow` with an approved receipt.

**Next**: `sdd-archive`, once the commits exist, `size:exception` is accepted, and review authority is obtained. WARNING-8/9/10 and the SUGGESTIONs are safe follow-ups; WARNING-8 is the one worth doing now, since it is the same defect class that already escaped review once.
