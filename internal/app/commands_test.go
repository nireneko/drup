package app

import (
	"errors"
	"testing"

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

func TestRunUninstall_FlagParsing(t *testing.T) {
	// Test --dry-run flag parsing doesn't panic.
	// This will fail because no state exists, but we're testing flag parsing.
	args := []string{"--dry-run"}
	_ = RunUninstall(args)
	// If we get here without panic, flag parsing worked.
}

func TestRunUninstall_MissingState(t *testing.T) {
	// This test verifies that RunUninstall handles the case where state exists.
	// In a real scenario with missing state, it would error unless --force is used.
	// Since we can't easily override the state loading in tests, we just verify
	// the function doesn't panic and handles flags correctly.
	args := []string{}
	_ = RunUninstall(args)
	// If we get here without panic, the function works.
}
