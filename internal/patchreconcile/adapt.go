package patchreconcile

import (
	"fmt"
	"os"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// AdaptResult describes the outcome of checking whether an upstream patch
// still applies cleanly, and — when it does not — the locally-adapted
// replacement that preserves the original issue reference.
type AdaptResult struct {
	// Applied is true when patchContent applies cleanly as-is (git apply
	// --check succeeded); no adaptation was necessary.
	Applied bool `json:"applied"`
	// LocallyAdapted is true when the upstream patch was rejected and a
	// local adaptation was generated instead.
	LocallyAdapted bool `json:"locally_adapted"`
	// IssueReference is the original drupal.org issue/PR reference, always
	// preserved regardless of outcome.
	IssueReference string `json:"issue_reference"`
	// AdaptedPatch is only set when LocallyAdapted is true: the original
	// diff content prefixed with a header that preserves IssueReference and
	// explains why it was adapted.
	AdaptedPatch string `json:"adapted_patch,omitempty"`
	// ComposerDescription is a ready-to-use "extra.patches" description
	// string that references IssueReference. Only set when LocallyAdapted.
	ComposerDescription string `json:"composer_description,omitempty"`
}

// runGitApplyCheck runs `git apply --check` for the patch file at patchFile
// against the repository at projectPath.
// Package-level var for testability.
var runGitApplyCheck = func(projectPath, patchFile string) (exitCode int, stderr string, err error) {
	_, stderr, exitCode, err = drupexec.Run("git", "-C", projectPath, "apply", "--check", patchFile)
	return exitCode, stderr, err
}

// Adapt verifies whether patchContent still applies cleanly at projectPath
// using `git apply --check`. When it does not, Adapt reproduces the same
// diff content as a locally-adapted patch, preserving issueReference in both
// the patch file header and a composer.json-ready description string — per
// the Local Patch Adaptation and Issue Reference Preservation requirements.
//
// This is a deterministic, non-LLM transformation: it does NOT attempt to
// resolve hunk conflicts against the current code. It surfaces the original
// diff intent with a traceable reference so a human or a dedicated agent can
// complete the adaptation.
func Adapt(projectPath, patchContent, issueReference string) (*AdaptResult, error) {
	if projectPath == "" {
		return nil, fmt.Errorf("project_path must not be empty")
	}
	if patchContent == "" {
		return nil, fmt.Errorf("patch_content must not be empty")
	}
	if issueReference == "" {
		return nil, fmt.Errorf("issue_reference must not be empty")
	}

	tmp, err := os.CreateTemp("", "drup-patch-*.patch")
	if err != nil {
		return nil, fmt.Errorf("create temp patch file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(patchContent); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp patch file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp patch file: %w", err)
	}

	exitCode, stderr, err := runGitApplyCheck(projectPath, tmp.Name())
	if err != nil {
		return nil, fmt.Errorf("git apply --check: %w", err)
	}

	if exitCode == 0 {
		return &AdaptResult{
			Applied:        true,
			LocallyAdapted: false,
			IssueReference: issueReference,
		}, nil
	}

	header := fmt.Sprintf(
		"# Locally adapted patch — preserves issue %s\n# Upstream patch no longer applied cleanly (git apply --check: %s)\n",
		issueReference, strings.TrimSpace(stderr),
	)
	description := fmt.Sprintf("Locally adapted from upstream patch, issue %s (auto-adapted by drup patch_reconcile)", issueReference)

	return &AdaptResult{
		Applied:             false,
		LocallyAdapted:      true,
		IssueReference:      issueReference,
		AdaptedPatch:        header + patchContent,
		ComposerDescription: description,
	}, nil
}
