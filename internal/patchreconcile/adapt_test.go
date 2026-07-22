package patchreconcile

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func initRepoWithFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("add", ".")
	run("commit", "-m", "initial")
}

const validPatch = `--- a/src/Foo.php
+++ b/src/Foo.php
@@ -1,2 +1,2 @@
 <?php
-echo "old";
+echo "new";
`

func TestAdapt_AppliesCleanly(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepoWithFile(t, dir, "src/Foo.php", "<?php\necho \"old\";\n")

	result, err := Adapt(dir, validPatch, "#3123456")
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if !result.Applied {
		t.Errorf("expected Applied=true for a patch that applies cleanly, got false")
	}
	if result.LocallyAdapted {
		t.Error("expected LocallyAdapted=false when the patch applies cleanly")
	}
	if result.IssueReference != "#3123456" {
		t.Errorf("IssueReference = %q, want %q", result.IssueReference, "#3123456")
	}
}

func TestAdapt_RejectedProducesLocalAdaptation(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	// File content does NOT match the patch's context lines, so `git apply
	// --check` must reject it.
	initRepoWithFile(t, dir, "src/Foo.php", "<?php\necho \"completely different\";\n")

	result, err := Adapt(dir, validPatch, "#3123456")
	if err != nil {
		t.Fatalf("Adapt error: %v", err)
	}
	if result.Applied {
		t.Error("expected Applied=false when git apply --check rejects the patch")
	}
	if !result.LocallyAdapted {
		t.Fatal("expected LocallyAdapted=true when the upstream patch no longer applies")
	}
	if result.IssueReference != "#3123456" {
		t.Errorf("IssueReference = %q, want %q", result.IssueReference, "#3123456")
	}
	if !strings.Contains(result.AdaptedPatch, "#3123456") {
		t.Errorf("AdaptedPatch header must preserve the issue reference, got: %q", result.AdaptedPatch)
	}
	if !strings.Contains(result.AdaptedPatch, "echo \"new\";") {
		t.Error("AdaptedPatch must still reproduce the original patch intent (diff content)")
	}
	if !strings.Contains(result.ComposerDescription, "#3123456") {
		t.Errorf("ComposerDescription must reference the original issue, got: %q", result.ComposerDescription)
	}
}

func TestAdapt_MissingIssueReference(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepoWithFile(t, dir, "src/Foo.php", "<?php\necho \"old\";\n")

	_, err := Adapt(dir, validPatch, "")
	if err == nil {
		t.Fatal("expected error for empty issue_reference, got nil")
	}
}

func TestAdapt_EmptyPatchContent(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initRepoWithFile(t, dir, "src/Foo.php", "<?php\necho \"old\";\n")

	_, err := Adapt(dir, "", "#3123456")
	if err == nil {
		t.Fatal("expected error for empty patch_content, got nil")
	}
}
