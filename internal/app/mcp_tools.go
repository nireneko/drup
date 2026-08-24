package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/audit"
	"github.com/nireneko/drup/internal/backup"
	"github.com/nireneko/drup/internal/composerutil"
	"github.com/nireneko/drup/internal/coreupgrade"
	"github.com/nireneko/drup/internal/drupalorg"
	"github.com/nireneko/drup/internal/envdetect"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/patch"
	"github.com/nireneko/drup/internal/patchreconcile"
	"github.com/nireneko/drup/internal/report"
	"github.com/nireneko/drup/internal/scan"
	"github.com/nireneko/drup/internal/semver"
	"github.com/nireneko/drup/internal/session"
)

// defaultEnvDetector is the shared environment detector.
var defaultEnvDetector envdetect.Detector = envdetect.NewDetector()

// defaultExecTimeout bounds every subprocess invocation that has no more
// specific entry in execTimeoutOverride. Without a deadline a hanging
// drush/composer call blocks its MCP request forever — the stdio server
// handles requests synchronously, so one stuck call stalls every
// subsequent tool call too.
var defaultExecTimeout = 5 * time.Minute

// execTimeoutOverride raises the deadline for tools whose underlying
// command legitimately runs longer than the 5-minute default: a full
// composer install, a core upgrade apply, or an upgrade_status re-scan.
var execTimeoutOverride = map[string]time.Duration{
	"composer_require":   10 * time.Minute,
	"core_upgrade_apply": 15 * time.Minute,
	"upgrade_scan":       10 * time.Minute,
}

// resolveExecTimeout returns the configured deadline for toolName, falling
// back to defaultExecTimeout when no override is set.
func resolveExecTimeout(toolName string) time.Duration {
	if d, ok := execTimeoutOverride[toolName]; ok {
		return d
	}
	return defaultExecTimeout
}

// drushBlocklist contains the canonical drush command names that must not be
// executed via drush_exec. Evaluated after normalizeDrushCommand resolves
// aliases, so an alias of a blocked command is blocked identically to its
// canonical form.
var drushBlocklist = map[string]bool{
	"sql-drop":         true,
	"site-install":     true,
	"site:install":     true,
	"sql-sanitize":     true,
	"php-eval":         true,
	"php:script":       true,
	"core:execute-cli": true,
	"sql:query":        true,
}

// drushAliasMap resolves known drush command aliases to their canonical
// form, evaluated before drushBlocklist so an alias cannot bypass it.
var drushAliasMap = map[string]string{
	"sqlq":         "sql:query",
	"sql-cli":      "sql:query",
	"sqlc":         "sql:query",
	"scr":          "php:script",
	"ev":           "php-eval",
	"exec":         "core:execute-cli",
	"core:execute": "core:execute-cli",
}

// normalizeDrushCommand trims whitespace, lowercases, and resolves known
// aliases to their canonical drush command name so blocklist evaluation
// cannot be bypassed via an alias, case, or surrounding whitespace.
func normalizeDrushCommand(cmd string) string {
	normalized := strings.ToLower(strings.TrimSpace(cmd))
	if canonical, ok := drushAliasMap[normalized]; ok {
		return canonical
	}
	return normalized
}

// shellMetacharPattern matches shell injection characters, including
// newlines, so a command or argument cannot smuggle a second statement.
var shellMetacharPattern = regexp.MustCompile("[;|&$`\n]")

// composerPackagePattern validates composer package names.
var composerPackagePattern = regexp.MustCompile(`^[a-z0-9]([_.\-]?[a-z0-9]+)*/[a-z0-9]([_.\-]?[a-z0-9]+)*(:[a-zA-Z0-9^~<>=*. -]+)?$`)

// moduleNamePattern validates Drupal module machine names.
var moduleNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// WireMCPTools registers real tool handlers on the MCP server, replacing
// placeholders. Every tool in session.GuardedTools() (the force-dry-run and
// refuse-only partitions from specs/agent-session's Guard Middleware
// Enforcement requirement) is wrapped with guardHandler at this single
// registration point. upgrade_scan is deliberately NOT wrapped here — its
// only mutation (the nested drupal/upgrade_status composer install) bypasses
// s.tools entirely, so it is guarded inline inside realHandleUpgradeScan via
// the same guardedCall helper the middleware uses, applied directly to that
// nested composer_require call.
func WireMCPTools(s *mcp.Server) {
	s.RegisterTool("scan", realHandleScan)
	s.RegisterTool("autofix", realHandleAutofix)
	s.RegisterTool("contrib_check", realHandleContribCheck)
	s.RegisterTool("issue_patches", realHandleIssuePatches)
	s.RegisterTool("apply_patch", guardHandler("apply_patch", realHandleApplyPatch))
	s.RegisterTool("validate", realHandleValidate)
	s.RegisterTool("create_patch", guardHandler("create_patch", realHandleCreatePatch))
	// New tools.
	s.RegisterTool("detect_env", realHandleDetectEnv)
	s.RegisterTool("composer_require", guardHandler("composer_require", realHandleComposerRequire))
	s.RegisterTool("drush_exec", realHandleDrushExec)
	s.RegisterTool("contrib_upgrade_path", realHandleContribUpgradePath)
	s.RegisterTool("upgrade_scan", realHandleUpgradeScan)
	s.RegisterTool("patch_status", realHandlePatchStatus)
	s.RegisterTool("patch_rollback", guardHandler("patch_rollback", realHandlePatchRollback))
	s.RegisterTool("generate_report", realHandleGenerateReport)
	s.RegisterTool("module_info", realHandleModuleInfo)
	s.RegisterTool("drupal_version_matrix", realHandleDrupalVersionMatrix)
	s.RegisterTool("core_upgrade_check", realHandleCoreUpgradeCheck)
	s.RegisterTool("core_upgrade_apply", guardHandler("core_upgrade_apply", realHandleCoreUpgradeApply))
	s.RegisterTool("patch_reconcile", realHandlePatchReconcile)
	s.RegisterTool("cleanup", guardHandler("cleanup", realHandleCleanup))
	s.RegisterTool("custom_compat_fix", guardHandler("custom_compat_fix", realHandleCustomCompatFix))
	s.RegisterTool("contrib_allow_lenient", guardHandler("contrib_allow_lenient", realHandleContribAllowLenient))
	s.RegisterTool("contrib_compat_patch", guardHandler("contrib_compat_patch", realHandleContribCompatPatch))
	s.RegisterTool("test_backup_create", realHandleTestBackupCreate)
	s.RegisterTool("test_backup_list", realHandleTestBackupList)
	s.RegisterTool("test_backup_restore", guardHandler("test_backup_restore", realHandleTestBackupRestore))
	s.RegisterTool("test_backup_delete", guardHandler("test_backup_delete", realHandleTestBackupDelete))
	s.RegisterTool("module_release_info", realHandleModuleReleaseInfo)
	s.RegisterTool("session_open", realHandleSessionOpen)
	s.RegisterTool("pipeline_status", realHandlePipelineStatus)
}

// realHandlePipelineStatus summarizes the project's mutation ledger
// (specs/mutation-audit's pipeline_status Tool requirement): per-tool
// counts, total mutations, and the remaining mutation-cap headroom for
// whichever window currently applies (per-session if a matching session is
// bound, otherwise per-day). A project with no ledger yet returns zero
// counts and the full cap, never an error.
func realHandlePipelineStatus(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	records, err := audit.ReadAll(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	perTool := map[string]int{}
	for _, r := range records {
		perTool[r.Tool]++
	}

	hasSession := false
	var openedAt time.Time
	if sess, ok := session.Current(); ok {
		if root, rerr := session.ResolveSymlinks(params.ProjectPath); rerr == nil && root == sess.Root {
			hasSession = true
			openedAt = sess.OpenedAt
		}
	}
	_, count, capN, capErr := audit.CheckCap(params.ProjectPath, hasSession, openedAt)
	remaining := capN - count
	if capErr != nil || remaining < 0 {
		remaining = 0
	}

	result := map[string]interface{}{
		"per_tool_counts": perTool,
		"total_mutations": len(records),
		"remaining_cap":   remaining,
	}
	return json.Marshal(result)
}

func backupParams(args json.RawMessage) (string, error) {
	var p struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", err
	}
	if p.ProjectPath == "" {
		return "", fmt.Errorf("project_path is required")
	}
	return p.ProjectPath, nil
}
func realHandleTestBackupCreate(args json.RawMessage) (json.RawMessage, error) {
	p, err := backupParams(args)
	if err != nil {
		return nil, err
	}
	m, err := backup.NewManager(p).Create(p)
	if err != nil {
		return nil, err
	}
	return json.Marshal(m)
}
func realHandleTestBackupList(args json.RawMessage) (json.RawMessage, error) {
	p, err := backupParams(args)
	if err != nil {
		return nil, err
	}
	v, err := backup.NewManager(p).List(p)
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
func realHandleTestBackupRestore(args json.RawMessage) (json.RawMessage, error) {
	var p struct {
		ProjectPath string `json:"project_path"`
		BackupID    string `json:"backup_id"`
		Confirm     bool   `json:"confirm"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.ProjectPath == "" || p.BackupID == "" {
		return nil, fmt.Errorf("project_path and backup_id are required")
	}
	if err := backup.NewManager(p.ProjectPath).Restore(p.ProjectPath, p.BackupID, p.Confirm); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"backup_id": p.BackupID, "restored": true})
}
func realHandleTestBackupDelete(args json.RawMessage) (json.RawMessage, error) {
	var p struct {
		ProjectPath string `json:"project_path"`
		BackupID    string `json:"backup_id"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return nil, err
	}
	if p.ProjectPath == "" || p.BackupID == "" {
		return nil, fmt.Errorf("project_path and backup_id are required")
	}
	if err := backup.NewManager(p.ProjectPath).Delete(p.ProjectPath, p.BackupID); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]interface{}{"backup_id": p.BackupID, "deleted": true})
}

func realHandleScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Ensure upgrade_status is enabled before running the scan.
	if err := ensureUpgradeStatusEnabled(params.ProjectPath, resolveExecTimeout("scan")); err != nil {
		return nil, err
	}

	stdout, stderr, exitCode, err := cliRun(params.ProjectPath, "drush", "upgrade_status:analyze", "--all", "--format=checkstyle", "--root="+params.ProjectPath)
	if err != nil {
		return nil, drushExecError("drush", []string{"upgrade_status:analyze", "--all", "--format=checkstyle", "--root=" + params.ProjectPath}, -1, err.Error(), "")
	}
	if !isScanExitOK(exitCode) {
		return nil, drushExecError("drush", []string{"upgrade_status:analyze", "--all", "--format=checkstyle", "--root=" + params.ProjectPath}, exitCode, stderr, stdout)
	}

	// Exit code 3 with empty stdout means drush crashed, not findings.
	if exitCode == 3 && strings.TrimSpace(stdout) == "" {
		return nil, fmt.Errorf("drush exited with code 3 but produced no output (command: drush upgrade_status:checkstyle --all --root=%s)\nstderr: %s", params.ProjectPath, stderr)
	}

	result, err := scan.ParseCheckstyle(strings.NewReader(stdout))
	if err != nil {
		return nil, fmt.Errorf("parse scan (command: drush upgrade_status:checkstyle --all --root=%s): %w\nstdout (truncated): %.500s", params.ProjectPath, err, stdout)
	}
	result.ProjectPath = params.ProjectPath
	return json.Marshal(result)
}

func realHandleAutofix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	root := resolveDrupalRoot(params.ProjectPath)
	customModules := filepath.Join(root, "modules", "custom")
	themes := filepath.Join(root, "themes")

	targets := []string{}
	for _, dir := range []string{customModules, themes} {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			targets = append(targets, dir)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no custom modules or themes found in %s", params.ProjectPath)
	}

	configPath, err := ensureRectorConfig(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	args2 := append([]string{"process"}, targets...)
	args2 = append(args2, "--config="+configPath)
	// Rector must run against the site's PHP, so it goes through the same
	// environment prefix as drush and composer.
	stdout, rectorStderr, rectorExit, err := cliRun(params.ProjectPath, projectRelPath(params.ProjectPath, "vendor", "bin", "rector"), args2...)
	if err != nil {
		return nil, fmt.Errorf("exec rector: %w", err)
	}
	// Rector's exit code used to be discarded, so a failed run reported an
	// empty summary and looked like success.
	if rectorExit != 0 {
		return nil, fmt.Errorf("rector exited %d: %s%.500s", rectorExit, rectorStderr, stdout)
	}

	// Re-scan to get remaining errors. A rescan that cannot be parsed must not
	// be reported as zero remaining errors.
	scanStdout, scanStderr, scanExit, _ := cliRun(params.ProjectPath, "drush", "upgrade_status:analyze", "--all", "--format=checkstyle", "--root="+params.ProjectPath)
	if !isScanExitOK(scanExit) {
		return nil, drushExecError("drush", []string{"upgrade_status:analyze", "--all", "--format=checkstyle"}, scanExit, scanStderr, scanStdout)
	}
	rescan, err := scan.ParseCheckstyle(strings.NewReader(scanStdout))
	if err != nil {
		return nil, fmt.Errorf("parse rescan after rector: %w", err)
	}
	remaining := rescan.TotalErrs

	response := map[string]interface{}{
		"rector_summary":   stdout,
		"remaining_errors": remaining,
	}
	return json.Marshal(response)
}

func realHandleContribCheck(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module string `json:"module_machine_name"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	info, err := drupalorg.CheckRelease(params.Module)
	if err != nil {
		return nil, err
	}
	return json.Marshal(info)
}

func realHandleIssuePatches(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		IssueNID string `json:"issue_nid,omitempty"`
		Module   string `json:"module_name,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	query := params.Module
	if params.IssueNID != "" {
		query = params.IssueNID
	}
	if query == "" {
		return nil, fmt.Errorf("module_name or issue_nid required")
	}

	result, err := drupalorg.SearchPatches(query)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleApplyPatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		PatchURL        string `json:"patch_url"`
		ProjectPath     string `json:"project_path"`
		ComposerPackage string `json:"composer_package,omitempty"`
		Description     string `json:"description,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	result, err := DoApplyPatch(params.PatchURL, params.ProjectPath, params.ComposerPackage, params.Description)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// normalizeScope validates a scope filter. "all" and "" mean no filtering;
// an unknown value is an error rather than a silent empty result.
func normalizeScope(scope string) (string, error) {
	switch scope {
	case "", "all":
		return "", nil
	case string(scan.ClassCustom), string(scan.ClassContrib), string(scan.ClassTheme), string(scan.ClassCore):
		return scope, nil
	default:
		return "", fmt.Errorf("invalid scope %q: use custom, contrib, theme, core or all", scope)
	}
}

func realHandleValidate(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath  string `json:"project_path"`
		Scope        string `json:"scope,omitempty"`
		Module       string `json:"module,omitempty"`
		File         string `json:"file,omitempty"`
		ExpectedHash string `json:"expected_hash,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	result, filtered, err := DoValidate(params.ProjectPath, params.Module)
	if err != nil {
		return nil, err
	}

	var currentHash string
	if result != nil {
		currentHash = result.EvidenceHash()
	}

	// Fail closed on a stale scan regardless of total_errors: a caller that
	// pins expected_hash to a prior scan is asserting "nothing has changed
	// since I last looked" — if the current evidence_hash disagrees, the
	// prior total_errors (even zero) can no longer be trusted.
	if params.ExpectedHash != "" && params.ExpectedHash != currentHash {
		return nil, fmt.Errorf("validate: evidence_hash mismatch — expected %s, got %s; the scan is stale, re-run upgrade_scan/validate before trusting total_errors", params.ExpectedHash, currentHash)
	}

	// Filter by scope. Without this the tool answered a request for custom
	// code with contrib findings.
	if scope, scopeErr := normalizeScope(params.Scope); scopeErr != nil {
		return nil, scopeErr
	} else if scope != "" {
		var byScope []scan.DepError
		for _, e := range filtered {
			if string(scan.Classify(e.File)) == scope {
				byScope = append(byScope, e)
			}
		}
		filtered = byScope
	}

	// Further filter by file if specified.
	if params.File != "" {
		var byFile []scan.DepError
		for _, e := range filtered {
			if strings.Contains(e.File, params.File) {
				byFile = append(byFile, e)
			}
		}
		filtered = byFile
	}

	response := map[string]interface{}{
		"total_errors":  len(filtered),
		"errors":        filtered,
		"evidence_hash": currentHash,
	}
	return json.Marshal(response)
}

// normalizePatchPaths rewrites the absolute paths git --no-index emits into
// module-relative ones, so the patch applies from the package root with -p1
// like every other composer patch.
func normalizePatchPaths(diff, pristineDir, moduleDir string) string {
	sep := string(os.PathSeparator)
	for _, dir := range []string{pristineDir, moduleDir} {
		// git writes "a/" + path-without-leading-slash, so collapsing the
		// directory to a bare separator keeps the a/ and b/ prefixes intact.
		diff = strings.ReplaceAll(diff, dir+sep, sep)
		diff = strings.ReplaceAll(diff, strings.TrimPrefix(dir, sep)+sep, "")
	}
	return diff
}

// copyTree copies a directory recursively, following the same rules as the
// backup helper: regular files and directories only.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func realHandleCreatePatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ModuleName         string `json:"module_name"`
		DeprecationDetails string `json:"deprecation_details"`
		ProjectPath        string `json:"project_path,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// Resolve project path: use provided project_path or fall back to cwd.
	projectPath := params.ProjectPath
	if projectPath == "" {
		var err error
		projectPath, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	// Determine web root from composer.json scaffold config.
	webRoot := composerutil.ReadWebRoot(projectPath)

	modulePath := filepath.Join(projectPath, webRoot, "modules", "contrib", params.ModuleName)
	if _, err := os.Stat(modulePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("module %s not found at %s", params.ModuleName, modulePath)
	}

	// Snapshot the module before rector touches it. Contrib lives under
	// web/modules/contrib, which every composer-based Drupal project
	// gitignores, so "git diff" against the repository sees nothing at all.
	pristine, err := os.MkdirTemp("", "drup-pristine-*")
	if err != nil {
		return nil, fmt.Errorf("create pristine copy: %w", err)
	}
	defer os.RemoveAll(pristine)
	pristineModule := filepath.Join(pristine, params.ModuleName)
	if err := copyTree(modulePath, pristineModule); err != nil {
		return nil, fmt.Errorf("copy module for diff: %w", err)
	}

	// Run rector on the specific module, through the environment prefix so it
	// uses the site's PHP version.
	configPath, err := ensureRectorConfig(projectPath)
	if err != nil {
		return nil, err
	}
	rectorTarget := modulePath
	if rel, relErr := filepath.Rel(projectPath, modulePath); relErr == nil && isContainerized(projectPath) {
		rectorTarget = rel
	}
	_, _, _, err = cliRun(projectPath, projectRelPath(projectPath, "vendor", "bin", "rector"), "process", rectorTarget, "--config="+configPath)
	if err != nil {
		return nil, fmt.Errorf("rector process: %w", err)
	}

	// Generate diff between the snapshot and the rewritten module.
	// --no-index compares two trees regardless of .gitignore, and exits 1 when
	// they differ.
	stdout, _, exitCode, err := drupexec.Run("git", "diff", "--no-index", "--no-color", pristineModule, modulePath)
	if err != nil {
		return nil, fmt.Errorf("git diff: %w", err)
	}
	stdout = normalizePatchPaths(stdout, pristineModule, modulePath)
	// --no-index reports differences with exit code 1; only higher codes are
	// real failures.
	if exitCode == 1 && stdout != "" {
		exitCode = 0
	}
	if exitCode != 0 || stdout == "" {
		response := map[string]interface{}{
			"patch_path": "",
			"applied":    false,
		}
		return json.Marshal(response)
	}

	// Write patch to temp file.
	// composer.json references patches by path, so they must live inside the
	// project. A file under /tmp disappears and breaks every later
	// composer install.
	patchDir := filepath.Join(projectPath, "patches", params.ModuleName)
	if err := os.MkdirAll(patchDir, 0o755); err != nil {
		return nil, fmt.Errorf("create patch directory: %w", err)
	}
	patchFile, err := os.CreateTemp(patchDir, fmt.Sprintf("drup-%s-*.patch", params.ModuleName))
	// CreateTemp uses 0600; composer and CI need to read the patch too.
	if err != nil {
		return nil, err
	}
	if _, err := patchFile.WriteString(stdout); err != nil {
		patchFile.Close()
		os.Remove(patchFile.Name())
		return nil, err
	}
	patchFile.Close()
	if err := os.Chmod(patchFile.Name(), 0o644); err != nil {
		return nil, fmt.Errorf("set patch permissions: %w", err)
	}

	response := map[string]interface{}{
		"patch_path": patchFile.Name(),
		"applied":    true,
	}
	return json.Marshal(response)
}

// --- New tool handlers ---

func realHandleDetectEnv(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		ForceDetect bool   `json:"force_detect"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	detection, err := defaultEnvDetector.Detect(params.ProjectPath, params.ForceDetect)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"environment":    string(detection.Environment),
		"command_prefix": detection.CommandPrefix,
		"detected_at":    detection.DetectedAt.Format(time.RFC3339),
	}
	return json.Marshal(result)
}

func realHandleComposerRequire(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Package     string `json:"package"`
		Dev         bool   `json:"dev"`
		NoUpdate    bool   `json:"no_update"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.Package == "" {
		return nil, fmt.Errorf("project_path and package are required")
	}

	// Validate package format.
	pkgName := params.Package
	if idx := strings.Index(pkgName, ":"); idx >= 0 {
		pkgName = pkgName[:idx]
	}
	if !composerPackagePattern.MatchString(params.Package) {
		result := map[string]interface{}{
			"success":           false,
			"installed_version": "",
			"stdout":            "",
			"stderr":            fmt.Sprintf("invalid package name format: %s", params.Package),
			"exit_code":         -1,
		}
		return json.Marshal(result)
	}

	// Check composer.json exists.
	composerJSON := filepath.Join(params.ProjectPath, "composer.json")
	if _, err := os.Stat(composerJSON); os.IsNotExist(err) {
		return nil, fmt.Errorf("composer.json not found at %s", params.ProjectPath)
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Dry-run first.
	dryArgs := []string{"require", "--dry-run", params.Package}
	if params.Dev {
		dryArgs = append(dryArgs, "--dev")
	}
	if params.NoUpdate {
		dryArgs = append(dryArgs, "--no-update")
	}
	dryCtx, dryCancel := context.WithTimeout(context.Background(), resolveExecTimeout("composer_require"))
	defer dryCancel()
	_, dryStderr, dryExit, err := drupexec.RunWithEnvCtx(dryCtx, params.ProjectPath, detection.CommandPrefix, "composer", dryArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec composer dry-run: %w", err)
	}
	if dryExit != 0 {
		result := map[string]interface{}{
			"success":           false,
			"installed_version": "",
			"stdout":            "",
			"stderr":            dryStderr,
			"exit_code":         dryExit,
		}
		return json.Marshal(result)
	}

	// Actual require.
	realArgs := []string{"require", params.Package}
	if params.Dev {
		realArgs = append(realArgs, "--dev")
	}
	if params.NoUpdate {
		realArgs = append(realArgs, "--no-update")
	}
	realCtx, realCancel := context.WithTimeout(context.Background(), resolveExecTimeout("composer_require"))
	defer realCancel()
	stdout, stderr, exitCode, err := drupexec.RunWithEnvCtx(realCtx, params.ProjectPath, detection.CommandPrefix, "composer", realArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec composer require: %w", err)
	}

	// Parse installed version from output.
	installedVersion := parseInstalledVersion(stdout, pkgName)

	result := map[string]interface{}{
		"success":           exitCode == 0,
		"installed_version": installedVersion,
		"stdout":            stdout,
		"stderr":            stderr,
		"exit_code":         exitCode,
	}
	return json.Marshal(result)
}

func parseInstalledVersion(output, pkgName string) string {
	// Look for "Installing vendor/package (version)" or "Upgrading vendor/package (version)".
	for _, prefix := range []string{"Installing ", "Upgrading "} {
		idx := strings.Index(output, prefix+pkgName+" (")
		if idx >= 0 {
			start := idx + len(prefix) + len(pkgName) + 2
			end := strings.Index(output[start:], ")")
			if end >= 0 {
				return output[start : start+end]
			}
		}
	}
	return ""
}

func realHandleDrushExec(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string   `json:"project_path"`
		Command     string   `json:"command"`
		Args        []string `json:"args"`
		Format      string   `json:"format"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.Command == "" {
		return nil, fmt.Errorf("project_path and command are required")
	}

	// Check blocklist against the alias-normalized canonical command name.
	if drushBlocklist[normalizeDrushCommand(params.Command)] {
		result := map[string]interface{}{
			"success":   false,
			"output":    "",
			"stderr":    fmt.Sprintf("command '%s' is blocked for safety", params.Command),
			"exit_code": -1,
		}
		return json.Marshal(result)
	}

	// Check shell metacharacters in command.
	if shellMetacharPattern.MatchString(params.Command) {
		result := map[string]interface{}{
			"success":   false,
			"output":    "",
			"stderr":    "command contains shell metacharacters",
			"exit_code": -1,
		}
		return json.Marshal(result)
	}

	// Check shell metacharacters in args.
	for _, arg := range params.Args {
		if shellMetacharPattern.MatchString(arg) {
			result := map[string]interface{}{
				"success":   false,
				"output":    "",
				"stderr":    fmt.Sprintf("argument '%s' contains shell metacharacters", arg),
				"exit_code": -1,
			}
			return json.Marshal(result)
		}
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Build command args.
	cmdArgs := []string{params.Command}
	cmdArgs = append(cmdArgs, params.Args...)
	cmdArgs = append(cmdArgs, "--root="+params.ProjectPath)
	if params.Format != "" {
		cmdArgs = append(cmdArgs, "--format="+params.Format)
	}

	drushCtx, drushCancel := context.WithTimeout(context.Background(), resolveExecTimeout("drush_exec"))
	defer drushCancel()
	stdout, stderr, exitCode, err := drupexec.RunWithEnvCtx(drushCtx, params.ProjectPath, detection.CommandPrefix, "drush", cmdArgs...)
	if err != nil {
		return nil, fmt.Errorf("exec drush: %w", err)
	}

	// Parse JSON output if format is json.
	var output interface{} = stdout
	if params.Format == "json" {
		var parsed interface{}
		if jsonErr := json.Unmarshal([]byte(stdout), &parsed); jsonErr == nil {
			output = parsed
		} else {
			if stderr != "" {
				stderr += "\n"
			}
			stderr += "warning: failed to parse JSON output"
		}
	}

	result := map[string]interface{}{
		"success":   exitCode == 0,
		"output":    output,
		"stderr":    stderr,
		"exit_code": exitCode,
	}
	return json.Marshal(result)
}

func realHandleContribUpgradePath(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module           string `json:"module_machine_name"`
		CurrentDrupalVer string `json:"current_drupal_version"`
		TargetDrupalVer  string `json:"target_drupal_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}
	if params.CurrentDrupalVer == "" || params.TargetDrupalVer == "" {
		return nil, fmt.Errorf("current_drupal_version and target_drupal_version are required")
	}

	rec, err := drupalorg.UpgradePath(params.Module, params.CurrentDrupalVer, params.TargetDrupalVer)
	if err != nil {
		return nil, err
	}
	return json.Marshal(rec)
}

func realHandleModuleReleaseInfo(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module      string `json:"module_machine_name"`
		CoreVersion string `json:"core_version,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}
	if params.CoreVersion != "" && drupalorg.MajorFromVersion(params.CoreVersion) == 0 {
		return nil, fmt.Errorf("invalid core_version: %s", params.CoreVersion)
	}

	result, err := drupalorg.ModuleReleaseInfo(params.Module, params.CoreVersion)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleUpgradeScan(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Scope       string `json:"scope"`
		Module      string `json:"module"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	// Check for path traversal.
	if strings.Contains(params.ProjectPath, "..") {
		return nil, fmt.Errorf("project_path must not contain '..' segments")
	}

	// Get environment prefix.
	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}

	// Check if upgrade_status is in composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	var composerJSON map[string]interface{}
	if err := json.Unmarshal(composerData, &composerJSON); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	upgradeStatusInstalled := hasPackage(composerJSON, "drupal/upgrade_status")

	upgradeScanTimeout := resolveExecTimeout("upgrade_scan")

	// Install if needed. This calls straight into realHandleComposerRequire,
	// bypassing s.tools and the registration-time guardHandler wrapping
	// composer_require entirely, so it re-enters the exact same guardedCall
	// path inline instead — the same session/backup-freshness/mutation-cap
	// partition and audit trail a direct composer_require call would get.
	if !upgradeStatusInstalled {
		reqArgs := json.RawMessage(fmt.Sprintf(`{"project_path":%q,"package":"drupal/upgrade_status","dev":true}`, params.ProjectPath))
		if _, reqErr := guardedCall("composer_require", reqArgs, realHandleComposerRequire); reqErr != nil {
			return nil, fmt.Errorf("install upgrade_status: %w", reqErr)
		}
	}

	// Ensure upgrade_status is enabled (shared helper — also used by
	// realHandleScan for DRY compliance). Uses upgradeScanTimeout, not the
	// flat default, so the "upgrade_scan" exec-timeout override still governs
	// every drush call this handler makes, not just the final analyze call.
	if err := ensureUpgradeStatusEnabled(params.ProjectPath, upgradeScanTimeout); err != nil {
		return nil, err
	}
	upgradeStatusEnabled := true

	// Run analysis. "--all" is a flag; passing a bare "all" makes
	// upgrade_status look for a project by that name and report nothing.
	analyzeTarget := "--all"
	if params.Module != "" {
		analyzeTarget = params.Module
	}
	analyzeArgs := []string{"upgrade_status:analyze", analyzeTarget, "--format=checkstyle", "--root=" + params.ProjectPath}
	analyzeCtx, analyzeCancel := context.WithTimeout(context.Background(), upgradeScanTimeout)
	analyzeStdout, analyzeStderr, analyzeExit, analyzeErr := drupexec.RunWithEnvCtx(analyzeCtx, params.ProjectPath, detection.CommandPrefix, "drush", analyzeArgs...)
	analyzeCancel()
	if analyzeErr != nil {
		return nil, drushExecError("drush", analyzeArgs, -1, analyzeErr.Error(), "")
	}

	// Parse results.
	scanResult, parseErr := scan.ParseCheckstyle(strings.NewReader(analyzeStdout))

	// Filter by scope if specified.
	scope, scopeErr := normalizeScope(params.Scope)
	if scopeErr != nil {
		return nil, scopeErr
	}
	var modules []interface{}
	totalErrors := 0
	if parseErr == nil && scanResult != nil {
		for _, mod := range scanResult.Modules {
			if scope != "" && string(mod.Type) != scope {
				continue
			}
			modules = append(modules, map[string]interface{}{
				"name":     mod.Name,
				"category": string(mod.Type),
				"errors":   len(mod.Errors),
			})
			totalErrors += len(mod.Errors)
		}
	}

	if modules == nil {
		modules = []interface{}{}
	}

	// Handle partial results.
	if analyzeExit != 0 && parseErr != nil {
		result := map[string]interface{}{
			"total_errors":             0,
			"modules":                  modules,
			"upgrade_status_installed": true,
			"upgrade_status_enabled":   upgradeStatusEnabled,
			"warning":                  "partial results: " + analyzeStderr,
		}
		return json.Marshal(result)
	}

	result := map[string]interface{}{
		"total_errors":             totalErrors,
		"modules":                  modules,
		"upgrade_status_installed": true,
		"upgrade_status_enabled":   upgradeStatusEnabled,
	}
	return json.Marshal(result)
}

func hasPackage(composerJSON map[string]interface{}, pkg string) bool {
	for _, section := range []string{"require", "require-dev"} {
		if deps, ok := composerJSON[section].(map[string]interface{}); ok {
			if _, exists := deps[pkg]; exists {
				return true
			}
		}
	}
	return false
}

// ensureUpgradeStatusEnabled checks if upgrade_status is enabled and enables it
// if present in composer.json but not enabled. Returns an error if the module
// is not installed or if enabling fails. timeout bounds each drush call —
// callers pass their own resolveExecTimeout(toolName) result so a caller with
// a longer override (e.g. "upgrade_scan") is not silently clamped down to the
// flat default.
func ensureUpgradeStatusEnabled(projectPath string, timeout time.Duration) error {
	detection, err := defaultEnvDetector.Detect(projectPath, false)
	if err != nil {
		return err
	}

	run := func(args ...string) (string, string, int, error) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		return drupexec.RunWithEnvCtx(ctx, projectPath, detection.CommandPrefix, "drush", args...)
	}

	// 1. Check composer.json for upgrade_status.
	composerPath := filepath.Join(projectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return fmt.Errorf("read composer.json: %w", err)
	}
	var composerJSON map[string]interface{}
	if err := json.Unmarshal(composerData, &composerJSON); err != nil {
		return fmt.Errorf("parse composer.json: %w", err)
	}
	if !hasPackage(composerJSON, "drupal/upgrade_status") {
		return fmt.Errorf("upgrade_status is not installed; run composer require drupal/upgrade_status first")
	}

	// 2. Check pm:list --status=enabled.
	pmListArgs := []string{"pm:list", "--status=enabled", "--format=json", "--root=" + projectPath}
	pmStdout, _, pmExit, _ := run(pmListArgs...)
	if pmExit == 0 {
		var pmData map[string]interface{}
		if json.Unmarshal([]byte(pmStdout), &pmData) == nil {
			if _, ok := pmData["upgrade_status"]; ok {
				return nil // already enabled
			}
		}
	}

	// 3. Auto-enable sequence: config:delete → drush en → drush cr.
	cdArgs := []string{"config:delete", "update.settings", "--root=" + projectPath}
	_, _, _, _ = run(cdArgs...)

	enArgs := []string{"en", "upgrade_status", "-y", "--root=" + projectPath}
	_, enStderr, enExit, enErr := run(enArgs...)
	if enErr != nil {
		return fmt.Errorf("enable upgrade_status: %w", enErr)
	}
	if enExit != 0 {
		return fmt.Errorf("enable upgrade_status failed (exit %d): %s", enExit, enStderr)
	}

	crArgs := []string{"cr", "--root=" + projectPath}
	_, _, _, _ = run(crArgs...)
	return nil
}

func realHandlePatchStatus(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	if params.PatchURL == "" && params.ComposerPackage == "" {
		return nil, fmt.Errorf("patch_url or composer_package is required")
	}

	// Read composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	var composerJSON struct {
		Extra struct {
			Patches map[string]patchList `json:"patches"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(composerData, &composerJSON); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	type patchInfoResult struct {
		URL         string `json:"url"`
		Package     string `json:"package"`
		Description string `json:"description"`
	}

	registeredInComposer := false
	var foundPatchInfo *patchInfoResult

	// Search patches.
	for pkg, entries := range composerJSON.Extra.Patches {
		if params.ComposerPackage != "" && pkg != params.ComposerPackage {
			continue
		}
		for _, entry := range entries {
			match := false
			if params.PatchURL != "" {
				match = entry.URL == params.PatchURL || strings.Contains(entry.URL, params.PatchURL) || strings.Contains(params.PatchURL, entry.URL)
			} else {
				match = true // Just matching by package.
			}
			if match {
				registeredInComposer = true
				foundPatchInfo = &patchInfoResult{
					URL:         entry.URL,
					Package:     pkg,
					Description: entry.Description,
				}
				break
			}
		}
		if foundPatchInfo != nil {
			break
		}
	}

	// Check git log for the patch commit. drup commits patches as
	// "apply D11 patch to <package>", so searching for the URL never matched
	// its own commits.
	commitHash := ""
	pkg := params.ComposerPackage
	if pkg == "" && foundPatchInfo != nil {
		pkg = foundPatchInfo.Package
	}
	searchTerms := []string{}
	if pkg != "" {
		searchTerms = append(searchTerms, patch.CommitSubject(pkg))
	}
	if params.PatchURL != "" {
		searchTerms = append(searchTerms, params.PatchURL)
	}
	if foundPatchInfo != nil && foundPatchInfo.URL != "" {
		searchTerms = append(searchTerms, foundPatchInfo.URL)
	}
	for _, searchTerm := range searchTerms {
		stdout, _, gitExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "--fixed-strings", "--grep="+searchTerm)
		if gitExit != 0 || strings.TrimSpace(stdout) == "" {
			continue
		}
		parts := strings.Fields(strings.Split(strings.TrimSpace(stdout), "\n")[0])
		if len(parts) > 0 {
			commitHash = parts[0]
			break
		}
	}

	// Determine is_applied.
	isApplied := registeredInComposer // If registered, assume applied.
	if registeredInComposer && commitHash != "" {
		// Check if there's a revert commit.
		revertStdout, _, revertExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "--grep=Revert")
		if revertExit == 0 && strings.Contains(revertStdout, commitHash) {
			isApplied = false
		}
	}

	result := map[string]interface{}{
		"is_applied":             isApplied,
		"commit_hash":            commitHash,
		"registered_in_composer": registeredInComposer,
		"patch_info":             foundPatchInfo,
	}
	return json.Marshal(result)
}

type patchEntry struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

// patchList reads the composer-patches entries for one package. The supported
// format maps a description to a patch URL; drup used to write a list of
// objects instead, so both shapes are accepted on read.
type patchList []patchEntry

func (p *patchList) UnmarshalJSON(data []byte) error {
	var asMap map[string]string
	if err := json.Unmarshal(data, &asMap); err == nil {
		descriptions := make([]string, 0, len(asMap))
		for description := range asMap {
			descriptions = append(descriptions, description)
		}
		sort.Strings(descriptions)
		list := make(patchList, 0, len(asMap))
		for _, description := range descriptions {
			list = append(list, patchEntry{URL: asMap[description], Description: description})
		}
		*p = list
		return nil
	}

	var asList []patchEntry
	if err := json.Unmarshal(data, &asList); err != nil {
		return fmt.Errorf("unsupported extra.patches entry: %w", err)
	}
	*p = asList
	return nil
}

// MarshalJSON always writes the format composer-patches understands.
func (p patchList) MarshalJSON() ([]byte, error) {
	out := make(map[string]string, len(p))
	for _, entry := range p {
		key := entry.Description
		if key == "" {
			key = entry.URL
		}
		out[key] = entry.URL
	}
	return json.Marshal(out)
}

func realHandlePatchRollback(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		PatchURL        string `json:"patch_url"`
		ComposerPackage string `json:"composer_package"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.PatchURL == "" || params.ComposerPackage == "" {
		return nil, fmt.Errorf("project_path, patch_url, and composer_package are required")
	}

	// Verify it's a git repo.
	_, _, gitExit, _ := drupexec.Run("git", "-C", params.ProjectPath, "rev-parse", "--git-dir")
	if gitExit != 0 {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "not a git repository",
		}
		return json.Marshal(result)
	}

	// Check working tree clean.
	statusStdout, _, _, _ := drupexec.Run("git", "-C", params.ProjectPath, "status", "--porcelain")
	if strings.TrimSpace(statusStdout) != "" {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "working tree is dirty; commit or stash changes first",
		}
		return json.Marshal(result)
	}

	// Get patch status to find commit hash.
	statusArgs := json.RawMessage(fmt.Sprintf(`{"project_path":%q,"patch_url":%q,"composer_package":%q}`,
		params.ProjectPath, params.PatchURL, params.ComposerPackage))
	statusResult, err := realHandlePatchStatus(statusArgs)
	if err != nil {
		return nil, err
	}

	var status struct {
		IsApplied  bool   `json:"is_applied"`
		CommitHash string `json:"commit_hash"`
		PatchInfo  *struct {
			URL     string `json:"url"`
			Package string `json:"package"`
		} `json:"patch_info"`
	}
	if err := json.Unmarshal(statusResult, &status); err != nil {
		return nil, err
	}

	if !status.IsApplied {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "patch is not applied",
		}
		return json.Marshal(result)
	}

	if status.CommitHash == "" {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 "cannot find patch commit to revert",
		}
		return json.Marshal(result)
	}

	// Keep a copy of the patch: reverting the commit removes the file, and the
	// patched code itself lives in a gitignored directory that the revert
	// cannot touch.
	patchCopy := ""
	if info := status.PatchInfo; info != nil && info.URL != "" {
		if local, isLocal, localErr := patch.LocalPatchPath(info.URL, params.ProjectPath); localErr == nil && isLocal {
			if data, readErr := os.ReadFile(local); readErr == nil {
				if tmp, tmpErr := os.CreateTemp("", "drup-rollback-*.patch"); tmpErr == nil {
					if _, writeErr := tmp.Write(data); writeErr == nil {
						patchCopy = tmp.Name()
						defer os.Remove(patchCopy)
					}
					tmp.Close()
				}
			}
		}
	}

	// Step 1: git revert (atomic — must succeed before modifying composer.json).
	revertStdout, revertStderr, revertExit, revertErr := drupexec.Run("git", "-C", params.ProjectPath, "revert", status.CommitHash, "--no-edit")
	if revertErr != nil {
		return nil, fmt.Errorf("git revert: %w", revertErr)
	}
	if revertExit != 0 {
		result := map[string]interface{}{
			"success":               false,
			"reverted_commit":       "",
			"removed_from_composer": false,
			"error":                 fmt.Sprintf("revert conflict: %s", revertStderr),
		}
		return json.Marshal(result)
	}

	// Reverse the patch in the package directory when the code is not tracked.
	if patchCopy != "" {
		pkgName := params.ComposerPackage
		if pkgName == "" && status.PatchInfo != nil {
			pkgName = status.PatchInfo.Package
		}
		patch.ReverseInPackage(params.ProjectPath, pkgName, patchCopy)
	}

	// Get new revert commit hash.
	revertedCommit := ""
	if revertStdout != "" {
		parts := strings.Fields(strings.TrimSpace(revertStdout))
		if len(parts) >= 2 {
			revertedCommit = strings.TrimPrefix(parts[1], "]")
			// Try to get the actual hash from git log.
			logStdout, _, _, _ := drupexec.Run("git", "-C", params.ProjectPath, "log", "--oneline", "-1")
			if logStdout != "" {
				logParts := strings.Fields(strings.TrimSpace(logStdout))
				if len(logParts) > 0 {
					revertedCommit = logParts[0]
				}
			}
		}
	}

	// Step 2: Remove from composer.json.
	composerPath := filepath.Join(params.ProjectPath, "composer.json")
	composerData, err := os.ReadFile(composerPath)
	if err != nil {
		result := map[string]interface{}{
			"success":               true,
			"reverted_commit":       revertedCommit,
			"removed_from_composer": false,
			"error":                 "warning: could not read composer.json",
		}
		return json.Marshal(result)
	}

	var composerMap map[string]json.RawMessage
	if err := json.Unmarshal(composerData, &composerMap); err != nil {
		result := map[string]interface{}{
			"success":               true,
			"reverted_commit":       revertedCommit,
			"removed_from_composer": false,
			"error":                 "warning: could not parse composer.json",
		}
		return json.Marshal(result)
	}

	// Remove patch entry from extra.patches.
	var extra map[string]json.RawMessage
	if raw, ok := composerMap["extra"]; ok {
		json.Unmarshal(raw, &extra)
	}
	if extra != nil {
		var patches map[string]json.RawMessage
		if raw, ok := extra["patches"]; ok {
			json.Unmarshal(raw, &patches)
		}
		if patches != nil {
			if raw, ok := patches[params.ComposerPackage]; ok {
				var entries patchList
				json.Unmarshal(raw, &entries)
				// Filter out the matching patch.
				var remaining patchList
				for _, e := range entries {
					if e.URL != params.PatchURL {
						remaining = append(remaining, e)
					}
				}
				if len(remaining) == 0 {
					delete(patches, params.ComposerPackage)
				} else {
					newEntries, _ := json.Marshal(remaining)
					patches[params.ComposerPackage] = newEntries
				}
			}
			newPatches, _ := json.Marshal(patches)
			extra["patches"] = newPatches
		}
		newExtra, _ := json.Marshal(extra)
		composerMap["extra"] = newExtra
	}

	updatedComposer, _ := json.MarshalIndent(composerMap, "", "    ")
	os.WriteFile(composerPath, updatedComposer, 0o644)

	// Step 3: composer update for the package.
	detection, _ := defaultEnvDetector.Detect(params.ProjectPath, false)
	_, compStderr, compExit, _ := drupexec.RunWithEnv(params.ProjectPath, detection.CommandPrefix, "composer", "update", params.ComposerPackage)

	result := map[string]interface{}{
		"success":               true,
		"reverted_commit":       revertedCommit,
		"removed_from_composer": true,
	}
	if compExit != 0 {
		result["error"] = fmt.Sprintf("warning: composer update failed: %s", compStderr)
	}
	return json.Marshal(result)
}

func realHandleGenerateReport(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath      string `json:"project_path"`
		ReportType       string `json:"report_type"`
		IncludeScanData  bool   `json:"include_scan_data"`
		IncludePatchList bool   `json:"include_patch_list"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	reportType := params.ReportType
	if reportType == "" {
		reportType = "both"
	}
	// An unrecognized type used to return success with no files written.
	switch reportType {
	case "json", "markdown", "both":
	default:
		return nil, fmt.Errorf("invalid report_type %q: use json, markdown or both", params.ReportType)
	}

	// Collect report data.
	data := &report.ReportData{
		ProjectPath:     params.ProjectPath,
		Resolved:        []report.ResolvedItem{},
		Pending:         []report.PendingItem{},
		PipelineMetrics: snapshotMetrics(),
	}

	// Get live scan data if requested
	modulesChecked := 0
	if params.IncludeScanData {
		result, filtered, err := DoValidate(params.ProjectPath, "")
		if err != nil {
			return nil, fmt.Errorf("scan for report: %w", err)
		}
		data.TotalErrors = len(filtered)
		if result != nil {
			modulesChecked = len(result.Modules)
		}

		// Convert scan errors to pending items
		for _, depErr := range filtered {
			data.Pending = append(data.Pending, report.PendingItem{
				Module:          extractModuleName(depErr.File),
				Type:            string(depErr.Severity),
				Error:           depErr.Message,
				SuggestedAction: fmt.Sprintf("Fix deprecation at %s:%d", depErr.File, depErr.Line),
			})
		}
	}

	// Count patches from composer.json.
	if params.IncludePatchList {
		composerPath := filepath.Join(params.ProjectPath, "composer.json")
		if composerData, err := os.ReadFile(composerPath); err == nil {
			var composerJSON struct {
				Extra struct {
					Patches map[string]patchList `json:"patches"`
				} `json:"extra"`
			}
			if json.Unmarshal(composerData, &composerJSON) == nil {
				for _, entries := range composerJSON.Extra.Patches {
					for range entries {
						data.TokenAccounting.Total++
					}
				}
			}
		}
	}

	result := map[string]interface{}{
		"success":              true,
		"json_report_path":     "",
		"markdown_report_path": "",
		"summary": map[string]interface{}{
			"total_modules_checked": modulesChecked,
			"patches_applied":       data.TokenAccounting.Total,
			"errors_remaining":      data.TotalErrors,
		},
	}

	if reportType == "json" || reportType == "both" {
		jsonData, err := report.GenerateJSON(data)
		if err != nil {
			return nil, fmt.Errorf("generate JSON report: %w", err)
		}
		jsonPath := filepath.Join(params.ProjectPath, "drup-report.json")
		if err := os.WriteFile(jsonPath, jsonData, 0o644); err != nil {
			return nil, fmt.Errorf("write JSON report: %w", err)
		}
		result["json_report_path"] = jsonPath
	}

	if reportType == "markdown" || reportType == "both" {
		mdData, err := report.GenerateMarkdown(data)
		if err != nil {
			return nil, fmt.Errorf("generate markdown report: %w", err)
		}
		mdPath := filepath.Join(params.ProjectPath, "drup-report.md")
		if err := os.WriteFile(mdPath, []byte(mdData), 0o644); err != nil {
			return nil, fmt.Errorf("write markdown report: %w", err)
		}
		result["markdown_report_path"] = mdPath
	}

	return json.Marshal(result)
}

func realHandleCustomCompatFix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath   string `json:"project_path"`
		TargetVersion string `json:"target_version,omitempty"`
		DryRun        bool   `json:"dry_run,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	target := params.TargetVersion
	if target == "" {
		target = "11"
	}

	result, err := BumpCustomCoreCompat(params.ProjectPath, target, params.DryRun)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleContribAllowLenient(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string   `json:"project_path"`
		Packages    []string `json:"packages"`
		DryRun      bool     `json:"dry_run,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	result, err := AllowLenient(params.ProjectPath, params.Packages, params.DryRun)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleContribCompatPatch(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath     string `json:"project_path"`
		Module          string `json:"module_machine_name"`
		TargetVersion   string `json:"target_version,omitempty"`
		DryRun          bool   `json:"dry_run,omitempty"`
		DeclarationOnly bool   `json:"declaration_only,omitempty"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" || params.Module == "" {
		return nil, fmt.Errorf("project_path and module_machine_name are required")
	}
	target := params.TargetVersion
	if target == "" {
		target = "11"
	}

	result, err := PatchContribForCore(params.ProjectPath, params.Module, target, params.DryRun, params.DeclarationOnly)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleModuleInfo(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module             string `json:"module_machine_name"`
		IncludeMaintainers bool   `json:"include_maintainers"`
		IncludeDeps        bool   `json:"include_dependencies"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}

	meta, err := drupalorg.ModuleInfo(params.Module)
	if err != nil {
		return nil, err
	}

	if !params.IncludeMaintainers {
		meta.Maintainers = []string{}
	}

	return json.Marshal(meta)
}

// versionMatrixData holds static Drupal/PHP compatibility data.
var versionMatrixData = map[string]struct {
	PHPMin         string `json:"minimum"`
	PHPRecommended string `json:"recommended"`
	SupportedUntil string `json:"supported_until"`
	NextMajor      string `json:"next_major"`
}{
	"9":  {PHPMin: "7.3", PHPRecommended: "8.1", SupportedUntil: "2024-06", NextMajor: "10"},
	"10": {PHPMin: "8.1", PHPRecommended: "8.3", SupportedUntil: "2026-06", NextMajor: "11"},
	"11": {PHPMin: "8.3", PHPRecommended: "8.4", SupportedUntil: "TBA", NextMajor: ""},
}

var phpVersionPattern = regexp.MustCompile(`^\d+\.\d+$`)

func realHandleDrupalVersionMatrix(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		DrupalVersion string `json:"drupal_version"`
		PHPVersion    string `json:"php_version"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}

	// If drupal_version provided, look up directly.
	if params.DrupalVersion != "" {
		entry, ok := versionMatrixData[params.DrupalVersion]
		if !ok {
			return nil, fmt.Errorf("unknown Drupal version: %s", params.DrupalVersion)
		}
		result := map[string]interface{}{
			"drupal_version": params.DrupalVersion,
			"php_requirements": map[string]string{
				"minimum":     entry.PHPMin,
				"recommended": entry.PHPRecommended,
			},
			"supported_until": entry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": entry.NextMajor,
			},
		}
		return json.Marshal(result)
	}

	// If php_version provided, find compatible Drupal versions.
	if params.PHPVersion != "" {
		if !phpVersionPattern.MatchString(params.PHPVersion) {
			return nil, fmt.Errorf("invalid php_version format: %s", params.PHPVersion)
		}
		// Find the latest Drupal version compatible with this PHP version.
		var latestVersion string
		var latestEntry struct {
			PHPMin         string
			PHPRecommended string
			SupportedUntil string
			NextMajor      string
		}
		for ver, entry := range versionMatrixData {
			if isPHPCompatible(params.PHPVersion, entry.PHPMin, entry.PHPRecommended) {
				if ver > latestVersion {
					latestVersion = ver
					latestEntry = struct {
						PHPMin         string
						PHPRecommended string
						SupportedUntil string
						NextMajor      string
					}{entry.PHPMin, entry.PHPRecommended, entry.SupportedUntil, entry.NextMajor}
				}
			}
		}
		if latestVersion == "" {
			return nil, fmt.Errorf("no Drupal version compatible with PHP %s", params.PHPVersion)
		}
		result := map[string]interface{}{
			"drupal_version": latestVersion,
			"php_requirements": map[string]string{
				"minimum":     latestEntry.PHPMin,
				"recommended": latestEntry.PHPRecommended,
			},
			"supported_until": latestEntry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": latestEntry.NextMajor,
			},
		}
		return json.Marshal(result)
	}

	// Neither provided — return full matrix.
	var matrix []interface{}
	for ver, entry := range versionMatrixData {
		matrix = append(matrix, map[string]interface{}{
			"drupal_version": ver,
			"php_requirements": map[string]string{
				"minimum":     entry.PHPMin,
				"recommended": entry.PHPRecommended,
			},
			"supported_until": entry.SupportedUntil,
			"upgrade_path": map[string]string{
				"next_major": entry.NextMajor,
			},
		})
	}
	return json.Marshal(map[string]interface{}{"matrix": matrix})
}

func isPHPCompatible(phpVer, phpMin, phpRecommended string) bool {
	v, err := semver.Parse(phpVer)
	if err != nil {
		return false
	}
	return semver.Satisfies(v, ">="+phpMin)
}

// --- Core upgrade + patch reconcile tool handlers ---

func realHandleCoreUpgradeCheck(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	resolvedPath, err := coreupgrade.ValidateProjectPath(params.ProjectPath)
	if err != nil {
		return nil, err
	}
	params.ProjectPath = resolvedPath

	detection, err := defaultEnvDetector.Detect(params.ProjectPath, false)
	if err != nil {
		return nil, err
	}
	if detection.Environment == envdetect.EnvUnsupported {
		result := map[string]interface{}{
			"current_version":        "",
			"next_version":           "",
			"composer_patch_preview": "",
			"supported":              false,
		}
		return json.Marshal(result)
	}

	currentVersion, err := coreCurrentVersion(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	check, err := coreupgrade.NextMajor(currentVersion)
	if err != nil {
		return nil, err
	}

	preview := ""
	if check.Available {
		composerData, rerr := os.ReadFile(filepath.Join(params.ProjectPath, "composer.json"))
		if rerr != nil {
			return nil, fmt.Errorf("read composer.json: %w", rerr)
		}
		diff, _, perr := coreupgrade.PreviewComposerPatch(composerData, check.Constraint)
		if perr != nil {
			return nil, perr
		}
		preview = diff
	}

	result := map[string]interface{}{
		"current_version":        currentVersion,
		"next_version":           check.NextVersion,
		"composer_patch_preview": preview,
		"supported":              true,
	}
	return json.Marshal(result)
}

// coreCurrentVersion resolves the installed drupal/core version: prefers
// composer.lock's exact pinned version, falling back to the composer.json
// require constraint when no lock file is present.
func coreCurrentVersion(projectPath string) (string, error) {
	if data, err := os.ReadFile(filepath.Join(projectPath, "composer.lock")); err == nil {
		var lock struct {
			Packages []struct {
				Name    string `json:"name"`
				Version string `json:"version"`
			} `json:"packages"`
		}
		if json.Unmarshal(data, &lock) == nil {
			for _, p := range lock.Packages {
				if p.Name == drupalCorePackageName {
					return strings.TrimPrefix(p.Version, "v"), nil
				}
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "composer.json"))
	if err != nil {
		return "", fmt.Errorf("read composer.json: %w", err)
	}
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "", fmt.Errorf("parse composer.json: %w", err)
	}
	for _, key := range []string{"drupal/core-recommended", drupalCorePackageName} {
		if v, ok := doc.Require[key]; ok {
			return v, nil
		}
	}
	return "", fmt.Errorf("drupal/core not found in composer.json or composer.lock")
}

// drupalCorePackageName is the composer package name for Drupal core itself.
const drupalCorePackageName = "drupal/core"

func realHandleCoreUpgradeApply(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath   string `json:"project_path"`
		TargetVersion string `json:"target_version"`
		DryRun        bool   `json:"dry_run"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}
	if params.TargetVersion == "" {
		return nil, fmt.Errorf("target_version is required")
	}

	// allow_dirty is CLI-only (drup upgrade-core --allow-dirty per proposal
	// decision 3); the MCP surface never accepts a dirty-tree override, so
	// allowDirty is always false here regardless of what the caller sends —
	// closing the schema-undocumented-argument backdoor (G4/G5/S2).
	result, err := coreupgrade.Apply(params.ProjectPath, params.TargetVersion, params.DryRun, false, false)
	if err != nil {
		return nil, err
	}

	response := map[string]interface{}{
		"success":             result.Success,
		"report":              result.Report,
		"rollback_checkpoint": result.RollbackCheckpoint,
		"stderr":              result.Stderr,
	}
	return json.Marshal(response)
}

func realHandlePatchReconcile(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		Module          string `json:"module_machine_name"`
		CurrentPatchURL string `json:"current_patch_url"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if !moduleNamePattern.MatchString(params.Module) {
		return nil, fmt.Errorf("invalid module machine name: %s", params.Module)
	}
	if params.CurrentPatchURL == "" {
		return nil, fmt.Errorf("current_patch_url is required")
	}

	result, err := patchreconcile.Reconcile(params.Module, params.CurrentPatchURL)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

func realHandleCleanup(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath    string `json:"project_path"`
		ValidatePassed bool   `json:"validate_passed"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	cliArgs := []string{params.ProjectPath}
	if params.ValidatePassed {
		cliArgs = append(cliArgs, "--validate-passed")
	} else {
		cliArgs = append(cliArgs, "--validate-failed")
	}

	// Capture RunCleanup's output directly into an in-memory buffer instead
	// of swapping the process-global os.Stdout through an os.Pipe. A pipe
	// has a fixed kernel buffer and needs a concurrent reader draining it;
	// a bytes.Buffer just grows, so there is no deadlock risk regardless of
	// output size, and no global state is mutated for the call's duration.
	var buf bytes.Buffer
	err := RunCleanup(&buf, cliArgs)
	if err != nil {
		return nil, err
	}

	// Parse the JSON output from RunCleanup and return it.
	var result interface{}
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr == nil {
		return json.Marshal(result)
	}
	// If not valid JSON, wrap the output.
	return json.Marshal(map[string]interface{}{
		"success": true,
		"output":  buf.String(),
	})
}
