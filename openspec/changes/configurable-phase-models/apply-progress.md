# Apply Progress: Configurable Per-Phase Models

## Status

5/5 phases complete, 19/19 tasks marked `[x]` in `tasks.md`. **Verify round 1 returned FAIL** (1 CRITICAL, 7 WARNING, 5 SUGGESTION) — see `verify-report.md`. Correction round 1 fixed CRITICAL-1 and two follow-up items. **Correction round 2 (this round)** fixed WARNING-1 (spec wording) and WARNING-7 (missing test) — see "Correction Round 2" below. 12/14 spec test scenarios now covered by a passing runtime test (up from the round-1 gate's reported partial coverage); still partial overall pending WARNING-2/4/5 and SUGGESTION-1/2/3/5, which remain out of this bounded round's scope. Ready for re-`sdd-verify`.

Mode: Standard (no `strict_tdd` config/test-runner gate found in this repo; tests were still written before/alongside each behavioral change per the Work Unit Evidence gate below).

## Correction Round (post-verify-FAIL)

Verify's CRITICAL-1 finding: the Stage 5 custom-loop retry line and the "MAX RETRIES" validation gate rule in all 3 platform `SKILL.md` files still hardcoded `haiku`/`sonnet` literally — these 6 lines were never converted to the `{{MODEL_` placeholder grammar during Phase 3, so the orchestrator's own escalation instruction ignored `ModelAssignments` entirely, and on opencode/codex the rendered file self-contradicted its own roster table with models that don't exist on those platforms.

Fixes applied:

1. **CRITICAL-1 fixed** — `internal/packaging/templates/{claude,opencode,codex}/SKILL.md`: reworded both hardcoded lines to generic wording ("escalate model", "the default model" / "the escalation model"), matching the phrasing the Stage 4 contrib-loop line already used correctly. Verified with `grep -rn "haiku\|sonnet\|qwen3\|gpt-4o" internal/packaging/templates/*/SKILL.md` → no output (was 6 matches before).
2. **Regression guard added** — new `TestRender_NoForeignPlatformModelLiterals` in `internal/packaging/packaging_test.go`: for each platform, asserts none of the OTHER platforms' built-in model vocabulary (`haiku`/`sonnet`, `qwen3`, `gpt-4o`) appears anywhere in that platform's rendered output. This is a stronger check than `TestRender_NoResidualPlaceholders` (which only detects unsubstituted `{{MODEL_` markers, not literals that bypassed the placeholder grammar entirely — exactly how CRITICAL-1 survived it).
3. **WARNING-3 fixed (dead code)** — removed `(*State).ValidateModelAssignments()` from `internal/state/state.go` (zero production callers; real validation runs in `packaging.validateAssignments`, which calls the still-used exported `ValidateModelValue`). The two tests that only exercised the dead method (`TestValidateModelAssignments_RejectsInjectionChars`, `TestValidateModelAssignments_AcceptsValidValue`) were rewritten to call `ValidateModelValue` directly instead of being deleted outright, preserving injection-char regression coverage under new names `TestValidateModelValue_RejectsInjectionChars` / `TestValidateModelValue_AcceptsValidValueAndEmpty`. `ModelAssignments` field and `ModelPhaseAssignment` type are untouched.
4. **WARNING-6 addressed (uncommitted tree)** — all previously unstaged/untracked files are now `git add`-staged (confirmed via `git status --porcelain`: every entry shows `M `/`A ` in the index column, nothing left in the working-tree column). **Commits themselves could not be created**: `git commit` is denied by the permission system in this executor context (same restriction the original apply run hit) — verified by testing a throwaway `git commit -m "test permission check"`, which was denied before touching history. The orchestrator/maintainer must run the actual `git commit` invocations; suggested atomic split is unchanged from the original apply-progress Delivery Note (6 commits: state → packaging → templates → rosters → sync → docs), or the 3-commit split suggested for this correction round (SKILL.md fix + regression test → dead-code removal → everything else).

Full re-run after these fixes: `go build ./...`, `go vet ./...`, `gofmt -l internal/` all clean; `go test -count=1 ./...` → all 20 packages `ok`, 0 failures.

Not addressed in this round (out of the CRITICAL-1 correction scope; still open from the original verify report, for the maintainer's judgment at archive time): WARNING-1 (REQ-003 scenario wording), WARNING-2 (`"*"` fallback deferral), WARNING-4 (`size:exception` decision on the ~958-line overrun), WARNING-5 (design text update for decision 8), WARNING-7 (`SyncFileResult` status test), and SUGGESTION-1/2/3/5.

## Correction Round 2 (WARNING-1 + WARNING-7)

Scope: this round is tightly bounded to WARNING-1 and WARNING-7 only, per orchestrator briefing. Dead code, CRITICAL-1, reversions, and new tasks are explicitly out of scope; none were touched.

1. **WARNING-1 fixed (spec wording contradiction)** — `openspec/changes/configurable-phase-models/spec.md` REQ-003: the old scenario claimed `Render` output is literally "byte-identical to pre-change rendered output" for empty/nil `ModelAssignments`, which directly contradicts REQ-004/REQ-005/REQ-007 (which intentionally rewrite 147 lines of template source, substituting hardcoded model literals for `{{MODEL_DEFAULT}}`/`{{MODEL_ESCALATION}}` placeholders). Reworded REQ-003's prose and its scenario (renamed "Empty config is byte-identical" → "Empty config resolves to unchanged built-in models") to state **functional** byte-identity: an empty/nil config resolves every agent's model field back to the exact same value that was previously hardcoded, so agent *behavior* is unchanged, even though template source (and surrounding rendered bytes) legitimately differ from the pre-change files. Also updated the "Test Scenarios" table row 3 to match. No code changed — this was a spec-text-only correction.
2. **WARNING-7 fixed (missing SyncFileResult test)** — added `TestInstallAgents_ReportsSyncFileResultsAlongsideFailures` in `internal/app/commands_test.go` (~34 lines). Mocks a two-platform scenario: `claude` has a corrupted `.claude.json` (fails to install), `codex` has a clean fake `$HOME` (installs successfully). Captures stdout during `installAgents(...)` and asserts: (a) `succeeded == ["codex"]` and `failures` contains exactly one claude entry — both results are captured for the same call, not just the failing or succeeding one in isolation; (b) the captured stdout contains `"Synced drup to codex"` and a `"new:"` status line, proving codex's per-file `installer.SyncFileResult.Status` values (REQ-009) are actually produced and reported (via `printSyncResults`), not silently dropped when a sibling platform fails. This closes the gap the round-1 verify report flagged: prior tests (`TestInstallAgents_IsolatesFailingAgent`, `TestInstallAgents_UnknownAssignmentFailsOnlyThatPlatform`) covered the succeeded/failures string lists but never asserted on the underlying `SyncFileResult` status values REQ-009 actually specifies.

Verification after these two fixes: `gofmt -l internal/`, `go vet ./...`, `go build ./...` all clean; `go test -count=1 ./...` → all 20 packages `ok`, 0 failures (includes the new test, run individually first with `-run TestInstallAgents -v` to confirm the `new:` status line appears in captured output).

Files touched this round: `openspec/changes/configurable-phase-models/spec.md` (REQ-003 prose + scenario + Test Scenarios table row 3), `internal/app/commands_test.go` (new test only, no production code changed), `openspec/changes/configurable-phase-models/apply-progress.md` (this merge). `tasks.md` unchanged (still 19/19 `[x]`, no new tasks — this round didn't add any task-tracked work item).

Still not addressed (out of this round's WARNING-1/WARNING-7 scope, deferred to maintainer/archive): WARNING-2 (`"*"` fallback deferral), WARNING-4 (`size:exception` decision on the ~958-line budget overrun), WARNING-5 (design text update for decision 8), and SUGGESTION-1/2/3/5.

Ready for re-`sdd-verify`.

## Phases Completed

### Phase 1: State & Validation
- `internal/state/state.go`: added `ModelPhaseAssignment{Default, Escalation string}`, `State.ModelAssignments map[string]map[string]ModelPhaseAssignment` (JSON `model_assignments`), `(*State).ValidateModelAssignments()`, exported `ValidateModelValue(string) error`.
- `Load()` decodes into an anonymous struct embedding `State` plus a `LegacyModelOverrides map[string]map[string]string` shadow field; if present, prints a one-time stderr warning and drops it (no migration), per REQ-008.
- Tests added: round-trip, nil-by-default (backward compat), unknown-JSON-key tolerance, legacy-key-warned-and-dropped, injection-char rejection (6 sub-cases), valid-value acceptance.

### Phase 2: Substitution Mechanism
- New `internal/packaging/models.go`: `agentNames` (6 agents), `AgentNames(platform)`, `builtinModels` (literal defaults per platform/agent, byte-identical to pre-change hardcoded values), `resolveModel` (per-field precedence: configured > builtin), `validateAssignments(platform, assignments)`.
- `packaging.Render` gained a 3rd parameter `assignments map[string]map[string]state.ModelPhaseAssignment`; validates before writing any file; calls `substituteModels` on every file's content before the Codex Markdown→TOML conversion.
- `substituteModels` replaces `{{MODEL_DEFAULT:<agent>}}` / `{{MODEL_ESCALATION:<agent>}}` for all 6 known agents in one pass (a single SKILL.md roster names all of them); asserts zero residual `{{MODEL_` afterward.
- `renderCodexAgentConfig` now also extracts and preserves the `model = "..."` line (previously silently dropped), with the same quoting check applied to `description`.
- All ~15 existing `Render(...)` call sites (test + production) updated to pass `nil` or explicit assignments.

### Phase 3: Template Reconciliation
- All 18 agent templates (6 agents × 3 platforms) had their literal `model:` / `model = "..."` frontmatter and "Default model:" prose ("haiku"/"sonnet" mentions) replaced with qualified placeholders.
- All 3 `SKILL.md` roster tables: replaced literal model names/labels in the 6 roster rows with the same placeholder pairs.
- OpenCode reconciled: frontmatter and prose now resolve from the same placeholder pair (fixes the pre-existing `qwen3-30b` vs "Default: haiku" contradiction).
- `drup-validator` prose fixed on all 3 platforms: removed "Default model: haiku (cheap, fast...)" and the "escalate to sonnet" line; new prose states it already runs on the strong tier and never the fixer agents' cheap default, aligned with the frontmatter's resolved default.

### Phase 4: Sync Flow Integration
- `internal/app/commands.go`: `RunInstall` now calls `statepkg.Load()` *before* `installAgents` (previously loaded state only after install) and passes `s.ModelAssignments` through; `RunSync` (which already loaded state first) now forwards `s.ModelAssignments` too; `installAgents` signature gained the `assignments` parameter and forwards it to `packaging.Render`.
- Integration tests added: configured assignments produce substituted installed files while unconfigured agents keep built-in defaults; an unknown agent key under one platform fails only that platform's install, the other still succeeds.

### Phase 5: Documentation
- New `docs/model-configuration.md`: shape, per-platform naming examples, override example, editing instructions, backward-compat and downgrade-caveat sections.
- `docs/configuration.md` does not exist in this repository (see Deviation #1 below) — added a short mention + link inside README.md's existing `## Configuration` section instead.
- Code comments added at every new/changed declaration in `state.go`, `models.go`, and `packaging.go` explaining resolution precedence, the placeholder grammar, and the legacy-key handling.

## Test Results

```
go build ./...   → clean
go vet ./...     → clean
gofmt -l <changed files> → no output (all formatted)
go test ./...    → ok, all 20 packages (including internal/state, internal/packaging, internal/app)
```

New/updated test files: `internal/state/state_test.go`, `internal/packaging/packaging_test.go`, `internal/app/commands_test.go`. All 7 spec test scenarios (round-trip, substitution correctness, backward compatibility, Codex field preserved, OpenCode reconciliation, unknown key rejected, zero leftover placeholders) have a corresponding test.

### Work Unit Evidence

| Evidence | Value |
|---|---|
| Focused test command / result | `go test ./internal/state/... ./internal/packaging/... ./internal/app/...` → all PASS |
| Runtime harness | `installAgents` integration tests write real files to a `t.TempDir()`-rooted fake `$HOME` and read them back (`TestInstallAgents_AppliesConfiguredModelAssignments`, `TestInstallAgents_UnknownAssignmentFailsOnlyThatPlatform`) — closest available substitute for a live `drup sync` run without a real installed agent environment |
| Rollback boundary | Revert this change set; `model_assignments` is additive and nil-safe — an older binary or a fresh checkout ignores it and renders the pre-change literals unchanged |

## Corrections / Interpretations of Spec and Design (please review)

1. **`docs/configuration.md` does not exist.** Design's File Changes table never lists it (only `docs/model-configuration.md` as "Create"); the orchestrator-injected task list asked to "Update `docs/configuration.md`". I added the short mention + link to README.md's existing `## Configuration` section instead, since that's the closest real analogue in this repo. Flagged in `tasks.md` next to 5.2.

2. **Validation isolation bug found and fixed during implementation.** My first draft of `validateAssignments` validated the *entire* assignments map on every `Render` call. Since `installAgents` passes the same full (all-platform) `state.ModelAssignments` map to every detected platform's `Render` call, a single typo under one platform (e.g. an unknown agent key under `"codex"`) would have failed the render for *every* platform, contradicting design decision 4's explicit "one bad platform fails alone." Fixed: an unknown top-level *platform* key is still rejected globally (there's no render call it could ever validly belong to), but an unknown *agent* key or bad value nested under a known platform is scoped to only the platform currently being rendered. Covered by `TestInstallAgents_UnknownAssignmentFailsOnlyThatPlatform`.

3. **`"*"` platform-wide fallback agent key: deferred, not implemented.** Design decision 1 introduces it, but design's own Open Questions list it as unresolved ("Is the `"*"` fallback in slice 1, or deferred?"), and neither `spec.md`'s requirements/scenarios nor `tasks.md` mention it anywhere. Per the instruction that spec/tasks are authoritative over design.md, and since spec never requires it, I did not implement it. If the maintainer wants it, it's a small additive follow-up to `resolveModel`.

4. **Built-in escalation-tier literals for opencode/codex are new inferences, not pre-existing values.** Before this change, every platform's fixer-agent prose said "escalate to sonnet" verbatim regardless of platform — including opencode and codex, which don't have a model called "sonnet" at all. That contradiction is exactly what this feature fixes. I chose each platform's escalation tier to equal its own `drup-validator`'s built-in default (the one "strong" model literal already present in that platform's codebase): `claude-sonnet-5` (claude, unchanged), `openrouter/qwen/qwen3-235b-a22b` (opencode, new), `gpt-4o` (codex, new). This reuses only literals already present in the repo (no invented model IDs) and keeps a consistent "cheap default → validator-tier escalation" shape across all 3 platforms. Please confirm this mapping matches your intent — it's a judgment call, not something spec.md pins down.

5. **Roster table format kept as `default → escalation (2 retries)` rather than a single collapsed literal.** REQ-007's own scenario text shows the resolved row as `claude-opus-4 (2 retries)` for a case where `default == escalation`, which would literally require dropping the arrow when both values coincide — logic the simple string-substitution design doesn't support. I kept the existing `{{MODEL_DEFAULT:x}} → {{MODEL_ESCALATION:x}} (2 retries)` shape (matching decision 2/6's "one substitution pass" design) and verified via `TestRender_RosterReflectsOverride` that the resolved values are present and the built-in literal is gone — satisfying the requirement's substance (roster reflects resolved names, not template literals) without matching the scenario's exact string byte-for-byte.

6. **Pre-existing, out-of-scope doc/code mismatch discovered (not fixed).** README.md's existing `## Configuration` section documents a `~/.config/drup/config.yaml` shape (`agents.claude-code.agents.drup-contrib.model`, etc.) that is not read by any code in this repository — only `~/.config/drup/state.json` is. This predates this change and is unrelated to `model_assignments`; I did not touch it beyond adding the new short mention immediately after it, to avoid scope creep on an unrelated pre-existing issue. Worth a separate ticket.

7. **`drup-validator`'s roster row rewritten to include its resolved default.** The pre-existing text ("the session model, never the cheapest (see below)") never named a literal, so REQ-007 didn't strictly force a change here — but the whole point of the feature is that a resolved model name should be visible and configurable, so I changed it to `{{MODEL_DEFAULT:drup-validator}} (never the cheapest — see below)`.

## Risks

- **Review budget overrun, discovered during implementation.** `tasks.md`'s Review Workload Forecast (recorded before implementation) estimated 600-700 changed lines against an 800-line project budget ("Low" risk, "single-pr", "Decision needed before apply: No"). The actual total is **~958 changed/added lines** (`git diff HEAD --shortstat` on the 26 modified tracked files = 680 insertions + 87 deletions = 767, plus 2 new files totaling 191 lines: `internal/packaging/models.go` 119 + `docs/model-configuration.md` 72). This exceeds both the original estimate and the stated 800-line budget. Roughly half of the excess (`internal/packaging/packaging_test.go` +271, `internal/state/state_test.go` +154, `internal/app/commands_test.go` +68 ≈ 493 lines) is test code covering the spec's 7 required scenarios plus the isolation-bug fix above; production code is closer to ~465 lines. Per design's own "Migration / Rollout" note, this was scoped as a single slice on purpose ("splitting would ship a knob that does nothing," since the Codex model fix is a prerequisite for the Codex config to have any effect) — I completed it as one unit rather than stopping mid-implementation, but the maintainer should decide whether to accept `size:exception` for this PR or request a follow-up split (e.g., tests could ship as a separate, less risky PR).
- No other deviations from design. All 7 spec test scenarios have a passing test. No open threat-matrix items were skipped (design's own Threat Matrix says N/A beyond the config-injection surface, which is covered).

## Remaining Work

None — all 5 phases and 15 tasks are complete.

## Delivery Note

**Updated in the correction round**: every file is now `git add`-staged — `git status --porcelain` shows an index-column letter (`M`/`A`) for all 33 changed/added files and nothing left untracked or unstaged. `git commit` itself is still denied by the permission system in this executor context (re-confirmed this round with a throwaway `git commit -m "test permission check"`, denied before any history was touched — same restriction the original apply run hit). The orchestrator or a human with commit permission must run the actual commits. Suggested split (either works, tree is fully staged either way):
- 3-commit correction-round split: (1) SKILL.md retry-prose fix + regression test, (2) dead-code removal (`ValidateModelAssignments`), (3) everything else in one commit.
- OR the original 6-commit atomic split from the injected task context:
1. State: add ModelAssignments type and validation
2. Packaging: add Render parameter, substitution, Codex fix
3. Templates: update all 18 agent templates with placeholders
4. Rosters: update SKILL.md orchestrator tables
5. Sync: wire ModelAssignments into install/sync flow
6. Docs: add model-configuration.md, update README.md, mark tasks.md complete
