```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:3dd76c2500bad9f2c88de1d1907d9cfdb81f87e1029474bdec98e8f4223a1eea
verdict: fail
blockers: 2
critical_findings: 0
requirements: 10/10
scenarios: 12/14
test_command: "go test -count=1 ./..."
test_exit_code: 0
test_output_hash: sha256:3dd76c2500bad9f2c88de1d1907d9cfdb81f87e1029474bdec98e8f4223a1eea
build_command: "go build ./..."
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

# Verification Report (Round 2 — post-correction re-verify)

**Change**: configurable-phase-models
**Artifact store**: openspec (+ Engram mirror)
**Mode**: Standard verification (no `strict_tdd` config or TDD runner gate found in this repo)
**Date**: 2026-07-28
**Verdict**: **FAIL (gate)** — 0 CRITICAL, 8 WARNING, 4 SUGGESTION. CRITICAL-1 is closed; the gate still reads FAIL because 2 of 14 spec scenarios lack complete runtime evidence (both carried over from round 1, neither newly introduced).
**Round 1 verdict**: FAIL (1 CRITICAL, 7 WARNING, 5 SUGGESTION)

```yaml
authority_only_failure: false
missing_review_authority: false
substantive_failure: true
command_failed: false
test_command: "go test -count=1 ./..."
test_exit_code: 0
test_output_hash: sha256:3dd76c2500bad9f2c88de1d1907d9cfdb81f87e1029474bdec98e8f4223a1eea
build_command: "go build ./..."
build_exit_code: 0
build_output_hash: sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
```

## Artifact Completeness

| Artifact | Present | Path |
|---|---|---|
| proposal | Yes | `openspec/changes/configurable-phase-models/proposal.md` |
| spec | Yes | `openspec/changes/configurable-phase-models/spec.md` |
| design | Yes | `openspec/changes/configurable-phase-models/design.md` |
| tasks | Yes | `openspec/changes/configurable-phase-models/tasks.md` |
| apply-progress | Yes | `openspec/changes/configurable-phase-models/apply-progress.md` |

Full artifact set present — all three dimensions verified (completeness, correctness, coherence). No dimension skipped.

Authoritative counts re-read from `spec.md`: **10 requirements**, **14 scenarios** (`grep -c '^### Requirement:'` = 10, `grep -c '^#### Scenario:'` = 14). Unchanged from round 1 — the correction round did not edit `spec.md`.

## CRITICAL-1 Resolution — VERIFIED CLOSED

### 1. Hardcoded escalation literals removed from all 3 `SKILL.md` templates

```
grep -rn "haiku\|sonnet\|qwen3\|gpt-4o" internal/packaging/templates/*/SKILL.md
→ no output (exit 1). Round 1: 6 matches (2 per platform).
```

The 6 offending lines were reworded to model-agnostic prose that defers to the roster table (which *is* placeholder-substituted), rather than being converted to inline `{{MODEL_ESCALATION:...}}` placeholders. Both approaches satisfy the requirement; the chosen one avoids naming a per-agent model in a line that covers all fixer agents at once.

| Platform | Line | Round 1 (broken) | Round 2 (fixed) |
|---|---|---|---|
| claude | 163, 172 | `… then escalate model haiku → sonnet, …` | `… then escalate model, …` |
| claude | 207 | `2 per scope on haiku, then 1 escalation attempt on sonnet.` | `2 per scope on the default model, then 1 escalation attempt on the escalation model.` |
| opencode | 165, 174 / 209 | same literals | same generic wording |
| codex | 160, 169 / 204 | same literals | same generic wording |

Additionally `SKILL.md:121` (all platforms) now reads `then escalate model per the roster rule`, and the roster preamble (claude:32) states the tier policy generically.

### 2. Verified in **rendered** output, not only in templates

Rendered all 3 platforms with `nil` assignments (32 files) and inspected:

| Platform | Roster escalation column | Retry prose | Self-contradiction |
|---|---|---|---|
| claude | `claude-haiku-4-5-20251001 → claude-sonnet-5 (2 retries)` | `the default model` / `the escalation model` | None |
| opencode | `openrouter/qwen/qwen3-30b-a3b:free → openrouter/qwen/qwen3-235b-a22b (2 retries)` | generic | **None** (round 1: instructed `haiku → sonnet`, models absent from the platform) |
| codex | `gpt-4o-mini → gpt-4o (2 retries)` | generic | **None** (round 1: same defect) |

`grep -rl "{{MODEL_"` over all 32 rendered files → no matches. REQ-004 holds.

### 3. Regression guard added — present, passing, and **mutation-proven**

`TestRender_NoForeignPlatformModelLiterals` (`internal/packaging/packaging_test.go:453`) renders each platform and asserts no file contains any *other* platform's model vocabulary, from `foreignModelVocab` (`claude: {haiku, sonnet}`, `opencode: {qwen3}`, `codex: {gpt-4o}`).

Mutation test (reverted immediately, worktree confirmed byte-identical to index afterwards): re-introducing `2 per scope on haiku, then 1 escalation attempt on sonnet` into `templates/opencode/SKILL.md` produced

```
--- FAIL: TestRender_NoForeignPlatformModelLiterals/opencode
    packaging_test.go:467: opencode/SKILL.md contains foreign vocabulary "haiku" belonging to platform "claude" …
    packaging_test.go:467: opencode/SKILL.md contains foreign vocabulary "sonnet" belonging to platform "claude" …
```

while `TestRender_NoResidualPlaceholders` still passed on the same mutation — confirming apply-progress's claim that the new test is a strictly stronger guard, and confirming the exact blind spot that let CRITICAL-1 through round 1.

**However, the guard is asymmetric.** See WARNING-8: the same mutation applied to `templates/claude/SKILL.md` is **not** caught by any test (whole `internal/packaging` suite still reported `ok`), because `haiku`/`sonnet` are claude's *own* vocabulary and the test skips same-platform words.

### 4. Dead code removed (round-1 WARNING-3)

```
grep -rn "ValidateModelAssignments" --include=*.go .
→ no output (exit 1)
```

`(*State).ValidateModelAssignments()` is fully gone from `internal/state/state.go`, with zero orphaned callers in production or test code. Injection-character coverage was preserved rather than deleted: the two tests were rewritten onto the still-live exported `ValidateModelValue` as `TestValidateModelValue_RejectsInjectionChars` (6 sub-cases) and `TestValidateModelValue_AcceptsValidValueAndEmpty` — both PASS. `ValidateModelValue` retains its two real production callers (`internal/packaging/models.go:111,114`). `ModelAssignments` and `ModelPhaseAssignment` untouched.

## Task Completion

19/19 task checkboxes `[x]` in `tasks.md`. No unchecked task. One task text is now stale against the code — see WARNING-9 (task 1.2 still names the deliberately-removed `ValidateModelAssignments()`).

## Build / Test / Static Evidence

| Check | Command | Exit | Result |
|---|---|---|---|
| Build | `go build ./...` | 0 | clean (empty output) |
| Tests | `go test -count=1 ./...` | 0 | 20/20 packages `ok`, 0 failures, 0 skips |
| Vet | `go vet ./...` | 0 | clean |
| Format | `gofmt -l internal/ cmd/` | 0 | no output |

All results from a forced `-count=1` run; no cached package results accepted as evidence.

Focused suites (verbose, per-test): `internal/packaging` 30/30 PASS (incl. the new guard), `internal/state` 13/13 PASS, `internal/app` 58/58 PASS incl. `TestInstallAgents_AppliesConfiguredModelAssignments` and `TestInstallAgents_UnknownAssignmentFailsOnlyThatPlatform`.

## Spec Compliance Matrix

Status legend: `PASS` = requirement met and a covering test passed at runtime; `PARTIAL` = substance met, covering test weaker than the scenario claims.

| Req | Scenario | R1 | R2 | Runtime evidence |
|---|---|---|---|---|
| REQ-001 | Full assignment resolves both slots | PASS | PASS | `TestRender_SubstitutionCorrectness` |
| REQ-001 | Unknown platform/agent key rejected, zero files | PASS | PASS | `TestRender_UnknownPlatformKeyRejected`, `TestRender_UnknownAgentKeyRejected` (both assert `len(files) == 0`) |
| REQ-002 | Arbitrary but valid string accepted | PASS | PASS | `TestRender_ArbitraryModelStringAccepted` |
| REQ-003 | Empty config is byte-identical | PARTIAL | PARTIAL | Unchanged spec defect — see WARNING-1 |
| REQ-003 | Partial config leaves other agents untouched | PASS | PASS | `TestRender_SubstitutionCorrectness` |
| REQ-004 | Zero placeholders survive render | PASS | PASS | `TestRender_NoResidualPlaceholders` + independent scan of 32 dumped rendered files |
| REQ-005 | Codex TOML retains model | PASS | PASS | `TestRenderCodexAgentConfig_PreservesModel`, `…_RejectsUnquotedModel`; all 6 rendered `.toml` carry `model = "…"` on line 2 |
| REQ-006 | OpenCode frontmatter and prose agree | PARTIAL | PARTIAL | `TestRender_OpenCodeFrontmatterProseAgree` PASS; dead guard still present (SUGGESTION-1). Substance re-confirmed by direct inspection of all 6 rendered opencode agent files |
| REQ-007 | Roster reflects override | PASS | PASS | `TestRender_RosterReflectsOverride` |
| REQ-008 | Round-trip through state.json | PASS | PASS | `TestLoadSave_ModelAssignments_RoundTrip`; `TestLoad_LegacyModelOverrides_WarnedAndDropped`; `TestLoad_ModelAssignments_NilByDefault` |
| REQ-009 | Sync applies configured models | PARTIAL | PARTIAL | `TestInstallAgents_AppliesConfiguredModelAssignments`. `SyncFileResult` clause still untested — WARNING-7 |
| Model Routing | Cheap model for mechanical work | PASS | PASS | `TestRender_NilAssignments_ByteIdentical` + `models.go` builtin table |
| Model Routing | **Escalation for custom code** | **FAIL** | **PASS** | **CRITICAL-1 closed.** Orchestrator escalation instruction is now model-agnostic and defers to the resolved roster; every agent file's prose names its own resolved escalation model. Evidence: `TestRender_RosterReflectsOverride`, `TestRender_SubstitutionCorrectness`, `TestRender_NoForeignPlatformModelLiterals`, plus rendered-output inspection across all 3 platforms |
| Model Routing | Configured override changes routing | PASS | PASS | `TestRender_SubstitutionCorrectness` |

**Requirements: 10/10** (round 1: 9/10). **Scenarios: 12/14** (round 1: 11/14); the 2 non-PASS rows are both PARTIAL for spec/test-strength reasons, not implementation defects.

Spec "Test Scenarios" table (7 rows): rows 1, 2, 4, 5, 6, 7 PASS; row 3 (byte-identical) PARTIAL per WARNING-1.

## Design Coherence

| # | Design decision | Honored | Note |
|---|---|---|---|
| 1 | Nested map + reserved `"*"` platform-wide fallback | Partial | `"*"` still not implemented — WARNING-2 (unchanged) |
| 2 | Qualified placeholders `{{MODEL_DEFAULT:agent}}` | Yes | 21 template files use the qualified grammar |
| 3 | No allowlist, structural validation only | Yes | `state.ValidateModelValue` rejects `\n`, `"`, `\`, `#`, edge whitespace |
| 4 | Validate + resolve inside `Render`; one bad platform fails alone | Yes | `TestInstallAgents_UnknownAssignmentFailsOnlyThatPlatform` PASS |
| 5 | Substitute → then TOML convert | Yes | Rendered `.toml` carries the resolved value on line 2 |
| 6 | One resolved string feeds frontmatter and prose | **Yes** | **Now honored on the `SKILL.md` surface too** — was the round-1 CRITICAL-1 violation |
| 7 | `drup-validator` resolves to strong tier | Yes | `claude-sonnet-5` / `qwen3-235b-a22b` / `gpt-4o` in both frontmatter and prose on all 3 platforms |
| 8 | Legacy `model_overrides` read, warned, dropped | Deviates (spec-compliant) | `Load`-local shadow field; spec REQ-008 wins — WARNING-5 (unchanged) |

The design File Changes row `internal/packaging/templates/** (21)` — "18 agent frontmatter + 'Default model:' prose; 3 SKILL.md rosters **+ retry prose**" — is now fully satisfied. The retry prose was the missing piece.

## Issues

### CRITICAL

**None.** CRITICAL-1 is closed and independently re-verified at the template level, the rendered-output level, and by mutation-testing the new regression guard.

### WARNING

**WARNING-8 (NEW) — The new regression guard does not protect the `claude` platform against its own literals.**
`TestRender_NoForeignPlatformModelLiterals` skips same-platform vocabulary (`if otherPlatform == platform { continue }`). Mutation-proven: re-introducing `2 per scope on haiku, then 1 escalation attempt on sonnet` into `internal/packaging/templates/claude/SKILL.md` leaves the entire `internal/packaging` suite reporting `ok`. Since claude is the reference platform and the origin of CRITICAL-1, the exact defect class can silently return there. Cheap fix: also assert that no rendered `SKILL.md` retry/`MAX RETRIES` line contains any built-in model literal for *its own* platform (or assert the prose lines match the model-agnostic wording).

**WARNING-9 (NEW) — `tasks.md` task 1.2 is stale against the code.**
Task 1.2 reads "Add `ValidateModelAssignments()` — reject newline, `"`, `\`, `#`, leading/trailing whitespace in values" and is checked `[x]`, but that method was deliberately removed this round (correctly — it was dead code, round-1 WARNING-3). The validation substance survives in `state.ValidateModelValue` + `packaging.validateAssignments`, so behaviour is fine, but the checked task now describes a symbol that does not exist. Reword task 1.2 before archive so the task record matches the code.

**WARNING-1 — REQ-003 "byte-identical" scenario is internally inconsistent with REQ-004/005/007 (unchanged from round 1, not addressed).**
`spec.md` was not edited this round; the scenario still demands byte-identical pre-change output while REQ-004/005/007 mandate 147 lines of intentional change. Implementation is right, spec text is wrong. Also `TestRender_NilAssignments_ByteIdentical` compares `nil` against an empty map, not against a pre-change baseline — the name overclaims. Reword the scenario and rename/strengthen the test.

**WARNING-2 — `"*"` platform-wide fallback still not implemented (unchanged).**
`grep '"\*"'` finds nothing in `models.go`/`packaging.go`/`state.go`. Design decision 1 (line 13), the Interfaces block (line 64), the validation contract (line 73), the Testing Strategy row (line 80) and Open Question (line 107) all still reference it. Acceptable deferral per apply-progress #3 (spec never requires it), but four design passages are now stale — amend them or close the Open Question before archive.

**WARNING-4 — Review budget overrun still needs an explicit `size:exception` decision (unchanged, and slightly larger).**
Staged change excluding `openspec/`: **30 files, 890 insertions + 93 deletions = 983 authored lines**, of which **521 are test code** (`*_test.go`), leaving ~462 production lines. `tasks.md` forecast 600–700 against a recorded 800-line project budget with `Decision needed before apply: No`; the shared phase-common default is 400. `delivery_strategy: single-pr` permits one PR but does not itself grant a size exception. A maintainer must accept `size:exception` explicitly, or split (e.g. ship tests separately).

**WARNING-5 — Design decision 8 text still contradicts the implementation (unchanged).**
`design.md:48` says "keep `ModelOverrides` as deprecated read-and-drop"; the implementation removed the exported field and decodes a `Load`-local `LegacyModelOverrides` shadow, following spec REQ-008 ("replacing the dead `model_overrides` field"). Correct precedence; update the design text for accuracy.

**WARNING-6 — Still zero commits; the change exists only in the Git index (partially improved).**
Improvement: `git status --porcelain` now shows an index-column letter for all 37 entries — nothing unstaged, nothing untracked (round 1: 26 files plus 4 new files were unstaged/untracked). Remaining gap: `git log --oneline -1` is still `c153980` (the pre-change merge), i.e. **no commit has been created**. apply-progress §4 records that `git commit` is denied by the permission system in the apply executor and re-confirmed this round. Archive must not proceed on an uncommitted tree — the orchestrator or a human with commit permission must run the commits. Suggested split is recorded in apply-progress's Delivery Note.

**WARNING-7 — REQ-009's `SyncFileResult` clause still has no covering test (unchanged).**
`grep -rn SyncFileResult --include=*_test.go` hits only `internal/installer/installer_test.go:1364,1535`, both predating this change and neither exercising `model_assignments`. `TestInstallAgents_AppliesConfiguredModelAssignments` asserts file contents, not sync statuses. Substance is very likely fine (the render→install path is unchanged), but the "status `modified` for changed files only" clause is unproven at runtime.

### SUGGESTION

**SUGGESTION-1 — Dead guard still present** in `TestRender_OpenCodeFrontmatterProseAgree` (`packaging_test.go`): `if !strings.Contains(content, frontmatterModel) { continue }` is unreachable, since `frontmatterModel` was just extracted from `content`. Remove it.

**SUGGESTION-2 — `TestRender_DrupValidatorProseMatchesFrontmatter` still does not test its name.** It only asserts the absence of the literal string `Default model: haiku`. It never extracts the frontmatter `model:` nor compares it to the prose. Note the assertion is also now platform-blind: on opencode/codex the forbidden string could never appear anyway, so 2 of its 3 sub-tests are vacuous. Substance was verified manually here (all 3 platforms agree), but strengthen or rename the test.

**SUGGESTION-3 — `TestSKILLMD_CrossPlatformIdentical` remains a misnomer** (it checks required/forbidden substrings, never cross-platform equality). Now that rosters legitimately differ per platform the name actively misleads. Rename to `TestSKILLMD_SharedLifecycleRules`.

**SUGGESTION-6 (NEW) — `drup-validator`'s prose escalates to its own default.** All 3 platforms render e.g. "this agent already runs on the strong tier … escalate the same scope to `claude-sonnet-5` for a third attempt", where `claude-sonnet-5` is already its default. This is a faithful rendering of design decision 7 (validator escalation == default, no stronger fallback in the catalog), so it is internally consistent, but the sentence reads as a no-op escalation. Consider "retry the same scope on the same model" wording.

Round-1 SUGGESTION-4 (add a stale-literal regression guard) is **resolved** by `TestRender_NoForeignPlatformModelLiterals`, with the residual gap tracked as WARNING-8. Round-1 SUGGESTION-5 (pre-existing README `config.yaml` vs `state.json` mismatch) stands as an out-of-scope separate ticket; correctly left untouched.

## Positive Findings

- The correction round fixed exactly the reported defect and nothing else drifted: build, vet, gofmt and all 20 packages are clean, and no previously-passing scenario regressed.
- The chosen fix is arguably better than the placeholder approach requested: a single line covering all fixer agents cannot name one agent's placeholder, so deferring to the (substituted) roster keeps one source of truth and honours design decision 6.
- The regression test was added with an explanatory comment naming the exact blind spot (`packaging_test.go:440-446`), and mutation testing confirms it genuinely closes that blind spot for opencode/codex.
- Dead-code removal preserved coverage instead of dropping it — the injection-character regression tests were rewritten onto the live `ValidateModelValue` rather than deleted.
- apply-progress's self-reported claims were all independently checkable and all proved accurate, including the honest disclosure that commits could not be created.

## Verdict

**FAIL (gate) — no CRITICAL remains.** 0 CRITICAL, 8 WARNING, 4 SUGGESTION. Requirements 10/10, scenarios **12/14**.

This needs care, because the two facts point in different directions and both are true:

- **The correction round succeeded.** CRITICAL-1 is closed and independently re-verified three ways (templates, rendered output, mutation-tested guard). All 10 requirements are now met. Nothing regressed. On the substance of what this re-verify was asked to check, the answer is *resolved*.
- **The formal gate still returns FAIL.** `gentle-ai sdd-verify-validate` refuses a passing verdict against 12/14 scenario evidence ("passing verdict contradicts failing or incomplete evidence"), and that is correct: REQ-003's byte-identical scenario is demonstrably not met as written (147 lines differ), and REQ-009's `SyncFileResult` clause has no covering runtime test. The skill's rule is explicit — a scenario is compliant only when a covering test passed at runtime. Neither gap is new; both are round-1 findings that the correction round deliberately left out of scope.

So this is not a regression and not a rejection of the apply work. It is the same two documentation/coverage gaps blocking a clean gate, now that the real defect is gone.

### Path to a clean gate

Four items block archive. None requires reworking the feature; two are one-line artifact edits, one is a small test, two are human decisions.

| # | Item | Blocks | Owner | Effort |
|---|---|---|---|---|
| 1 | **WARNING-1** — reword REQ-003's byte-identical scenario in `spec.md` so it stops contradicting REQ-004/005/007 (e.g. "resolved default values are unchanged") | the 12/14 gate | maintainer (spec edit) | minutes |
| 2 | **WARNING-7** — add one test asserting `SyncFileResult` status is `modified` only for files whose content changed under a non-empty `model_assignments` | the 12/14 gate | apply | small |
| 3 | **WARNING-6** — create the commits. Tree is fully staged, `HEAD` is still the pre-change merge `c153980` | archive | human/orchestrator (apply lacks commit permission) | minutes |
| 4 | **WARNING-4** — accept `size:exception` for the 983-line authored change, or request a split | archive | maintainer decision | decision only |

Items 1 and 2 are what stand between this report and `verdict: pass`. Once they land, re-run verify and the gate should admit 14/14.

Also recommended, cheap and low risk: **WARNING-8** — close the claude-side guard gap. This is the same defect class that already escaped review once, and claude is the reference platform. **WARNING-9** (stale task 1.2 wording) and WARNING-2/5 (stale design passages) are documentation-accuracy fixes. The four SUGGESTIONs are safe follow-ups.

**Next**: `sdd-apply` for items 1–2 (small, scoped), then `sdd-archive` once the commits exist and `size:exception` is decided. Archive must not proceed on the current uncommitted tree.
