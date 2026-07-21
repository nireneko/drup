package mcp

import (
	"encoding/json"
)

// defaultTools returns the 17 MCP tool handlers (7 existing + 10 new).
func defaultTools() map[string]ToolHandler {
	return map[string]ToolHandler{
		// Existing tools.
		"scan":          handleScan,
		"autofix":       handleAutofix,
		"contrib_check": handleContribCheck,
		"issue_patches": handleIssuePatches,
		"apply_patch":   handleApplyPatch,
		"validate":      handleValidate,
		"create_patch":  handleCreatePatch,
		// New tools.
		"detect_env":              handleDetectEnv,
		"composer_require":        handleComposerRequire,
		"drush_exec":              handleDrushExec,
		"contrib_upgrade_path":    handleContribUpgradePath,
		"upgrade_scan":            handleUpgradeScan,
		"patch_status":            handlePatchStatus,
		"patch_rollback":          handlePatchRollback,
		"generate_report":         handleGenerateReport,
		"module_info":             handleModuleInfo,
		"drupal_version_matrix":   handleDrupalVersionMatrix,
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

// --- New tool placeholders ---

func handleDetectEnv(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		ForceDetect bool   `json:"force_detect"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"environment":    "unknown",
		"command_prefix": []string{},
		"detected_at":    "",
	}
	return json.Marshal(result)
}

func handleComposerRequire(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Package     string `json:"package"`
		Dev         bool   `json:"dev"`
		NoUpdate    bool   `json:"no_update"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"success":          false,
		"installed_version": "",
		"stdout":           "",
		"stderr":           "not implemented",
		"exit_code":        -1,
	}
	return json.Marshal(result)
}

func handleDrushExec(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string   `json:"project_path"`
		Command     string   `json:"command"`
		Args        []string `json:"args"`
		Format      string   `json:"format"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"success":   false,
		"output":    "",
		"stderr":    "not implemented",
		"exit_code": -1,
	}
	return json.Marshal(result)
}

func handleContribUpgradePath(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module              string `json:"module_machine_name"`
		CurrentDrupalVer    string `json:"current_drupal_version"`
		TargetDrupalVer     string `json:"target_drupal_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"module":              params.Module,
		"recommended_upgrade": nil,
		"alternative_versions": []interface{}{},
	}
	return json.Marshal(result)
}

func handleUpgradeScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Scope       string `json:"scope"`
		Module      string `json:"module"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"total_errors":             0,
		"modules":                  []interface{}{},
		"upgrade_status_installed": false,
		"upgrade_status_enabled":   false,
	}
	return json.Marshal(result)
}

func handlePatchStatus(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"is_applied":            false,
		"commit_hash":           "",
		"registered_in_composer": false,
		"patch_info":            nil,
	}
	return json.Marshal(result)
}

func handlePatchRollback(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"success":              false,
		"reverted_commit":      "",
		"removed_from_composer": false,
		"error":                "not implemented",
	}
	return json.Marshal(result)
}

func handleGenerateReport(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath      string `json:"project_path"`
		ReportType       string `json:"report_type"`
		IncludeScanData  bool   `json:"include_scan_data"`
		IncludePatchList bool   `json:"include_patch_list"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"success":             false,
		"json_report_path":    "",
		"markdown_report_path": "",
		"summary":             map[string]interface{}{},
	}
	return json.Marshal(result)
}

func handleModuleInfo(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module             string `json:"module_machine_name"`
		IncludeMaintainers bool   `json:"include_maintainers"`
		IncludeDeps        bool   `json:"include_dependencies"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"module":      params.Module,
		"title":       "",
		"maintainers": []string{},
		"downloads":   0,
		"last_release": "",
		"open_issues": 0,
	}
	return json.Marshal(result)
}

func handleDrupalVersionMatrix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		DrupalVersion string `json:"drupal_version"`
		PHPVersion    string `json:"php_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result := map[string]interface{}{
		"drupal_version": "",
		"php_requirements": map[string]string{},
	}
	return json.Marshal(result)
}
