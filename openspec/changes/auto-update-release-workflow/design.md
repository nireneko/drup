# Design: Auto-Update Release Workflow

## Technical Approach

Wire up goreleaser + GitHub Actions to produce the exact artifacts `drup upgrade` expects, and fix two bugs in the self-update code (owner mismatch, blind asset selection). The approach mirrors the gentle-ai reference pattern: goreleaser handles cross-compilation/archives/checksums, a tag-triggered workflow runs goreleaser, a separate CI workflow validates PRs.

## Architecture Decisions

### Decision: Archive format per OS

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `.tar.gz` for all platforms | Simple, but Windows lacks native tar support | Rejected |
| `.tar.gz` for unix, `.zip` for windows | Native extraction on each platform | **Chosen** |

Goreleaser `format_overrides` handles this: default `tar.gz`, override `format: zip` for `goos: windows`.

### Decision: Asset selection strategy

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `CheckLatest` returns all asset URLs, caller filters | Splits logic across two files | Rejected |
| `CheckLatest` filters by `{os}_{arch}` suffix internally | Single function owns matching; caller gets exact URL | **Chosen** |

`CheckLatest` gains `goos, goarch string` parameters. It iterates `release.Assets`, matching suffix `_{goos}_{goarch}.tar.gz` or `_{goos}_{goarch}.zip`. First match wins. No match returns error.

### Decision: Windows `.zip` handling in `RunUpgrade`

| Option | Tradeoff | Decision |
|--------|----------|----------|
| Hardcode `.tar.gz`, skip Windows | Broken on Windows | Rejected |
| Determine archive extension from `runtime.GOOS` in `RunUpgrade` | Two places know about OS→extension mapping | **Chosen** (minimal change) |

`RunUpgrade` computes `ext` based on `goos`: `".zip"` for windows, `".tar.gz"` otherwise. This `ext` is used for both `assetName` construction and passed to `CheckLatest` implicitly (since `CheckLatest` matches both extensions).

### Decision: Version ldflags target

| Option | Tradeoff | Decision |
|--------|----------|----------|
| `-X main.version` | Requires new var in main, indirection | Rejected |
| `-X github.com/nireneko/drup/internal/app.Version` | Direct; `Version` already exists at `internal/app/app.go:6` | **Chosen** |

## Data Flow

```
git push v0.2.0
    │
    ▼
release.yml (tag trigger)
    │
    ▼
goreleaser-action
    ├── cross-compile: linux/darwin/windows × amd64/arm64
    ├── inject version: -X ...internal/app.Version={{.Version}}
    ├── archive: drup_{version}_{os}_{arch}.tar.gz (.zip for windows)
    ├── checksums: checksums.txt (sha256  filename)
    └── upload → GitHub Release
                      │
                      ▼
              drup upgrade (user runs)
                      │
                      ├── GET /repos/nireneko/drup/releases/latest
                      ├── filter assets by _{os}_{arch} suffix → asset URL
                      ├── download asset + checksums.txt
                      ├── verify sha256
                      └── atomic rename over current binary
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `.goreleaser.yaml` | Create | Build config: 6 targets, ldflags, archive naming, checksums |
| `.github/workflows/release.yml` | Create | Tag-triggered goreleaser execution |
| `.github/workflows/ci.yml` | Create | PR/push: gofmt, go vet, go test |
| `internal/update/update.go` | Modify | `CheckLatest` accepts `goos, goarch`; filters assets by suffix |
| `internal/update/update_test.go` | Modify | Add tests for OS/arch filtering and no-match error |
| `internal/app/commands.go` | Modify | Fix owner `"gentleman-programming"` → `"nireneko"`; add `.zip` extension logic for windows |

## Interfaces / Contracts

### `CheckLatest` new signature

```go
// CheckLatest checks GitHub Releases for the latest version.
// goos/goarch determine which asset to select (e.g. "linux", "amd64").
// Returns version (without "v" prefix), asset download URL, and error.
func CheckLatest(owner, repo, goos, goarch string) (version, assetURL string, err error)
```

Asset matching logic:
```go
suffix := fmt.Sprintf("_%s_%s.", goos, goarch)
for _, asset := range release.Assets {
    if strings.Contains(asset.Name, suffix) {
        return version, asset.BrowserDownloadURL, nil
    }
}
return "", "", fmt.Errorf("no release asset found for %s/%s", goos, goarch)
```

### `RunUpgrade` extension logic

```go
ext := ".tar.gz"
if goos == "windows" {
    ext = ".zip"
}
assetName := fmt.Sprintf("drup_%s_%s_%s%s", version, goos, goarch, ext)
```

### `.goreleaser.yaml` key sections

```yaml
builds:
  - ldflags: ["-s", "-w", "-X", "github.com/nireneko/drup/internal/app.Version={{.Version}}"]
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]

archives:
  - name_template: "drup_{{.Version}}_{{.Os}}_{{.Arch}}"
    format: tar.gz
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"
```

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `CheckLatest` filters by OS/arch | `httptest.NewServer` returning controlled JSON with multiple assets; assert correct URL returned for each `{goos, goarch}` pair |
| Unit | `CheckLatest` no-match error | Server returns assets for wrong platform; assert error contains `"no release asset found"` |
| Unit | `findChecksum` with windows `.zip` filename | Verify checksum lookup works for `drup_0.2.0_windows_amd64.zip` |
| Integration | CI workflow syntax | `actionlint` or manual validation after push |
| E2E | Tag push → release → download | First real tag `v0.2.0` after merge validates the full flow |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. The auto-update mechanism was non-functional before this change (no releases existed). After merge:
1. Create tag `v0.2.0` to trigger first release
2. Verify release contains all 6 platform archives + `checksums.txt`
3. Test `drup upgrade` from `v0.2.0` against a future `v0.3.0`

## Open Questions

- [ ] Confirm repo owner is `nireneko` (not an org) — the proposal says to fix from `"gentleman-programming"` to `"nireneko"`, verify this matches the actual GitHub repo URL
