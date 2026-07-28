package state

import (
	"io"
	"os"
	"path/filepath"
	"strings"
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

func TestLoadSave_ModelAssignments_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	s := &State{
		Version: "0.1.0",
		ModelAssignments: map[string]map[string]ModelPhaseAssignment{
			"claude": {
				"drup-rector": {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
			},
		},
	}
	if err := Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}

	got := loaded.ModelAssignments["claude"]["drup-rector"]
	want := s.ModelAssignments["claude"]["drup-rector"]
	if got != want {
		t.Errorf("ModelAssignments round-trip = %+v, want %+v", got, want)
	}
}

func TestLoad_ModelAssignments_NilByDefault(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	s := &State{Version: "0.1.0"}
	if err := Save(s); err != nil {
		t.Fatalf("Save error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.ModelAssignments != nil {
		t.Errorf("ModelAssignments = %v, want nil (backward compat)", loaded.ModelAssignments)
	}
}

func TestLoad_UnknownJSONKey_Tolerated(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	path := statePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"0.1.0","some_future_key":{"a":1}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.Version != "0.1.0" {
		t.Errorf("Version = %q, want %q", loaded.Version, "0.1.0")
	}
}

func TestLoad_LegacyModelOverrides_WarnedAndDropped(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	path := statePath(dir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{"version":"0.1.0","model_overrides":{"claude":{"drup-rector":"claude-opus-4"}}}`
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	loaded, err := Load()

	w.Close()
	os.Stderr = oldStderr
	var buf strings.Builder
	_, _ = io.Copy(&buf, r)

	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if loaded.ModelAssignments != nil {
		t.Errorf("ModelAssignments = %v, want nil — legacy key must not migrate", loaded.ModelAssignments)
	}
	if !strings.Contains(buf.String(), "model_overrides") {
		t.Errorf("expected a warning about legacy model_overrides, got %q", buf.String())
	}
}

func TestValidateModelValue_RejectsInjectionChars(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"newline", "claude-haiku\n"},
		{"double-quote", `claude"haiku`},
		{"backslash", `claude\haiku`},
		{"hash", "claude#haiku"},
		{"leading whitespace", " claude-haiku"},
		{"trailing whitespace", "claude-haiku "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateModelValue(tt.value); err == nil {
				t.Errorf("ValidateModelValue(%q) accepted invalid value", tt.value)
			}
		})
	}
}

func TestValidateModelValue_AcceptsValidValueAndEmpty(t *testing.T) {
	for _, v := range []string{"", "openrouter/qwen/qwen3-30b-a3b:free", "some-future-model-id"} {
		if err := ValidateModelValue(v); err != nil {
			t.Errorf("ValidateModelValue(%q) rejected a valid value: %v", v, err)
		}
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

func TestRemove_RemovesDirectory(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	// Create state directory with files.
	drupDir := filepath.Join(dir, "drup")
	os.MkdirAll(drupDir, 0o755)
	os.WriteFile(filepath.Join(drupDir, "state.json"), []byte(`{}`), 0o644)
	os.MkdirAll(filepath.Join(drupDir, "backups"), 0o755)
	os.WriteFile(filepath.Join(drupDir, "backups", "test.tar.gz"), []byte("test"), 0o644)

	// Remove it.
	if err := Remove(); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	// Verify directory removed.
	if _, err := os.Stat(drupDir); !os.IsNotExist(err) {
		t.Error("drup directory still exists after Remove")
	}
}

func TestRemove_RemovesLegacyDirectory(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	// Create legacy ~/.drup/ directory.
	legacyDir := filepath.Join(dir, ".drup")
	os.MkdirAll(legacyDir, 0o755)
	os.WriteFile(filepath.Join(legacyDir, "state.json"), []byte(`{}`), 0o644)

	// Remove it.
	if err := Remove(); err != nil {
		t.Fatalf("Remove error: %v", err)
	}

	// Verify legacy directory removed.
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Error("legacy .drup directory still exists after Remove")
	}
}

func TestRemove_Idempotent(t *testing.T) {
	dir := t.TempDir()

	orig := configDir
	configDir = func() (string, error) { return dir, nil }
	defer func() { configDir = orig }()

	// First remove (nothing exists).
	if err := Remove(); err != nil {
		t.Fatalf("first Remove error: %v", err)
	}

	// Second remove (should be idempotent).
	if err := Remove(); err != nil {
		t.Fatalf("second Remove error: %v", err)
	}
}
