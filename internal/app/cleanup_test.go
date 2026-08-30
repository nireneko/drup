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
)

// initCleanupGitRepo initializes a real git repo in dir with an initial
// initial state. Cleanup leaves its changes for checkpoint_commit rather than
// publishing history itself.
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

	diffOut, diffErr := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	if diffErr != nil {
		t.Fatalf("git status: %v\n%s", diffErr, diffOut)
	}
	changed := strings.TrimSpace(string(diffOut))
	if !strings.Contains(changed, "composer.json") || !strings.Contains(changed, "composer.lock") {
		t.Errorf("pending files = %q, want composer.json and composer.lock", changed)
	}

	output := buf.String()
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
	if got, ok := result["changed_files"].([]interface{}); !ok || len(got) != 2 {
		t.Errorf("changed_files = %#v, want composer.json and composer.lock", result["changed_files"])
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
	diffOut, diffErr := exec.Command("git", "-C", dir, "diff", "--name-only").CombinedOutput()
	if diffErr != nil {
		t.Fatalf("git diff: %v\n%s", diffErr, diffOut)
	}
	if strings.Contains(string(diffOut), "notes.txt") {
		t.Errorf("cleanup changed unrelated notes.txt: %q", diffOut)
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
