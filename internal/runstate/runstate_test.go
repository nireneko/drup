package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreCreatePersistsAllowedActionsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)

	run, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11, CommitStrategy: "per-fix", Scope: []string{"all"}})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if run.Root != root || run.Phase != PhaseGitSafety {
		t.Fatalf("created run = %+v, want canonical root and git_safety", run)
	}
	if got := run.AllowedActions; len(got) != 1 || got[0] != ActionRecordGitSafety {
		t.Fatalf("allowed actions = %v, want [%s]", got, ActionRecordGitSafety)
	}

	data, err := os.ReadFile(filepath.Join(root, ".drup", "runs", "run-1.json"))
	if err != nil {
		t.Fatalf("persisted run missing: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("persisted run is empty")
	}

	restarted, err := NewStore(root).Get("run-1")
	if err != nil {
		t.Fatalf("Get() after restart error = %v", err)
	}
	if restarted.Phase != PhaseGitSafety || len(restarted.AllowedActions) != 1 || restarted.AllowedActions[0] != ActionRecordGitSafety {
		t.Fatalf("restarted run = %+v, want same phase and allowed action", restarted)
	}
}

func TestStoreCreateRejectsSecondActiveRunForRoot(t *testing.T) {
	store := NewStore(t.TempDir())
	if _, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11}); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if _, err := store.Create(CreateInput{ID: "run-2", TargetMajor: 12}); !errors.Is(err, ErrActiveRunExists) {
		t.Fatalf("second Create() error = %v, want ErrActiveRunExists", err)
	}
}

func TestStoreCreateRejectsUnknownCommitStrategy(t *testing.T) {
	store := NewStore(t.TempDir())
	_, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11, CommitStrategy: "every-file"})
	if err == nil || !strings.Contains(err.Error(), "commit_strategy") {
		t.Fatalf("Create invalid strategy error = %v, want strategy refusal", err)
	}
}

func TestStoreRecordRejectsInvalidTransitionAndKeepsEvidenceAppendOnly(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(run.ID, RecordInput{Action: ActionRecordTooling, Kind: "check", Summary: "wrong phase"}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Record(wrong action) error = %v, want ErrInvalidTransition", err)
	}
	run, err = store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Evidence) != 0 || run.Phase != PhaseGitSafety {
		t.Fatalf("rejected transition changed run = %+v", run)
	}

	run, err = store.Record(run.ID, RecordInput{Action: ActionRecordGitSafety, Kind: "check", Summary: "authorization: token abc", Payload: []byte(`{"stdout":"sensitive"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if run.Phase != PhaseEnvironment || len(run.Evidence) != 1 {
		t.Fatalf("recorded run = %+v, want environment with one evidence entry", run)
	}
	if run.Evidence[0].Summary != "sanitized evidence" || run.Evidence[0].PayloadHash == "" {
		t.Fatalf("evidence = %+v, want sanitised summary and hash-only payload", run.Evidence[0])
	}
}

func TestStoreBlockPersistsRecoveryAndRootValidation(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	run, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	run, err = store.Block(run.ID, "composer secret conflict", "resolve composer constraint")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != StatusBlocked || len(run.AllowedActions) != 1 || run.AllowedActions[0] != ActionResolveBlock {
		t.Fatalf("blocked run = %+v, want persisted resolve action", run)
	}
	restarted, err := NewStore(root).Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restarted.Status != StatusBlocked || restarted.PendingHuman[0].Reason != "sanitized evidence" {
		t.Fatalf("restarted block = %+v, want sanitized persisted blocker", restarted)
	}
	if _, err := store.ValidateMutation(run.ID, root, "autofix"); !errors.Is(err, ErrMutationNotAllowed) {
		t.Fatalf("ValidateMutation(blocked) error = %v, want ErrMutationNotAllowed", err)
	}
	if _, err := store.ValidateMutation(run.ID, t.TempDir(), "autofix"); !errors.Is(err, ErrRootMismatch) {
		t.Fatalf("ValidateMutation(other root) error = %v, want ErrRootMismatch", err)
	}
	resolved, err := store.Record(run.ID, RecordInput{Action: ActionResolveBlock, Kind: "recovery", Summary: "constraint fixed"})
	if err != nil {
		t.Fatalf("Record(resolve_block) error = %v", err)
	}
	if resolved.Status != StatusActive || resolved.Phase != PhaseGitSafety || resolved.AllowedActions[0] != ActionRecordGitSafety {
		t.Fatalf("resolved run = %+v, want original active checkpoint", resolved)
	}
}

func TestStoreCompletesAndAllowsANewRun(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Status == StatusActive {
		run, err = store.Record(run.ID, RecordInput{Action: run.AllowedActions[0], Kind: "checkpoint", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if run.Status != StatusCompleted || run.Phase != PhaseCompleted || len(run.AllowedActions) != 0 {
		t.Fatalf("terminal run = %+v, want completed without actions", run)
	}
	if _, err := store.Create(CreateInput{ID: "run-2", TargetMajor: 12}); err != nil {
		t.Fatalf("Create() after completed run error = %v", err)
	}
}
