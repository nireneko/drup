package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/backup"
	drupexec "github.com/nireneko/drup/internal/exec"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/projectconfig"
	"github.com/nireneko/drup/internal/runstate"
)

// CheckpointExecuteInput is deliberately typed and contains only phase,
// target, package targets and reviewed paths. It has no command string: every
// operational command below is built as argv and run from the canonical root.
type CheckpointExecuteInput struct {
	ProjectPath string
	RunID       string
	Phase       runstate.Phase
	TargetMajor int
	Targets     []string
	Paths       []string
	Resume      bool
}

type CheckpointExecuteResult struct {
	Success       bool                     `json:"success"`
	RunID         string                   `json:"run_id"`
	Plan          *runstate.CheckpointPlan `json:"checkpoint_plan,omitempty"`
	CandidateHash string                   `json:"candidate_hash,omitempty"`
	Message       string                   `json:"message"`
}

func realHandleCheckpointExecute(args json.RawMessage) (json.RawMessage, error) {
	var params struct {
		ProjectPath string         `json:"project_path"`
		RunID       string         `json:"run_id"`
		Phase       runstate.Phase `json:"phase"`
		TargetMajor int            `json:"target_major"`
		Targets     []string       `json:"targets"`
		Paths       []string       `json:"paths"`
		Resume      bool           `json:"resume"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return nil, err
	}
	result, err := ExecuteCheckpoint(CheckpointExecuteInput{
		ProjectPath: params.ProjectPath, RunID: params.RunID, Phase: params.Phase,
		TargetMajor: params.TargetMajor, Targets: params.Targets, Paths: params.Paths, Resume: params.Resume,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// Test seams retain the production boundaries while allowing step-level tests
// to prove the persisted state without a Drupal installation.
var checkpointCreateBackup = func(projectPath string) (backup.Manifest, error) {
	return backup.NewManager(projectPath).Create(projectPath)
}

var checkpointRun = func(ctx context.Context, dir string, prefix []string, command string, args ...string) (string, string, int, error) {
	return drupexec.RunWithEnvCtx(ctx, dir, prefix, command, args...)
}

var checkpointValidate = func(projectPath string) (string, error) {
	raw, err := realHandleValidate(json.RawMessage(fmt.Sprintf(`{"project_path":%q,"scope":"all"}`, projectPath)))
	if err != nil {
		return "", err
	}
	var result struct {
		TotalErrors int `json:"total_errors"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("decode independent validation result: %w", err)
	}
	if result.TotalErrors != 0 {
		return "", fmt.Errorf("independent validation reported %d errors", result.TotalErrors)
	}
	return string(raw), nil
}

// ExecuteCheckpoint composes the already-existing backup, environment, exec,
// validation and git candidate services into one persisted boundary. It never
// calls CheckpointCommit: successful execution only makes publication
// eligible, and Phase 6 remains the sole publication boundary.
func ExecuteCheckpoint(input CheckpointExecuteInput) (CheckpointExecuteResult, error) {
	if input.ProjectPath == "" || input.RunID == "" {
		return CheckpointExecuteResult{}, fmt.Errorf("project_path and run_id are required")
	}
	store, root, err := canonicalRunStore(input.ProjectPath)
	if err != nil {
		return CheckpointExecuteResult{}, err
	}
	requireExport := managedConfig(root)
	smokeCommands := projectconfig.CheckpointSmokeCommands(root)
	run, err := store.BeginCheckpointPlan(input.RunID, runstate.CheckpointPlanInput{
		Phase: input.Phase, TargetMajor: input.TargetMajor, Targets: input.Targets,
		Paths: normalizedStrings(input.Paths), RequireConfigExport: requireExport, SmokeCommands: smokeCommands,
	})
	if err != nil {
		return CheckpointExecuteResult{}, err
	}
	if input.Resume {
		run, err = store.ResumeUnavailableCheckpoint(run.ID)
		if err != nil {
			return checkpointResult(run, "checkpoint resume blocked"), err
		}
	}
	plan := run.CheckpointPlan
	if plan == nil {
		return CheckpointExecuteResult{}, fmt.Errorf("checkpoint plan was not persisted")
	}
	if !plan.CompletedAt.IsZero() {
		return CheckpointExecuteResult{Success: true, RunID: run.ID, Plan: plan, CandidateHash: plan.CandidateHash, Message: "checkpoint already completed"}, nil
	}
	// The validator is authoritative only for an immutable candidate. Capture
	// it after every mutating checkpoint step and compare it after validation.
	var validatedCandidate gitops.Candidate
	if resumedCandidate, ok := checkpointValidatedCandidate(*plan); ok {
		validatedCandidate = resumedCandidate
	}

	detection, err := defaultEnvDetector.Detect(root, false)
	if err != nil {
		return checkpointResult(run, "environment unavailable"), fmt.Errorf("detect environment: %w", err)
	}
	drushRoot := root
	if len(detection.CommandPrefix) > 0 {
		// The project is the container's working directory; an absolute host
		// path is not mounted there. "." is still an explicit drush root.
		drushRoot = "."
	}
	for _, step := range plan.Steps {
		if step.Status == runstate.CheckpointStepSucceeded {
			continue
		}
		// Fail closed instead of treating a stale running/failed state as a
		// retry permission. A human must explicitly recover a partial effect.
		if step.Status != runstate.CheckpointStepPending {
			return checkpointResult(run, "checkpoint requires explicit recovery"), fmt.Errorf("checkpoint step %s is %s; refusing blind resume", step.Name, step.Status)
		}
		if _, err := store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepRunning, nil); err != nil {
			return checkpointResult(run, "checkpoint state unavailable"), err
		}
		var evidence *runstate.CheckpointStepEvidence
		switch step.Name {
		case runstate.CheckpointStepBackup:
			manifest, backupErr := checkpointCreateBackup(root)
			evidence = backupEvidence(backupErr, plan.Paths)
			if backupErr == nil {
				if _, err := store.BindCheckpointBackup(run.ID, manifest.BackupID); err != nil {
					return checkpointResult(run, "backup could not be bound"), err
				}
			}
			err = checkpointStepResult(store, run.ID, step.Name, backupErr, evidence)
		case runstate.CheckpointStepUpdate:
			updateArgs := append([]string{"update"}, plan.Targets...)
			updateArgs = append(updateArgs, "--with-all-dependencies")
			err, evidence = executeCheckpointCommand(root, detection.CommandPrefix, plan.Paths, "composer", updateArgs...)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		case runstate.CheckpointStepDatabase:
			err, evidence = executeCheckpointCommand(root, detection.CommandPrefix, plan.Paths, "drush", "updatedb", "--yes", "--root="+drushRoot)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		case runstate.CheckpointStepCacheRebuild:
			err, evidence = executeCheckpointCommand(root, detection.CommandPrefix, plan.Paths, "drush", "cache:rebuild", "--root="+drushRoot)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		case runstate.CheckpointStepSiteStatus:
			err, evidence = executeCheckpointCommand(root, detection.CommandPrefix, plan.Paths, "drush", "status", "--format=json", "--root="+drushRoot)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		case runstate.CheckpointStepValidation:
			candidateBefore, candidateErr := gitops.CandidateForPaths(root, plan.Paths)
			started := time.Now()
			output := ""
			validateErr := candidateErr
			if validateErr == nil {
				output, validateErr = checkpointValidate(root)
			}
			candidateAfter := candidateBefore
			if validateErr == nil {
				candidateAfter, validateErr = gitops.CandidateForPaths(root, plan.Paths)
				if validateErr == nil && (candidateAfter.Hash != candidateBefore.Hash || !sameStringSlice(candidateAfter.Paths, candidateBefore.Paths)) {
					validateErr = fmt.Errorf("candidate changed while independent validation ran")
				}
			}
			evidencePaths := plan.Paths
			if len(candidateBefore.Paths) > 0 {
				evidencePaths = candidateBefore.Paths
			}
			evidence = newCheckpointEvidence("validate", output, validateErr, 0, time.Since(started), evidencePaths)
			if validateErr == nil {
				evidence.CandidateHash = candidateAfter.Hash
			}
			err = checkpointStepResult(store, run.ID, step.Name, validateErr, evidence)
			if err == nil {
				validatedCandidate = candidateAfter
			}
		case runstate.CheckpointStepConfigExport:
			err, evidence = executeCheckpointCommand(root, detection.CommandPrefix, plan.Paths, "drush", "config:export", "--yes", "--root="+drushRoot)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		case runstate.CheckpointStepSmoke:
			err, evidence = executeCheckpointSmoke(root, detection.CommandPrefix, plan.Paths, plan.SmokeCommands)
			err = checkpointStepResult(store, run.ID, step.Name, err, evidence)
		default:
			err = fmt.Errorf("unsupported checkpoint step %s", step.Name)
		}
		if err != nil {
			return checkpointResultForStore(store, run.ID, "checkpoint step failed"), err
		}
	}

	candidate, err := gitops.CandidateForPaths(root, plan.Paths)
	if err != nil {
		return checkpointResultForStore(store, run.ID, "candidate identity unavailable"), err
	}
	if validatedCandidate.Hash == "" || candidate.Hash != validatedCandidate.Hash || !sameStringSlice(candidate.Paths, validatedCandidate.Paths) {
		return checkpointResultForStore(store, run.ID, "candidate changed after validation"), fmt.Errorf("candidate changed after independent validation")
	}
	run, err = store.CompleteCheckpointPlan(run.ID, candidate.Hash)
	if err != nil {
		return checkpointResultForStore(store, input.RunID, "checkpoint closure blocked"), err
	}
	return CheckpointExecuteResult{Success: true, RunID: run.ID, Plan: run.CheckpointPlan, CandidateHash: candidate.Hash, Message: "checkpoint completed; checkpoint_commit remains required for publication"}, nil
}

func executeCheckpointSmoke(root string, prefix []string, paths []string, commands [][]string) (error, *runstate.CheckpointStepEvidence) {
	started := time.Now()
	for _, command := range commands {
		if len(command) == 0 {
			return fmt.Errorf("configured smoke command is empty"), newCheckpointEvidence("smoke", "", fmt.Errorf("configured smoke command is empty"), -1, time.Since(started), paths)
		}
		if err, evidence := executeCheckpointCommand(root, prefix, paths, command[0], command[1:]...); err != nil {
			return err, evidence
		}
	}
	evidence := newCheckpointEvidence("smoke", "configured allowlisted smoke commands completed", nil, 0, time.Since(started), paths)
	evidence.CommandHash = checkpointCommandsHash(prefix, commands)
	return nil, evidence
}

func checkpointStepResult(store *runstate.Store, runID string, name runstate.CheckpointStepName, stepErr error, evidence *runstate.CheckpointStepEvidence) error {
	status := runstate.CheckpointStepSucceeded
	if stepErr != nil {
		status = runstate.CheckpointStepFailed
		if checkpointUnavailable(stepErr) {
			status = runstate.CheckpointStepUnavailable
		}
	}
	_, persistErr := store.RecordCheckpointStep(runID, name, status, evidence)
	if persistErr != nil {
		return fmt.Errorf("persist checkpoint step %s: %w", name, persistErr)
	}
	return stepErr
}

func checkpointValidatedCandidate(plan runstate.CheckpointPlan) (gitops.Candidate, bool) {
	for _, step := range plan.Steps {
		if step.Name != runstate.CheckpointStepValidation || step.Status != runstate.CheckpointStepSucceeded || step.Evidence == nil || strings.TrimSpace(step.Evidence.CandidateHash) == "" || !sameStringSlice(step.Evidence.Paths, plan.Paths) {
			continue
		}
		return gitops.Candidate{Hash: step.Evidence.CandidateHash, Paths: append([]string(nil), step.Evidence.Paths...)}, true
	}
	return gitops.Candidate{}, false
}

func checkpointUnavailable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "executable file not found") || strings.Contains(err.Error(), "command timed out") || strings.Contains(err.Error(), context.DeadlineExceeded.Error()) || strings.Contains(err.Error(), context.Canceled.Error())
}

func executeCheckpointCommand(root string, prefix []string, paths []string, command string, args ...string) (error, *runstate.CheckpointStepEvidence) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultExecTimeout)
	defer cancel()
	started := time.Now()
	stdout, stderr, exitCode, err := checkpointRun(ctx, root, prefix, command, args...)
	evidence := newCheckpointEvidence(command, stdout+"\n"+stderr, err, exitCode, time.Since(started), paths)
	evidence.CommandHash = checkpointArgvHash(prefix, command, args)
	if err != nil {
		return fmt.Errorf("execute %s: %w", command, err), evidence
	}
	if exitCode != 0 {
		return fmt.Errorf("execute %s: exit %d", command, exitCode), evidence
	}
	return nil, evidence
}

func backupEvidence(err error, paths []string) *runstate.CheckpointStepEvidence {
	return newCheckpointEvidence("backup.create", "", err, 0, 0, paths)
}

// checkpointArgvHash hashes the exact argv process shape. JSON's typed array
// encoding is deliberate: delimiter joining would make distinct vectors such
// as ["a", "b c"] and ["a b", "c"] ambiguous.
func checkpointArgvHash(prefix []string, command string, args []string) string {
	argv := make([]string, 0, len(prefix)+1+len(args))
	argv = append(argv, prefix...)
	argv = append(argv, command)
	argv = append(argv, args...)
	canonical, err := json.Marshal(struct {
		Argv []string `json:"argv"`
	}{Argv: argv})
	if err != nil {
		return digest(strings.Join(argv, "\x00"))
	}
	return digest(string(canonical))
}

func checkpointCommandsHash(prefix []string, commands [][]string) string {
	argvs := make([][]string, 0, len(commands))
	for _, command := range commands {
		if len(command) == 0 {
			continue
		}
		argv := make([]string, 0, len(prefix)+len(command))
		argv = append(argv, prefix...)
		argv = append(argv, command...)
		argvs = append(argvs, argv)
	}
	canonical, err := json.Marshal(struct {
		Argvs [][]string `json:"argvs"`
	}{Argvs: argvs})
	if err != nil {
		return digest("smoke")
	}
	return digest(string(canonical))
}

func newCheckpointEvidence(command, output string, err error, exitCode int, duration time.Duration, paths []string) *runstate.CheckpointStepEvidence {
	if err != nil {
		output += "\n" + err.Error()
	}
	return &runstate.CheckpointStepEvidence{CommandHash: digest(command), OutputHash: digest(output), ExitCode: exitCode, Duration: duration, Paths: append([]string(nil), paths...)}
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func sameStringSlice(left, right []string) bool {
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

func managedConfig(root string) bool {
	// A configured sync directory is a managed-config project. We avoid using
	// a caller supplied boolean because that would let a prompt waive export.
	info, err := os.Stat(filepath.Join(root, "config", "sync"))
	return err == nil && info.IsDir()
}

func checkpointResult(run runstate.Run, message string) CheckpointExecuteResult {
	return CheckpointExecuteResult{RunID: run.ID, Plan: run.CheckpointPlan, Message: message}
}

func checkpointResultForStore(store *runstate.Store, runID, message string) CheckpointExecuteResult {
	run, _ := store.Get(runID)
	return checkpointResult(run, message)
}
