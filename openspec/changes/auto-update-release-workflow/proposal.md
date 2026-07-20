# Proposal: Auto-Update Release Workflow

## Intent

The auto-update mechanism (`drup upgrade`) is fully coded but has no releases to discover. No build system, no CI, no GitHub Releases exist. Pushing a tag today does nothing. This change wires up goreleaser + GitHub Actions so that tagging a version produces the exact artifacts the auto-update code expects, and fixes two bugs that would surface once releases exist.

## Scope

### In Scope
- `.goreleaser.yaml` — multi-platform builds (linux/darwin/windows × amd64/arm64), checksums, `name_template` matching `drup_{version}_{os}_{arch}.tar.gz`
- `.github/workflows/release.yml` — tag-triggered (`v*`) goreleaser execution
- `.github/workflows/ci.yml` — format check + unit tests on PR/push
- Fix owner mismatch in `internal/app/commands.go` (`"gentleman-programming"` → correct owner)
- Fix `CheckLatest` in `internal/update/update.go` — filter assets by OS/arch instead of taking `Assets[0]`

### Out of Scope
- Homebrew/scoop tap publishing (goreleaser supports it later)
- Docker image builds
- E2E testing of the upgrade flow against real GitHub releases
- Changing the auto-update protocol (checksums.txt format, atomic replace)

## Capabilities

### New Capabilities
- `release-automation`: goreleaser config and GitHub Actions release workflow that produces versioned, checksummed, multi-platform archives on tag push

### Modified Capabilities
- `self-update`: fix owner hardcode and blind asset selection so `CheckLatest` returns the correct platform-specific asset

## Approach

GoReleaser + GitHub Actions, matching the gentle-ai reference project (same author, proven pattern). Goreleaser handles cross-compilation, archive naming, checksums, and changelog. The release workflow triggers on `push: tags: - "v*"` and runs goreleaser with `GITHUB_TOKEN`. A separate CI workflow runs `gofmt` + `go test ./...` + `go vet ./...` on PRs.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `.goreleaser.yaml` | New | Build config: targets, ldflags (`-X github.com/nireneko/drup/internal/app.Version`), archives, checksums |
| `.github/workflows/release.yml` | New | Tag-triggered goreleaser action |
| `.github/workflows/ci.yml` | New | PR/push format + test + vet checks |
| `internal/update/update.go` | Modified | `CheckLatest` filters assets by `{os}_{arch}` suffix instead of `Assets[0]` |
| `internal/app/commands.go` | Modified | Owner string corrected to match actual repo owner |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Owner mismatch — which org owns the release target? | Med | Confirm repo ownership before merge; code must match |
| Asset selection breaks on unexpected naming | Low | Goreleaser `name_template` is deterministic; filter by exact suffix match |
| Version ldflags path mismatch | Low | Goreleaser targets `internal/app.Version`; verify with `drup --version` after first release |
| No existing tags to test workflow | Low | Create `v0.2.0` tag after merge to trigger first release |

## Rollback Plan

Delete `.goreleaser.yaml` and `.github/workflows/release.yml`. Revert `internal/update/update.go` and `internal/app/commands.go` changes. No runtime impact — auto-update was non-functional before this change.

## Dependencies

- Go 1.26.0 (already in `go.mod`)
- GitHub Actions minutes (standard free tier)
- goreleaser (runs via action, no local install needed for CI)

## Success Criteria

- [ ] Pushing tag `v*` triggers release workflow and produces a GitHub Release
- [ ] Release contains `drup_{version}_{os}_{arch}.tar.gz` for all target platforms + `checksums.txt`
- [ ] `drup upgrade` on an older version discovers the new release and downloads the correct platform asset
- [ ] Checksum verification passes end-to-end
- [ ] CI workflow runs `gofmt`, `go vet`, `go test ./...` on every PR
