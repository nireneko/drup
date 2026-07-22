package app

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nireneko/drup/internal/drupalorg"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/installer"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/packaging"
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

// RunScan runs upgrade_status:analyze and outputs structured JSON.
func RunScan(path string) error {
	stdout, stderr, exitCode, err := drupexec.Run("drush", "-r", path, "upgrade_status:analyze", "--format=json")
	if err != nil {
		return fmt.Errorf("exec drush: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("drush upgrade_status:analyze exit %d: %s", exitCode, stderr)
	}

	result, err := scan.Parse(strings.NewReader(stdout))
	if err != nil {
		return fmt.Errorf("parse scan output: %w", err)
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

	// 6. Enable upgrade_status module.
	fmt.Println("Enabling upgrade_status module...")
	_, stderr, exitCode, err := drupexec.Run("drush", "en", "upgrade_status", "-y")
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
