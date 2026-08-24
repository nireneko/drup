package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/nireneko/drup/internal/metrics"
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

func TestServer_HandleRequest_ModuleReleaseInfoInvalidParamsReturnsEnvelopeFail(t *testing.T) {
	// Handler errors are now wrapped in {status:"fail"} envelopes, NOT
	// JSON-RPC errors. This test verifies the new behavior.
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      42,
		Method:  "tools/call",
		Params: json.RawMessage(`{
			"name": "module_release_info",
			"arguments": {"module_machine_name": "Bad-Name"}
		}`),
	}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	server.RegisterTool("module_release_info", func(args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("invalid module machine name: Bad-Name")
	})

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}

	// Should NOT be a JSON-RPC error.
	if resp.Error != nil {
		t.Fatalf("handler error should be envelope fail, not JSON-RPC error: %v", resp.Error)
	}

	// Should be an envelope with status:"fail".
	var envelope Envelope
	if err := json.Unmarshal(resp.Result, &envelope); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if envelope.Status != "fail" {
		t.Errorf("status = %q, want %q", envelope.Status, "fail")
	}
	if !strings.Contains(envelope.Summary, "invalid module machine name") {
		t.Errorf("summary = %q, want it to contain the error message", envelope.Summary)
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
	if len(tools) != 27 {
		t.Errorf("len(tools) = %d, want 27", len(tools))
	}
}

func TestServer_CallTool_DispatchesToRegisteredHandler(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	server.RegisterTool("probe", func(args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"ok":true}`), nil
	})

	result, err := server.CallTool("probe", json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if string(result) != `{"ok":true}` {
		t.Errorf("CallTool result = %s, want {\"ok\":true}", result)
	}
}

func TestServer_CallTool_UnknownToolReturnsError(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	if _, err := server.CallTool("does-not-exist", json.RawMessage(`{}`)); err == nil {
		t.Error("expected error calling an unregistered tool")
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

// TestToolRegistry_CoreUpgradeApplyHasNoAllowDirty guards G4/G5/S2
// (proposal decision 3): the MCP core_upgrade_apply schema must never
// expose allow_dirty — that flag stays CLI-only (drup upgrade-core
// --allow-dirty). The unguarded-session path forces dry_run: true instead
// of accepting any override.
func TestToolRegistry_CoreUpgradeApplyHasNoAllowDirty(t *testing.T) {
	schema, ok := toolRegistry["core_upgrade_apply"]
	if !ok {
		t.Fatal("core_upgrade_apply is missing from toolRegistry")
	}
	if _, exists := schema.Properties["allow_dirty"]; exists {
		t.Error("core_upgrade_apply schema exposes allow_dirty — it must be CLI-only, not part of the MCP surface")
	}
	if _, exists := schema.Properties["dry_run"]; !exists {
		t.Error("core_upgrade_apply schema is missing dry_run — the native override the guard forces instead of allow_dirty")
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

// TestServer_PostWireUpCountIs31 asserts that after the production-style
// registration of the 4 backup tools, tools/list reports 31 tools (the 27
// default stubs — including session_open added in PR4 and pipeline_status
// added in PR6 — + 4 reverse-asymmetric backup tools). This locks the
// runtime count that docs/mcp-tools.md §1 advertises.
func TestServer_PostWireUpCountIs31(t *testing.T) {
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
	if len(tools) != 31 {
		t.Errorf("post-wire-up tool count = %d, want 31 (27 default + 4 backup)", len(tools))
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

func TestServer_ToolCount(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	// defaultTools() returns 27 tools (25 original + session_open added in
	// PR4 + pipeline_status added in PR6).
	if got := server.ToolCount(); got != 27 {
		t.Errorf("default ToolCount() = %d, want 27", got)
	}

	// Register 2 more tools.
	dummy := func(args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}
	server.RegisterTool("extra_tool_1", dummy)
	server.RegisterTool("extra_tool_2", dummy)
	if got := server.ToolCount(); got != 29 {
		t.Errorf("after adding 2 tools, ToolCount() = %d, want 29", got)
	}
}

// --- REQ-2: Envelope wrapper tests ---

func TestHandleToolCall_EnvelopeWrap_Success(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	server.RegisterTool("test_tool", func(args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"foo":"bar"}`), nil
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test_tool","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	var envelope Envelope
	if err := json.Unmarshal(resp.Result, &envelope); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if envelope.Status != "pass" {
		t.Errorf("status = %q, want %q", envelope.Status, "pass")
	}
	if envelope.Summary == "" {
		t.Error("summary is empty")
	}
	// Verify payload is preserved.
	var payload map[string]interface{}
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatalf("invalid payload: %v", err)
	}
	if payload["foo"] != "bar" {
		t.Errorf("payload.foo = %v, want %q", payload["foo"], "bar")
	}

	var standardResult struct {
		Content []ContentBlock `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &standardResult); err != nil {
		t.Fatalf("invalid standard MCP result: %v", err)
	}
	if len(standardResult.Content) != 1 || standardResult.Content[0].Type != "text" {
		t.Fatalf("content = %#v, want one text block", standardResult.Content)
	}
	var contentEnvelope Envelope
	if err := json.Unmarshal([]byte(standardResult.Content[0].Text), &contentEnvelope); err != nil {
		t.Fatalf("invalid envelope in content: %v", err)
	}
	if contentEnvelope.Status != "pass" {
		t.Errorf("content status = %q, want %q", contentEnvelope.Status, "pass")
	}
}

func TestHandleToolCall_EnvelopeWrap_Error(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	server.RegisterTool("test_tool", func(args json.RawMessage) (json.RawMessage, error) {
		return nil, fmt.Errorf("test error message")
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"test_tool","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(buf.Bytes(), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	// Tool errors should NOT be JSON-RPC errors.
	if resp.Error != nil {
		t.Fatalf("tool error should not be a JSON-RPC error, got: %v", resp.Error)
	}

	var envelope Envelope
	if err := json.Unmarshal(resp.Result, &envelope); err != nil {
		t.Fatalf("invalid envelope: %v", err)
	}
	if envelope.Status != "fail" {
		t.Errorf("status = %q, want %q", envelope.Status, "fail")
	}
	if envelope.Summary != "test error message" {
		t.Errorf("summary = %q, want %q", envelope.Summary, "test error message")
	}
}

func TestHandleToolCall_PayloadIntact(t *testing.T) {
	complexPayload := `{"total_errors":3,"errors":{"contrib":[{"file":"a.module","line":1,"message":"dep"}]},"modules":[{"name":"ctools","errors":2}]}`
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	server.RegisterTool("scan", func(args json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(complexPayload), nil
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"scan","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)

	var envelope Envelope
	json.Unmarshal(resp.Result, &envelope)

	// Verify payload is byte-for-byte identical.
	if string(envelope.Payload) != complexPayload {
		t.Errorf("payload mismatch:\ngot:  %s\nwant: %s", string(envelope.Payload), complexPayload)
	}
}

func TestDeriveSummary_TotalErrors(t *testing.T) {
	payload := json.RawMessage(`{"total_errors":3}`)
	got := deriveSummary("scan", payload)
	if !strings.Contains(got, "3") {
		t.Errorf("deriveSummary with total_errors = %q, want it to contain '3'", got)
	}
}

func TestDeriveSummary_Success(t *testing.T) {
	payload := json.RawMessage(`{"success":true}`)
	got := deriveSummary("drush_exec", payload)
	if !strings.Contains(got, "succeeded") {
		t.Errorf("deriveSummary with success:true = %q, want it to contain 'succeeded'", got)
	}
}

func TestDeriveSummary_Fallback(t *testing.T) {
	payload := json.RawMessage(`{"custom":"data"}`)
	got := deriveSummary("unknown_tool", payload)
	want := "Tool unknown_tool completed"
	if got != want {
		t.Errorf("deriveSummary fallback = %q, want %q", got, want)
	}
}

func TestHandleToolCall_ProtocolErrors_StillJSONRPC(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	// Unknown tool → JSON-RPC error -32601.
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"nonexistent_tool","arguments":{}}`),
	}
	buf.Reset()
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}
	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error == nil {
		t.Fatal("expected JSON-RPC error for unknown tool, got nil")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("error code = %d, want -32601", resp.Error.Code)
	}

	// Malformed params → JSON-RPC error -32602.
	buf.Reset()
	req2 := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      2,
		Method:  "tools/call",
		Params:  json.RawMessage(`{invalid json`),
	}
	if err := server.handleRequest(req2); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}
	var resp2 JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp2)
	if resp2.Error == nil {
		t.Fatal("expected JSON-RPC error for malformed params, got nil")
	}
	if resp2.Error.Code != -32602 {
		t.Errorf("error code = %d, want -32602", resp2.Error.Code)
	}
}

// --- REQ-3: Selective retry tests ---

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{fmt.Errorf("context deadline exceeded"), true},
		{fmt.Errorf("connection refused"), true},
		{fmt.Errorf("i/o timeout"), true},
		{fmt.Errorf("broken pipe"), true},
		{fmt.Errorf("no such host"), true},
		{fmt.Errorf("command not found"), false},
		{fmt.Errorf("exit status 1"), false},
		{fmt.Errorf("no commands defined"), false},
		{nil, false},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.err), func(t *testing.T) {
			got := isTransientError(tt.err)
			if got != tt.want {
				t.Errorf("isTransientError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryLoop_TransientThenSuccess(t *testing.T) {
	// Override retryBaseDelay to speed up the test.
	origDelay := retryBaseDelay
	retryBaseDelay = 1 * time.Millisecond
	defer func() { retryBaseDelay = origDelay }()

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	beforeRetries := metrics.Default().Snapshot().Retries

	callCount := 0
	server.RegisterTool("flaky_tool", func(args json.RawMessage) (json.RawMessage, error) {
		callCount++
		if callCount < 3 {
			return nil, fmt.Errorf("context deadline exceeded")
		}
		return json.RawMessage(`{"result":"ok"}`), nil
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"flaky_tool","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	var envelope Envelope
	json.Unmarshal(resp.Result, &envelope)
	if envelope.Status != "pass" {
		t.Errorf("status = %q, want %q", envelope.Status, "pass")
	}
	if callCount != 3 {
		t.Errorf("handler called %d times, want 3", callCount)
	}
	if got := metrics.Default().Snapshot().Retries - beforeRetries; got != 2 {
		t.Errorf("retries recorded = %d, want 2", got)
	}
}

func TestRetryLoop_NoRetryOnRealError(t *testing.T) {
	origDelay := retryBaseDelay
	retryBaseDelay = 1 * time.Millisecond
	defer func() { retryBaseDelay = origDelay }()

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	callCount := 0
	server.RegisterTool("broken_tool", func(args json.RawMessage) (json.RawMessage, error) {
		callCount++
		return nil, fmt.Errorf("command not found")
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"broken_tool","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	var envelope Envelope
	json.Unmarshal(resp.Result, &envelope)
	if envelope.Status != "fail" {
		t.Errorf("status = %q, want %q", envelope.Status, "fail")
	}
	if callCount != 1 {
		t.Errorf("handler called %d times, want 1 (no retry on real error)", callCount)
	}
}

func TestRetryLoop_Exhausted(t *testing.T) {
	origDelay := retryBaseDelay
	retryBaseDelay = 1 * time.Millisecond
	defer func() { retryBaseDelay = origDelay }()

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	callCount := 0
	server.RegisterTool("always_timeout", func(args json.RawMessage) (json.RawMessage, error) {
		callCount++
		return nil, fmt.Errorf("i/o timeout")
	})

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"always_timeout","arguments":{}}`),
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}

	var resp JSONRPCResponse
	json.Unmarshal(buf.Bytes(), &resp)
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", resp.Error)
	}

	var envelope Envelope
	json.Unmarshal(resp.Result, &envelope)
	if envelope.Status != "fail" {
		t.Errorf("status = %q, want %q", envelope.Status, "fail")
	}
	if !strings.Contains(envelope.Summary, "after 3 attempts") {
		t.Errorf("summary = %q, want it to contain 'after 3 attempts'", envelope.Summary)
	}
	if callCount != 3 {
		t.Errorf("handler called %d times, want 3", callCount)
	}
}

// --- PR2: transport hardening (D1 scanner buffer, M3 sorted tools/list,
// M6 JSON-RPC notification handling) ---

// decodeResponses splits newline-delimited JSON-RPC response lines and
// decodes each one, skipping blank lines.
func decodeResponses(t *testing.T, data []byte) []JSONRPCResponse {
	t.Helper()
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil
	}
	var out []JSONRPCResponse
	for _, line := range strings.Split(trimmed, "\n") {
		if line == "" {
			continue
		}
		var resp JSONRPCResponse
		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			t.Fatalf("invalid response line %q: %v", line, err)
		}
		out = append(out, resp)
	}
	return out
}

// toolNamesFromResult extracts the ordered tool name list from a tools/list
// result payload, preserving JSON array order.
func toolNamesFromResult(t *testing.T, result json.RawMessage) []string {
	t.Helper()
	var parsed struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid tools/list result: %v", err)
	}
	names := make([]string, len(parsed.Tools))
	for i, tool := range parsed.Tools {
		names[i] = tool.Name
	}
	return names
}

// TestServer_ListTools_RepeatedCallsMatchAndSorted guards M3: tools/list must
// list tools in a deterministic, name-sorted order across repeated calls,
// not the randomized order Go map iteration would otherwise produce.
func TestServer_ListTools_RepeatedCallsMatchAndSorted(t *testing.T) {
	var buf bytes.Buffer
	server := NewServer(&buf, "test")
	req := JSONRPCRequest{JSONRPC: "2.0", ID: 1, Method: "tools/list"}

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error (call 1): %v", err)
	}
	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error (call 2): %v", err)
	}

	responses := decodeResponses(t, buf.Bytes())
	if len(responses) != 2 {
		t.Fatalf("got %d responses, want 2", len(responses))
	}

	names1 := toolNamesFromResult(t, responses[0].Result)
	names2 := toolNamesFromResult(t, responses[1].Result)
	if !reflect.DeepEqual(names1, names2) {
		t.Fatalf("tool order changed between two tools/list calls in the same session:\n1: %v\n2: %v", names1, names2)
	}

	sorted := append([]string(nil), names1...)
	sort.Strings(sorted)
	if !reflect.DeepEqual(names1, sorted) {
		t.Errorf("tools/list order = %v, want sorted by name = %v", names1, sorted)
	}
}

// TestServer_HandleRequest_NotificationProducesNoResponse guards M6: a
// JSON-RPC request with no "id" field is a notification. The server must not
// write any response for it, not even an error one.
func TestServer_HandleRequest_NotificationProducesNoResponse(t *testing.T) {
	req := JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/initialized"}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no response written for a notification, got: %q", buf.String())
	}
}

// TestServer_HandleRequest_NotificationForUnknownMethodStillNoResponse
// ensures the notification short-circuit applies before method dispatch, so
// even an unrecognized notification method never triggers the -32601 error
// response that would otherwise fire for a request with an id.
func TestServer_HandleRequest_NotificationForUnknownMethodStillNoResponse(t *testing.T) {
	req := JSONRPCRequest{JSONRPC: "2.0", Method: "notifications/some_unknown_event"}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no response written for an unknown notification, got: %q", buf.String())
	}
}

// TestServer_HandleRequest_OrdinaryRequestStillGetsResponse guards the other
// half of M6: a request that DOES carry a non-null id must still receive
// exactly one response.
func TestServer_HandleRequest_OrdinaryRequestStillGetsResponse(t *testing.T) {
	req := JSONRPCRequest{JSONRPC: "2.0", ID: float64(7), Method: "tools/list"}

	var buf bytes.Buffer
	server := NewServer(&buf, "test")

	if err := server.handleRequest(req); err != nil {
		t.Fatalf("handleRequest error: %v", err)
	}
	responses := decodeResponses(t, buf.Bytes())
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want exactly 1 for a request with a non-null id", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("unexpected error: %v", responses[0].Error)
	}
	if fmt.Sprint(responses[0].ID) != "7" {
		t.Errorf("response id = %v, want 7", responses[0].ID)
	}
}

// TestServer_Run_NotificationInStreamGetsNoResponseButNextRequestDoes drives
// the notification behavior through the stdin read loop (server.run), not
// just handleRequest directly, mirroring how a real client interleaves
// notifications/initialized with ordinary requests over stdio.
func TestServer_Run_NotificationInStreamGetsNoResponseButNextRequestDoes(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}` + "\n" +
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}` + "\n"

	var buf bytes.Buffer
	server := &Server{out: &buf, tools: defaultTools()}
	if err := server.run(strings.NewReader(input)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	responses := decodeResponses(t, buf.Bytes())
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want exactly 1 (the notification must not produce one)", len(responses))
	}
	if fmt.Sprint(responses[0].ID) != "1" {
		t.Errorf("response id = %v, want 1", responses[0].ID)
	}
}

// TestServer_Run_LineOver64KBWithinBoundParsesNormally guards D1's "within
// bound" scenario: a request larger than the default 64KB bufio.Scanner
// buffer, but within the server's configured maximum, must still parse.
func TestServer_Run_LineOver64KBWithinBoundParsesNormally(t *testing.T) {
	bigValue := strings.Repeat("x", 100*1024) // 100KB — over the 64KB default.
	req := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"scan","arguments":{"project_path":"` + bigValue + `"}}}` + "\n"

	var buf bytes.Buffer
	server := &Server{out: &buf, tools: defaultTools()}
	if err := server.run(strings.NewReader(req)); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	responses := decodeResponses(t, buf.Bytes())
	if len(responses) != 1 {
		t.Fatalf("got %d responses, want 1", len(responses))
	}
	if responses[0].Error != nil {
		t.Errorf("unexpected error for a large-but-within-bound line: %v", responses[0].Error)
	}
}

// TestServer_Run_OversizedLineDoesNotKillServer guards D1's core fix: today
// an oversized line makes bufio.Scanner return bufio.ErrTooLong, server.run
// propagates that as an error, and the whole stdio loop dies — every
// subsequent request in the same process is lost. After the fix, an
// oversized line must produce a parse error for that one request and the
// server must keep serving the next request in the stream.
func TestServer_Run_OversizedLineDoesNotKillServer(t *testing.T) {
	giant := strings.Repeat("a", 11*1024*1024) // over the 10MB configured max.
	oversized := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"scan","arguments":{"project_path":"` + giant + `"}}}`
	next := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	input := oversized + "\n" + next + "\n"

	var buf bytes.Buffer
	server := &Server{out: &buf, tools: defaultTools()}
	if err := server.run(strings.NewReader(input)); err != nil {
		t.Fatalf("run returned error — the server must survive an oversized line: %v", err)
	}

	responses := decodeResponses(t, buf.Bytes())
	if len(responses) == 0 {
		t.Fatal("expected at least one response, got none")
	}

	last := responses[len(responses)-1]
	if last.Error != nil {
		t.Fatalf("expected the trailing tools/list request to succeed after the oversized line, got error: %v", last.Error)
	}
	names := toolNamesFromResult(t, last.Result)
	if len(names) == 0 {
		t.Fatal("expected tools/list to return a non-empty tool list after recovering from the oversized line")
	}
}
