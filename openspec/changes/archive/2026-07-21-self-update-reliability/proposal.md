# Proposal: Self-Update Reliability

## Intent

`drup upgrade` has failed with a different error on each of several attempts, and two recent commits (`18a3007`, `2b521f2`) patched the same code path back-to-back. Root cause is fragile design, not independent bugs: a silent `GOOS`/`GOARCH` env override, a two-hop archive-extension inference chain, a root-only binary lookup inside the extracted archive, and zero test coverage on the whole upgrade path (`RunUpgrade`, `extractUpdateBinary`, `extractTarGz`, `copyFile`). The spec-required "restore backup on replacement failure" behavior is also unimplemented. Fix the design so the failure class stops recurring, and lock it in with tests.

## Proposal Question Round (completed upstream)

Scope-shaping questions (full vs. minimal fix, Windows/darwin dead code, `go install` fallback) were already asked and answered by the user before this phase. Assumptions below are final per those answers; no re-ask needed. If anything here is misread, correct before proceeding to `sdd-spec`/`sdd-design`.

## Scope

### In Scope
- Remove the `os.Getenv("GOOS"/"GOARCH")` override in `RunUpgrade` (`internal/app/commands.go:274-281`); use `runtime.GOOS`/`runtime.GOARCH` directly.
- Build the archive filename deterministically once (mirror gentle-ai's `resolveArchiveName`), eliminating the double `strings.HasSuffix` inference in `internal/update/update.go:88-93` and `commands.go` `extractUpdateBinary`.
- Replace full-archive-unpack-then-root-lookup with a single-pass, basename-matched tar reader that extracts only the target binary directly to its destination (mirror `extractBinaryFromTarGz`/`writeExecutable`), removing the root-only assumption and one temp-dir/copy hop.
- Implement the spec's "restore backup on replacement failure" requirement (currently only removes the staged `.new` file).
- Add table-driven tests (`t.TempDir()`) for `RunUpgrade`, extraction (including a nested-subdirectory archive case), checksum-mismatch, and cross-device-safe copy — closing the gap flagged in the prior `verify-report.md` (W1).

### Out of Scope
- Windows/darwin dead code (`extractZip`, `.zip` branch): goreleaser is linux-only, but this change does not touch or remove that code.
- `go install` fallback tier (gentle-ai has one; drup doesn't): worth considering, not committed — flag as future work for `sdd-design` to accept or reject.
- HTTP retry/backoff: neither drup nor gentle-ai has it today; not part of this change.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
None — this implements existing `self-update` spec requirements (atomic replacement, backup restore on failure) rather than changing them. No delta spec expected unless `sdd-spec` finds an implicit behavior gap.

## Approach

Port gentle-ai's proven pattern (`internal/update/upgrade/download.go`) into drup's `internal/update` and `internal/app/commands.go`: deterministic naming → single-pass streaming extraction → atomic replace with backup-restore-on-failure. Drop the env-var override. Add tests per `go-testing` skill conventions (table-driven, `t.TempDir()`, explicit failure cases) alongside the implementation, consistent with `openspec/config.yaml`'s `strict_tdd: true`.

## Affected Areas

| Area | Impact | Description |
|------|--------|--------------|
| `internal/update/update.go` | Modified | Deterministic archive naming; remove double extension inference |
| `internal/app/commands.go` | Modified | Remove GOOS/GOARCH override; single-pass extraction; backup restore on failure |
| `internal/update/update_test.go`, `internal/app/commands_test.go` (new) | New/Modified | Full upgrade-path coverage |
| `openspec/specs/self-update/spec.md` | Unchanged (implemented) | No requirement text changes expected |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Extraction rewrite regresses the linux tar.gz path currently in prod | Medium | Table-driven tests incl. nested-dir case before merge; keep behavior identical for root-level layout |
| Backup-restore logic itself fails mid-restore (partial state) | Low | Design phase must define explicit restore-failure error reporting, not just "best effort" |
| Removing env override breaks a CI/dev workflow relying on it | Low | Grep repo/CI for `GOOS`/`GOARCH` set before `drup upgrade` calls; none found in-repo |

## Rollback Plan

All changes are additive/refactor within `internal/update` and `internal/app/commands.go`; revert via `git revert` of the change's commit(s). No data migration, no persisted-state schema change — `state.json` version-compare logic is untouched.

## Dependencies

- gentle-ai source (`/home/borja/sites/borja/go/gentle-ai`, same author) as reference pattern only — not a code dependency.

## Success Criteria

- [ ] `drup upgrade` no longer misroutes platform selection regardless of `GOOS`/`GOARCH` env state
- [ ] Extraction succeeds for both root-level and nested-subdirectory archive layouts
- [ ] Failed binary replacement restores the pre-upgrade binary (spec scenario satisfied)
- [ ] `go test ./...` covers `RunUpgrade`, extraction, and copy/replace with passing table-driven cases
- [ ] No Windows/darwin dead code touched
