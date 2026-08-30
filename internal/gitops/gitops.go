package gitops

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// Candidate is a canonical snapshot of the currently publishable diff. Its
// hash includes both the ordered path list and staged/unstaged textual diffs,
// so validation evidence cannot be reused after the candidate changes.
type Candidate struct {
	Paths []string
	Hash  string
}

// CandidateForPaths validates that the complete current tracked diff is
// exactly paths, then returns the stable candidate identity. It deliberately
// rejects extra staged or unstaged files instead of quietly committing a
// subset that validation did not review.
func CandidateForPaths(path string, paths []string) (Candidate, error) {
	if len(paths) == 0 {
		return Candidate{}, fmt.Errorf("candidate paths are required")
	}
	declared := append([]string(nil), paths...)
	sort.Strings(declared)
	for i, candidate := range declared {
		if candidate == "" || strings.HasPrefix(candidate, "../") || candidate == ".." || strings.HasPrefix(candidate, "/") {
			return Candidate{}, fmt.Errorf("unsafe candidate path %q", candidate)
		}
		if i > 0 && declared[i-1] == candidate {
			return Candidate{}, fmt.Errorf("duplicate candidate path %q", candidate)
		}
	}

	actual, untracked, err := changedPaths(path)
	if err != nil {
		return Candidate{}, err
	}
	if !samePathSet(declared, actual) {
		return Candidate{}, fmt.Errorf("candidate paths do not match current diff: declared %s, actual %s", strings.Join(declared, ", "), strings.Join(actual, ", "))
	}

	unstagedArgs := append([]string{"diff", "--no-ext-diff", "--no-color", "--"}, declared...)
	unstaged, err := gitOutput(path, unstagedArgs...)
	if err != nil {
		return Candidate{}, err
	}
	stagedArgs := append([]string{"diff", "--cached", "--no-ext-diff", "--no-color", "--"}, declared...)
	staged, err := gitOutput(path, stagedArgs...)
	if err != nil {
		return Candidate{}, err
	}
	var untrackedData strings.Builder
	for _, name := range declared {
		if !untracked[name] {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(path, name))
		if readErr != nil {
			return Candidate{}, fmt.Errorf("read untracked candidate %s: %w", name, readErr)
		}
		untrackedData.WriteString(name)
		untrackedData.WriteByte('\n')
		untrackedData.Write(data)
	}
	sum := sha256.Sum256([]byte(strings.Join(declared, "\n") + "\n--unstaged--\n" + unstaged + "\n--staged--\n" + staged + "\n--untracked--\n" + untrackedData.String()))
	return Candidate{Paths: declared, Hash: hex.EncodeToString(sum[:])}, nil
}

func changedPaths(path string) ([]string, map[string]bool, error) {
	status, err := gitOutput(path, "status", "--porcelain=v1", "--untracked-files=all", "-z")
	if err != nil {
		return nil, nil, err
	}
	seen := map[string]struct{}{}
	untracked := map[string]bool{}
	for _, entry := range strings.Split(status, "\x00") {
		if entry == "" {
			continue
		}
		if len(entry) < 4 || entry[2] != ' ' {
			return nil, nil, fmt.Errorf("unexpected git status entry %q", entry)
		}
		changed := entry[3:]
		// Durable run authority is control data, not an upgrade candidate. It
		// must never force an unrelated user diff into a checkpoint nor be
		// staged by the publication adapter itself.
		if changed == ".drup" || strings.HasPrefix(changed, ".drup/") {
			continue
		}
		// Renames include a second NUL-delimited source path. They cannot be
		// represented as a single independently validated destination path.
		if entry[0] == 'R' || entry[1] == 'R' || entry[0] == 'C' || entry[1] == 'C' {
			return nil, nil, fmt.Errorf("rename/copy candidates are not supported")
		}
		seen[changed] = struct{}{}
		untracked[changed] = entry[:2] == "??"
	}
	paths := make([]string, 0, len(seen))
	for changed := range seen {
		paths = append(paths, changed)
	}
	sort.Strings(paths)
	return paths, untracked, nil
}

func gitOutput(path string, args ...string) (string, error) {
	args = append([]string{"-C", path}, args...)
	stdout, stderr, exitCode, err := runCommand("git", args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args[2:], " "), err)
	}
	if exitCode != 0 {
		return "", fmt.Errorf("git %s: exit %d: %s", strings.Join(args[2:], " "), exitCode, stderr)
	}
	return stdout, nil
}

func samePathSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

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
