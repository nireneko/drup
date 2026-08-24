package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nireneko/drup/internal/gitops"
)

// RunCleanup executes the post-validation cleanup stage (Stage 8).
// It uninstalls upgrade_status, removes it from composer.json, and commits.
// Output is written to w instead of os.Stdout so callers that need to
// capture it (like the MCP cleanup handler) can pass an in-memory buffer
// directly, instead of swapping the process-global os.Stdout through an
// os.Pipe — a technique with a fixed kernel buffer that can deadlock on
// large output with no concurrent reader.
// Args: [project-path] [--validate-passed|--validate-failed]
func RunCleanup(w io.Writer, args []string) error {
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
		fmt.Fprintln(w, string(data))
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
		fmt.Fprintln(w, string(data))
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

	// Step 3: scoped commit. Only composer.json/composer.lock are ever
	// modified by the steps above (drush pm:uninstall and composer remove) —
	// declaring exactly that set instead of `git add -A` means any other
	// uncommitted change already sitting in the working tree is left alone.
	commitMsg := "chore(cleanup): remove upgrade_status post D11 migration"
	declaredPaths := []string{"composer.json", "composer.lock"}
	if _, commitErr := gitops.Commit(projectPath, commitMsg, declaredPaths); commitErr != nil {
		// Nothing to commit is not a hard error — composer/drush may have
		// left composer.json/composer.lock unchanged (e.g. already removed).
		if !strings.Contains(commitErr.Error(), "nothing to commit") {
			return fmt.Errorf("git commit failed: %w", commitErr)
		}
	}

	output := map[string]interface{}{
		"success": true,
		"skipped": false,
		"message": "cleanup complete: upgrade_status removed",
	}
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Fprintln(w, string(data))
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
