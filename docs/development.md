# Development

`drup` is a Go module (`github.com/nireneko/drup`) with the CLI in `cmd/drup` and most behavior under `internal/`.

## Build and test

```bash
# Build the CLI.
GOCACHE=/tmp/drup-go-build go build ./cmd/drup

# Run the complete test suite.
GOCACHE=/tmp/drup-go-build go test ./...

# Check generated MCP catalog bytes without writing files.
GOCACHE=/tmp/drup-go-build go run ./cmd/mcp-catalog-gen --check

# Check the patch for whitespace errors.
git diff --check
```

Use a writable `GOCACHE` when the default Go build cache is unavailable in a sandboxed environment.

## Change boundaries

| Area | Source of truth |
|---|---|
| CLI command dispatch/help | `internal/app/app.go` |
| CLI behavior | `internal/app/*.go` and focused tests |
| MCP schemas, effects, guards, stubs | `internal/mcp.ToolSpec` |
| Generated MCP catalog | `cmd/mcp-catalog-gen`, `internal/mcp/tool-catalog.generated.json`, generated block in `docs/mcp-tools.md` |
| Agent skills and roles | `internal/packaging/templates/` |
| Installer behavior | `internal/installer/` |
| Durable run transitions | `internal/runstate/` |

Do not hand-edit generated content between `BEGIN GENERATED MCP CATALOG` and `END GENERATED MCP CATALOG` in `docs/mcp-tools.md`.

## Documentation maintenance

When a command, guard, persistence record, or generated agent template changes:

1. Update the relevant user-facing document in `docs/` and README links.
2. Verify the claim against implementation and tests, not only a roadmap or prior documentation.
3. Regenerate the MCP catalog if `ToolSpec` changed.
4. Run Markdown link checks and `git diff --check`.
5. Keep examples safe: use absolute placeholder paths, do not include secrets, and label operations that mutate a project.

Documentation should describe shipped behavior, boundaries, and recovery expectations. Proposed work belongs in roadmap/spec material, not the product reference.
