package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nireneko/drup/internal/audit"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/operation"
	"github.com/nireneko/drup/internal/session"
)

// writeFreshBackupManifest writes a minimal backup manifest that
// backup.Manager.List can parse, timestamped "now" so it always satisfies
// the backup-freshness gate regardless of when the session opened.
func writeFreshBackupManifest(t *testing.T, projectDir string) {
	t.Helper()
	dir := filepath.Join(projectDir, ".drup", "backups", "test-backup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]interface{}{
		"backup_id":    "test-backup",
		"created_at":   time.Now().UTC().Format(time.RFC3339Nano),
		"project_path": projectDir,
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGuardedCall_ConfirmedRequestIsDeduplicatedWithoutSecondAuditOrCapUse(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatal(err)
	}

	calls := 0
	handler := func(json.RawMessage) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"success":true,"report":"written"}`), nil
	}
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"request-1"}`)
	spec, _ := mcp.ToolSpecByName("generate_report")
	first, err := guardedSpecCall(spec, args, handler)
	if err != nil {
		t.Fatalf("first guardedCall() error = %v", err)
	}
	second, err := guardedSpecCall(spec, args, handler)
	if err != nil {
		t.Fatalf("second guardedCall() error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}
	if string(second) != string(first) {
		t.Fatalf("second result = %s, want stored %s", second, first)
	}
	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("audit records = %d, want 1", len(records))
	}
}

func TestGuardedCall_RequiresRequestIDBeforeForcedDryRunPolicy(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	spec, ok := mcp.ToolSpecByName("core_upgrade_apply")
	if !ok {
		t.Fatal("missing core_upgrade_apply descriptor")
	}
	_, err := guardedSpecCall(spec, json.RawMessage(`{"project_path":`+jsonStr(dir)+`}`), func(json.RawMessage) (json.RawMessage, error) {
		t.Fatal("handler must not run without request_id")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "request_id is required") {
		t.Fatalf("guardedSpecCall() error = %v, want missing request_id refusal", err)
	}
}

func TestGuardedCall_RejectsRequestIDReusedForDifferentOperation(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatal(err)
	}
	handler := func(json.RawMessage) (json.RawMessage, error) { return json.RawMessage(`{"success":true}`), nil }
	spec, _ := mcp.ToolSpecByName("generate_report")
	if _, err := guardedSpecCall(spec, json.RawMessage(`{"project_path":`+jsonStr(dir)+`,"request_id":"request-1","report_type":"json"}`), handler); err != nil {
		t.Fatal(err)
	}
	_, err := guardedSpecCall(spec, json.RawMessage(`{"project_path":`+jsonStr(dir)+`,"request_id":"request-1","report_type":"markdown"}`), handler)
	if !errors.Is(err, operation.ErrIdentityMismatch) {
		t.Fatalf("reused request error = %v, want ErrIdentityMismatch", err)
	}
}

func TestGuardedCall_UnknownBlocksEquivalentUntilObservableReconciliation(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler := func(json.RawMessage) (json.RawMessage, error) {
		calls++
		return nil, context.DeadlineExceeded
	}
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"request-1"}`)
	spec, _ := mcp.ToolSpecByName("generate_report")
	_, err := guardedSpecCall(spec, args, handler)
	if !operation.IsUnknown(err) {
		t.Fatalf("timeout error = %v, want unknown outcome", err)
	}
	_, err = guardedSpecCall(spec, json.RawMessage(`{"project_path":`+jsonStr(dir)+`,"request_id":"request-2"}`), handler)
	if !errors.Is(err, operation.ErrEquivalentUnknown) {
		t.Fatalf("equivalent retry error = %v, want ErrEquivalentUnknown", err)
	}
	if calls != 1 {
		t.Fatalf("handler calls = %d, want 1", calls)
	}

	if err := os.WriteFile(filepath.Join(dir, "drup-report.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := realHandleOperationReconcile(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"request_id":"request-1","evidence_path":"drup-report.json"}`)); err != nil {
		t.Fatalf("realHandleOperationReconcile() error = %v", err)
	}
	if _, err := guardedSpecCall(spec, json.RawMessage(`{"project_path":`+jsonStr(dir)+`,"request_id":"request-2"}`), func(json.RawMessage) (json.RawMessage, error) {
		calls++
		return json.RawMessage(`{"success":true}`), nil
	}); err != nil {
		t.Fatalf("retry after reconciliation error = %v", err)
	}
}

func TestGuardedCall_MatchingSessionWithFreshBackupAllowsAndAudits(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}

	called := false
	handler := func(args json.RawMessage) (json.RawMessage, error) {
		called = true
		return json.Marshal(map[string]interface{}{"commit_hash": "deadbeef"})
	}

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	if _, err := guardedCall("apply_patch", args, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the handler to run when session and backup are both fresh")
	}

	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatalf("audit.ReadAll error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].Result != audit.ResultSuccess {
		t.Errorf("Result = %q, want %q", records[0].Result, audit.ResultSuccess)
	}
	if records[0].CommitHash != "deadbeef" {
		t.Errorf("CommitHash = %q, want deadbeef (extracted from the handler's result)", records[0].CommitHash)
	}
	if records[0].Tool != "apply_patch" {
		t.Errorf("Tool = %q, want apply_patch", records[0].Tool)
	}
}

func TestGuardedCall_MatchingSessionWithoutFreshBackupRefusesNamingTestBackupCreate(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	// No backup manifest written at all.
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}

	called := false
	handler := func(args json.RawMessage) (json.RawMessage, error) {
		called = true
		return json.Marshal(map[string]interface{}{})
	}

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := guardedCall("apply_patch", args, handler)
	if err == nil {
		t.Fatal("expected refusal without a fresh backup")
	}
	if !strings.Contains(err.Error(), "test_backup_create") {
		t.Errorf("error %q does not name test_backup_create", err.Error())
	}
	if called {
		t.Error("handler must not run when the backup-freshness gate refuses")
	}

	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatalf("audit.ReadAll error: %v", err)
	}
	if len(records) != 1 || records[0].Result != audit.ResultFailure {
		t.Fatalf("expected exactly one failure record for the backup refusal, got %+v", records)
	}
}

func TestGuardedCall_SessionRefusalWithoutSessionIsAudited(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)

	handler := func(args json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]interface{}{})
	}
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	_, err := guardedCall("apply_patch", args, handler)
	if err == nil {
		t.Fatal("expected refusal without a bound session")
	}

	records, err := audit.ReadAll(dir)
	if err != nil {
		t.Fatalf("audit.ReadAll error: %v", err)
	}
	if len(records) != 1 || records[0].Result != audit.ResultFailure {
		t.Fatalf("expected exactly one failure record for the session refusal, got %+v", records)
	}
}

func TestGuardedCall_ForceDryRunWithoutSessionSkipsBackupAndCapGates(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	// No backup manifest present at all — the force-dry-run-without-session
	// path must not be gated by backup-freshness, only by the session guard
	// itself (which already forces dry_run).

	called := false
	handler := func(args json.RawMessage) (json.RawMessage, error) {
		called = true
		var fields map[string]interface{}
		if err := json.Unmarshal(args, &fields); err != nil {
			t.Fatal(err)
		}
		if fields["dry_run"] != true {
			t.Error("expected dry_run to be forced to true")
		}
		return json.Marshal(map[string]interface{}{"success": true})
	}

	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)
	if _, err := guardedCall("core_upgrade_apply", args, handler); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected the handler to run in forced dry-run mode")
	}
}

func TestGuardedCall_CapReachedRefusesWithCount(t *testing.T) {
	resetSessionForTest(t)
	dir := newDrupalProjectDir(t)
	writeFreshBackupManifest(t, dir)
	if err := os.WriteFile(filepath.Join(dir, ".drup", "config.json"), []byte(`{"mutation_cap_per_session":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := session.Open(dir); err != nil {
		t.Fatalf("session.Open error: %v", err)
	}

	handler := func(args json.RawMessage) (json.RawMessage, error) {
		return json.Marshal(map[string]interface{}{})
	}
	args := json.RawMessage(`{"project_path":` + jsonStr(dir) + `}`)

	if _, err := guardedCall("apply_patch", args, handler); err != nil {
		t.Fatalf("first call should be allowed under the cap: %v", err)
	}
	_, err := guardedCall("apply_patch", args, handler)
	if err == nil {
		t.Fatal("expected the second call to refuse — the configured cap of 1 was reached")
	}
	if !strings.Contains(err.Error(), "1") {
		t.Errorf("error %q does not name the cap/count", err.Error())
	}
}
