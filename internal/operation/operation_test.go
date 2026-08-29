package operation

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStore_PersistsLifecycleAtomically(t *testing.T) {
	project := t.TempDir()
	store := NewStore(project)

	op, err := store.Start("request-1", "generate_report", "fingerprint-1")
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if op.State != StateStarted {
		t.Fatalf("State = %q, want %q", op.State, StateStarted)
	}
	if _, err := store.Complete(op.RequestID, []byte(`{"success":true}`)); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	loaded, err := NewStore(project).FindRequest("request-1")
	if err != nil {
		t.Fatalf("FindRequest() error = %v", err)
	}
	if loaded.State != StateCompleted || string(loaded.Response) != `{"success":true}` {
		t.Fatalf("loaded = %#v, want completed operation with response", loaded)
	}
	if _, err := os.Stat(filepath.Join(project, ".drup", "operations.v1.json")); err != nil {
		t.Fatalf("ledger file missing: %v", err)
	}
}

func TestStore_FailsClosedForCorruptOrUnsupportedLedger(t *testing.T) {
	project := t.TempDir()
	path := filepath.Join(project, ".drup")
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, contents := range []string{"not json", `{"version":999,"operations":[]}`} {
		t.Run(contents, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(path, "operations.v1.json"), []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := NewStore(project).FindRequest("request-1")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("FindRequest() error = %v, want ErrUnavailable", err)
			}
		})
	}
}

func TestStore_UnknownBlocksEquivalentUntilEvidenceReconciles(t *testing.T) {
	store := NewStore(t.TempDir())
	op, err := store.Start("request-1", "generate_report", "same-operation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Unknown(op.RequestID, "context deadline exceeded"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start("request-2", "generate_report", "same-operation"); !errors.Is(err, ErrEquivalentUnknown) {
		t.Fatalf("Start equivalent error = %v, want ErrEquivalentUnknown", err)
	}
	if _, err := store.Reconcile(op.RequestID, Evidence{Kind: "report_file", Value: "drup-report.json"}, []byte(`{"success":true}`)); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if _, err := store.Start("request-2", "generate_report", "same-operation"); err != nil {
		t.Fatalf("Start after reconciliation error = %v", err)
	}
}
