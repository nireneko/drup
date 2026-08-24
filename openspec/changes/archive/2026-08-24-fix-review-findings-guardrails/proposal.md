# Proposal: fix-review-findings-guardrails

## Intent

PROJECT-REVIEW.md (2026-08-21) flagged 13 minor/medium/security findings; exploration re-verified every one against current code (line-exact, S1 bypass reproduced empirically, baseline build + 20/20 test packages green). Combined, they let an MCP client mutate any directory, evade the patch allowlist and drush blocklist via aliases, hang the single-threaded server, mis-answer notifications, and commit outside declared scope (`git add -A` in cleanup.go). This change closes all findings as guardrails G1–G9 with no new features.

## Scope

Delivery: `auto-chain`, 800-line review budget per slice; every slice independently green (`go build ./...` + `go test ./...`). Security fixes ship first; transport hardening precedes session infra; PR4 is the keystone.

| Slice | Contents (findings) | Est. lines |
|---|---|---|
| PR1 | Patch allowlist: URL parse, exact-host match, https-only, subdomain suffix rule; regression tests for the 3 bypass shapes (S1/C1) | ~120 |
| PR2 | Transport: scanner buffer cap, exec context/deadlines, sorted tools/list, ignore notifications, cleanup `io.Writer` refactor (D1, D2, M3, M6, M7) | ~450 |
| PR3 | Drush alias→canonical normalization + extended blocklist, metachar filter completion, stderr separator (S3/D3, S6, M2) | ~250 |
| PR4 | `internal/session` pkg, canonical-root resolver (unifies `ValidateProjectPath` variants), guard middleware at `WireMCPTools` incl. `upgrade_scan` nested-install path, `session_open` tool (G1, S4/D4) | ~600 |
| PR5 | Kill switch (`DRUP_DISABLE_MUTATIONS`, `--locked` flag + installer templates); force-dry-run vs refuse partition; drop `allow_dirty` from MCP surface (G4, G5, S2) | ~300 |
| PR6 | Backup-freshness gate; `internal/audit` JSONL ledger + caps; `pipeline_status` tool (G2, G3) | ~450 |
| PR7 | Scoped commits (cleanup.go `add -A` → declared paths via gitops helper); scan `evidence_hash` + validate `expected_hash` (G8, G9) | ~350 |
| PR8 | Docs counts (README, docs/mcp-tools.md); stale TypeRegA comment; double Close (M1, M4, M5) | ~120 |

Total ≈2,600 lines. The mapping is exhaustive: every exploration-table finding lands in exactly one slice, including exploration extras S6 and cleanup.go `add -A` (S2, unmapped in exploration, is assigned to PR5).

### Out of scope / Non-goals

- S5 cosign/signature verification for self-update — deferred to a separate future change.
- No features beyond the guardrails; no CLI flag removals (CLI keeps `--allow-dirty`).
- No new global state; `state.json` stays model-assignments-only.

## Decisions (exploration open questions 1–5, resolved)

1. **Token transport**: server-side session state bound to process lifetime (stdio is single-client); zero schema churn on mutating tools.
2. **Drush**: blocklist + canonical normalization by default; curated allowlist behind config opt-in.
3. **allow_dirty**: removed from MCP surface; CLI keeps the flag, preserving the documented end-of-pipeline dirty-tree flow.
4. **Config surface**: per-project `.drup/config.json` holds caps, timeouts, allowlist-mode; absent file = safe defaults.
5. **G5 partition**: force-dry-run = tools with native `dry_run` param (`core_upgrade_apply`, `contrib_compat_patch`, `contrib_allow_lenient`, `custom_compat_fix`); refuse = `apply_patch`, `composer_require`, `patch_rollback`, `cleanup`, `create_patch`, `test_backup_restore`, `test_backup_delete`.

Deferred to design with recommended defaults: Q6 S5 → defer; Q7 G9 → mechanical `expected_hash` fail-closed.

## Migration story (CRITICAL risk)

Requiring sessions must not break installed agent workflows on upgrade. Without a valid session: read-only tools unchanged; mutating tools degrade safely (forced dry-run or refusal with an actionable error naming the `session_open` flow). Runtime opt-out `DRUP_ALLOW_UNSAFE=1` restores legacy behavior with a logged warning. SKILL.md ×3 platforms and installer templates document the session flow in the same release; count-locking and wiring-symmetry tests are updated in-slice.

## Capabilities

> Contract for sdd-spec.

**New**
- `agent-session`: session lifecycle, root pinning/canonicalization, guard middleware semantics, `session_open`.
- `mutation-audit`: JSONL ledger, mutation caps, `pipeline_status` exposure.

**Modified**
- `apply-patch`: host-exact, https-only allowlist requirements.
- `mcp-server`: transport limits, timeouts, notification handling, sorted listing, drush filtering, guard enforcement, new tools.
- `cleanup-stage`: writer-injected output; scoped staging.
- `core-upgrade`: MCP `allow_dirty` removal; unified root validation.
- `scan`: evidence-hash serialization.
- `validation-gates`: `expected_hash` fail-closed check.
- `installer`: `--locked` in mcp.json template.
- `gitops`: scoped-commit helper.

## Approach

Reuse established patterns only: package-level var test seams (including new `session`/`audit` packages), wiring-symmetry invariant tests, count-locking tests, atomic tmp+rename writes, template parity across claude/opencode/codex, and the single `WireMCPTools` choke point for guard middleware. No invented patterns.

## Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Session requirement breaks installed workflows | High | Safe degradation + `DRUP_ALLOW_UNSAFE=1` opt-out + same-release template/docs sync |
| Guard coverage hole (nested installs, backup tools) | Medium | Explicit middleware set incl. `upgrade_scan` nested path; symmetry tests assert coverage |
| Tool-count drift from new tools | Medium | Count-locking + symmetry tests + docs updated in the same slice |

## Rollback Plan

Each chained PR reverts independently via `git revert`. New packages are additive — deleting the pkg plus middleware wiring restores prior behavior. `.drup/config.json` is opt-in; absence yields safe defaults. The env opt-out gives instant runtime rollback without redeploy.

## Dependencies

None external. Baseline verified green in exploration.

## Success Criteria

- [ ] All 13 review findings + 2 exploration extras fixed; each mapped to exactly one merged slice
- [ ] Reproduced S1 bypass shapes rejected by regression tests
- [ ] No MCP mutation possible without a valid session unless the explicit opt-out is set
- [ ] Every slice independently green and ≤800 changed lines
- [ ] Tool counts and docs consistent after new tools land
