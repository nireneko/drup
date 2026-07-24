package composerutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadWebRoot_ScaffoldConfigPresent(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{
		"extra": {
			"drupal-scaffold": {
				"locations": {
					"web-root": "docroot"
				}
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	got := ReadWebRoot(dir)
	if got != "docroot" {
		t.Errorf("ReadWebRoot = %q, want %q", got, "docroot")
	}
}

func TestReadWebRoot_ScaffoldAbsent_Fallback(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{"require": {"drupal/core": "^11.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	got := ReadWebRoot(dir)
	if got != "web" {
		t.Errorf("ReadWebRoot = %q, want %q", got, "web")
	}
}

func TestReadWebRoot_MalformedJSON_Fallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{not valid json`), 0o644)

	got := ReadWebRoot(dir)
	if got != "web" {
		t.Errorf("ReadWebRoot = %q, want %q (fallback on malformed JSON)", got, "web")
	}
}

func TestReadWebRoot_NoComposerJSON_Fallback(t *testing.T) {
	dir := t.TempDir()
	// No composer.json at all.
	got := ReadWebRoot(dir)
	if got != "web" {
		t.Errorf("ReadWebRoot = %q, want %q (fallback when no composer.json)", got, "web")
	}
}

func TestReadWebRoot_EmptyScaffoldLocations_Fallback(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{
		"extra": {
			"drupal-scaffold": {
				"locations": {}
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	got := ReadWebRoot(dir)
	if got != "web" {
		t.Errorf("ReadWebRoot = %q, want %q (fallback when web-root missing)", got, "web")
	}
}
