package patch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	drupexec "github.com/nireneko/drup/internal/exec"
)

// httpClient is the HTTP client for patch downloads.
// Package-level var for testability.
var httpClient = &http.Client{Timeout: 60 * time.Second}

// runCommand executes subprocess commands. Package-level var for testability.
var runCommand = drupexec.Run

// allowedDomains is the allowlist for patch download URLs.
var allowedDomains = []string{
	"www.drupal.org",
	"drupal.org",
	"git.drupal.org",
	"updates.drupal.org",
}

// checkAllowedURL validates a URL against the allowlist. Package-level var for testability.
var checkAllowedURL = defaultCheckAllowedURL

func defaultCheckAllowedURL(url string) bool {
	for _, domain := range allowedDomains {
		if strings.Contains(url, domain) {
			return true
		}
	}
	return false
}

// ApplyResult contains the result of a patch apply operation.
type ApplyResult struct {
	Applied    bool   `json:"applied"`
	CommitHash string `json:"commit_hash,omitempty"`
	Error      string `json:"error,omitempty"`
}

// Apply downloads a patch from patchURL, applies it via git apply, and
// registers it in composer.json under extra.patches.
// The operation is atomic: if any step fails, changes are reverted.
func Apply(patchURL, projectPath, composerPackage, description string) (*ApplyResult, error) {
	// Validate URL against allowlist.
	if !checkAllowedURL(patchURL) {
		return nil, fmt.Errorf("patch URL not in allowlist: %s", patchURL)
	}

	// Download patch to temp file.
	tmpFile, err := downloadPatch(patchURL)
	if err != nil {
		return nil, fmt.Errorf("download patch: %w", err)
	}
	defer os.Remove(tmpFile)

	// Try git apply.
	_, stderr, exitCode, err := runCommand("git", "-C", projectPath, "apply", tmpFile)
	if err != nil {
		return &ApplyResult{Applied: false, Error: err.Error()}, nil
	}
	if exitCode != 0 {
		// Try with --whitespace=nowarn fallback.
		_, stderr2, exitCode2, err2 := runCommand("git", "-C", projectPath, "apply", "--whitespace=nowarn", tmpFile)
		if err2 != nil {
			return &ApplyResult{Applied: false, Error: err2.Error()}, nil
		}
		if exitCode2 != 0 {
			return &ApplyResult{Applied: false, Error: stderr + "; " + stderr2}, nil
		}
	}

	// Stage and commit.
	_, stderr, exitCode, err = runCommand("git", "-C", projectPath, "add", "-A")
	if err != nil || exitCode != 0 {
		// Revert the apply.
		runCommand("git", "-C", projectPath, "apply", "-R", tmpFile)
		return &ApplyResult{Applied: false, Error: "git add failed: " + stderr}, nil
	}

	commitMsg := fmt.Sprintf("fix(contrib): apply D11 patch to %s", composerPackage)
	_, stderr, exitCode, err = runCommand("git", "-C", projectPath, "commit", "-m", commitMsg)
	if err != nil || exitCode != 0 {
		runCommand("git", "-C", projectPath, "reset", "HEAD~1")
		runCommand("git", "-C", projectPath, "apply", "-R", tmpFile)
		return &ApplyResult{Applied: false, Error: "git commit failed: " + stderr}, nil
	}

	// Get commit hash.
	stdout, _, _, _ := runCommand("git", "-C", projectPath, "rev-parse", "HEAD")
	commitHash := strings.TrimSpace(stdout)

	// Register in composer.json.
	if err := registerPatch(projectPath, composerPackage, patchURL, description); err != nil {
		// Revert commit.
		runCommand("git", "-C", projectPath, "revert", "--no-edit", "HEAD")
		return &ApplyResult{Applied: false, Error: "composer.json update failed: " + err.Error()}, nil
	}

	return &ApplyResult{Applied: true, CommitHash: commitHash}, nil
}

func downloadPatch(url string) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "drup-patch-*.patch")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

func registerPatch(projectPath, composerPackage, patchURL, description string) error {
	composerFile := filepath.Join(projectPath, "composer.json")
	data, err := os.ReadFile(composerFile)
	if err != nil {
		return fmt.Errorf("read composer.json: %w", err)
	}

	var composer map[string]interface{}
	if err := json.Unmarshal(data, &composer); err != nil {
		return fmt.Errorf("parse composer.json: %w", err)
	}

	// Ensure extra.patches exists.
	extra, ok := composer["extra"].(map[string]interface{})
	if !ok {
		extra = make(map[string]interface{})
		composer["extra"] = extra
	}
	patches, ok := extra["patches"].(map[string]interface{})
	if !ok {
		patches = make(map[string]interface{})
		extra["patches"] = patches
	}

	// Add patch entry.
	modulePatches, ok := patches[composerPackage].([]interface{})
	if !ok {
		modulePatches = []interface{}{}
	}
	modulePatches = append(modulePatches, map[string]interface{}{
		"description": description,
		"url":         patchURL,
	})
	patches[composerPackage] = modulePatches

	// Write back.
	data, err = json.MarshalIndent(composer, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal composer.json: %w", err)
	}
	if err := os.WriteFile(composerFile, data, 0o644); err != nil {
		return fmt.Errorf("write composer.json: %w", err)
	}

	return nil
}
