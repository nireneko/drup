package inventory

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCapture_IsReadOnlyAndDeterministic(t *testing.T) {
	root := t.TempDir()
	mustWrite := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("composer.json", `{"config":{"platform":{"php":"8.3.4"}},"extra":{"patches":{"drupal/token":{"fix":"https://example.test/token.patch"}}}}`)
	mustWrite("composer.lock", `{"packages":[{"name":"drupal/core-recommended","version":"11.1.0"},{"name":"drupal/token","version":"1.2.3"},{"name":"vendor/lib","version":"2.0.0"}]}`)
	mustWrite("config/sync/system.site.yml", "uuid: example\n")
	mustWrite("web/modules/custom/example/example.info.yml", "name: Example\n")
	mustWrite("web/modules/custom/example/tests/src/Unit/ExampleTest.php", "<?php\n")
	before, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Capture(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Capture(root)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("capture mutated project root")
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("capture is not deterministic:\n%#v\n%#v", first, second)
	}
	if first.Core.Version != "11.1.0" || first.PHP.Version != "8.3.4" {
		t.Fatalf("versions = core %#v php %#v", first.Core, first.PHP)
	}
	if len(first.Packages) != 3 || len(first.Extensions) != 1 || len(first.Patches) != 1 || len(first.Config) != 1 || len(first.Tests) != 1 {
		t.Fatalf("incomplete inventory: %#v", first)
	}
}

func TestDelta_IsStableAndLinksProvenance(t *testing.T) {
	before := Inventory{SchemaVersion: SchemaVersion, Core: Version{Version: "10.3", Source: "composer.lock"}, PHP: Version{Version: "8.1", Source: "composer.json"}, Packages: []Package{{Name: "vendor/package", Version: "1.0", Source: "composer.lock"}}, Extensions: []Package{{Name: "drupal/token", Version: "1.0", Source: "composer.lock"}}, Patches: []Patch{{Package: "drupal/token", Description: "old", URL: "https://example.test/old.patch", Source: "composer.json"}}, Config: []File{{Path: "config/sync/system.site.yml", Digest: "old-config", Source: "filesystem"}}, Tests: []File{{Path: "tests/src/Unit/ExampleTest.php", Digest: "old-test", Source: "filesystem"}}}
	after := Inventory{SchemaVersion: SchemaVersion, Core: Version{Version: "11.0", Source: "composer.lock"}, PHP: Version{Version: "8.3", Source: "composer.json"}, Packages: []Package{{Name: "vendor/package", Version: "1.1", Source: "composer.lock"}}, Extensions: []Package{{Name: "drupal/token", Version: "1.1", Source: "composer.lock"}}, Patches: []Patch{{Package: "drupal/token", Description: "new", URL: "https://example.test/new.patch", Source: "composer.json"}}, Config: []File{{Path: "config/sync/system.site.yml", Digest: "new-config", Source: "filesystem"}}, Tests: []File{{Path: "tests/src/Unit/ExampleTest.php", Digest: "new-test", Source: "filesystem"}}}
	changes := Delta(before, after)
	if len(changes) != 7 || changes[0].Kind != "config" || changes[0].Before != "old-config" || changes[0].After != "new-config" || changes[0].Source != "filesystem" {
		t.Fatalf("changes = %#v", changes)
	}
	for i := 1; i < len(changes); i++ {
		if changes[i-1].Kind > changes[i].Kind || (changes[i-1].Kind == changes[i].Kind && changes[i-1].Name > changes[i].Name) {
			t.Fatalf("changes are not deterministically ordered: %#v", changes)
		}
	}
	for _, kind := range []string{"core", "php", "package", "extension", "patch", "config", "test"} {
		found := false
		for _, change := range changes {
			found = found || change.Kind == kind
		}
		if !found {
			t.Errorf("missing %s delta: %#v", kind, changes)
		}
	}
}
