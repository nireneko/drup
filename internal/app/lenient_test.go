package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Composer resolves a package's core requirement from its released metadata,
// never from patched files. A module whose D11 patch already widened its
// .info.yml is still rejected without this list.
func TestAllowLenient_AddsPackagesToTheList(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")
	os.WriteFile(composerPath, []byte(`{"require":{"drupal/core-recommended":"^11.0"},"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"},"extra":{"patches":{}}}`), 0o644)

	result, err := AllowLenient(dir, []string{"drupal/switch_page_theme"}, false)
	if err != nil {
		t.Fatalf("AllowLenient error: %v", err)
	}
	if len(result.Added) != 1 || result.Added[0] != "drupal/switch_page_theme" {
		t.Errorf("Added = %v, want the one package", result.Added)
	}

	var composer struct {
		Extra struct {
			Lenient struct {
				AllowedList []string `json:"allowed-list"`
			} `json:"drupal-lenient"`
			Patches map[string]any `json:"patches"`
		} `json:"extra"`
	}
	data, _ := os.ReadFile(composerPath)
	if err := json.Unmarshal(data, &composer); err != nil {
		t.Fatalf("composer.json is no longer valid: %v", err)
	}
	if len(composer.Extra.Lenient.AllowedList) != 1 {
		t.Errorf("allowed-list = %v", composer.Extra.Lenient.AllowedList)
	}
	if composer.Extra.Patches == nil {
		t.Error("the existing extra.patches section was dropped")
	}
}

func TestAllowLenient_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"},"extra":{"drupal-lenient":{"allowed-list":["drupal/foo"]}}}`), 0o644)

	result, err := AllowLenient(dir, []string{"drupal/foo"}, false)
	if err != nil {
		t.Fatalf("AllowLenient error: %v", err)
	}
	if len(result.Added) != 0 {
		t.Errorf("Added = %v, want nothing for a package already listed", result.Added)
	}
	if len(result.AllowedList) != 1 {
		t.Errorf("AllowedList = %v, want no duplicate", result.AllowedList)
	}
}

func TestAllowLenient_RejectsBarePackageNames(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0o644)

	if _, err := AllowLenient(dir, []string{"switch_page_theme"}, false); err == nil {
		t.Error("a bare module name was accepted; composer needs vendor/name")
	}
}

func TestAllowLenient_DryRunLeavesTheFileAlone(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")
	original := `{"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"}}`
	os.WriteFile(composerPath, []byte(original), 0o644)

	if _, err := AllowLenient(dir, []string{"drupal/foo"}, true); err != nil {
		t.Fatalf("AllowLenient error: %v", err)
	}

	data, _ := os.ReadFile(composerPath)
	if string(data) != original {
		t.Errorf("dry run modified composer.json:\n%s", data)
	}
}
