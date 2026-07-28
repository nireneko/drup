package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nireneko/drup/internal/composerutil"
	"github.com/nireneko/drup/internal/coreupgrade"
	"github.com/nireneko/drup/internal/drupalorg"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/installer"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/metrics"
	"github.com/nireneko/drup/internal/packaging"
	"github.com/nireneko/drup/internal/patch"
	"github.com/nireneko/drup/internal/report"
	"github.com/nireneko/drup/internal/scan"
	"github.com/nireneko/drup/internal/semver"
	statepkg "github.com/nireneko/drup/internal/state"
	"github.com/nireneko/drup/internal/update"
)

// RunInit verifies the project is a valid Drupal project.
func RunInit(args []string) error {
	cwd, err := getwdFn()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Check for composer.json.
	composerPath := filepath.Join(cwd, "composer.json")
	if _, err := os.Stat(composerPath); os.IsNotExist(err) {
		return fmt.Errorf("not a Drupal project: composer.json not found in %s", cwd)
	}

	// Read composer.json and check for drupal/core.
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return fmt.Errorf("read composer.json: %w", err)
	}

	var composer map[string]interface{}
	if err := json.Unmarshal(data, &composer); err != nil {
		return fmt.Errorf("parse composer.json: %w", err)
	}

	require, _ := composer["require"].(map[string]interface{})
	hasCore := false
	for _, pkg := range []string{"drupal/core", "drupal/core-recommended"} {
		if _, ok := require[pkg]; ok {
			hasCore = true
			break
		}
	}
	if !hasCore {
		return fmt.Errorf("not a Drupal project: drupal/core or drupal/core-recommended not found in composer.json require")
	}

	fmt.Println("Drupal project initialized successfully.")
	return nil
}

// isScanExitOK returns true for exit codes that carry valid scan data.
// 0 = no findings, 3 = findings exist. 1, 2, >3 = real errors.
func isScanExitOK(exitCode int) bool {
	return exitCode == 0 || exitCode == 3
}

// webRootFor returns the configured docroot directory name.
func webRootFor(projectPath string) string {
	return composerutil.ReadWebRoot(projectPath)
}

// resolveDrupalRoot returns the directory holding modules/ and themes/.
// Callers pass either the project root (composer.json level) or the docroot,
// and assuming the wrong one makes a project full of custom code look empty.
func resolveDrupalRoot(path string) string {
	if webRoot := composerutil.ReadWebRoot(path); webRoot != "" {
		candidate := filepath.Join(path, webRoot)
		if info, err := os.Stat(filepath.Join(candidate, "modules")); err == nil && info.IsDir() {
			return candidate
		}
	}
	return path
}

// hasNoCustomCode returns true if both modules/custom/ and themes/custom/
// have no subdirectories (i.e., no custom modules or themes exist).
func hasNoCustomCode(projectPath string) bool {
	root := resolveDrupalRoot(projectPath)
	customModules := filepath.Join(root, "modules", "custom")
	customThemes := filepath.Join(root, "themes", "custom")
	return dirHasNoSubdirs(customModules) && dirHasNoSubdirs(customThemes)
}

// dirHasNoSubdirs returns true if the directory doesn't exist or has no subdirectories.
func dirHasNoSubdirs(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return true // Directory doesn't exist → no custom code.
	}
	for _, e := range entries {
		if e.IsDir() {
			return false
		}
	}
	return true
}

// cliRun detects the environment for projectPath and runs cmd with the
// appropriate prefix. Uses --root= instead of -r for drush commands.
// Returns the same (stdout, stderr, exitCode, err) as drupexec.Run.
func cliRun(projectPath string, cmd string, args ...string) (string, string, int, error) {
	detection, err := defaultEnvDetector.Detect(projectPath, false)
	if err != nil {
		return "", "", -1, fmt.Errorf("detect environment: %w", err)
	}
	// Run from the project so container CLIs can resolve it. Without this the
	// MCP server, whose working directory is wherever the agent started it,
	// fails every ddev and lando call.
	return drupexec.RunWithEnv(projectPath, detection.CommandPrefix, cmd, args...)
}

// isContainerized reports whether commands for projectPath run inside a
// container. Host paths are meaningless there: the project is mounted at the
// container's own working directory, so binaries and config must be addressed
// relative to the project root.
func isContainerized(projectPath string) bool {
	detection, err := defaultEnvDetector.Detect(projectPath, false)
	if err != nil {
		return false
	}
	return len(detection.CommandPrefix) > 0
}

// projectRelPath returns the path to use for a file inside the project:
// relative when the command runs in a container, absolute on the host.
func projectRelPath(projectPath string, parts ...string) string {
	if isContainerized(projectPath) {
		return filepath.Join(parts...)
	}
	return filepath.Join(append([]string{projectPath}, parts...)...)
}

// drushExecError wraps a drush execution failure with command context.
func drushExecError(cmd string, args []string, exitCode int, stderr, stdout string) error {
	fullCmd := cmd + " " + strings.Join(args, " ")
	truncated := stdout
	if len(truncated) > 500 {
		truncated = truncated[:500] + "..."
	}
	if exitCode != 0 {
		return fmt.Errorf("drush command %q exited %d\nstderr: %s\nstdout: %s", fullCmd, exitCode, stderr, truncated)
	}
	return fmt.Errorf("drush command %q failed: %s\nstderr: %s\nstdout: %s", fullCmd, stderr, stderr, truncated)
}

// RunScan runs upgrade_status:checkstyle and outputs structured JSON.
func RunScan(path string) error {
	// Smart no-op bypass: skip if both custom dirs are empty.
	if hasNoCustomCode(path) {
		fmt.Fprintln(os.Stderr, "scan: no custom code found, skipping rector and custom analysis")
		result := &scan.ScanResult{
			ProjectPath: path,
			Modules:     []scan.ModuleStatus{},
			TotalErrs:   0,
		}
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal result: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// --format=checkstyle: the XML schema is stable, while the human-readable
	// table changes shape between upgrade_status releases. The standalone
	// upgrade_status:checkstyle command is deprecated.
	scanArgs := []string{"upgrade_status:analyze", "--all", "--format=checkstyle", "--root=" + path}
	stdout, stderr, exitCode, err := cliRun(path, "drush", scanArgs...)
	if err != nil {
		return drushExecError("drush", scanArgs, -1, err.Error(), "")
	}
	if !isScanExitOK(exitCode) {
		return drushExecError("drush", scanArgs, exitCode, stderr, stdout)
	}

	// Exit code 3 with empty stdout means drush crashed, not findings.
	if exitCode == 3 && strings.TrimSpace(stdout) == "" {
		return fmt.Errorf("drush exited with code 3 but produced no output (command: drush %s)\nstderr: %s", strings.Join(scanArgs, " "), stderr)
	}

	result, err := scan.ParseCheckstyle(strings.NewReader(stdout))
	if err != nil {
		return fmt.Errorf("parse scan output (command: drush %s): %w\nstdout (truncated): %.500s", strings.Join(scanArgs, " "), err, stdout)
	}

	result.ProjectPath = path
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// ensureRectorConfig returns the rector config path to pass on the command
// line, creating one from drupal-rector's shipped sample when the project has
// none. Rector refuses to run without a config, and drupal-rector ships the
// sample precisely to be copied into the project root.
func ensureRectorConfig(projectPath string) (string, error) {
	configFile := filepath.Join(projectPath, "rector.php")
	if _, err := os.Stat(configFile); err == nil {
		return projectRelPath(projectPath, "rector.php"), nil
	}

	sample := filepath.Join(projectPath, "vendor", "palantirnet", "drupal-rector", "rector.php")
	data, err := os.ReadFile(sample)
	source := "the drupal-rector sample"
	if err != nil {
		data = []byte(defaultRectorConfig)
		source = "drup's built-in defaults"
	}
	if err := os.WriteFile(configFile, data, 0o644); err != nil {
		return "", fmt.Errorf("write rector.php: %w", err)
	}
	fmt.Fprintf(os.Stderr, "fix: created rector.php from %s\n", source)
	return projectRelPath(projectPath, "rector.php"), nil
}

// defaultRectorConfig is used when drupal-rector ships no sample to copy.
const defaultRectorConfig = `<?php

declare(strict_types=1);

use DrupalRector\Set\Drupal10SetList;
use Rector\Config\RectorConfig;

return static function (RectorConfig $rectorConfig): void {
    $rectorConfig->sets([Drupal10SetList::DRUPAL_10]);
    $rectorConfig->fileExtensions(['php', 'module', 'theme', 'install', 'profile', 'inc', 'engine']);
    $rectorConfig->importNames(true, false);
    $rectorConfig->importShortClasses(false);
};
`

// RunFix runs drupal-rector on the target project.
func RunFix(path string) error {
	// Run rector on custom modules and themes.
	root := resolveDrupalRoot(path)
	customModules := filepath.Join(root, "modules", "custom")
	themes := filepath.Join(root, "themes")

	targets := []string{}
	for _, dir := range []string{customModules, themes} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			rel, err := filepath.Rel(path, dir)
			if err != nil || !isContainerized(path) {
				targets = append(targets, dir)
				continue
			}
			targets = append(targets, rel)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("no custom modules or themes directories found in %s", path)
	}

	configPath, err := ensureRectorConfig(path)
	if err != nil {
		return err
	}

	args := append([]string{"process"}, targets...)
	args = append(args, "--config="+configPath)

	// Rector must run against the site's PHP, so it goes through the same
	// environment prefix as drush and composer.
	stdout, stderr, exitCode, err := cliRun(path, projectRelPath(path, "vendor", "bin", "rector"), args...)
	if err != nil {
		return fmt.Errorf("exec rector: %w", err)
	}

	fmt.Println(stdout)
	if exitCode != 0 {
		fmt.Fprintf(os.Stderr, "rector exit %d: %s\n", exitCode, stderr)
	}

	// Re-scan to show remaining errors.
	fmt.Fprintln(os.Stderr, "--- Remaining errors after fix ---")
	return RunScan(path)
}

// RunContrib checks Drupal.org for D11 compatibility of a module.
func RunContrib(module string) error {
	info, err := drupalorg.CheckRelease(module)
	if err != nil {
		return fmt.Errorf("check release: %w", err)
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// RunIssue extracts patch/diff/MR links from Drupal.org issues.
func RunIssue(query string) error {
	result, err := drupalorg.SearchPatches(query)
	if err != nil {
		return fmt.Errorf("search patches: %w", err)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// RunReport generates JSON and markdown reports.
func RunReport(path string) error {
	// Call DoValidate to get live scan data
	result, filtered, err := doValidateFn(path, "")
	if err != nil {
		return fmt.Errorf("scan for report: %w", err)
	}

	// Populate report data from scan results
	reportData := &report.ReportData{
		ProjectPath:     path,
		TotalErrors:     len(filtered),
		Resolved:        []report.ResolvedItem{},
		Pending:         []report.PendingItem{},
		PipelineMetrics: snapshotMetrics(),
	}

	// Convert scan errors to pending items
	for _, depErr := range filtered {
		reportData.Pending = append(reportData.Pending, report.PendingItem{
			Module:          extractModuleName(depErr.File),
			Type:            string(depErr.Severity),
			Error:           depErr.Message,
			SuggestedAction: fmt.Sprintf("Fix deprecation at %s:%d", depErr.File, depErr.Line),
		})
	}

	// Use result for additional context if needed
	_ = result

	jsonData, err := report.GenerateJSON(reportData)
	if err != nil {
		return fmt.Errorf("generate JSON report: %w", err)
	}

	mdData, err := report.GenerateMarkdown(reportData)
	if err != nil {
		return fmt.Errorf("generate markdown report: %w", err)
	}

	// Write files.
	jsonPath := filepath.Join(path, "drup-report.json")
	mdPath := filepath.Join(path, "drup-report.md")

	if err := os.WriteFile(jsonPath, jsonData, 0o644); err != nil {
		return fmt.Errorf("write JSON report: %w", err)
	}
	if err := os.WriteFile(mdPath, []byte(mdData), 0o644); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}

	fmt.Printf("Reports written to %s and %s\n", jsonPath, mdPath)
	return nil
}

// extractModuleName extracts the module name from a file path.
func extractModuleName(filePath string) string {
	parts := strings.Split(filePath, "/")
	for i, part := range parts {
		if part == "modules" && i+2 < len(parts) {
			return parts[i+2]
		}
	}
	return "unknown"
}

// snapshotMetrics returns a metrics snapshot, recovering from any panic.
func snapshotMetrics() *metrics.Metrics {
	defer func() { recover() }()
	snap := metrics.Default().Snapshot()
	return &snap
}

// RunMCP starts the MCP stdio server.
func RunMCP() error {
	server := mcp.NewServer(os.Stdout, Version)
	WireMCPTools(server)
	return server.Run()
}

// DoValidate runs upgrade_status:checkstyle and returns parsed results.
// Shared between CLI and MCP handlers.
// For Drupal >= 11.x, uses drush updb/cr/status as primary gates.
func DoValidate(projectPath, module string) (*scan.ScanResult, []scan.DepError, error) {
	// Detect core version to determine gate strategy.
	coreVersion := detectDrupalVersion(projectPath)
	isPostD11 := false
	if coreVersion != "" {
		v, err := semver.Parse(coreVersion)
		if err == nil && v.Major >= 11 {
			isPostD11 = true
		}
	}

	if isPostD11 {
		return doValidatePostD11(projectPath, module)
	}
	return doValidatePreD11(projectPath, module)
}

// doValidatePreD11 uses upgrade_status:checkstyle as the primary gate.
func doValidatePreD11(projectPath, module string) (*scan.ScanResult, []scan.DepError, error) {
	analyzeTarget := "--all"
	if module != "" {
		analyzeTarget = module
	}

	stdout, stderr, exitCode, err := cliRun(projectPath, "drush", "upgrade_status:analyze", analyzeTarget, "--format=checkstyle", "--root="+projectPath)
	if err != nil {
		return nil, nil, drushExecError("drush", []string{"upgrade_status:analyze", analyzeTarget, "--format=checkstyle", "--root=" + projectPath}, -1, err.Error(), "")
	}
	if !isScanExitOK(exitCode) {
		return nil, nil, drushExecError("drush", []string{"upgrade_status:analyze", analyzeTarget, "--format=checkstyle", "--root=" + projectPath}, exitCode, stderr, stdout)
	}

	// Exit code 3 with empty stdout means drush crashed, not findings.
	if exitCode == 3 && strings.TrimSpace(stdout) == "" {
		return nil, nil, fmt.Errorf("drush exited with code 3 but produced no output (command: drush upgrade_status:checkstyle %s --root=%s)\nstderr: %s", analyzeTarget, projectPath, stderr)
	}

	result, err := scan.ParseCheckstyle(strings.NewReader(stdout))
	if err != nil {
		return nil, nil, fmt.Errorf("parse scan output (command: drush upgrade_status:checkstyle %s --root=%s): %w\nstdout (truncated): %.500s", projectPath, analyzeTarget, err, stdout)
	}

	// Filter by module if specified.
	var filtered []scan.DepError
	for _, mod := range result.Modules {
		if module != "" && mod.Name != module {
			continue
		}
		filtered = append(filtered, mod.Errors...)
	}

	return result, filtered, nil
}

// doValidatePostD11 uses drush updb/cr/status as primary gates.
// upgrade_status:checkstyle is run as informational output only.
func doValidatePostD11(projectPath, module string) (*scan.ScanResult, []scan.DepError, error) {
	// Gate 1: drush updb -y.
	_, stderr, exitCode, err := cliRun(projectPath, "drush", "updb", "-y", "--root="+projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("drush updb: %w", err)
	}
	if exitCode != 0 {
		return nil, nil, fmt.Errorf("drush updb failed (exit %d): %s", exitCode, stderr)
	}

	// Gate 2: drush cr.
	_, stderr, exitCode, err = cliRun(projectPath, "drush", "cr", "--root="+projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("drush cr: %w", err)
	}
	if exitCode != 0 {
		return nil, nil, fmt.Errorf("drush cr failed (exit %d): %s", exitCode, stderr)
	}

	// Gate 3: drush status (must exit 0 = site bootstraps).
	_, stderr, exitCode, err = cliRun(projectPath, "drush", "status", "--root="+projectPath)
	if err != nil {
		return nil, nil, fmt.Errorf("drush status: %w", err)
	}
	if exitCode != 0 {
		return nil, nil, fmt.Errorf("site bootstrap failed: drush status exit %d: %s", exitCode, stderr)
	}

	// Informational: run upgrade_status:checkstyle but don't gate on it.
	analyzeTarget := "--all"
	if module != "" {
		analyzeTarget = module
	}
	stdout, _, analyzeExit, _ := cliRun(projectPath, "drush", "upgrade_status:analyze", analyzeTarget, "--format=checkstyle", "--root="+projectPath)

	result := &scan.ScanResult{
		ProjectPath: projectPath,
		Modules:     []scan.ModuleStatus{},
	}
	var filtered []scan.DepError

	if isScanExitOK(analyzeExit) && strings.TrimSpace(stdout) != "" {
		if parsed, err := scan.ParseCheckstyle(strings.NewReader(stdout)); err == nil {
			result = parsed
			for _, mod := range parsed.Modules {
				if module != "" && mod.Name != module {
					continue
				}
				filtered = append(filtered, mod.Errors...)
			}
		}
	}

	return result, filtered, nil
}

// RunValidate runs upgrade_status:checkstyle and outputs JSON with error count.
// Exit 0 if clean, exit 1 if errors remain.
func RunValidate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: drup validate <path> [module]")
	}

	projectPath := args[0]
	module := ""
	if len(args) > 1 {
		module = args[1]
	}

	_, filtered, err := DoValidate(projectPath, module)
	if err != nil {
		return err
	}

	output := map[string]interface{}{
		"total_errors": len(filtered),
		"errors":       filtered,
	}
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))

	if len(filtered) > 0 {
		return fmt.Errorf("validation found %d errors", len(filtered))
	}
	return nil
}

// DoApplyPatch downloads and applies a patch to the project.
// Shared between CLI and MCP handlers. composerPackage decides both where the
// patch is applied from and how it is registered in composer.json; without it
// a module patch cannot resolve its paths.
func DoApplyPatch(patchURL, projectPath, composerPackage, description string) (*patch.ApplyResult, error) {
	return patch.Apply(patchURL, projectPath, composerPackage, description)
}

// RunApplyPatch downloads and applies a patch, outputting JSON result.
func RunApplyPatch(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: drup apply-patch <url> <path> [composer-package] [description]")
	}

	patchURL := args[0]
	projectPath := args[1]
	composerPackage := ""
	if len(args) > 2 {
		composerPackage = args[2]
	}
	description := ""
	if len(args) > 3 {
		description = args[3]
	}

	result, err := DoApplyPatch(patchURL, projectPath, composerPackage, description)
	if err != nil {
		return err
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return nil
}

// RunInstall detects agents and writes skill files.
func RunInstall() error {
	agents := installer.DetectAgents()
	if len(agents) == 0 {
		return fmt.Errorf("no agents detected — install Claude Code, OpenCode, or Codex first")
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get binary path: %w", err)
	}

	// Render templates for each detected agent. A failure on one agent (for
	// example a corrupt config file) must not block the remaining agents.
	agentIDs, failures := installAgents(agents, binaryPath, "install")
	if len(agentIDs) == 0 {
		return fmt.Errorf("install failed for every detected agent:\n  %s", strings.Join(failures, "\n  "))
	}
	reportInstallFailures(failures)

	// Update state.
	s, _ := statepkg.Load()
	s.InstalledAgents = agentIDs
	s.Version = Version
	if err := statepkg.Save(s); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Println("\nRestart your agents to load the drup MCP server. Agents write their own config files, so a session running during the install may overwrite this registration.")

	return nil
}

// RunSync re-applies agent assets.
func RunSync() error {
	s, err := statepkg.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	if len(s.InstalledAgents) == 0 {
		return fmt.Errorf("no agents installed — run 'drup install' first")
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get binary path: %w", err)
	}

	// Re-install to all previously installed agents.
	agents := installer.DetectAgents()
	synced, failures := installAgents(agents, binaryPath, "sync")
	if len(synced) == 0 {
		return fmt.Errorf("sync failed for every detected agent:\n  %s", strings.Join(failures, "\n  "))
	}
	reportInstallFailures(failures)

	// Keep the flag set when an agent still needs a successful sync.
	s.PendingSync = len(failures) > 0
	if err := statepkg.Save(s); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
}

// installAgents renders and installs assets for each agent independently.
// It returns the agents that succeeded and a message per agent that failed,
// so one broken agent config cannot block the others.
func installAgents(agents []installer.AgentAdapter, binaryPath, action string) (succeeded []string, failures []string) {
	for _, agent := range agents {
		files, err := packaging.Render(agent.ID(), binaryPath)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: render templates: %v", agent.ID(), err))
			continue
		}
		results, err := installer.Install([]installer.AgentAdapter{agent}, binaryPath, files)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %s: %v", agent.ID(), action, err))
			continue
		}
		printSyncResults(agent.ID(), results)
		succeeded = append(succeeded, agent.ID())
	}
	return succeeded, failures
}

// reportInstallFailures warns about skipped agents without failing the run.
func reportInstallFailures(failures []string) {
	for _, f := range failures {
		fmt.Fprintf(os.Stderr, "Warning: skipped %s\n", f)
	}
}

// printSyncResults displays per-file sync status grouped by agent.
func printSyncResults(agentID string, results []installer.SyncFileResult) {
	fmt.Printf("Synced drup to %s\n", agentID)
	for _, r := range results {
		fmt.Printf("  %s: %s\n", r.Status, r.Path)
	}
}

// checkLatestFn and upgradeFn wrap the update package's entry points.
// Package-level vars for testability.
var checkLatestFn = update.CheckLatest
var upgradeFn = update.Upgrade

// RunUninstall override points for testability.
var stateLoadFn = statepkg.Load
var osExecutableFn = os.Executable
var osUserHomeDirFn = os.UserHomeDir
var stateRemoveFn = statepkg.Remove

// RunUpgradeCore override points for testability.
var getwdFn = os.Getwd
var isCleanFn = gitops.IsClean

// doValidateFn wraps DoValidate for testability.
var doValidateFn = DoValidate

// RunUpgrade self-updates the binary. It uses the runtime's actual
// GOOS/GOARCH for asset selection — GOOS/GOARCH environment overrides are
// never honored — and delegates the download/verify/extract/replace flow to
// update.Upgrade.
func RunUpgrade() error {
	version, _, err := checkLatestFn("nireneko", "drup", runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}

	if version == Version {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", version, Version)
	fmt.Println("Downloading and installing update...")

	opts := update.UpgradeOptions{
		Owner:   "nireneko",
		Repo:    "drup",
		Binary:  "drup",
		Version: version,
	}
	if err := upgradeFn(opts); err != nil {
		return fmt.Errorf("upgrade: %w", err)
	}

	// Set pending_sync in state.
	s, _ := statepkg.Load()
	s.PendingSync = true
	s.Version = version
	if err := statepkg.Save(s); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	fmt.Printf("Updated to version %s. Run 'drup sync' to update agent configs.\n", version)
	return nil
}

// Preflight check categories. The distinction decides what blocks a run:
// the environment must work before anything else can, while readiness
// describes the upgrade the pipeline exists to perform.
const (
	CategoryEnvironment = "environment"
	CategoryReadiness   = "readiness"
)

// readinessChecks are the checks that report outstanding upgrade work rather
// than a broken environment. Failing them is the reason to run the pipeline,
// not a reason to refuse to start it.
var readinessChecks = map[string]bool{
	"core_composer_constraint": true,
	"core_module_compat":       true,
}

// drupArtifacts are paths drup itself writes into a project. They must not
// count against the working tree being clean, or taking a backup would block
// the run that requested it.
var drupArtifacts = []string{".drup/", "drup-report.json", "drup-report.md", "rector.php"}

// withoutDrupArtifacts filters drup's own files out of a git status listing.
func withoutDrupArtifacts(files []string) []string {
	kept := make([]string, 0, len(files))
	for _, f := range files {
		name := strings.TrimSpace(f)
		// git status --porcelain lines start with a two-character status.
		if len(name) > 3 && (name[2] == ' ' || name[1] == ' ') {
			name = strings.TrimSpace(name[2:])
		}
		name = strings.Trim(name, `"`)
		drupOwned := false
		for _, artifact := range drupArtifacts {
			if name == artifact || strings.HasPrefix(name, artifact) {
				drupOwned = true
				break
			}
		}
		if !drupOwned {
			kept = append(kept, f)
		}
	}
	return kept
}

// PreflightResult holds the outcome of each preflight check.
type PreflightResult struct {
	Check    string `json:"check"`
	Pass     bool   `json:"pass"`
	Message  string `json:"message"`
	Category string `json:"category"`
}

// categorize fills in the category and reports whether a failure blocks the run.
func categorize(results []PreflightResult) (environmentFailures, readinessFailures int) {
	for i := range results {
		r := &results[i]
		if readinessChecks[r.Check] {
			r.Category = CategoryReadiness
		} else {
			r.Category = CategoryEnvironment
		}
		if r.Pass {
			continue
		}
		if r.Category == CategoryReadiness {
			readinessFailures++
		} else {
			environmentFailures++
		}
	}
	return environmentFailures, readinessFailures
}

// RunPreflight checks project readiness for upgrade automation.
// The project path is explicit: preflight installs dev dependencies with
// composer, and inferring the target from the shell's working directory once
// left a vendor tree in an unrelated repository.
func RunPreflight(args []string) error {
	cwd := ""
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown option %q — usage: drup preflight [path]", arg)
		}
		cwd = arg
	}
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", cwd, err)
	}
	cwd = abs
	if _, err := os.Stat(filepath.Join(cwd, "composer.json")); err != nil {
		return fmt.Errorf("not a Drupal project: no composer.json in %s", cwd)
	}

	var results []PreflightResult
	allPass := true

	// 1. Detect Drupal version from composer.lock.
	drupalVersion := detectDrupalVersion(cwd)
	if drupalVersion != "" {
		results = append(results, PreflightResult{
			Check:   "drupal_version",
			Pass:    true,
			Message: fmt.Sprintf("Drupal %s detected", drupalVersion),
		})
	} else {
		results = append(results, PreflightResult{
			Check:   "drupal_version",
			Pass:    false,
			Message: "Could not detect Drupal version from composer.lock",
		})
		allPass = false
	}

	// 2. Check git clean, ignoring drup's own artifacts. The pipeline takes a
	// backup into .drup/ before it starts, which used to make its own next
	// check fail.
	clean, files, err := gitops.IsClean(cwd)
	if err == nil {
		files = withoutDrupArtifacts(files)
		clean = len(files) == 0
	}
	if err != nil {
		results = append(results, PreflightResult{
			Check:   "git_clean",
			Pass:    false,
			Message: fmt.Sprintf("git check failed: %v", err),
		})
		allPass = false
	} else if !clean {
		results = append(results, PreflightResult{
			Check:   "git_clean",
			Pass:    false,
			Message: fmt.Sprintf("Working tree has %d uncommitted changes", len(files)),
		})
		allPass = false
	} else {
		results = append(results, PreflightResult{
			Check:   "git_clean",
			Pass:    true,
			Message: "Working tree is clean",
		})
	}

	// 3. Check composer available.
	if _, _, exitCode, err := drupexec.Run("composer", "--version"); err != nil || exitCode != 0 {
		results = append(results, PreflightResult{
			Check:   "composer",
			Pass:    false,
			Message: "composer not found on PATH",
		})
		allPass = false
	} else {
		results = append(results, PreflightResult{
			Check:   "composer",
			Pass:    true,
			Message: "composer available",
		})
	}

	// 4. Check drush available.
	drushFound := false
	for _, candidate := range []string{"drush", filepath.Join(cwd, "vendor", "bin", "drush")} {
		if _, _, exitCode, err := drupexec.Run(candidate, "--version"); err == nil && exitCode == 0 {
			drushFound = true
			break
		}
	}
	if !drushFound {
		results = append(results, PreflightResult{
			Check:   "drush",
			Pass:    false,
			Message: "drush not found on PATH or vendor/bin",
		})
		allPass = false
	} else {
		results = append(results, PreflightResult{
			Check:   "drush",
			Pass:    true,
			Message: "drush available",
		})
	}

	// 5. Install dev dependencies if missing.
	devDeps := []struct {
		Pkg string
		Dev bool
	}{
		{"drupal/upgrade_status", true},
		{"palantirnet/drupal-rector", true},
		{"mglaman/phpstan-drupal", true},
	}

	composerFile := filepath.Join(cwd, "composer.json")
	composerData, _ := os.ReadFile(composerFile)
	var composerJSON map[string]interface{}
	json.Unmarshal(composerData, &composerJSON)

	requireDev, _ := composerJSON["require-dev"].(map[string]interface{})

	for _, dep := range devDeps {
		if _, ok := requireDev[dep.Pkg]; ok {
			results = append(results, PreflightResult{
				Check:   "dev_dep_" + dep.Pkg,
				Pass:    true,
				Message: dep.Pkg + " already installed",
			})
			continue
		}

		// Install the dev dependency. It must run where the site runs: the
		// host PHP version rarely matches the container's, and composer
		// resolves platform requirements against the PHP running it.
		fmt.Printf("Installing %s...\n", dep.Pkg)
		_, stderr, exitCode, err := cliRun(cwd, "composer", "require", "--dev", dep.Pkg)
		if err != nil || exitCode != 0 {
			results = append(results, PreflightResult{
				Check:   "dev_dep_" + dep.Pkg,
				Pass:    false,
				Message: fmt.Sprintf("Failed to install %s: %s", dep.Pkg, stderr),
			})
			allPass = false
		} else {
			results = append(results, PreflightResult{
				Check:   "dev_dep_" + dep.Pkg,
				Pass:    true,
				Message: dep.Pkg + " installed",
			})
		}
	}

	// 5.5. Detect PHP version and patch settings.php for PHP 8.4+
	phpVersion, err := detectPHPVersion(cwd)
	if err != nil {
		results = append(results, PreflightResult{
			Check:   "php_version",
			Pass:    false,
			Message: fmt.Sprintf("Failed to detect PHP version: %v", err),
		})
		allPass = false
	} else {
		results = append(results, PreflightResult{
			Check:   "php_version",
			Pass:    true,
			Message: fmt.Sprintf("PHP %s detected", phpVersion),
		})

		if isPHP84OrLater(phpVersion) {
			fmt.Println("PHP 8.4+ detected, patching settings.php to suppress deprecation warnings...")
			if err := patchSettingsPHP(cwd); err != nil {
				results = append(results, PreflightResult{
					Check:   "php84_compat",
					Pass:    false,
					Message: fmt.Sprintf("Failed to patch settings.php: %v", err),
				})
				allPass = false
			} else {
				// Name the file and the backup: this is a mutation of a
				// tracked file, not just a check that passed.
				settingsPath := filepath.Join(cwd, webRootFor(cwd), "sites", "default", "settings.php")
				results = append(results, PreflightResult{
					Check:   "php84_compat",
					Pass:    true,
					Message: fmt.Sprintf("MODIFIED %s: appended error_reporting() to silence PHP 8.4 deprecation notices (original saved as settings.php.bak)", settingsPath),
				})
			}
		}
	}

	// 6. Enable upgrade_status module.
	fmt.Println("Enabling upgrade_status module...")
	// Delete conflicting update.settings config before enabling.
	_, _, _, _ = cliRun(cwd, "drush", "config:delete", "update.settings", "--root="+cwd)
	_, stderr, exitCode, err := cliRun(cwd, "drush", "en", "upgrade_status", "-y", "--root="+cwd)
	if err != nil || exitCode != 0 {
		results = append(results, PreflightResult{
			Check:   "enable_upgrade_status",
			Pass:    false,
			Message: fmt.Sprintf("Failed to enable upgrade_status: %s", stderr),
		})
		allPass = false
	} else {
		// Drush caches its command list, so upgrade_status:checkstyle stays
		// undefined until the cache is rebuilt.
		_, _, _, _ = cliRun(cwd, "drush", "cr", "--root="+cwd)
		results = append(results, PreflightResult{
			Check:   "enable_upgrade_status",
			Pass:    true,
			Message: "upgrade_status enabled",
		})
	}

	// 7. Core readiness check.
	coreResults, _ := checkCoreReadiness(cwd)
	for _, cr := range coreResults {
		results = append(results, cr)
		if !cr.Pass {
			allPass = false
		}
	}

	// Output results.
	environmentFailures, readinessFailures := categorize(results)
	_ = allPass

	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))

	if environmentFailures > 0 {
		return fmt.Errorf("preflight: %d environment check(s) failed — the pipeline cannot run until they are fixed", environmentFailures)
	}
	if readinessFailures > 0 {
		// Outstanding upgrade work is the normal state of a project about to
		// be upgraded. Gating on it would make the pipeline wait for its own
		// later stages.
		fmt.Printf("Environment ready. %d readiness item(s) remain — that is the work the pipeline performs.\n", readinessFailures)
		return nil
	}
	fmt.Println("All preflight checks passed.")
	return nil
}

// detectDrupalVersion parses composer.lock to find the drupal/core version.
func detectDrupalVersion(projectPath string) string {
	lockFile := filepath.Join(projectPath, "composer.lock")
	data, err := os.ReadFile(lockFile)
	if err != nil {
		return ""
	}

	var lock map[string]interface{}
	if err := json.Unmarshal(data, &lock); err != nil {
		return ""
	}

	packages, ok := lock["packages"].([]interface{})
	if !ok {
		return ""
	}

	for _, p := range packages {
		pkg, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if pkg["name"] == "drupal/core" {
			if v, ok := pkg["version"].(string); ok {
				return v
			}
		}
	}
	return ""
}

// checkCoreReadiness verifies that composer.json constraints and custom module/theme
// core_version_requirement values allow Drupal 11.
func checkCoreReadiness(projectPath string) ([]PreflightResult, error) {
	var results []PreflightResult

	// 1. Check composer.json drupal/core constraint.
	composerPath := filepath.Join(projectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		results = append(results, PreflightResult{
			Check:   "core_composer_constraint",
			Pass:    false,
			Message: "composer.json not found",
		})
		return results, nil
	}

	var composer map[string]interface{}
	if err := json.Unmarshal(composerData, &composer); err != nil {
		results = append(results, PreflightResult{
			Check:   "core_composer_constraint",
			Pass:    false,
			Message: "composer.json parse error",
		})
		return results, nil
	}

	require, _ := composer["require"].(map[string]interface{})
	coreConstraint := ""
	for _, pkg := range []string{"drupal/core", "drupal/core-recommended"} {
		if c, ok := require[pkg].(string); ok {
			coreConstraint = c
			break
		}
	}

	if coreConstraint == "" {
		results = append(results, PreflightResult{
			Check:   "core_composer_constraint",
			Pass:    false,
			Message: "drupal/core not found in composer.json require",
		})
		return results, nil
	}

	// Check if constraint allows Drupal 11.
	d11Version, _ := semver.Parse("11.0.0")
	if semver.Satisfies(d11Version, coreConstraint) {
		results = append(results, PreflightResult{
			Check:   "core_composer_constraint",
			Pass:    true,
			Message: fmt.Sprintf("composer.json constraint %q allows Drupal 11", coreConstraint),
		})
	} else {
		results = append(results, PreflightResult{
			Check:   "core_composer_constraint",
			Pass:    false,
			Message: fmt.Sprintf("composer.json constraint %s does not permit Drupal 11", coreConstraint),
		})
	}

	// 2. Scan custom modules/themes for core_version_requirement.
	drupalRoot := resolveDrupalRoot(projectPath)
	customModulesDir := filepath.Join(drupalRoot, "modules", "custom")
	customThemesDir := filepath.Join(drupalRoot, "themes", "custom")

	infoFiles := findInfoYMLFiles(customModulesDir)
	infoFiles = append(infoFiles, findInfoYMLFiles(customThemesDir)...)

	if len(infoFiles) == 0 {
		results = append(results, PreflightResult{
			Check:   "core_module_compat",
			Pass:    true,
			Message: "no custom code to check",
		})
		return results, nil
	}

	var blockers []string
	for _, infoFile := range infoFiles {
		data, err := os.ReadFile(infoFile)
		if err != nil {
			continue
		}
		constraint := parseCoreVersionRequirementFromInfo(string(data))
		if constraint == "" {
			continue
		}
		if !semver.Satisfies(d11Version, constraint) {
			// Extract module/theme name from path.
			name := filepath.Base(filepath.Dir(infoFile))
			blockers = append(blockers, fmt.Sprintf("%s (constraint: %s)", name, constraint))
		}
	}

	if len(blockers) > 0 {
		results = append(results, PreflightResult{
			Check:   "core_module_compat",
			Pass:    false,
			Message: fmt.Sprintf("blockers: %s", strings.Join(blockers, ", ")),
		})
	} else {
		results = append(results, PreflightResult{
			Check:   "core_module_compat",
			Pass:    true,
			Message: fmt.Sprintf("all %d custom modules/themes allow Drupal 11", len(infoFiles)),
		})
	}

	return results, nil
}

// findInfoYMLFiles finds all .info.yml files in subdirectories of dir.
func findInfoYMLFiles(dir string) []string {
	var files []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() {
			infoPath := filepath.Join(dir, e.Name(), e.Name()+".info.yml")
			if _, err := os.Stat(infoPath); err == nil {
				files = append(files, infoPath)
			}
		}
	}
	return files
}

// parseCoreVersionRequirementFromInfo extracts core_version_requirement from .info.yml content.
func parseCoreVersionRequirementFromInfo(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "core_version_requirement:") {
			val := strings.TrimPrefix(line, "core_version_requirement:")
			val = strings.TrimSpace(val)
			val = strings.Trim(val, `"'`)
			return val
		}
	}
	return ""
}

// UpgradeCoreResult is the JSON output of RunUpgradeCore.
type UpgradeCoreResult struct {
	CurrentConstraint string `json:"current_constraint"`
	TargetConstraint  string `json:"target_constraint"`
	DryRun            bool   `json:"dry_run"`
	Backup            string `json:"backup,omitempty"`
	Checkpoint        string `json:"checkpoint,omitempty"`
	ComposerExit      int    `json:"composer_exit,omitempty"`
	DrushUpdbExit     int    `json:"drush_updb_exit,omitempty"`
	VerifiedVersion   string `json:"verified_version,omitempty"`
	Success           bool   `json:"success"`
	AlreadyAtTarget   bool   `json:"already_at_target,omitempty"`
}

// RunUpgradeCore performs a deterministic Drupal core version upgrade.
// It parses target version + --dry-run flag, calls coreupgrade.Apply,
// then runs composer require, drush updb, and drush status verify.
func RunUpgradeCore(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: drup upgrade-core <target-version> [--dry-run]")
	}

	targetVersion := ""
	dryRun := false
	for _, arg := range args {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "-"):
			continue
		default:
			if targetVersion == "" {
				targetVersion = arg
			}
		}
	}

	if targetVersion == "" {
		return fmt.Errorf("usage: drup upgrade-core <target-version> [--dry-run]")
	}

	cwd, err := getwdFn()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}

	// Validate project path (security: absolute path, no traversal).
	if err := coreupgrade.ValidateProjectPath(cwd); err != nil {
		return err
	}

	composerPath := filepath.Join(cwd, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return fmt.Errorf("composer.json not found in %s: %w", cwd, err)
	}

	// Parse current constraint.
	var composerDoc map[string]json.RawMessage
	if err := json.Unmarshal(composerData, &composerDoc); err != nil {
		return fmt.Errorf("parse composer.json: %w", err)
	}
	var require map[string]string
	if raw, ok := composerDoc["require"]; ok {
		json.Unmarshal(raw, &require)
	}

	currentConstraint := ""
	for _, pkg := range []string{"drupal/core-recommended", "drupal/core"} {
		if c, ok := require[pkg]; ok {
			currentConstraint = c
			break
		}
	}

	targetMajor, _ := coreupgrade.MajorVersion(targetVersion)
	targetConstraint := fmt.Sprintf("^%d.0", targetMajor)

	result := &UpgradeCoreResult{
		CurrentConstraint: currentConstraint,
		TargetConstraint:  targetConstraint,
		DryRun:            dryRun,
	}

	// Check if already at target.
	if currentConstraint == targetConstraint {
		result.AlreadyAtTarget = true
		result.Success = true
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println("already at target version")
		fmt.Println(string(data))
		return nil
	}

	// Check for clean working tree (unless dry-run).
	if !dryRun {
		clean, dirtyFiles, err := isCleanFn(cwd)
		if err != nil {
			return fmt.Errorf("check git status: %w", err)
		}
		if !clean {
			return fmt.Errorf("working tree is dirty; commit or stash changes first: %s", strings.Join(dirtyFiles, ", "))
		}
	}

	// Call coreupgrade.Apply for the composer.json mutation.
	applyResult, err := coreupgrade.Apply(cwd, targetVersion, dryRun)
	if err != nil {
		return fmt.Errorf("core upgrade apply: %w", err)
	}
	if !applyResult.Success {
		if applyResult.RollbackCheckpoint == "" && !dryRun {
			return fmt.Errorf("core upgrade failed: %s", applyResult.Report)
		}
		if applyResult.RollbackCheckpoint != "" {
			return fmt.Errorf("core upgrade failed (checkpoint: %s): %s", applyResult.RollbackCheckpoint, applyResult.Report)
		}
	}

	result.Checkpoint = applyResult.RollbackCheckpoint
	result.Backup = "composer.json.bak"

	if dryRun {
		result.Success = true
		data, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Create backup (kept on failure for rollback, removed on success).
	backupPath := composerPath + ".bak"
	os.WriteFile(backupPath, composerData, 0o644)

	// Disable advisory blocking before require.
	_, stderr, exitCode, err := cliRun(cwd, "composer", "config", "policy.advisories.block", "false")
	if err != nil {
		return fmt.Errorf("composer config failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("composer config failed (exit %d): %s", exitCode, stderr)
	}

	// Run composer require with -W and --no-update.
	composerArgs := []string{
		"require",
		fmt.Sprintf("drupal/core-recommended:%s", targetConstraint),
		fmt.Sprintf("drupal/core-composer-scaffold:%s", targetConstraint),
		fmt.Sprintf("drupal/core-project-message:%s", targetConstraint),
		"-W",
		"--no-update",
	}
	_, stderr, exitCode, err = cliRun(cwd, "composer", composerArgs...)
	if err != nil {
		return fmt.Errorf("composer not found or failed: %w", err)
	}
	result.ComposerExit = exitCode
	if exitCode != 0 {
		return fmt.Errorf("composer require failed (exit %d): %s", exitCode, stderr)
	}

	// Run composer update -W for full dependency resolution.
	_, stderr, exitCode, err = cliRun(cwd, "composer", "update", "-W")
	if err != nil {
		return fmt.Errorf("composer update failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("composer update failed (exit %d): %s", exitCode, stderr)
	}

	// Run drush updb.
	_, stderr, exitCode, err = cliRun(cwd, "drush", "updb", "-y", "--root="+cwd)
	if err != nil {
		return fmt.Errorf("drush not found or failed: %w", err)
	}
	result.DrushUpdbExit = exitCode
	if exitCode != 0 {
		return fmt.Errorf("drush updb failed (checkpoint: %s, exit %d): %s", applyResult.RollbackCheckpoint, exitCode, stderr)
	}

	// Verify with drush status.
	stdout, stderr, exitCode, err := cliRun(cwd, "drush", "status", "--format=json", "--root="+cwd)
	if err != nil {
		return fmt.Errorf("drush status failed: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("drush status failed (exit %d): %s", exitCode, stderr)
	}

	// Parse drush status output for version verification.
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &status); err == nil {
		if drupalVersion, ok := status["drupal-version"].(string); ok {
			result.VerifiedVersion = drupalVersion
		}
	}

	// Verify the resulting Drupal version matches the target.
	if result.VerifiedVersion != "" {
		verifiedMajor, err := coreupgrade.MajorVersion(result.VerifiedVersion)
		if err == nil && verifiedMajor != targetMajor {
			return fmt.Errorf("version mismatch: expected Drupal %d.x, got %s (major %d)",
				targetMajor, result.VerifiedVersion, verifiedMajor)
		}
	}

	result.Success = true

	// Remove backup on success only — keep on failure for rollback per spec.
	os.Remove(backupPath)

	data, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(data))
	return nil
}

// detectPHPVersion detects the PHP version using cliRun.
func detectPHPVersion(projectPath string) (string, error) {
	stdout, _, exitCode, err := cliRun(projectPath, "php", "-r", "echo PHP_VERSION;")
	if err != nil {
		return "", fmt.Errorf("detect PHP version: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("php command exited with code %d", exitCode)
	}
	return strings.TrimSpace(stdout), nil
}

// isPHP84OrLater checks if the PHP version is 8.4 or later.
func isPHP84OrLater(version string) bool {
	v, err := semver.Parse(version)
	if err != nil {
		return false
	}
	return semver.Satisfies(v, ">=8.4")
}

// patchSettingsPHP patches settings.php to suppress E_DEPRECATED warnings for PHP 8.4+.
func patchSettingsPHP(projectPath string) error {
	settingsPath := filepath.Join(projectPath, "web", "sites", "default", "settings.php")

	// Read current content
	content, err := os.ReadFile(settingsPath)
	if err != nil {
		return fmt.Errorf("read settings.php: %w", err)
	}

	contentStr := string(content)
	suppressionLine := "error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED);"

	// Check if already patched (idempotent)
	if strings.Contains(contentStr, suppressionLine) {
		return nil
	}

	// Create backup
	backupPath := settingsPath + ".bak"
	if err := os.WriteFile(backupPath, content, 0o644); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	// Find DDEV include block end or EOF
	ddevBlockEnd := strings.Index(contentStr, "if (file_exists(__DIR__ . '/settings.ddev.php')) {")
	insertPos := len(contentStr)

	if ddevBlockEnd != -1 {
		// Find the closing brace of the DDEV block
		blockStart := ddevBlockEnd
		braceCount := 0
		inBlock := false
		for i := blockStart; i < len(contentStr); i++ {
			if contentStr[i] == '{' {
				braceCount++
				inBlock = true
			} else if contentStr[i] == '}' {
				braceCount--
				if inBlock && braceCount == 0 {
					// Found the end of the DDEV block
					insertPos = i + 1
					break
				}
			}
		}
	}

	// Insert suppression line after DDEV block
	newContent := contentStr[:insertPos] + "\n" + suppressionLine + "\n" + contentStr[insertPos:]

	// Write updated content
	if err := os.WriteFile(settingsPath, []byte(newContent), 0o644); err != nil {
		return fmt.Errorf("write settings.php: %w", err)
	}

	return nil
}

// RunUninstall removes drup from all installed agents.
func RunUninstall(args []string) error {
	// Parse flags manually (matching existing pattern).
	dryRun := false
	force := false
	for _, arg := range args {
		switch arg {
		case "--dry-run":
			dryRun = true
		case "--force":
			force = true
		}
	}

	// Load state.
	s, err := stateLoadFn()
	if err != nil {
		if force {
			fmt.Fprintf(os.Stderr, "Warning: could not load state: %v\n", err)
			fmt.Fprintln(os.Stderr, "Proceeding with --force...")
			s = &statepkg.State{}
		} else {
			return fmt.Errorf("load state: %w (use --force to override)", err)
		}
	}

	// Check if state is empty.
	if len(s.InstalledAgents) == 0 {
		if force {
			fmt.Fprintln(os.Stderr, "Warning: no agents in state, but proceeding with --force...")
		} else {
			return fmt.Errorf("no agents installed — state is empty (use --force to override)")
		}
	}

	// Build adapter list from state.
	home, err := osUserHomeDirFn()
	if err != nil {
		return fmt.Errorf("get home directory: %w", err)
	}

	var adapters []installer.AgentAdapter
	for _, agentID := range s.InstalledAgents {
		switch agentID {
		case "claude":
			adapters = append(adapters, &installer.ClaudeAdapter{HomeDir: home})
		case "opencode":
			adapters = append(adapters, &installer.OpenCodeAdapter{HomeDir: home})
		case "codex":
			adapters = append(adapters, &installer.CodexAdapter{HomeDir: home})
		}
	}

	if len(adapters) == 0 && !force {
		return fmt.Errorf("no valid adapters found in state")
	}

	// Confirmation prompt (skip in dry-run or force mode).
	if !dryRun && !force {
		fmt.Println("This will remove drup from the following agents:")
		for _, agent := range adapters {
			fmt.Printf("  - %s\n", agent.ID())
		}
		fmt.Println("\nState directory (~/.config/drup/) will be removed, including the config backups drup took before editing your agent configs.")
		fmt.Print("\nContinue? [y/N] ")

		var response string
		fmt.Scanln(&response)
		if response != "y" && response != "Y" {
			fmt.Println("Uninstall cancelled.")
			return nil
		}
	}

	// Uninstall from adapters.
	paths, err := installer.Uninstall(adapters, dryRun)
	if err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}

	if dryRun {
		fmt.Println("Dry-run mode — the following would be removed:")
		for _, path := range paths {
			fmt.Printf("  %s\n", path)
		}
		return nil
	}

	// Remove state directory.
	if err := stateRemoveFn(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not remove state directory: %v\n", err)
	}

	// Attempt binary self-removal.
	executable, err := osExecutableFn()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not determine executable path: %v\n", err)
	} else {
		if err := os.Remove(executable); err != nil {
			fmt.Fprintf(os.Stderr, "Could not remove binary %s: %v\n", executable, err)
			fmt.Fprintf(os.Stderr, "Please remove it manually: rm %s\n", executable)
		}
	}

	fmt.Println("Uninstall complete.")
	return nil
}
