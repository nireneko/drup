package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"drup/internal/drupalorg"
	drupexec "drup/internal/exec"
	"drup/internal/mcp"
	"drup/internal/patch"
	"drup/internal/scan"
)

// WireMCPTools registers real tool handlers on the MCP server, replacing placeholders.
func WireMCPTools(s *mcp.Server) {
	s.RegisterTool("scan", realHandleScan)
	s.RegisterTool("autofix", realHandleAutofix)
	s.RegisterTool("contrib_check", realHandleContribCheck)
	s.RegisterTool("issue_patches", realHandleIssuePatches)
	s.RegisterTool("apply_patch", realHandleApplyPatch)
	s.RegisterTool("validate", realHandleValidate)
	s.RegisterTool("create_patch", realHandleCreatePatch)
}

func realHandleScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	stdout, stderr, exitCode, err := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")
	if err != nil {
		return nil, fmt.Errorf("exec drush: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("drush exit %d: %s", exitCode, stderr)
	}

	result, err := scan.Parse(strings.NewReader(stdout))
	if err != nil {
		return nil, fmt.Errorf("parse scan: %w", err)
	}
	result.ProjectPath = params.ProjectPath
	return json.Marshal(result)
}

func realHandleAutofix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	customModules := filepath.Join(params.ProjectPath, "modules", "custom")
	themes := filepath.Join(params.ProjectPath, "themes")

	targets := []string{}
	for _, dir := range []string{customModules, themes} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			targets = append(targets, dir)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no custom modules or themes found in %s", params.ProjectPath)
	}

	args2 := append([]string{"process"}, targets...)
	stdout, _, _, err := drupexec.Run(filepath.Join(params.ProjectPath, "vendor", "bin", "rector"), args2...)
	if err != nil {
		return nil, fmt.Errorf("exec rector: %w", err)
	}

	// Re-scan to get remaining errors.
	scanStdout, _, scanExit, _ := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")
	remaining := 0
	if scanExit == 0 {
		result, err := scan.Parse(strings.NewReader(scanStdout))
		if err == nil {
			remaining = result.TotalErrs
		}
	}

	response := map[string]interface{}{
		"rector_summary":   stdout,
		"remaining_errors": remaining,
	}
	return json.Marshal(response)
}

func realHandleContribCheck(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module string `json:"module_machine_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	info, err := drupalorg.CheckRelease(params.Module)
	if err != nil {
		return nil, err
	}
	return json.Marshal(info)
}

func realHandleIssuePatches(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		IssueNID string `json:"issue_nid,omitempty"`
		Module   string `json:"module_name,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	query := params.Module
	if params.IssueNID != "" {
		query = params.IssueNID
	}
	if query == "" {
		return nil, fmt.Errorf("module_name or issue_nid required")
	}

	patches, err := drupalorg.SearchPatches(query)
	if err != nil {
		return nil, err
	}
	return json.Marshal(patches)
}

func realHandleApplyPatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		PatchURL    string `json:"patch_url"`
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	result, err := patch.Apply(params.PatchURL, params.ProjectPath, "", "")
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleValidate(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Scope       string `json:"scope,omitempty"`
		Module      string `json:"module,omitempty"`
		File        string `json:"file,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	stdout, stderr, exitCode, err := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--format=json")
	if err != nil {
		return nil, fmt.Errorf("exec drush: %w", err)
	}
	if exitCode != 0 {
		return nil, fmt.Errorf("drush exit %d: %s", exitCode, stderr)
	}

	result, err := scan.Parse(strings.NewReader(stdout))
	if err != nil {
		return nil, fmt.Errorf("parse scan: %w", err)
	}

	// Filter by module or file if specified.
	var filtered []scan.DepError
	for _, mod := range result.Modules {
		if params.Module != "" && mod.Name != params.Module {
			continue
		}
		for _, e := range mod.Errors {
			if params.File != "" && !strings.Contains(e.File, params.File) {
				continue
			}
			filtered = append(filtered, e)
		}
	}

	response := map[string]interface{}{
		"total_errors": len(filtered),
		"errors":       filtered,
	}
	return json.Marshal(response)
}

func realHandleCreatePatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ModuleName         string `json:"module_name"`
		DeprecationDetails string `json:"deprecation_details"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Generate a patch using git diff after applying rector fixes to the specific module.
	// This is a simplified implementation — creates a diff of the module directory.
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	modulePath := filepath.Join(cwd, "modules", "contrib", params.ModuleName)
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("module %s not found at %s", params.ModuleName, modulePath)
	}

	// Run rector on the specific module.
	_, _, _, err = drupexec.Run(filepath.Join(cwd, "vendor", "bin", "rector"), "process", modulePath)
	if err != nil {
		return nil, fmt.Errorf("rector process: %w", err)
	}

	// Generate diff.
	stdout, _, exitCode, err := drupexec.Run("git", "-C", cwd, "diff", "--", modulePath)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	if exitCode != 0 || stdout == "" {
		response := map[string]interface{}{
			"patch_path": "",
			"applied":    false,
		}
		return json.Marshal(response)
	}

	// Write patch to temp file.
	patchFile, err := os.CreateTemp("", fmt.Sprintf("drup-%s-*.patch", params.ModuleName))
	if err != nil {
		return nil, err
	}
	if _, err := patchFile.WriteString(stdout); err != nil {
		patchFile.Close()
		os.Remove(patchFile.Name())
		return nil, err
	}
	patchFile.Close()

	response := map[string]interface{}{
		"patch_path": patchFile.Name(),
		"applied":    true,
	}
	return json.Marshal(response)
}
