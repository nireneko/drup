package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
