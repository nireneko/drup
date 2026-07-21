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
