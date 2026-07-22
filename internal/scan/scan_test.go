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
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_d10.txt"))
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
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_d9.txt"))
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
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_empty.txt"))
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

func TestParse_UnparseableInput(t *testing.T) {
	// Garbage input should return a zero-result ScanResult, not an error.
	r := strings.NewReader("this is not valid upgrade_status output\njust random text\n")
	result, err := Parse(r)
	if err != nil {
		t.Fatalf("Parse should not error on unparseable input, got: %v", err)
	}
	if result.TotalErrs != 0 {
		t.Errorf("TotalErrs = %d, want 0 for unparseable input", result.TotalErrs)
	}
	if len(result.Modules) != 0 {
		t.Errorf("len(Modules) = %d, want 0 for unparseable input", len(result.Modules))
	}
}

func TestParse_PlainText(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantTotal   int
		wantModules int
		wantNames   []string
		checkErrors func(t *testing.T, result *ScanResult)
	}{
		{
			name: "multi-project",
			input: `
====================

Project: alpha (modules/contrib/alpha)

  - modules/contrib/alpha/alpha.module:10
    Deprecated function foo().
    Rule: deprecation

====================

Project: beta (modules/custom/beta)

  - modules/custom/beta/src/Svc.php:5
    Call to deprecated bar().
    Rule: drupal.deprecated

  - modules/custom/beta/beta.module:20
    Use of deprecated baz().
    Rule: deprecation
`,
			wantTotal:   3,
			wantModules: 2,
			wantNames:   []string{"alpha", "beta"},
			checkErrors: func(t *testing.T, result *ScanResult) {
				t.Helper()
				byName := make(map[string]ModuleStatus)
				for _, m := range result.Modules {
					byName[m.Name] = m
				}
				if alpha, ok := byName["alpha"]; !ok {
					t.Error("missing module 'alpha'")
				} else {
					if alpha.Type != ClassContrib {
						t.Errorf("alpha.Type = %q, want %q", alpha.Type, ClassContrib)
					}
					if len(alpha.Errors) != 1 {
						t.Fatalf("alpha errors = %d, want 1", len(alpha.Errors))
					}
					if alpha.Errors[0].File != "modules/contrib/alpha/alpha.module" {
						t.Errorf("alpha error file = %q", alpha.Errors[0].File)
					}
					if alpha.Errors[0].Line != 10 {
						t.Errorf("alpha error line = %d, want 10", alpha.Errors[0].Line)
					}
					if alpha.Errors[0].Rule != "deprecation" {
						t.Errorf("alpha error rule = %q, want %q", alpha.Errors[0].Rule, "deprecation")
					}
				}
				if beta, ok := byName["beta"]; !ok {
					t.Error("missing module 'beta'")
				} else {
					if beta.Type != ClassCustom {
						t.Errorf("beta.Type = %q, want %q", beta.Type, ClassCustom)
					}
					if len(beta.Errors) != 2 {
						t.Fatalf("beta errors = %d, want 2", len(beta.Errors))
					}
				}
			},
		},
		{
			name: "single-project",
			input: `
Project: solo (modules/contrib/solo)

  - modules/contrib/solo/solo.module:1
    One error here.
    Rule: test-rule
`,
			wantTotal:   1,
			wantModules: 1,
			wantNames:   []string{"solo"},
		},
		{
			name: "warnings-only",
			input: `[warning] Something happened.
[warning] Another warning.
`,
			wantTotal:   0,
			wantModules: 0,
			wantNames:   []string{},
		},
		{
			name:        "empty-input",
			input:       "",
			wantTotal:   0,
			wantModules: 0,
			wantNames:   []string{},
		},
		{
			name: "skip-warning-lines-between-errors",
			input: `
Project: mymod (modules/custom/mymod)

[warning] Some intermediate warning

  - modules/custom/mymod/mymod.module:5
    Deprecation message.
    Rule: deprecation
`,
			wantTotal:   1,
			wantModules: 1,
			wantNames:   []string{"mymod"},
			checkErrors: func(t *testing.T, result *ScanResult) {
				t.Helper()
				if len(result.Modules) != 1 {
					t.Fatalf("modules = %d, want 1", len(result.Modules))
				}
				if len(result.Modules[0].Errors) != 1 {
					t.Fatalf("errors = %d, want 1", len(result.Modules[0].Errors))
				}
				e := result.Modules[0].Errors[0]
				if !strings.Contains(e.Message, "Deprecation message") {
					t.Errorf("error message = %q, want it to contain 'Deprecation message'", e.Message)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Parse(strings.NewReader(tt.input))
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}
			if result.TotalErrs != tt.wantTotal {
				t.Errorf("TotalErrs = %d, want %d", result.TotalErrs, tt.wantTotal)
			}
			if len(result.Modules) != tt.wantModules {
				t.Errorf("len(Modules) = %d, want %d", len(result.Modules), tt.wantModules)
			}
			if tt.checkErrors != nil {
				tt.checkErrors(t, result)
			}
		})
	}
}

func TestParse_D10FixtureErrorDetails(t *testing.T) {
	f, err := os.Open(filepath.Join(testdataDir(t), "upgrade_status_d10.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	result, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}

	byName := make(map[string]ModuleStatus)
	for _, m := range result.Modules {
		byName[m.Name] = m
	}

	// Verify token error details.
	tok := byName["token"]
	if len(tok.Errors) != 1 {
		t.Fatalf("token errors = %d, want 1", len(tok.Errors))
	}
	if tok.Errors[0].File != "modules/contrib/token/token.module" {
		t.Errorf("token error file = %q", tok.Errors[0].File)
	}
	if tok.Errors[0].Line != 42 {
		t.Errorf("token error line = %d, want 42", tok.Errors[0].Line)
	}
	if tok.Errors[0].Rule != "deprecation" {
		t.Errorf("token error rule = %q", tok.Errors[0].Rule)
	}
	if !strings.Contains(tok.Errors[0].Message, "token_get_tree") {
		t.Errorf("token error message = %q, want it to contain 'token_get_tree'", tok.Errors[0].Message)
	}

	// Verify mymodule error details.
	mod := byName["mymodule"]
	if len(mod.Errors) != 2 {
		t.Fatalf("mymodule errors = %d, want 2", len(mod.Errors))
	}
	if mod.Errors[0].File != "modules/custom/mymodule/src/Service.php" {
		t.Errorf("mymodule error[0] file = %q", mod.Errors[0].File)
	}
	if mod.Errors[0].Line != 15 {
		t.Errorf("mymodule error[0] line = %d", mod.Errors[0].Line)
	}
	if mod.Errors[0].Rule != "drupal.entity_type_manager" {
		t.Errorf("mymodule error[0] rule = %q", mod.Errors[0].Rule)
	}
	if mod.Errors[1].File != "modules/custom/mymodule/mymodule.module" {
		t.Errorf("mymodule error[1] file = %q", mod.Errors[1].File)
	}
	if mod.Errors[1].Line != 8 {
		t.Errorf("mymodule error[1] line = %d", mod.Errors[1].Line)
	}

	// Verify severity and source defaults.
	for _, m := range result.Modules {
		for _, e := range m.Errors {
			if e.Severity != "warning" {
				t.Errorf("error severity = %q, want %q", e.Severity, "warning")
			}
			if e.Source != "upgrade_status" {
				t.Errorf("error source = %q, want %q", e.Source, "upgrade_status")
			}
		}
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
