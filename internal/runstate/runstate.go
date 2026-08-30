// Package runstate owns the durable authority for an upgrade run. It keeps
// workflow decisions out of prompts and refuses mutations that do not belong
// to the persisted project/phase currently being executed.
package runstate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 1

var (
	ErrNotFound           = errors.New("run not found")
	ErrActiveRunExists    = errors.New("an active run already exists for this project root")
	ErrInvalidTransition  = errors.New("invalid run transition")
	ErrRootMismatch       = errors.New("run does not belong to this project root")
	ErrMutationNotAllowed = errors.New("mutation is not allowed by the active run")
	ErrCheckpointDenied   = errors.New("checkpoint commit is not authorized")
)

// CommitStrategy controls when a run may publish its already-validated
// working-tree changes. The empty value is intentionally normalized to none:
// an older run must never gain publication authority merely by being read by a
// newer binary.
type CommitStrategy string

const (
	CommitStrategyNone   CommitStrategy = "none"
	CommitStrategySingle CommitStrategy = "single"
	CommitStrategyPerFix CommitStrategy = "per-fix"
)

// Phase names the authoritative workflow checkpoint.
type Phase string

const (
	PhaseGitSafety     Phase = "git_safety"
	PhaseEnvironment   Phase = "environment"
	PhaseTooling       Phase = "tooling"
	PhaseInitialBackup Phase = "initial_backup"
	PhaseBaseline      Phase = "baseline"
	PhaseCustomTheme   Phase = "custom_theme"
	PhaseContribPatch  Phase = "contrib_patch"
	PhaseContribMinor  Phase = "contrib_minor"
	PhaseContribMajor  Phase = "contrib_major"
	PhaseCoreLoop      Phase = "core_loop"
	PhaseCleanup       Phase = "cleanup"
	PhaseReport        Phase = "report"
	PhaseCompleted     Phase = "completed"
)

// Status describes whether a run may advance.
type Status string

const (
	StatusActive    Status = "active"
	StatusBlocked   Status = "blocked"
	StatusCompleted Status = "completed"
	StatusAbandoned Status = "abandoned"
)

// Action is a Go-owned next step. Callers must use an action returned by the
// stored run rather than derive their own transition from conversational state.
type Action string

const (
	ActionRecordGitSafety     Action = "record_git_safety"
	ActionRecordEnvironment   Action = "record_environment"
	ActionRecordTooling       Action = "record_tooling"
	ActionRecordInitialBackup Action = "record_initial_backup"
	ActionRecordBaseline      Action = "record_baseline"
	ActionRecordCustomTheme   Action = "record_custom_theme"
	ActionRecordContribPatch  Action = "record_contrib_patch"
	ActionRecordContribMinor  Action = "record_contrib_minor"
	ActionRecordContribMajor  Action = "record_contrib_major"
	ActionRecordCoreLoop      Action = "record_core_loop"
	ActionRecordCleanup       Action = "record_cleanup"
	ActionRecordReport        Action = "record_report"
	ActionResolveBlock        Action = "resolve_block"
	ActionConfirmCoreUpgrade  Action = "core_upgrade"
	ActionConfirmRestore      Action = "restore"
)

// Evidence is append-only and stores only a safe summary plus a content hash;
// raw stdout and secret-bearing payloads are deliberately not persisted.
type Evidence struct {
	Phase          Phase     `json:"phase"`
	Action         Action    `json:"action"`
	Kind           string    `json:"kind"`
	Summary        string    `json:"summary"`
	PayloadHash    string    `json:"payload_hash,omitempty"`
	ValidationHash string    `json:"validation_hash,omitempty"`
	CandidateHash  string    `json:"candidate_hash,omitempty"`
	Paths          []string  `json:"paths,omitempty"`
	Target         string    `json:"target,omitempty"`
	RecordedAt     time.Time `json:"recorded_at"`
}

type PendingHuman struct {
	Reason    string    `json:"reason"`
	Target    string    `json:"target,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Run is the on-disk schema. AllowedActions is persisted, then read back as
// data, so a server restart cannot reconstruct authority from a prompt.
type Run struct {
	Version        int            `json:"version"`
	ID             string         `json:"id"`
	Root           string         `json:"project_path"`
	TargetMajor    int            `json:"target_major"`
	CommitStrategy CommitStrategy `json:"commit_strategy"`
	Scope          []string       `json:"scope,omitempty"`
	Status         Status         `json:"status"`
	Phase          Phase          `json:"phase"`
	AllowedActions []Action       `json:"allowed_actions"`
	Evidence       []Evidence     `json:"evidence"`
	PendingHuman   []PendingHuman `json:"pending_human,omitempty"`
	Confirmations  []Action       `json:"confirmations,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type CreateInput struct {
	ID             string
	TargetMajor    int
	CommitStrategy CommitStrategy
	Scope          []string
}

type RecordInput struct {
	Action  Action
	Kind    string
	Summary string
	Payload json.RawMessage
	// ValidationHash, CandidateHash and Paths bind independently observed
	// validation to the exact candidate a later checkpoint may publish.
	ValidationHash string
	CandidateHash  string
	Paths          []string
	Target         string
}

// CheckpointInput is the immutable binding checked before a commit is
// exposed. The application layer computes CandidateHash from the current git
// diff; runstate only compares it with persisted validator evidence.
type CheckpointInput struct {
	Strategy       CommitStrategy
	Scope          []string
	Paths          []string
	ValidationHash string
	CandidateHash  string
	Target         string
}

// Store is project-root scoped. The caller must pass the canonical root from
// session.CanonicalRoot; this package persists and compares that exact value.
type Store struct{ root string }

func NewStore(root string) *Store { return &Store{root: root} }

func (s *Store) Root() string          { return s.root }
func (s *Store) dir() string           { return filepath.Join(s.root, ".drup", "runs") }
func (s *Store) path(id string) string { return filepath.Join(s.dir(), id+".json") }

var storeMu sync.Mutex

func (s *Store) Create(input CreateInput) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := validateCreate(input); err != nil {
		return Run{}, err
	}
	if err := s.ensureDir(); err != nil {
		return Run{}, err
	}
	if _, err := os.Stat(s.path(input.ID)); err == nil {
		return Run{}, fmt.Errorf("%w: %s", ErrActiveRunExists, input.ID)
	} else if !os.IsNotExist(err) {
		return Run{}, err
	}
	if active, err := s.activeLocked(); err != nil {
		return Run{}, err
	} else if active != "" {
		return Run{}, fmt.Errorf("%w: %s", ErrActiveRunExists, active)
	}
	now := time.Now().UTC()
	run := Run{Version: schemaVersion, ID: input.ID, Root: s.root, TargetMajor: input.TargetMajor, CommitStrategy: normalizedCommitStrategy(input.CommitStrategy), Scope: append([]string(nil), input.Scope...), Status: StatusActive, Phase: PhaseGitSafety, Evidence: []Evidence{}, CreatedAt: now, UpdatedAt: now}
	run.AllowedActions = actionsFor(run)
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) Get(id string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	return s.getLocked(id)
}

func (s *Store) Active() (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	id, err := s.activeLocked()
	if err != nil {
		return Run{}, err
	}
	if id == "" {
		return Run{}, ErrNotFound
	}
	return s.getLocked(id)
}

func (s *Store) Record(id string, input RecordInput) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if !containsAction(run.AllowedActions, input.Action) {
		return Run{}, fmt.Errorf("%w: expected one of %v", ErrInvalidTransition, run.AllowedActions)
	}
	if run.Status == StatusBlocked && input.Action == ActionResolveBlock {
		if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Summary) == "" {
			return Run{}, fmt.Errorf("evidence kind and summary are required")
		}
		run.Evidence = append(run.Evidence, evidenceFromInput(run.Phase, input.Action, input))
		run.Status = StatusActive
		run.UpdatedAt = time.Now().UTC()
		run.AllowedActions = actionsFor(run)
		if err := s.writeLocked(run); err != nil {
			return Run{}, err
		}
		return run, nil
	}
	if run.Status != StatusActive {
		return Run{}, fmt.Errorf("%w: run is %s", ErrInvalidTransition, run.Status)
	}
	next, ok := nextPhase(input.Action)
	if !ok {
		return Run{}, fmt.Errorf("%w: %s does not record a phase", ErrInvalidTransition, input.Action)
	}
	if strings.TrimSpace(input.Kind) == "" || strings.TrimSpace(input.Summary) == "" {
		return Run{}, fmt.Errorf("evidence kind and summary are required")
	}
	run.Evidence = append(run.Evidence, evidenceFromInput(run.Phase, input.Action, input))
	run.Phase = next
	if next == PhaseCompleted {
		run.Status = StatusCompleted
	}
	run.UpdatedAt = time.Now().UTC()
	run.AllowedActions = actionsFor(run)
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// AuthorizeCheckpoint proves that a proposed commit still represents the
// independently validated candidate. It never performs a git effect.
func (s *Store) AuthorizeCheckpoint(id string, input CheckpointInput) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if err := authorizeCheckpoint(run, input); err != nil {
		return Run{}, err
	}
	return run, nil
}

// RecordCheckpoint persists the successful publication receipt. Callers must
// invoke it immediately after the scoped git commit and treat a persistence
// failure as ambiguous rather than retrying the commit blindly.
func (s *Store) RecordCheckpoint(id string, input CheckpointInput, commitHash string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if err := authorizeCheckpoint(run, input); err != nil {
		return Run{}, err
	}
	run.Evidence = append(run.Evidence, Evidence{
		Phase: run.Phase, Action: "checkpoint_commit", Kind: "checkpoint_commit",
		Summary: "validated checkpoint commit", PayloadHash: hashPayload(json.RawMessage(fmt.Sprintf(`{"commit_hash":%q}`, commitHash))),
		ValidationHash: input.ValidationHash, CandidateHash: input.CandidateHash,
		Paths: append([]string(nil), input.Paths...), Target: input.Target, RecordedAt: time.Now().UTC(),
	})
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func authorizeCheckpoint(run Run, input CheckpointInput) error {
	if run.Status != StatusActive {
		return fmt.Errorf("%w: run is %s", ErrCheckpointDenied, run.Status)
	}
	strategy := normalizedCommitStrategy(input.Strategy)
	if strategy != run.CommitStrategy {
		return fmt.Errorf("%w: strategy %q does not match run strategy %q", ErrCheckpointDenied, strategy, run.CommitStrategy)
	}
	if strategy == CommitStrategyNone {
		return nil
	}
	if strategy == CommitStrategySingle && run.Phase != PhaseReport {
		return fmt.Errorf("%w: single strategy may commit only at final report", ErrCheckpointDenied)
	}
	if strings.TrimSpace(input.ValidationHash) == "" || strings.TrimSpace(input.CandidateHash) == "" || len(input.Paths) == 0 {
		return fmt.Errorf("%w: validation_hash, candidate_hash and paths are required", ErrCheckpointDenied)
	}
	if !sameStrings(input.Scope, run.Scope) {
		return fmt.Errorf("%w: checkpoint scope differs from run scope", ErrCheckpointDenied)
	}
	for i := len(run.Evidence) - 1; i >= 0; i-- {
		evidence := run.Evidence[i]
		if evidence.Kind != "validation" || evidence.ValidationHash != input.ValidationHash {
			continue
		}
		if evidence.CandidateHash != input.CandidateHash || !sameStrings(evidence.Paths, input.Paths) || evidence.Target != input.Target {
			return fmt.Errorf("%w: validation evidence is stale or belongs to another target/path set", ErrCheckpointDenied)
		}
		return nil
	}
	return fmt.Errorf("%w: no matching independent validation evidence", ErrCheckpointDenied)
}

func (s *Store) Confirm(id string, action Action) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusActive || (action != ActionConfirmCoreUpgrade && action != ActionConfirmRestore) {
		return Run{}, fmt.Errorf("%w: confirmation %s", ErrInvalidTransition, action)
	}
	if action == ActionConfirmCoreUpgrade && run.Phase != PhaseCoreLoop {
		return Run{}, fmt.Errorf("%w: core upgrade requires %s", ErrInvalidTransition, PhaseCoreLoop)
	}
	run.Confirmations = appendUniqueAction(run.Confirmations, action)
	run.Evidence = append(run.Evidence, Evidence{Phase: run.Phase, Action: action, Kind: "confirmation", Summary: string(action), RecordedAt: time.Now().UTC()})
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) Block(id, reason, target string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusActive || strings.TrimSpace(reason) == "" {
		return Run{}, fmt.Errorf("%w: block requires an active run and reason", ErrInvalidTransition)
	}
	run.Status = StatusBlocked
	run.PendingHuman = append(run.PendingHuman, PendingHuman{Reason: sanitizeSummary(reason), Target: sanitizeSummary(target), CreatedAt: time.Now().UTC()})
	run.UpdatedAt = time.Now().UTC()
	run.AllowedActions = actionsFor(run)
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) ResolveBlock(id string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusBlocked {
		return Run{}, fmt.Errorf("%w: run is not blocked", ErrInvalidTransition)
	}
	run.Status = StatusActive
	run.UpdatedAt = time.Now().UTC()
	run.AllowedActions = actionsFor(run)
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) Abandon(id, reason string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if (run.Status != StatusActive && run.Status != StatusBlocked) || strings.TrimSpace(reason) == "" {
		return Run{}, fmt.Errorf("%w: abandon requires a non-terminal run and reason", ErrInvalidTransition)
	}
	run.Status = StatusAbandoned
	run.Evidence = append(run.Evidence, Evidence{Phase: run.Phase, Action: "abandon", Kind: "abandon", Summary: sanitizeSummary(reason), RecordedAt: time.Now().UTC()})
	run.UpdatedAt = time.Now().UTC()
	run.AllowedActions = nil
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// ValidateMutation proves a run belongs to the given canonical root and is
// currently permitted to execute tool. It is deliberately read-only.
func (s *Store) ValidateMutation(id, root, tool string) (Run, error) {
	run, err := s.Get(id)
	if err != nil {
		return Run{}, err
	}
	if run.Root != root {
		return Run{}, ErrRootMismatch
	}
	if run.Status != StatusActive {
		return Run{}, fmt.Errorf("%w: run is %s", ErrMutationNotAllowed, run.Status)
	}
	if !toolAllowed(run, tool) {
		return Run{}, fmt.Errorf("%w: %s is not allowed in %s; allowed actions are %v", ErrMutationNotAllowed, tool, run.Phase, run.AllowedActions)
	}
	return run, nil
}

func (s *Store) getLocked(id string) (Run, error) {
	if id == "" {
		return Run{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(id))
	if os.IsNotExist(err) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, fmt.Errorf("read run: %w", err)
	}
	var run Run
	if err := json.Unmarshal(data, &run); err != nil {
		return Run{}, fmt.Errorf("decode run: %w", err)
	}
	if run.Version != schemaVersion || run.ID != id || run.Root != s.root {
		return Run{}, fmt.Errorf("invalid or incompatible run record")
	}
	return run, nil
}

func (s *Store) activeLocked() (string, error) {
	entries, err := os.ReadDir(s.dir())
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("list runs: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		run, err := s.getLocked(id)
		if err != nil {
			return "", err
		}
		if run.Status == StatusActive || run.Status == StatusBlocked {
			return id, nil
		}
	}
	return "", nil
}

func (s *Store) ensureDir() error { return os.MkdirAll(s.dir(), 0o700) }

func (s *Store) writeLocked(run Run) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	data, err := json.Marshal(run)
	if err != nil {
		return fmt.Errorf("encode run: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir(), ".run.*.tmp")
	if err != nil {
		return fmt.Errorf("create run temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write run temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod run temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync run temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close run temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path(run.ID)); err != nil {
		return fmt.Errorf("replace run: %w", err)
	}
	return nil
}

func validateCreate(input CreateInput) error {
	if strings.TrimSpace(input.ID) == "" || strings.Contains(input.ID, "/") || strings.Contains(input.ID, "..") {
		return fmt.Errorf("run_id must be a safe non-empty identifier")
	}
	if input.TargetMajor < 1 {
		return fmt.Errorf("target_major must be positive")
	}
	strategy := normalizedCommitStrategy(input.CommitStrategy)
	if strategy != CommitStrategyNone && strategy != CommitStrategySingle && strategy != CommitStrategyPerFix {
		return fmt.Errorf("commit_strategy must be none, single, or per-fix")
	}
	return nil
}

func normalizedCommitStrategy(strategy CommitStrategy) CommitStrategy {
	if strategy == "" {
		return CommitStrategyNone
	}
	return strategy
}

func evidenceFromInput(phase Phase, action Action, input RecordInput) Evidence {
	return Evidence{
		Phase: phase, Action: action, Kind: input.Kind, Summary: sanitizeSummary(input.Summary),
		PayloadHash: hashPayload(input.Payload), ValidationHash: input.ValidationHash,
		CandidateHash: input.CandidateHash, Paths: append([]string(nil), input.Paths...),
		Target: input.Target, RecordedAt: time.Now().UTC(),
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func actionsFor(run Run) []Action {
	if run.Status == StatusBlocked {
		return []Action{ActionResolveBlock}
	}
	if run.Status != StatusActive {
		return nil
	}
	if action, ok := phaseRecordAction[run.Phase]; ok {
		return []Action{action}
	}
	return nil
}

var phaseRecordAction = map[Phase]Action{
	PhaseGitSafety: ActionRecordGitSafety, PhaseEnvironment: ActionRecordEnvironment,
	PhaseTooling: ActionRecordTooling, PhaseInitialBackup: ActionRecordInitialBackup,
	PhaseBaseline: ActionRecordBaseline, PhaseCustomTheme: ActionRecordCustomTheme,
	PhaseContribPatch: ActionRecordContribPatch, PhaseContribMinor: ActionRecordContribMinor,
	PhaseContribMajor: ActionRecordContribMajor, PhaseCoreLoop: ActionRecordCoreLoop,
	PhaseCleanup: ActionRecordCleanup, PhaseReport: ActionRecordReport,
}

var actionNextPhase = map[Action]Phase{
	ActionRecordGitSafety: PhaseEnvironment, ActionRecordEnvironment: PhaseTooling,
	ActionRecordTooling: PhaseInitialBackup, ActionRecordInitialBackup: PhaseBaseline,
	ActionRecordBaseline: PhaseCustomTheme, ActionRecordCustomTheme: PhaseContribPatch,
	ActionRecordContribPatch: PhaseContribMinor, ActionRecordContribMinor: PhaseContribMajor,
	ActionRecordContribMajor: PhaseCoreLoop, ActionRecordCoreLoop: PhaseCleanup,
	ActionRecordCleanup: PhaseReport, ActionRecordReport: PhaseCompleted,
}

func nextPhase(action Action) (Phase, bool) { phase, ok := actionNextPhase[action]; return phase, ok }
func containsAction(actions []Action, wanted Action) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}
func appendUniqueAction(actions []Action, wanted Action) []Action {
	if containsAction(actions, wanted) {
		return actions
	}
	return append(actions, wanted)
}

func toolAllowed(run Run, tool string) bool {
	if tool == "operation_reconcile" {
		return true
	}
	if tool == "test_backup_create" {
		return run.Phase == PhaseInitialBackup
	}
	if tool == "core_upgrade_apply" {
		return run.Phase == PhaseCoreLoop && containsAction(run.Confirmations, ActionConfirmCoreUpgrade)
	}
	if tool == "test_backup_restore" {
		return containsAction(run.Confirmations, ActionConfirmRestore)
	}
	switch tool {
	case "prepare_upgrade_status":
		return run.Phase == PhaseTooling
	case "autofix", "create_patch", "custom_compat_fix":
		return run.Phase == PhaseCustomTheme
	case "apply_patch", "patch_rollback", "contrib_compat_patch":
		return run.Phase == PhaseContribPatch
	case "generate_report":
		return run.Phase == PhaseReport
	case "composer_require", "drush_exec":
		return run.Phase == PhaseTooling || run.Phase == PhaseContribMinor || run.Phase == PhaseContribMajor || run.Phase == PhaseCoreLoop || run.Phase == PhaseCleanup
	case "contrib_allow_lenient":
		return run.Phase == PhaseContribMinor || run.Phase == PhaseContribMajor
	case "cleanup":
		return run.Phase == PhaseCleanup
	default:
		return false
	}
}

func hashPayload(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value interface{}
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func sanitizeSummary(summary string) string {
	summary = strings.TrimSpace(summary)
	for _, word := range []string{"password", "secret", "token", "authorization", "cookie", "stdout", "stderr", "command output"} {
		if strings.Contains(strings.ToLower(summary), word) {
			return "sanitized evidence"
		}
	}
	if strings.ContainsAny(summary, "\r\n") {
		return "sanitized evidence"
	}
	if len(summary) > 512 {
		return summary[:512]
	}
	return summary
}

// SortedRunIDs is a small read-only helper for status tooling and tests.
func (s *Store) SortedRunIDs() ([]string, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	entries, err := os.ReadDir(s.dir())
	if os.IsNotExist(err) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			ids = append(ids, strings.TrimSuffix(entry.Name(), ".json"))
		}
	}
	sort.Strings(ids)
	return ids, nil
}
