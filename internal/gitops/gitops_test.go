package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// helper: init a git repo in dir with an initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@test.com")
	git(t, dir, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, "README"), []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "initial")
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestIsClean_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	clean, files, err := IsClean(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !clean {
		t.Errorf("expected clean, got dirty with files: %v", files)
	}
}

func TestIsClean_DirtyRepo(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	clean, files, err := IsClean(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if clean {
		t.Error("expected dirty, got clean")
	}
	if len(files) == 0 {
		t.Error("expected changed files, got none")
	}
}

func TestEnsureBranch_CreatesBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	if err := EnsureBranch(dir, "upgrade/drupal-11"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cmd := exec.Command("git", "-C", dir, "branch", "--show-current")
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	got := string(out)
	if len(got) > 0 && got[len(got)-1] == '\n' {
		got = got[:len(got)-1]
	}
	if got != "upgrade/drupal-11" {
		t.Errorf("branch = %q, want %q", got, "upgrade/drupal-11")
	}
}

func TestEnsureBranch_ExistingBranch(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	git(t, dir, "branch", "upgrade/drupal-11")

	if err := EnsureBranch(dir, "upgrade/drupal-11"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCommit_CreatesCommit(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	testFile := filepath.Join(dir, "fix.txt")
	if err := os.WriteFile(testFile, []byte("fixed"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := Commit(dir, "fix(contrib): apply D11 patch to token", []string{"fix.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Error("expected commit hash, got empty")
	}
	if len(hash) < 7 {
		t.Errorf("hash too short: %q", hash)
	}
}

func TestCommit_NothingToCommit(t *testing.T) {
	dir := t.TempDir()
	initGitRepo(t, dir)

	_, err := Commit(dir, "empty commit", []string{})
	if err == nil {
		t.Error("expected error for empty commit, got nil")
	}
}
