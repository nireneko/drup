# Tasks: Self-Update Reliability

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1300–1500 total (additions-heavy: new `upgrade.go`/`upgrade_test.go` ~900–1050; cutover ~400–460) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR1→PR2→PR3→PR4 (additive, independently mergeable) → PR5 (atomic cutover) |
| Delivery strategy | ask-on-risk |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

**Note for chain-strategy decision**: PR1–PR4 only add new, unreferenced symbols to new files (`upgrade.go`/`upgrade_test.go`) — each compiles and is safely revertible on its own, so `stacked-to-main` works for them. PR5 is different: it deletes `update.go`'s old `Download`/`findChecksum` AND rewires `commands.go` in the same commit, because `commands.go` calls the old `Download` signature — these cannot be merged to `main` as two separate PRs without an intermediate broken build. PR5 alone is also the unit most likely to land near/over 400 lines. Choose: accept PR5 as a `size:exception` (atomic, ~460 lines, not further splittable against `main`), or run PR1–PR5 as a `feature-branch-chain` so PR5 can still be split into two commits against the tracker branch (which tolerates a temporarily broken intermediate state) before the tracker merges to `main`.

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Deterministic naming + rewritten `Download` | PR1 | `go test ./internal/update/... -run Resolve\|Download` | N/A — pure unit/httptest, no real network/binary | Delete new funcs from `upgrade.go`/`upgrade_test.go` |
| 2 | Single-pass tar extraction | PR2 | `go test ./internal/update/... -run ExtractBinaryFromTarGz` | N/A — synthetic tar fixtures | Delete `ExtractBinaryFromTarGz`/`writeExecutable` + tests |
| 3 | Backup/restore/replace | PR3 | `go test ./internal/update/... -run Replace\|atomicReplace` | N/A — `t.TempDir()` only | Delete replace/backup/restore funcs + tests |
| 4 | `Upgrade()` orchestrator | PR4 | `go test ./internal/update/... -run TestUpgrade` | N/A — package-var seams, no real download | Delete `Upgrade`/`UpgradeOptions` + test |
| 5 | Cutover: retire old code, rewire `RunUpgrade` | PR5 | `go test ./internal/update/... ./internal/app/...` | Manual: build binary, run `drup upgrade` against a test release | `git revert` PR5 (all-or-nothing by design) |

## Phase 1: Naming + Download (PR1)
- [x] 1.1 RED — `upgrade_test.go`: table tests for `ResolveArchiveName/AssetURL/ChecksumURL`.
- [x] 1.2 GREEN — `upgrade.go`: implement those + `UpgradeOptions`.
- [x] 1.3 RED — `upgrade_test.go`: `Download` tests (match/mismatch/missing-checksums/asset-not-listed) via `httptest`.
- [x] 1.4 GREEN — `upgrade.go`: implement `Download(assetURL, checksumURL, archiveName, archivePath) error`.

## Phase 2: Extraction (PR2)
- [x] 2.1 RED — `upgrade_test.go`: root, nested-subdir, symlink-reject, not-found cases.
- [x] 2.2 GREEN — `upgrade.go`: `ExtractBinaryFromTarGz`/`writeExecutable`, single-pass basename match, reg-only, 0o755.

## Phase 3: Replace/Backup/Restore (PR3)
- [x] 3.1 RED — `upgrade_test.go`: `atomicReplace` (`t.TempDir()`).
- [x] 3.2 RED — `upgrade_test.go`: `ReplaceBinary` success / restore-succeeds / restore-also-fails (both errors + backup path in message).
- [x] 3.3 GREEN — `upgrade.go`: `atomicReplace`, `copyFile`, `BackupBinary`, `RestoreBinary`, `ReplaceBinary` (unconditional restore on any failure).

## Phase 4: Orchestrator (PR4)
- [x] 4.1 RED — `upgrade_test.go`: full `Upgrade()` flow via `httpClient`/`executableFn`/`homeDirFn` seams, `t.TempDir()` fake binary. (Also added `resolveAssetURLFn`/`resolveChecksumURLFn` package-var seams, not explicitly named in design's Testing Strategy row but required for the same reason — redirecting `Upgrade()`'s exported URL builders to a local `httptest` server without a real GitHub round trip.)
- [x] 4.2 GREEN — `upgrade.go`: `Upgrade(UpgradeOptions) error` wiring `CheckLatest → Download → Extract → Replace`.

## Phase 5: Cutover (PR5 — atomic)
- [x] 5.1 Remove old `Download`/`findChecksum` from `update.go`; `CheckLatest` unchanged. (Done during Phase 1 GREEN, not deferred to Phase 5 — Go disallows two `Download` funcs in the same package, so the old one had to go the moment the new one was introduced. See Deviations note below.)
- [x] 5.2 Remove `TestDownload_AndVerify`/`TestDownload_ChecksumMismatch` from `update_test.go` (superseded). (Same early-execution reason as 5.1.)
- [x] 5.3 Rewrite `RunUpgrade` in `commands.go` as a thin wrapper: build `UpgradeOptions` from `runtime.GOOS/GOARCH` (no env override), call `update.Upgrade`, keep state.json bookkeeping. (Also introduced `checkLatestFn`/`upgradeFn` package-var seams in `commands.go`, needed for 5.5's tests since `update`'s own seams are unexported and unreachable from package `app`.)
- [x] 5.4 Delete `extractUpdateBinary`/`extractTarGz`/`copyFile` from `commands.go`; prune `archive/tar`/`compress/gzip` imports; leave `extractZip` in place, unreferenced.
- [x] 5.5 Add `internal/app/commands_test.go` (new): 2 cases — already-up-to-date early return, error propagation from `update.Upgrade` — closes the coverage gap without duplicating `upgrade_test.go`.

## Phase 6: Full-Suite Verification
- [x] 6.1 Run `go test ./internal/update/... ./internal/app/...` — all RED tests now GREEN, no regressions. Also ran full `go test ./...` — all packages pass.
- [x] 6.2 Run `go vet ./...` — clean (no findings; `extractZip` unreferenced but `go vet` does not flag unused unexported functions, only unused imports/vars).
