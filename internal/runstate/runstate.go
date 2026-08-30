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
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nireneko/drup/internal/backup"
	"github.com/nireneko/drup/internal/inventory"
)

const schemaVersion = 1

var (
	ErrNotFound             = errors.New("run not found")
	ErrActiveRunExists      = errors.New("an active run already exists for this project root")
	ErrInvalidTransition    = errors.New("invalid run transition")
	ErrRootMismatch         = errors.New("run does not belong to this project root")
	ErrMutationNotAllowed   = errors.New("mutation is not allowed by the active run")
	ErrCheckpointDenied     = errors.New("checkpoint commit is not authorized")
	ErrCheckpointPlanDenied = errors.New("checkpoint plan is not authorized")
	ErrInventoryIncomplete  = errors.New("run inventory snapshots are incomplete")
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
	CommitHash     string    `json:"commit_hash,omitempty"`
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
	// CheckpointPlan is the durable, resumable operational boundary for the
	// current phase. It deliberately lives in Run: a second state file would
	// allow a restarted caller to derive authority from stale prompt state.
	CheckpointPlan *CheckpointPlan `json:"checkpoint_plan,omitempty"`
	// CheckpointHistory keeps completed plans observable after the run advances
	// into its next phase. It is append-only control evidence, not a second
	// transition machine.
	CheckpointHistory []CheckpointPlan `json:"checkpoint_history,omitempty"`
	// InventoryBaseline and InventoryFinal are immutable read-only captures used
	// to regenerate reports without re-scanning a changed project.
	InventoryBaseline *inventory.Inventory `json:"inventory_baseline,omitempty"`
	InventoryFinal    *inventory.Inventory `json:"inventory_final,omitempty"`
	// ContribPlan is the sanitized, read-only Composer plan for the current
	// immediate core-major cycle. It is evidence, not transition authority.
	ContribPlan json.RawMessage `json:"contrib_plan,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// CheckpointStepName is a fixed command boundary. Callers cannot invent an
// arbitrary shell step; the executor maps these names to argv-only adapters.
type CheckpointStepName string

const (
	CheckpointStepBackup       CheckpointStepName = "backup"
	CheckpointStepUpdate       CheckpointStepName = "update"
	CheckpointStepDatabase     CheckpointStepName = "database_update"
	CheckpointStepCacheRebuild CheckpointStepName = "cache_rebuild"
	CheckpointStepSiteStatus   CheckpointStepName = "status"
	CheckpointStepValidation   CheckpointStepName = "validation"
	CheckpointStepConfigExport CheckpointStepName = "config_export"
	CheckpointStepSmoke        CheckpointStepName = "smoke"
)

type CheckpointStepStatus string

const (
	CheckpointStepPending     CheckpointStepStatus = "pending"
	CheckpointStepRunning     CheckpointStepStatus = "running"
	CheckpointStepSucceeded   CheckpointStepStatus = "succeeded"
	CheckpointStepFailed      CheckpointStepStatus = "failed"
	CheckpointStepUnavailable CheckpointStepStatus = "unavailable"
)

// CheckpointStepEvidence intentionally excludes command output. Output can
// contain credentials, URLs, and project data; a digest permits correlation
// without making the run record a secret store.
type CheckpointStepEvidence struct {
	CommandHash string `json:"command_hash,omitempty"`
	OutputHash  string `json:"output_hash,omitempty"`
	// CandidateHash is set only by the independent validation step after the
	// candidate has been captured both before and after validation. It lets a
	// restarted executor close the durable crash window without inferring that
	// a previously successful validator still applies.
	CandidateHash string        `json:"candidate_hash,omitempty"`
	ExitCode      int           `json:"exit_code"`
	Duration      time.Duration `json:"duration_ns,omitempty"`
	Paths         []string      `json:"paths,omitempty"`
}

type CheckpointStep struct {
	Name     CheckpointStepName      `json:"name"`
	Status   CheckpointStepStatus    `json:"status"`
	Evidence *CheckpointStepEvidence `json:"evidence,omitempty"`
}

// CheckpointPlan binds an operational checkpoint to one run phase and one
// target-major. A contrib major plan is intentionally cardinality-one.
type CheckpointPlan struct {
	Phase               Phase            `json:"phase"`
	TargetMajor         int              `json:"target_major"`
	Targets             []string         `json:"targets"`
	Paths               []string         `json:"paths"`
	BackupID            string           `json:"backup_id,omitempty"`
	CandidateHash       string           `json:"candidate_hash,omitempty"`
	RequireConfigExport bool             `json:"require_config_export"`
	SmokeCommands       [][]string       `json:"smoke_commands,omitempty"`
	Steps               []CheckpointStep `json:"steps"`
	CompletedAt         time.Time        `json:"completed_at,omitempty"`
}

type CheckpointPlanInput struct {
	Phase               Phase
	TargetMajor         int
	Targets             []string
	Paths               []string
	RequireConfigExport bool
	SmokeCommands       [][]string
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
	if checkpointPhase(run.Phase) {
		if err := authorizeCheckpointProgress(run, input); err != nil {
			return Run{}, err
		}
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

// BeginCheckpointPlan creates the authoritative operational plan or returns
// the same plan after a process restart. A caller may never replace a plan
// with different phase, target, target set, or paths.
func (s *Store) BeginCheckpointPlan(id string, input CheckpointPlanInput) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if err := validateCheckpointPlan(run, input); err != nil {
		return Run{}, err
	}
	plan := checkpointPlanFromInput(input)
	if run.CheckpointPlan != nil {
		if !sameCheckpointIdentity(*run.CheckpointPlan, plan) {
			if run.CheckpointPlan.CompletedAt.IsZero() {
				return Run{}, fmt.Errorf("%w: existing plan identity differs", ErrCheckpointPlanDenied)
			}
			run.CheckpointHistory = append(run.CheckpointHistory, *run.CheckpointPlan)
			run.CheckpointPlan = &plan
			run.UpdatedAt = time.Now().UTC()
			if err := s.writeLocked(run); err != nil {
				return Run{}, err
			}
			return run, nil
		}
		return run, nil
	}
	run.CheckpointPlan = &plan
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// RecordCheckpointStep persists every executor transition. Failed and
// unavailable are terminal states: retrying them blindly would turn an
// ambiguous external outcome into a duplicated mutation.
func (s *Store) RecordCheckpointStep(id string, name CheckpointStepName, status CheckpointStepStatus, evidence *CheckpointStepEvidence) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.CheckpointPlan == nil {
		return Run{}, fmt.Errorf("%w: no active checkpoint plan", ErrCheckpointPlanDenied)
	}
	if status != CheckpointStepRunning && status != CheckpointStepSucceeded && status != CheckpointStepFailed && status != CheckpointStepUnavailable {
		return Run{}, fmt.Errorf("%w: invalid checkpoint step status %q", ErrCheckpointPlanDenied, status)
	}
	for i := range run.CheckpointPlan.Steps {
		step := &run.CheckpointPlan.Steps[i]
		if step.Name != name {
			continue
		}
		for previous := 0; previous < i; previous++ {
			prior := run.CheckpointPlan.Steps[previous]
			if prior.Status != CheckpointStepSucceeded {
				return Run{}, fmt.Errorf("%w: prior checkpoint step %s is %s", ErrCheckpointPlanDenied, prior.Name, prior.Status)
			}
		}
		if step.Status == CheckpointStepSucceeded || step.Status == CheckpointStepFailed || step.Status == CheckpointStepUnavailable {
			return Run{}, fmt.Errorf("%w: checkpoint step %s is terminal (%s)", ErrCheckpointPlanDenied, name, step.Status)
		}
		if status != CheckpointStepRunning && step.Status != CheckpointStepRunning {
			return Run{}, fmt.Errorf("%w: checkpoint step %s must be persisted running first", ErrCheckpointPlanDenied, name)
		}
		step.Status = status
		step.Evidence = cloneCheckpointEvidence(evidence)
		run.UpdatedAt = time.Now().UTC()
		if err := s.writeLocked(run); err != nil {
			return Run{}, err
		}
		return run, nil
	}
	return Run{}, fmt.Errorf("%w: unknown checkpoint step %s", ErrCheckpointPlanDenied, name)
}

// BindCheckpointBackup attaches the observed backup identity to the plan.
// The executor must call it immediately after the backup succeeds, before it
// starts any mutation, so a restart can never use a merely "latest" backup.
func (s *Store) BindCheckpointBackup(id, backupID string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.CheckpointPlan == nil || strings.TrimSpace(backupID) == "" {
		return Run{}, fmt.Errorf("%w: checkpoint plan and backup_id are required", ErrCheckpointPlanDenied)
	}
	if run.CheckpointPlan.BackupID != "" && run.CheckpointPlan.BackupID != backupID {
		return Run{}, fmt.Errorf("%w: backup_id is already bound to this checkpoint", ErrCheckpointPlanDenied)
	}
	run.CheckpointPlan.BackupID = backupID
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// ResumeUnavailableCheckpoint is an explicit recovery boundary. Only steps
// known to be unavailable become pending again; failed and in-flight effects
// remain blocked because retrying them could duplicate a mutation.
func (s *Store) ResumeUnavailableCheckpoint(id string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.CheckpointPlan == nil || !run.CheckpointPlan.CompletedAt.IsZero() {
		return Run{}, fmt.Errorf("%w: no resumable checkpoint plan", ErrCheckpointPlanDenied)
	}
	resumed := false
	for i := range run.CheckpointPlan.Steps {
		step := &run.CheckpointPlan.Steps[i]
		if step.Status == CheckpointStepUnavailable {
			step.Status = CheckpointStepPending
			step.Evidence = nil
			resumed = true
		}
	}
	if !resumed {
		return Run{}, fmt.Errorf("%w: only unavailable checkpoint steps may be resumed", ErrCheckpointPlanDenied)
	}
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

// CompleteCheckpointPlan stores the candidate identity recomputed by the
// executor only after every required independent step completed. It does not
// publish a commit; CheckpointCommit remains the sole publication boundary.
func (s *Store) CompleteCheckpointPlan(id, candidateHash string) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.CheckpointPlan == nil {
		return Run{}, fmt.Errorf("%w: no active checkpoint plan", ErrCheckpointPlanDenied)
	}
	plan := run.CheckpointPlan
	if plan.BackupID == "" {
		return Run{}, fmt.Errorf("%w: backup_id is required", ErrCheckpointPlanDenied)
	}
	if strings.TrimSpace(candidateHash) == "" {
		return Run{}, fmt.Errorf("%w: recomputed candidate hash is required", ErrCheckpointPlanDenied)
	}
	for _, step := range plan.Steps {
		if step.Name == CheckpointStepSmoke && step.Status == CheckpointStepPending {
			continue // smoke is optional when no configured allowlisted command exists.
		}
		if step.Status != CheckpointStepSucceeded {
			return Run{}, fmt.Errorf("%w: required checkpoint step %s is %s", ErrCheckpointPlanDenied, step.Name, step.Status)
		}
		if step.Evidence == nil || !sameStrings(step.Evidence.Paths, plan.Paths) {
			return Run{}, fmt.Errorf("%w: checkpoint step %s is missing deterministic evidence paths", ErrCheckpointPlanDenied, step.Name)
		}
		if step.Name == CheckpointStepValidation && (strings.TrimSpace(step.Evidence.CommandHash) == "" || strings.TrimSpace(step.Evidence.OutputHash) == "") {
			return Run{}, fmt.Errorf("%w: checkpoint step validation is missing independent validation evidence", ErrCheckpointPlanDenied)
		}
		if step.Name == CheckpointStepValidation && strings.TrimSpace(step.Evidence.CandidateHash) == "" {
			return Run{}, fmt.Errorf("%w: checkpoint step validation is missing its candidate identity", ErrCheckpointPlanDenied)
		}
	}
	plan.CandidateHash = candidateHash
	plan.CompletedAt = time.Now().UTC()
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func validateCheckpointPlan(run Run, input CheckpointPlanInput) error {
	if run.Status != StatusActive || input.Phase != run.Phase || !checkpointPhase(input.Phase) {
		return fmt.Errorf("%w: checkpoint phase does not match active run phase", ErrCheckpointPlanDenied)
	}
	if input.TargetMajor != run.TargetMajor || input.TargetMajor < 1 {
		return fmt.Errorf("%w: target major does not match run", ErrCheckpointPlanDenied)
	}
	if len(input.Targets) == 0 || len(input.Paths) == 0 {
		return fmt.Errorf("%w: targets and paths are required", ErrCheckpointPlanDenied)
	}
	if err := validateCheckpointStrings(input.Targets, false); err != nil {
		return err
	}
	if err := validateCheckpointStrings(input.Paths, true); err != nil {
		return err
	}
	if input.Phase == PhaseContribMajor && len(input.Targets) != 1 {
		return fmt.Errorf("%w: contrib major checkpoints require exactly one target", ErrCheckpointPlanDenied)
	}
	return nil
}

func validateCheckpointStrings(values []string, paths bool) error {
	canonical := normalizedCheckpointStrings(values)
	for i, value := range canonical {
		if value == "" {
			return fmt.Errorf("%w: checkpoint values may not be empty", ErrCheckpointPlanDenied)
		}
		if i > 0 && canonical[i-1] == value {
			return fmt.Errorf("%w: duplicate checkpoint value %q", ErrCheckpointPlanDenied, value)
		}
		if paths && (value == ".." || strings.HasPrefix(value, "../") || filepath.IsAbs(value)) {
			return fmt.Errorf("%w: unsafe checkpoint path %q", ErrCheckpointPlanDenied, value)
		}
	}
	return nil
}

func checkpointPhase(phase Phase) bool {
	switch phase {
	case PhaseCustomTheme, PhaseContribPatch, PhaseContribMinor, PhaseContribMajor, PhaseCoreLoop, PhaseCleanup:
		return true
	default:
		return false
	}
}

func checkpointPlanFromInput(input CheckpointPlanInput) CheckpointPlan {
	steps := []CheckpointStep{
		{Name: CheckpointStepBackup, Status: CheckpointStepPending},
		{Name: CheckpointStepDatabase, Status: CheckpointStepPending},
		{Name: CheckpointStepCacheRebuild, Status: CheckpointStepPending},
		{Name: CheckpointStepSiteStatus, Status: CheckpointStepPending},
	}
	if input.Phase == PhaseContribPatch || input.Phase == PhaseContribMinor || input.Phase == PhaseContribMajor {
		steps = append([]CheckpointStep{{Name: CheckpointStepUpdate, Status: CheckpointStepPending}}, steps...)
	}
	if input.RequireConfigExport {
		steps = append(steps, CheckpointStep{Name: CheckpointStepConfigExport, Status: CheckpointStepPending})
	}
	if len(input.SmokeCommands) > 0 {
		steps = append(steps, CheckpointStep{Name: CheckpointStepSmoke, Status: CheckpointStepPending})
	}
	// Config export can mutate the candidate. It must precede validation so
	// validation and the final Git identity both describe the same bytes.
	steps = append(steps, CheckpointStep{Name: CheckpointStepValidation, Status: CheckpointStepPending})
	return CheckpointPlan{Phase: input.Phase, TargetMajor: input.TargetMajor, Targets: normalizedCheckpointStrings(input.Targets), Paths: normalizedCheckpointStrings(input.Paths), RequireConfigExport: input.RequireConfigExport, SmokeCommands: cloneCommandVectors(input.SmokeCommands), Steps: steps}
}

func normalizedCheckpointStrings(values []string) []string {
	result := append([]string(nil), values...)
	for i := range result {
		result[i] = strings.TrimSpace(result[i])
	}
	sort.Strings(result)
	return result
}

func sameCheckpointIdentity(left, right CheckpointPlan) bool {
	return left.Phase == right.Phase && left.TargetMajor == right.TargetMajor && left.RequireConfigExport == right.RequireConfigExport && sameStrings(left.Targets, right.Targets) && sameStrings(left.Paths, right.Paths) && sameCommandVectors(left.SmokeCommands, right.SmokeCommands)
}

func cloneCommandVectors(commands [][]string) [][]string {
	result := make([][]string, len(commands))
	for i := range commands {
		result[i] = append([]string(nil), commands[i]...)
	}
	return result
}

func sameCommandVectors(left, right [][]string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if !sameStrings(left[i], right[i]) {
			return false
		}
	}
	return true
}

func cloneCheckpointEvidence(value *CheckpointStepEvidence) *CheckpointStepEvidence {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Paths = append([]string(nil), value.Paths...)
	return &copy
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
		Paths: append([]string(nil), input.Paths...), Target: input.Target, CommitHash: commitHash, RecordedAt: time.Now().UTC(),
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
	// A no-commit policy remains compatible for phases that Phase 7 does not
	// govern. Once an operational checkpoint exists (or is the active phase),
	// it is still a publication decision and must prove the same validated
	// boundary before reporting an unpublished outcome.
	checkpointRequired := checkpointPhase(run.Phase) || len(checkpointPlans(run)) > 0
	if strategy == CommitStrategyNone && !checkpointRequired {
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
		if !completedCheckpointForEvidence(run, evidence) {
			return fmt.Errorf("%w: completed operational checkpoint does not match validation evidence", ErrCheckpointDenied)
		}
		return nil
	}
	return fmt.Errorf("%w: no matching independent validation evidence", ErrCheckpointDenied)
}

// authorizeCheckpointProgress keeps Phase 7 operational phases from moving
// forward on a conversational claim. The final independent validator must
// bind its evidence to the exact candidate completed by the persisted plan.
func authorizeCheckpointProgress(run Run, input RecordInput) error {
	if input.Kind != "validation" || strings.TrimSpace(input.ValidationHash) == "" || strings.TrimSpace(input.CandidateHash) == "" || len(input.Paths) == 0 || strings.TrimSpace(input.Target) == "" {
		return fmt.Errorf("%w: operational phase progress requires independent validation bound to a completed checkpoint", ErrCheckpointPlanDenied)
	}
	for _, plan := range checkpointPlans(run) {
		if plan.Phase != run.Phase || !checkpointPlanComplete(plan) {
			continue
		}
		if plan.CandidateHash == input.CandidateHash && sameStrings(plan.Paths, input.Paths) && input.Target == strconv.Itoa(plan.TargetMajor) {
			return nil
		}
	}
	return fmt.Errorf("%w: validation does not match the completed checkpoint candidate", ErrCheckpointPlanDenied)
}

func completedCheckpointForEvidence(run Run, evidence Evidence) bool {
	for _, plan := range checkpointPlans(run) {
		if plan.Phase != evidence.Phase || !checkpointPlanComplete(plan) {
			continue
		}
		if plan.CandidateHash == evidence.CandidateHash && sameStrings(plan.Paths, evidence.Paths) && evidence.Target == strconv.Itoa(plan.TargetMajor) {
			return true
		}
	}
	return false
}

func checkpointPlans(run Run) []CheckpointPlan {
	plans := append([]CheckpointPlan(nil), run.CheckpointHistory...)
	if run.CheckpointPlan != nil {
		plans = append(plans, *run.CheckpointPlan)
	}
	return plans
}

func checkpointPlanComplete(plan CheckpointPlan) bool {
	if plan.BackupID == "" || plan.CandidateHash == "" || plan.CompletedAt.IsZero() {
		return false
	}
	for _, step := range plan.Steps {
		if step.Name == CheckpointStepSmoke && step.Status == CheckpointStepPending {
			continue
		}
		if step.Status != CheckpointStepSucceeded {
			return false
		}
		if step.Evidence == nil || !sameStrings(step.Evidence.Paths, plan.Paths) {
			return false
		}
		if step.Name == CheckpointStepValidation && (strings.TrimSpace(step.Evidence.CommandHash) == "" || strings.TrimSpace(step.Evidence.OutputHash) == "") {
			return false
		}
		if step.Name == CheckpointStepValidation && strings.TrimSpace(step.Evidence.CandidateHash) == "" {
			return false
		}
	}
	return true
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
	if tool != "restore_recover" && backup.HasIncompleteRestore(s.root) {
		return Run{}, fmt.Errorf("%w: an incomplete restore journal requires explicit recovery", ErrMutationNotAllowed)
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
	if tool == "test_backup_restore" || tool == "restore_recover" {
		return containsAction(run.Confirmations, ActionConfirmRestore)
	}
	switch tool {
	case "checkpoint_execute":
		return checkpointPhase(run.Phase)
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

// InventoryReport is the durable report input. It refuses partial state so a
// caller cannot manufacture a before/after report from a later filesystem scan.
type InventoryReport struct {
	Baseline inventory.Inventory `json:"baseline"`
	Final    inventory.Inventory `json:"final"`
	Delta    []inventory.Change  `json:"delta"`
}

func (s *Store) RecordInventoryBaseline(id string, snapshot inventory.Inventory) (Run, error) {
	return s.recordInventory(id, snapshot, true)
}
func (s *Store) RecordInventoryFinal(id string, snapshot inventory.Inventory) (Run, error) {
	return s.recordInventory(id, snapshot, false)
}

// RecordContribPlan atomically replaces the plan evidence for the current
// immediate-major cycle. It does not advance a run: callers must still use
// the existing phase actions to authorize mutations.
func (s *Store) RecordContribPlan(id string, plan json.RawMessage) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if len(plan) == 0 || !json.Valid(plan) {
		return Run{}, fmt.Errorf("invalid contrib plan")
	}
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if run.Status != StatusActive {
		return Run{}, fmt.Errorf("%w: contrib plan is not allowed in %s", ErrInvalidTransition, run.Phase)
	}
	run.ContribPlan = append(json.RawMessage(nil), plan...)
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func (s *Store) recordInventory(id string, snapshot inventory.Inventory, baseline bool) (Run, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	if err := validInventory(snapshot); err != nil {
		return Run{}, err
	}
	run, err := s.getLocked(id)
	if err != nil {
		return Run{}, err
	}
	if err := inventoryRecordingAllowed(run, baseline); err != nil {
		return Run{}, err
	}
	copy := snapshot
	if baseline {
		if run.InventoryBaseline != nil {
			return Run{}, fmt.Errorf("%w: baseline already recorded", ErrInvalidTransition)
		}
		run.InventoryBaseline = &copy
	} else {
		if run.InventoryFinal != nil {
			if reflect.DeepEqual(*run.InventoryFinal, snapshot) {
				return run, nil
			}
			return Run{}, fmt.Errorf("%w: final inventory already recorded", ErrInvalidTransition)
		}
		run.InventoryFinal = &copy
	}
	run.UpdatedAt = time.Now().UTC()
	if err := s.writeLocked(run); err != nil {
		return Run{}, err
	}
	return run, nil
}

func inventoryRecordingAllowed(run Run, baseline bool) error {
	if run.Status != StatusActive {
		return fmt.Errorf("%w: run is %s", ErrInvalidTransition, run.Status)
	}
	if baseline {
		if run.Phase != PhaseBaseline || !containsAction(run.AllowedActions, ActionRecordBaseline) {
			return fmt.Errorf("%w: baseline inventory is not authorized in %s", ErrInvalidTransition, run.Phase)
		}
		return nil
	}
	if run.Phase != PhaseReport || !containsAction(run.AllowedActions, ActionRecordReport) {
		return fmt.Errorf("%w: final inventory is not authorized in %s", ErrInvalidTransition, run.Phase)
	}
	return nil
}
func validInventory(snapshot inventory.Inventory) error {
	if snapshot.SchemaVersion != inventory.SchemaVersion || strings.TrimSpace(snapshot.Core.Version) == "" || strings.TrimSpace(snapshot.Core.Source) == "" {
		return fmt.Errorf("%w: schema version, core version and provenance are required", ErrInventoryIncomplete)
	}
	return nil
}
func (s *Store) InventoryReport(id string) (InventoryReport, error) {
	storeMu.Lock()
	defer storeMu.Unlock()
	run, err := s.getLocked(id)
	if err != nil {
		return InventoryReport{}, err
	}
	if run.InventoryBaseline == nil || run.InventoryFinal == nil {
		return InventoryReport{}, ErrInventoryIncomplete
	}
	if err := validInventory(*run.InventoryBaseline); err != nil {
		return InventoryReport{}, err
	}
	if err := validInventory(*run.InventoryFinal); err != nil {
		return InventoryReport{}, err
	}
	return InventoryReport{Baseline: *run.InventoryBaseline, Final: *run.InventoryFinal, Delta: inventory.Delta(*run.InventoryBaseline, *run.InventoryFinal)}, nil
}
