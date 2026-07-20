package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"drup/internal/drupalorg"
	"drup/internal/installer"
	"drup/internal/mcp"
	"drup/internal/packaging"
	"drup/internal/report"
	"drup/internal/scan"
	statepkg "drup/internal/state"
	"drup/internal/update"
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
	// In a real implementation, this would exec drush upgrade_status:analyze.
	// For now, return a placeholder.
	fmt.Fprintf(os.Stderr, "scan: would analyze %s\n", path)
	return nil
}

// RunFix runs drupal-rector on the target project.
func RunFix(path string) error {
	fmt.Fprintf(os.Stderr, "fix: would run drupal-rector on %s\n", path)
	return nil
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
	fmt.Printf("Download: %s\n", assetURL)
	// Full download + replace would go here.
	return nil
}

// Unused import guard — scan is used by RunScan in the real implementation.
var _ = scan.ClassContrib
