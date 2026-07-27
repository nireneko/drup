package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeExtension(t *testing.T, root, kind, name, body string) string {
	t.Helper()
	dir := filepath.Join(root, "web", kind, "custom", name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	info := filepath.Join(dir, name+".info.yml")
	if err := os.WriteFile(info, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return info
}

func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"),
		[]byte(`{"extra":{"drupal-scaffold":{"locations":{"web-root":"web/"}}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "web", "modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestBumpCustomCoreCompat_WidensBlockingConstraints(t *testing.T) {
	root := newProject(t)
	blocking := writeExtension(t, root, "modules", "repository_sync",
		"name: Repository Sync\ntype: module\ncore_version_requirement: ^9 || ^10\n")
	quoted := writeExtension(t, root, "themes", "repository",
		"name: Repository\ntype: theme\ncore_version_requirement: \"^10\"\n")
	compatible := writeExtension(t, root, "modules", "already_ok",
		"name: Fine\ntype: module\ncore_version_requirement: ^10 || ^11\n")

	result, err := BumpCustomCoreCompat(root, "11", false)
	if err != nil {
		t.Fatalf("BumpCustomCoreCompat error: %v", err)
	}

	if result.Updated != 2 {
		t.Errorf("Updated = %d, want 2", result.Updated)
	}
	if result.AlreadyCompatible != 1 {
		t.Errorf("AlreadyCompatible = %d, want 1", result.AlreadyCompatible)
	}

	got, _ := os.ReadFile(blocking)
	if !strings.Contains(string(got), "core_version_requirement: '^9 || ^10 || ^11'") {
		t.Errorf("unquoted constraint not widened:\n%s", got)
	}
	// The original quoting style survives.
	got, _ = os.ReadFile(quoted)
	if !strings.Contains(string(got), `core_version_requirement: "^10 || ^11"`) {
		t.Errorf("quoted constraint not widened in place:\n%s", got)
	}
	// A compatible extension is left untouched.
	got, _ = os.ReadFile(compatible)
	if !strings.Contains(string(got), "core_version_requirement: ^10 || ^11\n") {
		t.Errorf("compatible extension was modified:\n%s", got)
	}
}

func TestBumpCustomCoreCompat_DryRunLeavesFilesAlone(t *testing.T) {
	root := newProject(t)
	info := writeExtension(t, root, "modules", "blocking",
		"name: Blocking\ntype: module\ncore_version_requirement: ^9 || ^10\n")
	before, _ := os.ReadFile(info)

	result, err := BumpCustomCoreCompat(root, "11", true)
	if err != nil {
		t.Fatalf("BumpCustomCoreCompat error: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated = %d, want 1 (reported, not written)", result.Updated)
	}

	after, _ := os.ReadFile(info)
	if string(before) != string(after) {
		t.Errorf("dry run modified the file:\n%s", after)
	}
}

func TestBumpCustomCoreCompat_ReportsMissingDeclaration(t *testing.T) {
	root := newProject(t)
	writeExtension(t, root, "modules", "legacy", "name: Legacy\ntype: module\ncore: 8.x\n")

	result, err := BumpCustomCoreCompat(root, "11", false)
	if err != nil {
		t.Fatalf("BumpCustomCoreCompat error: %v", err)
	}
	if result.NeedsAttention != 1 || result.Updated != 0 {
		t.Fatalf("result = %+v, want one extension needing attention", result)
	}
	if !strings.Contains(result.Changes[0].Note, "add one manually") {
		t.Errorf("note = %q, want guidance to add the key", result.Changes[0].Note)
	}
}

func TestBumpCustomCoreCompat_IgnoresContrib(t *testing.T) {
	root := newProject(t)
	contribDir := filepath.Join(root, "web", "modules", "contrib", "token")
	if err := os.MkdirAll(contribDir, 0o755); err != nil {
		t.Fatal(err)
	}
	contribInfo := filepath.Join(contribDir, "token.info.yml")
	body := "name: Token\ntype: module\ncore_version_requirement: ^9 || ^10\n"
	if err := os.WriteFile(contribInfo, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := BumpCustomCoreCompat(root, "11", false); err != nil {
		t.Fatalf("BumpCustomCoreCompat error: %v", err)
	}

	got, _ := os.ReadFile(contribInfo)
	if string(got) != body {
		t.Errorf("contrib module edited in place; it must be patched instead:\n%s", got)
	}
}

func TestBumpCustomCoreCompat_RejectsBadTarget(t *testing.T) {
	root := newProject(t)
	if _, err := BumpCustomCoreCompat(root, "next", false); err == nil {
		t.Error("expected an error for a non-numeric target version")
	}
}

func TestMajorOf(t *testing.T) {
	cases := map[string]string{
		"11": "11", "11.1": "11", "^11": "11", " 11.2.3 ": "11", "": "", "next": "",
	}
	for in, want := range cases {
		if got := majorOf(in); got != want {
			t.Errorf("majorOf(%q) = %q, want %q", in, got, want)
		}
	}
}
