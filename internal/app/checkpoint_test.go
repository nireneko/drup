package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/backup"
	"github.com/nireneko/drup/internal/gitops"
	"github.com/nireneko/drup/internal/runstate"
)

func TestCheckpointCommit_CommitsOnlyMatchingValidationCandidate(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run := checkpointRunAtReport(t, store, runstate.CommitStrategySingle, "all", "11", []string{"composer.json"})

	result, err := CheckpointCommit(CheckpointCommitInput{
		ProjectPath: dir, RunID: run.ID, Strategy: runstate.CommitStrategySingle,
		Scope: []string{"all"}, Paths: []string{"composer.json"}, ValidationHash: "validation-11", Target: "11",
		Message: "chore(checkpoint): validate Drupal 11",
	})
	if err != nil {
		t.Fatalf("CheckpointCommit error = %v", err)
	}
	if !result.Success || result.Skipped || result.CommitHash == "" {
		t.Fatalf("CheckpointCommit result = %+v, want committed receipt", result)
	}
	if got := runGitOutput(t, dir, "show", "--name-only", "--pretty=format:", "HEAD"); strings.TrimSpace(got) != "composer.json" {
		t.Errorf("checkpoint commit paths = %q, want composer.json", got)
	}
	persisted, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := persisted.Evidence[len(persisted.Evidence)-1].Kind; got != "checkpoint_commit" {
		t.Errorf("last evidence kind = %q, want checkpoint_commit", got)
	}
}

func TestCheckpointCommit_RejectsStaleValidationAndLeavesHistoryUntouched(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run := checkpointRunAtReport(t, store, runstate.CommitStrategySingle, "all", "11", []string{"composer.json"})
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11.1"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before := runGitOutput(t, dir, "rev-list", "--count", "HEAD")
	_, err := CheckpointCommit(CheckpointCommitInput{
		ProjectPath: dir, RunID: run.ID, Strategy: runstate.CommitStrategySingle,
		Scope: []string{"all"}, Paths: []string{"composer.json"}, ValidationHash: "validation-11", Target: "11",
	})
	if err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("CheckpointCommit stale error = %v, want stale evidence refusal", err)
	}
	if after := runGitOutput(t, dir, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("stale checkpoint changed history: before %s, after %s", before, after)
	}
}

func TestCheckpointCommit_NoneNeverCreatesCommit(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "none", TargetMajor: 11, CommitStrategy: runstate.CommitStrategyNone, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	before := runGitOutput(t, dir, "rev-list", "--count", "HEAD")
	result, err := CheckpointCommit(CheckpointCommitInput{ProjectPath: dir, RunID: run.ID, Strategy: runstate.CommitStrategyNone, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped || !result.Success {
		t.Errorf("none result = %+v, want successful skip", result)
	}
	if after := runGitOutput(t, dir, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("none strategy changed history: before %s, after %s", before, after)
	}
}

func TestCheckpointCommit_NoneRejectsOperationalPhaseWithoutCompletedPlan(t *testing.T) {
	dir := checkpointRepository(t)
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "none-operational", TargetMajor: 11, CommitStrategy: runstate.CommitStrategyNone, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseCustomTheme {
		run, err = store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}

	_, err = CheckpointCommit(CheckpointCommitInput{ProjectPath: dir, RunID: run.ID, Strategy: runstate.CommitStrategyNone, Scope: []string{"all"}})
	if err == nil {
		t.Fatalf("none operational checkpoint error = %v, want closed checkpoint gate", err)
	}
}

func TestCheckpointCommit_PerFixPublishesItsValidatedCheckpointBeforeFinalReport(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "per-fix-run", TargetMajor: 11, CommitStrategy: runstate.CommitStrategyPerFix, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseCustomTheme {
		run, err = store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := gitops.CandidateForPaths(dir, []string{"composer.json"})
	if err != nil {
		t.Fatal(err)
	}
	planRun, err := store.BeginCheckpointPlan(run.ID, runstate.CheckpointPlanInput{Phase: run.Phase, TargetMajor: 11, Targets: []string{"custom"}, Paths: candidate.Paths, RequireConfigExport: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range planRun.CheckpointPlan.Steps {
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepRunning, nil); err != nil {
			t.Fatal(err)
		}
		if step.Name == runstate.CheckpointStepBackup {
			if _, err = store.BindCheckpointBackup(run.ID, "custom-backup"); err != nil {
				t.Fatal(err)
			}
		}
		evidence := &runstate.CheckpointStepEvidence{CommandHash: "command-" + string(step.Name), OutputHash: "output-" + string(step.Name), CandidateHash: candidate.Hash, Paths: candidate.Paths}
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepSucceeded, evidence); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = store.CompleteCheckpointPlan(run.ID, candidate.Hash); err != nil {
		t.Fatal(err)
	}
	run, err = store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "validation", Summary: "validated fix", ValidationHash: "fix-validation", CandidateHash: candidate.Hash, Paths: candidate.Paths, Target: "11"})
	if err != nil {
		t.Fatal(err)
	}
	if run.Phase == runstate.PhaseReport {
		t.Fatal("per-fix setup unexpectedly reached final report")
	}
	result, err := CheckpointCommit(CheckpointCommitInput{ProjectPath: dir, RunID: run.ID, Strategy: runstate.CommitStrategyPerFix, Scope: []string{"all"}, Paths: []string{"composer.json"}, ValidationHash: "fix-validation", Target: "11"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.CommitHash == "" {
		t.Errorf("per-fix result = %+v, want checkpoint commit", result)
	}
}

func TestCheckpointCommit_CLIAndMCPShareObservablePublication(t *testing.T) {
	requireGitForApp(t)
	cliDir := checkpointRepository(t)
	mcpDir := checkpointRepository(t)
	for _, dir := range []string{cliDir, mcpDir} {
		if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		checkpointRunAtReport(t, runstate.NewStore(dir), runstate.CommitStrategySingle, "all", "11", []string{"composer.json"})
	}

	cliOutput := captureCheckpointStdout(t, func() error {
		return RunCheckpointCommit([]string{
			"--project-path=" + cliDir, "--run-id=single-run", "--commit-strategy=single",
			"--scope=all", "--paths=composer.json", "--validation-hash=validation-11", "--target=11",
		})
	})
	var cliResult CheckpointCommitResult
	if err := json.Unmarshal([]byte(cliOutput), &cliResult); err != nil {
		t.Fatalf("CLI result is not JSON: %v\n%s", err, cliOutput)
	}
	mcpOutput, err := realHandleCheckpointCommit(json.RawMessage(`{"project_path":` + jsonStr(mcpDir) + `,"run_id":"single-run","commit_strategy":"single","scope":["all"],"paths":["composer.json"],"validation_hash":"validation-11","target":"11"}`))
	if err != nil {
		t.Fatalf("MCP checkpoint error = %v", err)
	}
	var mcpResult CheckpointCommitResult
	if err := json.Unmarshal(mcpOutput, &mcpResult); err != nil {
		t.Fatal(err)
	}
	if !cliResult.Success || !mcpResult.Success || cliResult.Skipped != mcpResult.Skipped || cliResult.Strategy != mcpResult.Strategy || len(cliResult.ChangedFiles) != len(mcpResult.ChangedFiles) {
		t.Errorf("CLI result = %+v, MCP result = %+v; want the same publication semantics", cliResult, mcpResult)
	}
	for _, dir := range []string{cliDir, mcpDir} {
		if got := runGitOutput(t, dir, "show", "--name-only", "--pretty=format:", "HEAD"); got != "composer.json" {
			t.Errorf("%s committed paths = %q, want composer.json", dir, got)
		}
	}
}

func TestExecuteCheckpointPersistsEvidenceAndDoesNotPublish(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "operational", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseContribMinor {
		run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
		if err != nil {
			t.Fatal(err)
		}
	}

	origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
	checkpointCreateBackup = func(string) (backup.Manifest, error) { return backup.Manifest{BackupID: "backup-bound"}, nil }
	checkpointRun = func(_ context.Context, gotDir string, _ []string, command string, args ...string) (string, string, int, error) {
		if gotDir != dir {
			t.Errorf("command cwd = %q, want explicit root %q", gotDir, dir)
		}
		if command == "sh" || command == "bash" {
			t.Fatalf("checkpoint invoked shell")
		}
		return command + strings.Join(args, " "), "", 0, nil
	}
	checkpointValidate = func(string) (string, error) { return `{"total_errors":0}`, nil }
	t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })

	before := runGitOutput(t, dir, "rev-list", "--count", "HEAD")
	result, err := ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: runstate.PhaseContribMinor, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
	if err != nil {
		t.Fatalf("ExecuteCheckpoint error = %v", err)
	}
	if !result.Success || result.Plan == nil || result.Plan.BackupID != "backup-bound" || result.CandidateHash == "" {
		t.Fatalf("checkpoint result = %+v", result)
	}
	if after := runGitOutput(t, dir, "rev-list", "--count", "HEAD"); after != before {
		t.Errorf("executor published history: before %s after %s", before, after)
	}
	persisted, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range persisted.CheckpointPlan.Steps {
		if step.Status != runstate.CheckpointStepSucceeded || step.Evidence == nil || step.Evidence.OutputHash == "" || len(step.Evidence.Paths) == 0 {
			t.Errorf("step %+v, want persisted sanitized success evidence", step)
		}
	}
}

func TestExecuteCheckpointRunsComposerUpdateOnlyForContribPhases(t *testing.T) {
	requireGitForApp(t)
	for _, tt := range []struct {
		phase      runstate.Phase
		wantUpdate bool
	}{
		{runstate.PhaseCustomTheme, false},
		{runstate.PhaseContribPatch, true},
		{runstate.PhaseContribMinor, true},
		{runstate.PhaseContribMajor, true},
		{runstate.PhaseCoreLoop, false},
		{runstate.PhaseCleanup, false},
	} {
		t.Run(string(tt.phase), func(t *testing.T) {
			dir := checkpointRepository(t)
			if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			store := runstate.NewStore(dir)
			run, err := store.Create(runstate.CreateInput{ID: "phase-" + string(tt.phase), TargetMajor: 11, Scope: []string{"all"}})
			if err != nil {
				t.Fatal(err)
			}
			for run.Phase != tt.phase {
				run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
				if err != nil {
					t.Fatal(err)
				}
			}

			origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
			checkpointCreateBackup = func(string) (backup.Manifest, error) {
				return backup.Manifest{BackupID: "backup-" + string(tt.phase)}, nil
			}
			composerUpdates := 0
			checkpointRun = func(_ context.Context, _ string, _ []string, command string, args ...string) (string, string, int, error) {
				if command == "composer" && len(args) > 0 && args[0] == "update" {
					composerUpdates++
				}
				return "ok", "", 0, nil
			}
			checkpointValidate = func(string) (string, error) { return `{"total_errors":0}`, nil }
			t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })

			_, err = ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: tt.phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := composerUpdates > 0; got != tt.wantUpdate {
				t.Fatalf("composer update invoked = %v, want %v", got, tt.wantUpdate)
			}
		})
	}
}

func TestCheckpointInvocationHashBindsFullUnambiguousArgv(t *testing.T) {
	left := checkpointArgvHash([]string{"docker", "compose", "exec", "php"}, "composer", []string{"update", "drupal/example"})
	right := checkpointArgvHash([]string{"docker", "compose", "exec", "php"}, "composer", []string{"update", "drupal/example"})
	if left != right {
		t.Fatalf("identical argv hash differs: %s != %s", left, right)
	}
	if left == checkpointArgvHash(nil, "composer", []string{"update", "drupal/example"}) {
		t.Fatal("execution prefix was omitted from the argv hash")
	}
	if left == checkpointArgvHash([]string{"docker", "compose", "exec", "php"}, "composer", []string{"update drupal/example"}) {
		t.Fatal("argument boundaries produced an ambiguous hash")
	}
}

func TestExecuteCheckpointValidatesAfterConfigExportChangesCandidate(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.MkdirAll(filepath.Join(dir, "config", "sync"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "final-candidate", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseContribPatch {
		run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
		if err != nil {
			t.Fatal(err)
		}
	}
	origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
	checkpointCreateBackup = func(string) (backup.Manifest, error) { return backup.Manifest{BackupID: "backup-final"}, nil }
	checkpointRun = func(_ context.Context, _ string, _ []string, command string, args ...string) (string, string, int, error) {
		if command == "drush" && len(args) > 0 && args[0] == "config:export" {
			if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11.1"}}`), 0o644); err != nil {
				return "", "", -1, err
			}
		}
		return "ok", "", 0, nil
	}
	validatedFinal := false
	checkpointValidate = func(project string) (string, error) {
		data, err := os.ReadFile(filepath.Join(project, "composer.json"))
		if err != nil {
			return "", err
		}
		validatedFinal = strings.Contains(string(data), "^11.1")
		if !validatedFinal {
			return "", fmt.Errorf("validator saw pre-export candidate")
		}
		return `{"total_errors":0}`, nil
	}
	t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })
	result, err := ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: runstate.PhaseContribPatch, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if !validatedFinal {
		t.Fatal("validation did not observe final exported candidate")
	}
	candidate, err := gitops.CandidateForPaths(dir, []string{"composer.json"})
	if err != nil {
		t.Fatal(err)
	}
	if result.CandidateHash != candidate.Hash {
		t.Fatalf("candidate hash = %s, want final %s", result.CandidateHash, candidate.Hash)
	}
}

func TestExecuteCheckpointRejectsValidationThatChangesTheCapturedCandidate(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "stale-validation", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseContribPatch {
		run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
		if err != nil {
			t.Fatal(err)
		}
	}

	origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
	checkpointCreateBackup = func(string) (backup.Manifest, error) { return backup.Manifest{BackupID: "stale-backup"}, nil }
	checkpointRun = func(_ context.Context, _ string, _ []string, _ string, _ ...string) (string, string, int, error) {
		return "ok", "", 0, nil
	}
	checkpointValidate = func(project string) (string, error) {
		if err := os.WriteFile(filepath.Join(project, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11.2"}}`), 0o644); err != nil {
			return "", err
		}
		return `{"total_errors":0}`, nil
	}
	t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })

	_, err = ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: runstate.PhaseContribPatch, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
	if err == nil || !strings.Contains(err.Error(), "candidate changed") {
		t.Fatalf("ExecuteCheckpoint stale validation error = %v, want final-candidate refusal", err)
	}
	persisted, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.CheckpointPlan == nil || !persisted.CheckpointPlan.CompletedAt.IsZero() {
		t.Fatalf("stale validation completed plan = %+v, want incomplete checkpoint", persisted.CheckpointPlan)
	}
}

func TestExecuteCheckpointResumesOnlyMatchingPersistedValidationCandidate(t *testing.T) {
	for _, tt := range []struct {
		name       string
		mutate     bool
		wantErr    bool
		wantClosed bool
	}{
		{name: "same candidate resumes without replaying validation", wantClosed: true},
		{name: "changed candidate is rejected", mutate: true, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, store, run := checkpointValidationCrashWindow(t)
			if tt.mutate {
				if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11.3"}}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
			checkpointCreateBackup = func(string) (backup.Manifest, error) {
				t.Fatal("resume repeated backup")
				return backup.Manifest{}, nil
			}
			checkpointRun = func(_ context.Context, _ string, _ []string, _ string, _ ...string) (string, string, int, error) {
				t.Fatal("resume repeated mutation")
				return "", "", 0, nil
			}
			checkpointValidate = func(string) (string, error) {
				t.Fatal("resume replayed validation instead of using durable evidence")
				return "", nil
			}
			t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })

			result, err := ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: run.Phase, TargetMajor: run.TargetMajor, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "candidate changed") {
					t.Fatalf("crash-window stale candidate error = %v, want refusal", err)
				}
			} else if err != nil || !result.Success {
				t.Fatalf("crash-window resume result = %+v, error = %v", result, err)
			}
			persisted, getErr := store.Get(run.ID)
			if getErr != nil {
				t.Fatal(getErr)
			}
			if got := persisted.CheckpointPlan != nil && !persisted.CheckpointPlan.CompletedAt.IsZero(); got != tt.wantClosed {
				t.Fatalf("crash-window completion = %v, want %v", got, tt.wantClosed)
			}
		})
	}
}

func checkpointValidationCrashWindow(t *testing.T) (string, *runstate.Store, runstate.Run) {
	t.Helper()
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "validation-crash-window", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseContribPatch {
		run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := gitops.CandidateForPaths(dir, []string{"composer.json"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.BeginCheckpointPlan(run.ID, runstate.CheckpointPlanInput{Phase: run.Phase, TargetMajor: run.TargetMajor, Targets: []string{"drupal/example"}, Paths: candidate.Paths})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range run.CheckpointPlan.Steps {
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepRunning, nil); err != nil {
			t.Fatal(err)
		}
		if step.Name == runstate.CheckpointStepBackup {
			if _, err = store.BindCheckpointBackup(run.ID, "crash-window-backup"); err != nil {
				t.Fatal(err)
			}
		}
		evidence := &runstate.CheckpointStepEvidence{CommandHash: "command-" + string(step.Name), OutputHash: "output-" + string(step.Name), CandidateHash: candidate.Hash, Paths: candidate.Paths}
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepSucceeded, evidence); err != nil {
			t.Fatal(err)
		}
	}
	return dir, store, run
}

func TestExecuteCheckpointRequiresExportForManagedConfig(t *testing.T) {
	requireGitForApp(t)
	dir := checkpointRepository(t)
	if err := os.MkdirAll(filepath.Join(dir, "config", "sync"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^11"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "managed-config", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseContribPatch {
		run, err = advanceCheckpointRunForTest(store, run, []string{"composer.json"})
		if err != nil {
			t.Fatal(err)
		}
	}
	origBackup, origRun, origValidate := checkpointCreateBackup, checkpointRun, checkpointValidate
	checkpointCreateBackup = func(string) (backup.Manifest, error) { return backup.Manifest{BackupID: "managed-backup"}, nil }
	var exported bool
	checkpointRun = func(_ context.Context, _ string, _ []string, command string, args ...string) (string, string, int, error) {
		if command == "drush" && len(args) > 0 && args[0] == "config:export" {
			exported = true
		}
		return "ok", "", 0, nil
	}
	checkpointValidate = func(string) (string, error) { return `{"total_errors":0}`, nil }
	t.Cleanup(func() { checkpointCreateBackup, checkpointRun, checkpointValidate = origBackup, origRun, origValidate })
	result, err := ExecuteCheckpoint(CheckpointExecuteInput{ProjectPath: dir, RunID: run.ID, Phase: runstate.PhaseContribPatch, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.json"}})
	if err != nil {
		t.Fatal(err)
	}
	if !exported || result.Plan == nil || !result.Plan.RequireConfigExport {
		t.Fatalf("managed config checkpoint = %+v, exported=%v; want required config export", result.Plan, exported)
	}
}

func checkpointRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/core-recommended":"^10"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@example.test")
	runGitCmd(t, dir, "config", "user.name", "Test")
	runGitCmd(t, dir, "add", "composer.json")
	runGitCmd(t, dir, "commit", "-m", "initial")
	return dir
}

func checkpointRunAtReport(t *testing.T, store *runstate.Store, strategy runstate.CommitStrategy, scope, target string, paths []string) runstate.Run {
	t.Helper()
	run, err := store.Create(runstate.CreateInput{ID: string(strategy) + "-run", TargetMajor: 11, CommitStrategy: strategy, Scope: []string{scope}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != runstate.PhaseCleanup {
		run, err = advanceCheckpointRunForTest(store, run, paths)
		if err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := gitops.CandidateForPaths(run.Root, paths)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = completeCheckpointPlanForTest(store, run, candidate); err != nil {
		t.Fatal(err)
	}
	run, err = store.Record(run.ID, runstate.RecordInput{
		Action: runstate.ActionRecordCleanup, Kind: "validation", Summary: "independent validation passed",
		ValidationHash: "validation-11", CandidateHash: candidate.Hash, Paths: candidate.Paths, Target: target,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func advanceCheckpointRunForTest(store *runstate.Store, run runstate.Run, paths []string) (runstate.Run, error) {
	if !isOperationalCheckpointPhaseForTest(run.Phase) {
		return store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
	}
	candidate, err := gitops.CandidateForPaths(run.Root, paths)
	if err != nil {
		return runstate.Run{}, err
	}
	if _, err = completeCheckpointPlanForTest(store, run, candidate); err != nil {
		return runstate.Run{}, err
	}
	return store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "validation", Summary: "independent validation", ValidationHash: "validation-" + string(run.Phase), CandidateHash: candidate.Hash, Paths: candidate.Paths, Target: "11"})
}

func completeCheckpointPlanForTest(store *runstate.Store, run runstate.Run, candidate gitops.Candidate) (runstate.Run, error) {
	planRun, err := store.BeginCheckpointPlan(run.ID, runstate.CheckpointPlanInput{Phase: run.Phase, TargetMajor: run.TargetMajor, Targets: []string{"drupal/example"}, Paths: candidate.Paths, RequireConfigExport: true})
	if err != nil {
		return runstate.Run{}, err
	}
	for _, step := range planRun.CheckpointPlan.Steps {
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepRunning, nil); err != nil {
			return runstate.Run{}, err
		}
		if step.Name == runstate.CheckpointStepBackup {
			if _, err = store.BindCheckpointBackup(run.ID, "backup-"+string(run.Phase)); err != nil {
				return runstate.Run{}, err
			}
		}
		evidence := &runstate.CheckpointStepEvidence{CommandHash: "command-" + string(step.Name), OutputHash: "output-" + string(step.Name), CandidateHash: candidate.Hash, Paths: candidate.Paths}
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, runstate.CheckpointStepSucceeded, evidence); err != nil {
			return runstate.Run{}, err
		}
	}
	if _, err = store.CompleteCheckpointPlan(run.ID, candidate.Hash); err != nil {
		return runstate.Run{}, err
	}
	return store.Get(run.ID)
}

func isOperationalCheckpointPhaseForTest(phase runstate.Phase) bool {
	switch phase {
	case runstate.PhaseCustomTheme, runstate.PhaseContribPatch, runstate.PhaseContribMinor, runstate.PhaseContribMajor, runstate.PhaseCoreLoop, runstate.PhaseCleanup:
		return true
	default:
		return false
	}
}

func runGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func captureCheckpointStdout(t *testing.T, call func() error) string {
	t.Helper()
	previous := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	err = call()
	writer.Close()
	os.Stdout = previous
	if err != nil {
		t.Fatal(err)
	}
	output, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(output)
}
