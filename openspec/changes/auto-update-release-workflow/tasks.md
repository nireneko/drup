# Tasks: Auto-Update Release Workflow

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~200 (6 files, 3 new + 3 modified) |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | pending |

Decision needed before apply: Yes
Chained PRs recommended: No
Chain strategy: pending
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | Fix `CheckLatest` OS/arch filtering + owner/zip bug | PR 1 | `go test ./internal/update/ ./internal/app/` | `drup upgrade` against test server | `internal/update/update.go`, `internal/update/update_test.go`, `internal/app/commands.go` |
| 2 | Add goreleaser config + CI/release workflows | PR 1 (same) | `goreleaser check` + workflow YAML lint | Tag push `v*` triggers release | `.goreleaser.yaml`, `.github/workflows/release.yml`, `.github/workflows/ci.yml` |

## Phase 1: Bug Fixes — CheckLatest Asset Filtering

- [x] 1.1 Modify `internal/update/update.go`: change `CheckLatest(owner, repo string)` → `CheckLatest(owner, repo, goos, goarch string)`. Replace `Assets[0]` with suffix-matching loop: build `suffix := fmt.Sprintf("_%s_%s.", goos, goarch)`, iterate assets, match via `strings.Contains`. Return error `"no release asset found for %s/%s"` on no match.
- [x] 1.2 Update `internal/update/update_test.go`: fix existing `TestCheckLatest` call to pass `"linux", "amd64"`. Add table-driven `TestCheckLatest_PlatformFilter` with cases: linux/amd64 → correct tar.gz URL, darwin/arm64 → correct tar.gz URL, windows/amd64 → correct zip URL, no-match → error containing `"no release asset found"`. Use `httptest.NewServer` returning multi-asset JSON.
- [x] 1.3 Verify: `go test ./internal/update/ -v -run TestCheckLatest` — all cases pass.

## Phase 2: Bug Fixes — Owner and Windows Archive Handling

- [x] 2.1 In `internal/app/commands.go` `RunUpgrade()`: change owner from `"gentleman-programming"` to `"nireneko"` in `CheckLatest` call. Add `goos, goarch` params from `runtime.GOOS`/`runtime.GOARCH` (already computed below — move usage up to call site).
- [x] 2.2 In `RunUpgrade()`: add `ext` variable — `".zip"` if `goos == "windows"`, else `".tar.gz"`. Use `ext` in `assetName` construction instead of hardcoded `.tar.gz`. Update `CheckLatest` call to pass `goos, goarch`.
- [x] 2.3 Verify: `go build ./...` compiles, `go test ./internal/update/ ./internal/app/` passes.

## Phase 3: Release Infrastructure

- [x] 3.1 Create `.goreleaser.yaml`: builds with `ldflags: ["-s", "-w", "-X", "github.com/nireneko/drup/internal/app.Version={{.Version}}"]`, `goos: [linux, darwin, windows]`, `goarch: [amd64, arm64]`. Archives with `name_template: "drup_{{.Version}}_{{.Os}}_{{.Arch}}"`, `format: tar.gz`, `format_overrides` for windows → zip. Checksum with `name_template: "checksums.txt"`.
- [x] 3.2 Create `.github/workflows/release.yml`: trigger on `push: tags: - "v*"`. Jobs: checkout, setup-go, run `goreleaser/goreleaser-action@v6` with `GITHUB_TOKEN`.
- [x] 3.3 Create `.github/workflows/ci.yml`: trigger on `pull_request` and `push` to main. Steps: checkout, setup-go, run `gofmt` check (`gofmt -l .` + fail if output), `go vet ./...`, `go test ./...`.
- [x] 3.4 Verify: `goreleaser check` passes locally (if goreleaser installed) or YAML syntax valid.

## Phase 4: Final Verification

- [x] 4.1 Run full test suite: `go test ./...` — all pass.
- [x] 4.2 Run `go vet ./...` — clean.
- [x] 4.3 Run `gofmt -l .` — no output (all formatted).
