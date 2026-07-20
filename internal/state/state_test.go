package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Override configDir for testing.
	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	// Save a state.
	s := &State{
		Version:         "0.1.0",
		InstalledAgents: []string{"claude", "opencode"},
		PendingSync:     true,
	}
	if err := Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Load it back.
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	if loaded.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", loaded.Version, "0.1.0")
	}
	if len(loaded.InstalledAgents) != 2 {
		t.Errorf("len(InstalledAgents) = %d, want 2", len(loaded.InstalledAgents))
	}
	if !loaded.PendingSync {
		t.Error("PendingSync = false, want true")
	}
}

func TestLoad_NoFile(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	// Load when no file exists should return default state.
	s, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if s.Version != "" {
		t.Errorf("Version = %q, want empty", s.Version)
	}
}

func TestStatePath(t *testing.T) {
	dir := t.TempDir()
	path := statePath(dir)
	expected := filepath.Join(dir, "drup", "state.json")
	if path != expected {
		t.Errorf("statePath = %q, want %q", path, expected)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "nested")

	orig := configDir
	configDir = func() (string, error) { return subDir, nil }
	defer func() { configDir = orig }()

	s := &State{Version: "0.1.0"}
	if err := Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	// Verify directory was created.
	if _, err := os.Stat(filepath.Join(subDir, "drup")); os.IsNotExist(err) {
		t.Error("drup directory not created")
	}
}
