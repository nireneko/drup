package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nireneko/drup/internal/runstate"
)

func TestContribPlanPersistsDeterministicReadOnlyPlan(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "composer.lock"), []byte(`{"packages":[{"name":"drupal/root","version":"1.0.0"},{"name":"drupal/a","version":"1.0.0"},{"name":"drupal/b","version":"1.0.0"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"drupal/a":"^1","drupal/b":"^2"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store := runstate.NewStore(dir)
	run, err := store.Create(runstate.CreateInput{ID: "plan-run", TargetMajor: 11, Scope: []string{"all"}})
	if err != nil {
		t.Fatal(err)
	}
	orig := contribPlanComposer
	contribPlanComposer = func(string) ([]byte, error) {
		return []byte(`{"installed":[{"name":"drupal/a","latest":"1.0.1"},{"name":"drupal/b","latest":"2.0.0"}]}`), nil
	}
	origWhyNot := contribPlanWhyNot
	contribPlanWhyNot = func(string, string, string) error { return nil }
	t.Cleanup(func() { contribPlanComposer = orig; contribPlanWhyNot = origWhyNot })
	raw, err := realHandleContribPlan(json.RawMessage(`{"project_path":` + jsonStr(dir) + `,"run_id":"plan-run"}`))
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Success bool `json:"success"`
		Plan    struct {
			Groups []struct {
				Kind    string   `json:"kind"`
				Targets []string `json:"targets"`
			} `json:"groups"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		t.Fatal(err)
	}
	if !response.Success || len(response.Plan.Groups) != 2 || len(response.Plan.Groups[1].Targets) != 1 || response.Plan.Groups[1].Targets[0] != "drupal/b" {
		t.Fatalf("response = %s", raw)
	}
	persisted, err := store.Get(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(persisted.ContribPlan) == 0 {
		t.Fatal("contrib plan was not persisted")
	}
}

func TestContribPlanRejectsInvalidComposerJSONBeforePersisting(t *testing.T) {
	if _, err := parseComposerOutdated([]byte(`not json`)); err == nil {
		t.Fatal("invalid Composer JSON was accepted")
	}
}
