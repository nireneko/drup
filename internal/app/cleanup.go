package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// RunCleanup executes the post-validation cleanup stage (Stage 8).
// It uninstalls upgrade_status, removes it from composer.json, and commits.
// Args: [project-path] [--validate-passed|--validate-failed]
func RunCleanup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: drup cleanup <project-path> [--validate-passed|--validate-failed]")
	}

	projectPath := args[0]
	validatePassed := false
	for _, arg := range args[1:] {
		if arg == "--validate-passed" {
			validatePassed = true
		}
	}

	if !validatePassed {
		output := map[string]interface{}{
			"success": true,
			"skipped": true,
			"message": "cleanup skipped: validation failed",
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Check if upgrade_status is in composer.json (idempotent).
	if !hasUpgradeStatus(projectPath) {
		output := map[string]interface{}{
			"success": true,
			"skipped": true,
			"message": "cleanup: nothing to do — upgrade_status not found",
		}
		data, _ := json.MarshalIndent(output, "", "  ")
		fmt.Println(string(data))
		return nil
	}

	// Step 1: drush pm:uninstall upgrade_status -y.
	_, stderr, exitCode, err := cliRun(projectPath, "drush", "pm:uninstall", "upgrade_status", "-y", "--root="+projectPath)
	if err != nil {
		return fmt.Errorf("drush pm:uninstall: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("drush pm:uninstall failed (exit %d): %s", exitCode, stderr)
	}

	// Step 2: composer remove drupal/upgrade_status.
	_, stderr, exitCode, err = cliRun(projectPath, "composer", "remove", "drupal/upgrade_status")
	if err != nil {
		return fmt.Errorf("composer remove: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("composer remove failed (exit %d): %s", exitCode, stderr)
	}

	// Step 3: git add + commit.
	_, stderr, exitCode, err = drupexec.Run("git", "-C", projectPath, "add", "-A")
	if err != nil || exitCode != 0 {
		return fmt.Errorf("git add failed: %s", stderr)
	}

	commitMsg := "chore(cleanup): remove upgrade_status post D11 migration"
	_, stderr, exitCode, err = drupexec.Run("git", "-C", projectPath, "commit", "-m", commitMsg)
	if err != nil || exitCode != 0 {
		// Commit may fail if nothing changed — not a hard error.
		if !strings.Contains(stderr, "nothing to commit") {
			return fmt.Errorf("git commit failed: %s", stderr)
		}
	}

	output := map[string]interface{}{
		"success": true,
		"skipped": false,
		"message": "cleanup complete: upgrade_status removed",
	}
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
	return nil
}

// hasUpgradeStatus checks if drupal/upgrade_status is in composer.json.
func hasUpgradeStatus(projectPath string) bool {
	data, err := os.ReadFile(filepath.Join(projectPath, "composer.json"))
	if err != nil {
		return false
	}
	return strings.Contains(string(data), "drupal/upgrade_status")
}
