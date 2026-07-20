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

// Commit stages the specified files and creates a commit with the given message.
// Returns the commit hash on success.
func Commit(path, msg string, files []string) (string, error) {
	if len(files) == 0 {
		return "", fmt.Errorf("no files to commit")
	}

	// Stage files.
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

	// Commit.
	_, stderr, exitCode, err = runCommand("git", "-C", path, "commit", "-m", msg)
	if err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git commit: exit %d: %s", exitCode, stderr)
	}

	// Get commit hash.
	stdout, stderr, exitCode, err := runCommand("git", "-C", path, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse: %w", err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git rev-parse: exit %d: %s", exitCode, stderr)
	}

	return strings.TrimSpace(stdout), nil
}
