package projectconfig

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	got := Load(dir)
	want := Defaults()
	if got != want {
		t.Errorf("Load(missing config) = %+v, want defaults %+v", got, want)
	}
}

func TestLoad_OverridesMutationCaps(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"mutation_cap_per_session": 5, "mutation_cap_per_day": 10}`)

	got := Load(dir)
	if got.MutationCapPerSession != 5 {
		t.Errorf("MutationCapPerSession = %d, want 5", got.MutationCapPerSession)
	}
	if got.MutationCapPerDay != 10 {
		t.Errorf("MutationCapPerDay = %d, want 10", got.MutationCapPerDay)
	}
	// Fields omitted from the file keep their defaults.
	if got.AllowlistMode != "strict" {
		t.Errorf("AllowlistMode = %q, want default %q", got.AllowlistMode, "strict")
	}
}

func TestLoad_OverridesBackupFreshnessWindow(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"backup_freshness_window": "48h"}`)

	got := Load(dir)
	if got.BackupFreshnessWindow != 48*time.Hour {
		t.Errorf("BackupFreshnessWindow = %v, want 48h", got.BackupFreshnessWindow)
	}
}

func TestLoad_OverridesAllowlistMode(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"allowlist_mode": "lenient"}`)

	got := Load(dir)
	if got.AllowlistMode != "lenient" {
		t.Errorf("AllowlistMode = %q, want %q", got.AllowlistMode, "lenient")
	}
}

func TestLoad_MalformedJSONFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{not valid json`)

	got := Load(dir)
	want := Defaults()
	if got != want {
		t.Errorf("Load(malformed config) = %+v, want defaults %+v", got, want)
	}
}

func TestLoad_EmptyFileFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "")

	got := Load(dir)
	want := Defaults()
	if got != want {
		t.Errorf("Load(empty config) = %+v, want defaults %+v", got, want)
	}
}

func TestLoad_InvalidValuesIgnoredIndividually(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, `{"mutation_cap_per_session": -1, "mutation_cap_per_day": 0, "backup_freshness_window": "not-a-duration"}`)

	got := Load(dir)
	want := Defaults()
	if got.MutationCapPerSession != want.MutationCapPerSession {
		t.Errorf("MutationCapPerSession = %d, want default %d for a negative override", got.MutationCapPerSession, want.MutationCapPerSession)
	}
	if got.MutationCapPerDay != want.MutationCapPerDay {
		t.Errorf("MutationCapPerDay = %d, want default %d for a zero override", got.MutationCapPerDay, want.MutationCapPerDay)
	}
	if got.BackupFreshnessWindow != want.BackupFreshnessWindow {
		t.Errorf("BackupFreshnessWindow = %v, want default %v for an unparsable override", got.BackupFreshnessWindow, want.BackupFreshnessWindow)
	}
}

func writeConfig(t *testing.T, projectPath, content string) {
	t.Helper()
	dir := filepath.Join(projectPath, ".drup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
