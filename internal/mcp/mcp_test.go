package mcp

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestServer_HandleRequest_Scan(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "scan",
			"arguments": {"project_path": "/tmp/test"}
		}`),
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	err := server.handleRequest(req)
	if err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	// Parse response.
	var resp JSONRPCResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestServer_HandleRequest_UnknownTool(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "nonexistent",
			"arguments": {}
		}`),
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	err := server.handleRequest(req)
	if err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	if resp.Error == nil {
		t.Error("expected error for unknown tool, got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}
}

func TestServer_HandleRequest_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	// Simulate invalid JSON input.
	err := server.handleRaw([]byte("{invalid"))
	if err != nil {
		t.Fatalf("handleRaw error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	if resp.Error == nil {
		t.Error("expected parse error, got nil")
	}
	if resp.Error.Code != -32700 {
		t.Errorf("error code = %d, want -32700", resp.Error.Code)
	}
}

func TestServer_ListTools(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      3,
		Method:  "tools/list",
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	err := server.handleRequest(req)
	if err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	// Check that result contains tools.
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("invalid result: %v", err)
	}

	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("missing tools array in result")
	}
	if len(tools) != 22 {
		t.Errorf("len(tools) = %d, want 22", len(tools))
	}
}

func TestServer_Run_ReadsStdin(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"
	reader := strings.NewReader(input)
	var buf bytes.Buffer

	server := &Server{out: &buf, tools: defaultTools()}
	server.run(reader)

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

// Phase 3: MCP Tool Schemas - RED tests

func TestServer_ListTools_HasInputSchemaProperties(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	err := server.handleRequest(req)
	if err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	var result map[string]interface{}
	json.Unmarshal(resp.Result, &result)

	tools := result["tools"].([]interface{})
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		inputSchema := toolMap["inputSchema"].(map[string]interface{})
		properties, ok := inputSchema["properties"]
		if !ok {
			t.Errorf("tool %s missing inputSchema.properties", toolMap["name"])
		}
		propsMap, ok := properties.(map[string]interface{})
		if !ok {
			t.Errorf("tool %s inputSchema.properties is not a map", toolMap["name"])
		}
		if len(propsMap) == 0 {
			t.Errorf("tool %s has empty inputSchema.properties", toolMap["name"])
		}
	}
}

func TestServer_ListTools_ScanToolSchema(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	err := server.handleRequest(req)
	if err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	var result map[string]interface{}
	json.Unmarshal(resp.Result, &result)

	tools := result["tools"].([]interface{})
	var scanTool map[string]interface{}
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		if toolMap["name"] == "scan" {
			scanTool = toolMap
			break
		}
	}

	if scanTool == nil {
		t.Fatal("scan tool not found in tools list")
	}

	inputSchema := scanTool["inputSchema"].(map[string]interface{})
	properties := inputSchema["properties"].(map[string]interface{})

	projectPath, ok := properties["project_path"]
	if !ok {
		t.Fatal("scan tool missing project_path property")
	}

	propMap := projectPath.(map[string]interface{})
	if propMap["type"] != "string" {
		t.Errorf("project_path type = %v, want string", propMap["type"])
	}

	required, ok := inputSchema["required"].([]interface{})
	if !ok {
		t.Fatal("scan tool missing required array")
	}

	found := false
	for _, r := range required {
		if r == "project_path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("scan tool required array does not include project_path")
	}
}

// --- Wiring-symmetry invariant tests ---
//
// These tests enforce the invariants documented in docs/mcp-tools.md:
//   - §1 directive 6 (every registered tool must appear in defaultTools() AND toolRegistry)
//   - §6, §7 (backup tools are intentionally NOT in defaultTools() — reverse asymmetry
//     that keeps them out of tools/list when WireMCPTools has not run)
//
// A failure here means somebody added a real handler without updating the stubs or
// schemas (or broke the documented intentional asymmetry). Fix it and update docs.

// TestServer_WiringSymmetryEveryDefaultToolHasSchema asserts that every tool
// exposed through defaultTools() has a matching JSON Schema entry in
// toolRegistry so clients receive a non-empty inputSchema on tools/list.
// Two independent assertions: (a) the schema exists, (b) its Properties map is
// non-empty. Adding a 5th tool without a property will fail (b); adding a 6th
// tool without a schema will fail (a).
func TestServer_WiringSymmetryEveryDefaultToolHasSchema(t *testing.T) {
	defaults := defaultTools()
	for name := range defaults {
		schema, ok := toolRegistry[name]
		if !ok {
			t.Errorf("tool %q is in defaultTools() but missing from toolRegistry — clients will receive empty inputSchema. Update internal/mcp/server.go::toolRegistry.", name)
			continue
		}
		if len(schema.Properties) == 0 {
			t.Errorf("tool %q has empty Properties in toolRegistry — every tool must declare at least one argument", name)
		}
	}
}

// backupToolPrefix identifies tools that are intentionally reverse-asymmetric:
// they have a real handler (WireMCPTools) and a schema (toolRegistry) but no
// stub (defaultTools). The test below treats any name with this prefix as
// exempt from the forward symmetry rule. See docs/mcp-tools.md §6 + §7.
const backupToolPrefix = "test_backup_"

// TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric is the
// pattern-based version of the previous hardcoded list: for every name in
// toolRegistry that is NOT in defaultTools(), the name MUST begin with
// backupToolPrefix. If a future tool is added to toolRegistry without a stub
// and without the backup_ prefix, this test catches it.
func TestServer_WiringSymmetryOnlyBackupToolsAreReverseAsymmetric(t *testing.T) {
	defaults := defaultTools()
	for name, schema := range toolRegistry {
		if _, inDefaults := defaults[name]; inDefaults {
			continue
		}
		if !strings.HasPrefix(name, backupToolPrefix) {
			t.Errorf("tool %q is in toolRegistry but NOT in defaultTools() and does not have the %q prefix. Either add it to defaultTools() (forward symmetry) or rename it so it has the backup prefix (documented reverse asymmetry). Update docs/mcp-tools.md §6 if a new exception is intentional.", name, backupToolPrefix)
			continue
		}
		// Reverse-asymmetric backup tools must still require project_path so
		// even their non-wired path is safe (empty handler would still get
		// validated by the schema).
		if len(schema.Required) == 0 {
			t.Errorf("backup tool %q has empty Required list in toolRegistry — must at least require project_path", name)
		}
	}
}

// TestServer_WiringSymmetryCleanupToolIsSymmetric guards the fix that
// re-introduced `cleanup` to defaultTools() and toolRegistry. Both must stay
// in sync; if either is dropped, this test fails.
func TestServer_WiringSymmetryCleanupToolIsSymmetric(t *testing.T) {
	defaults := defaultTools()
	if _, ok := defaults["cleanup"]; !ok {
		t.Error("cleanup is missing from defaultTools() — agents will see the stub return empty results in non-wired contexts")
	}
	schema, ok := toolRegistry["cleanup"]
	if !ok {
		t.Fatal("cleanup is missing from toolRegistry — clients will receive empty inputSchema")
	}
	if _, ok := schema.Properties["project_path"]; !ok {
		t.Error("cleanup schema missing project_path property")
	}
	if _, ok := schema.Properties["validate_passed"]; !ok {
		t.Error("cleanup schema missing validate_passed property")
	}
	requiredHas := func(name string) bool {
		for _, r := range schema.Required {
			if r == name {
				return true
			}
		}
		return false
	}
	if !requiredHas("project_path") {
		t.Error("cleanup schema missing project_path in Required array")
	}
	if !requiredHas("validate_passed") {
		t.Error("cleanup schema missing validate_passed in Required array")
	}
}

// runtimeBackupNames mirrors the 4 backup tool names registered by
// internal/app.WireMCPTools in production. The next test uses this list to
// simulate the production wiring without depending on the internal/app
// package (mcp_test.go must stay package-pure for the transport-layer tests).
var runtimeBackupNames = []string{
	"test_backup_create",
	"test_backup_list",
	"test_backup_restore",
	"test_backup_delete",
}

// TestServer_PostWireUpCountIs26 asserts that after the production-style
// registration of the 4 backup tools, tools/list reports 25 tools (the 21
// default stubs + 4 reverse-asymmetric backup tools). This locks the runtime
// count that docs/mcp-tools.md §1 advertises.
func TestServer_PostWireUpCountIs26(t *testing.T) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	// Dump handler: returns valid empty JSON so the registered tool is dispatchable.
	dummy := func(args json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]interface{}{})
	}
	for _, name := range runtimeBackupNames {
		server.RegisterTool(name, dummy)
	}

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("invalid result: %v", err)
	}
	tools, ok := result["tools"].([]interface{})
	if !ok {
		t.Fatal("missing tools array in result")
	}
	if len(tools) != 26 {
		t.Errorf("post-wire-up tool count = %d, want 26 (22 default + 4 backup)", len(tools))
	}
}

// Handlers that accept project_path must advertise it, otherwise the agent
// never sends it and the tool silently falls back to the process working
// directory, which is rarely the Drupal project.
func TestServer_ListTools_ProjectPathAwareToolsAdvertiseIt(t *testing.T) {
	for _, name := range []string{"create_patch", "composer_require", "scan"} {
		schema, ok := toolRegistry[name]
		if !ok {
			t.Fatalf("tool %q missing from registry", name)
		}
		if _, ok := schema.Properties["project_path"]; !ok {
			t.Errorf("tool %q does not declare project_path", name)
		}
	}
}
