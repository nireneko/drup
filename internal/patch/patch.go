package patch

import (
	"crypto/sha256"
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

// maxPatchBytes bounds downloaded patch bodies before they reach disk.
const maxPatchBytes int64 = 10 << 20

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
	Applied       bool          `json:"applied"`
	ChangedFiles  []string      `json:"changed_files,omitempty"`
	PatchEvidence PatchEvidence `json:"patch_evidence,omitempty"`
	Error         string        `json:"error,omitempty"`
}

// PatchEvidence is the immutable provenance retained alongside a Composer
// patch registration. It deliberately records hashes rather than patch bytes.
type PatchEvidence struct {
	SHA256     string    `json:"sha256"`
	Size       int64     `json:"size"`
	InitialURL string    `json:"initial_url"`
	FinalURL   string    `json:"final_url"`
	RecordedAt time.Time `json:"recorded_at"`
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
	var evidence PatchEvidence
	if !isLocal {
		if !checkAllowedURL(patchURL) {
			return nil, fmt.Errorf("patch URL not in allowlist: %s", patchURL)
		}

		// Download patch to temp file.
		tmpFile, evidence, err = downloadPatch(patchURL)
		if err != nil {
			return nil, fmt.Errorf("download patch: %w", err)
		}
		defer os.Remove(tmpFile)
	} else if evidence, err = evidenceForLocalPatch(tmpFile, patchURL); err != nil {
		return nil, err
	}

	// Module patches are written relative to the package root, which is where
	// composer-patches applies them from. git apply resolves paths against the
	// repository root, so the package directory goes through --directory.
	applyArgs := []string{"-C", projectPath, "apply"}
	if rel := packageRelDir(projectPath, composerPackage); rel != "" {
		applyArgs = append(applyArgs, "--directory="+rel)
	}
	if err := validatePatchPaths(tmpFile); err != nil {
		return nil, err
	}
	if _, stderr, exitCode, err := runCommand("git", append(append([]string{}, applyArgs...), "--check", tmpFile)...); err != nil || exitCode != 0 {
		if err != nil {
			return &ApplyResult{Applied: false, Error: err.Error()}, nil
		}
		return &ApplyResult{Applied: false, Error: "git apply --check: " + stderr}, nil
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
	if err := registerPatchWithEvidence(projectPath, composerPackage, reference, description, evidence); err != nil {
		runCommand("git", revertArgs...)
		return &ApplyResult{Applied: false, Error: "composer.json update failed: " + err.Error()}, nil
	}

	changed := []string{"composer.json"}
	if rel := packageRelDir(projectPath, composerPackage); rel != "" {
		changed = append(changed, rel)
	}
	return &ApplyResult{Applied: true, ChangedFiles: changed, PatchEvidence: evidence}, nil
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

func downloadPatch(rawURL string) (string, PatchEvidence, error) {
	client := *httpClient
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("too many redirects")
		}
		if !checkAllowedURL(req.URL.String()) {
			return fmt.Errorf("redirect URL not in allowlist: %s", req.URL)
		}
		return nil
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", PatchEvidence{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", PatchEvidence{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > maxPatchBytes {
		return "", PatchEvidence{}, fmt.Errorf("patch body exceeds %d bytes", maxPatchBytes)
	}

	tmpFile, err := os.CreateTemp("", "drup-patch-*.patch")
	if err != nil {
		return "", PatchEvidence{}, err
	}
	name := tmpFile.Name()
	defer func() { _ = tmpFile.Close() }()

	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxPatchBytes+1))
	if err != nil || written > maxPatchBytes {
		_ = os.Remove(name)
		if err != nil {
			return "", PatchEvidence{}, err
		}
		return "", PatchEvidence{}, fmt.Errorf("patch body exceeds %d bytes", maxPatchBytes)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(name)
		return "", PatchEvidence{}, err
	}
	evidence, err := evidenceForLocalPatch(name, rawURL)
	if err != nil {
		_ = os.Remove(name)
		return "", PatchEvidence{}, err
	}
	evidence.FinalURL = resp.Request.URL.String()
	return name, evidence, nil
}

func evidenceForLocalPatch(path, source string) (PatchEvidence, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return PatchEvidence{}, err
	}
	digest := sha256.Sum256(data)
	return PatchEvidence{SHA256: fmt.Sprintf("%x", digest), Size: int64(len(data)), InitialURL: source, FinalURL: source, RecordedAt: time.Now().UTC()}, nil
}

func validatePatchPaths(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "new file mode 120000") || strings.HasPrefix(line, "new mode 120000") {
			return fmt.Errorf("patch creates or changes a symbolic link")
		}
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			name := strings.Fields(strings.TrimSpace(line[4:]))
			if len(name) == 0 || name[0] == "/dev/null" {
				continue
			}
			candidate := strings.TrimPrefix(strings.TrimPrefix(name[0], "a/"), "b/")
			if filepath.IsAbs(candidate) || candidate == ".." || strings.HasPrefix(filepath.Clean(candidate), ".."+string(filepath.Separator)) {
				return fmt.Errorf("patch path outside declared package: %s", name[0])
			}
		}
	}
	return nil
}

func registerPatch(projectPath, composerPackage, patchURL, description string) error {
	return registerPatchWithEvidence(projectPath, composerPackage, patchURL, description, PatchEvidence{})
}

func registerPatchWithEvidence(projectPath, composerPackage, patchURL, description string, evidence PatchEvidence) error {
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
	if evidence.SHA256 != "" {
		drup, ok := extra["drup"].(map[string]interface{})
		if !ok {
			drup = make(map[string]interface{})
			extra["drup"] = drup
		}
		allEvidence, ok := drup["patch_evidence"].(map[string]interface{})
		if !ok {
			allEvidence = make(map[string]interface{})
			drup["patch_evidence"] = allEvidence
		}
		packageEvidence, ok := allEvidence[composerPackage].(map[string]interface{})
		if !ok {
			packageEvidence = make(map[string]interface{})
			allEvidence[composerPackage] = packageEvidence
		}
		packageEvidence[key] = evidence
	}

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
