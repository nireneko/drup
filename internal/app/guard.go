package app

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/nireneko/drup/internal/audit"
	"github.com/nireneko/drup/internal/backup"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/projectconfig"
	"github.com/nireneko/drup/internal/session"
)

// realHandleSessionOpen resolves project_path to its canonical Drupal
// project root and binds a session to it for the remainder of the server
// process's lifetime, replacing whatever session (if any) was bound before.
func realHandleSessionOpen(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	if params.ProjectPath == "" {
		return nil, fmt.Errorf("project_path is required")
	}

	sess, err := session.Open(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"session_active": true,
		"root":           sess.Root,
	}
	return json.Marshal(result)
}

// guardProjectPath extracts args' project_path field for guard evaluation.
type guardProjectPath struct {
	ProjectPath string `json:"project_path"`
}

// guardHandler wraps a mutating tool's real handler with the registration-
// time session guard (session/backup-freshness/mutation-cap, per
// specs/agent-session and specs/mutation-audit) via guardedCall. This is
// the middleware half of the guard placement design decision — the other
// half is the nested composer_require call inside realHandleUpgradeScan,
// which bypasses this wrapper entirely (it calls straight into
// realHandleComposerRequire) and so re-enters the same guardedCall path
// explicitly at its own call site instead.
func guardHandler(name string, handler mcp.ToolHandler) mcp.ToolHandler {
	return func(args json.RawMessage) (json.RawMessage, error) {
		return guardedCall(name, args, handler)
	}
}

// resolveGuardProjectPath extracts args' project_path, falling back to the
// working directory when absent — create_patch's project_path is optional
// and defaults the same way inside its real handler, so the guard must
// evaluate the identical root the handler will actually operate on.
func resolveGuardProjectPath(args json.RawMessage) string {
	var p guardProjectPath
	if err := json.Unmarshal(args, &p); err != nil {
		return ""
	}
	if p.ProjectPath != "" {
		return p.ProjectPath
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return ""
}

// newestBackupManifest returns the CreatedAt timestamp of the most recent
// backup manifest for projectPath, and whether one exists at all. It reuses
// backup.Manager.List (already sorted CreatedAt desc) — internal/session
// cannot call it directly without an import cycle (internal/backup already
// imports internal/session for ResolveSymlinks), so this app-level glue is
// what actually satisfies specs/agent-session's Backup-Freshness Gate.
func newestBackupManifest(projectPath string) (time.Time, bool) {
	manifests, err := backup.NewManager(projectPath).List(projectPath)
	if err != nil || len(manifests) == 0 {
		return time.Time{}, false
	}
	return manifests[0].CreatedAt, true
}

// guardedCall evaluates the full guard chain for a mutating tool call
// (design.md's Data Flow: kill-switch/session → backup-freshness → mutation
// cap → real handler), auditing every branch outcome via internal/audit and
// the real handler's own completion. It is the single place both the
// registration-time guardHandler middleware and the nested upgrade_scan
// composer-install call site route through, so both get identical
// guarding and auditing.
func guardedCall(name string, args json.RawMessage, handler mcp.ToolHandler) (json.RawMessage, error) {
	projectPath := resolveGuardProjectPath(args)

	outcome := session.EvaluateGuard(name, projectPath)
	if !outcome.Allowed {
		audit.Append(projectPath, name, args, audit.ResultFailure, "")
		return nil, outcome.Err
	}

	if outcome.SessionBound {
		sess, _ := session.Current()
		cfg := projectconfig.Load(projectPath)

		newestAt, found := newestBackupManifest(projectPath)
		if bfOutcome := session.EvaluateBackupFreshness(name, newestAt, found, sess.OpenedAt, cfg.BackupFreshnessWindow); !bfOutcome.Allowed {
			audit.Append(projectPath, name, args, audit.ResultFailure, "")
			return nil, bfOutcome.Err
		}

		if allowed, count, capN, err := audit.CheckCap(projectPath, true, sess.OpenedAt); err == nil && !allowed {
			refuseErr := fmt.Errorf("%s: mutation cap reached (%d/%d) for this session — see pipeline_status", name, count, capN)
			audit.Append(projectPath, name, args, audit.ResultFailure, "")
			return nil, refuseErr
		}
	}

	if outcome.ForceDryRun {
		forced, err := forceDryRunArg(args)
		if err != nil {
			return nil, err
		}
		args = forced
	}

	result, err := handler(args)
	if err != nil {
		audit.Append(projectPath, name, args, audit.ResultFailure, "")
		return nil, err
	}
	audit.Append(projectPath, name, args, audit.ResultSuccess, extractCommitHash(result))
	return result, nil
}

// extractCommitHash looks for the handful of result field names the
// guarded tools use to report a git commit that resulted from the
// mutation (apply_patch's "commit_hash", patch_rollback's
// "reverted_commit", core_upgrade_apply's "rollback_checkpoint"). Absence
// of all three simply means the mutation was not applicable/did not
// produce a commit, per specs/mutation-audit's "commit hash when
// applicable".
func extractCommitHash(result json.RawMessage) string {
	if len(result) == 0 {
		return ""
	}
	var fields map[string]interface{}
	if err := json.Unmarshal(result, &fields); err != nil {
		return ""
	}
	for _, key := range []string{"commit_hash", "reverted_commit", "rollback_checkpoint"} {
		if v, ok := fields[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// forceDryRunArg returns a copy of args with "dry_run" set to true,
// overriding whatever value (or absence) was there before.
func forceDryRunArg(args json.RawMessage) (json.RawMessage, error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(args, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["dry_run"] = true
	return json.Marshal(fields)
}
