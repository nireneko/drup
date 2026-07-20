package mcp

import (
	"encoding/json"
)

// defaultTools returns the 7 MCP tool handlers.
func defaultTools() map[string]ToolHandler {
	return map[string]ToolHandler{
		"scan":          handleScan,
		"autofix":       handleAutofix,
		"contrib_check": handleContribCheck,
		"issue_patches": handleIssuePatches,
		"apply_patch":   handleApplyPatch,
		"validate":      handleValidate,
		"create_patch":  handleCreatePatch,
	}
}

func handleScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would call scan.Parse in real implementation.
	result := map[string]interface{}{
		"project_path": params.ProjectPath,
		"total_errors": 0,
		"modules":      []interface{}{},
	}
	return json.Marshal(result)
}

func handleAutofix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"rector_summary":   "No changes needed",
		"remaining_errors": 0,
	}
	return json.Marshal(result)
}

func handleContribCheck(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module string `json:"module_machine_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would call drupalorg.CheckRelease.
	result := map[string]interface{}{
		"module":              params.Module,
		"has_d11_release":     false,
		"latest_version":      "",
		"compatible_branches": []string{},
	}
	return json.Marshal(result)
}

func handleIssuePatches(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		IssueNID string `json:"issue_nid,omitempty"`
		Module   string `json:"module_name,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would call drupalorg.SearchPatches.
	result := []interface{}{}
	return json.Marshal(result)
}

func handleApplyPatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		PatchURL    string `json:"patch_url"`
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would call patch.Apply.
	result := map[string]interface{}{
		"applied":     false,
		"commit_hash": "",
		"error":       "not implemented",
	}
	return json.Marshal(result)
}

func handleValidate(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Scope       string `json:"scope,omitempty"`
		Module      string `json:"module,omitempty"`
		File        string `json:"file,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would re-run scan.
	result := map[string]interface{}{
		"total_errors": 0,
		"errors":       []interface{}{},
	}
	return json.Marshal(result)
}

func handleCreatePatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ModuleName         string `json:"module_name"`
		DeprecationDetails string `json:"deprecation_details"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Placeholder — would generate a patch.
	result := map[string]interface{}{
		"patch_path": "",
		"applied":    false,
	}
	return json.Marshal(result)
}
