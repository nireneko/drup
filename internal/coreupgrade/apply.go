package coreupgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
)

// ApplyResult is returned by Apply.
type ApplyResult struct {
	Success bool `json:"success"`
	// Report holds the composer.json diff preview (dry-run) or a human
	// readable explanation when Success is false.
	Report string `json:"report"`
	// RollbackCheckpoint is the git commit SHA created immediately before the
	// mutation. Empty when dryRun was true or nothing needed to change.
	RollbackCheckpoint string `json:"rollback_checkpoint,omitempty"`
	Stderr             string `json:"stderr,omitempty"`
}

// validateProjectPath enforces the same absolute-path, no-traversal guard
// used elsewhere in drup (see internal/app upgrade_scan handler).
func validateProjectPath(projectPath string) error {
	if projectPath == "" {
		return fmt.Errorf("project_path must not be empty")
	}
	if !filepath.IsAbs(projectPath) {
		return fmt.Errorf("project_path must be an absolute path: %s", projectPath)
	}
	if strings.Contains(projectPath, "..") {
		return fmt.Errorf("project_path must not contain '..' segments")
	}
	return nil
}

// Apply updates the drupal/core constraint(s) in composer.json at projectPath
// to targetVersion.
//
//   - dryRun=true: returns the composer.json diff preview only. No file or git
//     mutation happens.
//   - dryRun=false: requires a clean git working tree, creates a checkpoint
//     commit BEFORE mutating composer.json (so Rollback can restore the prior
//     state), then writes the new constraint and returns the checkpoint SHA.
func Apply(projectPath, targetVersion string, dryRun bool) (*ApplyResult, error) {
	if err := validateProjectPath(projectPath); err != nil {
		return nil, err
	}
	if targetVersion == "" {
		return nil, fmt.Errorf("target_version must not be empty")
	}

	composerPath := filepath.Join(projectPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	targetMajor, err := majorVersion(targetVersion)
	if err != nil {
		return nil, fmt.Errorf("parse target version %q: %w", targetVersion, err)
	}
	constraint := fmt.Sprintf("^%d.0", targetMajor)

	diff, changed, err := PreviewComposerPatch(data, constraint)
	if err != nil {
		return nil, err
	}
	if !changed {
		return &ApplyResult{Success: false, Report: "no drupal/core requirement change needed; already at target constraint or no drupal/core requirement present"}, nil
	}

	if dryRun {
		return &ApplyResult{Success: true, Report: diff}, nil
	}

	clean, dirtyFiles, err := gitops.IsClean(projectPath)
	if err != nil {
		return nil, fmt.Errorf("check git status: %w", err)
	}
	if !clean {
		return &ApplyResult{
			Success: false,
			Report:  fmt.Sprintf("working tree is dirty; commit or stash changes first: %s", strings.Join(dirtyFiles, ", ")),
		}, nil
	}

	checkpointSHA, err := createCheckpoint(projectPath, fmt.Sprintf("checkpoint: before core upgrade to %s", targetVersion))
	if err != nil {
		return nil, fmt.Errorf("create checkpoint commit: %w", err)
	}

	updated, err := applyConstraint(data, constraint)
	if err != nil {
		return nil, fmt.Errorf("apply constraint: %w", err)
	}
	if err := os.WriteFile(composerPath, updated, 0o644); err != nil {
		return nil, fmt.Errorf("write composer.json: %w", err)
	}

	return &ApplyResult{
		Success:            true,
		Report:             diff,
		RollbackCheckpoint: checkpointSHA,
	}, nil
}

// createCheckpoint records an empty commit marking the pre-mutation state.
// Callers MUST have already verified the tree is clean, so no actual content
// changes are staged — the commit exists purely as a durable rollback anchor.
func createCheckpoint(projectPath, message string) (string, error) {
	_, stderr, exitCode, err := drupexec.Run("git", "-C", projectPath, "commit", "--allow-empty", "-m", message)
	if err != nil {
		return "", fmt.Errorf("git commit --allow-empty: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git commit --allow-empty: exit %d: %s", exitCode, stderr)
	}

	stdout, stderr, exitCode, err := drupexec.Run("git", "-C", projectPath, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse HEAD: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git rev-parse HEAD: exit %d: %s", exitCode, stderr)
	}
	return strings.TrimSpace(stdout), nil
}

// applyConstraint rewrites every drupal/core / drupal/core-* require entry to
// newConstraint and returns the re-marshaled composer.json content.
func applyConstraint(composerJSON []byte, newConstraint string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(composerJSON, &doc); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	var require map[string]string
	raw, ok := doc["require"]
	if !ok {
		return composerJSON, nil
	}
	if err := json.Unmarshal(raw, &require); err != nil {
		return nil, fmt.Errorf("parse composer.json require: %w", err)
	}

	for pkg := range require {
		if pkg == drupalCorePackage || strings.HasPrefix(pkg, drupalCorePackage+"-") {
			require[pkg] = newConstraint
		}
	}

	newRequire, err := json.Marshal(require)
	if err != nil {
		return nil, fmt.Errorf("marshal require: %w", err)
	}
	doc["require"] = newRequire

	return json.MarshalIndent(doc, "", "    ")
}
