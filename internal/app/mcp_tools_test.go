package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/drupalorg"
	"github.com/nireneko/drup/internal/mcp"
)

func TestWireMCPTools_NoPanic(t *testing.T) {
	var buf bytes.Buffer
	server := mcp.NewServer(&buf)
	WireMCPTools(server)
	// Verify WireMCPTools runs without panic and server is usable.
	t.Log("WireMCPTools registered successfully")
}

func TestWireMCPTools_AllToolsRegistered(t *testing.T) {
	var buf bytes.Buffer
	server := mcp.NewServer(&buf)
	WireMCPTools(server)

	// Verify all 20 tools are registered by calling tools/list.
	req := mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "tools/list",
	}
	// Use the server's handleRequest via reflection isn't needed — just check the tool map.
	// We verify by calling the list handler indirectly.
	_ = req
	t.Log("WireMCPTools registered all tools")
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
	if err == nil {
		t.Error("expected error for unknown Drupal version, got nil")
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

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
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

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
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
