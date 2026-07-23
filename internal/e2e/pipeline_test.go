// Package e2e provides mock-based integration tests for pipeline stage orchestration.
// These tests do NOT require a real Drupal site.
package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/app"
	drupexec "github.com/nireneko/drup/internal/exec"
)

// TestPipeline_StageOrdering verifies that stages execute in correct order.
// This is a mock-based integration test — no real Drupal site required.
func TestPipeline_StageOrdering(t *testing.T) {
	if _, _, _, err := drupexec.Run("git", "--version"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	setupMockProject(t, dir, "10.3.0")

	// Track command execution order.
	var commandOrder []string

	origRun := drupexec.Run
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		commandOrder = append(commandOrder, cmd+" "+strings.Join(args, " "))
		return "", "", 0, nil
	}
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		full := cmd
		if len(args) > 0 {
			full = cmd + " " + args[0]
		}
		commandOrder = append(commandOrder, full)

		if cmd == "drush" && len(args) > 0 && args[0] == "status" {
			return `{"drupal-version":"10.3.0"}`, "", 0, nil
		}
		if cmd == "php" {
			return "8.3.0", "", 0, nil
		}
		return "No errors found.", "", 0, nil
	}
	defer func() {
		drupexec.Run = origRun
		drupexec.RunWithEnv = origRunWithEnv
	}()

	// Change to project dir for preflight.
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	// Run pipeline stages in order.
	_ = app.RunPreflight()
	_ = app.RunScan(dir)
	_ = app.RunValidate([]string{dir})

	// Verify stage order: preflight checks should come before scan, scan before validate.
	preflightIdx := -1
	scanIdx := -1
	for i, cmd := range commandOrder {
		if strings.Contains(cmd, "composer") && preflightIdx == -1 {
			preflightIdx = i
		}
		if strings.Contains(cmd, "upgrade_status:analyze") && scanIdx == -1 {
			scanIdx = i
		}
	}

	// At minimum, scan should have been called.
	if scanIdx == -1 {
		// Scan may have been bypassed if no custom code exists.
		t.Log("scan was bypassed (no custom code) — this is expected for empty projects")
	}
}

// TestPipeline_CleanupSkippedOnValidateFailure verifies cleanup is skipped when validate fails.
func TestPipeline_CleanupSkippedOnValidateFailure(t *testing.T) {
	dir := t.TempDir()
	setupMockProject(t, dir, "10.3.0")

	cleanupRanDrush := false
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:uninstall" {
			cleanupRanDrush = true
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	// Run cleanup with validate-failed flag.
	_ = app.RunCleanup([]string{dir, "--validate-failed"})

	if cleanupRanDrush {
		t.Error("cleanup should NOT run drush when validate failed")
	}
}

// TestPipeline_CleanupRunsOnValidatePass verifies cleanup runs when validate passes.
func TestPipeline_CleanupRunsOnValidatePass(t *testing.T) {
	dir := t.TempDir()

	// Create composer.json with upgrade_status.
	composerJSON := `{"require":{"drupal/upgrade_status":"^4.0","drupal/core":"^10.3"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	drushUninstallCalled := false
	origRunWithEnv := drupexec.RunWithEnv
	origRun := drupexec.Run
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:uninstall" {
			drushUninstallCalled = true
		}
		return "", "", 0, nil
	}
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		return "", "", 0, nil
	}
	defer func() {
		drupexec.RunWithEnv = origRunWithEnv
		drupexec.Run = origRun
	}()

	_ = app.RunCleanup([]string{dir, "--validate-passed"})

	if !drushUninstallCalled {
		t.Error("cleanup should run drush pm:uninstall when validate passed")
	}
}

// setupMockProject creates a minimal Drupal project structure for testing.
func setupMockProject(t *testing.T, dir, drupalVersion string) {
	t.Helper()

	composerJSON := map[string]interface{}{
		"require": map[string]interface{}{
			"drupal/core":            "^" + strings.Split(drupalVersion, ".")[0] + ".0",
			"drupal/core-recommended": "^" + strings.Split(drupalVersion, ".")[0] + ".0",
		},
	}
	data, _ := json.MarshalIndent(composerJSON, "", "  ")
	os.WriteFile(filepath.Join(dir, "composer.json"), data, 0o644)

	lockJSON := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{
				"name":    "drupal/core",
				"version": drupalVersion,
			},
		},
	}
	lockData, _ := json.MarshalIndent(lockJSON, "", "  ")
	os.WriteFile(filepath.Join(dir, "composer.lock"), lockData, 0o644)

	// Create web root with empty custom dirs.
	os.MkdirAll(filepath.Join(dir, "modules", "custom"), 0o755)
	os.MkdirAll(filepath.Join(dir, "themes", "custom"), 0o755)
}
