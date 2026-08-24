package app

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
)

// initCleanupGitRepo initializes a real git repo in dir with an initial
// commit. RunCleanup's scoped commit (internal/gitops.Commit) runs real git
// commands against production logic, so cleanup tests that reach the git
// step need a real repository rather than a mocked drupexec.Run — see
// gitops.runCommand, which captures the real drupexec.Run function value at
// package-init time, before any later test override of the exec package var
// takes effect.
func initCleanupGitRepo(t *testing.T, dir string) {
	t.Helper()
	cleanupGit(t, dir, "init")
	cleanupGit(t, dir, "config", "user.email", "test@test.com")
	cleanupGit(t, dir, "config", "user.name", "Test")
	cleanupGit(t, dir, "add", ".")
	cleanupGit(t, dir, "commit", "-m", "initial")
}

func cleanupGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestRunCleanup_ValidatePass_RunsCleanup(t *testing.T) {
	dir := t.TempDir()
	// Create composer.json with upgrade_status present, then commit it as
	// the repo's initial state so the scoped commit has a real baseline to
	// diff against.
	composerJSON := `{"require":{"drupal/upgrade_status":"^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	initCleanupGitRepo(t, dir)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	var drushCalls, composerCalls []string

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		switch cmd {
		case "drush":
			drushCalls = append(drushCalls, strings.Join(args, " "))
			return "", "", 0, nil
		case "composer":
			composerCalls = append(composerCalls, strings.Join(args, " "))
			// Simulate composer remove's real effect: composer.json loses
			// the dependency and composer.lock is regenerated, so the
			// scoped commit has real, declared-path changes to stage.
			os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)
			os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"content-hash":"x"}`), 0o644)
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() {
		drupexec.RunWithEnv = origRunWithEnv
	}()

	var buf bytes.Buffer
	err := RunCleanup(&buf, []string{dir, "--validate-passed"})
	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}

	// Verify drush pm:uninstall was called.
	drushUninstallFound := false
	for _, call := range drushCalls {
		if strings.Contains(call, "pm:uninstall") && strings.Contains(call, "upgrade_status") {
			drushUninstallFound = true
			break
		}
	}
	if !drushUninstallFound {
		t.Errorf("drush pm:uninstall upgrade_status was not called, got: %v", drushCalls)
	}

	// Verify composer remove was called.
	composerRemoveFound := false
	for _, call := range composerCalls {
		if strings.Contains(call, "remove") && strings.Contains(call, "drupal/upgrade_status") {
			composerRemoveFound = true
			break
		}
	}
	if !composerRemoveFound {
		t.Errorf("composer remove drupal/upgrade_status was not called, got: %v", composerCalls)
	}

	// Verify a real commit was created via the scoped gitops.Commit path,
	// and that it covers exactly composer.json and composer.lock.
	logOut, logErr := exec.Command("git", "-C", dir, "log", "--oneline", "-1").CombinedOutput()
	if logErr != nil {
		t.Fatalf("git log: %v\n%s", logErr, logOut)
	}
	if !strings.Contains(string(logOut), "cleanup") {
		t.Errorf("git log = %q, want it to mention the cleanup commit message", logOut)
	}
	showOut, showErr := exec.Command("git", "-C", dir, "show", "--name-only", "--pretty=format:", "HEAD").CombinedOutput()
	if showErr != nil {
		t.Fatalf("git show: %v\n%s", showErr, showOut)
	}
	committed := strings.TrimSpace(string(showOut))
	if !strings.Contains(committed, "composer.json") || !strings.Contains(committed, "composer.lock") {
		t.Errorf("commit files = %q, want composer.json and composer.lock", committed)
	}

	clean, dirtyFiles, cleanErr := gitops.IsClean(dir)
	if cleanErr != nil {
		t.Fatalf("IsClean error: %v", cleanErr)
	}
	if !clean {
		t.Errorf("expected clean tree after cleanup commit, got dirty with files: %v", dirtyFiles)
	}

	output := buf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
}

// TestRunCleanup_UnrelatedDirtyFile_ExcludedFromCommit guards G8: an
// unrelated uncommitted file sitting in the working tree alongside the
// composer.json/composer.lock changes cleanup actually produces must not be
// swept into the cleanup commit, unlike the old `git add -A` behavior.
func TestRunCleanup_UnrelatedDirtyFile_ExcludedFromCommit(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{"require":{"drupal/upgrade_status":"^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	initCleanupGitRepo(t, dir)

	// An unrelated file with pending, uncommitted changes — outside the
	// declared scoped-commit path list.
	unrelatedPath := filepath.Join(dir, "notes.txt")
	os.WriteFile(unrelatedPath, []byte("unrelated dirty content"), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "composer" {
			os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)
			os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"content-hash":"x"}`), 0o644)
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	var buf bytes.Buffer
	if err := RunCleanup(&buf, []string{dir, "--validate-passed"}); err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}

	// The unrelated file must still be uncommitted afterward.
	showOut, showErr := exec.Command("git", "-C", dir, "show", "--name-only", "--pretty=format:", "HEAD").CombinedOutput()
	if showErr != nil {
		t.Fatalf("git show: %v\n%s", showErr, showOut)
	}
	if strings.Contains(string(showOut), "notes.txt") {
		t.Errorf("commit files = %q, want notes.txt excluded", showOut)
	}

	statusOut, statusErr := exec.Command("git", "-C", dir, "status", "--porcelain", "notes.txt").CombinedOutput()
	if statusErr != nil {
		t.Fatalf("git status: %v\n%s", statusErr, statusOut)
	}
	if strings.TrimSpace(string(statusOut)) == "" {
		t.Error("expected notes.txt to remain dirty/untracked after cleanup, got clean")
	}
}

func TestRunCleanup_ValidateFailed_Skips(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	drushCalled := false
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			drushCalled = true
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	var buf bytes.Buffer
	err := RunCleanup(&buf, []string{"/tmp/test", "--validate-failed"})
	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if drushCalled {
		t.Error("drush should NOT be called when validate failed")
	}

	output := buf.String()
	if !strings.Contains(output, "skipped") {
		t.Errorf("output = %q, want it to mention 'skipped'", output)
	}
}

func TestRunCleanup_AlreadyRemoved_Idempotent(t *testing.T) {
	dir := t.TempDir()
	// composer.json without upgrade_status.
	composerJSON := `{"require":{"drupal/core":"^11.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	drushUninstallCalled := false
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			for _, a := range args {
				if a == "pm:uninstall" {
					drushUninstallCalled = true
				}
			}
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	var buf bytes.Buffer
	err := RunCleanup(&buf, []string{dir, "--validate-passed"})
	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if drushUninstallCalled {
		t.Error("drush pm:uninstall should NOT be called when upgrade_status is not in composer.json")
	}

	output := buf.String()
	if !strings.Contains(output, "nothing to do") {
		t.Errorf("output = %q, want 'nothing to do'", output)
	}
}

func TestRunCleanup_DrushFailure_Halts(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{"require":{"drupal/upgrade_status":"^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	composerRemoveCalled := false
	origRunWithEnv := drupexec.RunWithEnv
	origRun := drupexec.Run
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return "", "drush pm:uninstall failed", 1, nil
		}
		if cmd == "composer" {
			composerRemoveCalled = true
		}
		return "", "", 0, nil
	}
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		return "", "", 0, nil
	}
	defer func() {
		drupexec.RunWithEnv = origRunWithEnv
		drupexec.Run = origRun
	}()

	var buf bytes.Buffer
	err := RunCleanup(&buf, []string{dir, "--validate-passed"})
	if err == nil {
		t.Fatal("expected error when drush fails, got nil")
	}
	if composerRemoveCalled {
		t.Error("composer remove should NOT be called when drush fails")
	}
}

// TestRunCleanup_DoesNotSwapOSStdout guards M7: RunCleanup used to write
// through fmt.Println (real os.Stdout), forcing every caller that wants the
// output to temporarily swap os.Stdout for an os.Pipe and read it back
// afterward. Besides being globally unsafe (any concurrent goroutine's
// stdout writes leak into the pipe), an os.Pipe has a fixed kernel buffer:
// if RunCleanup ever wrote more than that buffer in one call with nothing
// draining it concurrently, the write would block forever. Writing directly
// into the caller-supplied io.Writer removes the swap and, with it, the
// pipe and its bounded-buffer deadlock risk — a bytes.Buffer (or any other
// io.Writer) simply grows to fit the output, no concurrent reader required.
func TestRunCleanup_DoesNotSwapOSStdout(t *testing.T) {
	before := os.Stdout

	var buf bytes.Buffer
	if err := RunCleanup(&buf, []string{"/tmp/test", "--validate-failed"}); err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}

	if os.Stdout != before {
		t.Error("RunCleanup swapped os.Stdout — it must write only to the supplied io.Writer")
	}
	if buf.Len() == 0 {
		t.Error("expected output to be written to the supplied writer, got none")
	}
}
