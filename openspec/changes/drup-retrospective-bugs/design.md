# Design: Fix Retrospective Bugs (drup-retrospective-bugs)

## Technical Approach

Four localized bug fixes following existing codebase patterns. Each fix is isolated to a single function with table-driven tests. No new packages, no refactors.

## Architecture Decisions

| Decision | Options | Tradeoff | Choice |
|----------|---------|----------|--------|
| `RunInit` core check | (a) mirror `checkCoreReadiness` loop, (b) extract shared helper | (a) 3-line change, (b) over-engineering for two call sites | Mirror the existing `checkCoreReadiness` pattern (line 904) — iterate `["drupal/core", "drupal/core-recommended"]` |
| `resolveFilePath` sub-skill paths | (a) add `strings.HasPrefix(path, "skills/")` case, (b) strip all `skills/` prefixes in default | (a) explicit branch, (b) fragile | Add explicit `skills/` prefix case that strips the prefix and maps to `{SkillsDir}/{rest}`, plus `SKILL.md` dedup |
| HTTP retry | (a) inline retry in each caller, (b) single `doWithRetry` wrapper around `HTTPClient.Get` | (a) duplication across 6 call sites, (b) one function, testable in isolation | Single `doWithRetry(fn func() (*http.Response, error))` wrapper |
| Slash command template | (a) hardcode in installer, (b) embed as `templates/opencode/commands/drup.md` | (a) breaks packaging pattern, (b) follows existing embed+render flow | Create template file; `resolveFilePath` `commands/` case already handles routing |

## Data Flow

### Fix 1: RunInit core check
```
composer.json → parse require → check "drupal/core" OR "drupal/core-recommended" → pass/fail
```

### Fix 2: resolveFilePath
```
Template path "skills/skills/foo/SKILL.md"
  → HasPrefix "skills/" → strip → "skills/foo/SKILL.md"
  → HasPrefix "skills/" → strip → "foo/SKILL.md"
  → default → {SkillsDir}/foo/SKILL.md/SKILL.md  ← BUG: SKILL.md treated as dir

Fixed:
  → HasPrefix "skills/" → strip prefix chain → "foo/SKILL.md"
  → If ends with "SKILL.md" → {SkillsDir}/foo/SKILL.md
  → Else → {SkillsDir}/{path}/SKILL.md
```

### Fix 3: Slash command
```
packaging.Render("opencode") → walks templates/opencode/ → finds commands/drup.md
  → files["commands/drup.md"] = content
  → resolveFilePath(agent, "commands/drup.md") → {CommandsDir}/drup.md
  → writeFileContent → ~/.config/opencode/commands/drup.md
```

### Fix 4: HTTP retry
```
Caller → doWithRetry(func() { HTTPClient.Get(url) })
  → attempt 1: fail (412/timeout/5xx)
  → sleep 500ms → attempt 2: fail
  → sleep 1000ms → attempt 3: return result/error
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/app/commands.go` | Modify | `RunInit()`: check both `drupal/core` and `drupal/core-recommended` in require map |
| `internal/installer/installer.go` | Modify | `resolveFilePath()`: add `skills/` prefix handling with dedup |
| `internal/packaging/templates/opencode/commands/drup.md` | Create | OpenCode slash command template |
| `internal/drupalorg/drupalorg.go` | Modify | Add `doWithRetry` wrapper; replace all `HTTPClient.Get` calls |
| `internal/app/commands_test.go` | Modify | Table-driven tests for `RunInit` with `drupal/core`, `drupal/core-recommended`, and missing |
| `internal/installer/installer_test.go` | Modify | Table-driven tests for `resolveFilePath` covering sub-skill paths across all adapters |
| `internal/drupalorg/drupalorg_test.go` | Modify | Tests for retry: success-first, 412-then-success, all-fail, non-retryable (404) |

## Interfaces / Contracts

```go
// doWithRetry — new unexported function in drupalorg package
func doWithRetry(fn func() (*http.Response, error)) (*http.Response, error)
// 3 attempts max, 500ms base exponential backoff
// Retryable: 412, 429, 500, 502, 503, 504, timeout errors
```

No new exported types or interfaces.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | `RunInit` with `drupal/core`, `drupal/core-recommended`, neither | Table-driven with `t.TempDir()` + synthetic `composer.json` |
| Unit | `resolveFilePath` for sub-skill paths across Claude/OpenCode/Codex | Table-driven: `skills/skills/foo/SKILL.md`, `skills/foo/SKILL.md`, `SKILL.md`, `commands/drup.md` |
| Unit | `doWithRetry` retry behavior | `httptest.Server` returning configurable status codes per attempt |
| Integration | Slash command written on install | `t.TempDir()` + `OpenCodeAdapter` + verify file content |

## Threat Matrix

N/A — no routing, shell, subprocess, VCS/PR automation, executable-file classification, or process-integration boundary.

## Migration / Rollout

No migration required. All changes are backward-compatible bug fixes. Users re-run `drup install` to pick up the slash command template and fixed path resolution.

## Open Questions

None.
