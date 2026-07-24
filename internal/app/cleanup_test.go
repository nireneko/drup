package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	drupexec "github.com/nireneko/drup/internal/exec"
)

func TestRunCleanup_ValidatePass_RunsCleanup(t *testing.T) {
	dir := t.TempDir()
	// Create composer.json with upgrade_status present.
	composerJSON := `{"require":{"drupal/upgrade_status":"^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	var drushCalls, composerCalls []string
	gitCommitCalled := false

	origRunWithEnv := drupexec.RunWithEnv
	origRun := drupexec.Run
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		switch cmd {
		case "drush":
			drushCalls = append(drushCalls, strings.Join(args, " "))
			return "", "", 0, nil
		case "composer":
			composerCalls = append(composerCalls, strings.Join(args, " "))
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		if cmd == "git" {
			for _, a := range args {
				if a == "commit" {
					gitCommitCalled = true
				}
			}
		}
		return "", "", 0, nil
	}
	defer func() {
		drupexec.RunWithEnv = origRunWithEnv
		drupexec.Run = origRun
	}()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunCleanup([]string{dir, "--validate-passed"})

	w.Close()
	os.Stdout = oldStdout

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

	if !gitCommitCalled {
		t.Error("git commit was not called")
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
}

func TestRunCleanup_ValidateFailed_Skips(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	drushCalled := false
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			drushCalled = true
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunCleanup([]string{"/tmp/test", "--validate-failed"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if drushCalled {
		t.Error("drush should NOT be called when validate failed")
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
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
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
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

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunCleanup([]string{dir, "--validate-passed"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if drushUninstallCalled {
		t.Error("drush pm:uninstall should NOT be called when upgrade_status is not in composer.json")
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
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
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
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

	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	err := RunCleanup([]string{dir, "--validate-passed"})

	w.Close()
	os.Stdout = oldStdout

	if err == nil {
		t.Fatal("expected error when drush fails, got nil")
	}
	if composerRemoveCalled {
		t.Error("composer remove should NOT be called when drush fails")
	}
}
