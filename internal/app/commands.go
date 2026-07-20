package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"drup/internal/drupalorg"
	"drup/internal/report"
	"drup/internal/scan"
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
	fmt.Fprintln(os.Stderr, "mcp: not yet implemented")
	return nil
}

// RunInstall detects agents and writes skill files.
func RunInstall() error {
	fmt.Fprintln(os.Stderr, "install: not yet implemented")
	return nil
}

// RunSync re-applies agent assets.
func RunSync() error {
	fmt.Fprintln(os.Stderr, "sync: not yet implemented")
	return nil
}

// RunUpgrade self-updates the binary.
func RunUpgrade() error {
	fmt.Fprintln(os.Stderr, "upgrade: not yet implemented")
	return nil
}

// Unused import guard — scan is used by RunScan in the real implementation.
var _ = scan.ClassContrib
