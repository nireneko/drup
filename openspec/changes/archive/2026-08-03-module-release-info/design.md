# Design: Module Release Info MCP Tool

## Technical Approach

Additive extension of `internal/drupalorg`: the existing `releaseHistoryFull`/`releaseFull` XML structs gain the two nodes the feed already publishes (project-level `<terms>`, per-release `<security covered>`), and one new exported function `ModuleReleaseInfo` curates them. It reuses `FetchReleaseHistory` (HTTP + retry + `<error>` detection), `releaseHistoryBranch`, `majorFromVersion`, and `constraintMatchesDrupal` — no new HTTP call, no new constraint parser, no second XML parser. The MCP surface follows the repo's 3-file pattern exactly as `contrib_upgrade_path` does (`internal/mcp/server.go` schema → `internal/mcp/tools.go` placeholder → `internal/app/mcp_tools.go` real handler + `WireMCPTools`).

## Architecture Decisions

| Decision | Choice | Alternatives rejected | Rationale |
|---|---|---|---|
| Function signature | `ModuleReleaseInfo(module, coreVersion string) (*ReleaseInfoResult, error)`; `""` = unfiltered | `coreVersion *int` (proposal sketch); `int` with `0` sentinel | A string feeds `releaseHistoryBranch` (D7 projects resolve to `7.x`) and `majorFromVersion` for the int the constraint matcher needs — both already exist. Matches `UpgradePath(module, current, target string)` and the string version params of every existing tool schema. No nil handling. |
| XML parsing | Add `Terms []term \`xml:"terms>term"\`` to `releaseHistoryFull` and `Security` to `releaseFull` | Separate parse structs for the curated path | Go's `terms>term` binds only to `project`'s direct child, so release terms are unaffected; additive fields keep `UpgradePath`/`ModuleInfo` green. |
| not-found vs zero-release | `FetchReleaseHistory` already returns `(nil, nil)` for HTTP 404 **and** the HTTP-200 `<error>` document → `found:false, status:"not_found"`. Non-nil `rh` with no matching releases → `found:true, status:"no_releases_found", releases:[]` | Sniffing `<error>` again in the new function | Detection already exists in one place; reusing it makes the two cases structurally distinguishable for free. |
| Error convention | Three semantic statuses (`releases_found`/`no_releases_found`/`not_found`) with `message`+`suggestion`; genuine transport/parse failures return a Go `error` | Swallow every failure into `status:"error"` like `SearchPatches` | Unknown module must not be a bare error (success criterion), but `doWithRetry` builds a descriptive exhaustion error that MCP already surfaces; swallowing it would hide the HTTP cause. |
| Release admission | A release is listed only when `status == "published"`; the filtered path additionally requires `constraintMatchesDrupal(CoreCompat, major)` | Expose a per-release `status` field and list unpublished releases | Unpublished means retracted, never pinnable, so the agreed response field list stays exactly as proposed. Empty `<core_compatibility>` is dropped **only** when a filter is requested (fail closed on an explicit compatibility assertion). |
| `insecure` derivation | Set only from a term with `Name == "Release type"` whose `strings.TrimSpace(Value) == "Insecure"` (case-sensitive, Drupal.org vocabulary) | `strings.Contains`; case-insensitive; any term name | Exact match cannot false-positive on vocabulary drift, and scoping to `Release type` stops a future project-level term hijacking the boolean; `TrimSpace` absorbs pretty-printed XML. Every value — known or not — is copied verbatim into `release_type[]` and never errors (fail open). |
| Absent maintenance status | Emit `"unknown"` | `""` / `omitempty` | An empty string reads as "fine" to an agent; `"unknown"` is a normalization, not an interpretation. |
| Fixture strategy | **Add** `internal/drupalorg/testdata/release_info_real.xml`; leave `release_d11.xml` untouched | Replace `release_d11.xml` in place (proposal wording) | Audit: the only live consumer is `drupalorg_test.go:24` `TestCheckRelease_HasD11`, which asserts `HasD11` from the retired `Core compatibility`/`Drupal 11` term. A rewrite whose first release is not `^11` would silently drop `parseReleaseXML` into its `fetchInfoYML` network fallback and make that test flaky. A dedicated fixture also carries the new edge cases (Insecure term, unpublished release, missing maintenance status) without distorting a shared one. |

## Data Flow

    module_release_info{module_machine_name, core_version?}
        │  internal/mcp/server.go schema validation
        ▼
    realHandleModuleReleaseInfo (internal/app/mcp_tools.go)
        │  json.Unmarshal → moduleNamePattern guard → core_version guard
        ▼
    drupalorg.ModuleReleaseInfo(module, coreVersion)
        │
        ├─ releaseHistoryBranch(coreVersion) ─▶ FetchReleaseHistory ─▶ doWithRetry ─▶ updates.drupal.org
        │        rh == nil ──▶ {found:false, status:"not_found", releases:[]}
        ▼
    curate: project terms ─▶ maintenance_status
            per release: status=="published"? ─▶ [filter: constraintMatchesDrupal(CoreCompat, majorFromVersion(coreVersion))]
                         terms(Release type) ─▶ release_type[] + insecure
                         <security covered> ─▶ security_covered
        ▼
    *ReleaseInfoResult ─▶ json.Marshal ─▶ MCP content

## File Changes

| File | Action | Description |
|---|---|---|
| `internal/drupalorg/drupalorg.go` | Modify | `ReleaseInfoResult`, `ReleaseDetail`, `releaseSecurity`; project `Terms` on `releaseHistoryFull`, `Security` on `releaseFull`; `ModuleReleaseInfo`; unexported `curateReleases` |
| `internal/drupalorg/testdata/release_info_real.xml` | Create | Real-shape fixture: project `<terms>` with `Maintenance status`, per-release `Release type` (incl. `Insecure`), `<security covered="1">`, single `<core_compatibility>`, one unpublished release |
| `internal/drupalorg/drupalorg_test.go` | Modify | `httptest` cases: maintenance status, insecure, filter, unfiltered, not-found, zero-release, unknown term |
| `internal/mcp/server.go` | Modify | `toolRegistry["module_release_info"]` (28 → 29 schemas) |
| `internal/mcp/tools.go` | Modify | `defaultTools()` placeholder (24 → 25) |
| `internal/app/mcp_tools.go` | Modify | `realHandleModuleReleaseInfo` + `WireMCPTools` registration (28 → 29) |
| `internal/mcp/mcp_test.go` | Modify | **Blocking**: hard-coded counts at lines 123-124 (`24`) and 392-393 (`28`) must become `25` and `29`, comment text included |
| `openspec/specs/mcp-server/spec.md` | Modify | Correct the self-contradicting counts: line 5 (`17`), line 320 (`7 + 10 = 17`), lines 357/371/375 (`20`) → actual 25 placeholder-backed tools / 29 registered handlers and schemas |
| `docs/mcp-tools.md` | Modify | Tool Dictionary entry + totals (§1/§5 currently claim 25 and 26); required by the `tools.go:22` mirror convention |

## Interfaces / Contracts

```go
type ReleaseInfoResult struct {
	Status            string          `json:"status"` // releases_found | no_releases_found | not_found
	Module            string          `json:"module"`
	Found             bool            `json:"found"`
	MaintenanceStatus string          `json:"maintenance_status"` // "unknown" when absent
	CoreVersionFilter string          `json:"core_version_filter,omitempty"`
	Message           string          `json:"message"`
	Suggestion        string          `json:"suggestion"`
	Releases          []ReleaseDetail `json:"releases"`
}

type ReleaseDetail struct {
	Version           string   `json:"version"`
	Tag               string   `json:"tag"`
	CoreCompatibility string   `json:"core_compatibility"`
	ReleaseType       []string `json:"release_type"` // verbatim feed terms
	Insecure          bool     `json:"insecure"`     // Go-derived
	SecurityCovered   bool     `json:"security_covered"`
	Date              string   `json:"date"` // raw feed value (Unix epoch string), as UpgradePath already exposes it
}

type releaseSecurity struct {
	Covered string `xml:"covered,attr"`
	Text    string `xml:",chardata"`
}
```

MCP schema: `Description: "Get maintenance status and curated release list for a contrib module"`, properties `module_machine_name` (string, required) and `core_version` (string, optional — "Drupal core major to filter by, e.g. 11"), `Required: []string{"module_machine_name"}`. Handler guards: `moduleNamePattern` (same as `contrib_upgrade_path`), and a non-empty `core_version` whose `majorFromVersion` is `0` returns `fmt.Errorf("invalid core_version: %s", ...)` rather than silently returning everything.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | insecure derivation, unknown-term pass-through, published gate, `"unknown"` maintenance default, empty `core_compatibility` under filter | Table-driven over `curateReleases` |
| Integration | maintenance status, filter `11` vs `^10.2 \|\| ^11`, `<error>` → not_found, zero-release → `found:true, releases:[]` | `httptest` + `SetHTTPClientForTest` + `releaseHistoryVersionURL` override (existing pattern) |
| Integration | tool advertised, dispatchable, counts 25/29 | `internal/mcp/mcp_test.go` registry + count assertions |
| Unit | invalid module name, invalid `core_version` | `internal/app/mcp_tools_test.go`, mirroring `TestContribUpgradePath_InvalidName` |
| Regression | `TestCheckRelease_HasD11`, `UpgradePath`, `ModuleInfo` unchanged | `go test ./...`, `go vet ./...` |

## Threat Matrix

| Boundary | Applicability | Design response | Planned RED tests |
|---|---|---|---|
| Documentation-like paths | N/A: no file classification or execution | — | — |
| Git repository selection | N/A: no git invocation, no `project_path` parameter | — | — |
| Commit state | N/A: read-only HTTP tool | — | — |
| Push state | N/A: no push automation | — | — |
| PR commands | N/A: no PR automation | — | — |

No shell, subprocess, or VCS boundary is introduced. The only untrusted input reaches a URL path segment, which `moduleNamePattern` (`^[a-z][a-z0-9_]*$`) already constrains in the handler — covered by the invalid-name test above.

## Migration / Rollout

No migration. Purely additive: one new tool, no changed signatures, no state-file or on-disk asset change. Rollback = revert the commit; an older binary simply lacks the tool. Single PR: forecast well under the 400-line budget.

## Open Questions

- [ ] Confirm the exact `<security covered="1">` attribute name against a live `curl https://updates.drupal.org/release-history/pathauto/current` capture while writing the fixture; if the feed differs, only the struct tag changes.
- [ ] `date` stays a raw Unix-epoch string for consistency with `UpgradePath`; a human-readable ISO field would be a new derived field, deferred.
