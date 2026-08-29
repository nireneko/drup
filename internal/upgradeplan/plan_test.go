package upgradeplan

import "testing"

func TestBuild_CreatesConsecutiveStepsForMultiMajorUpgrade(t *testing.T) {
	current, err := NewMajor(9)
	if err != nil {
		t.Fatal(err)
	}
	target, err := NewMajor(11)
	if err != nil {
		t.Fatal(err)
	}

	plan, err := Build(current, target, []Metadata{
		{From: 9, To: 10, CatalogID: "9-to-10"},
		{From: 10, To: 11, CatalogID: "10-to-11"},
	})
	if err != nil {
		t.Fatalf("Build() error: %v", err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("len(Steps) = %d, want 2", len(plan.Steps))
	}
	if got, want := plan.Steps[0].From, Major(9); got != want {
		t.Errorf("first step From = %d, want %d", got, want)
	}
	if got, want := plan.Steps[0].To, Major(10); got != want {
		t.Errorf("first step To = %d, want %d", got, want)
	}
	if got, want := plan.Steps[1].CatalogID, "10-to-11"; got != want {
		t.Errorf("second step CatalogID = %q, want %q", got, want)
	}
}

func TestBuild_FailsClosedWhenARequiredStepHasNoMetadata(t *testing.T) {
	_, err := Build(10, 12, []Metadata{{From: 10, To: 11, CatalogID: "10-to-11"}})
	if err == nil {
		t.Fatal("Build() succeeded without 11-to-12 metadata")
	}
}

func TestBuild_NoOpAndRejectsDowngrades(t *testing.T) {
	plan, err := Build(11, 11, nil)
	if err != nil {
		t.Fatalf("Build() no-op error: %v", err)
	}
	if !plan.NoOp() {
		t.Error("plan should be a no-op at the target")
	}
	if _, err := Build(12, 11, nil); err == nil {
		t.Error("Build() accepted downgrade")
	}
}
