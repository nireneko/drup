package scan

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestParse_D10Fixture(t *testing.T) {
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_d10.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if result.TotalErrs != 4 {
		t.Errorf("TotalErrs = %d, want 4", result.TotalErrs)
	}
	if len(result.Modules) != 3 {
		t.Fatalf("len(Modules) = %d, want 3", len(result.Modules))
	}

	// Check classification.
	byName := make(map[string]ModuleStatus)
	for _, m := range result.Modules {
		byName[m.Name] = m
	}

	// token → contrib
	if tok, ok := byName["token"]; !ok {
		t.Error("missing module 'token'")
	} else if tok.Type != ClassContrib {
		t.Errorf("token.Type = %q, want %q", tok.Type, ClassContrib)
	} else if len(tok.Errors) != 1 {
		t.Errorf("token errors = %d, want 1", len(tok.Errors))
	}

	// mymodule → custom
	if mod, ok := byName["mymodule"]; !ok {
		t.Error("missing module 'mymodule'")
	} else if mod.Type != ClassCustom {
		t.Errorf("mymodule.Type = %q, want %q", mod.Type, ClassCustom)
	} else if len(mod.Errors) != 2 {
		t.Errorf("mymodule errors = %d, want 2", len(mod.Errors))
	}

	// mytheme → theme
	if th, ok := byName["mytheme"]; !ok {
		t.Error("missing module 'mytheme'")
	} else if th.Type != ClassTheme {
		t.Errorf("mytheme.Type = %q, want %q", th.Type, ClassTheme)
	}
}

func TestParse_D9Fixture(t *testing.T) {
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_d9.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if result.TotalErrs != 1 {
		t.Errorf("TotalErrs = %d, want 1", result.TotalErrs)
	}
	if len(result.Modules) != 1 {
		t.Fatalf("len(Modules) = %d, want 1", len(result.Modules))
	}
	if result.Modules[0].Name != "oldmodule" {
		t.Errorf("module name = %q, want %q", result.Modules[0].Name, "oldmodule")
	}
	if result.Modules[0].Type != ClassContrib {
		t.Errorf("module type = %q, want %q", result.Modules[0].Type, ClassContrib)
	}
}

func TestParse_EmptyFixture(t *testing.T) {
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_empty.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	if result.TotalErrs != 0 {
		t.Errorf("TotalErrs = %d, want 0", result.TotalErrs)
	}
	if len(result.Modules) != 0 {
		t.Errorf("len(Modules) = %d, want 0", len(result.Modules))
	}
}

func TestParse_MalformedJSON(t *testing.T) {
	r := strings.NewReader("{invalid json")
	_, err := Parse(r)
	if err == nil {
		t.Error("expected error for malformed JSON, got nil")
	}
}

func TestClassifyPath(t *testing.T) {
	tests := []struct {
		path string
		want ErrorClass
	}{
		{"modules/contrib/token/token.module", ClassContrib},
		{"modules/custom/mymodule/src/Service.php", ClassCustom},
		{"themes/mytheme/templates/page.html.twig", ClassTheme},
		{"core/modules/node/node.module", ClassCore},
		{"some/other/path.php", ClassCustom}, // fallback
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := classifyPath(tt.path)
			if got != tt.want {
				t.Errorf("classifyPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
