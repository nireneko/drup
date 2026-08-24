package gitops

import (
	"fmt"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// runCommand is the function used to execute git commands.
// Package-level var for testability — tests can override to avoid real git calls.
var runCommand = drupexec.Run

// IsClean checks if the git working tree at path is clean.
// Returns (clean, changedFiles, error).
func IsClean(path string) (bool, []string, error) {
	stdout, stderr, exitCode, err := runCommand("git", "-C", path, "status", "--porcelain")
	if err != nil {
		return false, nil, fmt.Errorf("git status: %w", err)
	}
	if exitCode != 0 {
		return false, nil, fmt.Errorf("git status: exit %d: %s", exitCode, stderr)
	}

	stdout = strings.TrimSpace(stdout)
	if stdout == "" {
		return true, nil, nil
	}

	files := strings.Split(stdout, "\n")
	return false, files, nil
}

// EnsureBranch creates and checks out the named branch.
// If the branch already exists, it just checks it out.
func EnsureBranch(path, name string) error {
	// Check if branch exists.
	_, _, exitCode, _ := runCommand("git", "-C", path, "rev-parse", "--verify", name)
	if exitCode != 0 {
		// Branch doesn't exist — create and checkout.
		_, stderr, exitCode, err := runCommand("git", "-C", path, "checkout", "-b", name)
		if err != nil {
			return fmt.Errorf("git checkout -b: %w", err)
		}
		if exitCode != 0 {
			return fmt.Errorf("git checkout -b: exit %d: %s", exitCode, stderr)
		}
		return nil
	}

	// Branch exists — just checkout.
	_, stderr, exitCode, err := runCommand("git", "-C", path, "checkout", name)
	if err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("git checkout: exit %d: %s", exitCode, stderr)
	}
	return nil
}

// Commit stages exactly the declared files, verifies that the resulting
// staged set does not contain anything outside that declared list, and
// creates a commit with the given message. Returns the commit hash on
// success.
//
// The verification step exists because `git add -- <declared files>` only
// controls what THIS call newly stages — it says nothing about changes
// already sitting in the index from an earlier, unrelated operation. A
// caller that scopes its own intent to a handful of files must not silently
// commit whatever else happens to be staged; scoped commits replace
// `git add -A` (which stages everything indiscriminately) precisely to
// avoid that. So after staging, the helper re-reads the index and aborts
// (git reset + a descriptive error) if it finds paths outside the declared
// set, rather than trusting that staging was clean.
func Commit(path, msg string, files []string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("nothing to commit: no files declared")
	}

	// Stage the declared files.
	addArgs := make([]string, 0, 4+len(files))
	addArgs = append(addArgs, "-C", path, "add", "--")
	addArgs = append(addArgs, files...)
	_, stderr, exitCode, err := runCommand("git", addArgs...)
	if err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git add: exit %d: %s", exitCode, stderr)
	}

	// Verify the staged set is a subset of the declared files before
	// committing anything.
	stdout, stderr, exitCode, err := runCommand("git", "-C", path, "diff", "--cached", "--name-only")
	if err != nil {
		return "", fmt.Errorf("git diff --cached: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git diff --cached: exit %d: %s", exitCode, stderr)
	}

	declared := make(map[string]bool, len(files))
	for _, f := range files {
		declared[f] = true
	}

	var unexpected []string
	for _, staged := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if staged == "" {
			continue
		}
		if !declared[staged] {
			unexpected = append(unexpected, staged)
		}
	}

	if len(unexpected) > 0 {
		// Unstage everything rather than committing an unknown mix of
		// declared and unexpected changes.
		_, _, _, _ = runCommand("git", "-C", path, "reset")
		return "", fmt.Errorf("scoped commit aborted: unexpected staged file(s) outside declared scope: %s", strings.Join(unexpected, ", "))
	}

	// Commit.
	_, stderr, exitCode, err = runCommand("git", "-C", path, "commit", "-m", msg)
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git commit: exit %d: %s", exitCode, stderr)
	}

	// Get commit hash.
	stdout, stderr, exitCode, err = runCommand("git", "-C", path, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git rev-parse: exit %d: %s", exitCode, stderr)
	}

	return strings.TrimSpace(stdout), nil
}
