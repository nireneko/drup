package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nireneko/drup/internal/audit"
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
