# Tasks: Fix Retrospective Bugs

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 200–250 |
| 400-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR |
| Delivery strategy | single-pr |
| Chain strategy | size-exception |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: Low

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|------|------|-----------|----------------------|-----------------|-------------------|
| 1 | All 4 bug fixes + tests | Single PR | `go test ./internal/app/... ./internal/installer/... ./internal/drupalorg/...` | `drup init` in temp dir with `drupal/core-recommended` composer.json | Revert commit; re-run `drup install` |

## Phase 1: Foundation

- [x] 1.1 Create `internal/packaging/templates/opencode/commands/drup.md` with valid OpenCode command JSON invoking drup MCP.
- [x] 1.2 Add `doWithRetry(fn func() (*http.Response, error))` in `internal/drupalorg/drupalorg.go` — 3 attempts, 500ms base exponential backoff, retry on 412/429/5xx/timeout.

## Phase 2: Core Bug Fixes

- [x] 2.1 Modify `RunInit()` in `internal/app/commands.go` to check both `drupal/core` and `drupal/core-recommended` in require map, mirroring `checkCoreReadiness()` pattern at line ~904.
- [x] 2.2 Modify `resolveFilePath()` in `internal/installer/installer.go` to add `strings.HasPrefix(path, "skills/")` branch: strip prefix, map to `{SkillsDir}/{rest}`, deduplicate `SKILL.md` suffix.

## Phase 3: Testing

- [x] 3.1 Add table-driven tests in `internal/app/commands_test.go`: `drupal/core` present, `drupal/core-recommended` present, neither present. Use `t.TempDir()` + synthetic `composer.json`.
- [x] 3.2 Add table-driven tests in `internal/installer/installer_test.go`: `skills/skills/foo/SKILL.md`, `skills/foo/SKILL.md`, `SKILL.md` as dir segment, `commands/drup.md`. Cover Claude/OpenCode/Codex adapters.
- [x] 3.3 Add tests in `internal/drupalorg/drupalorg_test.go` using `httptest.Server`: success-first-attempt, 412-then-success, all-3-fail, non-retryable 404 returns immediately.
- [x] 3.4 Add integration test in `internal/installer/installer_test.go`: OpenCode adapter writes `commands/drup.md` to expected path; non-OpenCode adapter does not.

## Phase 4: Manual Validation

- [x] 4.1 Run `drup init` in a temp project with `drupal/core-recommended` in `composer.json` — verify no error.
- [x] 4.2 Run `drup install` for OpenCode — verify `~/.config/opencode/skills/<name>/SKILL.md` has no nested `skills/skills/` paths and `~/.config/opencode/commands/drup.md` exists.
