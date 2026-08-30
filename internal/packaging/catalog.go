package packaging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nireneko/drup/internal/mcp"
)

//go:generate go run ../../cmd/mcp-catalog-gen

const (
	generatedCatalogPath = "internal/mcp/tool-catalog.generated.json"
	readmePath           = "README.md"
	mcpToolsPath         = "docs/mcp-tools.md"
	generatedStart       = "<!-- BEGIN GENERATED MCP CATALOG -->"
	generatedEnd         = "<!-- END GENERATED MCP CATALOG -->"
)

// CatalogTool is the serializable MCP contract used by generated artifacts.
// ToolSpec remains the source of truth; this is only its stable projection.
type CatalogTool struct {
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	InputSchema   CatalogInputSchema `json:"input_schema"`
	Effect        mcp.ToolEffect     `json:"effect"`
	Role          string             `json:"role"`
	Preconditions []string           `json:"preconditions"`
	Stub          bool               `json:"stub"`
	SessionPolicy mcp.SessionPolicy  `json:"session_policy"`
	RequiresRun   bool               `json:"requires_run"`
	RetryEligible bool               `json:"retry_eligible"`
}

// CatalogInputSchema mirrors MCP's object-schema contract in JSON.
type CatalogInputSchema struct {
	Type       string                      `json:"type"`
	Properties map[string]mcp.ToolProperty `json:"properties"`
	Required   []string                    `json:"required"`
}

// BuildCatalogArtifacts renders every committed artifact from ToolSpecs.
// Its map is deliberately complete so the test suite can make drift a hard
// failure rather than relying on a convention to run the generator.
func BuildCatalogArtifacts(root string) (map[string][]byte, error) {
	catalog, err := generatedCatalog()
	if err != nil {
		return nil, err
	}
	registry, err := json.MarshalIndent(struct {
		Version string        `json:"version"`
		Tools   []CatalogTool `json:"tools"`
	}{Version: "v1", Tools: catalog}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal catalog: %w", err)
	}
	registry = append(registry, '\n')

	readme, err := os.ReadFile(filepath.Join(root, readmePath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", readmePath, err)
	}
	mcpTools, err := os.ReadFile(filepath.Join(root, mcpToolsPath))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", mcpToolsPath, err)
	}

	readme = normalizeLegacyREADME(readme)
	mcpTools = normalizeLegacyMCPTools(mcpTools)
	readme, err = replaceGeneratedBlock(readme, renderCatalogBlock(catalog, "MCP catalog"))
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", readmePath, err)
	}
	mcpTools, err = replaceGeneratedBlock(mcpTools, renderCatalogBlock(catalog, "MCP catalog contract"))
	if err != nil {
		return nil, fmt.Errorf("render %s: %w", mcpToolsPath, err)
	}

	return map[string][]byte{
		generatedCatalogPath: registry,
		readmePath:           readme,
		mcpToolsPath:         mcpTools,
	}, nil
}

// WriteCatalogArtifacts is the go:generate entry point.
func WriteCatalogArtifacts(root string) error {
	artifacts, err := BuildCatalogArtifacts(root)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		fullPath := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(fullPath, artifacts[path], 0o644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// CheckCatalogArtifacts compares committed artifacts against freshly rendered
// bytes without writing to the repository. It is the CI drift-check boundary.
func CheckCatalogArtifacts(root string) error {
	artifacts, err := BuildCatalogArtifacts(root)
	if err != nil {
		return err
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		actual, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		if !bytes.Equal(actual, artifacts[path]) {
			return fmt.Errorf("generated catalog drift in %s; run go generate ./...", path)
		}
	}
	return nil
}

func generatedCatalog() ([]CatalogTool, error) {
	specs := mcp.ToolSpecs()
	tools := make([]CatalogTool, 0, len(specs))
	for _, spec := range specs {
		if spec.Name == "" || spec.Effect == "" || len(spec.Properties) == 0 {
			return nil, fmt.Errorf("incomplete ToolSpec %q", spec.Name)
		}
		properties := make(map[string]mcp.ToolProperty, len(spec.Properties))
		for name, property := range spec.Properties {
			properties[name] = property
		}
		tools = append(tools, CatalogTool{
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: CatalogInputSchema{
				Type:       "object",
				Properties: properties,
				Required:   append([]string(nil), spec.Required...),
			},
			Effect:        spec.Effect,
			Role:          spec.Role,
			Preconditions: append([]string(nil), spec.Preconditions...),
			Stub:          spec.Stub,
			SessionPolicy: spec.SessionPolicy,
			RequiresRun:   spec.RequiresRun,
			RetryEligible: spec.RetryEligible,
		})
	}
	return tools, nil
}

func renderCatalogBlock(tools []CatalogTool, title string) []byte {
	stubCount := 0
	for _, tool := range tools {
		if tool.Stub {
			stubCount++
		}
	}
	var out strings.Builder
	out.WriteString(generatedStart)
	out.WriteString("\n\n## ")
	out.WriteString(title)
	out.WriteString(" (generated)\n\n")
	fmt.Fprintf(&out, "`ToolSpec` is the only source for these %d implemented MCP contracts: schemas, effects, guards, and stub visibility. Regenerate with `go generate ./...`; CI rejects byte drift. %d tools are available as transport stubs. Planned or obsolete tools are intentionally absent from this runtime catalog.\n\n", len(tools), stubCount)
	out.WriteString("| Tool | Required input | Effect | Guard contract | Stub |\n")
	out.WriteString("|---|---|---|---|---|\n")
	for _, tool := range tools {
		required := strings.Join(tool.InputSchema.Required, ", ")
		if required == "" {
			required = "—"
		}
		stub := "no"
		if tool.Stub {
			stub = "yes"
		}
		fmt.Fprintf(&out, "| `%s` | `%s` | `%s` | %s | %s |\n", tool.Name, required, tool.Effect, guardContract(tool), stub)
	}
	out.WriteString("\n**Side-effect assertions:** `read_only` changes no project or workflow state; `workflow` changes only persisted run authority; `mutating` requires the listed guard evidence before its handler runs. The manual tool dictionary below is explanatory and must not weaken this contract.\n\n")
	out.WriteString(generatedEnd)
	return []byte(out.String())
}

func guardContract(tool CatalogTool) string {
	parts := append([]string(nil), tool.Preconditions...)
	if tool.RequiresRun {
		parts = append(parts, "run")
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " + ")
}

func replaceGeneratedBlock(document, block []byte) ([]byte, error) {
	start := bytes.Index(document, []byte(generatedStart))
	end := bytes.Index(document, []byte(generatedEnd))
	if start < 0 || end < 0 || end < start {
		return nil, fmt.Errorf("missing generated catalog markers")
	}
	end += len(generatedEnd)
	return append(append([]byte(nil), document[:start]...), append(block, document[end:]...)...), nil
}

func normalizeLegacyREADME(document []byte) []byte {
	text := string(document)
	const oldStart = "### Core Tools (7 original)"
	const oldEnd = "---\n\n## Deterministic work vs. orchestration"
	if start := strings.Index(text, oldStart); start >= 0 {
		if end := strings.Index(text[start:], oldEnd); end >= 0 {
			text = text[:start] + generatedStart + "\n" + generatedEnd + "\n\n" + text[start+end:]
		}
	}
	text = strings.ReplaceAll(text, "MCP server exposes 31 tools with JSON types and schemas", "MCP server exposes a generated ToolSpec catalog with JSON types and schemas")
	text = strings.ReplaceAll(text, "Registers `drup mcp` as a user-scoped MCP server with 31 tools", "Registers `drup mcp` as a user-scoped MCP server with its generated ToolSpec catalog")
	text = strings.ReplaceAll(text, "The 31 exposed tools are documented", "The generated catalog tools are documented")
	text = strings.ReplaceAll(text, "the 31 exposed tools are documented", "the generated MCP catalog is documented")
	text = strings.ReplaceAll(text, "all 31 tools above run", "all generated catalog tools run")
	return []byte(text)
}

func normalizeLegacyMCPTools(document []byte) []byte {
	text := string(document)
	if !strings.Contains(text, generatedStart) {
		anchor := "## 1. Core Directives"
		if index := strings.Index(text, anchor); index >= 0 {
			text = text[:index] + generatedStart + "\n" + generatedEnd + "\n\n" + text[index:]
		}
	}
	return []byte(text)
}
