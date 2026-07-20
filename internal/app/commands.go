package app

import (
	"encoding/json"
	"fmt"
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
	server := mcp.NewServer(os.Stdout)
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

// RunUpgrade self-updates the binary.
func RunUpgrade() error {
	version, assetURL, err := update.CheckLatest("gentleman-programming", "drup")
	if err != nil {
		return fmt.Errorf("check for updates: %w", err)
	}

	if version == Version {
		fmt.Println("Already up to date.")
		return nil
	}

	fmt.Printf("New version available: %s (current: %s)\n", version, Version)

	// Determine asset filename for current OS/arch.
	goos := os.Getenv("GOOS")
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := os.Getenv("GOARCH")
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	assetName := fmt.Sprintf("drup_%s_%s_%s.tar.gz", version, goos, goarch)

	// Build download and checksum URLs from the asset URL.
	// assetURL is like https://github.com/.../drup_0.2.0_linux_amd64.tar.gz
	// checksums.txt is at the same release: replace asset name with checksums.txt.
	checksumURL := strings.TrimSuffix(assetURL, "/"+assetName) + "/checksums.txt"
	// If the asset URL doesn't contain the asset name, try direct construction.
	if !strings.HasSuffix(assetURL, assetName) {
		checksumURL = strings.TrimSuffix(assetURL, filepath.Ext(assetURL))
		checksumURL = strings.TrimSuffix(checksumURL, filepath.Ext(checksumURL))
		// Fallback: just use the release download base.
		base := assetURL[:strings.LastIndex(assetURL, "/")]
		checksumURL = base + "/checksums.txt"
	}

	fmt.Printf("Downloading %s...\n", assetName)
	tmpPath, err := update.Download(assetURL, checksumURL, assetName)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer os.Remove(tmpPath)

	// Atomic replace: get current binary path, rename tmp over it.
	currentBin, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get current binary path: %w", err)
	}

	// Resolve symlinks.
	currentBin, err = filepath.EvalSymlinks(currentBin)
	if err != nil {
		return fmt.Errorf("resolve symlinks: %w", err)
	}

	// Rename downloaded file to replace current binary.
	if err := os.Rename(tmpPath, currentBin); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	// Ensure executable permission.
	if err := os.Chmod(currentBin, 0o755); err != nil {
		return fmt.Errorf("set executable permission: %w", err)
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
		Pkg  string
		Dev  bool
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


