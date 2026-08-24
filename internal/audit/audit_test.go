package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// captureLog stubs the package-level logFn seam so a write failure can be
// observed without depending on the real os.Stderr stream.
func captureLog(t *testing.T) *[]string {
	t.Helper()
	orig := logFn
	var captured []string
	logFn = func(msg string) { captured = append(captured, msg) }
	t.Cleanup(func() { logFn = orig })
	return &captured
}

func TestAppend_SuccessRecordShape(t *testing.T) {
	dir := t.TempDir()
	Append(dir, "apply_patch", []byte(`{"patch_url":"https://drupal.org/x"}`), ResultSuccess, "abc123")

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	r := records[0]
	if r.Tool != "apply_patch" {
		t.Errorf("Tool = %q, want apply_patch", r.Tool)
	}
	if r.ArgsHash == "" {
		t.Error("ArgsHash must not be empty")
	}
	if r.Result != ResultSuccess {
		t.Errorf("Result = %q, want %q", r.Result, ResultSuccess)
	}
	if r.CommitHash != "abc123" {
		t.Errorf("CommitHash = %q, want abc123", r.CommitHash)
	}
	if r.Timestamp.IsZero() {
		t.Error("Timestamp must not be zero")
	}
}

func TestAppend_FailureRecordShape(t *testing.T) {
	dir := t.TempDir()
	Append(dir, "composer_require", []byte(`{"package":"drupal/foo"}`), ResultFailure, "")

	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	r := records[0]
	if r.Result != ResultFailure {
		t.Errorf("Result = %q, want %q", r.Result, ResultFailure)
	}
	if r.CommitHash != "" {
		t.Errorf("CommitHash = %q, want empty for a failed mutation", r.CommitHash)
	}
}

func TestAppend_ArgsHashIsSHA256OfRawArgs(t *testing.T) {
	dir := t.TempDir()
	raw := []byte(`{"a":1}`)
	Append(dir, "tool", raw, ResultSuccess, "")

	records, err := ReadAll(dir)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadAll: %d records, err=%v", len(records), err)
	}
	if got, want := records[0].ArgsHash, HashArgs(raw); got != want {
		t.Errorf("ArgsHash = %q, want %q", got, want)
	}
}

func TestAppend_WriteFailureDoesNotBlockAndIsLogged(t *testing.T) {
	warnings := captureLog(t)

	// A regular file used as the "project path" makes os.MkdirAll(<file>/.drup)
	// fail deterministically (a path component is not a directory), without
	// any mock seam over the filesystem.
	fileAsProject := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(fileAsProject, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Must not panic — the whole point of Append is that it never blocks the
	// caller's tool response on ledger I/O.
	Append(fileAsProject, "cleanup", []byte(`{}`), ResultSuccess, "")

	if len(*warnings) != 1 {
		t.Fatalf("expected exactly one logged write failure, got %d: %v", len(*warnings), *warnings)
	}
	if !strings.Contains((*warnings)[0], "cleanup") {
		t.Errorf("logged message %q does not identify the tool", (*warnings)[0])
	}
}

func TestReadAll_EmptyLedgerReturnsNoRecordsNoError(t *testing.T) {
	dir := t.TempDir()
	records, err := ReadAll(dir)
	if err != nil {
		t.Fatalf("ReadAll error on empty/missing ledger: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("len(records) = %d, want 0 for a project with no ledger yet", len(records))
	}
}

func TestCount_CountsRecordsAtOrAfterSince(t *testing.T) {
	dir := t.TempDir()
	Append(dir, "apply_patch", []byte(`{}`), ResultSuccess, "")
	cutoff := time.Now()
	time.Sleep(2 * time.Millisecond)
	Append(dir, "composer_require", []byte(`{}`), ResultSuccess, "")

	count, err := Count(dir, cutoff)
	if err != nil {
		t.Fatalf("Count error: %v", err)
	}
	if count != 1 {
		t.Errorf("Count(since=cutoff) = %d, want 1 (only the record written after cutoff)", count)
	}
}

func TestCheckCap_DefaultCapAppliedWhenUnconfigured(t *testing.T) {
	dir := t.TempDir()
	// No .drup/config.json written — projectconfig.Load must fall back to
	// its built-in safe default (50 per session, per internal/projectconfig).
	allowed, count, capN, err := CheckCap(dir, true, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("CheckCap error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected an empty ledger to be well within any default cap")
	}
	if count != 0 {
		t.Errorf("count = %d, want 0 for an empty ledger", count)
	}
	if capN <= 0 {
		t.Errorf("capN = %d, want a positive built-in default cap", capN)
	}
}

func TestCheckCap_CapReachedRefusesWithCount(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".drup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".drup", "config.json"), []byte(`{"mutation_cap_per_session":2}`), 0o644); err != nil {
		t.Fatal(err)
	}

	openedAt := time.Now().Add(-time.Hour)
	Append(dir, "apply_patch", []byte(`{}`), ResultSuccess, "")
	Append(dir, "apply_patch", []byte(`{}`), ResultSuccess, "")

	allowed, count, capN, err := CheckCap(dir, true, openedAt)
	if err != nil {
		t.Fatalf("CheckCap error: %v", err)
	}
	if allowed {
		t.Fatal("expected the cap to be reached and refuse further mutations")
	}
	if count != 2 {
		t.Errorf("count = %d, want 2", count)
	}
	if capN != 2 {
		t.Errorf("capN = %d, want 2 (configured)", capN)
	}
}

func TestCheckCap_PerDayWhenNoSession(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".drup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".drup", "config.json"), []byte(`{"mutation_cap_per_day":1}`), 0o644); err != nil {
		t.Fatal(err)
	}

	Append(dir, "apply_patch", []byte(`{}`), ResultSuccess, "")

	allowed, count, capN, err := CheckCap(dir, false, time.Time{})
	if err != nil {
		t.Fatalf("CheckCap error: %v", err)
	}
	if allowed {
		t.Fatal("expected the per-day cap (1) to be reached by the one recorded mutation")
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
	if capN != 1 {
		t.Errorf("capN = %d, want 1 (configured per-day cap)", capN)
	}
}
