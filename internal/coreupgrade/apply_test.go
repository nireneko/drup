package coreupgrade

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@test.com")
	runGit(t, dir, "config", "user.name", "Test")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func writeComposerFixture(t *testing.T, dir string) {
	t.Helper()
	data := readTestdata(t, "composer_d10.json")
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestApply_DryRunReturnsPreviewOnly(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	result, err := Apply(dir, "11.0.9", true, false, false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true for dry-run preview, got false: %s", result.Report)
	}
	if result.RollbackCheckpoint != "" {
		t.Errorf("dry-run must not create a checkpoint, got %q", result.RollbackCheckpoint)
	}

	// The file on disk must be untouched.
	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := readTestdata(t, "composer_d10.json")
	if string(after) != string(before) {
		t.Error("dry-run must not modify composer.json on disk")
	}
}

func TestApply_RejectsDirtyTree(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	// Dirty the tree with an unrelated file.
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := Apply(dir, "11.0.9", false, false, false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if result.Success {
		t.Error("expected Success=false for dirty working tree")
	}
	if !strings.Contains(result.Report, "dirty") {
		t.Errorf("Report = %q, want it to mention 'dirty'", result.Report)
	}
}

func TestApply_ChecksClean_CreatesCheckpoint_AndMutates(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	writeComposerFixture(t, dir)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	result, err := Apply(dir, "11.0.9", false, false, false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true, got false: %s", result.Report)
	}
	if result.RollbackCheckpoint == "" {
		t.Fatal("expected a non-empty rollback checkpoint SHA")
	}

	// Verify composer.json was actually mutated on disk.
	after, err := os.ReadFile(filepath.Join(dir, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(after, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Require["drupal/core-recommended"] != "^11" {
		t.Errorf("drupal/core-recommended = %q, want %q", doc.Require["drupal/core-recommended"], "^11")
	}

	// The checkpoint commit must exist and predate the mutation.
	out := runGit(t, dir, "cat-file", "-e", result.RollbackCheckpoint)
	_ = out
}

func TestApply_PathTraversalRejected(t *testing.T) {
	_, err := Apply("/tmp/../../etc", "11.0.9", true, false, false)
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want it to mention '..'", err.Error())
	}
}

func TestApply_RelativePathRejected(t *testing.T) {
	_, err := Apply("relative/path", "11.0.9", true, false, false)
	if err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
}

func TestValidateProjectPath_ReturnsResolvedPath(t *testing.T) {
	dir := t.TempDir()
	resolved, err := ValidateProjectPath(dir)
	if err != nil {
		t.Fatalf("ValidateProjectPath error: %v", err)
	}
	wantResolved, evalErr := filepath.EvalSymlinks(dir)
	if evalErr != nil {
		t.Fatalf("EvalSymlinks error: %v", evalErr)
	}
	if resolved != wantResolved {
		t.Errorf("ValidateProjectPath(%q) = %q, want %q", dir, resolved, wantResolved)
	}
}

func TestApply_ResolvesSymlinkedProjectPath(t *testing.T) {
	requireGit(t)
	real := t.TempDir()
	initGitRepo(t, real)
	writeComposerFixture(t, real)
	runGit(t, real, "add", ".")
	runGit(t, real, "commit", "-m", "initial")

	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	result, err := Apply(link, "11.0.9", false, false, false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected Success=true, got false: %s", result.Report)
	}

	// The mutation must have landed on the real target, resolved from the
	// symlink, not attempted against the symlink path itself.
	after, err := os.ReadFile(filepath.Join(real, "composer.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(after, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Require["drupal/core-recommended"] != "^11" {
		t.Errorf("drupal/core-recommended = %q, want %q", doc.Require["drupal/core-recommended"], "^11")
	}
}

func TestApply_NoChangeReported(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	initGitRepo(t, dir)
	data := readTestdata(t, "composer_d11.json")
	os.WriteFile(filepath.Join(dir, "composer.json"), data, 0o644)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")

	result, err := Apply(dir, "11.0.9", false, false, false)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true when already at target constraint")
	}
	if !strings.Contains(result.Report, "already at target") {
		t.Errorf("Report = %q, want already-at-target guidance", result.Report)
	}
	if result.RollbackCheckpoint != "" {
		t.Error("expected no checkpoint created when there is nothing to change")
	}
}

func TestApply_RejectsUnsafeTargetsAndNoOpsAtCurrentMajor(t *testing.T) {
	requireGit(t)
	tests := []struct {
		name        string
		fixture     string
		target      string
		wantSuccess bool
		wantError   string
	}{
		{name: "equal target is a successful no-op", fixture: "composer_d11.json", target: "11", wantSuccess: true},
		{name: "lower target is rejected", fixture: "composer_d11.json", target: "10", wantError: "lower"},
		{name: "target without every required catalog is rejected", fixture: "composer_d10.json", target: "12", wantError: "missing compatibility metadata for Drupal 11-to-12"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			initGitRepo(t, dir)
			before := readTestdata(t, tt.fixture)
			if err := os.WriteFile(filepath.Join(dir, "composer.json"), before, 0o644); err != nil {
				t.Fatal(err)
			}
			runGit(t, dir, "add", ".")
			runGit(t, dir, "commit", "-m", "initial")
			commitsBefore := runGit(t, dir, "rev-list", "--count", "HEAD")

			result, err := Apply(dir, tt.target, false, false, false)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("Apply error = %v, want it to contain %q", err, tt.wantError)
				}
			} else if err != nil {
				t.Fatalf("Apply error: %v", err)
			} else if result.Success != tt.wantSuccess || !strings.Contains(result.Report, "already at target") {
				t.Fatalf("Apply result = %#v, want successful already-at-target no-op", result)
			}

			after, readErr := os.ReadFile(filepath.Join(dir, "composer.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(after) != string(before) {
				t.Fatal("unsafe or no-op target modified composer.json")
			}
			if commitsAfter := runGit(t, dir, "rev-list", "--count", "HEAD"); commitsAfter != commitsBefore {
				t.Fatalf("unsafe or no-op target created a checkpoint: before %s, after %s", commitsBefore, commitsAfter)
			}
		})
	}
}
