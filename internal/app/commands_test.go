package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nireneko/drup/internal/envdetect"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/scan"
	statepkg "github.com/nireneko/drup/internal/state"
	"github.com/nireneko/drup/internal/update"
)

func TestRunUpgrade_AlreadyUpToDate(t *testing.T) {
	origCheck := checkLatestFn
	origUpgrade := upgradeFn
	origVersion := Version
	t.Cleanup(func() {
		checkLatestFn = origCheck
		upgradeFn = origUpgrade
		Version = origVersion
	})

	Version = "0.2.0"
	checkLatestFn = func(owner, repo, goos, goarch string) (string, string, error) {
		return "0.2.0", "http://example.com/asset.tar.gz", nil
	}
	upgradeCalled := false
	upgradeFn = func(opts update.UpgradeOptions) error {
		upgradeCalled = true
		return nil
	}

	if err := RunUpgrade(); err != nil {
		t.Fatalf("RunUpgrade() error = %v, want nil", err)
	}
	if upgradeCalled {
		t.Error("upgradeFn should not be called when already up to date")
	}
}

func TestRunUpgrade_UpgradeErrorPropagates(t *testing.T) {
	origCheck := checkLatestFn
	origUpgrade := upgradeFn
	origVersion := Version
	t.Cleanup(func() {
		checkLatestFn = origCheck
		upgradeFn = origUpgrade
		Version = origVersion
	})

	Version = "0.1.0"
	checkLatestFn = func(owner, repo, goos, goarch string) (string, string, error) {
		return "0.2.0", "http://example.com/asset.tar.gz", nil
	}
	wantErr := errors.New("boom")
	upgradeFn = func(opts update.UpgradeOptions) error {
		return wantErr
	}

	err := RunUpgrade()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want it to wrap %v", err, wantErr)
	}
}

// Phase 3: RunUninstall tests

func TestRunUninstall_StateDrivenAdapterSelection(t *testing.T) {
	// Override stateLoadFn to return state with both claude and opencode.
	origStateLoad := stateLoadFn
	origHomeDir := osUserHomeDirFn
	origStateRemove := stateRemoveFn
	origExecutable := osExecutableFn
	t.Cleanup(func() {
		stateLoadFn = origStateLoad
		osUserHomeDirFn = origHomeDir
		stateRemoveFn = origStateRemove
		osExecutableFn = origExecutable
	})

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	stateLoadFn = func() (*statepkg.State, error) {
		return &statepkg.State{
			InstalledAgents: []string{"claude", "opencode"},
			Version:         "1.0.0",
		}, nil
	}
	stateRemoveFn = func() error { return nil }
	osExecutableFn = func() (string, error) {
		return filepath.Join(t.TempDir(), "drup"), nil
	}

	// Install something to both adapters first.
	claudeAdapter := &struct{ home string }{home}
	_ = claudeAdapter

	// Create the skill directories so Uninstall has something to remove.
	os.MkdirAll(filepath.Join(home, ".claude", "skills", "drup"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "skills", "drup", "SKILL.md"), []byte("# skill"), 0o644)
	os.MkdirAll(filepath.Join(home, ".config", "opencode", "skills", "drup"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "opencode", "skills", "drup", "SKILL.md"), []byte("# skill"), 0o644)

	// Run uninstall with --force to skip confirmation prompt.
	err := RunUninstall([]string{"--force"})
	if err != nil {
		t.Fatalf("RunUninstall error: %v", err)
	}

	// Verify both adapters' skill directories were removed.
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "drup")); !os.IsNotExist(err) {
		t.Error("Claude skill directory should be removed")
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "opencode", "skills", "drup")); !os.IsNotExist(err) {
		t.Error("OpenCode skill directory should be removed")
	}
}

func TestRunUninstall_DryRunOutput(t *testing.T) {
	origStateLoad := stateLoadFn
	origHomeDir := osUserHomeDirFn
	t.Cleanup(func() {
		stateLoadFn = origStateLoad
		osUserHomeDirFn = origHomeDir
	})

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	stateLoadFn = func() (*statepkg.State, error) {
		return &statepkg.State{
			InstalledAgents: []string{"claude"},
		}, nil
	}

	// Create skill directory so Uninstall has something to list.
	os.MkdirAll(filepath.Join(home, ".claude", "skills", "drup"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "skills", "drup", "SKILL.md"), []byte("# skill"), 0o644)

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunUninstall([]string{"--dry-run"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunUninstall --dry-run error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify dry-run output mentions paths.
	if !strings.Contains(output, "Dry-run mode") {
		t.Errorf("dry-run output should contain 'Dry-run mode', got: %s", output)
	}

	// Verify files NOT deleted.
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "drup")); os.IsNotExist(err) {
		t.Error("skill directory should NOT be removed in dry-run mode")
	}
}

func TestRunUninstall_ForceWithMissingState(t *testing.T) {
	origStateLoad := stateLoadFn
	origHomeDir := osUserHomeDirFn
	origStateRemove := stateRemoveFn
	origExecutable := osExecutableFn
	t.Cleanup(func() {
		stateLoadFn = origStateLoad
		osUserHomeDirFn = origHomeDir
		stateRemoveFn = origStateRemove
		osExecutableFn = origExecutable
	})

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	stateLoadFn = func() (*statepkg.State, error) {
		return nil, errors.New("state file not found")
	}
	stateRemoveFn = func() error { return nil }
	osExecutableFn = func() (string, error) {
		return filepath.Join(t.TempDir(), "drup"), nil
	}

	// Capture stderr for warning check.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := RunUninstall([]string{"--force"})

	w.Close()
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("RunUninstall --force should not error with missing state: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	stderrOutput := buf.String()

	if !strings.Contains(stderrOutput, "Warning") {
		t.Errorf("--force with missing state should print warning, got: %s", stderrOutput)
	}
}

func TestRunUninstall_SelfRemovalError(t *testing.T) {
	origStateLoad := stateLoadFn
	origHomeDir := osUserHomeDirFn
	origStateRemove := stateRemoveFn
	origExecutable := osExecutableFn
	t.Cleanup(func() {
		stateLoadFn = origStateLoad
		osUserHomeDirFn = origHomeDir
		stateRemoveFn = origStateRemove
		osExecutableFn = origExecutable
	})

	home := t.TempDir()
	osUserHomeDirFn = func() (string, error) { return home, nil }
	stateLoadFn = func() (*statepkg.State, error) {
		return &statepkg.State{
			InstalledAgents: []string{"claude"},
		}, nil
	}
	stateRemoveFn = func() error { return nil }

	// Return a non-existent path for the executable — os.Remove will fail.
	osExecutableFn = func() (string, error) {
		return "/nonexistent/path/drup-binary-that-does-not-exist", nil
	}

	// Create skill directory so Uninstall has something to remove.
	os.MkdirAll(filepath.Join(home, ".claude", "skills", "drup"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "skills", "drup", "SKILL.md"), []byte("# skill"), 0o644)

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := RunUninstall([]string{"--force"})

	w.Close()
	os.Stderr = oldStderr

	// Should NOT panic or return error — self-removal failure is a warning.
	if err != nil {
		t.Fatalf("RunUninstall should not error on self-removal failure: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	stderrOutput := buf.String()

	if !strings.Contains(stderrOutput, "Could not remove binary") {
		t.Errorf("should warn about binary removal failure, got: %s", stderrOutput)
	}
}

// --- Phase 2: RunUpgradeCore tests (TDD RED) ---

func TestRunUpgradeCore_MissingArg(t *testing.T) {
	err := RunUpgradeCore([]string{})
	if err == nil {
		t.Fatal("expected error for missing target version, got nil")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error = %q, want usage error", err.Error())
	}
}

func TestRunUpgradeCore_RelativePathRejected(t *testing.T) {
	// Override getwdFn to return a relative path.
	orig := getwdFn
	getwdFn = func() (string, error) { return "relative/path", nil }
	defer func() { getwdFn = orig }()

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for relative path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("error = %q, want absolute path error", err.Error())
	}
}

func TestRunUpgradeCore_PathTraversalRejected(t *testing.T) {
	// Override getwdFn to return a path with `..` segment.
	orig := getwdFn
	getwdFn = func() (string, error) { return "/tmp/../../etc", nil }
	defer func() { getwdFn = orig }()

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want path traversal error mentioning '..'", err.Error())
	}
}

func TestRunUpgradeCore_DirtyTreeReturnsErrorWithFileList(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) {
		return false, []string{"modified.txt", "staged.php"}, nil
	}
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
	}()

	// Write a composer.json so we get past that check.
	composerJSON := `{"require":{"drupal/core-recommended":"^10.3"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for dirty working tree, got nil")
	}
	if !strings.Contains(err.Error(), "dirty") {
		t.Errorf("error = %q, want dirty tree error", err.Error())
	}
	if !strings.Contains(err.Error(), "modified.txt") {
		t.Errorf("error = %q, want file list including 'modified.txt'", err.Error())
	}
}

func TestRunUpgradeCore_MissingComposerJSON(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
	}()

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for missing composer.json, got nil")
	}
	if !strings.Contains(err.Error(), "composer.json") {
		t.Errorf("error = %q, want composer.json not found error", err.Error())
	}
}

func TestRunUpgradeCore_AlreadyAtTarget(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
	}()

	// Write composer.json already at target.
	composerJSON := `{"require":{"drupal/core-recommended":"^11.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	// Capture stdout for JSON output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunUpgradeCore([]string{"11"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunUpgradeCore error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if !strings.Contains(output, "already") {
		t.Errorf("output = %q, want 'already at target' message", output)
	}
}

func TestRunUpgradeCore_DryRunOutput(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
	}()

	composerJSON := `{"require":{"drupal/core-recommended":"^10.3"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunUpgradeCore([]string{"11", "--dry-run"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunUpgradeCore --dry-run error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Parse JSON output.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["dry_run"] != true {
		t.Errorf("dry_run = %v, want true", result["dry_run"])
	}
	if result["success"] != true {
		t.Errorf("success = %v, want true", result["success"])
	}
}

func TestRunUpgradeCore_ComposerNotFound(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	origExecRun := execRunFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		if cmd == "composer" {
			return "", "", -1, errors.New("composer not found")
		}
		return "", "", 0, nil
	}
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
	}()

	// Initialize git repo and commit composer.json so coreupgrade.Apply's internal clean check passes.
	initTestGitRepo(t, dir)
	composerJSON := `{"require":{"drupal/core-recommended":"^10.3"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "add composer.json")

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for composer not found, got nil")
	}
	if !strings.Contains(err.Error(), "composer") {
		t.Errorf("error = %q, want composer not found error", err.Error())
	}
}

func TestRunUpgradeCore_DrushNotFound(t *testing.T) {
	dir := t.TempDir()
	origGetwd := getwdFn
	origIsClean := isCleanFn
	origExecRun := execRunFn
	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		if cmd == "composer" {
			return "", "", 0, nil
		}
		if cmd == "drush" {
			return "", "", -1, errors.New("drush not found")
		}
		return "", "", 0, nil
	}
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
	}()

	initTestGitRepo(t, dir)
	composerJSON := `{"require":{"drupal/core-recommended":"^10.3"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "add composer.json")

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for drush not found, got nil")
	}
	if !strings.Contains(err.Error(), "drush") {
		t.Errorf("error = %q, want drush not found error", err.Error())
	}
}

// initTestGitRepo creates a minimal git repo for testing.
// Uses the runGitCmd helper already declared in mcp_tools_test.go.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	// Create an initial commit so the repo is valid.
	os.WriteFile(filepath.Join(dir, ".gitkeep"), []byte(""), 0o644)
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")
}

// TestRunUpgradeCore_Integration tests the full upgrade-core flow with mocked exec.
func TestRunUpgradeCore_Integration(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	composerJSON := `{
    "require": {
        "drupal/core-recommended": "^10.3",
        "drupal/core": "^10.3"
    }
}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "add composer.json")

	origGetwd := getwdFn
	origIsClean := isCleanFn
	origExecRun := execRunFn
	origDetector := defaultEnvDetector
	origRunWithEnv := drupexec.RunWithEnv
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
		defaultEnvDetector = origDetector
		drupexec.RunWithEnv = origRunWithEnv
	}()

	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	defaultEnvDetector = &mockEnvDetectorDirect{}

	// Track composer calls to verify the new sequence.
	var composerCalls [][]string
	drushUpdbCalled := false
	drushStatusCalled := false

	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "composer":
			composerCalls = append(composerCalls, args)
			return "", "", 0, nil
		case cmd == "git":
			// Let real git commands pass through for coreupgrade.Apply
			return realExecRun(cmd, args...)
		default:
			return "", "", 0, nil
		}
	}

	// Mock drupexec.RunWithEnv for drush calls via cliRun
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "drush" && len(args) > 0 && args[0] == "updb":
			drushUpdbCalled = true
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "status":
			drushStatusCalled = true
			return `{"drupal-version":"11.0.0"}`, "", 0, nil
		default:
			return "", "", 0, nil
		}
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunUpgradeCore([]string{"11"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		var buf bytes.Buffer
		io.Copy(&buf, r)
		t.Fatalf("RunUpgradeCore error: %v\noutput: %s", err, buf.String())
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify composer call sequence: config, require, update.
	if len(composerCalls) < 3 {
		t.Fatalf("expected at least 3 composer calls, got %d: %v", len(composerCalls), composerCalls)
	}

	// First call: composer config policy.advisories.block false
	if composerCalls[0][0] != "config" || composerCalls[0][1] != "policy.advisories.block" || composerCalls[0][2] != "false" {
		t.Errorf("first composer call = %v, want 'config policy.advisories.block false'", composerCalls[0])
	}

	// Second call: composer require ... -W --no-update
	if composerCalls[1][0] != "require" {
		t.Errorf("second composer call = %v, want 'require ...'", composerCalls[1])
	}
	hasW := false
	hasNoUpdate := false
	for _, arg := range composerCalls[1] {
		if arg == "-W" {
			hasW = true
		}
		if arg == "--no-update" {
			hasNoUpdate = true
		}
	}
	if !hasW {
		t.Errorf("composer require call = %v, want -W flag present", composerCalls[1])
	}
	if !hasNoUpdate {
		t.Errorf("composer require call = %v, want --no-update flag present", composerCalls[1])
	}

	// Third call: composer update -W
	if composerCalls[2][0] != "update" {
		t.Errorf("third composer call = %v, want 'update -W'", composerCalls[2])
	}
	hasWUpdate := false
	for _, arg := range composerCalls[2] {
		if arg == "-W" {
			hasWUpdate = true
		}
	}
	if !hasWUpdate {
		t.Errorf("composer update call = %v, want -W flag present", composerCalls[2])
	}

	// Verify drush steps were called.
	if !drushUpdbCalled {
		t.Error("drush updb was not called")
	}
	if !drushStatusCalled {
		t.Error("drush status was not called")
	}

	// Verify JSON output.
	var result UpgradeCoreResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if !result.Success {
		t.Error("expected success=true")
	}
	if result.CurrentConstraint != "^10.3" {
		t.Errorf("current_constraint = %q, want ^10.3", result.CurrentConstraint)
	}
	if result.TargetConstraint != "^11.0" {
		t.Errorf("target_constraint = %q, want ^11.0", result.TargetConstraint)
	}
	if result.VerifiedVersion != "11.0.0" {
		t.Errorf("verified_version = %q, want 11.0.0", result.VerifiedVersion)
	}

	// Verify backup was cleaned up after success.
	backupPath := filepath.Join(dir, "composer.json.bak")
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Error("composer.json.bak should be removed after successful upgrade")
	}
}

// realExecRun wraps the real exec.Run for pass-through in integration tests.
func TestRunUpgradeCore_VersionMismatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	composerJSON := `{
        "require": {
            "drupal/core-recommended": "^10.3",
            "drupal/core": "^10.3"
        }
    }`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "add composer.json")

	origGetwd := getwdFn
	origIsClean := isCleanFn
	origExecRun := execRunFn
	origDetector := defaultEnvDetector
	origRunWithEnv := drupexec.RunWithEnv
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
		defaultEnvDetector = origDetector
		drupexec.RunWithEnv = origRunWithEnv
	}()

	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }
	defaultEnvDetector = &mockEnvDetectorDirect{}

	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "composer":
			return "", "", 0, nil
		case cmd == "git":
			return realExecRun(cmd, args...)
		default:
			return "", "", 0, nil
		}
	}

	// Mock drupexec.RunWithEnv for drush calls via cliRun
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "drush" && len(args) > 0 && args[0] == "updb":
			// Simulate: updb passed but Drupal actually didn't upgrade.
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "status":
			// Return OLD version — upgrade didn't actually take effect.
			return `{"drupal-version":"10.3.0"}`, "", 0, nil
		default:
			return "", "", 0, nil
		}
	}

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected version mismatch error, got nil")
	}
	if !strings.Contains(err.Error(), "version mismatch") {
		t.Errorf("error message does not mention version mismatch: %v", err)
	}
}

func TestRunUpgradeCore_ErrorMessageIncludesCheckpoint(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	composerJSON := `{
        "require": {
            "drupal/core-recommended": "^10.3",
            "drupal/core": "^10.3"
        }
    }`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "add composer.json")

	origGetwd := getwdFn
	origIsClean := isCleanFn
	origExecRun := execRunFn
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
	}()

	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }

	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "composer":
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "updb":
			// Simulate updb failure.
			return "", "updb failed", 1, nil
		case cmd == "git":
			return realExecRun(cmd, args...)
		default:
			return "", "", 0, nil
		}
	}

	err := RunUpgradeCore([]string{"11"})
	if err == nil {
		t.Fatal("expected error for updb failure, got nil")
	}
	// Error message should include checkpoint SHA for rollback.
	errMsg := err.Error()
	if !strings.Contains(errMsg, "checkpoint") {
		t.Errorf("error message = %q, want it to mention 'checkpoint'", errMsg)
	}
}

func realExecRun(cmd string, args ...string) (string, string, int, error) {
	return drupexec.Run(cmd, args...)
}

// --- Phase 1: RED tests for --all flag ---

func TestRunScan_PassesAllFlag(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			capturedArgs = args
			return "no errors found", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := RunScan("/tmp/test-project")
	if err != nil {
		t.Fatalf("RunScan error: %v", err)
	}

	// Verify --all flag is present in drush args.
	found := false
	for _, arg := range capturedArgs {
		if arg == "--all" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("drush args = %v, want --all flag present", capturedArgs)
	}
}

func TestRunScan_PlainTextParsing(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return `
====================

Project: token (modules/contrib/token)

  - modules/contrib/token/token.module:42
    Call to deprecated function foo().
    Rule: deprecation
`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunScan("/tmp/test-project")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunScan error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify JSON output contains expected data.
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", result["total_errors"])
	}
}

func TestRunScan_DrushExitNonZero(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return "", "drush failed: bootstrap error", 1, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := RunScan("/tmp/test-project")
	if err == nil {
		t.Fatal("expected error for non-zero drush exit, got nil")
	}
	errMsg := err.Error()
	// Error should include command, exit code, and stderr.
	if !strings.Contains(errMsg, "exit") || !strings.Contains(errMsg, "1") {
		t.Errorf("error = %q, want it to contain exit code 1", errMsg)
	}
	if !strings.Contains(errMsg, "bootstrap error") {
		t.Errorf("error = %q, want it to contain stderr 'bootstrap error'", errMsg)
	}
}

func TestRunScan_ParseFailure(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			// Return empty output — parser returns zero-result, not error.
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := RunScan("/tmp/test-project")
	// Empty output is valid (zero errors), not a parse failure.
	if err != nil {
		t.Fatalf("RunScan should not error on empty output: %v", err)
	}
}

func TestRunScan_NoFormatJSON(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			capturedArgs = args
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	_ = RunScan("/tmp/test-project")

	for _, arg := range capturedArgs {
		if arg == "--format=json" {
			t.Errorf("drush args = %v, must NOT contain --format=json", capturedArgs)
		}
	}
}

// --- CLI validate and apply-patch tests ---

func TestRunValidate_CleanProject(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return "[warning] No errors found.", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunValidate([]string{"/tmp/test-project"})

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunValidate error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["total_errors"].(float64) != 0 {
		t.Errorf("total_errors = %v, want 0", result["total_errors"])
	}
}

func TestRunValidate_ErrorsRemain(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return `
Project: mymod (modules/custom/mymod)

  - modules/custom/mymod/mymod.module:5
    Deprecated function foo().
    Rule: deprecation
`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunValidate([]string{"/tmp/test-project"})

	w.Close()
	os.Stdout = oldStdout

	// Should return error (exit 1) when errors remain.
	if err == nil {
		t.Fatal("expected error for remaining errors, got nil")
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", result["total_errors"])
	}
}

func TestRunValidate_MissingArgs(t *testing.T) {
	err := RunValidate([]string{})
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error = %q, want usage error", err.Error())
	}
}

func TestRunApplyPatch_MissingArgs(t *testing.T) {
	err := RunApplyPatch([]string{})
	if err == nil {
		t.Fatal("expected error for missing args, got nil")
	}
	if !strings.Contains(err.Error(), "usage") {
		t.Errorf("error = %q, want usage error", err.Error())
	}
}

func TestRunValidate_Dispatch(t *testing.T) {
	// Verify validate command is dispatched correctly.
	err := Run([]string{"validate"})
	// Will fail because no args, but should not be "unknown command".
	if err != nil && strings.Contains(err.Error(), "unknown command") {
		t.Error("validate should be a known command")
	}
}

func TestRunApplyPatch_Dispatch(t *testing.T) {
	// Verify apply-patch command is dispatched correctly.
	err := Run([]string{"apply-patch"})
	// Will fail because no args, but should not be "unknown command".
	if err != nil && strings.Contains(err.Error(), "unknown command") {
		t.Error("apply-patch should be a known command")
	}
}

// Phase 5: PHP 8.4 Deprecation Suppression - RED tests

func TestIsPHP84OrLater(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"8.4.2", true},
		{"8.3.0", false},
		{"8.4.0", true},
		{"8.5.0", true},
		{"7.4.0", false},
		{"9.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got := isPHP84OrLater(tt.version)
			if got != tt.want {
				t.Errorf("isPHP84OrLater(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}

func TestPatchSettingsPHP_Idempotent(t *testing.T) {
	dir := t.TempDir()
	settingsPath := filepath.Join(dir, "web", "sites", "default", "settings.php")
	os.MkdirAll(filepath.Dir(settingsPath), 0o755)

	// Create initial settings.php with DDEV include block
	initialContent := `<?php
// DDEV include block
if (file_exists(__DIR__ . '/settings.ddev.php')) {
  include __DIR__ . '/settings.ddev.php';
}
// Other settings
$settings['some_key'] = 'value';
`
	os.WriteFile(settingsPath, []byte(initialContent), 0o644)

	// First patch
	err := patchSettingsPHP(dir)
	if err != nil {
		t.Fatalf("first patchSettingsPHP error: %v", err)
	}

	// Verify backup was created
	backupPath := settingsPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup file was not created")
	}

	// Verify suppression line was added
	content, _ := os.ReadFile(settingsPath)
	if !strings.Contains(string(content), "error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)") {
		t.Error("suppression line was not added")
	}

	// Second patch (should be idempotent)
	err = patchSettingsPHP(dir)
	if err != nil {
		t.Fatalf("second patchSettingsPHP error: %v", err)
	}

	// Verify content hasn't changed (idempotent)
	content2, _ := os.ReadFile(settingsPath)
	if string(content) != string(content2) {
		t.Error("second patch changed the file (not idempotent)")
	}
}

// Phase 6: Report Data Collection - RED test

func TestRunReport_PopulatesRealData(t *testing.T) {
	dir := t.TempDir()

	// Mock DoValidate to return 15 errors
	origDoValidate := doValidateFn
	doValidateFn = func(projectPath, module string) (*scan.ScanResult, []scan.DepError, error) {
		result := &scan.ScanResult{
			TotalErrs: 15,
			Modules: []scan.ModuleStatus{
				{
					Name: "custom_module",
					Type: scan.ClassCustom,
					Errors: []scan.DepError{
						{
							File:     "modules/custom/mymod/mymod.module",
							Line:     42,
							Message:  "Deprecated function call",
							Rule:     "deprecation",
							Severity: "warning",
							Source:   "upgrade_status",
						},
					},
				},
			},
		}
		// Return 15 errors total
		errors := make([]scan.DepError, 15)
		for i := 0; i < 15; i++ {
			errors[i] = scan.DepError{
				File:     fmt.Sprintf("modules/custom/mymod/file%d.module", i),
				Line:     i + 1,
				Message:  fmt.Sprintf("Error %d", i),
				Rule:     "deprecation",
				Severity: "warning",
				Source:   "upgrade_status",
			}
		}
		return result, errors, nil
	}
	defer func() { doValidateFn = origDoValidate }()

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunReport(dir)

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunReport error: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	// Verify report files were created
	jsonPath := filepath.Join(dir, "drup-report.json")
	mdPath := filepath.Join(dir, "drup-report.md")

	if _, err := os.Stat(jsonPath); os.IsNotExist(err) {
		t.Error("JSON report was not created")
	}
	if _, err := os.Stat(mdPath); os.IsNotExist(err) {
		t.Error("Markdown report was not created")
	}

	// Read and verify JSON report
	jsonData, _ := os.ReadFile(jsonPath)
	var reportData map[string]interface{}
	if err := json.Unmarshal(jsonData, &reportData); err != nil {
		t.Fatalf("failed to parse JSON report: %v", err)
	}

	totalErrors, ok := reportData["total_errors"].(float64)
	if !ok {
		t.Fatal("total_errors not found in report")
	}
	if totalErrors != 15 {
		t.Errorf("total_errors = %v, want 15", totalErrors)
	}

	// Verify output mentions the reports
	if !strings.Contains(output, "drup-report.json") {
		t.Error("output does not mention JSON report")
	}
	if !strings.Contains(output, "drup-report.md") {
		t.Error("output does not mention markdown report")
	}
}

// --- Phase 1: Exit code 3 semantic handling (RED) ---

func TestIsScanExitOK(t *testing.T) {
	tests := []struct {
		name     string
		exitCode int
		want     bool
	}{
		{"exit 0 is OK", 0, true},
		{"exit 3 is OK (findings exist)", 3, true},
		{"exit 1 is NOT OK", 1, false},
		{"exit 2 is NOT OK", 2, false},
		{"exit 4 is NOT OK", 4, false},
		{"exit -1 is NOT OK", -1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isScanExitOK(tt.exitCode)
			if got != tt.want {
				t.Errorf("isScanExitOK(%d) = %v, want %v", tt.exitCode, got, tt.want)
			}
		})
	}
}

func TestRunScan_ExitCode3WithFindings(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return `
Project: token (modules/contrib/token)

  - modules/contrib/token/token.module:42
    Call to deprecated function foo().
    Rule: deprecation
`, "", 3, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := RunScan("/tmp/test-project")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("RunScan should succeed with exit code 3 and valid stdout: %v", err)
	}

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}
	if result["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", result["total_errors"])
	}
}

func TestRunScan_ExitCode3EmptyStdoutIsError(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			return "", "drush crashed: bootstrap failed", 3, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := RunScan("/tmp/test-project")
	if err == nil {
		t.Fatal("expected error for exit code 3 with empty stdout, got nil")
	}
	if !strings.Contains(err.Error(), "code 3") {
		t.Errorf("error = %q, want it to mention exit code 3", err.Error())
	}
	if !strings.Contains(err.Error(), "bootstrap failed") {
		t.Errorf("error = %q, want it to contain stderr", err.Error())
	}
}

// --- Phase 2: DDEV-aware execution (RED) ---

func TestCliRun_DetectsEnvironment(t *testing.T) {
	// Override defaultEnvDetector to return DDEV prefix.
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDDEV{}
	defer func() { defaultEnvDetector = origDetector }()

	// Override RunWithEnv to capture the prefix.
	origRunWithEnv := drupexec.RunWithEnv
	var capturedPrefix []string
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		capturedPrefix = prefix
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	_, _, _, _ = cliRun("/tmp/test-project", "drush", "status")

	if len(capturedPrefix) == 0 || capturedPrefix[0] != "ddev" {
		t.Errorf("capturedPrefix = %v, want [ddev]", capturedPrefix)
	}
}

func TestCliRun_DirectEnvironment(t *testing.T) {
	// Override defaultEnvDetector to return direct (empty prefix).
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origRunWithEnv := drupexec.RunWithEnv
	var capturedPrefix []string
	drupexec.RunWithEnv = func(prefix []string, cmd string, args ...string) (string, string, int, error) {
		capturedPrefix = prefix
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	_, _, _, _ = cliRun("/tmp/test-project", "drush", "status")

	if len(capturedPrefix) != 0 {
		t.Errorf("capturedPrefix = %v, want empty for direct environment", capturedPrefix)
	}
}

// mockEnvDetectorDDEV returns DDEV environment for testing.
type mockEnvDetectorDDEV struct{}

func (m *mockEnvDetectorDDEV) Detect(projectPath string, forceDetect bool) (*envdetect.Detection, error) {
	return &envdetect.Detection{
		Environment:   envdetect.EnvDdev,
		CommandPrefix: []string{"ddev"},
		DetectedAt:    time.Now(),
	}, nil
}

// mockEnvDetectorDirect returns direct environment for testing.
type mockEnvDetectorDirect struct{}

func (m *mockEnvDetectorDirect) Detect(projectPath string, forceDetect bool) (*envdetect.Detection, error) {
	return &envdetect.Detection{
		Environment:   envdetect.EnvDirect,
		CommandPrefix: []string{},
		DetectedAt:    time.Now(),
	}, nil
}
