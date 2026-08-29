package mcp

import "testing"

func TestToolSpecsAreTheSingleCatalogForSchemasAndStubs(t *testing.T) {
	specs := ToolSpecs()
	if len(specs) == 0 {
		t.Fatal("ToolSpecs returned no tools")
	}

	seen := map[string]bool{}
	stubs := defaultTools()
	for _, spec := range specs {
		if spec.Name == "" {
			t.Fatal("tool spec has no name")
		}
		if seen[spec.Name] {
			t.Fatalf("duplicate tool spec: %s", spec.Name)
		}
		seen[spec.Name] = true
		if len(spec.Properties) == 0 {
			t.Errorf("%s has no input schema properties", spec.Name)
		}
		if spec.Effect == "" {
			t.Errorf("%s has no effect class", spec.Name)
		}
		if spec.Stub && stubs[spec.Name] == nil {
			t.Errorf("stub tool %s is absent from defaultTools", spec.Name)
		}
		if !spec.Stub {
			if len(spec.Name) < len("test_backup_") || spec.Name[:len("test_backup_")] != "test_backup_" {
				t.Errorf("non-stub tool %s is not an intentional backup asymmetry", spec.Name)
			}
		}
	}
	for name := range stubs {
		if !seen[name] {
			t.Errorf("stub tool %s has no descriptor", name)
		}
	}
}
