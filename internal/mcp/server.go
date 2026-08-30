package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/metrics"
)

// scannerStartBuf is the initial per-line buffer bufio.Scanner grows from.
// scannerMaxLine is the hard ceiling a single JSON-RPC request line may
// reach before it is rejected. Both are well above the 64KB bufio.Scanner
// default: agents can legitimately send large tool arguments (e.g. long
// diffs or file contents), and the previous default silently killed the
// whole stdio loop once a line exceeded it.
const (
	scannerStartBuf = 64 * 1024
	scannerMaxLine  = 10 * 1024 * 1024
)

// JSONRPCRequest is a JSON-RPC 2.0 request.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error.
type JSONRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// ToolCallParams is the params for a tools/call request.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Server is the MCP stdio server.
type Server struct {
	out     io.Writer
	tools   map[string]ToolHandler
	version string
}

// ToolHandler is a function that handles a tool call.
type ToolHandler func(args json.RawMessage) (json.RawMessage, error)

// jsonSchemaProperty defines a single property in a JSON Schema.
type jsonSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ToolEffect declares whether a tool may change project or workflow state.
// The class is consumed by both MCP wiring and the app guard; callers must
// never infer it from a tool name or its description.
type ToolEffect string

const (
	EffectReadOnly ToolEffect = "read_only"
	EffectMutating ToolEffect = "mutating"
	// EffectWorkflow changes the durable run authority but is not a project
	// mutation. Workflow tools intentionally do not enter the session/backup
	// guard or the operation ledger.
	EffectWorkflow ToolEffect = "workflow"
)

// SessionPolicy controls how a mutating tool behaves without a matching
// bound session. It is separate from Effect because a dry-run capable tool
// may still mutate when a session is valid.
type SessionPolicy string

const (
	SessionPolicyNone        SessionPolicy = "none"
	SessionPolicyRefuse      SessionPolicy = "refuse"
	SessionPolicyForceDryRun SessionPolicy = "force_dry_run"
)

// ToolSpec is the single descriptor for an MCP tool. The same descriptor
// supplies tools/list schema data, stub visibility, and effect policy used by
// production wiring. Handler implementations remain in their owning package.
type ToolSpec struct {
	Name           string
	Description    string                        `json:"description"`
	Properties     map[string]jsonSchemaProperty `json:"properties"`
	Required       []string                      `json:"required"`
	Effect         ToolEffect
	Timeout        time.Duration
	Role           string
	Preconditions  []string
	Stub           bool
	SessionPolicy  SessionPolicy
	RequiresBackup bool
	RetryEligible  bool
	// ReconcilesOperation marks the recovery endpoint. It persists observed
	// evidence but does not itself invoke the guarded domain effect.
	ReconcilesOperation bool
	// RequiresRun makes a project-mutating tool prove membership in the
	// canonical persisted run before its handler can execute.
	RequiresRun bool
}

// toolSchema preserves the package-private name used by existing transport
// tests while making every registry entry a complete ToolSpec.
type toolSchema = ToolSpec

// toolRegistry maps tool names to their schemas.
var toolRegistry = map[string]toolSchema{
	"scan": {
		Description: "Run read-only upgrade_status:analyze on a prepared Drupal project",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"prepare_upgrade_status": {
		Description: "Install and enable Upgrade Status before read-only analysis",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"autofix": {
		Description: "Run drupal-rector on custom modules and themes",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"contrib_check": {
		Description: "Check Drupal.org for D11 compatibility of a module",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name": {Type: "string", Description: "Module machine name"},
		},
		Required: []string{"module_machine_name"},
	},
	"issue_patches": {
		Description: "Extract patch/diff/MR links from Drupal.org issues",
		Properties: map[string]jsonSchemaProperty{
			"issue_nid":   {Type: "string", Description: "Issue node ID"},
			"module_name": {Type: "string", Description: "Module machine name"},
		},
		Required: []string{},
	},
	"apply_patch": {
		Description: "Download and apply a patch to the project",
		Properties: map[string]jsonSchemaProperty{
			"patch_url":        {Type: "string", Description: "URL of the patch file, or a path inside the project"},
			"project_path":     {Type: "string", Description: "Absolute path to the Drupal project"},
			"composer_package": {Type: "string", Description: "Package the patch belongs to, e.g. drupal/devel. Required for module and theme patches so paths resolve from the package root"},
			"description":      {Type: "string", Description: "Patch description recorded in composer.json extra.patches"},
		},
		Required: []string{"patch_url", "project_path", "composer_package", "description"},
	},
	"validate": {
		Description: "Re-run scan and return error state",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"scope":        {Type: "string", Description: "Scope filter: custom, contrib, theme, core or all (default all)"},
			"module_name":  {Type: "string", Description: "Module name filter (optional)"},
			"file":         {Type: "string", Description: "File path filter (optional)"},
		},
		Required: []string{"project_path"},
	},
	"create_patch": {
		Description: "Generate a patch from rector fixes",
		Properties: map[string]jsonSchemaProperty{
			"module_name":         {Type: "string", Description: "Module machine name"},
			"deprecation_details": {Type: "string", Description: "Deprecation details"},
			"project_path":        {Type: "string", Description: "Absolute path to the Drupal project root (defaults to the current directory)"},
		},
		Required: []string{"module_name"},
	},
	"detect_env": {
		Description: "Detect the development environment",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"force_detect": {Type: "boolean", Description: "Force re-detection"},
		},
		Required: []string{"project_path"},
	},
	"composer_require": {
		Description: "Run composer require with environment awareness",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"package":      {Type: "string", Description: "Composer package name"},
			"dev":          {Type: "boolean", Description: "Install as dev dependency"},
			"no_update":    {Type: "boolean", Description: "Skip composer update"},
		},
		Required: []string{"project_path", "package"},
	},
	"drush_exec": {
		Description: "Execute drush commands with environment awareness",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"command":      {Type: "string", Description: "Drush command"},
			"args":         {Type: "array", Description: "Command arguments"},
			"format":       {Type: "string", Description: "Output format (json, table, etc.)"},
		},
		Required: []string{"project_path", "command"},
	},
	"contrib_upgrade_path": {
		Description: "Get upgrade path for a contrib module",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name":    {Type: "string", Description: "Module machine name"},
			"current_drupal_version": {Type: "string", Description: "Current Drupal version"},
			"target_drupal_version":  {Type: "string", Description: "Target Drupal version"},
		},
		Required: []string{"module_machine_name", "current_drupal_version", "target_drupal_version"},
	},
	"upgrade_scan": {
		Description: "Run read-only upgrade scan on a prepared project",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"scope":        {Type: "string", Description: "Scope filter: custom, contrib, theme, core or all (default all)"},
			"module":       {Type: "string", Description: "Module name filter"},
		},
		Required: []string{"project_path"},
	},
	"patch_status": {
		Description: "Check if a patch is applied",
		Properties: map[string]jsonSchemaProperty{
			"project_path":     {Type: "string", Description: "Absolute path to the Drupal project"},
			"patch_url":        {Type: "string", Description: "URL of the patch"},
			"composer_package": {Type: "string", Description: "Composer package name"},
		},
		Required: []string{"project_path", "patch_url", "composer_package"},
	},
	"patch_rollback": {
		Description: "Rollback a patch",
		Properties: map[string]jsonSchemaProperty{
			"project_path":     {Type: "string", Description: "Absolute path to the Drupal project"},
			"patch_url":        {Type: "string", Description: "URL of the patch"},
			"composer_package": {Type: "string", Description: "Composer package name"},
		},
		Required: []string{"project_path", "patch_url", "composer_package"},
	},
	"generate_report": {
		Description: "Generate upgrade report",
		Properties: map[string]jsonSchemaProperty{
			"project_path":       {Type: "string", Description: "Absolute path to the Drupal project"},
			"report_type":        {Type: "string", Description: "Report type (json, markdown, both)"},
			"include_scan_data":  {Type: "boolean", Description: "Include scan data in report"},
			"include_patch_list": {Type: "boolean", Description: "Include patch list in report"},
		},
		Required: []string{"project_path"},
	},
	"module_info": {
		Description: "Get module metadata from Drupal.org",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name":  {Type: "string", Description: "Module machine name"},
			"include_maintainers":  {Type: "boolean", Description: "Include maintainer info"},
			"include_dependencies": {Type: "boolean", Description: "Include dependency info"},
		},
		Required: []string{"module_machine_name"},
	},
	"drupal_version_matrix": {
		Description: "Get Drupal/PHP version compatibility matrix",
		Properties: map[string]jsonSchemaProperty{
			"drupal_version": {Type: "string", Description: "Drupal version"},
			"php_version":    {Type: "string", Description: "PHP version"},
		},
		Required: []string{},
	},
	"core_upgrade_check": {
		Description: "Check if core upgrade is available",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"target_major": {Type: "integer", Description: "Optional requested Drupal target major; must have exact compatibility metadata"},
		},
		Required: []string{"project_path"},
	},
	"core_upgrade_apply": {
		Description: "Apply one planned core upgrade step",
		Properties: map[string]jsonSchemaProperty{
			"project_path":   {Type: "string", Description: "Absolute path to the Drupal project"},
			"target_version": {Type: "string", Description: "Deprecated compatibility alias for target_major"},
			"target_major":   {Type: "integer", Description: "Target Drupal major with an exact compatibility catalog"},
			"dry_run":        {Type: "boolean", Description: "Dry run mode"},
		},
		Required: []string{"project_path"},
	},
	"patch_reconcile": {
		Description: "Reconcile patches with upstream",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name": {Type: "string", Description: "Module machine name"},
			"current_patch_url":   {Type: "string", Description: "Current patch URL"},
		},
		Required: []string{"module_machine_name", "current_patch_url"},
	},
	"contrib_compat_patch": {
		Description: "Make a contributed module work on a newer Drupal major: run drupal-rector and the Drupal coding standards over it, widen its core_version_requirement, save the result as a patch in the project, register it in composer.json and add the package to the lenient allow list. Composer reads published metadata rather than patched files, so the patch alone is not enough",
		Properties: map[string]jsonSchemaProperty{
			"project_path":        {Type: "string", Description: "Absolute path to the Drupal project"},
			"module_machine_name": {Type: "string", Description: "Contrib module or theme machine name"},
			"target_version":      {Type: "string", Description: "Target Drupal major, e.g. 11 (default 11)"},
			"dry_run":             {Type: "boolean", Description: "Report the change without writing it"},
			"declaration_only":    {Type: "boolean", Description: "Only widen core_version_requirement, skipping rector and the coding standards pass"},
		},
		Required: []string{"project_path", "module_machine_name"},
	},
	"contrib_allow_lenient": {
		Description: "Let a patched contrib module install against a newer core. Composer resolves against a package's released metadata, never its patched files, so a module whose D11 patch already widened its .info.yml is still rejected. Adds the named packages to composer's drupal-lenient allow list, installing the plugin if needed",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"packages":     {Type: "array", Description: "Composer package names, e.g. drupal/switch_page_theme"},
			"dry_run":      {Type: "boolean", Description: "Report the change without writing it"},
		},
		Required: []string{"project_path", "packages"},
	},
	"custom_compat_fix": {
		Description: "Declare support for a target Drupal major in the project's own modules, themes and profiles by widening core_version_requirement. Contrib is never edited in place",
		Properties: map[string]jsonSchemaProperty{
			"project_path":   {Type: "string", Description: "Absolute path to the Drupal project"},
			"target_version": {Type: "string", Description: "Target Drupal major, e.g. 11 (default 11)"},
			"dry_run":        {Type: "boolean", Description: "Report the rewrites without writing them"},
		},
		Required: []string{"project_path"},
	},
	"cleanup": {
		Description: "Post-pipeline cleanup — uninstall dev modules and revert any temporary patches. Only runs when validate_passed=true.",
		Properties: map[string]jsonSchemaProperty{
			"project_path":    {Type: "string", Description: "Absolute path to the Drupal project"},
			"validate_passed": {Type: "boolean", Description: "If false, cleanup is skipped to preserve debugging state"},
		},
		Required: []string{"project_path", "validate_passed"},
	},
	"test_backup_create": {
		Description: "Create a local testing backup of a Drupal project",
		Properties:  map[string]jsonSchemaProperty{"project_path": {Type: "string", Description: "Absolute Drupal project path"}},
		Required:    []string{"project_path"},
	},
	"test_backup_list": {
		Description: "List local testing backups",
		Properties:  map[string]jsonSchemaProperty{"project_path": {Type: "string", Description: "Absolute Drupal project path"}},
		Required:    []string{"project_path"},
	},
	"test_backup_restore": {
		Description: "Restore a confirmed local testing backup",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute Drupal project path"},
			"backup_id":    {Type: "string", Description: "Backup ID"},
			"confirm":      {Type: "boolean", Description: "Explicitly confirm destructive restore"},
		},
		Required: []string{"project_path", "backup_id", "confirm"},
	},
	"test_backup_delete": {
		Description: "Delete a local testing backup",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute Drupal project path"},
			"backup_id":    {Type: "string", Description: "Backup ID"},
		},
		Required: []string{"project_path", "backup_id"},
	},
	"module_release_info": {
		Description: "Get maintenance status and curated release list for a contrib module",
		Properties: map[string]jsonSchemaProperty{
			"module_machine_name": {Type: "string", Description: "Module machine name"},
			"core_version":        {Type: "string", Description: "Drupal core major to filter by, e.g. 11"},
		},
		Required: []string{"module_machine_name"},
	},
	"session_open": {
		Description: "Bind an agent session to the canonical root of a Drupal project for the rest of the server process. Every mutating tool call needs a session bound to a matching root, or it is forced into dry-run (where supported) or refused",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"pipeline_status": {
		Description: "Read-only summary of the project's mutation ledger: per-tool call counts, total mutations, and remaining mutation-cap headroom",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
		},
		Required: []string{"project_path"},
	},
	"operation_reconcile": {
		Description: "Resolve an unknown mutation only after verifying an observable filesystem artifact",
		Properties: map[string]jsonSchemaProperty{
			"project_path":  {Type: "string", Description: "Absolute path to the Drupal project"},
			"request_id":    {Type: "string", Description: "Request ID of the unknown operation"},
			"evidence_path": {Type: "string", Description: "Project-relative path to the observed artifact"},
		},
		Required: []string{"project_path", "request_id", "evidence_path"},
	},
	"checkpoint_commit": {
		Description: "Commit only the exact current diff previously approved by independent validation evidence",
		Properties: map[string]jsonSchemaProperty{
			"project_path":    {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":          {Type: "string", Description: "Persisted upgrade run authorizing this checkpoint"},
			"commit_strategy": {Type: "string", Description: "Must exactly match the run strategy: none, single, or per-fix"},
			"scope":           {Type: "array", Description: "Must exactly match the run scope"},
			"paths":           {Type: "array", Description: "Complete set of diff paths reviewed by validation"},
			"validation_hash": {Type: "string", Description: "Independent validation evidence hash"},
			"target":          {Type: "string", Description: "Validated target bound to the evidence"},
			"commit_message":  {Type: "string", Description: "Optional conventional commit message"},
		},
		Required: []string{"project_path", "run_id", "commit_strategy", "scope", "paths", "validation_hash", "target"},
	},
	"checkpoint_execute": {
		Description: "Execute and persist a deterministic operational checkpoint: backup, package update, database updates, cache rebuild, status, independent validation, and managed configuration export. It never publishes a commit; checkpoint_commit remains the only publication boundary",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Persisted run ID authorizing this checkpoint"},
			"phase":        {Type: "string", Description: "Active run phase: custom_theme, contrib_patch, contrib_minor, contrib_major, core_loop, or cleanup"},
			"target_major": {Type: "integer", Description: "Must exactly match the run target major"},
			"targets":      {Type: "array", Description: "Composer/package targets; contrib_major accepts exactly one"},
			"paths":        {Type: "array", Description: "Complete candidate paths whose identity is recomputed after validation"},
			"resume":       {Type: "boolean", Description: "Explicitly retry only a previously unavailable step; requires a fresh request_id"},
		},
		Required: []string{"project_path", "run_id", "phase", "target_major", "targets", "paths"},
	},
	"run_create": {
		Description: "Create the persisted upgrade run authority for a canonical Drupal project root",
		Properties: map[string]jsonSchemaProperty{
			"project_path":    {Type: "string", Description: "Absolute path to the Drupal project"},
			"target_major":    {Type: "integer", Description: "Target Drupal major version"},
			"commit_strategy": {Type: "string", Description: "Commit strategy selected for this run"},
			"scope":           {Type: "array", Description: "Selected upgrade scope"},
		},
		Required: []string{"project_path", "target_major", "commit_strategy", "scope"},
	},
	"run_status": {
		Description: "Read the persisted run phase, allowed actions, evidence, and recovery state",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Run ID (defaults to the project's active run)"},
		},
		Required: []string{"project_path"},
	},
	"run_record": {
		Description: "Append sanitized checkpoint evidence and advance only through the persisted allowed action",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Persisted run ID"},
			"action":       {Type: "string", Description: "One action returned by run_status"},
			"evidence":     {Type: "object", Description: "Structured evidence; validation records must include validation_hash, candidate_hash, paths, and target; raw payload is hash-only at rest"},
		},
		Required: []string{"project_path", "run_id", "action", "evidence"},
	},
	"run_confirm": {
		Description: "Record explicit confirmation for a persisted run action",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Persisted run ID"},
			"action":       {Type: "string", Description: "Confirmation action"},
		},
		Required: []string{"project_path", "run_id", "action"},
	},
	"run_block": {
		Description: "Block a run with a persisted reason and explicit recovery target",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Persisted run ID"},
			"reason":       {Type: "string", Description: "Safe explanation of the blocker"},
			"target":       {Type: "string", Description: "Optional recovery target"},
		},
		Required: []string{"project_path", "run_id", "reason"},
	},
	"run_abandon": {
		Description: "End a persisted run without deleting its backups or evidence",
		Properties: map[string]jsonSchemaProperty{
			"project_path": {Type: "string", Description: "Absolute path to the Drupal project"},
			"run_id":       {Type: "string", Description: "Persisted run ID"},
			"reason":       {Type: "string", Description: "Safe reason for abandonment"},
		},
		Required: []string{"project_path", "run_id", "reason"},
	},
}

// ToolSpecs returns a deterministic snapshot of the tool catalog. Returning
// values prevents callers from mutating the registry that tools/list uses.
func ToolSpecs() []ToolSpec {
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}
	sort.Strings(names)

	specs := make([]ToolSpec, 0, len(names))
	for _, name := range names {
		spec := toolRegistry[name]
		spec.Name = name
		specs = append(specs, spec)
	}
	return specs
}

// ToolSpecByName returns the descriptor for a registered MCP tool.
func ToolSpecByName(name string) (ToolSpec, bool) {
	spec, ok := toolRegistry[name]
	if !ok {
		return ToolSpec{}, false
	}
	spec.Name = name
	return spec, true
}

func init() {
	for name, spec := range toolRegistry {
		spec.Name = name
		spec.Effect = EffectReadOnly
		spec.Timeout = 5 * time.Minute
		spec.Role = "validator"
		spec.Stub = !strings.HasPrefix(name, "test_backup_")
		spec.SessionPolicy = SessionPolicyNone
		toolRegistry[name] = spec
	}

	for _, name := range []string{
		"prepare_upgrade_status", "autofix", "apply_patch", "create_patch",
		"composer_require", "drush_exec", "patch_rollback", "core_upgrade_apply",
		"cleanup", "custom_compat_fix", "contrib_allow_lenient", "contrib_compat_patch", "generate_report",
		"test_backup_create", "test_backup_restore", "test_backup_delete", "checkpoint_commit",
		"checkpoint_execute",
	} {
		spec := toolRegistry[name]
		spec.Effect = EffectMutating
		spec.Role = "executor"
		spec.Preconditions = []string{"session", "backup", "mutation_cap"}
		spec.SessionPolicy = SessionPolicyRefuse
		spec.RequiresBackup = true
		spec.Properties["request_id"] = jsonSchemaProperty{Type: "string", Description: "Stable client-generated ID used to deduplicate this mutation"}
		spec.Required = append(spec.Required, "request_id")
		toolRegistry[name] = spec
	}
	for _, name := range []string{
		"prepare_upgrade_status", "autofix", "apply_patch", "create_patch",
		"composer_require", "drush_exec", "patch_rollback", "core_upgrade_apply",
		"cleanup", "custom_compat_fix", "contrib_allow_lenient", "contrib_compat_patch", "generate_report",
		"test_backup_create", "test_backup_restore", "test_backup_delete", "checkpoint_commit",
		"checkpoint_execute",
	} {
		spec := toolRegistry[name]
		spec.RequiresRun = true
		spec.Properties["run_id"] = jsonSchemaProperty{Type: "string", Description: "Persisted upgrade run ID authorizing this mutation"}
		spec.Required = append(spec.Required, "run_id")
		toolRegistry[name] = spec
	}
	// Reconciliation changes only the authoritative operation ledger. Its
	// request_id identifies the unknown operation being resolved, so it must
	// not be recursively idempotency-wrapped as a new domain mutation.
	spec := toolRegistry["operation_reconcile"]
	spec.Effect = EffectMutating
	spec.Role = "reconciler"
	spec.ReconcilesOperation = true
	spec.RequiresRun = true
	spec.Properties["run_id"] = jsonSchemaProperty{Type: "string", Description: "Persisted upgrade run ID authorizing this reconciliation"}
	spec.Required = append(spec.Required, "run_id")
	toolRegistry["operation_reconcile"] = spec
	for _, name := range []string{"run_create", "run_status", "run_record", "run_confirm", "run_block", "run_abandon"} {
		spec := toolRegistry[name]
		spec.Effect = EffectWorkflow
		spec.Role = "workflow_authority"
		toolRegistry[name] = spec
	}
	for _, name := range []string{"core_upgrade_apply", "contrib_compat_patch", "contrib_allow_lenient", "custom_compat_fix"} {
		spec := toolRegistry[name]
		spec.SessionPolicy = SessionPolicyForceDryRun
		toolRegistry[name] = spec
	}
	for name, timeout := range map[string]time.Duration{
		"composer_require":   10 * time.Minute,
		"core_upgrade_apply": 15 * time.Minute,
		"upgrade_scan":       10 * time.Minute,
	} {
		spec := toolRegistry[name]
		spec.Timeout = timeout
		toolRegistry[name] = spec
	}
	for _, name := range []string{"scan", "upgrade_scan", "validate"} {
		spec := toolRegistry[name]
		spec.RetryEligible = true
		toolRegistry[name] = spec
	}

	// The first backup establishes the prerequisite for later mutations, so it
	// must be auditable but cannot require a pre-existing backup.
	spec = toolRegistry["test_backup_create"]
	spec.Preconditions = []string{"session", "mutation_cap"}
	spec.RequiresBackup = false
	toolRegistry["test_backup_create"] = spec
	// checkpoint_execute creates and binds its own fresh backup as the first
	// persisted step, so requiring a prior backup would make the transaction
	// impossible to start and encourage callers to reuse a stale one.
	spec = toolRegistry["checkpoint_execute"]
	spec.Preconditions = []string{"session", "mutation_cap"}
	spec.RequiresBackup = false
	toolRegistry["checkpoint_execute"] = spec
}

// NewServer creates a new MCP server writing to out.
func NewServer(out io.Writer, version string) *Server {
	return &Server{
		out:     out,
		tools:   defaultTools(),
		version: version,
	}
}

// ToolCount returns the number of registered tool handlers.
func (s *Server) ToolCount() int {
	return len(s.tools)
}

// ToolNames returns the sorted list of registered tool names.
func (s *Server) ToolNames() []string {
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RegisterTool overrides or adds a tool handler by name.
func (s *Server) RegisterTool(name string, handler ToolHandler) {
	s.tools[name] = handler
}

// CallTool invokes the registered handler for name directly, bypassing the
// JSON-RPC transport. It is the same lookup handleToolCall uses, exposed so
// callers (chiefly tests exercising registration-time middleware such as
// internal/app's guard wrapping) can dispatch a tool call without spinning
// up a stdio round trip.
func (s *Server) CallTool(name string, args json.RawMessage) (json.RawMessage, error) {
	handler, ok := s.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool not found: %s", name)
	}
	return handler(args)
}

// Run starts the server, reading from stdin and writing to stdout.
func (s *Server) Run() error {
	return s.run(os.Stdin)
}

func (s *Server) run(in io.Reader) error {
	// bufio.Scanner is terminal once it reports an error: a further Scan()
	// call keeps returning false. An oversized line must not end the whole
	// stdio session, so on bufio.ErrTooLong we report a parse error for that
	// one request and rebuild a fresh scanner over the same underlying
	// reader to keep serving whatever comes after it.
	for {
		scanner := bufio.NewScanner(in)
		scanner.Buffer(make([]byte, 0, scannerStartBuf), scannerMaxLine)

		for scanner.Scan() {
			line := scanner.Bytes()
			if err := s.handleRaw(line); err != nil {
				return err
			}
		}

		err := scanner.Err()
		if err == nil {
			return nil
		}
		if errors.Is(err, bufio.ErrTooLong) {
			if sendErr := s.sendError(nil, -32700, "Parse error: request exceeds maximum line size"); sendErr != nil {
				return sendErr
			}
			continue
		}
		return err
	}
}

func (s *Server) handleRaw(data []byte) error {
	var req JSONRPCRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return s.sendError(nil, -32700, "Parse error")
	}
	return s.handleRequest(req)
}

func (s *Server) handleRequest(req JSONRPCRequest) error {
	// A JSON-RPC request with no "id" field is a notification. Per the
	// JSON-RPC 2.0 spec the server MUST NOT write any response for it — not
	// a result, and not an error — because the client has signaled it is
	// not listening for a reply.
	if req.ID == nil {
		return s.handleNotification(req)
	}

	switch req.Method {
	case "initialize":
		result := fmt.Sprintf(`{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"drup","version":"%s"}}`, s.version)
		return s.sendResult(req.ID, json.RawMessage(result))
	case "tools/list":
		return s.handleListTools(req.ID)
	case "tools/call":
		return s.handleToolCall(req.ID, req.Params)
	default:
		return s.sendError(req.ID, -32601, "Method not found")
	}
}

// handleNotification processes a JSON-RPC notification (a request whose
// "id" field is absent or null). None of the methods this server currently
// recognizes (e.g. notifications/initialized) carry state that needs
// updating on receipt, so there is nothing to execute beyond declining to
// write a response.
func (s *Server) handleNotification(req JSONRPCRequest) error {
	return nil
}

func (s *Server) handleListTools(id interface{}) error {
	// Go map iteration order is randomized. Sorting by name here keeps
	// tools/list deterministic across repeated calls in the same server run
	// — clients and tests that diff two responses would otherwise see
	// spurious reordering.
	names := make([]string, 0, len(s.tools))
	for name := range s.tools {
		names = append(names, name)
	}
	sort.Strings(names)

	tools := []map[string]interface{}{}
	for _, name := range names {
		// Look up schema from registry
		schema, hasSchema := toolRegistry[name]

		tool := map[string]interface{}{
			"name": name,
		}

		if hasSchema {
			tool["description"] = schema.Description

			// Build properties map
			properties := make(map[string]interface{})
			for propName, propDef := range schema.Properties {
				properties[propName] = map[string]interface{}{
					"type":        propDef.Type,
					"description": propDef.Description,
				}
			}

			tool["inputSchema"] = map[string]interface{}{
				"type":       "object",
				"properties": properties,
				"required":   schema.Required,
			}
		} else {
			// Fallback for tools not in registry
			tool["description"] = fmt.Sprintf("Tool: %s", name)
			tool["inputSchema"] = map[string]interface{}{
				"type": "object",
			}
		}

		tools = append(tools, tool)
	}

	result, _ := json.Marshal(map[string]interface{}{"tools": tools})
	return s.sendResult(id, result)
}

// retryBaseDelay is the base delay for exponential backoff in retryLoop.
// Tests override this to 1ms to avoid slow test runs.
var retryBaseDelay = 1 * time.Second

// isTransientError reports whether err is a transient transport error
// that should be retried (timeout, connection refused, etc.).
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, pattern := range []string{
		"context deadline exceeded",
		"connection refused",
		"i/o timeout",
		"broken pipe",
		"no such host",
	} {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// retryLoop calls handler with exponential backoff on transient errors.
// Max 3 attempts (2 retries). Non-transient errors fail immediately.
func (s *Server) retryLoop(toolName string, handler ToolHandler, args json.RawMessage) (json.RawMessage, error) {
	const maxAttempts = 3

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := handler(args)
		if err == nil {
			return result, nil
		}
		if !isTransientError(err) {
			return nil, err // non-transient: fail immediately
		}
		lastErr = err
		if attempt < maxAttempts {
			metrics.Default().RecordRetry()
			delay := retryBaseDelay * time.Duration(1<<(attempt-1))
			time.Sleep(delay)
		}
	}
	// All attempts exhausted.
	return nil, fmt.Errorf("%v (after %d attempts)", lastErr, maxAttempts)
}

func (s *Server) handleToolCall(id interface{}, params json.RawMessage) error {
	var p ToolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		return s.sendError(id, -32602, "Invalid params")
	}

	handler, ok := s.tools[p.Name]
	if !ok {
		return s.sendError(id, -32601, fmt.Sprintf("Tool not found: %s", p.Name))
	}

	var result json.RawMessage
	var err error
	if spec, known := ToolSpecByName(p.Name); known && spec.RetryEligible {
		result, err = s.retryLoop(p.Name, handler, p.Arguments)
	} else {
		result, err = handler(p.Arguments)
	}

	// Wrap ALL tool outcomes (success and error) in a uniform envelope.
	// Tool errors become {status:"fail"} in the result channel, NOT JSON-RPC errors.
	envelope := wrapInEnvelope(p.Name, result, err)
	envelopeJSON, marshalErr := json.Marshal(envelope)
	if marshalErr != nil {
		// Envelope marshal failure is a server bug, not a tool failure.
		return s.sendError(id, -32603, fmt.Sprintf("envelope marshal: %v", marshalErr))
	}
	// MCP clients consume tool output from content[]. Keep the project-specific
	// envelope at the result level for existing agents, while also exposing it
	// as standard MCP text content.
	toolResult := struct {
		Content []ContentBlock `json:"content"`
		Envelope
	}{
		Content:  []ContentBlock{{Type: "text", Text: string(envelopeJSON)}},
		Envelope: envelope,
	}
	toolResultJSON, marshalErr := json.Marshal(toolResult)
	if marshalErr != nil {
		return s.sendError(id, -32603, fmt.Sprintf("tool result marshal: %v", marshalErr))
	}
	return s.sendResult(id, toolResultJSON)
}

// ContentBlock is the standard MCP representation for tool output.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// Envelope wraps every MCP tool response with a uniform status signal.
type Envelope struct {
	Status         string          `json:"status"`            // "pass" | "fail" | "unknown"
	Summary        string          `json:"summary"`           // human-readable one-liner
	Payload        json.RawMessage `json:"payload,omitempty"` // tool-specific response (only on pass)
	OperationState string          `json:"operation_state,omitempty"`
}

// wrapInEnvelope builds an Envelope from a handler outcome.
func wrapInEnvelope(toolName string, result json.RawMessage, handlerErr error) Envelope {
	if handlerErr != nil {
		if stateErr, ok := handlerErr.(interface{ OperationState() string }); ok && stateErr.OperationState() == "unknown" {
			return Envelope{Status: "unknown", Summary: handlerErr.Error(), OperationState: stateErr.OperationState()}
		}
		return Envelope{
			Status:  "fail",
			Summary: handlerErr.Error(),
		}
	}
	if payloadHasSuccessFalse(result) {
		return Envelope{Status: "fail", Summary: deriveSummary(toolName, result), Payload: result}
	}
	return Envelope{
		Status:  "pass",
		Summary: deriveSummary(toolName, result),
		Payload: result,
	}
}

func payloadHasSuccessFalse(payload json.RawMessage) bool {
	var fields struct {
		Success *bool `json:"success"`
	}
	return json.Unmarshal(payload, &fields) == nil && fields.Success != nil && !*fields.Success
}

// deriveSummary extracts a one-line summary from the tool payload.
func deriveSummary(toolName string, payload json.RawMessage) string {
	var fields map[string]interface{}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return fmt.Sprintf("Tool %s completed", toolName)
	}

	// Check for total_errors (scan-like tools).
	if te, ok := fields["total_errors"]; ok {
		return fmt.Sprintf("Scan complete: %v errors", te)
	}
	// Check for success boolean (drush_exec, composer_require, etc.).
	if s, ok := fields["success"]; ok {
		if b, ok := s.(bool); ok && b {
			return fmt.Sprintf("Tool %s succeeded", toolName)
		}
		return fmt.Sprintf("Tool %s failed", toolName)
	}
	// Check for summary string (some tools already provide one).
	if sum, ok := fields["summary"]; ok {
		if str, ok := sum.(string); ok {
			return str
		}
	}
	return fmt.Sprintf("Tool %s completed", toolName)
}

func (s *Server) sendResult(id interface{}, result json.RawMessage) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	return s.writeResponse(resp)
}

func (s *Server) sendError(id interface{}, code int, message string) error {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &JSONRPCError{
			Code:    code,
			Message: message,
		},
	}
	return s.writeResponse(resp)
}

func (s *Server) writeResponse(resp JSONRPCResponse) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(s.out, string(data))
	return err
}
