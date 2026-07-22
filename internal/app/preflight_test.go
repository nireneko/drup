package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	drupexec "github.com/nireneko/drup/internal/exec"
)

func TestDetectDrupalVersion(t *testing.T) {
	dir := t.TempDir()

	// Create a composer.lock with drupal/core.
	lock := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{
				"name":    "drupal/core",
				"version": "10.3.0",
			},
			map[string]interface{}{
				"name":    "drupal/token",
				"version": "1.15.0",
			},
		},
	}
	data, _ := json.Marshal(lock)
	os.WriteFile(filepath.Join(dir, "composer.lock"), data, 0o644)

	version := detectDrupalVersion(dir)
	if version != "10.3.0" {
		t.Errorf("detectDrupalVersion = %q, want %q", version, "10.3.0")
	}
}

func TestDetectDrupalVersion_NoLock(t *testing.T) {
	dir := t.TempDir()
	version := detectDrupalVersion(dir)
	if version != "" {
		t.Errorf("detectDrupalVersion = %q, want empty", version)
	}
}

func TestDetectDrupalVersion_NoCore(t *testing.T) {
	dir := t.TempDir()
	lock := map[string]interface{}{
		"packages": []interface{}{
			map[string]interface{}{
				"name":    "drupal/token",
				"version": "1.15.0",
			},
		},
	}
	data, _ := json.Marshal(lock)
	os.WriteFile(filepath.Join(dir, "composer.lock"), data, 0o644)

	version := detectDrupalVersion(dir)
	if version != "" {
		t.Errorf("detectDrupalVersion = %q, want empty", version)
	}
}

// --- Phase 5: RED tests for config conflict handling ---

func TestRunPreflight_DeletesUpdateSettingsBeforeEnable(t *testing.T) {
	// This test verifies that RunPreflight calls drush config:delete update.settings
	// before drush en upgrade_status.
	// Note: RunPreflight uses drupexec.Run directly, not execRunFn, so we need to
	// override drupexec.Run for this test.
	origRun := drupexec.Run
	var drushCalls [][]string
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			drushCalls = append(drushCalls, args)
		}
		// Return success for all calls.
		return "", "", 0, nil
	}
	defer func() { drupexec.Run = origRun }()

	// Create a minimal project structure.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core":"^10.3"}}`), 0o644)
	os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"packages":[{"name":"drupal/core","version":"10.3.0"}]}`), 0o644)

	// Change to the temp dir for the test.
	origWd, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origWd)

	// Run preflight (will fail because git/composer/drush aren't real, but we just want to capture the drush calls).
	_ = RunPreflight()

	// Find the config:delete and en calls.
	configDeleteIdx := -1
	enIdx := -1
	for i, args := range drushCalls {
		if len(args) > 0 && args[0] == "config:delete" {
			configDeleteIdx = i
		}
		if len(args) > 0 && args[0] == "en" {
			enIdx = i
		}
	}

	// Verify config:delete was called before en.
	if configDeleteIdx == -1 {
		t.Error("drush config:delete was not called")
	}
	if enIdx == -1 {
		t.Error("drush en was not called")
	}
	if configDeleteIdx >= 0 && enIdx >= 0 && configDeleteIdx >= enIdx {
		t.Errorf("drush config:delete (idx %d) should be called before drush en (idx %d)", configDeleteIdx, enIdx)
	}

	// Verify config:delete targets update.settings.
	if configDeleteIdx >= 0 {
		args := drushCalls[configDeleteIdx]
		foundUpdateSettings := false
		for _, arg := range args {
			if arg == "update.settings" {
				foundUpdateSettings = true
				break
			}
		}
		if !foundUpdateSettings {
			t.Errorf("drush config:delete args = %v, want 'update.settings' present", args)
		}
	}
}
