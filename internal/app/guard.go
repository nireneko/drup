package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nireneko/drup/internal/audit"
	"github.com/nireneko/drup/internal/backup"
	"github.com/nireneko/drup/internal/mcp"
	"github.com/nireneko/drup/internal/operation"
	"github.com/nireneko/drup/internal/projectconfig"
	"github.com/nireneko/drup/internal/runstate"
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

// guardHandler wraps a mutating descriptor's real handler with the
// session/backup-freshness/mutation-cap guard. WireMCPTools calls it solely
// from ToolSpec.Effect, so adding a mutating tool cannot accidentally bypass
// the guard by being omitted from a second registration list.
func guardHandler(spec mcp.ToolSpec, handler mcp.ToolHandler) mcp.ToolHandler {
	return func(args json.RawMessage) (json.RawMessage, error) {
		return guardedSpecCall(spec, args, handler)
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
	spec, ok := mcp.ToolSpecByName(name)
	if !ok {
		return nil, fmt.Errorf("missing tool descriptor: %s", name)
	}
	// This helper exists for internal composed effects (notably upgrade_scan's
	// install path) that have no client request identity of their own. External
	// MCP dispatch, including Server.CallTool, always enters guardedSpecCall.
	return guardedSpecCallLegacy(spec, args, handler)
}

// guardedSpecCall is the effect-bound guard implementation. Its behavior is
// entirely determined by the descriptor passed from MCP wiring; the legacy
// session name partitions are retained only for their direct package API.
func guardedSpecCall(spec mcp.ToolSpec, args json.RawMessage, handler mcp.ToolHandler) (json.RawMessage, error) {
	projectPath := resolveGuardProjectPath(args)
	outcome := session.EvaluateGuardPolicy(spec.Name, projectPath, string(spec.SessionPolicy))
	if !outcome.Allowed {
		audit.Append(projectPath, spec.Name, args, audit.ResultFailure, "")
		return nil, outcome.Err
	}
	// The operation identity is mandatory for every external mutation,
	// including policies that force a dry-run. A dry-run may be retried by the
	// caller, but it must not let a mutating contract bypass its identity.
	// Session and safety-policy errors retain their established precedence.
	requestID, err := requiredRequestID(args)
	if err != nil {
		return nil, err
	}
	if spec.RequiresRun {
		runID, err := requiredRunID(args)
		if err != nil {
			return nil, err
		}
		root, err := session.ResolveSymlinks(projectPath)
		if err != nil {
			return nil, err
		}
		if _, err := runstate.NewStore(root).ValidateMutation(runID, root, spec.Name); err != nil {
			return nil, err
		}
	}
	if outcome.ForceDryRun {
		return guardedSpecCallLegacy(spec, args, handler)
	}

	fingerprint, err := operation.Fingerprint(spec.Name, args)
	if err != nil {
		return nil, err
	}
	store := operation.NewStore(projectPath)
	if existing, findErr := store.FindRequest(requestID); findErr == nil {
		if existing.Tool != spec.Name || existing.Fingerprint != fingerprint {
			return nil, operation.ErrIdentityMismatch
		}
		switch existing.State {
		case operation.StateCompleted:
			return existing.Response, nil
		case operation.StateFailed:
			if len(existing.Response) > 0 {
				return existing.Response, nil
			}
			return nil, fmt.Errorf("previous operation failed: %s", existing.Error)
		case operation.StateUnknown, operation.StateStarted:
			return nil, operation.UnknownError(fmt.Errorf("operation %s is %s; reconcile observable evidence before retrying", requestID, existing.State))
		}
	} else if !errors.Is(findErr, operation.ErrNotFound) {
		return nil, findErr
	}

	if _, err := store.Start(requestID, spec.Name, fingerprint); err != nil {
		if errors.Is(err, operation.ErrEquivalentUnknown) {
			return nil, operation.UnknownError(err)
		}
		return nil, err
	}
	result, err := guardedSpecCallLegacy(spec, args, handler)
	if err != nil {
		if isAmbiguousMutationError(err) {
			if _, persistErr := store.Unknown(requestID, err.Error()); persistErr != nil {
				return nil, persistErr
			}
			return nil, operation.UnknownError(err)
		}
		if _, persistErr := store.Fail(requestID, err.Error()); persistErr != nil {
			return nil, persistErr
		}
		return nil, err
	}
	if payloadIndicatesFailure(result) {
		if _, err := store.FailWithResponse(requestID, result, "handler returned success:false"); err != nil {
			return nil, err
		}
		return result, nil
	}
	if _, err := store.Complete(requestID, result); err != nil {
		return nil, err
	}
	return result, nil
}

func guardedSpecCallLegacy(spec mcp.ToolSpec, args json.RawMessage, handler mcp.ToolHandler) (json.RawMessage, error) {
	name := spec.Name
	projectPath := resolveGuardProjectPath(args)

	outcome := session.EvaluateGuardPolicy(name, projectPath, string(spec.SessionPolicy))
	if !outcome.Allowed {
		audit.Append(projectPath, name, args, audit.ResultFailure, "")
		return nil, outcome.Err
	}

	if outcome.SessionBound {
		sess, _ := session.Current()
		cfg := projectconfig.Load(projectPath)

		if spec.RequiresBackup {
			newestAt, found := newestBackupManifest(projectPath)
			if bfOutcome := session.EvaluateBackupFreshnessPolicy(name, newestAt, found, sess.OpenedAt, cfg.BackupFreshnessWindow, true); !bfOutcome.Allowed {
				audit.Append(projectPath, name, args, audit.ResultFailure, "")
				return nil, bfOutcome.Err
			}
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
	if payloadIndicatesFailure(result) {
		audit.Append(projectPath, name, args, audit.ResultFailure, "")
		return result, nil
	}
	audit.Append(projectPath, name, args, audit.ResultSuccess, extractCommitHash(result))
	return result, nil
}

func requiredRequestID(args json.RawMessage) (string, error) {
	var input struct {
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if input.RequestID == "" {
		return "", fmt.Errorf("request_id is required for mutating tool calls")
	}
	return input.RequestID, nil
}

func requiredRunID(args json.RawMessage) (string, error) {
	var input struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return "", err
	}
	if input.RunID == "" {
		return "", fmt.Errorf("run_id is required for mutating tool calls")
	}
	return input.RunID, nil
}

func isAmbiguousMutationError(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func payloadIndicatesFailure(result json.RawMessage) bool {
	var payload struct {
		Success *bool `json:"success"`
	}
	return json.Unmarshal(result, &payload) == nil && payload.Success != nil && !*payload.Success
}

// extractCommitHash looks for the handful of result field names the
// guarded tools use to report a git revision. checkpoint_commit and legacy
// recovery operations may produce commits; core_upgrade_apply reports its
// pre-mutation rollback anchor instead. Absence means no revision was
// applicable to the effect.
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
