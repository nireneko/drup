package coreupgrade

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/session"
	"github.com/nireneko/drup/internal/upgradeplan"
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

// ValidateProjectPath resolves projectPath through the shared
// session.ResolveSymlinks canonical-root helper (absolute, no traversal,
// symlink-evaluated), returning the resolved path callers must use for every
// subsequent file or git operation. This is the same helper CLI and MCP
// entry points across the codebase share (see specs/agent-session's
// "Canonical Root Resolution" requirement), so a symlinked and
// non-symlinked call for the same project always resolve identically.
func ValidateProjectPath(projectPath string) (string, error) {
	return session.ResolveSymlinks(projectPath)
}

// Apply updates the drupal/core constraint(s) in composer.json at projectPath
// to targetVersion.
//
//   - dryRun=true: returns the composer.json diff preview only. No file or git
//     mutation happens.
//   - dryRun=false: requires a clean git working tree, creates a checkpoint
//     commit BEFORE mutating composer.json (so Rollback can restore the prior
//     state), then writes the new constraint and returns the checkpoint SHA.
//
// Apply rewrites the core requirement and runs the update. allowDirty lets it
// proceed over uncommitted work: the checkpoint commit it takes first captures
// that state, which is exactly what a rollback needs. Without it the command
// was unusable at the end of a pipeline whose earlier stages leave changes
// behind by design.
func Apply(projectPath, targetVersion string, dryRun, allowDirty, force bool) (*ApplyResult, error) {
	resolvedPath, err := ValidateProjectPath(projectPath)
	if err != nil {
		return nil, err
	}
	if targetVersion == "" {
		return nil, fmt.Errorf("target_version must not be empty")
	}

	composerPath := filepath.Join(resolvedPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	targetMajor, err := upgradeplan.ParseMajor(targetVersion)
	if err != nil {
		return nil, fmt.Errorf("parse target version %q: %w", targetVersion, err)
	}
	currentMajor, err := composerCoreMajor(data)
	if err != nil {
		return nil, err
	}
	current, err := upgradeplan.NewMajor(currentMajor)
	if err != nil {
		return nil, err
	}
	plan, err := upgradeplan.Build(current, targetMajor, upgradeplan.KnownCatalog())
	if err != nil {
		return nil, err
	}
	if plan.NoOp() {
		return &ApplyResult{Success: true, Report: "already at target; no changes made"}, nil
	}
	return ApplyStep(resolvedPath, plan.Steps[0], dryRun, allowDirty, force)
}

// ApplyStep applies one validated immediate step. The upgradeplan domain owns
// the transition invariant; this package verifies only that composer.json is
// at the declared source before it performs an effect.
func ApplyStep(projectPath string, step upgradeplan.Step, dryRun, allowDirty, force bool) (*ApplyResult, error) {
	if err := step.Validate(); err != nil {
		return nil, fmt.Errorf("invalid upgrade step: %w", err)
	}
	resolvedPath, err := ValidateProjectPath(projectPath)
	if err != nil {
		return nil, err
	}
	projectPath = resolvedPath
	composerPath := filepath.Join(projectPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}
	currentMajor, err := composerCoreMajor(data)
	if err != nil {
		return nil, err
	}
	if upgradeplan.Major(currentMajor) != step.From {
		return nil, fmt.Errorf("composer.json is at Drupal major %d, but step requires source major %d", currentMajor, step.From)
	}
	constraint := step.Constraint()

	diff, changed, err := PreviewComposerPatch(data, constraint)
	if err != nil {
		return nil, err
	}
	if !changed && !force {
		return &ApplyResult{Success: false, Report: "no drupal/core requirement change needed; already at target constraint or no drupal/core requirement present"}, nil
	}
	if !changed {
		// The constraint is already at the target while the installed code is
		// not: what a killed or failed run leaves behind. There is nothing to
		// rewrite, but the resolution still has to happen, so carry on.
		diff = "constraint already at target; resolving the installed version"
	}

	if dryRun {
		return &ApplyResult{Success: true, Report: diff}, nil
	}

	if !allowDirty {
		clean, dirtyFiles, err := gitops.IsClean(projectPath)
		if err != nil {
			return nil, fmt.Errorf("check git status: %w", err)
		}
		if !clean {
			return &ApplyResult{
				Success: false,
				Report: fmt.Sprintf("working tree has %d uncommitted changes; commit or stash them, or pass --allow-dirty to fold them into the checkpoint: %s",
					len(dirtyFiles), strings.Join(dirtyFiles, ", ")),
			}, nil
		}
	}

	checkpointSHA, err := createCheckpoint(projectPath, fmt.Sprintf("checkpoint: before core upgrade to %d", step.To))
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

// ApplyPlan consumes a domain plan without reimplementing its transition
// rules. Effects are intentionally limited to one step; callers must persist
// and execute multi-step plans one step at a time.
func ApplyPlan(projectPath string, plan upgradeplan.Plan, dryRun, allowDirty, force bool) (*ApplyResult, error) {
	if plan.NoOp() {
		return &ApplyResult{Success: true, Report: "already at target; no changes made"}, nil
	}
	if len(plan.Steps) != 1 {
		return nil, fmt.Errorf("upgrade plan has %d steps; execute one validated step at a time", len(plan.Steps))
	}
	return ApplyStep(projectPath, plan.Steps[0], dryRun, allowDirty, force)
}

// CurrentMajor reads the Drupal core major declared in composer.json.
func CurrentMajor(projectPath string) (upgradeplan.Major, error) {
	resolvedPath, err := ValidateProjectPath(projectPath)
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(filepath.Join(resolvedPath, "composer.json"))
	if err != nil {
		return 0, fmt.Errorf("read composer.json: %w", err)
	}
	major, err := composerCoreMajor(data)
	if err != nil {
		return 0, err
	}
	return upgradeplan.NewMajor(major)
}

func composerCoreMajor(composerJSON []byte) (int, error) {
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(composerJSON, &doc); err != nil {
		return 0, fmt.Errorf("parse composer.json: %w", err)
	}
	for _, pkg := range []string{"drupal/core-recommended", drupalCorePackage} {
		if constraint, ok := doc.Require[pkg]; ok {
			major, err := MajorVersion(constraint)
			if err != nil {
				return 0, fmt.Errorf("parse %s constraint %q: %w", pkg, constraint, err)
			}
			return major, nil
		}
	}
	return 0, fmt.Errorf("composer.json has no drupal/core requirement")
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
