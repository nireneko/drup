package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
