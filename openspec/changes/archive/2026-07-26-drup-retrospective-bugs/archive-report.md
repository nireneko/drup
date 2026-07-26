# Archive Report — drup-retrospective-bugs

**Date:** 2026-07-26
**Commit:** `eddf7c6` on main

## Summary

Fixed 3 code bugs identified from retrospective and install-bug analysis, plus added HTTP retry/backoff to Drupal.org API calls.

## Changes Applied

| Fix | Files Changed | Tests Added |
|-----|---------------|-------------|
| `RunInit` core-recommended arg | `internal/app/commands.go` | 2 |
| `resolveFilePath` nested paths | `internal/installer/installer.go` | 5 |
| OpenCode slash command | `internal/packaging/templates/opencode/commands/drup.md` | 1 |
| Drupal.org HTTP retry/backoff | `internal/drupalorg/drupalorg.go` | 5 |

**Total:** 14 new tests, 16 files changed, 1057 insertions, 9 deletions

## Verification

- All 20 packages pass (`go test ./...`)
- `go vet` clean
- Strict TDD followed (tests written first)

## Artifacts

- `exploration.md` — 15 of ~20 issues already fixed, 3 code bugs identified
- `proposal.md` — 4 fixes scoped
- `specs/installer/spec.md`, `specs/platform-bootstrap/spec.md`, `specs/drupal-org-resilience/spec.md`
- `design.md`, `tasks.md`, `verify-report.md`

## Untracked (not in commit)

- `UPGRADE-RETROSPECTIVA.md` — per user request
- `informe-drup-bugs.md` — per user request
