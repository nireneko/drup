package runstate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/inventory"
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

func TestCheckpointPlanPersistsIdentityAndMajorCardinality(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "checkpoint", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseContribMajor {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: PhaseContribMajor, TargetMajor: 11, Targets: []string{"drupal/a", "drupal/b"}, Paths: []string{"composer.lock"}})
	if err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("multi-target contrib major plan error = %v, want cardinality refusal", err)
	}
	run, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: PhaseContribMajor, TargetMajor: 11, Targets: []string{"drupal/a"}, Paths: []string{"composer.lock"}, RequireConfigExport: true})
	if err != nil {
		t.Fatal(err)
	}
	if run.CheckpointPlan == nil || run.CheckpointPlan.RequireConfigExport != true {
		t.Fatalf("persisted plan = %+v, want required config export", run.CheckpointPlan)
	}
	if _, err := store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: PhaseContribMajor, TargetMajor: 11, Targets: []string{"drupal/b"}, Paths: []string{"composer.lock"}, RequireConfigExport: true}); err == nil {
		t.Fatal("replacing checkpoint target unexpectedly succeeded")
	}
}

func TestCheckpointPlanCanonicalizesEvidencePathsBeforeAnyEffect(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "canonical-paths", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseContribPatch {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: run.Phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{" z.txt", "a.txt "}})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := run.CheckpointPlan.Paths, []string{"a.txt", "z.txt"}; !sameStrings(got, want) {
		t.Fatalf("checkpoint evidence paths = %v, want deterministic %v", got, want)
	}
	duplicateStore := NewStore(t.TempDir())
	duplicateRun, err := duplicateStore.Create(CreateInput{ID: "duplicate-paths", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for duplicateRun.Phase != PhaseContribPatch {
		duplicateRun, err = advanceRunForCheckpointTest(duplicateStore, duplicateRun)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := duplicateStore.BeginCheckpointPlan(duplicateRun.ID, CheckpointPlanInput{Phase: duplicateRun.Phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"a.txt", " a.txt "}}); err == nil {
		t.Fatal("duplicate canonical checkpoint paths unexpectedly accepted")
	}
}

func TestCheckpointPlanOnlyResumesUnavailableSteps(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "resume", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseContribPatch {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: run.Phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.lock"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordCheckpointStep(run.ID, CheckpointStepUpdate, CheckpointStepRunning, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordCheckpointStep(run.ID, CheckpointStepUpdate, CheckpointStepUnavailable, &CheckpointStepEvidence{ExitCode: -1}); err != nil {
		t.Fatal(err)
	}
	run, err = store.ResumeUnavailableCheckpoint(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range run.CheckpointPlan.Steps {
		if step.Name == CheckpointStepUpdate && step.Status != CheckpointStepPending {
			t.Fatalf("resumed update = %s, want pending", step.Status)
		}
	}
	if _, err := store.ResumeUnavailableCheckpoint(run.ID); err == nil {
		t.Fatal("second resume without unavailable step unexpectedly succeeded")
	}
}

func TestCheckpointStepsCannotValidateBeforeEarlierMutations(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "ordered-steps", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseContribPatch {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: run.Phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.lock"}, RequireConfigExport: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.RecordCheckpointStep(run.ID, CheckpointStepValidation, CheckpointStepRunning, nil)
	if err == nil || !strings.Contains(err.Error(), "prior") {
		t.Fatalf("out-of-order validation error = %v, want mutation-order refusal", err)
	}
}

func TestOperationalPhaseProgressRequiresCompletedMatchingCheckpoint(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "closed-gate", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseCustomTheme {
		run, err = store.Record(run.ID, RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.Record(run.ID, RecordInput{Action: ActionRecordCustomTheme, Kind: "validation", Summary: "unbound", ValidationHash: "v", CandidateHash: "candidate", Paths: []string{"composer.lock"}, Target: "11"})
	if err == nil || !strings.Contains(err.Error(), "completed checkpoint") {
		t.Fatalf("direct phase progress error = %v, want fail-closed checkpoint refusal", err)
	}
}

func TestOperationalPhaseProgressRequiresIndependentValidationEvidence(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "validation-evidence", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseCustomTheme {
		run, err = store.Record(run.ID, RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
		if err != nil {
			t.Fatal(err)
		}
	}
	run, err = store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: run.Phase, TargetMajor: 11, Targets: []string{"custom"}, Paths: []string{"composer.lock"}, RequireConfigExport: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range run.CheckpointPlan.Steps {
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, CheckpointStepRunning, nil); err != nil {
			t.Fatal(err)
		}
		if step.Name == CheckpointStepBackup {
			if _, err = store.BindCheckpointBackup(run.ID, "backup-evidence"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, CheckpointStepSucceeded, &CheckpointStepEvidence{Paths: []string{"composer.lock"}}); err != nil {
			t.Fatal(err)
		}
	}
	_, err = store.CompleteCheckpointPlan(run.ID, "candidate")
	if err == nil || !strings.Contains(err.Error(), "independent validation") {
		t.Fatalf("checkpoint completion with missing validator evidence = %v, want refusal", err)
	}
}

func TestCheckpointPlanUpdatesOnlyContribPhases(t *testing.T) {
	for _, tt := range []struct {
		phase      Phase
		wantUpdate bool
	}{
		{PhaseCustomTheme, false}, {PhaseContribPatch, true}, {PhaseContribMinor, true}, {PhaseContribMajor, true}, {PhaseCoreLoop, false}, {PhaseCleanup, false},
	} {
		t.Run(string(tt.phase), func(t *testing.T) {
			plan := checkpointPlanFromInput(CheckpointPlanInput{Phase: tt.phase, TargetMajor: 11, Targets: []string{"drupal/example"}, Paths: []string{"composer.lock"}})
			gotUpdate := false
			for _, step := range plan.Steps {
				gotUpdate = gotUpdate || step.Name == CheckpointStepUpdate
			}
			if gotUpdate != tt.wantUpdate {
				t.Fatalf("update step = %v, want %v", gotUpdate, tt.wantUpdate)
			}
		})
	}
}

func advanceRunForCheckpointTest(store *Store, run Run) (Run, error) {
	if !checkpointPhase(run.Phase) {
		return store.Record(run.ID, RecordInput{Action: run.AllowedActions[0], Kind: "check", Summary: "passed"})
	}
	targets := []string{"drupal/example"}
	planRun, err := store.BeginCheckpointPlan(run.ID, CheckpointPlanInput{Phase: run.Phase, TargetMajor: run.TargetMajor, Targets: targets, Paths: []string{"composer.lock"}, RequireConfigExport: true})
	if err != nil {
		return Run{}, err
	}
	candidate := "candidate-" + string(run.Phase)
	for _, step := range planRun.CheckpointPlan.Steps {
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, CheckpointStepRunning, nil); err != nil {
			return Run{}, err
		}
		if step.Name == CheckpointStepBackup {
			if _, err = store.BindCheckpointBackup(run.ID, "backup-"+string(run.Phase)); err != nil {
				return Run{}, err
			}
		}
		evidence := &CheckpointStepEvidence{CommandHash: "command-" + string(step.Name), OutputHash: "output-" + string(step.Name), CandidateHash: candidate, Paths: []string{"composer.lock"}}
		if _, err = store.RecordCheckpointStep(run.ID, step.Name, CheckpointStepSucceeded, evidence); err != nil {
			return Run{}, err
		}
	}
	if _, err = store.CompleteCheckpointPlan(run.ID, candidate); err != nil {
		return Run{}, err
	}
	return store.Record(run.ID, RecordInput{Action: run.AllowedActions[0], Kind: "validation", Summary: "independent validation", ValidationHash: "validation-" + string(run.Phase), CandidateHash: candidate, Paths: []string{"composer.lock"}, Target: strconv.Itoa(run.TargetMajor)})
}

func TestStoreCompletesAndAllowsANewRun(t *testing.T) {
	store := NewStore(t.TempDir())
	run, err := store.Create(CreateInput{ID: "run-1", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for run.Status == StatusActive {
		run, err = advanceRunForCheckpointTest(store, run)
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

func TestInventorySnapshots_PersistAtomicallyAndRejectIncompleteReport(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	run, err := store.Create(CreateInput{ID: "inventory-run", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	baseline := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "10.3", Source: "composer.lock"}}
	if _, err := store.RecordInventoryBaseline(run.ID, baseline); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("RecordInventoryBaseline() outside authorized phase error = %v, want ErrInvalidTransition", err)
	}
	for run.Phase != PhaseBaseline {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.RecordInventoryBaseline(run.ID, baseline); err != nil {
		t.Fatalf("RecordInventoryBaseline() error = %v", err)
	}
	loaded, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.InventoryBaseline == nil || loaded.InventoryBaseline.Core.Version != "10.3" {
		t.Fatalf("baseline not persisted: %#v", loaded.InventoryBaseline)
	}
	if _, err := store.InventoryReport(run.ID); !errors.Is(err, ErrInventoryIncomplete) {
		t.Fatalf("InventoryReport incomplete error = %v", err)
	}
	run, err = advanceRunForCheckpointTest(store, run)
	if err != nil {
		t.Fatal(err)
	}
	for run.Phase != PhaseReport {
		run, err = advanceRunForCheckpointTest(store, run)
		if err != nil {
			t.Fatal(err)
		}
	}
	final := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.0", Source: "composer.lock"}}
	if _, err := store.RecordInventoryFinal(run.ID, final); err != nil {
		t.Fatalf("RecordInventoryFinal() error = %v", err)
	}
	// A retry after a restart is safe only for the identical immutable snapshot.
	restarted := NewStore(root)
	if _, err := restarted.RecordInventoryFinal(run.ID, final); err != nil {
		t.Fatalf("RecordInventoryFinal() identical retry error = %v", err)
	}
	if _, err := restarted.RecordInventoryFinal(run.ID, inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.1", Source: "composer.lock"}}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("RecordInventoryFinal() overwrite error = %v, want ErrInvalidTransition", err)
	}
	persisted, err := restarted.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.InventoryFinal == nil || persisted.InventoryFinal.Core.Version != "11.0" {
		t.Fatalf("final snapshot was overwritten: %#v", persisted.InventoryFinal)
	}
	got, err := store.InventoryReport(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Delta) != 1 || got.Delta[0].Before != "10.3" || got.Delta[0].After != "11.0" {
		t.Fatalf("stable delta = %#v", got.Delta)
	}
}

func TestValidateMutationBlocksIncompleteRestoreJournal(t *testing.T) {
	root := t.TempDir()
	store := NewStore(root)
	run, err := store.Create(CreateInput{ID: "restore-interlock", TargetMajor: 11})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []Action{ActionRecordGitSafety, ActionRecordEnvironment} {
		run, err = store.Record(run.ID, RecordInput{Action: action, Kind: "check", Summary: "ok"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".drup", "restores"), 0o700); err != nil {
		t.Fatal(err)
	}
	journal := `{"version":1,"id":"restore-1","backup_id":"backup-1","state":"recovery_required","database_mode":"non_atomic","continuation":"reconcile","updated_at":"2026-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(root, ".drup", "restores", "restore-1.json"), []byte(journal), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ValidateMutation(run.ID, root, "composer_require"); !errors.Is(err, ErrMutationNotAllowed) || !strings.Contains(err.Error(), "incomplete restore") {
		t.Fatalf("ValidateMutation() error = %v, want incomplete restore interlock", err)
	}
}
