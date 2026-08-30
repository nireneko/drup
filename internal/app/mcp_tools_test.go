package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nireneko/drup/internal/audit"
	"github.com/nireneko/drup/internal/drupalorg"
	"github.com/nireneko/drup/internal/envdetect"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/runstate"
	"github.com/nireneko/drup/internal/session"
)

func TestWireMCPTools_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)
	// Verify WireMCPTools runs without panic and server is usable.
	t.Log("WireMCPTools registered successfully")
}

func TestWireMCPTools_AllToolsRegistered(t *testing.T) {
	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	expected := 41
	actual := server.ToolCount()
	if actual != expected {
		t.Errorf("expected %d tools, got %d", expected, actual)
	}

	// Diagnostic: list registered tools on failure.
	if actual != expected {
		names := server.ToolNames()
		t.Logf("registered tools (%d): %s", len(names), strings.Join(names, ", "))
	}
}

func TestWireMCPTools_DescriptorRefuseToolsCannotBypassSessionGuard(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)
	projectPath := t.TempDir()

	for _, spec := range mcp.ToolSpecs() {
		if spec.Effect != mcp.EffectMutating || spec.SessionPolicy != mcp.SessionPolicyRefuse {
			continue
		}
		t.Run(spec.Name, func(t *testing.T) {
			_, err := server.CallTool(spec.Name, json.RawMessage(`{"project_path":`+jsonStr(projectPath)+`}`))
			if err == nil || !strings.Contains(err.Error(), "session_open") {
				t.Fatalf("%s error = %v, want descriptor-driven session refusal", spec.Name, err)
			}
		})
	}
}

func TestRealHandleContribCheck_InvalidJSON(t *testing.T) {
	_, err := realHandleContribCheck(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleIssuePatches_MissingParams(t *testing.T) {
	_, err := realHandleIssuePatches(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing module_name and issue_nid, got nil")
	}
}

func TestRealHandleApplyPatch_InvalidJSON(t *testing.T) {
	_, err := realHandleApplyPatch(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleValidate_InvalidJSON(t *testing.T) {
	_, err := realHandleValidate(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleCreatePatch_InvalidJSON(t *testing.T) {
	_, err := realHandleCreatePatch(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleScan_InvalidJSON(t *testing.T) {
	_, err := realHandleScan(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRealHandleAutofix_InvalidJSON(t *testing.T) {
	_, err := realHandleAutofix(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestRunPreflight_Dispatch(t *testing.T) {
	// Run the dispatch check in an isolated project directory and stub external
	// commands. RunPreflight may otherwise invoke Composer in the package
	// directory, leaving composer.json, composer.lock, vendor/, and MCP metadata
	// behind in the working tree.
	t.Chdir(t.TempDir())

	origRun := drupexec.Run
	origRunWithEnv := drupexec.RunWithEnv
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		return "", "unavailable in dispatch test", 1, nil
	}
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		return "", "unavailable in dispatch test", 1, nil
	}
	t.Cleanup(func() {
		drupexec.Run = origRun
		drupexec.RunWithEnv = origRunWithEnv
	})

	// Verify preflight command is dispatched correctly.
	err := Run([]string{"preflight"})
	// Will fail because we're not in a Drupal project, but should not be "unknown command".
	if err != nil && err.Error() == `unknown command "preflight"` {
		t.Error("preflight should be a known command")
	}
}

// --- RED tests: security threats ---

func TestComposerRequire_ShellInjection(t *testing.T) {
	// Verify composer_require rejects package with shell injection.
	args := json.RawMessage(`{"project_path":"/tmp","package":"\"; rm -rf /"}`)
	result, err := realHandleComposerRequire(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] == true {
		t.Error("expected success=false for shell injection package name")
	}
	if stderr, ok := resp["stderr"].(string); ok {
		if !strings.Contains(stderr, "invalid package name") {
			t.Errorf("stderr = %q, want it to mention invalid package name", stderr)
		}
	}
}

func TestDrushExec_Blocklist(t *testing.T) {
	blocked := []string{"sql-drop", "site-install", "site:install", "sql-sanitize", "php-eval", "core:execute-cli"}
	for _, cmd := range blocked {
		t.Run(cmd, func(t *testing.T) {
			args := json.RawMessage(`{"project_path":"/tmp","command":"` + cmd + `"}`)
			result, err := realHandleDrushExec(args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var resp map[string]interface{}
			json.Unmarshal(result, &resp)
			if resp["success"] == true {
				t.Errorf("expected success=false for blocked command %q", cmd)
			}
			if stderr, ok := resp["stderr"].(string); ok {
				if !strings.Contains(stderr, "blocked for safety") {
					t.Errorf("stderr = %q, want it to mention 'blocked for safety'", stderr)
				}
			}
		})
	}
}

func TestDrushExec_ShellMetacharacters(t *testing.T) {
	args := json.RawMessage(`{"project_path":"/tmp","command":"status; rm -rf /"}`)
	result, err := realHandleDrushExec(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] == true {
		t.Error("expected success=false for command with shell metacharacters")
	}
}

// Task 3.1: normalizeDrushCommand trims, lowercases, and resolves known
// aliases to their canonical drush command name before blocklist evaluation.
func TestNormalizeDrushCommand(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"already canonical, unchanged", "sql-drop", "sql-drop"},
		{"trims and lowercases a legitimate command", "  PM:List  ", "pm:list"},
		{"sqlq alias resolves to sql:query", "sqlq", "sql:query"},
		{"sql-cli alias resolves to sql:query", "sql-cli", "sql:query"},
		{"sqlc alias resolves to sql:query", "sqlc", "sql:query"},
		{"scr alias resolves to php:script", "scr", "php:script"},
		{"ev alias resolves to php-eval", "ev", "php-eval"},
		{"exec alias resolves to core:execute-cli", "exec", "core:execute-cli"},
		{"core:execute alias resolves to core:execute-cli", "core:execute", "core:execute-cli"},
		{"alias resolution is case-insensitive and trims whitespace", "  SQLQ  ", "sql:query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeDrushCommand(tt.in); got != tt.want {
				t.Errorf("normalizeDrushCommand(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Task 3.1/3.4: drush_exec must block every documented alias identically to
// its canonical form, and block the two newly-extended canonical entries
// (sql:query, php:script) directly.
func TestDrushExec_BlocklistViaAlias(t *testing.T) {
	blocked := []string{
		"sql:query", "php:script", // newly-extended canonical forms
		"sqlq", "sql-cli", "sqlc", // sql:query aliases
		"scr",          // php:script alias
		"ev",           // php-eval alias
		"exec",         // core:execute-cli alias
		"core:execute", // core:execute-cli alias
	}
	for _, cmd := range blocked {
		t.Run(cmd, func(t *testing.T) {
			args := json.RawMessage(`{"project_path":"/tmp","command":"` + cmd + `"}`)
			result, err := realHandleDrushExec(args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var resp map[string]interface{}
			json.Unmarshal(result, &resp)
			if resp["success"] == true {
				t.Errorf("expected success=false for blocked command %q", cmd)
			}
			if stderr, ok := resp["stderr"].(string); ok {
				if !strings.Contains(stderr, "blocked for safety") {
					t.Errorf("stderr = %q, want it to mention 'blocked for safety'", stderr)
				}
			} else {
				t.Errorf("expected stderr field to be present for blocked command %q", cmd)
			}
		})
	}
}

// Task 3.1: legitimate, non-destructive drush commands must still pass the
// blocklist evaluation after alias normalization is introduced.
func TestDrushExec_LegitimateCommandsNotBlocked(t *testing.T) {
	allowed := []string{"cr", "updb", "pm:list"}
	for _, cmd := range allowed {
		t.Run(cmd, func(t *testing.T) {
			normalized := normalizeDrushCommand(cmd)
			if drushBlocklist[normalized] {
				t.Errorf("expected legitimate command %q (normalized %q) to NOT be blocked", cmd, normalized)
			}
		})
	}
}

// Task 3.2/3.4: the shell metacharacter filter must reject newline and
// backtick injection identically to semicolon/pipe/ampersand/dollar.
func TestShellMetacharPattern_RejectsNewlineAndBacktick(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		matches bool
	}{
		{"semicolon", "status; rm -rf /", true},
		{"pipe", "status | cat", true},
		{"ampersand", "status & echo", true},
		{"dollar", "status $(whoami)", true},
		{"backtick", "status `whoami`", true},
		{"newline", "status\nrm -rf /", true},
		{"safe command", "pm:list", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellMetacharPattern.MatchString(tt.in); got != tt.matches {
				t.Errorf("shellMetacharPattern.MatchString(%q) = %v, want %v", tt.in, got, tt.matches)
			}
		})
	}
}

// Task 3.2: drush_exec rejects a command argument containing a newline,
// matching the semicolon/pipe/ampersand/dollar/backtick behavior end-to-end.
func TestDrushExec_NewlineInArgRejected(t *testing.T) {
	args := json.RawMessage(`{"project_path":"/tmp","command":"status","args":["--foo\nrm -rf /"]}`)
	result, err := realHandleDrushExec(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] == true {
		t.Error("expected success=false for argument containing a newline")
	}
	stderr, _ := resp["stderr"].(string)
	if !strings.Contains(stderr, "shell metacharacters") {
		t.Errorf("stderr = %q, want it to mention shell metacharacters", stderr)
	}
}

// Task 3.3: drushExecError / realHandleDrushExec's JSON-parse-warning append
// must insert a newline separator so it never glues onto the prior stderr
// line, per the "Warning appended with separator" spec scenario.
func TestRealHandleDrushExec_StderrWarningSeparator(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	defer func() { drupexec.RunWithEnv = origRun }()

	t.Run("existing stderr content gets a newline before the warning", func(t *testing.T) {
		drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
			return "not valid json", "deprecation notice: something", 0, nil
		}
		args := json.RawMessage(`{"project_path":"/tmp","command":"status","format":"json"}`)
		result, err := realHandleDrushExec(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(result, &resp)
		stderr, _ := resp["stderr"].(string)
		want := "deprecation notice: something\nwarning: failed to parse JSON output"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})

	t.Run("empty stderr gets no leading blank line", func(t *testing.T) {
		drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
			return "not valid json", "", 0, nil
		}
		args := json.RawMessage(`{"project_path":"/tmp","command":"status","format":"json"}`)
		result, err := realHandleDrushExec(args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		var resp map[string]interface{}
		json.Unmarshal(result, &resp)
		stderr, _ := resp["stderr"].(string)
		want := "warning: failed to parse JSON output"
		if stderr != want {
			t.Errorf("stderr = %q, want %q", stderr, want)
		}
	})
}

func TestUpgradeScan_PathTraversal(t *testing.T) {
	args := json.RawMessage(`{"project_path":"/tmp/../../etc"}`)
	_, err := realHandleUpgradeScan(args)
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want it to mention '..'", err.Error())
	}
}

func TestPatchRollback_DirtyWorkingTree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	// Initialize git repo.
	exec.Command("git", "-C", dir, "init").Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	// Create initial commit.
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()

	// Create dirty working tree.
	os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("uncommitted"), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"patch_url":"https://example.com/patch.patch","composer_package":"drupal/token"}`)
	result, err := realHandlePatchRollback(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] == true {
		t.Error("expected success=false for dirty working tree")
	}
	if errMsg, ok := resp["error"].(string); ok {
		if !strings.Contains(errMsg, "dirty") {
			t.Errorf("error = %q, want it to mention 'dirty'", errMsg)
		}
	}
}

func TestPatchRollback_NonGitDir(t *testing.T) {
	dir := t.TempDir()
	// Not a git repo.
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"patch_url":"https://example.com/patch.patch","composer_package":"drupal/token"}`)
	result, err := realHandlePatchRollback(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] == true {
		t.Error("expected success=false for non-git directory")
	}
	if errMsg, ok := resp["error"].(string); ok {
		if !strings.Contains(errMsg, "not a git repository") {
			t.Errorf("error = %q, want it to mention 'not a git repository'", errMsg)
		}
	}
}

// --- Version matrix tests ---

func TestDrupalVersionMatrix_LookupByDrupalVersion(t *testing.T) {
	tests := []struct {
		version    string
		wantPHPMin string
		wantPHPRec string
	}{
		{"9", "7.3", "8.1"},
		{"10", "8.1", "8.3"},
		{"11", "8.3", "8.4"},
	}
	for _, tt := range tests {
		t.Run("D"+tt.version, func(t *testing.T) {
			args := json.RawMessage(`{"drupal_version":"` + tt.version + `"}`)
			result, err := realHandleDrupalVersionMatrix(args)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			var resp map[string]interface{}
			json.Unmarshal(result, &resp)
			phpReq := resp["php_requirements"].(map[string]interface{})
			if phpReq["minimum"] != tt.wantPHPMin {
				t.Errorf("PHP minimum = %v, want %v", phpReq["minimum"], tt.wantPHPMin)
			}
			if phpReq["recommended"] != tt.wantPHPRec {
				t.Errorf("PHP recommended = %v, want %v", phpReq["recommended"], tt.wantPHPRec)
			}
		})
	}
}

func TestDrupalVersionMatrix_LookupByPHPVersion(t *testing.T) {
	args := json.RawMessage(`{"php_version":"8.3"}`)
	result, err := realHandleDrupalVersionMatrix(args)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["drupal_version"] == nil || resp["drupal_version"] == "" {
		t.Error("expected drupal_version in response")
	}
}

func TestDrupalVersionMatrix_SelectsHighestNumericMajorForPHP84(t *testing.T) {
	tests := []struct {
		phpVersion    string
		wantDrupalVer string
	}{
		{phpVersion: "8.2", wantDrupalVer: "10"},
		{phpVersion: "8.4", wantDrupalVer: "11"},
	}
	for _, tt := range tests {
		t.Run("PHP "+tt.phpVersion, func(t *testing.T) {
			result, err := realHandleDrupalVersionMatrix(json.RawMessage(`{"php_version":"` + tt.phpVersion + `"}`))
			if err != nil {
				t.Fatalf("realHandleDrupalVersionMatrix error: %v", err)
			}
			var response struct {
				DrupalVersion string `json:"drupal_version"`
			}
			if err := json.Unmarshal(result, &response); err != nil {
				t.Fatal(err)
			}
			if response.DrupalVersion != tt.wantDrupalVer {
				t.Errorf("drupal_version = %q, want %q", response.DrupalVersion, tt.wantDrupalVer)
			}
		})
	}
}

func TestDrupalVersionMatrix_FullMatrix(t *testing.T) {
	args := json.RawMessage(`{}`)
	result, err := realHandleDrupalVersionMatrix(args)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	matrix, ok := resp["matrix"].([]interface{})
	if !ok {
		t.Fatal("expected matrix array in response")
	}
	if len(matrix) != 3 {
		t.Errorf("len(matrix) = %d, want 3", len(matrix))
	}
}

func TestDrupalVersionMatrix_UnknownVersion(t *testing.T) {
	args := json.RawMessage(`{"drupal_version":"99"}`)
	_, err := realHandleDrupalVersionMatrix(args)
	if err == nil || err.Error() != "unknown Drupal version: 99" {
		t.Errorf("error = %v, want unknown Drupal version: 99", err)
	}
}

func TestDetectEnv_InvalidJSON(t *testing.T) {
	_, err := realHandleDetectEnv(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestDetectEnv_EmptyProjectPath(t *testing.T) {
	_, err := realHandleDetectEnv(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for empty project_path, got nil")
	}
}

func TestDetectEnv_ValidPath(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ddev"), 0o755)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"prepare-backup-refusal"}`)
	result, err := realHandleDetectEnv(args)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["environment"] != "ddev" {
		t.Errorf("environment = %v, want ddev", resp["environment"])
	}
}

func TestModuleInfo_InvalidName(t *testing.T) {
	_, err := realHandleModuleInfo(json.RawMessage(`{"module_machine_name":"INVALID"}`))
	if err == nil {
		t.Error("expected error for invalid module name, got nil")
	}
}

func TestContribUpgradePath_InvalidName(t *testing.T) {
	_, err := realHandleContribUpgradePath(json.RawMessage(`{"module_machine_name":"123bad","current_drupal_version":"10","target_drupal_version":"11"}`))
	if err == nil {
		t.Error("expected error for invalid module name, got nil")
	}
}

func TestModuleReleaseInfo_InvalidName(t *testing.T) {
	_, err := realHandleModuleReleaseInfo(json.RawMessage(`{"module_machine_name":"123bad"}`))
	if err == nil {
		t.Error("expected error for invalid module name, got nil")
	}
}

func TestModuleReleaseInfo_InvalidCoreVersion(t *testing.T) {
	_, err := realHandleModuleReleaseInfo(json.RawMessage(`{"module_machine_name":"token","core_version":"abc"}`))
	if err == nil {
		t.Error("expected error for invalid core_version, got nil")
	}
}

func TestGenerateReport_InvalidJSON(t *testing.T) {
	_, err := realHandleGenerateReport(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestGenerateReport_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"report_type":"both"}`)
	result, err := realHandleGenerateReport(args)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] != true {
		t.Error("expected success=true")
	}

	// Check files were created.
	if _, err := os.Stat(filepath.Join(dir, "drup-report.json")); os.IsNotExist(err) {
		t.Error("drup-report.json was not created")
	}
	if _, err := os.Stat(filepath.Join(dir, "drup-report.md")); os.IsNotExist(err) {
		t.Error("drup-report.md was not created")
	}
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func preparedValidateProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"^4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// --- PR2: D2 bounded subprocess execution wired into MCP handlers ---

// TestResolveExecTimeout_UsesDescriptorOrDefault guards the descriptor-driven
// timeout lookup: catalogued tools carry their own limit, unknown tools fall
// back to defaultExecTimeout.
func TestResolveExecTimeout_UsesOverrideOrDefault(t *testing.T) {
	timeoutOf := func(name string) time.Duration {
		spec, ok := mcp.ToolSpecByName(name)
		if !ok {
			t.Fatalf("%s is missing from the tool catalog", name)
		}
		return spec.Timeout
	}
	tests := []struct {
		tool string
		want time.Duration
	}{
		{"composer_require", timeoutOf("composer_require")},
		{"core_upgrade_apply", timeoutOf("core_upgrade_apply")},
		{"upgrade_scan", timeoutOf("upgrade_scan")},
		{"drush_exec", timeoutOf("drush_exec")},
		{"some_unknown_tool", defaultExecTimeout},
	}
	for _, tt := range tests {
		if got := resolveExecTimeout(tt.tool); got != tt.want {
			t.Errorf("resolveExecTimeout(%q) = %v, want %v", tt.tool, got, tt.want)
		}
	}
}

// TestResolveExecTimeout_OverridesAreLongerThanDefault documents why the 3
// descriptors carry longer timeouts: their underlying commands legitimately
// run longer than a typical drush/composer call.
func TestResolveExecTimeout_OverridesAreLongerThanDefault(t *testing.T) {
	for _, tool := range []string{"composer_require", "core_upgrade_apply", "upgrade_scan"} {
		spec, ok := mcp.ToolSpecByName(tool)
		if !ok {
			t.Fatalf("%s is missing from the tool catalog", tool)
		}
		if spec.Timeout <= defaultExecTimeout {
			t.Errorf("ToolSpec(%q).Timeout = %v, want it longer than defaultExecTimeout %v", tool, spec.Timeout, defaultExecTimeout)
		}
	}
}

// TestComposerRequire_ExecTimeout_ReturnsErrorPromptly guards D2 end-to-end
// through the composer_require handler: a hanging composer call must not
// block the handler past its configured deadline.
func TestComposerRequire_ExecTimeout_ReturnsErrorPromptly(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origTimeout := resolveExecTimeout
	resolveExecTimeout = func(tool string) time.Duration {
		if tool == "composer_require" {
			return 50 * time.Millisecond
		}
		return origTimeout(tool)
	}
	defer func() { resolveExecTimeout = origTimeout }()

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
		time.Sleep(2 * time.Second) // far past the 50ms deadline
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{}`), 0o644)

	start := time.Now()
	_, err := realHandleComposerRequire(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"package":"drupal/token"}`))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error from realHandleComposerRequire, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("realHandleComposerRequire blocked for %v past its 50ms exec deadline, want it to return promptly", elapsed)
	}
}

// TestDrushExec_ExecTimeout_ReturnsErrorPromptly mirrors the above for
// drush_exec, which has no override and so relies on defaultExecTimeout.
func TestDrushExec_ExecTimeout_ReturnsErrorPromptly(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetectorDirect{}
	defer func() { defaultEnvDetector = origDetector }()

	origTimeout := resolveExecTimeout
	resolveExecTimeout = func(tool string) time.Duration {
		if tool == "drush_exec" {
			return 50 * time.Millisecond
		}
		return origTimeout(tool)
	}
	defer func() { resolveExecTimeout = origTimeout }()

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
		time.Sleep(2 * time.Second)
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	start := time.Now()
	_, err := realHandleDrushExec(json.RawMessage(`{"project_path":"/tmp","command":"status"}`))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error from realHandleDrushExec, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("realHandleDrushExec blocked for %v past its 50ms exec deadline, want it to return promptly", elapsed)
	}
}

// TestUpgradeScan_ExecTimeout_ReturnsErrorPromptly mirrors the above for
// upgrade_scan's drush calls.
func TestUpgradeScan_ExecTimeout_ReturnsErrorPromptly(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origTimeout := resolveExecTimeout
	resolveExecTimeout = func(tool string) time.Duration {
		if tool == "upgrade_scan" {
			return 50 * time.Millisecond
		}
		return origTimeout(tool)
	}
	defer func() { resolveExecTimeout = origTimeout }()

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			time.Sleep(2 * time.Second)
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"*"}}`), 0o644)

	start := time.Now()
	_, err := realHandleUpgradeScan(json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`))
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error from realHandleUpgradeScan, got nil")
	}
	if elapsed > 3*time.Second {
		t.Errorf("realHandleUpgradeScan blocked for %v past its 50ms exec deadline, want it to return promptly", elapsed)
	}
}

// --- core_upgrade_check ---

func TestCoreUpgradeCheck_InvalidJSON(t *testing.T) {
	_, err := realHandleCoreUpgradeCheck(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestCoreUpgradeCheck_MissingProjectPath(t *testing.T) {
	_, err := realHandleCoreUpgradeCheck(json.RawMessage(`{}`))
	if err == nil {
		t.Error("expected error for missing project_path, got nil")
	}
}

func TestCoreUpgradeCheck_PathTraversal(t *testing.T) {
	args := json.RawMessage(`{"project_path":"/tmp/../../etc"}`)
	_, err := realHandleCoreUpgradeCheck(args)
	if err == nil {
		t.Error("expected error for path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "..") {
		t.Errorf("error = %q, want it to mention '..'", err.Error())
	}
}

func TestCoreUpgradeCheck_RelativePathRejected(t *testing.T) {
	args := json.RawMessage(`{"project_path":"relative/path"}`)
	_, err := realHandleCoreUpgradeCheck(args)
	if err == nil {
		t.Error("expected error for relative path, got nil")
	}
}

func TestCoreUpgradeCheck_UnsupportedEnvironment(t *testing.T) {
	dir := t.TempDir() // no markers at all — envdetect reports EnvUnsupported

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"prepare-cap-refusal"}`)
	result, err := realHandleCoreUpgradeCheck(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["supported"] != false {
		t.Errorf("supported = %v, want false for a project with no recognized environment markers", resp["supported"])
	}
	if resp["next_version"] != "" {
		t.Errorf("next_version = %v, want empty when unsupported", resp["next_version"])
	}
}

// --- core_upgrade_apply ---

func TestCoreUpgradeApply_InvalidJSON(t *testing.T) {
	_, err := realHandleCoreUpgradeApply(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestCoreUpgradeApply_MissingProjectPath(t *testing.T) {
	_, err := realHandleCoreUpgradeApply(json.RawMessage(`{"target_version":"11.0.9"}`))
	if err == nil {
		t.Error("expected error for missing project_path, got nil")
	}
}

func TestCoreUpgradeApply_MissingTargetVersion(t *testing.T) {
	args := json.RawMessage(`{"project_path":` + jsonStr("/tmp") + `}`)
	_, err := realHandleCoreUpgradeApply(args)
	if err == nil {
		t.Error("expected error for missing target_version, got nil")
	}
}

func TestCoreUpgradeApply_TargetMajorRequiresItsOwnCatalog(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644)

	_, err := realHandleCoreUpgradeApply(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"target_major":12,"dry_run":true}`))
	if err == nil || !strings.Contains(err.Error(), "missing compatibility metadata for Drupal 11-to-12") {
		t.Errorf("error = %v, want missing 11-to-12 metadata", err)
	}
}

func TestCoreUpgradeApply_DryRunPreview(t *testing.T) {
	requireGitForApp(t)
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^10.1"}}`), 0o644)
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")

	args := json.RawMessage(fmt.Sprintf(`{"project_path":%s,"target_version":"11.0.9","dry_run":true}`, jsonStr(dir)))
	result, err := realHandleCoreUpgradeApply(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] != true {
		t.Errorf("success = %v, want true", resp["success"])
	}
	if resp["rollback_checkpoint"] != "" {
		t.Errorf("rollback_checkpoint = %v, want empty for dry-run", resp["rollback_checkpoint"])
	}
	report, _ := resp["report"].(string)
	if !strings.Contains(report, "drupal/core-recommended") {
		t.Errorf("report = %q, want it to mention drupal/core-recommended", report)
	}
}

// TestCoreUpgradeApply_AllowDirtyArgumentIgnored guards G4/G5/S2: allow_dirty
// was removed from the MCP surface (proposal decision 3), so an
// undocumented allow_dirty argument in the raw JSON call must never bypass
// the dirty-tree check — only the separate CLI `drup upgrade-core
// --allow-dirty` command keeps that behavior.
func TestCoreUpgradeApply_AllowDirtyArgumentIgnored(t *testing.T) {
	requireGitForApp(t)
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^10.1"}}`), 0o644)
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Dirty the tree after the commit so Apply's dirty-tree check has
	// something to refuse.
	os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x"), 0o644)

	args := json.RawMessage(fmt.Sprintf(`{"project_path":%s,"target_version":"11.0.9","dry_run":false,"allow_dirty":true}`, jsonStr(dir)))
	result, err := realHandleCoreUpgradeApply(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["success"] != false {
		t.Errorf("success = %v, want false — an MCP-supplied allow_dirty must be ignored", resp["success"])
	}
	report, _ := resp["report"].(string)
	if !strings.Contains(report, "uncommitted changes") {
		t.Errorf("report = %q, want it to mention uncommitted changes (allow_dirty must not bypass the MCP dirty-tree check)", report)
	}
}

func requireGitForApp(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// --- patch_reconcile ---

func TestPatchReconcile_InvalidJSON(t *testing.T) {
	_, err := realHandlePatchReconcile(json.RawMessage(`{invalid`))
	if err == nil {
		t.Error("expected error for invalid JSON, got nil")
	}
}

func TestPatchReconcile_InvalidModuleName(t *testing.T) {
	args := json.RawMessage(`{"module_machine_name":"INVALID","current_patch_url":"https://www.drupal.org/node/1"}`)
	_, err := realHandlePatchReconcile(args)
	if err == nil {
		t.Error("expected error for invalid module machine name, got nil")
	}
}

func TestPatchReconcile_MissingPatchURL(t *testing.T) {
	args := json.RawMessage(`{"module_machine_name":"token"}`)
	_, err := realHandlePatchReconcile(args)
	if err == nil {
		t.Error("expected error for missing current_patch_url, got nil")
	}
}

func TestPatchReconcile_ReturnsResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"list":[{"node":{"nid":"555","title":"Fixed","status":"Fixed"}}],"next":""}`))
	}))
	defer srv.Close()

	origClient := drupalorg.HTTPClient
	drupalorg.HTTPClient = srv.Client()
	defer func() { drupalorg.HTTPClient = origClient }()

	origBase := drupalorg.APID7BaseURL
	drupalorg.APID7BaseURL = srv.URL + "/api-d7/node.json?field_project_machine_name=%s"
	defer func() { drupalorg.APID7BaseURL = origBase }()

	args := json.RawMessage(`{"module_machine_name":"token","current_patch_url":"https://www.drupal.org/node/555"}`)
	result, err := realHandlePatchReconcile(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["is_still_needed"] != false {
		t.Errorf("is_still_needed = %v, want false for a merged/Fixed issue", resp["is_still_needed"])
	}
	recommendation, _ := resp["recommendation"].(string)
	if !strings.Contains(recommendation, "remove") {
		t.Errorf("recommendation = %q, want it to mention remove", recommendation)
	}
}

// --- Phase 1: RED tests for --all flag in MCP tools ---

func TestRealHandleScan_PassesAllFlag(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"^4.0"}}`), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			if len(args) > 0 && args[0] == "pm:list" {
				return `{"upgrade_status":{}}`, "", 0, nil
			}
			capturedArgs = args
			return "", "", 0, nil // empty plain text = zero errors
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(fmt.Sprintf(`{"project_path":%q}`, dir))
	_, err := realHandleScan(args)
	if err != nil {
		t.Fatalf("realHandleScan error: %v", err)
	}

	found := false
	for _, arg := range capturedArgs {
		if arg == "--all" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("drush args = %v, want --all flag present", capturedArgs)
	}
}

func TestRealHandleAutofix_RemainingErrors(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.Run
	origRunWithEnv := drupexec.RunWithEnv
	callCount := 0
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		// rector
		return "rector summary", "", 0, nil
	}
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			callCount++
			// Re-scan returns checkstyle XML with 2 remaining errors.
			return `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymod/a.module"><error line="1" message="Error one." severity="error"/></file><file name="modules/custom/mymod/b.module"><error line="2" message="Error two." severity="error"/></file></checkstyle>`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() {
		drupexec.Run = origRun
		drupexec.RunWithEnv = origRunWithEnv
	}()

	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "modules", "custom"), 0o755)
	os.MkdirAll(filepath.Join(dir, "themes"), 0o755)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	result, err := realHandleAutofix(args)
	if err != nil {
		t.Fatalf("realHandleAutofix error: %v", err)
	}

	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if _, ok := resp["remaining_errors"]; ok {
		t.Errorf("autofix must not return rescan results: %v", resp)
	}
}

func TestRealHandleScan_PlainText(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"^4.0"}}`), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			return `<?xml version="1.0"?><checkstyle><file name="modules/contrib/token/token.module"><error line="42" message="Call to deprecated function foo()." severity="error"/></file></checkstyle>`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(fmt.Sprintf(`{"project_path":%q}`, dir))
	result, err := realHandleScan(args)
	if err != nil {
		t.Fatalf("realHandleScan error: %v", err)
	}

	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", resp["total_errors"])
	}
	modules := resp["modules"].([]interface{})
	if len(modules) != 1 {
		t.Errorf("modules = %d, want 1", len(modules))
	}
}

func TestRealHandleScan_NoFormatJSON(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			capturedArgs = args
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(`{"project_path":"/tmp/test-project"}`)
	realHandleScan(args)

	for _, arg := range capturedArgs {
		if arg == "--format=json" {
			t.Errorf("drush args = %v, must NOT contain --format=json", capturedArgs)
		}
	}
}

func TestRealHandleAutofix_PassesAllFlagInRescan(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.Run
	origRunWithEnv := drupexec.RunWithEnv
	var capturedDrushArgs [][]string
	drupexec.Run = func(cmd string, args ...string) (string, string, int, error) {
		// rector
		return "rector output", "", 0, nil
	}
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			capturedDrushArgs = append(capturedDrushArgs, args)
			return "", "", 0, nil // empty plain text = zero remaining errors
		}
		return "", "", 0, nil
	}
	defer func() {
		drupexec.Run = origRun
		drupexec.RunWithEnv = origRunWithEnv
	}()

	// Create temp dir with modules/custom and themes dirs.
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "modules", "custom"), 0o755)
	os.MkdirAll(filepath.Join(dir, "themes"), 0o755)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := realHandleAutofix(args)
	if err != nil {
		t.Fatalf("realHandleAutofix error: %v", err)
	}

	if len(capturedDrushArgs) != 0 {
		t.Errorf("autofix must not rescan: %v", capturedDrushArgs)
	}
}

func TestRealHandleValidate_PassesAllFlagWhenNoModule(t *testing.T) {
	dir := preparedValidateProject(t)
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			capturedArgs = args
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := realHandleValidate(args)
	if err != nil {
		t.Fatalf("realHandleValidate error: %v", err)
	}

	found := false
	for _, arg := range capturedArgs {
		if arg == "--all" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("drush args = %v, want --all flag when no module specified", capturedArgs)
	}
}

func TestRealHandleValidate_AcceptsModuleNameWhenSet(t *testing.T) {
	dir := preparedValidateProject(t)
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var capturedArgs []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			capturedArgs = args
			return "", "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"module_name":"mymodule"}`)
	_, err := realHandleValidate(args)
	if err != nil {
		t.Fatalf("realHandleValidate error: %v", err)
	}

	// Verify module name is in args, not --all.
	foundModule := false
	foundAll := false
	for _, arg := range capturedArgs {
		if arg == "mymodule" {
			foundModule = true
		}
		if arg == "--all" {
			foundAll = true
		}
	}
	if !foundModule {
		t.Errorf("drush args = %v, want 'mymodule' present", capturedArgs)
	}
	if foundAll {
		t.Errorf("drush args = %v, want --all NOT present when module is specified", capturedArgs)
	}
}

func TestRealHandleValidate_PlainText(t *testing.T) {
	dir := preparedValidateProject(t)
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			return `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymod/mymod.module"><error line="5" message="Deprecated function foo()." severity="error"/></file></checkstyle>`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"module":"mymod"}`)
	result, err := realHandleValidate(args)
	if err != nil {
		t.Fatalf("realHandleValidate error: %v", err)
	}

	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", resp["total_errors"])
	}
}

// --- Phase 7: G9 — validate's expected_hash gate ---

const validateChecklistXML = `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymod/mymod.module"><error line="5" message="Deprecated function foo()." severity="error"/></file></checkstyle>`

func realHandleValidateWithMockedDrush(t *testing.T, xml string) (json.RawMessage, error) {
	t.Helper()
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	t.Cleanup(func() { defaultEnvDetector = origDetector })

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			return xml, "", 0, nil
		}
		return "", "", 0, nil
	}
	t.Cleanup(func() { drupexec.RunWithEnv = origRun })

	args := json.RawMessage(`{"project_path":` + jsonStr(preparedValidateProject(t)) + `,"module":"mymod"}`)
	return realHandleValidate(args)
}

func TestRealHandleValidate_ExpectedHashOmitted_PreservesTotalErrorsGating(t *testing.T) {
	result, err := realHandleValidateWithMockedDrush(t, validateChecklistXML)
	if err != nil {
		t.Fatalf("realHandleValidate error: %v", err)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", resp["total_errors"])
	}
	hash, ok := resp["evidence_hash"].(string)
	if !ok || hash == "" {
		t.Errorf("evidence_hash = %v, want a non-empty string", resp["evidence_hash"])
	}
}

func TestRealHandleValidate_MatchingExpectedHash_ProceedsWithNormalGating(t *testing.T) {
	// First call to learn the current evidence_hash.
	first, err := realHandleValidateWithMockedDrush(t, validateChecklistXML)
	if err != nil {
		t.Fatalf("realHandleValidate error: %v", err)
	}
	var firstResp map[string]interface{}
	json.Unmarshal(first, &firstResp)
	hash := firstResp["evidence_hash"].(string)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()
	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			return validateChecklistXML, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(fmt.Sprintf(`{"project_path":%s,"module":"mymod","expected_hash":%q}`, jsonStr(preparedValidateProject(t)), hash))
	result, err := realHandleValidate(args)
	if err != nil {
		t.Fatalf("realHandleValidate error with matching expected_hash: %v", err)
	}

	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", resp["total_errors"])
	}
}

func TestRealHandleValidate_MismatchedExpectedHash_FailsClosedRegardlessOfTotalErrors(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status":{}}`, "", 0, nil
		}
		if cmd == "drush" {
			// Zero findings — even so, a stale expected_hash must fail
			// closed rather than reporting total_errors == 0 as trustworthy.
			return `<?xml version="1.0"?><checkstyle></checkstyle>`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	staleHash := "0000000000000000000000000000000000000000000000000000000000stale"
	args := json.RawMessage(fmt.Sprintf(`{"project_path":%s,"module":"mymod","expected_hash":%q}`, jsonStr(preparedValidateProject(t)), staleHash))
	_, err := realHandleValidate(args)
	if err == nil {
		t.Fatal("expected error for mismatched expected_hash, got nil")
	}
	if !strings.Contains(err.Error(), staleHash) {
		t.Errorf("error = %q, want it to include the expected hash %q", err.Error(), staleHash)
	}
}

// --- Phase 5: RED test for config conflict in realHandleUpgradeScan ---

func TestRealHandleUpgradeScan_RequiresPreparationBeforeConfigMutation(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"*"}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := realHandleUpgradeScan(args)
	if err == nil || !strings.Contains(err.Error(), "prepare_upgrade_status") {
		t.Fatalf("unprepared upgrade_scan error = %v", err)
	}
}

func TestRealHandleUpgradeScan_SkipsEnableWhenAlreadyEnabled(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRunWithEnv := drupexec.RunWithEnv
	var drushCalls [][]string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd != "drush" {
			return "", "", 0, nil
		}
		drushCalls = append(drushCalls, args)
		// Return upgrade_status as already enabled for pm:list.
		for _, arg := range args {
			if arg == "pm:list" {
				return `{"upgrade_status":"11.0.0"}`, "", 0, nil
			}
		}
		// Return empty plain text for upgrade_status:analyze.
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"*"}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := realHandleUpgradeScan(args)
	if err != nil {
		t.Fatalf("realHandleUpgradeScan error: %v", err)
	}

	// Verify config:delete and en were NOT called (already enabled).
	for _, drushArgs := range drushCalls {
		for _, arg := range drushArgs {
			if arg == "config:delete" {
				t.Error("drush config:delete should NOT be called when upgrade_status is already enabled")
			}
			if arg == "en" {
				t.Error("drush en should NOT be called when upgrade_status is already enabled")
			}
		}
	}

	// Verify upgrade_status:analyze WAS called.
	foundAnalyze := false
	for _, drushArgs := range drushCalls {
		for _, arg := range drushArgs {
			if arg == "upgrade_status:analyze" {
				foundAnalyze = true
			}
		}
	}
	if !foundAnalyze {
		t.Error("upgrade_status:analyze should be called when upgrade_status is already enabled")
	}
}

func TestRealHandleUpgradeScan_PlainText(t *testing.T) {
	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRunWithEnv := drupexec.RunWithEnv
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd != "drush" {
			return "", "", 0, nil
		}
		for _, arg := range args {
			if arg == "pm:list" {
				return `{"upgrade_status":"11.0.0"}`, "", 0, nil
			}
			if arg == "upgrade_status:analyze" {
				return `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymod/mymod.module"><error line="5" message="Deprecated function foo()." severity="error"/></file></checkstyle>`, "", 0, nil
			}
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRunWithEnv }()

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"*"}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	result, err := realHandleUpgradeScan(args)
	if err != nil {
		t.Fatalf("realHandleUpgradeScan error: %v", err)
	}

	var resp map[string]interface{}
	json.Unmarshal(result, &resp)
	if resp["total_errors"].(float64) != 1 {
		t.Errorf("total_errors = %v, want 1", resp["total_errors"])
	}
	modules := resp["modules"].([]interface{})
	if len(modules) != 1 {
		t.Errorf("modules = %d, want 1", len(modules))
	}
}

// mockEnvDetector returns empty environment for testing.
type mockEnvDetector struct{}

func (m *mockEnvDetector) Detect(projectPath string, forceDetect bool) (*envdetect.Detection, error) {
	return &envdetect.Detection{
		Environment:   "",
		CommandPrefix: []string{},
		DetectedAt:    time.Now(),
	}, nil
}

// Task 2.5: create_patch uses project_path + web root from composer.json.
func TestRealHandleCreatePatch_UsesProjectPath(t *testing.T) {
	dir := t.TempDir()

	// Create composer.json with custom web-root.
	composerJSON := `{
		"extra": {
			"drupal-scaffold": {
				"locations": {
					"web-root": "docroot"
				}
			}
		}
	}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	// Create the module directory at docroot/modules/contrib/testmod.
	modulePath := filepath.Join(dir, "docroot", "modules", "contrib", "testmod")
	os.MkdirAll(modulePath, 0o755)
	os.WriteFile(filepath.Join(modulePath, "testmod.info.yml"), []byte("name: testmod"), 0o644)

	// Initialize git repo.
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")

	// Override exec to capture rector and git calls. Rector goes through the
	// environment prefix, git stays on the host.
	origRun := drupexec.Run
	origRunWithEnv := drupexec.RunWithEnv
	var capturedCmds []string
	capture := func(cmd string, args ...string) (string, string, int, error) {
		capturedCmds = append(capturedCmds, cmd+" "+strings.Join(args, " "))
		// Return empty diff so it reports "not applied".
		return "", "", 0, nil
	}
	drupexec.Run = capture
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		return capture(cmd, args...)
	}
	defer func() {
		drupexec.Run = origRun
		drupexec.RunWithEnv = origRunWithEnv
	}()

	args := json.RawMessage(fmt.Sprintf(`{"module_name":"testmod","deprecation_details":"test","project_path":%q}`, dir))
	_, err := realHandleCreatePatch(args)
	if err != nil {
		t.Fatalf("realHandleCreatePatch error: %v", err)
	}

	// Verify rector was called with the docroot-based path.
	rectorFound := false
	for _, cmd := range capturedCmds {
		if strings.Contains(cmd, "rector") && strings.Contains(cmd, filepath.Join(dir, "docroot", "modules", "contrib", "testmod")) {
			rectorFound = true
			break
		}
	}
	if !rectorFound {
		t.Errorf("rector should be called with docroot-based path, got commands: %v", capturedCmds)
	}
}

// Contrib lives in a gitignored directory, so the patch has to come from a
// tree comparison rather than the repository index.
func TestNormalizePatchPaths_MakesPatchApplyFromPackageRoot(t *testing.T) {
	pristine := "/tmp/drup-pristine-123/active_filters"
	module := "/srv/site/web/modules/contrib/active_filters"
	diff := "diff --git a" + pristine + "/src/Foo.php b" + module + "/src/Foo.php\n" +
		"--- a" + pristine + "/src/Foo.php\n" +
		"+++ b" + module + "/src/Foo.php\n" +
		"@@ -1 +1 @@\n-old\n+new\n"

	got := normalizePatchPaths(diff, pristine, module)

	if strings.Contains(got, pristine) || strings.Contains(got, module) {
		t.Errorf("absolute paths survived normalization:\n%s", got)
	}
	for _, want := range []string{"--- a/src/Foo.php", "+++ b/src/Foo.php"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestCopyTree_CopiesNestedFiles(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "src", "Plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "src", "Plugin", "Block.php"), []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "copy")
	if err := copyTree(src, dst); err != nil {
		t.Fatalf("copyTree error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "src", "Plugin", "Block.php")); err != nil {
		t.Errorf("nested file not copied: %v", err)
	}
}

// --- REQ-1: ensureUpgradeStatusEnabled tests ---

func TestEnsureUpgradeStatusEnabled_AlreadyEnabled(t *testing.T) {
	dir := t.TempDir()
	// composer.json with upgrade_status installed.
	composerJSON := `{"require": {"drupal/upgrade_status": "^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	drushEnCalled := false
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		// pm:list returns upgrade_status as enabled.
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{"upgrade_status": {"name": "upgrade_status"}}`, "", 0, nil
		}
		if cmd == "drush" && len(args) > 0 && args[0] == "en" {
			drushEnCalled = true
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := ensureUpgradeStatusEnabled(dir, defaultExecTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if drushEnCalled {
		t.Error("drush en should NOT be called when upgrade_status is already enabled")
	}
}

func TestEnsureUpgradeStatusEnabled_AutoEnables(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{"require": {"drupal/upgrade_status": "^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var calls []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 {
			calls = append(calls, args[0])
		}
		// pm:list returns empty (not enabled).
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{}`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	err := ensureUpgradeStatusEnabled(dir, defaultExecTimeout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have called: pm:list, config:delete, en, cr.
	expectedCalls := []string{"pm:list", "config:delete", "en", "cr"}
	if len(calls) < len(expectedCalls) {
		t.Fatalf("expected at least %d drush calls, got %d: %v", len(expectedCalls), len(calls), calls)
	}
	for i, expected := range expectedCalls {
		if calls[i] != expected {
			t.Errorf("call[%d] = %q, want %q; all calls: %v", i, calls[i], expected, calls)
		}
	}
}

func TestEnsureUpgradeStatusEnabled_NotInstalled(t *testing.T) {
	dir := t.TempDir()
	// composer.json WITHOUT upgrade_status.
	composerJSON := `{"require": {"drupal/core": "^10.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	err := ensureUpgradeStatusEnabled(dir, defaultExecTimeout)
	if err == nil {
		t.Fatal("expected error when upgrade_status is not in composer.json, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention 'not installed', got: %v", err)
	}
}

func TestRealHandleScan_RequiresPreparationBeforeMutation(t *testing.T) {
	dir := t.TempDir()
	composerJSON := `{"require": {"drupal/upgrade_status": "^4.0"}}`
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(composerJSON), 0o644)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	origRun := drupexec.RunWithEnv
	var calls []string
	drupexec.RunWithEnv = func(_ string, prefix []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" && len(args) > 0 {
			calls = append(calls, args[0])
		}
		// pm:list returns empty (not enabled).
		if cmd == "drush" && len(args) > 0 && args[0] == "pm:list" {
			return `{}`, "", 0, nil
		}
		// upgrade_status:analyze returns empty checkstyle.
		if cmd == "drush" && len(args) > 0 && args[0] == "upgrade_status:analyze" {
			return `<?xml version="1.0" encoding="UTF-8"?><checkstyle></checkstyle>`, "", 0, nil
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnv = origRun }()

	args := json.RawMessage(fmt.Sprintf(`{"project_path": %q}`, dir))
	_, err := realHandleScan(args)
	if err == nil || !strings.Contains(err.Error(), "prepare_upgrade_status") {
		t.Fatalf("unprepared scan error = %v", err)
	}
	if strings.Join(calls, ",") != "pm:list" {
		t.Errorf("unprepared scan calls = %v, want only pm:list", calls)
	}
}

// --- session_open ---

func resetSessionForTest(t *testing.T) {
	t.Helper()
	session.Reset()
	t.Cleanup(session.Reset)
}

func newDrupalProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRealHandleSessionOpen_BindsCanonicalRootOnSuccess(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	result, err := realHandleSessionOpen(args)
	if err != nil {
		t.Fatalf("realHandleSessionOpen error: %v", err)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("invalid result JSON: %v", err)
	}
	if resp["session_active"] != true {
		t.Errorf("session_active = %v, want true", resp["session_active"])
	}
	if resp["root"] == "" {
		t.Error("root must not be empty on a successful session_open")
	}

	sess, ok := session.Current()
	if !ok {
		t.Fatal("expected an active session after realHandleSessionOpen")
	}
	if sess.Root != resp["root"] {
		t.Errorf("bound session root = %q, response root = %v", sess.Root, resp["root"])
	}
}

func TestRealHandleSessionOpen_RejectsNonDrupalDirectory(t *testing.T) {
	resetSessionForTest(t)
	dir := t.TempDir() // no composer.json, no web root markers

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	if _, err := realHandleSessionOpen(args); err == nil {
		t.Fatal("expected an error opening a session against a non-Drupal directory")
	}
	if _, ok := session.Current(); ok {
		t.Error("no session should be bound after a rejected session_open")
	}
}

func TestRealHandleSessionOpen_MissingProjectPath(t *testing.T) {
	resetSessionForTest(t)
	if _, err := realHandleSessionOpen(json.RawMessage(`{}`)); err == nil {
		t.Error("expected an error for a missing project_path")
	}
}

// --- Guard middleware wiring (specs/agent-session Guard Middleware Enforcement) ---

func TestWireMCPTools_RefuseOnlyToolsRefuseWithoutSession(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := t.TempDir()
	for name := range session.RefuseOnlyTools {
		t.Run(name, func(t *testing.T) {
			args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"force-dry-run-` + name + `"}`)
			_, err := server.CallTool(name, args)
			if err == nil {
				t.Fatalf("%s: expected refusal without a bound session", name)
			}
			if !strings.Contains(err.Error(), "session_open") {
				t.Errorf("%s: error %q does not name the session_open flow", name, err.Error())
			}
		})
	}
}

func TestWireMCPTools_PrepareUpgradeStatusRefusesWithoutSession(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := newDrupalProjectDir(t)
	_, err := server.CallTool("prepare_upgrade_status", json.RawMessage(`{"project_path":`+jsonStr(dir)+`}`))
	if err == nil {
		t.Fatal("prepare_upgrade_status must refuse without a bound session")
	}
	if !strings.Contains(err.Error(), "session_open") {
		t.Errorf("prepare_upgrade_status error = %q, want session_open guidance", err)
	}
}

func TestWireMCPTools_PrepareUpgradeStatusHonorsKillSwitch(t *testing.T) {
	resetSessionForTest(t)
	t.Setenv("DRUP_DISABLE_MUTATIONS", "1")

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := newDrupalProjectDir(t)
	_, err := server.CallTool("prepare_upgrade_status", json.RawMessage(`{"project_path":`+jsonStr(dir)+`}`))
	if err == nil {
		t.Fatal("prepare_upgrade_status must refuse while mutations are disabled")
	}
	if !strings.Contains(err.Error(), "DRUP_DISABLE_MUTATIONS") {
		t.Errorf("prepare_upgrade_status error = %q, want kill-switch guidance", err)
	}
}

func TestWireMCPTools_PrepareUpgradeStatusRequiresBackupAndAuditsRefusal(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := newDrupalProjectDir(t)
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}
	runID := createRunAtPhaseForTool(t, dir, runstate.PhaseTooling)
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"run_id":` + jsonStr(runID) + `,"request_id":"prepare-backup-refusal"}`)
	_, err := server.CallTool("prepare_upgrade_status", args)
	if err == nil || !strings.Contains(err.Error(), "test_backup_create") {
		t.Fatalf("prepare_upgrade_status error = %v, want backup guidance", err)
	}

	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatalf("audit.ReadAll error: %v", err)
	}
	if len(records) != 1 || records[0].Tool != "prepare_upgrade_status" || records[0].Result != audit.ResultFailure {
		t.Errorf("audit records = %+v, want one prepare_upgrade_status failure", records)
	}
}

func TestWireMCPTools_PrepareUpgradeStatusRefusesAtMutationCap(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".drup", "config.json"), []byte(`{"mutation_cap_per_session":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}
	runID := createRunAtPhaseForTool(t, dir, runstate.PhaseTooling)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"run_id":` + jsonStr(runID) + `,"request_id":"prepare-cap-refusal"}`)
	audit.Append(dir, "apply_patch", args, audit.ResultSuccess, "")
	_, err := server.CallTool("prepare_upgrade_status", args)
	if err == nil || !strings.Contains(err.Error(), "mutation cap reached (1/1)") {
		t.Fatalf("prepare_upgrade_status error = %v, want mutation-cap refusal", err)
	}

	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatalf("audit.ReadAll error: %v", err)
	}
	if len(records) != 2 || records[1].Tool != "prepare_upgrade_status" || records[1].Result != audit.ResultFailure {
		t.Errorf("audit records = %+v, want cap refusal recorded for prepare_upgrade_status", records)
	}
}

func TestWireMCPTools_ForceDryRunToolsNeverRefusedWithoutSession(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := t.TempDir()
	for name := range session.ForceDryRunTools {
		t.Run(name, func(t *testing.T) {
			// Minimal args (project_path only). Whatever the real handler
			// does with incomplete input, the guard itself must never
			// produce the refuse-only session_open error for a force-dry-run
			// tool — it forces dry_run and lets the call through instead.
			args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
			_, err := server.CallTool(name, args)
			if err != nil && strings.Contains(err.Error(), "session_open") {
				t.Errorf("%s: guard refused instead of forcing dry-run: %v", name, err)
			}
		})
	}
}

func TestWireMCPTools_CoreUpgradeApply_ForcesDryRunWithoutSession(t *testing.T) {
	resetSessionForTest(t)
	requireGitForApp(t)

	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@test.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^10.1"}}`), 0o644)
	runGitCmd(t, dir, "add", ".")
	runGitCmd(t, dir, "commit", "-m", "initial")

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	// No persisted run exists, so the run guard must refuse before the legacy
	// no-session force-dry-run fallback can enter the handler.
	args := json.RawMessage(fmt.Sprintf(`{"project_path":%s,"target_version":"11.0.9","request_id":"core-upgrade-dry-run"}`, jsonStr(dir)))
	result, err := server.CallTool("core_upgrade_apply", args)
	if err == nil || !strings.Contains(err.Error(), "run_id") {
		t.Fatalf("core_upgrade_apply result = %s, error = %v; want run_id refusal", result, err)
	}
}

func createRunAtPhaseForTool(t *testing.T, root string, phase runstate.Phase) string {
	t.Helper()
	store := runstate.NewStore(root)
	run, err := store.Create(runstate.CreateInput{ID: "run-tool-" + string(phase), TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != phase {
		run, err = store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "test", Summary: "checkpoint"})
		if err != nil {
			t.Fatal(err)
		}
	}
	return run.ID
}

func TestWireMCPTools_MatchingSessionAllowsGuardedToolThrough(t *testing.T) {
	resetSessionForTest(t)

	var buf bytes.Buffer
	server := mcp.NewServer(&buf, "test")
	WireMCPTools(server)

	dir := newDrupalProjectDir(t)
	// A matching session alone is not enough since PR6 — the guard also
	// requires a fresh backup manifest before reaching the real handler.
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}

	// composer_require with project_path only (missing "package") must fail
	// on the handler's own validation, never on the guard.
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"composer-require-validation"}`)
	_, err := server.CallTool("composer_require", args)
	if err == nil {
		t.Fatal("expected an error from composer_require's own validation (missing package)")
	}
	if strings.Contains(err.Error(), "session_open") || strings.Contains(err.Error(), "test_backup_create") {
		t.Errorf("guard refused despite a matching bound session and fresh backup: %v", err)
	}
}

func TestWireMCPTools_CallToolRequiresRequestIDBeforeGenerateReportEffect(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&bytes.Buffer{}, "test")
	WireMCPTools(server)
	_, err := server.CallTool("generate_report", json.RawMessage(`{"project_path":`+jsonStr(dir)+`,"report_type":"json"}`))
	if err == nil || !strings.Contains(err.Error(), "request_id is required") {
		t.Fatalf("CallTool error = %v, want missing request_id refusal", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "drup-report.json")); !os.IsNotExist(err) {
		t.Fatalf("generate_report effect ran without request_id, stat error = %v", err)
	}
}

func TestWireMCPTools_UpgradeScanItselfIsNotRegistrationGuarded(t *testing.T) {
	// upgrade_scan is intentionally excluded from the registration-time
	// guard set (session.GuardedTools()); it is guarded only at its nested
	// composer-install path. Registering it directly must not wrap it.
	if session.GuardedTools()["upgrade_scan"] {
		t.Fatal("upgrade_scan must not be part of the registration-time guarded set")
	}
}

// --- upgrade_scan nested install path guard (session.RequireInstallAllowed) ---

func TestRealHandleUpgradeScan_NestedInstallPathGuardedWithoutSession(t *testing.T) {
	resetSessionForTest(t)

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := realHandleUpgradeScan(args)
	if err == nil {
		t.Fatal("expected the nested composer-install path to be refused without a bound session")
	}
	if !strings.Contains(err.Error(), "prepare_upgrade_status") {
		t.Errorf("error = %q, want preparation guidance", err.Error())
	}
}

// --- pipeline_status ---

func TestRealHandlePipelineStatus_WithPriorMutations(t *testing.T) {
	resetSessionForTest(t)
	dir := t.TempDir()
	audit.Append(dir, "apply_patch", []byte(`{}`), audit.ResultSuccess, "abc123")
	audit.Append(dir, "apply_patch", []byte(`{}`), audit.ResultSuccess, "def456")
	audit.Append(dir, "composer_require", []byte(`{}`), audit.ResultFailure, "")

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	result, err := realHandlePipelineStatus(args)
	if err != nil {
		t.Fatalf("realHandlePipelineStatus error: %v", err)
	}

	var resp struct {
		PerToolCounts  map[string]int `json:"per_tool_counts"`
		TotalMutations int            `json:"total_mutations"`
		RemainingCap   int            `json:"remaining_cap"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if resp.PerToolCounts["apply_patch"] != 2 {
		t.Errorf("per_tool_counts[apply_patch] = %d, want 2", resp.PerToolCounts["apply_patch"])
	}
	if resp.PerToolCounts["composer_require"] != 1 {
		t.Errorf("per_tool_counts[composer_require] = %d, want 1", resp.PerToolCounts["composer_require"])
	}
	if resp.TotalMutations != 3 {
		t.Errorf("total_mutations = %d, want 3", resp.TotalMutations)
	}
	// No session bound in this test, so the per-day default cap (200) applies
	// and all 3 recorded mutations happened within the last 24h.
	if resp.RemainingCap != 197 {
		t.Errorf("remaining_cap = %d, want 197 (default per-day cap 200 - 3 recorded)", resp.RemainingCap)
	}
}

func TestRealHandlePipelineStatus_EmptyLedgerReturnsZeroCountsFullCapNoError(t *testing.T) {
	resetSessionForTest(t)
	dir := t.TempDir()

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	result, err := realHandlePipelineStatus(args)
	if err != nil {
		t.Fatalf("realHandlePipelineStatus error on empty ledger: %v", err)
	}

	var resp struct {
		PerToolCounts  map[string]int `json:"per_tool_counts"`
		TotalMutations int            `json:"total_mutations"`
		RemainingCap   int            `json:"remaining_cap"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		t.Fatalf("invalid response JSON: %v", err)
	}
	if len(resp.PerToolCounts) != 0 {
		t.Errorf("per_tool_counts = %v, want empty for an unused ledger", resp.PerToolCounts)
	}
	if resp.TotalMutations != 0 {
		t.Errorf("total_mutations = %d, want 0", resp.TotalMutations)
	}
	if resp.RemainingCap != 200 {
		t.Errorf("remaining_cap = %d, want 200 (the full built-in default per-day cap)", resp.RemainingCap)
	}
}

func TestRealHandleUpgradeScan_NestedInstallPathAllowedWithMatchingSession(t *testing.T) {
	resetSessionForTest(t)

	origDetector := defaultEnvDetector
	defaultEnvDetector = &mockEnvDetector{}
	defer func() { defaultEnvDetector = origDetector }()

	dir := t.TempDir()

	origRunWithEnvCtx := drupexec.RunWithEnvCtx
	drupexec.RunWithEnvCtx = func(_ context.Context, projectPath string, _ []string, cmd string, args ...string) (string, string, int, error) {
		// A real `composer require drupal/upgrade_status` would add the
		// package to composer.json; simulate that so the later
		// ensureUpgradeStatusEnabled check (which re-reads composer.json)
		// sees the package as installed, exactly like it would in production.
		if cmd == "composer" && len(args) > 1 && args[0] == "require" && args[1] == "drupal/upgrade_status" {
			os.WriteFile(filepath.Join(projectPath, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"*"}}`), 0o644)
		}
		return "", "", 0, nil
	}
	defer func() { drupexec.RunWithEnvCtx = origRunWithEnvCtx }()
	os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{}}`), 0o644)
	// The nested composer-install path now goes through the same
	// guardedCall as every other guarded tool (PR6), which also requires a
	// fresh backup manifest before allowing the mutation through.
	writeFreshBackupManifest(t, dir)

	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	if _, err := realHandleUpgradeScan(args); err == nil || !strings.Contains(err.Error(), "prepare_upgrade_status") {
		t.Fatalf("unprepared upgrade_scan error = %v", err)
	}
}

func TestRealHandlePrepareUpgradeStatus(t *testing.T) {
	tests := []struct {
		name         string
		composer     string
		pmList       string
		wantCommands []string
		wantComposer bool
	}{
		{"uninstalled", `{"require":{}}`, `{}`, []string{"pm:list", "config:delete", "en", "cr"}, true},
		{"disabled with conflict", `{"require":{"drupal/upgrade_status":"^4"}}`, `{}`, []string{"pm:list", "config:delete", "en", "cr"}, false},
		{"already enabled", `{"require":{"drupal/upgrade_status":"^4"}}`, `{"upgrade_status":{}}`, []string{"pm:list"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(tt.composer), 0o644); err != nil {
				t.Fatal(err)
			}
			origDetector, origRun := defaultEnvDetector, drupexec.RunWithEnv
			defaultEnvDetector = &mockEnvDetectorDirect{}
			var commands []string
			drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
				if cmd == "composer" {
					if tt.wantComposer {
						if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"^4"}}`), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					return "", "", 0, nil
				}
				if cmd == "drush" && len(args) > 0 {
					commands = append(commands, args[0])
					if args[0] == "pm:list" {
						return tt.pmList, "", 0, nil
					}
				}
				return "", "", 0, nil
			}
			t.Cleanup(func() { defaultEnvDetector, drupexec.RunWithEnv = origDetector, origRun })

			if _, err := realHandlePrepareUpgradeStatus(json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)); err != nil {
				t.Fatalf("prepare_upgrade_status error: %v", err)
			}
			if strings.Join(commands, ",") != strings.Join(tt.wantCommands, ",") {
				t.Errorf("drush commands = %v, want %v", commands, tt.wantCommands)
			}
		})
	}
}

func TestReadOnlyScansRequirePreparedUpgradeStatus(t *testing.T) {
	tests := []struct {
		name     string
		composer string
		pmList   string
		handler  func(json.RawMessage) (json.RawMessage, error)
		wantRun  bool
	}{
		{"scan prepared", `{"require":{"drupal/upgrade_status":"^4"}}`, `{"upgrade_status":{}}`, realHandleScan, true},
		{"scan disabled", `{"require":{"drupal/upgrade_status":"^4"}}`, `{}`, realHandleScan, false},
		{"scan missing", `{"require":{}}`, `{}`, realHandleScan, false},
		{"upgrade scan prepared", `{"require":{"drupal/upgrade_status":"^4"}}`, `{"upgrade_status":{}}`, realHandleUpgradeScan, true},
		{"upgrade scan conflict", `{"require":{"drupal/upgrade_status":"^4"}}`, `{}`, realHandleUpgradeScan, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(tt.composer), 0o644); err != nil {
				t.Fatal(err)
			}
			origDetector, origRun := defaultEnvDetector, drupexec.RunWithEnv
			defaultEnvDetector = &mockEnvDetectorDirect{}
			var calls []string
			drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
				if cmd == "composer" || (cmd == "drush" && len(args) > 0 && (args[0] == "config:delete" || args[0] == "en" || args[0] == "cr")) {
					t.Errorf("read-only handler executed mutation: %s %v", cmd, args)
				}
				if cmd == "drush" && len(args) > 0 {
					calls = append(calls, args[0])
					if args[0] == "pm:list" {
						return tt.pmList, "", 0, nil
					}
					if args[0] == "upgrade_status:analyze" {
						return `<?xml version="1.0"?><checkstyle/>`, "", 0, nil
					}
				}
				return "", "", 0, nil
			}
			t.Cleanup(func() { defaultEnvDetector, drupexec.RunWithEnv = origDetector, origRun })

			_, err := tt.handler(json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`))
			if tt.wantRun && err != nil {
				t.Fatalf("prepared scan error: %v", err)
			}
			if !tt.wantRun {
				if err == nil || !strings.Contains(err.Error(), "prepare_upgrade_status") {
					t.Fatalf("unprepared error = %v, want preparation guidance", err)
				}
				if tt.name == "scan missing" && len(calls) != 0 {
					t.Errorf("missing-module calls = %v, want none", calls)
				}
				if tt.name != "scan missing" && (len(calls) != 1 || calls[0] != "pm:list") {
					t.Errorf("unprepared calls = %v, want only pm:list", calls)
				}
			}
		})
	}
	_, err := realHandleScan(json.RawMessage(`{"project_path":"/nonexistent"}`))
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("invalid scan path error = %v, want path-not-found", err)
	}
}

func TestValidateIsReadOnlyAndRequiresPreparedUpgradeStatus(t *testing.T) {
	tests := []struct {
		name    string
		pmList  string
		module  string
		xml     string
		wantErr bool
		wantN   int
	}{
		{"zero findings", `{"upgrade_status":{}}`, "", `<?xml version="1.0"?><checkstyle/>`, false, 0},
		{"all findings", `{"upgrade_status":{}}`, "", `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymodule/a.module"><error line="1" message="Deprecated" severity="error"/></file></checkstyle>`, false, 1},
		{"module findings", `{"upgrade_status":{}}`, "mymodule", `<?xml version="1.0"?><checkstyle><file name="modules/custom/mymodule/a.module"><error line="1" message="Deprecated" severity="error"/></file></checkstyle>`, false, 1},
		{"unprepared", `{}`, "", `<?xml version="1.0"?><checkstyle/>`, true, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/upgrade_status":"^4"}}`), 0o644)
			os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"packages":[{"name":"drupal/core","version":"11.0.0"}]}`), 0o644)
			origDetector, origRun := defaultEnvDetector, drupexec.RunWithEnv
			defaultEnvDetector = &mockEnvDetectorDirect{}
			var calls []string
			drupexec.RunWithEnv = func(_ string, _ []string, cmd string, args ...string) (string, string, int, error) {
				if cmd == "drush" && len(args) > 0 {
					calls = append(calls, args[0])
					if args[0] == "pm:list" {
						return tt.pmList, "", 0, nil
					}
					if args[0] == "upgrade_status:analyze" {
						return tt.xml, "", 0, nil
					}
				}
				return "", "", 0, nil
			}
			t.Cleanup(func() { defaultEnvDetector, drupexec.RunWithEnv = origDetector, origRun })

			result, err := realHandleValidate(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"module_name":` + jsonStr(tt.module) + `}`))
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "prepare_upgrade_status") {
					t.Fatalf("unprepared validate error = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate error: %v", err)
			}
			var response struct {
				TotalErrors  int    `json:"total_errors"`
				EvidenceHash string `json:"evidence_hash"`
			}
			json.Unmarshal(result, &response)
			if response.TotalErrors != tt.wantN || response.EvidenceHash == "" {
				t.Errorf("response = %+v, want errors=%d and evidence hash", response, tt.wantN)
			}
			for _, call := range calls {
				if call == "updb" || call == "cr" || call == "en" || call == "config:delete" {
					t.Errorf("validate executed mutation: %s", call)
				}
			}
		})
	}
}

func TestAutofixDoesNotRescan(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "modules", "custom"), 0o755)
	origDetector, origRun := defaultEnvDetector, drupexec.RunWithEnvCtx
	defaultEnvDetector = &mockEnvDetectorDirect{}
	drushCalled := false
	drupexec.RunWithEnvCtx = func(_ context.Context, _ string, _ []string, cmd string, args ...string) (string, string, int, error) {
		if cmd == "drush" {
			drushCalled = true
		}
		return "rector summary", "", 0, nil
	}
	t.Cleanup(func() { defaultEnvDetector, drupexec.RunWithEnvCtx = origDetector, origRun })

	result, err := realHandleAutofix(json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`))
	if err != nil {
		t.Fatalf("autofix error: %v", err)
	}
	if drushCalled {
		t.Fatal("autofix ran a forbidden analysis command")
	}
	if !strings.Contains(string(result), "rector summary") || strings.Contains(string(result), "remaining_errors") {
		t.Errorf("autofix result = %s, want rector summary without rescan result", result)
	}
}
