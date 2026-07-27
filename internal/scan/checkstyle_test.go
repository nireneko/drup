package scan

import (
	"strings"
	"testing"
)

// Real output shape from upgrade_status:checkstyle 4.3.x.
const sampleCheckstyle = `<?xml version="1.0"?>
<checkstyle><file name="themes/custom/repository/repository.info.yml"><error line="4" message="Value of core_version_requirement: ^9 || ^10 is not compatible with the next major version of Drupal core." severity="error"/></file><file name="modules/custom/repository_sync/src/Sync.php"><error line="12" message="Call to deprecated method foo()." severity="error"/><error line="30" message="Class Bar is deprecated." severity="warning"/></file></checkstyle>`

func TestParseCheckstyle_GroupsErrorsByModule(t *testing.T) {
	result, err := ParseCheckstyle(strings.NewReader(sampleCheckstyle))
	if err != nil {
		t.Fatalf("ParseCheckstyle error: %v", err)
	}

	if result.TotalErrs != 3 {
		t.Errorf("TotalErrs = %d, want 3", result.TotalErrs)
	}
	if len(result.Modules) != 2 {
		t.Fatalf("modules = %d, want 2", len(result.Modules))
	}

	// Sorted by name: repository, repository_sync.
	if result.Modules[0].Name != "repository" || result.Modules[0].Type != ClassTheme {
		t.Errorf("module[0] = %q/%s, want repository/theme", result.Modules[0].Name, result.Modules[0].Type)
	}
	if result.Modules[1].Name != "repository_sync" || result.Modules[1].Type != ClassCustom {
		t.Errorf("module[1] = %q/%s, want repository_sync/custom", result.Modules[1].Name, result.Modules[1].Type)
	}
	if len(result.Modules[1].Errors) != 2 {
		t.Errorf("repository_sync errors = %d, want 2", len(result.Modules[1].Errors))
	}
	first := result.Modules[1].Errors[0]
	if first.Line != 12 || first.Source != "upgrade_status" || first.Message == "" {
		t.Errorf("unexpected error entry: %+v", first)
	}
}

func TestParseCheckstyle_EmptyReport(t *testing.T) {
	result, err := ParseCheckstyle(strings.NewReader(`<?xml version="1.0"?><checkstyle/>`))
	if err != nil {
		t.Fatalf("ParseCheckstyle error: %v", err)
	}
	if result.TotalErrs != 0 || len(result.Modules) != 0 {
		t.Errorf("empty report parsed as %+v", result)
	}
}

func TestParseCheckstyle_InvalidXML(t *testing.T) {
	if _, err := ParseCheckstyle(strings.NewReader("not xml at all")); err == nil {
		t.Error("expected an error for non-XML input")
	}
}

func TestModuleNameFromPath(t *testing.T) {
	tests := map[string]string{
		"modules/custom/repository_sync/src/Sync.php":  "repository_sync",
		"modules/contrib/address/address.module":       "address",
		"themes/custom/repository/repository.theme":    "repository",
		"themes/contrib/bootstrap_barrio/x.twig":       "bootstrap_barrio",
		"profiles/custom/myprofile/myprofile.info.yml": "myprofile",
		"core/lib/Drupal.php":                          "core",
	}
	for path, want := range tests {
		if got := moduleNameFromPath(path); got != want {
			t.Errorf("moduleNameFromPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestParseCheckstyle_NoticesOnlyMeanNoFindings(t *testing.T) {
	for _, in := range []string{"[warning] No errors found.\n", "No errors found.", "  "} {
		result, err := ParseCheckstyle(strings.NewReader(in))
		if err != nil {
			t.Fatalf("ParseCheckstyle(%q) error: %v", in, err)
		}
		if result.TotalErrs != 0 {
			t.Errorf("ParseCheckstyle(%q) = %d errors, want 0", in, result.TotalErrs)
		}
	}
}

func TestParseCheckstyle_RejectsUnrecognizedOutput(t *testing.T) {
	legacy := "================\nRepository, --\nScanned on Lun\n\nFILE: modules/custom/x/x.module\n\nSTATUS LINE MESSAGE\nCheck manually 4 something\n"
	if _, err := ParseCheckstyle(strings.NewReader(legacy)); err == nil {
		t.Error("legacy text report parsed as a valid empty result, hiding every finding")
	}
}

func TestParseCheckstyle_SkipsLeadingNotices(t *testing.T) {
	in := "[notice] Processing modules.\n" + sampleCheckstyle
	result, err := ParseCheckstyle(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseCheckstyle error: %v", err)
	}
	if result.TotalErrs != 3 {
		t.Errorf("TotalErrs = %d, want 3", result.TotalErrs)
	}
}
