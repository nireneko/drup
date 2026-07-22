# Tasks: Drupal Upgrade Orchestrator (No-Execute Coordinator)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 900-1400 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1 Go/MCP tools → PR2 agent templates → PR3 config cleanup |
| Delivery strategy | ask-always (treated as ask-on-risk) |
| Chain strategy | feature-branch-chain |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | PR | Focused test | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Go/MCP tools + envdetect state, tests | PR1 (base=tracker) | `go test ./internal/{coreupgrade,patchreconcile,envdetect,mcp,app}/...` | temp git repo + exec stub | revert commit, tools unregistered |
| 2 | Agent templates, 3 platforms | PR2 (base=PR1) | template sync diff review | N/A, prompt-only | revert commit, old prompts restored |
| 3 | config.yaml cleanup | PR3 (base=PR2) | doc review only | N/A | revert commit |

## Phase 1: Foundation — envdetect

- [x] 1.1 RED: `envdetect_test.go` — no markers yields `EnvUnsupported`
- [x] 1.2 GREEN: add `EnvUnsupported` const + terminal branch, `internal/envdetect/envdetect.go`

## Phase 2: Core Upgrade (`internal/coreupgrade/`)

- [x] 2.1 RED: `NextMajor` (available/latest) + `PreviewComposerPatch` (diff-only) tests, testdata fixtures
- [x] 2.2 GREEN: implement `NextMajor`, `PreviewComposerPatch` (`internal/coreupgrade/check.go`)
- [x] 2.3 RED: `Apply`/`Rollback` tests — dirty-tree refusal, path-traversal rejection (threat matrix), checkpoint+restore round-trip
- [x] 2.4 GREEN: implement `Apply`/`Rollback` — clean-tree + absolute-path guards, `git -C` checkpoint, mutate/restore (`apply.go`, `rollback.go`)

## Phase 3: Patch Reconcile (`internal/patchreconcile/`)

- [x] 3.1 RED: `httptest` tests — newer patch detection, obsolete-when-merged, still-needed
- [x] 3.2 GREEN: implement `Reconcile(module, currentPatchURL) (*Result, error)` via `drupalorg.SearchIssuesAPI` (JSON api-d7 only, no HTML parsing)
- [x] 3.3 RED: adaptation preserves original issue reference when upstream patch rejects
- [x] 3.4 GREEN: adaptation branch (`Adapt`) reproducing patch intent, keeping issue ref in header + composer description

## Phase 4: MCP Tool Wiring

- [x] 4.1 Register placeholders `core_upgrade_check`, `core_upgrade_apply`, `patch_reconcile` in `internal/mcp/tools.go`
- [x] 4.2 Add real handlers in `internal/app/mcp_tools.go`, wire via `WireMCPTools`
- [x] 4.3 RED: handler arg-validation tests (missing `project_path`, bad package name)
- [x] 4.4 GREEN: reuse `composerPackagePattern`/`moduleNamePattern`/absolute-path guards for validation

## Phase 5: Agent Templates (claude/opencode/codex, identical)

- [x] 5.1 Create `agents/drup-validator.md` (owns scan/validate/upgrade_scan/generate_report, haiku, no remediation) in all 3 dirs
- [x] 5.2 Create `agents/drup-rector.md` (owns `autofix` only) in all 3 dirs
- [x] 5.3 Rewrite `SKILL.md` in all 3 dirs: pure coordinator, report envelope `{agent,status,summary,artifacts,evidence,risks}`, 7-stage pipeline, retry/escalation, zero Bash/MCP calls
- [x] 5.4 Update `drup-preflight.md` in all 3 dirs: detection-only, delegate scan/validate, add unsupported-PM terminal report
- [x] 5.5 Update `drup-contrib.md`, `drup-custom.md`, `drup-theme.md` in all 3 dirs: remove direct validate/scan calls
- [x] 5.6 Diff claude/opencode/codex dirs to confirm zero drift (body content byte-identical across all 3 platforms; frontmatter format is intentionally platform-native per the pre-existing convention and the agent-packaging spec — see apply-progress note)

## Phase 6: Configuration & Cleanup

- [x] 6.1 Update `openspec/config.yaml`: drop `cobra`/`llm`/`heal`, list real packages
- [x] 6.2 Update README/docs describing old orchestrator-executes-tools model

## Phase 7: Integration Testing

- [ ] 7.1 Static assertion: no SKILL.md contains a direct tool/Bash call instruction (Phase 5 SKILL.md rewrite is done; this Go-test assertion is unblocked but not yet written — out of scope for PR2/Phase 5, tracked for Phase 7)
- [ ] 7.2 Round-trip test: `core_upgrade_apply` dry_run → apply → rollback, temp git repo, as a single combined flow (equivalent coverage exists today as 3 separate focused tests: `TestApply_DryRunReturnsPreviewOnly`, `TestApply_ChecksClean_CreatesCheckpoint_AndMutates`, `TestRollback_RoundTrip_RestoresComposerJSON`)
- [x] 7.3 `patch_reconcile` three-state test (obsolete, newer-available, still-needed) via `httptest` — `internal/patchreconcile/reconcile_test.go`
