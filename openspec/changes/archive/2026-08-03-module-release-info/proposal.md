# Proposal: Module Release Info MCP Tool

## Intent

Agents deciding "is this module safe, and which version do I pin?" have no tool that answers it. `contrib_check` returns only `has_d11_release`; `contrib_upgrade_path` returns branches. Two signals the release-history feed publishes are parsed nowhere in drup:

- project-level `terms → Maintenance status` ("Actively maintained" … "Unsupported") — no struct in `internal/drupalorg` captures project-level terms at all.
- per-release `terms → Release type` ("Bug fixes", "New features", "Security update", **"Insecure"**) — `Terms` is parsed but only ever matched against the retired `"Core compatibility"` term.

Without them an agent can recommend pinning an insecure release of an abandoned module. Interpreting editorial term strings must not be left to the LLM: Go decides.

## Scope

### In Scope
- New curated fetch function in `internal/drupalorg` reusing `FetchReleaseHistory`, `doWithRetry`, and `constraintMatchesDrupal` — no new HTTP or constraint logic.
- Result shape: `module`, `found`, `maintenance_status`, `releases[]` with `version`, `core_compatibility` (raw range string), `release_type[]` (raw terms), `insecure` (Go-derived from the "Insecure" term), `security_covered`, `date`.
- Optional core-version filter parameter; filtering evaluates the single `<core_compatibility>` range via existing `constraintMatchesDrupal`, never string equality.
- One combined MCP tool (`module_release_info`) via the 3-file pattern: schema in `internal/mcp/server.go`, placeholder in `internal/mcp/tools.go`, `realHandleModuleReleaseInfo` in `internal/app/mcp_tools.go`.
- Test fixture matching the real feed shape (project-level terms, per-release Release type, single `core_compatibility`), replacing synthetic `testdata/release_d11.xml`.

### Out of Scope
- Caching/TTL for updates.drupal.org responses (flagged as a future concern).
- Any CLI/report presentation of this data beyond the MCP response.
- Splitting into two tools (`module_maintenance_status` + `module_releases`) — rejected; `module_info` precedent is one tool per concern.
- Changing `contrib_check` / `contrib_upgrade_path` behavior.

## Capabilities

### New Capabilities
None.

### Modified Capabilities
- `contrib-check`: parse project-level `Maintenance status`, per-release `Release type`, derive `insecure`; curated release-info lookup with optional core filter.
- `mcp-server`: add `module_release_info` tool, its `inputSchema`, and registration; tool-count requirements need correcting.

## Approach

Add a curated struct + one exported function beside `FetchReleaseHistory`, extending the existing XML structs with a project-level `terms` node rather than a second parser. Derive `insecure` in Go; pass unrecognized `Release type` values through unchanged (fail open). Handler is a thin unmarshal → call → `json.Marshal`, matching `contrib_upgrade_path`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `internal/drupalorg/drupalorg.go` | Modified | Project-level terms struct, curated fetch, term derivation |
| `internal/drupalorg/testdata/release_d11.xml` | Modified | Replace with real-shape fixture |
| `internal/drupalorg/drupalorg_test.go` | Modified | httptest coverage incl. insecure + filter |
| `internal/mcp/server.go` | Modified | Tool schema entry |
| `internal/mcp/tools.go` | Modified | Placeholder handler |
| `internal/app/mcp_tools.go` | Modified | Real handler + `WireMCPTools` |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Zero-release published project treated as error | Med | `{found: true, releases: []}` distinct from `<error>` unknown-project case |
| `Release type` vocabulary drift (editorial list, not versioned enum) | Med | Fail open: pass unknown terms through, never error |
| No caching → agent scanning many modules hammers updates.drupal.org | Med | Out of scope; flag for a later change |
| Extending shared XML structs regresses `UpgradePath`/`CheckRelease` | Low | Additive fields only; existing tests must stay green |
| Fixture replacement breaks tests that depend on the retired term | Med | Audit every `release_d11.xml` consumer before swap |
| `mcp-server` spec already self-contradicts (17 vs 20 tools; 24 registered) | High | Correct counts to actual in the delta spec |

## Rollback Plan

Revert the commit. The change is additive: no existing tool signature, state file, or on-disk asset changes, so an older binary and a newer one expose the same tools minus this one.

## Dependencies

None. Live endpoint shape already verified against `pathauto` and `commerce`.

## Success Criteria

- [ ] Tool returns `maintenance_status` for a real module (pathauto: "Actively maintained")
- [ ] A release carrying the "Insecure" term returns `insecure: true`
- [ ] Core filter `11` returns only releases whose range satisfies major 11 (e.g. `^10.2 || ^11`)
- [ ] Unknown module returns a clear not-found result, not a bare Go error
- [ ] Unrecognized `Release type` value passes through without error
- [ ] `go test ./...` and `go vet ./...` pass

## Proposal question round

Confirmed with the user before spec/design:

1. **Filter semantics** — `core_version` filtering returns only releases with `status == "published"` that also satisfy the requested core range; non-published releases are dropped.
2. **Unsupported project** — releases are listed normally regardless of project status; `maintenance_status` in the response is the warning signal, no separate refusal shape.
3. **Release cap** — no cap; return every release matching the applied filter.
4. **Derived fields** — `insecure` is the only Go-derived boolean; no `recommended` pick in this change.
5. **Error convention** — reuse the `status/message/suggestion` shape from `PatchSearchResult`.
