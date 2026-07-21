# Design: Self-Update Reliability

## Technical Approach

Move the entire upgrade flow (download → verify → extract → replace) into `internal/update` as a single testable orchestrator, mirroring gentle-ai's `internal/update/upgrade/download.go` structurally (naming, single-pass extraction, atomic replace ported function-for-function). `internal/app.RunUpgrade` becomes a thin CLI wrapper: version check, call the orchestrator, state.json bookkeeping. Backup-restore-on-failure is new logic gentle-ai does not have.

## Architecture Decisions

| Decision | Choice | Alternatives rejected | Rationale |
|---|---|---|---|
| Package boundary | Whole flow lives in `internal/update` (new `upgrade.go`); `RunUpgrade` calls one `update.Upgrade()` | Keep orchestration in `commands.go` | Same-package private var seams (`httpClient`, `executableFn`, `homeDirFn`) are the only way to unit-test the full flow without exporting internals — mirrors gentle-ai's private-seam testing (`lookPathFn`, `resolveAssetURLFn`) |
| Naming/URLs | `ResolveArchiveName`/`ResolveAssetURL`/`ResolveChecksumURL` build all URLs deterministically from `(owner, repo, version, goos, goarch)` | Keep `CheckLatest`'s scraped GitHub asset URL for the download itself | Removes the fragile `strings.TrimSuffix`/`LastIndex` chain (commands.go:302-313) entirely; `CheckLatest` is kept only as an early "asset exists for this platform" validation |
| Extraction | Single-pass tar stream, basename match, write directly to `binaryPath+".new"` | Full unpack to scratch dir + root-only `os.Stat` lookup (current) | Matches gentle-ai; removes the path-depth assumption and the temp-dir/copy hop |
| Cross-device copy | Keep `copyFile` (read+write), but scope it to backup/restore only (`$HOME/.drup/backups` vs. arbitrary install dir) | Drop `copyFile` entirely | Extraction's cross-device hop is eliminated by construction (writes straight into the install dir); the backup directory is a *different* boundary and can still sit on another filesystem |
| Restore-on-failure | Unconditional: any replace failure triggers `RestoreBinary`, regardless of whether `currentBin` looks intact | Conditional restore via an `os.Stat` "corruption check" | POSIX `rename` is atomic so `currentBin` is normally untouched on failure, but the spec scenario requires restore on *any* replace failure — literal compliance, avoids false-safety heuristics |
| Restore-failure reporting | Return one error naming both failures plus the backup path for manual recovery; never return nil | Best-effort restore, log-only | "must not silently succeed" — tests assert both causes appear in the returned error string |
| Windows/.zip | Left untouched; `extractZip` and ext-branching become unreferenced dead code, not deleted | Delete the dead code now | Out of scope per proposal; goreleaser publishes linux-only assets today |

## Data Flow

    RunUpgrade (app)
        │ CheckLatest(owner, repo, GOOS, GOARCH)  -- version + asset-exists check
        ▼
    update.Upgrade(opts)
        │ ResolveArchiveName / ResolveAssetURL / ResolveChecksumURL
        ▼
    Download(assetURL, checksumURL, archiveName, archivePath)   -- verifies SHA256
        ▼
    ExtractBinaryFromTarGz(archive, binaryName, currentBin+".new")  -- single pass
        ▼
    ReplaceBinary(new, currentBin, backupPath)
        │ BackupBinary → atomicReplace ──success──▶ done
        │                     │
        │                  failure
        │                     ▼
        │               RestoreBinary ──success──▶ error (rolled back, reported)
        │                     │
        │                  failure
        │                     ▼
        │           error (manual recovery required, backup path named)

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/update/upgrade.go` | Create | `ResolveArchiveName/AssetURL/ChecksumURL`, `Download`, `ExtractBinaryFromTarGz`, `writeExecutable`, `atomicReplace`, `copyFile`, `BackupBinary`, `RestoreBinary`, `ReplaceBinary`, `Upgrade`, `UpgradeOptions` |
| `internal/update/upgrade_test.go` | Create | Table-driven tests per Testing Strategy |
| `internal/update/update.go` | Modify | Remove old `Download`/`findChecksum` (extension-inference); `CheckLatest` unchanged |
| `internal/update/update_test.go` | Modify | Drop `TestDownload_AndVerify`/`TestDownload_ChecksumMismatch` (old signature, superseded) |
| `internal/app/commands.go` | Modify | `RunUpgrade` becomes a thin wrapper; delete `extractUpdateBinary`, `extractTarGz`, `copyFile`; prune now-unused `archive/tar`/`compress/gzip` imports; `extractZip` left in place, unreferenced |

## Interfaces / Contracts

```go
type UpgradeOptions struct{ Owner, Repo, Binary, Version string }

func Upgrade(opts UpgradeOptions) error
func ResolveArchiveName(repo, version, goos, goarch string) string
func ResolveAssetURL(owner, repo, version, goos, goarch string) string
func ResolveChecksumURL(owner, repo, version string) string
func Download(assetURL, checksumURL, archiveName, archivePath string) error
func ExtractBinaryFromTarGz(r io.Reader, binaryName, outPath string) error
func ReplaceBinary(newBinaryPath, currentBin, backupPath string) error
```

`ExtractBinaryFromTarGz`: single tar pass; matches `filepath.Base(hdr.Name) == binaryName`; accepts only `tar.TypeReg`/`TypeRegA` (rejects symlinks, hardlinks, dirs); on first match calls `writeExecutable` and returns immediately — no full unpack. `hdr.Name` is used only for basename comparison, never joined into a filesystem path, so path-traversal entries in the archive are structurally inert. `writeExecutable` uses `os.OpenFile(outPath, O_CREATE|O_WRONLY|O_TRUNC, 0o755)` — executable bit set at write time, no trailing `chmod` pass.

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | `ResolveArchiveName/AssetURL/ChecksumURL` | Table-driven, mirrors gentle-ai's `TestAssetURLResolution` |
| Unit | `ExtractBinaryFromTarGz` | Root-level + nested-subdirectory fixture (mirrors `TestFindBinaryInTar`); reject-symlink case (new) |
| Unit | `atomicReplace` | `t.TempDir()`, mirrors `TestAtomicReplace` |
| Integration | `Download` checksum verification | 4 cases mirroring `TestDownload_ChecksumVerification`: match, mismatch, missing checksums.txt, archive not listed — `httptest.NewServer` |
| Integration | `ReplaceBinary` | success; replace-fails→restore-succeeds; replace-fails→restore-also-fails (new — asserts both error causes and backup path appear in the message) |
| Integration | `Upgrade`/`RunUpgrade` | Full flow via `httpClient`/`executableFn`/`homeDirFn` package-var seams, `t.TempDir()` fake `currentBin` |

All tests are table-driven with `t.Run(tt.name, ...)` and `t.TempDir()`, per `strict_tdd: true` — written alongside each function, not after.

## Threat Matrix

N/A — no routing, git, shell/subprocess, or PR-automation boundary. Archive extraction is executable-file-adjacent, but the only adversarial surface (tar path traversal, symlinks) is closed structurally (see Interfaces / Contracts) rather than via the routing/git/PR matrix rows, none of which apply here.

## Migration / Rollout

No migration; `state.json` version-compare logic untouched. Revert via `git revert` of the change's commits.

## Open Questions

- [ ] None blocking. The `go install` fallback tier remains a future-work suggestion per the proposal, not part of this change.
