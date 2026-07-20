## Exploration: Auto-Update Release Workflow

### Current State

**Auto-update mechanism (`internal/update/update.go`)**:
- `CheckLatest(owner, repo)` hits `GET https://api.github.com/repos/{owner}/{repo}/releases/latest`
- Parses JSON response for `tag_name` (strips `v` prefix) and `assets[0].browser_download_url`
- Returns version string, asset URL, error
- **Bug**: takes `Assets[0]` blindly — no filtering by OS/arch. If multiple assets exist, it may download the wrong one.

**Upgrade command (`internal/app/commands.go:268-344`)**:
- Calls `update.CheckLatest("gentleman-programming", "drup")` — **owner mismatch**: git remote is `nireneko/drup`
- Constructs expected asset name: `drup_{version}_{goos}_{goarch}.tar.gz`
- Derives `checksums.txt` URL by replacing asset name in the download URL
- Calls `update.Download(assetURL, checksumURL, assetName)` which:
  - Downloads binary to temp file, computes SHA256
  - Fetches `checksums.txt`, finds matching line (`{hash}  {filename}`)
  - Ver hashes match, returns temp path
- Atomic replace: rename temp over current binary, chmod 0755
- Sets `PendingSync = true` in state.json for deferred skill sync

**Version injection**:
- `internal/app/app.go:6` — `var Version = "dev"` (set via ldflags at build time)
- Currently no build system sets this — manual `go build` leaves it as "dev"

**Build/packaging**:
- No `.goreleaser.yml`, no `Makefile`, no `Taskfile.yml`, no build scripts
- Manual: `go build ./cmd/drup` (from README)
- `openspec/config.yaml` mentions "Build: goreleaser, CI: GitHub Actions" — aspirational, not implemented
- README says releases are "próximamente" (coming soon)

**Existing CI/CD**:
- No `.github/` directory exists
- No workflows, no CI checks

**Reference project (gentle-ai)** — same author, established pattern:
- `.goreleaser.yaml` with: linux/darwin/windows × amd64/arm64, `tar.gz` (zip for windows), ldflags `-s -w -X main.version={{.Version}}`, `checksums.txt`, `name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `.github/workflows/release.yml`: triggers on `push: tags: - "v*"`, uses `goreleaser/goreleaser-action@v7.2.2` with `GITHUB_TOKEN`
- `.github/workflows/ci.yml`: format check + unit tests on PR/push

### Affected Areas

- `internal/update/update.go` — `CheckLatest` needs asset filtering by OS/arch (currently takes `Assets[0]`)
- `internal/app/commands.go` — owner `"gentleman-programming"` must match actual repo owner; checksum URL derivation is fragile
- `internal/app/app.go` — `Version` variable needs ldflags path alignment with goreleaser config
- `.goreleaser.yaml` — needs to be created
- `.github/workflows/release.yml` — needs to be created
- `.github/workflows/ci.yml` — should be created for consistency with gentle-ai

### Approaches

1. **GoReleaser + GitHub Actions (match gentle-ai pattern)** — Create `.goreleaser.yaml` modeled on gentle-ai's config, plus `release.yml` workflow triggered by tag push.
   - Pros: proven pattern from reference project, multi-platform builds, checksums, changelog, extensible (brew/scoop later), zero custom scripting
   - Cons: adds goreleaser as a build dependency (but it's standard for Go projects)
   - Effort: Low — mostly copy/adapt from gentle-ai

2. **Custom GitHub Actions workflow (no goreleaser)** — Write a workflow that uses `go build` with GOOS/GOARCH matrix, `shasum`, and `gh release create`.
   - Pros: no extra tooling, full control, simpler for small projects
   - Cons: must manually handle archive creation, checksums, cross-compilation matrix, asset naming — all things goreleaser does for free
   - Effort: Medium — more YAML, more edge cases

3. **GitHub CLI only (`gh release create`)** — Minimal workflow: build, archive, checksum, `gh release create` with assets.
   - Pros: simplest possible, no goreleaser config file
   - Cons: doesn't scale to multiple platforms cleanly, no changelog, no extensibility
   - Effort: Low — but limited ceiling

### Recommendation

**Approach 1: GoReleaser + GitHub Actions**, matching the gentle-ai pattern exactly.

Rationale:
- The reference project already uses this pattern successfully
- The auto-update code expects goreleaser-style output (`checksums.txt`, `drup_{version}_{os}_{arch}.tar.gz`)
- `openspec/config.yaml` already declares "Build: goreleaser, CI: GitHub Actions" as the intended stack
- Low effort — adapt gentle-ai's `.goreleaser.yaml` (71 lines) and `release.yml` (32 lines)
- Future-proof: can add brew/scoop taps later if needed

### Risks

- **Owner mismatch**: `commands.go` hardcodes `"gentleman-programming"` but git remote is `nireneko/drup`. Must fix before release workflow can work. Need to confirm which GitHub org owns the release target.
- **Fragile asset selection**: `CheckLatest` takes `Assets[0]` — with multi-platform builds this will often return the wrong binary. Must add OS/arch filtering.
- **Checksum URL derivation**: The logic in `commands.go:292-303` has multiple fallbacks and edge cases. With goreleaser's predictable URL structure, this can be simplified.
- **Version ldflags path**: drup uses `internal/app.Version` (unexported package var), gentle-ai uses `main.version`. Goreleaser config must target the correct path: `-X github.com/nireneko/drup/internal/app.Version={{.Version}}`.
- **No existing tags**: `git tag -l` returns empty. First tag needs to be created (e.g., `v0.2.0`) to trigger the workflow.

### Ready for Proposal

**Yes.** The exploration reveals a clear path:
1. Fix the owner mismatch in `commands.go`
2. Fix `CheckLatest` to filter assets by OS/arch
3. Create `.goreleaser.yaml` adapted from gentle-ai
4. Create `.github/workflows/release.yml` (tag trigger → goreleaser)
5. Optionally create `.github/workflows/ci.yml` for consistency

The auto-update mechanism is already 90% complete — it just needs releases to exist with the right asset format. GoReleaser produces exactly that format.
