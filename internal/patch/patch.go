package patch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/composerutil"
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

func defaultCheckAllowedURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if parsed.Scheme != "https" {
		return false
	}

	host := parsed.Hostname()
	for _, domain := range allowedDomains {
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

// localPatchPath resolves a patch reference that points at a file inside the
// project. Paths outside the project are rejected so a tool call cannot read
// arbitrary files from disk.
func localPatchPath(patchRef, projectPath string) (string, bool, error) {
	if strings.HasPrefix(patchRef, "http://") || strings.HasPrefix(patchRef, "https://") {
		return "", false, nil
	}

	candidate := patchRef
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(projectPath, candidate)
	}
	candidate = filepath.Clean(candidate)

	root := filepath.Clean(projectPath)
	if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
		return "", false, fmt.Errorf("patch path outside the project: %s", patchRef)
	}
	if _, err := os.Stat(candidate); err != nil {
		return "", false, fmt.Errorf("patch file not found: %s", patchRef)
	}
	return candidate, true, nil
}

// ApplyResult contains the result of a patch apply operation.
type ApplyResult struct {
	Applied      bool     `json:"applied"`
	ChangedFiles []string `json:"changed_files,omitempty"`
	Error        string   `json:"error,omitempty"`
}

// Apply downloads a patch from patchURL, applies it via git apply, and
// registers it in composer.json under extra.patches.
// The operation is atomic: if any step fails, changes are reverted.
func Apply(patchURL, projectPath, composerPackage, description string) (*ApplyResult, error) {
	// A patch already inside the project needs no download and no allowlist:
	// create_patch writes exactly such files, and refusing them made the
	// create-then-apply flow impossible.
	local, isLocal, err := localPatchPath(patchURL, projectPath)
	if err != nil {
		return nil, err
	}

	tmpFile := local
	if !isLocal {
		if !checkAllowedURL(patchURL) {
			return nil, fmt.Errorf("patch URL not in allowlist: %s", patchURL)
		}

		// Download patch to temp file.
		tmpFile, err = downloadPatch(patchURL)
		if err != nil {
			return nil, fmt.Errorf("download patch: %w", err)
		}
		defer os.Remove(tmpFile)
	}

	// Module patches are written relative to the package root, which is where
	// composer-patches applies them from. git apply resolves paths against the
	// repository root, so the package directory goes through --directory.
	applyArgs := []string{"-C", projectPath, "apply"}
	if rel := packageRelDir(projectPath, composerPackage); rel != "" {
		applyArgs = append(applyArgs, "--directory="+rel)
	}
	revertArgs := append(append([]string{}, applyArgs...), "-R", tmpFile)

	// Try git apply.
	_, stderr, exitCode, err := runCommand("git", append(append([]string{}, applyArgs...), tmpFile)...)
	if err != nil {
		return &ApplyResult{Applied: false, Error: err.Error()}, nil
	}
	if exitCode != 0 {
		// Try with --whitespace=nowarn fallback.
		fallback := append(append([]string{}, applyArgs...), "--whitespace=nowarn", tmpFile)
		_, stderr2, exitCode2, err2 := runCommand("git", fallback...)
		if err2 != nil {
			return &ApplyResult{Applied: false, Error: err2.Error()}, nil
		}
		if exitCode2 != 0 {
			return &ApplyResult{Applied: false, Error: stderr + "; " + stderr2}, nil
		}
	}

	// Register in composer.json as part of the mutation. Publication is a
	// separate checkpoint_commit operation: applying a patch must never create
	// history before independent validation binds this exact candidate.
	reference := patchURL
	if isLocal {
		if rel, relErr := filepath.Rel(projectPath, tmpFile); relErr == nil {
			reference = rel
		}
	}
	if err := registerPatch(projectPath, composerPackage, reference, description); err != nil {
		runCommand("git", revertArgs...)
		return &ApplyResult{Applied: false, Error: "composer.json update failed: " + err.Error()}, nil
	}

	changed := []string{"composer.json"}
	if rel := packageRelDir(projectPath, composerPackage); rel != "" {
		changed = append(changed, rel)
	}
	return &ApplyResult{Applied: true, ChangedFiles: changed}, nil
}

// LocalPatchPath exposes local patch resolution to callers that need to read
// a registered patch file.
func LocalPatchPath(patchRef, projectPath string) (string, bool, error) {
	return localPatchPath(patchRef, projectPath)
}

// ReverseInPackage undoes a patch inside the package directory. Contrib code
// is gitignored in composer-based projects, so reverting the commit leaves the
// patched files in place. Failures are not fatal: the patch may already be
// gone if the project tracks its contrib code.
func ReverseInPackage(projectPath, composerPackage, patchFile string) {
	args := []string{"-C", projectPath, "apply", "-R"}
	if rel := packageRelDir(projectPath, composerPackage); rel != "" {
		args = append(args, "--directory="+rel)
	}
	runCommand("git", append(args, patchFile)...)
}

// RegisterInComposer records a patch under extra.patches for a package.
func RegisterInComposer(projectPath, composerPackage, patchRef, description string) error {
	return registerPatch(projectPath, composerPackage, patchRef, description)
}

// CommitSubject is the commit message used when a patch is applied. Rollback
// finds the commit by this exact text, so both sides share one definition.
func CommitSubject(composerPackage string) string {
	return fmt.Sprintf("fix(contrib): apply D11 patch to %s", composerPackage)
}

// packageRelDir returns the install directory of a composer package relative
// to the project root, or "" when the package is not a Drupal extension.
func packageRelDir(projectPath, composerPackage string) string {
	dir := packageDir(projectPath, composerPackage)
	if dir == projectPath {
		return ""
	}
	rel, err := filepath.Rel(projectPath, dir)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// packageDir returns the directory a composer package is installed into, so a
// patch can be applied from the same root composer-patches uses. Falls back to
// the project root for packages that are not Drupal extensions.
func packageDir(projectPath, composerPackage string) string {
	name := composerPackage
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if name == "" {
		return projectPath
	}

	webRoot := composerutil.ReadWebRoot(projectPath)
	for _, kind := range []string{"modules", "themes", "profiles"} {
		for _, group := range []string{"contrib", "custom"} {
			candidate := filepath.Join(projectPath, webRoot, kind, group, name)
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
		}
	}
	return projectPath
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

	// Add patch entry. composer-patches keys each patch by its description:
	// {"drupal/foo": {"Fix bar": "https://…patch"}}. Writing a list here would
	// both be ignored by composer and discard the project's existing patches
	// for this package.
	modulePatches, ok := patches[composerPackage].(map[string]interface{})
	if !ok {
		modulePatches = make(map[string]interface{})
		// Convert entries written by older drup versions.
		if legacy, isList := patches[composerPackage].([]interface{}); isList {
			for _, item := range legacy {
				entry, isMap := item.(map[string]interface{})
				if !isMap {
					continue
				}
				url, _ := entry["url"].(string)
				if url == "" {
					continue
				}
				desc, _ := entry["description"].(string)
				if desc == "" {
					desc = url
				}
				modulePatches[desc] = url
			}
		}
	}
	key := description
	if key == "" {
		key = patchURL
	}
	modulePatches[key] = patchURL
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
