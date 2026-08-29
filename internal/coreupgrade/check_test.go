package coreupgrade

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/drupalorg"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// stubCheckRelease overrides the package-level checkRelease var for the
// duration of the test.
func stubCheckRelease(t *testing.T, info *drupalorg.ReleaseInfo) {
	t.Helper()
	orig := checkRelease
	checkRelease = func(module string) (*drupalorg.ReleaseInfo, error) {
		return info, nil
	}
	t.Cleanup(func() { checkRelease = orig })
}

func TestNextMajor_Available(t *testing.T) {
	stubCheckRelease(t, &drupalorg.ReleaseInfo{Module: "drupal/core", Latest: "11.0.9", HasD11: true})

	result, err := NextMajor("10.1.5")
	if err != nil {
		t.Fatalf("NextMajor error: %v", err)
	}
	if !result.Available {
		t.Fatal("expected Available=true, got false")
	}
	if result.NextVersion != "11.0.9" {
		t.Errorf("NextVersion = %q, want %q", result.NextVersion, "11.0.9")
	}
	if result.Constraint != "^11.0" {
		t.Errorf("Constraint = %q, want %q", result.Constraint, "^11.0")
	}
	if result.CurrentVersion != "10.1.5" {
		t.Errorf("CurrentVersion = %q, want %q", result.CurrentVersion, "10.1.5")
	}
}

func TestNextMajor_OnlyOffersImmediateNextMajor(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		wantErr string
	}{
		{
			name:    "later release cannot skip the immediate major",
			current: "10.3.0",
			latest:  "12.0.0",
			wantErr: "immediate next Drupal major 11 is unavailable; upgrade to 11 before checking for 12",
		},
		{
			name:    "multiple later majors still require the next one",
			current: "11.2.0",
			latest:  "13.0.0",
			wantErr: "immediate next Drupal major 12 is unavailable; upgrade to 12 before checking for 13",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stubCheckRelease(t, &drupalorg.ReleaseInfo{Module: "drupal/core", Latest: tt.latest})

			result, err := NextMajor(tt.current)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NextMajor error = %v, want it to contain %q", err, tt.wantErr)
			}
			if result != nil {
				t.Fatalf("NextMajor result = %#v, want nil when the immediate major is unavailable", result)
			}
		})
	}
}

func TestNextMajor_AlreadyOnLatest(t *testing.T) {
	stubCheckRelease(t, &drupalorg.ReleaseInfo{Module: "drupal/core", Latest: "11.0.9", HasD11: true})

	result, err := NextMajor("11.0.9")
	if err != nil {
		t.Fatalf("NextMajor error: %v", err)
	}
	if result.Available {
		t.Fatal("expected Available=false when already on latest major, got true")
	}
	if result.NextVersion != "" {
		t.Errorf("NextVersion = %q, want empty", result.NextVersion)
	}
}

func TestNextMajor_InvalidCurrentVersion(t *testing.T) {
	stubCheckRelease(t, &drupalorg.ReleaseInfo{Module: "drupal/core", Latest: "11.0.9"})

	_, err := NextMajor("not-a-version")
	if err == nil {
		t.Fatal("expected error for invalid current version, got nil")
	}
}

func TestPreviewComposerPatch_ShowsDiffOnly(t *testing.T) {
	data := readTestdata(t, "composer_d10.json")

	diff, changed, err := PreviewComposerPatch(data, "^11.0")
	if err != nil {
		t.Fatalf("PreviewComposerPatch error: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true, got false")
	}
	if !strings.Contains(diff, `-"drupal/core-recommended": "^10.1"`) {
		t.Errorf("diff = %q, want it to contain removed constraint line", diff)
	}
	if !strings.Contains(diff, `+"drupal/core-recommended": "^11.0"`) {
		t.Errorf("diff = %q, want it to contain added constraint line", diff)
	}
	// All three drupal/core-* entries must be present in the diff.
	for _, pkg := range []string{"drupal/core-recommended", "drupal/core-composer-scaffold", "drupal/core-project-message"} {
		if !strings.Contains(diff, pkg) {
			t.Errorf("diff missing entry for %q", pkg)
		}
	}
}

func TestPreviewComposerPatch_AlreadyAtTarget(t *testing.T) {
	data := readTestdata(t, "composer_d11.json")

	diff, changed, err := PreviewComposerPatch(data, "^11.0")
	if err != nil {
		t.Fatalf("PreviewComposerPatch error: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false when already at target constraint, got true (diff=%q)", diff)
	}
}

func TestPreviewComposerPatch_NoCoreRequirement(t *testing.T) {
	diff, changed, err := PreviewComposerPatch([]byte(`{"require":{"php":">=8.1"}}`), "^11.0")
	if err != nil {
		t.Fatalf("PreviewComposerPatch error: %v", err)
	}
	if changed {
		t.Errorf("expected changed=false with no drupal/core requirement, got true (diff=%q)", diff)
	}
}

func TestPreviewComposerPatch_InvalidJSON(t *testing.T) {
	_, _, err := PreviewComposerPatch([]byte(`{invalid`), "^11.0")
	if err == nil {
		t.Fatal("expected error for invalid composer.json, got nil")
	}
}
