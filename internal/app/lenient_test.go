package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// installLenientPlugin fakes the plugin being present in vendor/, which is
// what AllowLenient now requires before it will trust the allow list.
func installLenientPlugin(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "vendor", "mglaman", "composer-drupal-lenient"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Composer resolves a package's core requirement from its released metadata,
// never from patched files. A module whose D11 patch already widened its
// .info.yml is still rejected without this list.
func TestAllowLenient_AddsPackagesToTheList(t *testing.T) {
	dir := t.TempDir()
	composerPath := filepath.Join(dir, "composer.json")
	os.WriteFile(composerPath, []byte(`{"require":{"drupal/core-recommended":"^11.0"},"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"},"extra":{"patches":{}}}`), 0o644)

	installLenientPlugin(t, dir)

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

	installLenientPlugin(t, dir)

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

// composer writes composer.json and the lock before allow-plugins blocks a
// plugin install, so the package can be declared while vendor/ has nothing.
// An allow list without the plugin behind it changes nothing, and reporting
// that as success is why three upgrade attempts failed for no visible reason.
func TestLenientPluginInstalled_DeclaredIsNotInstalled(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"),
		[]byte(`{"require-dev":{"mglaman/composer-drupal-lenient":"^1.0"}}`), 0o644)

	if lenientPluginInstalled(dir) {
		t.Error("a package declared in composer.json was reported as installed")
	}

	os.MkdirAll(filepath.Join(dir, "vendor", "mglaman", "composer-drupal-lenient"), 0o755)
	if !lenientPluginInstalled(dir) {
		t.Error("a package present in vendor/ was not detected")
	}
}

func TestLenientPluginInstalled_ReadsInstalledJSON(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "vendor", "composer"), 0o755)
	os.WriteFile(filepath.Join(dir, "vendor", "composer", "installed.json"),
		[]byte(`{"packages":[{"name":"mglaman/composer-drupal-lenient"}]}`), 0o644)

	if !lenientPluginInstalled(dir) {
		t.Error("a package recorded in installed.json was not detected")
	}
}
