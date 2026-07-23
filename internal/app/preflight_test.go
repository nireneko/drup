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
	// Note: RunPreflight now uses cliRun which calls drupexec.RunWithEnv.
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var drushCalls [][]string
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			drushCalls = append(drushCalls, args)
		}
		// Return success for all calls.
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

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

// Task 3.6: Core readiness check tests.

func TestCheckCoreReadiness_AllConstraintsAllowD11(t *testing.T) {
	dir := t.TempDir()
	// composer.json allows D11.
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core":"^10.3 || ^11"}}`), 0o644)
	// Custom module allows D11.
	modDir := filepath.Join(dir, "web", "modules", "custom", "mymod")
	os.MkdirAll(modDir, 0o755)
	os.WriteFile(filepath.Join(modDir, "mymod.info.yml"), []byte("name: mymod\ncore_version_requirement: '>=10.0 || ^11'\n"), 0o644)

	results, err := checkCoreReadiness(dir)
	if err != nil {
		t.Fatalf("checkCoreReadiness error: %v", err)
	}
	for _, r := range results {
		if !r.Pass {
			t.Errorf("check %q should pass: %s", r.Check, r.Message)
		}
	}
}

func TestCheckCoreReadiness_ComposerBlocksD11(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core":"^10.3"}}`), 0o644)

	results, err := checkCoreReadiness(dir)
	if err != nil {
		t.Fatalf("checkCoreReadiness error: %v", err)
	}
	foundFail := false
	for _, r := range results {
		if !r.Pass && r.Check == "core_composer_constraint" {
			foundFail = true
			break
		}
	}
	if !foundFail {
		t.Error("expected core_composer_constraint to fail when ^10.3 doesn't allow D11")
	}
}

func TestCheckCoreReadiness_ModuleBlocksD11(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core":"^10.3 || ^11"}}`), 0o644)

	// Module with <11 constraint.
	modDir := filepath.Join(dir, "web", "modules", "custom", "oldmod")
	os.MkdirAll(modDir, 0o755)
	os.WriteFile(filepath.Join(modDir, "oldmod.info.yml"), []byte("name: oldmod\ncore_version_requirement: '<11'\n"), 0o644)

	results, err := checkCoreReadiness(dir)
	if err != nil {
		t.Fatalf("checkCoreReadiness error: %v", err)
	}
	foundBlocker := false
	for _, r := range results {
		if !r.Pass && r.Check == "core_module_compat" {
			foundBlocker = true
			if !contains(r.Message, "oldmod") {
				t.Errorf("message should mention 'oldmod': %s", r.Message)
			}
		}
	}
	if !foundBlocker {
		t.Error("expected core_module_compat to fail for oldmod with <11 constraint")
	}
}

func TestCheckCoreReadiness_NoCustomCode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core":"^10.3 || ^11"}}`), 0o644)

	results, err := checkCoreReadiness(dir)
	if err != nil {
		t.Fatalf("checkCoreReadiness error: %v", err)
	}
	// Should pass with "no custom code to check".
	foundSkip := false
	for _, r := range results {
		if r.Check == "core_module_compat" && contains(r.Message, "no custom") {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Error("expected 'no custom code to check' message when dirs are empty")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
