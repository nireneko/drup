package contribplan

import "testing"

func TestBuildIsDeterministicAndIsolatesMajorGroups(t *testing.T) {
	lock, err := ParseLock([]byte(`{"packages":[
		{"name":"drupal/a","version":"1.0.0","require":{"drupal/b":"^1"}},
		{"name":"drupal/b","version":"1.0.0"},
		{"name":"drupal/c","version":"1.0.0"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	input := Input{Lock: lock, Outdated: map[string]string{"drupal/a": "1.0.1", "drupal/b": "1.1.0", "drupal/c": "2.0.0"}}
	first, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Build(input)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(first, second) {
		t.Fatalf("identical input produced different plans: %#v != %#v", first, second)
	}
	if got := first.Groups[2]; len(got.Targets) != 1 || got.Targets[0] != "drupal/c" {
		t.Fatalf("major group = %#v, want only drupal/c", got)
	}
	if first.Groups[0].Kind != GroupPatch || first.Groups[1].Kind != GroupMinor {
		t.Fatalf("groups = %#v, want patch then minor then major", first.Groups)
	}
}

func TestBuildReportsRootAndConstraintForDependencyConflict(t *testing.T) {
	lock, err := ParseLock([]byte(`{"packages":[
		{"name":"drupal/root","version":"1.0.0","require":{"drupal/blocked":"^1.0"}},
		{"name":"drupal/blocked","version":"1.0.0"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Build(Input{Lock: lock, Outdated: map[string]string{"drupal/blocked": "2.0.0"}})
	if err != nil {
		t.Fatal(err)
	}
	item := plan.Items["drupal/blocked"]
	if item.Class != BlockedDependency || len(item.Blockers) != 1 {
		t.Fatalf("item = %#v", item)
	}
	if got := item.Blockers[0]; got.Root != "drupal/root" || got.Constraint != "^1.0" {
		t.Fatalf("blocker = %#v", got)
	}
}

func TestParseLockRejectsMissingPackages(t *testing.T) {
	if _, err := ParseLock([]byte(`{"packages":[]}`)); err == nil {
		t.Fatal("ParseLock accepted empty lock")
	}
}
