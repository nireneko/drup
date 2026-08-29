# Tasks: Upgrade Workflow Safety Foundation

## Review Workload Forecast

| Field | Value |
|---|---|
| Estimated changed lines | 600–760 |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 → PR 2 → PR 3 |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High

### Suggested Work Units

| Unit | Goal | Likely PR | Focused test command | Runtime harness | Rollback boundary |
|---|---|---|---|---|---|
| 1 | Core gate and numeric matrix | PR 1 | `go test ./internal/coreupgrade ./internal/app` | N/A—command seams cover filesystem/subprocess behavior | `internal/coreupgrade/*`, matrix code/tests |
| 2 | Explicit preparation and read-only analysis | PR 2 | `go test ./internal/app` | N/A—captured `RunWithEnv` sequences | preparation/scan/validate/autofix handlers/tests |
| 3 | MCP schema and retry allowlist | PR 3 | `go test ./internal/mcp` | N/A—stubbed handlers prove dispatch | `internal/mcp/{server,tools}.go`, tests |

## Phase 1: Core Safety and Matrix

- [x] 1.1 RED: table-test `internal/coreupgrade/*_test.go` equal, lower, skipped, dirty targets and canonical-root success/relative/`..` rejection; unsafe/no-op paths make no Git, checkpoint, write, or subprocess call.
- [x] 1.2 GREEN: guard `internal/coreupgrade/{check,apply}.go` before mutation; return successful “already at target” no-op, reject lower/skipped roots, and update only immediate-next core constraints.
- [x] 1.3 RED: table-test `internal/app/*_test.go` PHP 8.4 numeric selection and unknown Drupal `99` error.
- [x] 1.4 GREEN: parse matrix version components numerically in `internal/app/mcp_tools.go` while preserving lookup responses.

## Phase 2: Preparation and Read-only Analysis

- [x] 2.1 RED: capture `internal/app/{commands,guard,mcp_tools}_test.go` command sequences for uninstalled, disabled/conflicting, and enabled Upgrade Status; enabled performs no mutation.
- [x] 2.2 GREEN: add guarded `prepare_upgrade_status` in `internal/app/mcp_tools.go`: install if absent, remove conflict, enable, and rebuild cache.
- [x] 2.3 RED: capture prepared, disabled, missing, invalid-path, and configuration-conflict scan/upgrade-scan tests; unprepared returns guidance with no Composer/config/enable/cache command.
- [x] 2.4 GREEN: make scan and upgrade-scan prerequisite checks and analysis-only; register preparation schema/stub in `internal/mcp/{server,tools}.go`.
- [x] 2.5 RED: test read-only `validate` zero/all/module-filtered findings and evidence hash; assert no `updb`, cache, install, enable, or config command.
- [x] 2.6 GREEN: implement that `validate` contract in `internal/app/mcp_tools.go`.
- [x] 2.7 RED: prove `autofix` runs rector without analysis in `internal/app/mcp_tools_test.go`.
- [x] 2.8 GREEN: remove the autofix rescan in `internal/app/mcp_tools.go`.

## Phase 3: Retry Policy and Verification

- [x] 3.1 RED: stub `internal/mcp/mcp_test.go` for scan two transient retries/success, validate exhaustion (three calls/two records), mutator single call, and `command not found` no retry.
- [x] 3.2 GREEN: restrict `internal/mcp/server.go` retry dispatch to read-only scan, upgrade-scan, and validate; retain backoff and retry metrics.
- [x] 3.3 Run `go test ./...`, `go vet ./...`, and `gofmt -w` on modified Go files; confirm each spec scenario and threat RED test passes.
