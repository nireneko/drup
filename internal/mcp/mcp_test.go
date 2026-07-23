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
	if len(tools) != 20 {
		t.Errorf("len(tools) = %d, want 20", len(tools))
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

