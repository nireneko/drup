package app

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

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
		run, err = store.Record(run.ID, runstate.RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}
	candidate, err := gitops.CandidateForPaths(run.Root, paths)
	if err != nil {
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
