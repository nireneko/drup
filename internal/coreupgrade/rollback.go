package coreupgrade

import (
	"fmt"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// Rollback restores composer.json (and composer.lock, if it existed at the
// checkpoint) at projectPath to the content captured at checkpointSHA. Used
// after a core-version bump when a follow-up step (e.g. composer install)
// fails and the mutation must be undone.
func Rollback(projectPath, checkpointSHA string) error {
	if err := validateProjectPath(projectPath); err != nil {
		return err
	}
	if checkpointSHA == "" {
		return fmt.Errorf("checkpoint_sha must not be empty")
	}

	if _, stderr, exitCode, err := drupexec.Run("git", "-C", projectPath, "cat-file", "-e", checkpointSHA); err != nil {
		return fmt.Errorf("verify checkpoint commit: %w", err)
	} else if exitCode != 0 {
		return fmt.Errorf("checkpoint commit %q not found: %s", checkpointSHA, stderr)
	}

	if _, stderr, exitCode, err := drupexec.Run("git", "-C", projectPath, "checkout", checkpointSHA, "--", "composer.json"); err != nil {
		return fmt.Errorf("git checkout composer.json: %w", err)
	} else if exitCode != 0 {
		return fmt.Errorf("git checkout composer.json failed: %s", strings.TrimSpace(stderr))
	}

	// composer.lock is optional — only restore it if it existed at the checkpoint.
	if _, _, existsExit, _ := drupexec.Run("git", "-C", projectPath, "cat-file", "-e", checkpointSHA+":composer.lock"); existsExit == 0 {
		if _, stderr, exitCode, err := drupexec.Run("git", "-C", projectPath, "checkout", checkpointSHA, "--", "composer.lock"); err != nil {
			return fmt.Errorf("git checkout composer.lock: %w", err)
		} else if exitCode != 0 {
			return fmt.Errorf("git checkout composer.lock failed: %s", strings.TrimSpace(stderr))
		}
	}

	return nil
}
