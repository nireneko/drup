package app

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/nireneko/drup/internal/coreupgrade"
	"github.com/nireneko/drup/internal/drupalorg"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/installer"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/packaging"
	"github.com/nireneko/drup/internal/patch"
	"github.com/nireneko/drup/internal/report"
	"github.com/nireneko/drup/internal/scan"
	statepkg "github.com/nireneko/drup/internal/state"
	"github.com/nireneko/drup/internal/update"
)

// RunInit verifies the project is a valid Drupal project.
func RunInit(args []string) error {
	cwd, err := os.Getwd()
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
	if _, ok := require["drupal/core"]; !ok {
		return fmt.Errorf("not a Drupal project: drupal/core not found in composer.json require")
	}

	fmt.Println("Drupal project initialized successfully.")
	return nil
}

// isScanExitOK returns true for exit codes that carry valid scan data.
// 0 = no findings, 3 = findings exist. 1, 2, >3 = real errors.
func isScanExitOK(exitCode int) bool {
	return exitCode == 0 || exitCode == 3
}

// cliRun detects the environment for projectPath and runs cmd with the
// appropriate prefix. Uses --root= instead of -r for drush commands.
// Returns the same (stdout, stderr, exitCode, err) as drupexec.Run.
func cliRun(projectPath string, cmd string, args ...string) (string, string, int, error) {
	detection, err := defaultEnvDetector.Detect(projectPath, false)
	if err != nil {
		return "", "", -1, fmt.Errorf("detect environment: %w", err)
	}
	return drupexec.RunWithEnv(detection.CommandPrefix, cmd, args...)
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

// RunScan runs upgrade_status:analyze and outputs structured JSON.
func RunScan(path string) error {
	stdout, stderr, exitCode, err := cliRun(path, "drush", "upgrade_status:analyze", "--all", "--root="+path)
	if err != nil {
		return drushExecError("drush", []string{"upgrade_status:analyze", "--all", "--root=" + path}, -1, err.Error(), "")
	}
	if !isScanExitOK(exitCode) {
		return drushExecError("drush", []string{"upgrade_status:analyze", "--all", "--root=" + path}, exitCode, stderr, stdout)
	}

	// Exit code 3 with empty stdout means drush crashed, not findings.
	if exitCode == 3 && strings.TrimSpace(stdout) == "" {
		return fmt.Errorf("drush exited with code 3 but produced no output (command: drush upgrade_status:analyze --all --root=%s)\nstderr: %s", path, stderr)
	}

	result, err := scan.Parse(strings.NewReader(stdout))
	if err != nil {
		return fmt.Errorf("parse scan output (command: drush upgrade_status:analyze --all --root=%s): %w\nstdout (truncated): %.500s", path, err, stdout)
	}

	result.ProjectPath = path
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// RunFix runs drupal-rector on the target project.
func RunFix(path string) error {
	// Run rector on custom modules and themes.
	customModules := filepath.Join(path, "modules", "custom")
	themes := filepath.Join(path, "themes")

	targets := []string{}
	for _, dir := range []string{customModules, themes} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			targets = append(targets, dir)
		}
	}

	if len(targets) == 0 {
		return fmt.Errorf("no custom modules or themes directories found in %s", path)
	}

	args := append([]string{"process"}, targets...)
	args = append(args, "--config="+filepath.Join(path, "rector.php"))

	stdout, stderr, exitCode, err := drupexec.Run(filepath.Join(path, "vendor", "bin", "rector"), args...)
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
	patches, err := drupalorg.SearchPatches(query)
	if err != nil {
		return fmt.Errorf("search patches: %w", err)
	}

	data, err := json.MarshalIndent(patches, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	fmt.Println(string(data))
	return nil
}

// RunReport generates JSON and markdown reports.
func RunReport(path string) error {
	// Placeholder — in a real implementation, this would gather scan results.
	reportData := &report.ReportData{
		ProjectPath: path,
		TotalErrors: 0,
		Resolved:    []report.ResolvedItem{},
		Pending:     []report.PendingItem{},
	}

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

// RunMCP starts the MCP stdio server.
func RunMCP() error {
	server := mcp.NewServer(os.Stdout, Version)
	WireMCPTools(server)
	return server.Run()
}

// DoValidate runs upgrade_status:analyze and returns parsed results.
// Shared between CLI and MCP handlers.
func DoValidate(projectPath, module string) (*scan.ScanResult, []scan.DepError, error) {
	analyzeTarget := "--all"
	if module != "" {
		analyzeTarget = module
	}

	stdout, stderr, exitCode, err := cliRun(projectPath, "drush", "upgrade_status:analyze", analyzeTarget, "--root="+projectPath)
	if err != nil {
		return nil, nil, drushExecError("drush", []string{"upgrade_status:analyze", analyzeTarget, "--root=" + projectPath}, -1, err.Error(), "")
	}
	if !isScanExitOK(exitCode) {
		return nil, nil, drushExecError("drush", []string{"upgrade_status:analyze", analyzeTarget, "--root=" + projectPath}, exitCode, stderr, stdout)
	}

	// Exit code 3 with empty stdout means drush crashed, not findings.
	if exitCode == 3 && strings.TrimSpace(stdout) == "" {
		return nil, nil, fmt.Errorf("drush exited with code 3 but produced no output (command: drush upgrade_status:analyze %s --root=%s)\nstderr: %s", analyzeTarget, projectPath, stderr)
	}

	result, err := scan.Parse(strings.NewReader(stdout))
	if err != nil {
		return nil, nil, fmt.Errorf("parse scan output (command: drush upgrade_status:analyze %s --root=%s): %w\nstdout (truncated): %.500s", projectPath, analyzeTarget, err, stdout)
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

// RunValidate runs upgrade_status:analyze and outputs JSON with error count.
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
// Shared between CLI and MCP handlers.
func DoApplyPatch(patchURL, projectPath string) (*patch.ApplyResult, error) {
	return patch.Apply(patchURL, projectPath, "", "")
}

// RunApplyPatch downloads and applies a patch, outputting JSON result.
func RunApplyPatch(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: drup apply-patch <url> <path>")
	}

	patchURL := args[0]
	projectPath := args[1]

	result, err := DoApplyPatch(patchURL, projectPath)
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

	// Render templates for each detected agent.
	for _, agent := range agents {
		files, err := packaging.Render(agent.ID(), binaryPath)
		if err != nil {
			return fmt.Errorf("render templates for %s: %w", agent.ID(), err)
		}
		if err := installer.Install([]installer.AgentAdapter{agent}, binaryPath, files); err != nil {
			return fmt.Errorf("install to %s: %w", agent.ID(), err)
		}
		fmt.Printf("Installed drup to %s\n", agent.ID())
	}

	// Update state.
	s, _ := statepkg.Load()
	agentIDs := make([]string, len(agents))
	for i, a := range agents {
		agentIDs[i] = a.ID()
	}
	s.InstalledAgents = agentIDs
	s.Version = Version
	if err := statepkg.Save(s); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

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
	for _, agent := range agents {
		files, err := packaging.Render(agent.ID(), binaryPath)
		if err != nil {
			return fmt.Errorf("render templates for %s: %w", agent.ID(), err)
		}
		if err := installer.Install([]installer.AgentAdapter{agent}, binaryPath, files); err != nil {
			return fmt.Errorf("sync to %s: %w", agent.ID(), err)
		}
		fmt.Printf("Synced drup to %s\n", agent.ID())
	}

	// Clear PendingSync flag.
	s.PendingSync = false
	if err := statepkg.Save(s); err != nil {
		return fmt.Errorf("save state: %w", err)
	}

	return nil
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
var execRunFn = drupexec.Run

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

// PreflightResult holds the outcome of each preflight check.
type PreflightResult struct {
	Check   string `json:"check"`
	Pass    bool   `json:"pass"`
	Message string `json:"message"`
}

// RunPreflight checks project readiness for upgrade automation.
func RunPreflight() error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
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

	// 2. Check git clean.
	clean, files, err := gitops.IsClean(cwd)
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

		// Install the dev dependency.
		fmt.Printf("Installing %s...\n", dep.Pkg)
		_, stderr, exitCode, err := drupexec.Run("composer", "require", "--dev", dep.Pkg)
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
				results = append(results, PreflightResult{
					Check:   "php84_compat",
					Pass:    true,
					Message: "settings.php patched to suppress E_DEPRECATED",
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
		results = append(results, PreflightResult{
			Check:   "enable_upgrade_status",
			Pass:    true,
			Message: "upgrade_status enabled",
		})
	}

	// Output results.
	data, _ := json.MarshalIndent(results, "", "  ")
	fmt.Println(string(data))

	if !allPass {
		return fmt.Errorf("preflight: some checks failed")
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
	_, stderr, exitCode, err := execRunFn("composer", "config", "policy.advisories.block", "false")
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
	_, stderr, exitCode, err = execRunFn("composer", composerArgs...)
	if err != nil {
		return fmt.Errorf("composer not found or failed: %w", err)
	}
	result.ComposerExit = exitCode
	if exitCode != 0 {
		return fmt.Errorf("composer require failed (exit %d): %s", exitCode, stderr)
	}

	// Run composer update -W for full dependency resolution.
	_, stderr, exitCode, err = execRunFn("composer", "update", "-W")
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

func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)
		if f.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		outFile, err := os.Create(target)
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}
		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
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
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}
	return major > 8 || (major == 8 && minor >= 4)
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
		fmt.Println("\nState directory (~/.config/drup/) will be removed.")
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
