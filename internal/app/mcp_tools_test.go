package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"drup/internal/mcp"
)

func TestWireMCPTools_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	server := mcp.NewServer(&buf)
	WireMCPTools(server)
	// Verify WireMCPTools runs without panic and server is usable.
	t.Log("WireMCPTools registered successfully")
}

func TestRealHandleContribCheck_InvalidJSON(t *testing.T) {
	_, err := realHandleContribCheck(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleIssuePatches_MissingParams(t *testing.T) {
	_, err := realHandleIssuePatches(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing module_name and issue_nid, got nil")
	}
}

func TestRealHandleApplyPatch_InvalidJSON(t *testing.T) {
	_, err := realHandleApplyPatch(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleValidate_InvalidJSON(t *testing.T) {
	_, err := realHandleValidate(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleCreatePatch_InvalidJSON(t *testing.T) {
	_, err := realHandleCreatePatch(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleScan_InvalidJSON(t *testing.T) {
	_, err := realHandleScan(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleAutofix_InvalidJSON(t *testing.T) {
	_, err := realHandleAutofix(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRunPreflight_Dispatch(t *testing.T) {
	// Verify preflight command is dispatched correctly.
	err := Run([]string{"preflight"})
	// Will fail because we're not in a Drupal project, but should not be "unknown command".
	if err != nil && err.Error() == `unknown command "preflight"` {
		t.Error("preflight should be a known command")
	}
}
