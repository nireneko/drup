package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	drupexec "github.com/nireneko/drup/internal/exec"
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
	defer func() {
		getwdFn = origGetwd
		isCleanFn = origIsClean
		execRunFn = origExecRun
	}()

	getwdFn = func() (string, error) { return dir, nil }
	isCleanFn = func(path string) (bool, []string, error) { return true, nil, nil }

	// Track composer calls to verify the new sequence.
	var composerCalls [][]string
	drushUpdbCalled := false
	drushStatusCalled := false

	execRunFn = func(cmd string, args ...string) (string, string, int, error) {
		switch {
		case cmd == "composer":
			composerCalls = append(composerCalls, args)
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "updb":
			drushUpdbCalled = true
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "status":
			drushStatusCalled = true
			return `{"drupal-version":"11.0.0"}`, "", 0, nil
		case cmd == "git":
			// Let real git commands pass through for coreupgrade.Apply
			return realExecRun(cmd, args...)
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
			// Simulate: updb passed but Drupal actually didn't upgrade.
			return "", "", 0, nil
		case cmd == "drush" && len(args) > 0 && args[0] == "status":
			// Return OLD version — upgrade didn't actually take effect.
			return `{"drupal-version":"10.3.0"}`, "", 0, nil
		case cmd == "git":
			return realExecRun(cmd, args...)
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
	origRun := drupexec.Run
	var capturedArgs []string
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			capturedArgs = args
			return `{}`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.Run = origRun }()

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
