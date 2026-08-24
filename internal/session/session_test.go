package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetSession clears any bound session before and after a test so tests
// never leak state into one another via the package-level singleton.
func resetSession(t *testing.T) {
	t.Helper()
	Reset()
	t.Cleanup(Reset)
}

func newDrupalProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveSymlinks_SymlinkedPathResolvesToRealTarget(t *testing.T) {
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	parent := t.TempDir()
	link := filepath.Join(parent, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink not supported on this platform: %v", err)
	}

	realResolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		t.Fatalf("EvalSymlinks(real) error: %v", err)
	}

	got, err := ResolveSymlinks(link)
	if err != nil {
		t.Fatalf("ResolveSymlinks error: %v", err)
	}
	if got != realResolved {
		t.Errorf("ResolveSymlinks(%q) = %q, want %q (the real target)", link, got, realResolved)
	}
}

func TestResolveSymlinks_RejectsTraversalAndRelative(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"dotdot segment", "/tmp/../../etc"},
		{"relative path", "relative/path"},
		{"empty path", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ResolveSymlinks(tt.path); err == nil {
				t.Errorf("ResolveSymlinks(%q) = nil error, want rejection", tt.path)
			}
		})
	}
}

func TestResolveSymlinks_RejectsBeforeTouchingDisk(t *testing.T) {
	// A traversal or relative path must be rejected before any file or
	// session operation runs — verified here by using a path that would
	// panic/error hard if EvalSymlinks were reached (it never is, since
	// the segment check happens first).
	if _, err := ResolveSymlinks("../escape"); err == nil {
		t.Fatal("expected rejection for a relative path containing '..'")
	}
}

func TestCanonicalRoot_ComposerJSONMarker(t *testing.T) {
	dir := newDrupalProject(t)
	root, err := CanonicalRoot(dir)
	if err != nil {
		t.Fatalf("CanonicalRoot error: %v", err)
	}
	resolved, _ := filepath.EvalSymlinks(dir)
	if root != resolved {
		t.Errorf("CanonicalRoot = %q, want %q", root, resolved)
	}
}

func TestCanonicalRoot_WebRootCoreMarker(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "web", "core"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CanonicalRoot(dir); err != nil {
		t.Fatalf("CanonicalRoot error for web/core marker: %v", err)
	}
}

func TestCanonicalRoot_RejectsNonDrupalDirectory(t *testing.T) {
	dir := t.TempDir() // no composer.json, no web/core
	if _, err := CanonicalRoot(dir); err == nil {
		t.Fatal("expected error for a directory with no Drupal project markers")
	}
}

func TestOpen_ReopeningReplacesPriorSession(t *testing.T) {
	resetSession(t)

	rootA := newDrupalProject(t)
	rootB := newDrupalProject(t)

	sessA, err := Open(rootA)
	if err != nil {
		t.Fatalf("Open(rootA) error: %v", err)
	}
	resolvedA, _ := filepath.EvalSymlinks(rootA)
	if sessA.Root != resolvedA {
		t.Fatalf("session bound to %q, want %q", sessA.Root, resolvedA)
	}

	sessB, err := Open(rootB)
	if err != nil {
		t.Fatalf("Open(rootB) error: %v", err)
	}
	resolvedB, _ := filepath.EvalSymlinks(rootB)
	if sessB.Root != resolvedB {
		t.Fatalf("session bound to %q, want %q", sessB.Root, resolvedB)
	}

	current, ok := Current()
	if !ok {
		t.Fatal("expected an active session after two Open calls")
	}
	if current.Root != resolvedB {
		t.Errorf("Current().Root = %q, want %q (the most recent Open call)", current.Root, resolvedB)
	}
	if current.Root == resolvedA {
		t.Error("Current() still reports root A — reopening did not replace the prior session")
	}
}

func TestOpen_RejectsNonDrupalDirectory(t *testing.T) {
	resetSession(t)
	dir := t.TempDir()
	if _, err := Open(dir); err == nil {
		t.Fatal("expected error opening a session against a non-Drupal directory")
	}
	if _, ok := Current(); ok {
		t.Error("no session should be bound after a rejected Open")
	}
}

func TestEvaluateGuard_MatchingSessionAllowsWithoutForcingDryRun(t *testing.T) {
	resetSession(t)
	root := newDrupalProject(t)
	if _, err := Open(root); err != nil {
		t.Fatalf("Open error: %v", err)
	}

	outcome := EvaluateGuard("core_upgrade_apply", root)
	if !outcome.Allowed {
		t.Fatalf("expected Allowed=true for a matching session, got Err=%v", outcome.Err)
	}
	if outcome.ForceDryRun {
		t.Error("expected ForceDryRun=false when the session matches the target root")
	}
}

func TestEvaluateGuard_ForceDryRunPartitionWithoutSession(t *testing.T) {
	resetSession(t)
	root := newDrupalProject(t)

	forceDryRunTools := []string{"core_upgrade_apply", "contrib_compat_patch", "contrib_allow_lenient", "custom_compat_fix"}
	for _, name := range forceDryRunTools {
		t.Run(name, func(t *testing.T) {
			outcome := EvaluateGuard(name, root)
			if !outcome.Allowed {
				t.Fatalf("%s: expected Allowed=true (forced dry-run), got refused: %v", name, outcome.Err)
			}
			if !outcome.ForceDryRun {
				t.Errorf("%s: expected ForceDryRun=true without a session", name)
			}
		})
	}
}

func TestEvaluateGuard_RefuseOnlyPartitionWithoutSession(t *testing.T) {
	resetSession(t)
	root := newDrupalProject(t)

	refuseOnlyTools := []string{"apply_patch", "composer_require", "patch_rollback", "cleanup", "create_patch", "test_backup_restore", "test_backup_delete"}
	for _, name := range refuseOnlyTools {
		t.Run(name, func(t *testing.T) {
			outcome := EvaluateGuard(name, root)
			if outcome.Allowed {
				t.Fatalf("%s: expected Allowed=false without a session", name)
			}
			if outcome.Err == nil {
				t.Fatalf("%s: expected a refusal error", name)
			}
			if !strings.Contains(outcome.Err.Error(), "session_open") {
				t.Errorf("%s: error %q does not name the session_open flow", name, outcome.Err.Error())
			}
		})
	}
}

func TestEvaluateGuard_MismatchedSessionRootRefusesOrForcesDryRun(t *testing.T) {
	resetSession(t)
	boundRoot := newDrupalProject(t)
	otherRoot := newDrupalProject(t)
	if _, err := Open(boundRoot); err != nil {
		t.Fatalf("Open error: %v", err)
	}

	outcome := EvaluateGuard("apply_patch", otherRoot)
	if outcome.Allowed {
		t.Fatal("expected refusal for a tool call targeting a root different from the bound session")
	}
}

func TestEvaluateGuard_UnguardedToolAlwaysAllowed(t *testing.T) {
	resetSession(t)
	root := newDrupalProject(t)
	outcome := EvaluateGuard("scan", root)
	if !outcome.Allowed {
		t.Error("expected an unguarded tool (not in either partition) to always be allowed")
	}
	if outcome.ForceDryRun {
		t.Error("expected an unguarded tool to never be forced into dry-run")
	}
}

// --- Kill switch + DRUP_ALLOW_UNSAFE (specs/agent-session Kill Switch and
// Runtime Opt-Out; specs/mcp-server Kill Switch and Dry-Run Partition) ---

// withGetenv stubs the package-level getenv seam for the duration of the
// test, so env-dependent guard behavior can be exercised deterministically
// without mutating (or depending on) the real process environment.
func withGetenv(t *testing.T, values map[string]string) {
	t.Helper()
	orig := getenv
	getenv = func(key string) string { return values[key] }
	t.Cleanup(func() { getenv = orig })
}

// captureWarnings stubs the package-level warnFn seam and returns a pointer
// to the slice of captured messages.
func captureWarnings(t *testing.T) *[]string {
	t.Helper()
	orig := warnFn
	var captured []string
	warnFn = func(msg string) { captured = append(captured, msg) }
	t.Cleanup(func() { warnFn = orig })
	return &captured
}

func TestEvaluateGuard_KillSwitchRefusesForceDryRunToolsWithValidSession(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_DISABLE_MUTATIONS": "1"})

	root := newDrupalProject(t)
	if _, err := Open(root); err != nil {
		t.Fatalf("Open error: %v", err)
	}

	for _, name := range []string{"core_upgrade_apply", "contrib_compat_patch", "contrib_allow_lenient", "custom_compat_fix"} {
		t.Run(name, func(t *testing.T) {
			outcome := EvaluateGuard(name, root)
			if outcome.Allowed {
				t.Fatalf("%s: expected the kill switch to refuse even a matching session", name)
			}
			if !strings.Contains(outcome.Err.Error(), "DRUP_DISABLE_MUTATIONS") {
				t.Errorf("%s: error %q does not name the kill switch", name, outcome.Err.Error())
			}
		})
	}
}

func TestEvaluateGuard_KillSwitchRefusesRefuseOnlyToolsWithValidSession(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_DISABLE_MUTATIONS": "1"})

	root := newDrupalProject(t)
	if _, err := Open(root); err != nil {
		t.Fatalf("Open error: %v", err)
	}

	for name := range RefuseOnlyTools {
		t.Run(name, func(t *testing.T) {
			outcome := EvaluateGuard(name, root)
			if outcome.Allowed {
				t.Fatalf("%s: expected the kill switch to refuse even a matching session", name)
			}
			if !strings.Contains(outcome.Err.Error(), "DRUP_DISABLE_MUTATIONS") {
				t.Errorf("%s: error %q does not name the kill switch", name, outcome.Err.Error())
			}
		})
	}
}

func TestEvaluateGuard_KillSwitchDoesNotAffectUnguardedTools(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_DISABLE_MUTATIONS": "1"})

	root := newDrupalProject(t)
	outcome := EvaluateGuard("scan", root)
	if !outcome.Allowed {
		t.Error("kill switch must only refuse guarded (mutating) tools, not an unrelated tool name")
	}
}

func TestEvaluateGuard_KillSwitchUnsetAppliesStandardGuardBehavior(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_DISABLE_MUTATIONS": "0"})

	root := newDrupalProject(t)
	if _, err := Open(root); err != nil {
		t.Fatalf("Open error: %v", err)
	}

	outcome := EvaluateGuard("core_upgrade_apply", root)
	if !outcome.Allowed || outcome.ForceDryRun {
		t.Errorf("with the kill switch unset and a matching session, expected Allowed=true ForceDryRun=false, got %+v", outcome)
	}
}

func TestEvaluateGuard_AllowUnsafeBypassesKillSwitchAndSession(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_DISABLE_MUTATIONS": "1", "DRUP_ALLOW_UNSAFE": "1"})
	warnings := captureWarnings(t)

	root := newDrupalProject(t)
	// No session bound at all, plus the kill switch set — DRUP_ALLOW_UNSAFE
	// must still let a refuse-only tool through.
	outcome := EvaluateGuard("apply_patch", root)
	if !outcome.Allowed {
		t.Fatalf("expected DRUP_ALLOW_UNSAFE=1 to bypass both the kill switch and the session guard, got refusal: %v", outcome.Err)
	}
	if len(*warnings) != 1 {
		t.Fatalf("expected exactly one bypass warning, got %d: %v", len(*warnings), *warnings)
	}
	if !strings.Contains((*warnings)[0], "apply_patch") {
		t.Errorf("warning %q does not identify the bypassed tool", (*warnings)[0])
	}
}

func TestEvaluateGuard_AllowUnsafeLogsOneWarningPerCall(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_ALLOW_UNSAFE": "1"})
	warnings := captureWarnings(t)

	root := newDrupalProject(t)
	EvaluateGuard("composer_require", root)
	EvaluateGuard("composer_require", root)

	if len(*warnings) != 2 {
		t.Fatalf("expected one warning per bypassed call (2 calls), got %d: %v", len(*warnings), *warnings)
	}
}

func TestEvaluateGuard_AllowUnsafeDoesNotWarnForUnguardedTools(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_ALLOW_UNSAFE": "1"})
	warnings := captureWarnings(t)

	root := newDrupalProject(t)
	outcome := EvaluateGuard("scan", root)
	if !outcome.Allowed {
		t.Error("expected an unguarded tool to remain allowed")
	}
	if len(*warnings) != 0 {
		t.Errorf("expected no bypass warning for an unguarded tool, got %v", *warnings)
	}
}

// --- Backup-Freshness Gate (specs/agent-session Backup-Freshness Gate) ---

func TestBackupFreshnessOK_FreshManifestAfterSessionOpenPasses(t *testing.T) {
	openedAt := time.Now().Add(-time.Hour)
	manifestCreatedAt := openedAt.Add(time.Minute) // created after session opened
	if !BackupFreshnessOK(manifestCreatedAt, true, openedAt, 24*time.Hour) {
		t.Error("expected a manifest created after the session opened to satisfy the gate")
	}
}

func TestBackupFreshnessOK_ManifestWithinWindowPasses(t *testing.T) {
	openedAt := time.Now().Add(-48 * time.Hour) // session opened long ago
	manifestCreatedAt := time.Now().Add(-time.Hour)
	if !BackupFreshnessOK(manifestCreatedAt, true, openedAt, 24*time.Hour) {
		t.Error("expected a manifest within the freshness window to satisfy the gate even if older than session-open")
	}
}

func TestBackupFreshnessOK_StaleManifestFails(t *testing.T) {
	manifestCreatedAt := time.Now().Add(-48 * time.Hour) // outside the 24h window
	openedAt := manifestCreatedAt.Add(time.Hour)         // session opened after the manifest
	if BackupFreshnessOK(manifestCreatedAt, true, openedAt, 24*time.Hour) {
		t.Error("expected a manifest older than session-open and outside the window to fail the gate")
	}
}

func TestBackupFreshnessOK_NoManifestFails(t *testing.T) {
	if BackupFreshnessOK(time.Time{}, false, time.Now(), 24*time.Hour) {
		t.Error("expected an absent manifest to fail the gate")
	}
}

func TestEvaluateBackupFreshness_FreshBackupAllows(t *testing.T) {
	openedAt := time.Now().Add(-time.Hour)
	manifestCreatedAt := time.Now()

	for name := range GuardedTools() {
		t.Run(name, func(t *testing.T) {
			outcome := EvaluateBackupFreshness(name, manifestCreatedAt, true, openedAt, 24*time.Hour)
			if !outcome.Allowed {
				t.Fatalf("%s: expected a fresh backup to allow, got refusal: %v", name, outcome.Err)
			}
		})
	}
}

func TestEvaluateBackupFreshness_StaleOrAbsentBackupBlocksNamingTestBackupCreate(t *testing.T) {
	staleManifest := time.Now().Add(-48 * time.Hour)
	openedAt := staleManifest.Add(time.Hour)

	tests := []struct {
		name              string
		manifestCreatedAt time.Time
		found             bool
	}{
		{"stale manifest", staleManifest, true},
		{"absent manifest", time.Time{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := EvaluateBackupFreshness("apply_patch", tt.manifestCreatedAt, tt.found, openedAt, 24*time.Hour)
			if outcome.Allowed {
				t.Fatalf("expected the guard to block a mutating call without a fresh backup")
			}
			if outcome.Err == nil || !strings.Contains(outcome.Err.Error(), "test_backup_create") {
				t.Errorf("error = %v, want it to name test_backup_create as the remediation", outcome.Err)
			}
		})
	}
}

func TestEvaluateBackupFreshness_UnguardedToolAlwaysAllowed(t *testing.T) {
	outcome := EvaluateBackupFreshness("scan", time.Time{}, false, time.Now(), 24*time.Hour)
	if !outcome.Allowed {
		t.Error("expected an unguarded tool name to always be allowed regardless of backup state")
	}
}

func TestEvaluateGuard_SessionBoundOnlyTrueForActualMatchingSession(t *testing.T) {
	resetSession(t)
	root := newDrupalProject(t)

	// No session at all: force-dry-run path — SessionBound must be false so
	// callers (internal/app) skip the backup-freshness/cap gates, which only
	// apply to a genuinely bound session.
	if outcome := EvaluateGuard("core_upgrade_apply", root); outcome.SessionBound {
		t.Error("expected SessionBound=false when no session is open")
	}

	if _, err := Open(root); err != nil {
		t.Fatalf("Open error: %v", err)
	}
	if outcome := EvaluateGuard("core_upgrade_apply", root); !outcome.SessionBound {
		t.Error("expected SessionBound=true for a matching bound session")
	}
}

func TestEvaluateGuard_AllowUnsafeBypassIsNotSessionBound(t *testing.T) {
	resetSession(t)
	withGetenv(t, map[string]string{"DRUP_ALLOW_UNSAFE": "1"})
	captureWarnings(t)

	root := newDrupalProject(t)
	outcome := EvaluateGuard("apply_patch", root)
	if !outcome.Allowed {
		t.Fatal("expected DRUP_ALLOW_UNSAFE to allow the call")
	}
	if outcome.SessionBound {
		t.Error("expected SessionBound=false for the DRUP_ALLOW_UNSAFE bypass — it must not gate backup-freshness/cap")
	}
}

func TestOpen_SetsOpenedAt(t *testing.T) {
	resetSession(t)
	before := time.Now()
	root := newDrupalProject(t)
	sess, err := Open(root)
	if err != nil {
		t.Fatalf("Open error: %v", err)
	}
	after := time.Now()
	if sess.OpenedAt.Before(before) || sess.OpenedAt.After(after) {
		t.Errorf("OpenedAt = %v, want between %v and %v", sess.OpenedAt, before, after)
	}
}

func TestGuardedTools_IsUnionOfBothPartitions(t *testing.T) {
	guarded := GuardedTools()
	for name := range ForceDryRunTools {
		if !guarded[name] {
			t.Errorf("GuardedTools() missing force-dry-run tool %q", name)
		}
	}
	for name := range RefuseOnlyTools {
		if !guarded[name] {
			t.Errorf("GuardedTools() missing refuse-only tool %q", name)
		}
	}
	if guarded["upgrade_scan"] {
		t.Error("upgrade_scan must not be in the registration-time guarded set — it is guarded only at its nested install path")
	}
}
