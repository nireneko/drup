## Exploration: add-core-upgrade-stage

### Current State

**The core upgrade stage already exists.** The codebase already implements a full 8-stage pipeline with Stage 5 as CORE UPGRADE. Every layer is in place:

| Layer | Status | Location |
|-------|--------|----------|
| CLI command | ✅ Implemented | `internal/app/commands.go` → `RunUpgradeCore()` (lines 1026–1222) |
| CLI dispatch | ✅ Registered | `internal/app/app.go` → `case "upgrade-core"` |
| Core logic package | ✅ Full package | `internal/coreupgrade/` — `check.go`, `apply.go`, `rollback.go` |
| MCP tools | ✅ Two tools | `core_upgrade_check` + `core_upgrade_apply` in `internal/app/mcp_tools.go` |
| Shipped SKILL.md | ✅ 8 stages | `internal/packaging/templates/{claude,opencode,codex}/SKILL.md` — Stage 5 is CORE UPGRADE |
| Installed SKILL.md | ✅ 8 stages | `~/.config/opencode/skills/drup/SKILL.md` — Stage 5 is CORE UPGRADE |
| OpenSpec spec | ✅ Written | `openspec/specs/core-upgrade/spec.md` — 7 requirements with Given/When/Then scenarios |
| Orchestrator spec | ✅ Documents it | `openspec/specs/orchestrator-skill/spec.md` — "8-stage pipeline: preflight → dep check → rector → contrib loop → **core upgrade** → custom loop → final validation → report" |
| Tests | ✅ Comprehensive | 12+ test functions for `RunUpgradeCore` (missing arg, path traversal, dirty tree, already-at-target, dry-run, composer not found, drush not found, integration, version mismatch, checkpoint, DDEV prefix) |
| Tests (coreupgrade pkg) | ✅ Comprehensive | `apply_test.go` (5 tests), `check_test.go` (6 tests), `rollback_test.go` |

**The actual 8-stage pipeline (as implemented):**

1. PREFLIGHT — `drup preflight`
2. DEP CHECK — `drup scan <path>`
3. RECTOR — `drup fix <path>`
4. CONTRIB LOOP — `drup contrib <module>` + `drup issue` + `drup apply-patch`
5. **CORE UPGRADE** — `drup upgrade-core <target-version>`
6. CUSTOM LOOP — per-file fix + `drup scan`
7. FINAL VALIDATION — `drup validate <path>`
8. REPORT — `drup report <path>`

**What `drup upgrade-core` does (fully implemented):**
- Parses target version + `--dry-run` flag
- Reads current constraint from `composer.json` (`drupal/core-recommended` or `drupal/core`)
- Short-circuits if already at target
- Checks git clean working tree
- Calls `coreupgrade.Apply()` → creates git checkpoint commit, mutates `composer.json`
- Runs `composer config policy.advisories.block false`
- Runs `composer require drupal/core-recommended:^<N> ... -W --no-update`
- Runs `composer update -W`
- Runs `drush updb -y`
- Verifies with `drush status --format=json` (checks Drupal version matches target)
- Removes backup on success, retains on failure for rollback

### Affected Areas

**None — the feature is already implemented.** The user's "desired" 8-stage pipeline with core upgrade at Stage 6 (between contrib loop and custom loop) is functionally identical to the existing implementation, which places it at Stage 5. The ordering is the same: contrib loop → core upgrade → custom loop → final validation → report.

### Approaches

1. **No-op — feature already exists**
   - Pros: Zero work needed. All code, tests, specs, and SKILL.md templates are in place.
   - Cons: None. The user's description of a "7-stage pipeline" doesn't match reality.
   - Effort: None

2. **Rename/reorder to match user's exact numbering (Stage 6 instead of Stage 5)**
   - Pros: Matches the user's mental model exactly.
   - Cons: Purely cosmetic. Would require changing SKILL.md templates (3 agents), orchestrator spec, and possibly the shipped skill. No functional difference. The current numbering (Stage 5) is more logical — core upgrade before custom fixes means custom code is fixed against the target Drupal version.
   - Effort: Low (documentation-only, ~6 files)

### Recommendation

**Approach 1: No-op.** The core upgrade stage is fully implemented and tested. The user's "desired" pipeline already exists. The numbering difference (Stage 5 vs Stage 6) is cosmetic — the actual ordering of operations is identical (contrib → core upgrade → custom → validate → report).

If the user wants to verify the pipeline works end-to-end, the existing integration test `TestRunUpgradeCore_Integration` covers the full flow with mocked exec.

### Risks

- **User confusion**: The user described a 7-stage pipeline that doesn't match the codebase. They may be working from outdated documentation or a different version. Clarify with the user what they actually see vs. what they expect.

### Ready for Proposal

**No.** This change does not need a proposal — the feature already exists. The orchestrator should inform the user that:

1. `drup upgrade-core <target-version>` is already implemented and tested.
2. The SKILL.md already defines it as Stage 5 in an 8-stage pipeline.
3. The ordering is: contrib loop → core upgrade → custom loop → validate → report.
4. Ask the user if there's a specific behavior they want changed or added, or if they were working from outdated information.
