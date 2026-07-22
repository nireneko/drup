package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/coreupgrade"
	"github.com/nireneko/drup/internal/drupalorg"
	"github.com/nireneko/drup/internal/envdetect"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/patch"
	"github.com/nireneko/drup/internal/patchreconcile"
	"github.com/nireneko/drup/internal/report"
	"github.com/nireneko/drup/internal/scan"
)

// defaultEnvDetector is the shared environment detector.
var defaultEnvDetector envdetect.Detector = envdetect.NewDetector()

// drushBlocklist contains commands that must not be executed via drush_exec.
var drushBlocklist = map[string]bool{
	"sql-drop":         true,
	"site-install":     true,
	"site:install":     true,
	"sql-sanitize":     true,
	"php-eval":         true,
	"core:execute-cli": true,
}

// shellMetacharPattern matches shell injection characters.
var shellMetacharPattern = regexp.MustCompile("[;|&$`]")

// composerPackagePattern validates composer package names.
var composerPackagePattern = regexp.MustCompile(`^[a-z0-9]([_.\-]?[a-z0-9]+)*/[a-z0-9]([_.\-]?[a-z0-9]+)*(:[a-zA-Z0-9^~<>=*. -]+)?$`)

// moduleNamePattern validates Drupal module machine names.
var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// WireMCPTools registers real tool handlers on the MCP server, replacing placeholders.
func WireMCPTools(s *mcp.Server) {
	s.RegisterTool("scan", realHandleScan)
	s.RegisterTool("autofix", realHandleAutofix)
	s.RegisterTool("contrib_check", realHandleContribCheck)
	s.RegisterTool("issue_patches", realHandleIssuePatches)
	s.RegisterTool("apply_patch", realHandleApplyPatch)
	s.RegisterTool("validate", realHandleValidate)
	s.RegisterTool("create_patch", realHandleCreatePatch)
	// New tools.
	s.RegisterTool("detect_env", realHandleDetectEnv)
	s.RegisterTool("composer_require", realHandleComposerRequire)
	s.RegisterTool("drush_exec", realHandleDrushExec)
	s.RegisterTool("contrib_upgrade_path", realHandleContribUpgradePath)
	s.RegisterTool("upgrade_scan", realHandleUpgradeScan)
	s.RegisterTool("patch_status", realHandlePatchStatus)
	s.RegisterTool("patch_rollback", realHandlePatchRollback)
	s.RegisterTool("generate_report", realHandleGenerateReport)
	s.RegisterTool("module_info", realHandleModuleInfo)
	s.RegisterTool("drupal_version_matrix", realHandleDrupalVersionMatrix)
	s.RegisterTool("core_upgrade_check", realHandleCoreUpgradeCheck)
	s.RegisterTool("core_upgrade_apply", realHandleCoreUpgradeApply)
	s.RegisterTool("patch_reconcile", realHandlePatchReconcile)
}

func realHandleScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	stdout, stderr, exitCode, err := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--all", "--format=json")
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
	scanStdout, _, scanExit, _ := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", "--all", "--format=json")
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

	// Use module name when specified, otherwise --all for full project analysis.
	analyzeTarget := "--all"
	if params.Module != "" {
		analyzeTarget = params.Module
	}

	stdout, stderr, exitCode, err := drupexec.Run("drush", "-r", params.ProjectPath, "upgrade_status:analyze", analyzeTarget, "--format=json")
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

// --- New tool handlers ---

func realHandleDetectEnv(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		ForceDetect bool   `json:"force_detect"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	detection, err := defaultEnvDetector.Detect(params.ProjectPath, params.ForceDetect)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"environment":    string(detection.Environment),
		"command_prefix": detection.CommandPrefix,
		"detected_at":    detection.DetectedAt.Format(time.RFC3339),
	}
	return json.Marshal(result)
}

func realHandleComposerRequire(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Package     string `json:"package"`
		Dev         bool   `json:"dev"`
		NoUpdate    bool   `json:"no_update"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.Package == "" {
		return nil, fmt.Errorf("project_path and package are required")
	}

	// Validate package format.
	pkgName := params.Package
	if idx := strings.Index(pkgName, ":"); idx >= 0 {
		pkgName = pkgName[:idx]
	}
	if !composerPackagePattern.MatchString(params.Package) {
		result := map[string]interface{}{
			"success":           false,
			"installed_version": "",
			"stdout":            "",
			"stderr":            fmt.Sprintf("invalid package name format: %s", params.Package),
			"exit_code":         -1,
		}
		return json.Marshal(result)
	}

	// Check composer.json exists.
	composerJSON := filepath.Join(params.ProjectPath, "composer.json")
	if _, err := os.Stat(composerJSON); os.IsNotExist(err) {
		return nil, fmt.Errorf("composer.json not found at %s", params.ProjectPath)
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Dry-run first.
	dryArgs := []string{"require", "--dry-run", params.Package}
	if params.Dev {
		dryArgs = append(dryArgs, "--dev")
	}
	if params.NoUpdate {
		dryArgs = append(dryArgs, "--no-update")
	}
	_, dryStderr, dryExit, err := drupexec.RunWithEnv(detection.CommandPrefix, "composer", dryArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec composer dry-run: %w", err)
	}
	if dryExit != 0 {
		result := map[string]interface{}{
			"success":           false,
			"installed_version": "",
			"stdout":            "",
			"stderr":            dryStderr,
			"exit_code":         dryExit,
		}
		return json.Marshal(result)
	}

	// Actual require.
	realArgs := []string{"require", params.Package}
	if params.Dev {
		realArgs = append(realArgs, "--dev")
	}
	if params.NoUpdate {
		realArgs = append(realArgs, "--no-update")
	}
	stdout, stderr, exitCode, err := drupexec.RunWithEnv(detection.CommandPrefix, "composer", realArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec composer require: %w", err)
	}

	// Parse installed version from output.
	installedVersion := parseInstalledVersion(stdout, pkgName)

	result := map[string]interface{}{
		"success":           exitCode == 0,
		"installed_version": installedVersion,
		"stdout":            stdout,
		"stderr":            stderr,
		"exit_code":         exitCode,
	}
	return json.Marshal(result)
}

func parseInstalledVersion(output, pkgName string) string {
	// Look for "Installing vendor/package (version)" or "Upgrading vendor/package (version)".
	for _, prefix := range []string{"Installing ", "Upgrading "} {
		idx := strings.Index(output, prefix+pkgName+" (")
		if idx >= 0 {
			start := idx + len(prefix) + len(pkgName) + 2
			end := strings.Index(output[start:], ")")
			if end >= 0 {
				return output[start : start+end]
			}
		}
	}
	return ""
}

func realHandleDrushExec(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string   `json:"project_path"`
		Command     string   `json:"command"`
		Args        []string `json:"args"`
		Format      string   `json:"format"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.Command == "" {
		return nil, fmt.Errorf("project_path and command are required")
	}

	// Check blocklist.
	if drushBlocklist[params.Command] {
		result := map[string]interface{}{
			"success":   false,
			"output":    "",
			"stderr":    fmt.Sprintf("command '%s' is blocked for safety", params.Command),
			"exit_code": -1,
		}
		return json.Marshal(result)
	}

	// Check shell metacharacters in command.
	if shellMetacharPattern.MatchString(params.Command) {
		result := map[string]interface{}{
			"success":   false,
			"output":    "",
			"stderr":    "command contains shell metacharacters",
			"exit_code": -1,
		}
		return json.Marshal(result)
	}

	// Check shell metacharacters in args.
	for _, arg := range params.Args {
		if shellMetacharPattern.MatchString(arg) {
			result := map[string]interface{}{
				"success":   false,
				"output":    "",
				"stderr":    fmt.Sprintf("argument '%s' contains shell metacharacters", arg),
				"exit_code": -1,
			}
			return json.Marshal(result)
		}
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Build command args.
	cmdArgs := []string{params.Command}
	cmdArgs = append(cmdArgs, params.Args...)
	cmdArgs = append(cmdArgs, "--root="+params.ProjectPath)
	if params.Format != "" {
		cmdArgs = append(cmdArgs, "--format="+params.Format)
	}

	stdout, stderr, exitCode, err := drupexec.RunWithEnv(detection.CommandPrefix, "drush", cmdArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec drush: %w", err)
	}

	// Parse JSON output if format is json.
	var output interface{} = stdout
	if params.Format == "json" {
		var parsed interface{}
		if jsonErr := json.Unmarshal([]byte(stdout), &parsed); jsonErr == nil {
			output = parsed
		} else {
			stderr = stderr + "warning: failed to parse JSON output"
		}
	}

	result := map[string]interface{}{
		"success":   exitCode == 0,
		"output":    output,
		"stderr":    stderr,
		"exit_code": exitCode,
	}
	return json.Marshal(result)
}

func realHandleContribUpgradePath(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module           string `json:"module_machine_name"`
		CurrentDrupalVer string `json:"current_drupal_version"`
		TargetDrupalVer  string `json:"target_drupal_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}
	if params.CurrentDrupalVer == "" || params.TargetDrupalVer == "" {
		return nil, fmt.Errorf("current_drupal_version and target_drupal_version are required")
	}

	rec, err := drupalorg.UpgradePath(params.Module, params.CurrentDrupalVer, params.TargetDrupalVer)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rec)
}

func realHandleUpgradeScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Scope       string `json:"scope"`
		Module      string `json:"module"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	// Check for path traversal.
	if strings.Contains(params.ProjectPath, "..") {
		return nil, fmt.Errorf("project_path must not contain '..' segments")
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Check if upgrade_status is in composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	var composerJSON map[string]interface{}
	if err := json.Unmarshal(composerData, &composerJSON); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	upgradeStatusInstalled := hasPackage(composerJSON, "drupal/upgrade_status")

	// Install if needed.
	if !upgradeStatusInstalled {
		reqArgs := json.RawMessage(fmt.Sprintf(`{"project_path":%q,"package":"drupal/upgrade_status","dev":true}`, params.ProjectPath))
		_, reqErr := realHandleComposerRequire(reqArgs)
		if reqErr != nil {
			return nil, fmt.Errorf("install upgrade_status: %w", reqErr)
		}
	}

	// Check if upgrade_status is enabled.
	upgradeStatusEnabled := false
	pmListArgs := []string{"pm:list", "--status=enabled", "--format=json"}
	pmStdout, _, pmExit, _ := drupexec.RunWithEnv(detection.CommandPrefix, "drush", append(pmListArgs, "--root="+params.ProjectPath)...)
	if pmExit == 0 {
		var pmData map[string]interface{}
		if json.Unmarshal([]byte(pmStdout), &pmData) == nil {
			if _, ok := pmData["upgrade_status"]; ok {
				upgradeStatusEnabled = true
			}
		}
	}

	// Enable if needed.
	if !upgradeStatusEnabled {
		// Delete conflicting update.settings config before enabling.
		cdArgs := []string{"config:delete", "update.settings", "--root=" + params.ProjectPath}
		_, _, _, _ = drupexec.RunWithEnv(detection.CommandPrefix, "drush", cdArgs...)

		enArgs := []string{"en", "upgrade_status", "-y", "--root=" + params.ProjectPath}
		_, enStderr, enExit, enErr := drupexec.RunWithEnv(detection.CommandPrefix, "drush", enArgs...)
		if enErr != nil {
			return nil, fmt.Errorf("enable upgrade_status: %w", enErr)
		}
		if enExit != 0 {
			return nil, fmt.Errorf("enable upgrade_status failed (exit %d): %s", enExit, enStderr)
		}
		upgradeStatusEnabled = true
	}

	// Run analysis.
	analyzeTarget := "all"
	if params.Module != "" {
		analyzeTarget = params.Module
	}
	analyzeArgs := []string{"upgrade_status:analyze", analyzeTarget, "--format=json", "--root=" + params.ProjectPath}
	analyzeStdout, analyzeStderr, analyzeExit, analyzeErr := drupexec.RunWithEnv(detection.CommandPrefix, "drush", analyzeArgs...)
	if analyzeErr != nil {
		return nil, fmt.Errorf("exec upgrade_status:analyze: %w", analyzeErr)
	}

	// Parse results.
	scanResult, parseErr := scan.Parse(strings.NewReader(analyzeStdout))

	// Filter by scope if specified.
	var modules []interface{}
	totalErrors := 0
	if parseErr == nil && scanResult != nil {
		for _, mod := range scanResult.Modules {
			if params.Scope != "" && string(mod.Type) != params.Scope {
				continue
			}
			modules = append(modules, map[string]interface{}{
				"name":     mod.Name,
				"category": string(mod.Type),
				"errors":   len(mod.Errors),
			})
			totalErrors += len(mod.Errors)
		}
	}

	if modules == nil {
		modules = []interface{}{}
	}

	// Handle partial results.
	if analyzeExit != 0 && parseErr != nil {
		result := map[string]interface{}{
			"total_errors":             0,
			"modules":                  modules,
			"upgrade_status_installed": true,
			"upgrade_status_enabled":   upgradeStatusEnabled,
			"warning":                  "partial results: " + analyzeStderr,
		}
		return json.Marshal(result)
	}

	result := map[string]interface{}{
		"total_errors":             totalErrors,
		"modules":                  modules,
		"upgrade_status_installed": true,
		"upgrade_status_enabled":   upgradeStatusEnabled,
	}
	return json.Marshal(result)
}

func hasPackage(composerJSON map[string]interface{}, pkg string) bool {
	for _, section := range []string{"require", "require-dev"} {
		if deps, ok := composerJSON[section].(map[string]interface{}); ok {
			if _, exists := deps[pkg]; exists {
				return true
			}
		}
	}
	return false
}

func realHandlePatchStatus(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	if params.PatchURL == "" && params.ComposerPackage == "" {
		return nil, fmt.Errorf("patch_url or composer_package is required")
	}

	// Read composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	var composerJSON struct {
		Extra struct {
			Patches map[string][]patchEntry `json:"patches"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(composerData, &composerJSON); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	type patchInfoResult struct {
		URL         string `json:"url"`
		Package     string `json:"package"`
		Description string `json:"description"`
	}

	registeredInComposer := false
	var foundPatchInfo *patchInfoResult

	// Search patches.
	for pkg, entries := range composerJSON.Extra.Patches {
		if params.ComposerPackage != "" && pkg != params.ComposerPackage {
			continue
		}
		for _, entry := range entries {
			match := false
			if params.PatchURL != "" {
				match = entry.URL == params.PatchURL || strings.Contains(entry.URL, params.PatchURL) || strings.Contains(params.PatchURL, entry.URL)
			} else {
				match = true // Just matching by package.
			}
			if match {
				registeredInComposer = true
				foundPatchInfo = &patchInfoResult{
					URL:         entry.URL,
					Package:     pkg,
					Description: entry.Description,
				}
				break
			}
		}
		if foundPatchInfo != nil {
			break
		}
	}

	// Check git log for patch commit.
	commitHash := ""
	searchTerm := params.PatchURL
	if searchTerm == "" && foundPatchInfo != nil {
		searchTerm = foundPatchInfo.URL
	}
	if searchTerm != "" {
		stdout, _, gitExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "--grep="+searchTerm)
		if gitExit == 0 && stdout != "" {
			lines := strings.Split(strings.TrimSpace(stdout), "\n")
			if len(lines) > 0 {
				parts := strings.Fields(lines[0])
				if len(parts) > 0 {
					commitHash = parts[0]
				}
			}
		}
	}

	// Determine is_applied.
	isApplied := registeredInComposer // If registered, assume applied.
	if registeredInComposer && commitHash != "" {
		// Check if there's a revert commit.
		revertStdout, _, revertExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "--grep=Revert")
		if revertExit == 0 && strings.Contains(revertStdout, commitHash) {
			isApplied = false
		}
	}

	result := map[string]interface{}{
		"is_applied":             isApplied,
		"commit_hash":            commitHash,
		"registered_in_composer": registeredInComposer,
		"patch_info":             foundPatchInfo,
	}
	return json.Marshal(result)
}

type patchEntry struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

func realHandlePatchRollback(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.PatchURL == "" || params.ComposerPackage == "" {
		return nil, fmt.Errorf("project_path, patch_url, and composer_package are required")
	}

	// Verify it's a git repo.
	_, _, gitExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "rev-parse", "--git-dir")
	if gitExit != 0 {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "not a git repository",
		}
		return json.Marshal(result)
	}

	// Check working tree clean.
	statusStdout, _, _, _ := drupexec.Run("git", "-C", params.ProjectPath, "status", "--porcelain")
	if strings.TrimSpace(statusStdout) != "" {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "working tree is dirty; commit or stash changes first",
		}
		return json.Marshal(result)
	}

	// Get patch status to find commit hash.
	statusArgs := json.RawMessage(fmt.Sprintf(`{"project_path":%q,"patch_url":%q,"composer_package":%q}`,
		params.ProjectPath, params.PatchURL, params.ComposerPackage))
	statusResult, err := realHandlePatchStatus(statusArgs)
	if err != nil {
		return nil, err
	}

	var status struct {
		IsApplied  bool   `json:"is_applied"`
		CommitHash string `json:"commit_hash"`
	}
	if err := json.Unmarshal(statusResult, &status); err != nil {
		return nil, err
	}

	if !status.IsApplied {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "patch is not applied",
		}
		return json.Marshal(result)
	}

	if status.CommitHash == "" {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "cannot find patch commit to revert",
		}
		return json.Marshal(result)
	}

	// Step 1: git revert (atomic — must succeed before modifying composer.json).
	revertStdout, revertStderr, revertExit, revertErr := drupexec.Run("git", "-C", params.ProjectPath, "revert", status.CommitHash, "--no-edit")
	if revertErr != nil {
		return nil, fmt.Errorf("git revert: %w", revertErr)
	}
	if revertExit != 0 {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 fmt.Sprintf("revert conflict: %s", revertStderr),
		}
		return json.Marshal(result)
	}

	// Get new revert commit hash.
	revertedCommit := ""
	if revertStdout != "" {
		parts := strings.Fields(strings.TrimSpace(revertStdout))
		if len(parts) >= 2 {
			revertedCommit = strings.TrimPrefix(parts[1], "]")
			// Try to get the actual hash from git log.
			logStdout, _, _, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "-1")
			if logStdout != "" {
				logParts := strings.Fields(strings.TrimSpace(logStdout))
				if len(logParts) > 0 {
					revertedCommit = logParts[0]
				}
			}
		}
	}

	// Step 2: Remove from composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		result := map[string]interface{}{
			"success":               true,
			"reverted_commit":       revertedCommit,
			"removed_from_composer": false,
			"error":                 "warning: could not read composer.json",
		}
		return json.Marshal(result)
	}

	var composerMap map[string]json.RawMessage
	if err := json.Unmarshal(composerData, &composerMap); err != nil {
		result := map[string]interface{}{
			"success":               true,
			"reverted_commit":       revertedCommit,
			"removed_from_composer": false,
			"error":                 "warning: could not parse composer.json",
		}
		return json.Marshal(result)
	}

	// Remove patch entry from extra.patches.
	var extra map[string]json.RawMessage
	if raw, ok := composerMap["extra"]; ok {
		json.Unmarshal(raw, &extra)
	}
	if extra != nil {
		var patches map[string]json.RawMessage
		if raw, ok := extra["patches"]; ok {
			json.Unmarshal(raw, &patches)
		}
		if patches != nil {
			if raw, ok := patches[params.ComposerPackage]; ok {
				var entries []patchEntry
				json.Unmarshal(raw, &entries)
				// Filter out the matching patch.
				var remaining []patchEntry
				for _, e := range entries {
					if e.URL != params.PatchURL {
						remaining = append(remaining, e)
					}
				}
				if len(remaining) == 0 {
					delete(patches, params.ComposerPackage)
				} else {
					newEntries, _ := json.Marshal(remaining)
					patches[params.ComposerPackage] = newEntries
				}
			}
			newPatches, _ := json.Marshal(patches)
			extra["patches"] = newPatches
		}
		newExtra, _ := json.Marshal(extra)
		composerMap["extra"] = newExtra
	}

	updatedComposer, _ := json.MarshalIndent(composerMap, "", "    ")
	os.WriteFile(composerPath, updatedComposer, 0o644)

	// Step 3: composer update for the package.
	detection, _ := defaultEnvDetector.Detect(params.ProjectPath, false)
	_, compStderr, compExit, _ := drupexec.RunWithEnv(detection.CommandPrefix, "composer", "update", params.ComposerPackage)

	result := map[string]interface{}{
		"success":               true,
		"reverted_commit":       revertedCommit,
		"removed_from_composer": true,
	}
	if compExit != 0 {
		result["error"] = fmt.Sprintf("warning: composer update failed: %s", compStderr)
	}
	return json.Marshal(result)
}

func realHandleGenerateReport(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath      string `json:"project_path"`
		ReportType       string `json:"report_type"`
		IncludeScanData  bool   `json:"include_scan_data"`
		IncludePatchList bool   `json:"include_patch_list"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	reportType := params.ReportType
	if reportType == "" {
		reportType = "both"
	}

	// Collect report data.
	data := &report.ReportData{
		ProjectPath: params.ProjectPath,
	}

	// Count patches from composer.json.
	if params.IncludePatchList {
		composerPath := filepath.Join(params.ProjectPath, "composer.json")
		if composerData, err := os.ReadFile(composerPath); err == nil {
			var composerJSON struct {
				Extra struct {
					Patches map[string][]patchEntry `json:"patches"`
				} `json:"extra"`
			}
			if json.Unmarshal(composerData, &composerJSON) == nil {
				for _, entries := range composerJSON.Extra.Patches {
					for range entries {
						data.TokenAccounting.Total++
					}
				}
			}
		}
	}

	result := map[string]interface{}{
		"success":              true,
		"json_report_path":     "",
		"markdown_report_path": "",
		"summary": map[string]interface{}{
			"total_modules_checked": 0,
			"patches_applied":       data.TokenAccounting.Total,
			"errors_remaining":      data.TotalErrors,
		},
	}

	if reportType == "json" || reportType == "both" {
		jsonData, err := report.GenerateJSON(data)
		if err != nil {
			return nil, fmt.Errorf("generate JSON report: %w", err)
		}
		jsonPath := filepath.Join(params.ProjectPath, "drup-report.json")
		if err := os.WriteFile(jsonPath, jsonData, 0o644); err != nil {
			return nil, fmt.Errorf("write JSON report: %w", err)
		}
		result["json_report_path"] = jsonPath
	}

	if reportType == "markdown" || reportType == "both" {
		mdData, err := report.GenerateMarkdown(data)
		if err != nil {
			return nil, fmt.Errorf("generate markdown report: %w", err)
		}
		mdPath := filepath.Join(params.ProjectPath, "drup-report.md")
		if err := os.WriteFile(mdPath, []byte(mdData), 0o644); err != nil {
			return nil, fmt.Errorf("write markdown report: %w", err)
		}
		result["markdown_report_path"] = mdPath
	}

	return json.Marshal(result)
}

func realHandleModuleInfo(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module             string `json:"module_machine_name"`
		IncludeMaintainers bool   `json:"include_maintainers"`
		IncludeDeps        bool   `json:"include_dependencies"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}

	meta, err := drupalorg.ModuleInfo(params.Module)
	if err != nil {
		return nil, err
	}

	if !params.IncludeMaintainers {
		meta.Maintainers = []string{}
	}

	return json.Marshal(meta)
}

// versionMatrixData holds static Drupal/PHP compatibility data.
var versionMatrixData = map[string]struct {
	PHPMin         string `json:"minimum"`
	PHPRecommended string `json:"recommended"`
	SupportedUntil string `json:"supported_until"`
	NextMajor      string `json:"next_major"`
}{
	"9":  {PHPMin: "7.3", PHPRecommended: "8.1", SupportedUntil: "2024-06", NextMajor: "10"},
	"10": {PHPMin: "8.1", PHPRecommended: "8.3", SupportedUntil: "2026-06", NextMajor: "11"},
	"11": {PHPMin: "8.3", PHPRecommended: "8.4", SupportedUntil: "TBA", NextMajor: ""},
}

var phpVersionPattern = regexp.MustCompile(`^\d+\.\d+$`)

func realHandleDrupalVersionMatrix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		DrupalVersion string `json:"drupal_version"`
		PHPVersion    string `json:"php_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// If drupal_version provided, look up directly.
	if params.DrupalVersion != "" {
		entry, ok := versionMatrixData[params.DrupalVersion]
		if !ok {
			return nil, fmt.Errorf("unknown Drupal version: %s", params.DrupalVersion)
		}
		result := map[string]interface{}{
			"drupal_version": params.DrupalVersion,
			"php_requirements": map[string]string{
				"minimum":     entry.PHPMin,
				"recommended": entry.PHPRecommended,
			},
			"supported_until": entry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": entry.NextMajor,
			},
		}
		return json.Marshal(result)
	}

	// If php_version provided, find compatible Drupal versions.
	if params.PHPVersion != "" {
		if !phpVersionPattern.MatchString(params.PHPVersion) {
			return nil, fmt.Errorf("invalid php_version format: %s", params.PHPVersion)
		}
		// Find the latest Drupal version compatible with this PHP version.
		var latestVersion string
		var latestEntry struct {
			PHPMin         string
			PHPRecommended string
			SupportedUntil string
			NextMajor      string
		}
		for ver, entry := range versionMatrixData {
			if isPHPCompatible(params.PHPVersion, entry.PHPMin, entry.PHPRecommended) {
				if ver > latestVersion {
					latestVersion = ver
					latestEntry = struct {
						PHPMin         string
						PHPRecommended string
						SupportedUntil string
						NextMajor      string
					}{entry.PHPMin, entry.PHPRecommended, entry.SupportedUntil, entry.NextMajor}
				}
			}
		}
		if latestVersion == "" {
			return nil, fmt.Errorf("no Drupal version compatible with PHP %s", params.PHPVersion)
		}
		result := map[string]interface{}{
			"drupal_version": latestVersion,
			"php_requirements": map[string]string{
				"minimum":     latestEntry.PHPMin,
				"recommended": latestEntry.PHPRecommended,
			},
			"supported_until": latestEntry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": latestEntry.NextMajor,
			},
		}
		return json.Marshal(result)
	}

	// Neither provided — return full matrix.
	var matrix []interface{}
	for ver, entry := range versionMatrixData {
		matrix = append(matrix, map[string]interface{}{
			"drupal_version": ver,
			"php_requirements": map[string]string{
				"minimum":     entry.PHPMin,
				"recommended": entry.PHPRecommended,
			},
			"supported_until": entry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": entry.NextMajor,
			},
		})
	}
	return json.Marshal(map[string]interface{}{"matrix": matrix})
}

func isPHPCompatible(phpVer, phpMin, phpRecommended string) bool {
	// Simple comparison: phpVer >= phpMin.
	return phpVer >= phpMin
}

// --- Core upgrade + patch reconcile tool handlers ---

func realHandleCoreUpgradeCheck(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	if !filepath.IsAbs(params.ProjectPath) {
		return nil, fmt.Errorf("project_path must be an absolute path: %s", params.ProjectPath)
	}
	if strings.Contains(params.ProjectPath, "..") {
		return nil, fmt.Errorf("project_path must not contain '..' segments")
	}

	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}
	if detection.Environment == envdetect.EnvUnsupported {
		result := map[string]interface{}{
			"current_version":        "",
			"next_version":           "",
			"composer_patch_preview": "",
			"supported":              false,
		}
		return json.Marshal(result)
	}

	currentVersion, err := coreCurrentVersion(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	check, err := coreupgrade.NextMajor(currentVersion)
	if err != nil {
		return nil, err
	}

	preview := ""
	if check.Available {
		composerData, rerr := os.ReadFile(filepath.Join(params.ProjectPath, "composer.json"))
		if rerr != nil {
			return nil, fmt.Errorf("read composer.json: %w", rerr)
		}
		diff, _, perr := coreupgrade.PreviewComposerPatch(composerData, check.Constraint)
		if perr != nil {
			return nil, perr
		}
		preview = diff
	}

	result := map[string]interface{}{
		"current_version":        currentVersion,
		"next_version":           check.NextVersion,
		"composer_patch_preview": preview,
		"supported":              true,
	}
	return json.Marshal(result)
}

// coreCurrentVersion resolves the installed drupal/core version: prefers
// composer.lock's exact pinned version, falling back to the composer.json
// require constraint when no lock file is present.
func coreCurrentVersion(projectPath string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(projectPath, "composer.lock")); err == nil {
		var lock struct {
			Packages []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"packages"`
		}
		if json.Unmarshal(data, &lock) == nil {
			for _, p := range lock.Packages {
				if p.Name == drupalCorePackageName {
					return strings.TrimPrefix(p.Version, "v"), nil
				}
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "composer.json"))
	if err != nil {
		return "", fmt.Errorf("read composer.json: %w", err)
	}
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse composer.json: %w", err)
	}
	for _, key := range []string{"drupal/core-recommended", drupalCorePackageName} {
		if v, ok := doc.Require[key]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("drupal/core not found in composer.json or composer.lock")
}

// drupalCorePackageName is the composer package name for Drupal core itself.
const drupalCorePackageName = "drupal/core"

func realHandleCoreUpgradeApply(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath   string `json:"project_path"`
		TargetVersion string `json:"target_version"`
		DryRun        bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	if params.TargetVersion == "" {
		return nil, fmt.Errorf("target_version is required")
	}

	result, err := coreupgrade.Apply(params.ProjectPath, params.TargetVersion, params.DryRun)
	if err != nil {
		return nil, err
	}

	response := map[string]interface{}{
		"success":             result.Success,
		"report":              result.Report,
		"rollback_checkpoint": result.RollbackCheckpoint,
		"stderr":              result.Stderr,
	}
	return json.Marshal(response)
}

func realHandlePatchReconcile(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module          string `json:"module_machine_name"`
		CurrentPatchURL string `json:"current_patch_url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}
	if params.CurrentPatchURL == "" {
		return nil, fmt.Errorf("current_patch_url is required")
	}

	result, err := patchreconcile.Reconcile(params.Module, params.CurrentPatchURL)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
