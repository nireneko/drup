package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCoreUpgradePlan_RejectsTargetWithoutExactCatalogMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := buildCoreUpgradePlan(dir, "12")
	if err == nil {
		t.Fatal("buildCoreUpgradePlan accepted 11-to-12 without its own catalog")
	}
	if !strings.Contains(err.Error(), "missing compatibility metadata for Drupal 11-to-12") {
		t.Errorf("error = %q, want missing 11-to-12 metadata", err)
	}
}

func TestBuildCoreUpgradePlan_UsesTheExact10To11Catalog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^10.3"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := buildCoreUpgradePlan(dir, "11")
	if err != nil {
		t.Fatalf("buildCoreUpgradePlan error: %v", err)
	}
	if got, want := plan.Steps[0].CatalogID, "10-to-11"; got != want {
		t.Errorf("CatalogID = %q, want %q", got, want)
	}
}
