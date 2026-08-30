package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// RunCleanup executes the post-validation cleanup stage (Stage 8).
// It uninstalls upgrade_status and removes it from composer.json. Publication
// is deliberately deferred to checkpoint_commit after independent validation.
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

	output := map[string]interface{}{
		"success":       true,
		"skipped":       false,
		"changed_files": []string{"composer.json", "composer.lock"},
		"message":       "cleanup complete: upgrade_status removed; checkpoint commit required after validation",
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
