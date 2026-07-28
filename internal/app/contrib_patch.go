package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nireneko/drup/internal/composerutil"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/patch"
	"github.com/nireneko/drup/internal/semver"
)

// ContribPatchResult is the JSON output of a contrib compatibility patch.
type ContribPatchResult struct {
	Module        string   `json:"module"`
	Package       string   `json:"composer_package"`
	TargetVersion string   `json:"target_version"`
	Before        string   `json:"core_version_requirement_before"`
	After         string   `json:"core_version_requirement_after,omitempty"`
	PatchPath     string   `json:"patch_path,omitempty"`
	ChangedFiles  []string `json:"changed_files"`
	Registered    bool     `json:"registered_in_composer"`
	LenientListed bool     `json:"listed_as_lenient"`
	DryRun        bool     `json:"dry_run"`
	Note          string   `json:"note,omitempty"`
}

// PatchContribForCore makes a contributed module installable on a newer Drupal
// major: it widens the module's core_version_requirement, records the change
// as a patch inside the project, registers that patch in composer.json, and
// adds the package to the lenient allow list.
//
// The last step is not optional. Composer resolves a package's core
// requirement from the metadata drupal.org publishes, never from files a patch
// has rewritten, so a patch alone leaves the module just as uninstallable as
// before.
func PatchContribForCore(projectPath, module, targetVersion string, dryRun bool) (*ContribPatchResult, error) {
	major := majorOf(targetVersion)
	if major == "" {
		return nil, fmt.Errorf("invalid target version %q: use a major like 11", targetVersion)
	}
	target, err := semver.Parse(major + ".0.0")
	if err != nil {
		return nil, fmt.Errorf("invalid target version %q: %w", targetVersion, err)
	}

	webRoot := composerutil.ReadWebRoot(projectPath)
	moduleDir := ""
	for _, kind := range []string{"modules", "themes", "profiles"} {
		candidate := filepath.Join(projectPath, webRoot, kind, "contrib", module)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			moduleDir = candidate
			break
		}
	}
	if moduleDir == "" {
		return nil, fmt.Errorf("contrib module %s not found under %s", module, filepath.Join(projectPath, webRoot))
	}

	infoPath := filepath.Join(moduleDir, module+".info.yml")
	infoData, err := os.ReadFile(infoPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", infoPath, err)
	}

	result := &ContribPatchResult{
		Module:        module,
		Package:       "drupal/" + module,
		TargetVersion: major,
		DryRun:        dryRun,
		ChangedFiles:  []string{},
	}

	constraint := parseCoreVersionRequirementFromInfo(string(infoData))
	result.Before = constraint
	if constraint == "" {
		return nil, fmt.Errorf("%s declares no core_version_requirement; it cannot be widened automatically", module)
	}
	if semver.Satisfies(target, constraint) {
		result.Note = fmt.Sprintf("%s already declares support for Drupal %s; no patch needed", module, major)
		return result, nil
	}
	result.After = widenConstraint(constraint, major)

	if dryRun {
		result.ChangedFiles = []string{module + ".info.yml"}
		return result, nil
	}

	// Diff against a pristine copy: contrib lives in a gitignored directory,
	// so the repository cannot produce the patch.
	pristine, err := os.MkdirTemp("", "drup-contrib-*")
	if err != nil {
		return nil, fmt.Errorf("create pristine copy: %w", err)
	}
	defer os.RemoveAll(pristine)
	pristineModule := filepath.Join(pristine, module)
	if err := copyTree(moduleDir, pristineModule); err != nil {
		return nil, fmt.Errorf("copy module for diff: %w", err)
	}

	updated, ok := rewriteCoreVersionRequirement(string(infoData), result.After)
	if !ok {
		return nil, fmt.Errorf("could not rewrite core_version_requirement in %s", infoPath)
	}
	if err := os.WriteFile(infoPath, []byte(updated), 0o644); err != nil {
		return nil, fmt.Errorf("write %s: %w", infoPath, err)
	}
	result.ChangedFiles = append(result.ChangedFiles, module+".info.yml")

	diff, _, exitCode, err := drupexec.Run("git", "diff", "--no-index", "--no-color", pristineModule, moduleDir)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	diff = normalizePatchPaths(diff, pristineModule, moduleDir)
	if exitCode > 1 || strings.TrimSpace(diff) == "" {
		return nil, fmt.Errorf("no differences were produced for %s", module)
	}

	patchDir := filepath.Join(projectPath, "patches", module)
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return nil, fmt.Errorf("create patch directory: %w", err)
	}
	patchPath := filepath.Join(patchDir, fmt.Sprintf("%s-drupal%s-compatibility.patch", module, major))
	if err := os.WriteFile(patchPath, []byte(diff), 0o644); err != nil {
		return nil, fmt.Errorf("write patch: %w", err)
	}
	result.PatchPath = patchPath

	description := fmt.Sprintf("Drupal %s compatibility for %s", major, module)
	relPatch, err := filepath.Rel(projectPath, patchPath)
	if err != nil {
		relPatch = patchPath
	}
	if err := patch.RegisterInComposer(projectPath, result.Package, relPatch, description); err != nil {
		return nil, fmt.Errorf("register patch: %w", err)
	}
	result.Registered = true

	// Without this composer still reads the published metadata and refuses.
	if _, err := AllowLenient(projectPath, []string{result.Package}, false); err != nil {
		return nil, fmt.Errorf("allow lenient for %s: %w", result.Package, err)
	}
	result.LenientListed = true

	return result, nil
}

// RunContribPatch exposes the contrib compatibility patch on the command line.
func RunContribPatch(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: drup contrib-patch <path> <module> [--target=11] [--dry-run]")
	}

	projectPath := args[0]
	module := args[1]
	target := "11"
	dryRun := false
	for _, arg := range args[2:] {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "--target="):
			target = strings.TrimPrefix(arg, "--target=")
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}

	result, err := PatchContribForCore(projectPath, module, target, dryRun)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
