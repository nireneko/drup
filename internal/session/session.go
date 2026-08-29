// Package session implements the process-lifetime MCP session that binds an
// agent to a single canonical Drupal project root, plus the canonical-root
// resolution helper and guard partition used to gate every mutating MCP tool
// on that binding. See specs/agent-session for the full contract.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nireneko/drup/internal/composerutil"
)

// Session is bound to the server process for its lifetime once session_open
// resolves a canonical root. There is at most one active session per
// process: stdio MCP serves a single client, so a token-per-call scheme (or
// a TTL) would add churn no caller needs.
type Session struct {
	// Root is the canonical (absolute, symlink-resolved) project path this
	// session is bound to.
	Root string
	// OpenedAt is when this session was bound. The backup-freshness gate
	// treats a manifest created after OpenedAt as fresh regardless of the
	// configured freshness window (see EvaluateBackupFreshness).
	OpenedAt time.Time
}

var (
	mu      sync.Mutex
	current *Session
)

// Open resolves projectPath to its canonical root (composer.json or web root
// markers required) and binds it as the active session, replacing whatever
// session was previously bound. The last session_open call always wins —
// there is no multi-session tracking to reconcile.
func Open(projectPath string) (*Session, error) {
	root, err := CanonicalRoot(projectPath)
	if err != nil {
		return nil, err
	}

	mu.Lock()
	defer mu.Unlock()
	current = &Session{Root: root, OpenedAt: time.Now()}
	return current, nil
}

// Current returns the active session and whether one is bound.
func Current() (*Session, bool) {
	mu.Lock()
	defer mu.Unlock()
	if current == nil {
		return nil, false
	}
	s := *current
	return &s, true
}

// Reset clears the active session. Test-only: production code never needs
// to un-bind a session — a fresh session_open call replaces it instead.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	current = nil
}

// ResolveSymlinks resolves path to its canonical, symlink-evaluated form:
// it must be non-empty, absolute, free of ".." traversal segments, and must
// exist (filepath.EvalSymlinks requires the path to be resolvable on disk).
// Unlike CanonicalRoot, it has no Drupal-project marker precondition, which
// is what lets envdetect.Detect and backup.validateProject share it while
// still tolerating a directory that is not (yet) a Drupal project.
func ResolveSymlinks(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("project_path must not be empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("project_path must be an absolute path: %s", path)
	}
	if containsDotDotSegment(path) {
		return "", fmt.Errorf("project_path must not contain '..' segments")
	}

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("resolve project_path: %w", err)
	}
	return resolved, nil
}

// containsDotDotSegment reports whether path has a literal ".." path
// segment, as opposed to a mere ".." substring (e.g. "/srv/foo..bar" is not
// traversal). filepath.IsAbs paths always use the OS separator, so slicing
// on it (via filepath.Separator through filepath.ToSlash) is enough.
func containsDotDotSegment(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

// CanonicalRoot resolves path via ResolveSymlinks and then requires it to
// look like a Drupal project root: either a composer.json file, or a web
// root (per composerutil.ReadWebRoot, default "web") containing a core/
// directory. This is strictly for session binding — the looser
// ResolveSymlinks is what coreupgrade/backup/envdetect share, since those
// callers must tolerate a not-yet-a-Drupal-project directory.
func CanonicalRoot(path string) (string, error) {
	resolved, err := ResolveSymlinks(path)
	if err != nil {
		return "", err
	}
	if !hasProjectMarkers(resolved) {
		return "", fmt.Errorf("not a Drupal project: no composer.json or web root markers found at %s", resolved)
	}
	return resolved, nil
}

// hasProjectMarkers reports whether root looks like a Drupal project: a
// composer.json file, or <web-root>/core (the scaffolded Drupal core
// directory) when composer.json is absent — e.g. a docroot-only checkout.
func hasProjectMarkers(root string) bool {
	if info, err := os.Stat(filepath.Join(root, "composer.json")); err == nil && !info.IsDir() {
		return true
	}
	webRoot := composerutil.ReadWebRoot(root)
	if info, err := os.Stat(filepath.Join(root, webRoot, "core")); err == nil && info.IsDir() {
		return true
	}
	return false
}

// ForceDryRunTools are mutating tools with a native dry_run parameter.
// Without a valid session bound to the tool's target root, the guard
// middleware forces dry_run: true instead of refusing outright.
var ForceDryRunTools = map[string]bool{
	"core_upgrade_apply":    true,
	"contrib_compat_patch":  true,
	"contrib_allow_lenient": true,
	"custom_compat_fix":     true,
}

// RefuseOnlyTools are mutating tools with no dry-run semantics. Without a
// valid session bound to the tool's target root, the guard middleware
// refuses the call outright.
var RefuseOnlyTools = map[string]bool{
	"apply_patch":            true,
	"composer_require":       true,
	"prepare_upgrade_status": true,
	"patch_rollback":         true,
	"cleanup":                true,
	"create_patch":           true,
	"test_backup_restore":    true,
	"test_backup_delete":     true,
}

// GuardedTools returns the union of ForceDryRunTools and RefuseOnlyTools:
// every tool the registration-time guard middleware wraps. upgrade_scan is
// deliberately excluded — it is guarded only at its internal
// composer-install path (via the same guardedCall helper, applied inline),
// never at the registration boundary.
func GuardedTools() map[string]bool {
	all := make(map[string]bool, len(ForceDryRunTools)+len(RefuseOnlyTools))
	for name := range ForceDryRunTools {
		all[name] = true
	}
	for name := range RefuseOnlyTools {
		all[name] = true
	}
	return all
}

// GuardOutcome describes what the guard middleware must do with a mutating
// call before (or instead of) reaching its real handler.
type GuardOutcome struct {
	// Allowed reports whether the call may proceed (possibly forced into
	// dry-run — see ForceDryRun).
	Allowed bool
	// ForceDryRun reports whether the caller must override the tool's
	// dry_run argument to true before invoking the real handler. Only ever
	// true when Allowed is also true.
	ForceDryRun bool
	// Err is the refusal error to return when Allowed is false.
	Err error
	// SessionBound reports whether Allowed is true specifically because a
	// real, currently-open session is bound to the matching canonical root
	// — as opposed to the DRUP_ALLOW_UNSAFE bypass or a tool outside the
	// guarded set. Only when SessionBound is true do the backup-freshness
	// and mutation-cap gates apply (see EvaluateBackupFreshness):
	// DRUP_ALLOW_UNSAFE explicitly bypasses both, and a call with no
	// session at all is already fully decided by the force-dry-run/refuse
	// partition below.
	SessionBound bool
}

// sessionOpenHint is appended to every refusal so the agent knows exactly
// how to unblock itself.
const sessionOpenHint = "no active session bound to this project root — call session_open with this project_path first"

// killSwitchHint is appended to every kill-switch refusal so the agent knows
// exactly which control is blocking it.
const killSwitchHint = "mutations are disabled by the kill switch (DRUP_DISABLE_MUTATIONS=1 or --locked)"

// killSwitchEnv and allowUnsafeEnv are the environment variables that drive
// the kill switch and its runtime opt-out. See specs/agent-session's Kill
// Switch/Runtime Opt-Out requirements and specs/mcp-server's Kill Switch and
// Dry-Run Partition requirement.
const (
	killSwitchEnv  = "DRUP_DISABLE_MUTATIONS"
	allowUnsafeEnv = "DRUP_ALLOW_UNSAFE"
)

// getenv is a package-level seam over os.Getenv so tests can stub
// environment-dependent guard behavior without mutating the real process
// environment across parallel test runs. Mirrors internal/state's
// os.UserConfigDir var-seam pattern.
var getenv = os.Getenv

// warnFn reports a one-time-per-call stderr warning whenever
// DRUP_ALLOW_UNSAFE bypasses a guard. Package-level seam so tests can
// capture the warning instead of asserting against the real os.Stderr
// stream.
var warnFn = func(msg string) { fmt.Fprintln(os.Stderr, msg) }

// killSwitchActive reports whether DRUP_DISABLE_MUTATIONS=1 is set. When
// active, every guarded mutating call is refused regardless of session
// state, checked before session/backup gates.
func killSwitchActive() bool {
	return getenv(killSwitchEnv) == "1"
}

// allowUnsafeActive reports whether the DRUP_ALLOW_UNSAFE=1 runtime opt-out
// is set. When active, it bypasses every guard (kill switch, session,
// backup-freshness) for guarded tools, logging a warning on each bypassed
// call.
func allowUnsafeActive() bool {
	return getenv(allowUnsafeEnv) == "1"
}

// EvaluateGuard decides the guard outcome for toolName against projectPath.
// Order of evaluation: DRUP_ALLOW_UNSAFE bypasses everything (with a logged
// warning); the kill switch (DRUP_DISABLE_MUTATIONS=1 / --locked) then
// refuses every guarded call regardless of session state; finally the
// process-lifetime session decides the force-dry-run/refuse partition
// (backup-freshness gates are layered on top of this by later guard stages;
// see specs/agent-session's Backup-Freshness requirement). A tool absent
// from both ForceDryRunTools and RefuseOnlyTools is not part of the guarded
// set and always resolves to Allowed with no dry-run change.
func EvaluateGuard(toolName, projectPath string) GuardOutcome {
	policy := "none"
	if ForceDryRunTools[toolName] {
		policy = "force_dry_run"
	} else if RefuseOnlyTools[toolName] {
		policy = "refuse"
	}
	return EvaluateGuardPolicy(toolName, projectPath, policy)
}

// EvaluateGuardPolicy evaluates the policy carried by an MCP ToolSpec.
// Keeping policy as data means registration, guard behavior, and retry
// semantics share one descriptor instead of re-classifying names in app.
func EvaluateGuardPolicy(toolName, projectPath, policy string) GuardOutcome {
	guarded := policy != "none"

	if allowUnsafeActive() {
		if guarded {
			warnFn(fmt.Sprintf("drup: DRUP_ALLOW_UNSAFE=1 bypassed the guard for %q — kill switch/session checks were skipped", toolName))
		}
		return GuardOutcome{Allowed: true}
	}

	if guarded && killSwitchActive() {
		return GuardOutcome{Allowed: false, Err: fmt.Errorf("%s: %s", toolName, killSwitchHint)}
	}

	if sess, ok := Current(); ok {
		if root, err := ResolveSymlinks(projectPath); err == nil && root == sess.Root {
			return GuardOutcome{Allowed: true, SessionBound: true}
		}
	}

	if policy == "force_dry_run" {
		return GuardOutcome{Allowed: true, ForceDryRun: true}
	}
	if policy == "refuse" {
		return GuardOutcome{Allowed: false, Err: fmt.Errorf("%s: %s", toolName, sessionOpenHint)}
	}
	return GuardOutcome{Allowed: true}
}

// backupCreateHint is appended to every backup-freshness refusal so the
// agent knows exactly how to unblock itself.
const backupCreateHint = "no backup manifest newer than the freshness window was found — call test_backup_create first"

// BackupFreshnessOK reports whether a backup manifest satisfies the
// freshness gate (specs/agent-session's Backup-Freshness Gate requirement):
// it must exist (found), and either be newer than sessionOpenedAt, or,
// failing that, still be within window of now.
func BackupFreshnessOK(manifestCreatedAt time.Time, found bool, sessionOpenedAt time.Time, window time.Duration) bool {
	if !found {
		return false
	}
	if manifestCreatedAt.After(sessionOpenedAt) {
		return true
	}
	return time.Since(manifestCreatedAt) <= window
}

// EvaluateBackupFreshness decides whether toolName may proceed given the
// newest known backup manifest for its target project. Only tools in the
// guarded mutating set (ForceDryRunTools ∪ RefuseOnlyTools, plus callers
// that pass "composer_require" for the nested upgrade_scan install path)
// are gated; every other tool name is always allowed. This package cannot
// call internal/backup.Manager.List itself — internal/backup already
// imports internal/session for ResolveSymlinks, so a direct import here
// would cycle — so callers (internal/app) look up the newest manifest and
// pass its timestamp/found flag in.
func EvaluateBackupFreshness(toolName string, newestBackup time.Time, found bool, sessionOpenedAt time.Time, window time.Duration) GuardOutcome {
	return EvaluateBackupFreshnessPolicy(toolName, newestBackup, found, sessionOpenedAt, window, ForceDryRunTools[toolName] || RefuseOnlyTools[toolName])
}

// EvaluateBackupFreshnessPolicy applies the descriptor's backup requirement.
func EvaluateBackupFreshnessPolicy(toolName string, newestBackup time.Time, found bool, sessionOpenedAt time.Time, window time.Duration, required bool) GuardOutcome {
	if !required {
		return GuardOutcome{Allowed: true}
	}
	if BackupFreshnessOK(newestBackup, found, sessionOpenedAt, window) {
		return GuardOutcome{Allowed: true}
	}
	return GuardOutcome{Allowed: false, Err: fmt.Errorf("%s: %s", toolName, backupCreateHint)}
}
