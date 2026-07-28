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
	RectorRan     bool     `json:"rector_ran"`
	StandardsRan  bool     `json:"standards_ran"`
	Package       string   `json:"composer_package"`
	TargetVersion string   `json:"target_version"`
	Before        string   `json:"core_version_requirement_before"`
	After         string   `json:"core_version_requirement_after,omitempty"`
	PatchPath     string   `json:"patch_path,omitempty"`
	ChangedFiles  []string `json:"changed_files"`
	Registered    bool     `json:"registered_in_composer"`
	LenientListed bool     `json:"listed_as_lenient"`
	// Remaining is what upgrade_status still reports for the module after the
	// patch. A patch is not evidence of compatibility; this is.
	Remaining      int      `json:"remaining_findings"`
	RemainingItems []string `json:"remaining_items,omitempty"`
	Compatible     bool     `json:"compatible"`
	DryRun         bool     `json:"dry_run"`
	Note           string   `json:"note,omitempty"`
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
func PatchContribForCore(projectPath, module, targetVersion string, dryRun, declarationOnly bool) (*ContribPatchResult, error) {
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
	if semver.Satisfies(target, constraint) && declarationOnly {
		result.Note = fmt.Sprintf("%s already declares support for Drupal %s; no patch needed", module, major)
		return result, nil
	}
	if !semver.Satisfies(target, constraint) {
		result.After = widenConstraint(constraint, major)
	}

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

	if result.After != "" {
		updated, ok := rewriteCoreVersionRequirement(string(infoData), result.After)
		if !ok {
			return nil, fmt.Errorf("could not rewrite core_version_requirement in %s", infoPath)
		}
		if err := os.WriteFile(infoPath, []byte(updated), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", infoPath, err)
		}
		result.ChangedFiles = append(result.ChangedFiles, module+".info.yml")
	}

	// A declaration alone only claims compatibility. Put the module through
	// the same treatment the project's own code gets, so the patch carries the
	// fixes that make the claim true.
	if !declarationOnly {
		if err := rectorModule(projectPath, moduleDir); err != nil {
			return nil, err
		}
		result.RectorRan = true
		result.StandardsRan = formatToDrupalStandards(projectPath, moduleDir)
	}

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

	// Measure. Widening a declaration and running rector says nothing about
	// whether the module still calls removed APIs, and reporting a written
	// patch as a finished job is how a module gets declared compatible while
	// upgrade_status still has findings against it.
	remaining, items, err := moduleFindings(projectPath, module)
	if err != nil {
		result.Note = fmt.Sprintf("patch written, but the module could not be validated: %v", err)
		return result, nil
	}
	result.Remaining = remaining
	result.RemainingItems = items
	result.Compatible = remaining == 0
	if remaining > 0 {
		result.Note = fmt.Sprintf("patch written, but upgrade_status still reports %d finding(s) for %s — the module is not compatible yet", remaining, module)
	}

	return result, nil
}

// moduleFindings asks upgrade_status what it still reports for one module.
func moduleFindings(projectPath, module string) (int, []string, error) {
	_, filtered, err := DoValidate(projectPath, module)
	if err != nil {
		return 0, nil, err
	}

	items := make([]string, 0, len(filtered))
	for _, e := range filtered {
		items = append(items, fmt.Sprintf("%s:%d %s", filepath.Base(e.File), e.Line, e.Message))
	}
	return len(filtered), items, nil
}

// rectorModule runs drupal-rector over one module, through the environment
// prefix so it uses the site's PHP.
func rectorModule(projectPath, moduleDir string) error {
	configPath, err := ensureRectorConfig(projectPath)
	if err != nil {
		return err
	}

	target := moduleDir
	if rel, relErr := filepath.Rel(projectPath, moduleDir); relErr == nil && isContainerized(projectPath) {
		target = rel
	}
	_, stderr, exitCode, err := cliRun(projectPath, projectRelPath(projectPath, "vendor", "bin", "rector"),
		"process", target, "--config="+configPath)
	if err != nil {
		return fmt.Errorf("exec rector: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("rector exited %d: %s", exitCode, stderr)
	}
	return nil
}

// formatToDrupalStandards applies the Drupal coding standards with phpcbf when
// the project has it. Reports whether it ran: a project without drupal/coder
// still gets its compatibility patch, just without the formatting pass.
func formatToDrupalStandards(projectPath, moduleDir string) bool {
	if _, err := os.Stat(filepath.Join(projectPath, "vendor", "bin", "phpcbf")); err != nil {
		return false
	}

	target := moduleDir
	if rel, relErr := filepath.Rel(projectPath, moduleDir); relErr == nil && isContainerized(projectPath) {
		target = rel
	}
	// phpcbf exits 1 when it fixed something and 2 when some issues remain,
	// so the exit code is not a failure signal here.
	_, _, _, err := cliRun(projectPath, projectRelPath(projectPath, "vendor", "bin", "phpcbf"),
		"--standard=Drupal,DrupalPractice",
		"--extensions=php,module,inc,install,test,profile,theme,yml",
		target)
	return err == nil
}

// RunContribPatch exposes the contrib compatibility patch on the command line.
func RunContribPatch(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: drup contrib-patch <path> <module> [--target=11] [--declaration-only] [--dry-run]")
	}

	projectPath := args[0]
	module := args[1]
	target := "11"
	dryRun := false
	declarationOnly := false
	for _, arg := range args[2:] {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case arg == "--declaration-only":
			declarationOnly = true
		case strings.HasPrefix(arg, "--target="):
			target = strings.TrimPrefix(arg, "--target=")
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}

	result, err := PatchContribForCore(projectPath, module, target, dryRun, declarationOnly)
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
