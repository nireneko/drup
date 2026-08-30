package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/inventory"
	"github.com/nireneko/drup/internal/metrics"
	"github.com/nireneko/drup/internal/runstate"
)

func sampleReportData() *ReportData {
	return &ReportData{
		ProjectPath: "/path/to/drupal",
		TotalErrors: 5,
		Resolved: []ResolvedItem{
			{Module: "token", Type: "contrib", Detail: "Applied D11 patch"},
			{Module: "mymodule", Type: "custom", Detail: "Fixed deprecation in Service.php"},
		},
		Pending: []PendingItem{
			{Module: "oldmodule", Type: "contrib", Error: "No D11 release, no working patch", SuggestedAction: "Manual review required"},
		},
		TokenAccounting: TokenAccounting{
			Total: 15000,
			ByAgent: map[string]int{
				"drup-contrib": 8000,
				"drup-custom":  7000,
			},
		},
	}
}

func TestGenerateJSON(t *testing.T) {
	data := sampleReportData()
	result, err := GenerateJSON(data)
	if err != nil {
		t.Fatalf("GenerateJSON error: %v", err)
	}

	// Verify it's valid JSON.
	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Check key fields.
	if parsed["project_path"] != "/path/to/drupal" {
		t.Errorf("project_path = %v, want /path/to/drupal", parsed["project_path"])
	}
	if parsed["total_errors"].(float64) != 5 {
		t.Errorf("total_errors = %v, want 5", parsed["total_errors"])
	}

	resolved := parsed["resolved"].([]interface{})
	if len(resolved) != 2 {
		t.Errorf("len(resolved) = %d, want 2", len(resolved))
	}

	pending := parsed["pending"].([]interface{})
	if len(pending) != 1 {
		t.Errorf("len(pending) = %d, want 1", len(pending))
	}
}

func TestGenerateMarkdown(t *testing.T) {
	data := sampleReportData()
	result, err := GenerateMarkdown(data)
	if err != nil {
		t.Fatalf("GenerateMarkdown error: %v", err)
	}

	// Check sections exist.
	if !strings.Contains(result, "# Summary") {
		t.Error("missing Summary section")
	}
	if !strings.Contains(result, "# Resolved") {
		t.Error("missing Resolved section")
	}
	if !strings.Contains(result, "# Pending Human Review") {
		t.Error("missing Pending Human Review section")
	}
	if !strings.Contains(result, "# Token Usage") {
		t.Error("missing Token Usage section")
	}

	// Check content.
	if !strings.Contains(result, "token") {
		t.Error("missing token module in resolved")
	}
	if !strings.Contains(result, "oldmodule") {
		t.Error("missing oldmodule in pending")
	}
	if !strings.Contains(result, "15000") {
		t.Error("missing total token count")
	}
}

func TestGenerateJSON_EmptyReport(t *testing.T) {
	data := &ReportData{
		ProjectPath: "/path/to/drupal",
		TotalErrors: 0,
		Resolved:    []ResolvedItem{},
		Pending:     []PendingItem{},
	}

	result, err := GenerateJSON(data)
	if err != nil {
		t.Fatalf("GenerateJSON error: %v", err)
	}

	var parsed map[string]interface{}
	json.Unmarshal(result, &parsed)
	if parsed["total_errors"].(float64) != 0 {
		t.Errorf("total_errors = %v, want 0", parsed["total_errors"])
	}
}

// Task 4.3: Report includes pipeline metrics.
func TestGenerateJSON_WithMetrics(t *testing.T) {
	m := &metrics.Metrics{
		TotalDurationMS:  5000,
		StageDurations:   map[string]int64{"preflight": 1000, "scan": 2000},
		CommandsExecuted: 15,
		FilesModified:    3,
		Retries:          1,
		Interventions:    0,
	}
	data := &ReportData{
		ProjectPath:     "/path/to/drupal",
		TotalErrors:     0,
		Resolved:        []ResolvedItem{},
		Pending:         []PendingItem{},
		PipelineMetrics: m,
	}

	result, err := GenerateJSON(data)
	if err != nil {
		t.Fatalf("GenerateJSON error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	pm, ok := parsed["pipeline_metrics"]
	if !ok || pm == nil {
		t.Fatal("pipeline_metrics missing from JSON output")
	}
	pmMap := pm.(map[string]interface{})
	if pmMap["total_duration_ms"].(float64) != 5000 {
		t.Errorf("total_duration_ms = %v, want 5000", pmMap["total_duration_ms"])
	}
	if pmMap["commands_executed"].(float64) != 15 {
		t.Errorf("commands_executed = %v, want 15", pmMap["commands_executed"])
	}
}

func TestGenerateMarkdown_WithMetrics(t *testing.T) {
	m := &metrics.Metrics{
		TotalDurationMS:  5000,
		StageDurations:   map[string]int64{"preflight": 1000},
		CommandsExecuted: 10,
		FilesModified:    2,
		Retries:          0,
		Interventions:    1,
	}
	data := &ReportData{
		ProjectPath:     "/path",
		TotalErrors:     0,
		Resolved:        []ResolvedItem{},
		Pending:         []PendingItem{},
		PipelineMetrics: m,
	}

	result, err := GenerateMarkdown(data)
	if err != nil {
		t.Fatalf("GenerateMarkdown error: %v", err)
	}

	if !strings.Contains(result, "# Pipeline Metrics") {
		t.Error("missing Pipeline Metrics section in markdown")
	}
	if !strings.Contains(result, "5000") {
		t.Error("missing total duration in markdown")
	}
	if !strings.Contains(result, "10") {
		t.Error("missing commands executed in markdown")
	}
}

func TestBuildFromRun_RendersRestartStableSnapshotAndDoesNotCountPatchesAsTokens(t *testing.T) {
	run := runstate.Run{ID: "run-1", Root: "/project", InventoryBaseline: &inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "10.3", Source: "composer.lock"}, Patches: []inventory.Patch{{Package: "drupal/token", Description: "fix", URL: "https://example.test/fix", Source: "composer.json"}}}, InventoryFinal: &inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.0", Source: "composer.lock"}, Patches: []inventory.Patch{{Package: "drupal/token", Description: "fix", URL: "https://example.test/fix", Source: "composer.json"}}}, Evidence: []runstate.Evidence{{Kind: "checkpoint_commit", Summary: "commit", CommitHash: "abc123", CandidateHash: "candidate", Paths: []string{"composer.lock"}}, {Kind: "validation", Summary: "validated", ValidationHash: "validation", CandidateHash: "candidate", Target: "11", Paths: []string{"composer.lock"}}}, CheckpointHistory: []runstate.CheckpointPlan{{Paths: []string{"composer.lock"}, BackupID: "backup-1", CandidateHash: "candidate", Steps: []runstate.CheckpointStep{{Name: runstate.CheckpointStepBackup, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "backup-command", OutputHash: "backup-output", Paths: []string{"composer.lock"}}}, {Name: runstate.CheckpointStepValidation, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "command", OutputHash: "output", CandidateHash: "candidate", Paths: []string{"composer.lock"}}}}}}}
	data, err := BuildFromRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if data.TokenAccounting.Total != 0 {
		t.Fatalf("patches changed tokens: %#v", data.TokenAccounting)
	}
	first, _ := GenerateJSON(data)
	second, _ := GenerateJSON(data)
	if string(first) != string(second) {
		t.Fatalf("restart output differs:\n%s\n%s", first, second)
	}
	if len(data.Changes) != 1 || len(data.Changes[0].Evidence) != 2 || data.Changes[0].Evidence[0].CommitHash != "abc123" {
		t.Fatalf("changes lack persisted typed evidence: %#v", data.Changes)
	}
	md, err := GenerateMarkdown(data)
	if err != nil || !strings.Contains(md, "# Before") || !strings.Contains(md, "# Checkpoints") {
		t.Fatalf("markdown = %q err=%v", md, err)
	}
}

func TestBuildFromRun_RejectsChangesWithoutTheirTypedPersistedEvidence(t *testing.T) {
	baseline := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "10.3", Source: "composer.lock"}}
	final := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.0", Source: "composer.lock"}}
	base := runstate.Run{ID: "run-1", Root: "/project", InventoryBaseline: &baseline, InventoryFinal: &final}

	if _, err := BuildFromRun(base); err == nil {
		t.Fatal("BuildFromRun accepted a changed snapshot without evidence")
	}

	base.Evidence = []runstate.Evidence{
		{Kind: "unrelated", Summary: "not provenance", Paths: []string{"composer.lock"}},
	}
	if _, err := BuildFromRun(base); err == nil {
		t.Fatal("BuildFromRun accepted untyped evidence")
	}

	base.Evidence = []runstate.Evidence{
		{Kind: "checkpoint_commit", Summary: "commit", CommitHash: "abc123", CandidateHash: "candidate", Paths: []string{"composer.lock"}},
		{Kind: "validation", Summary: "validated", ValidationHash: "validation", CandidateHash: "candidate", Target: "11", Paths: []string{"composer.lock"}},
	}
	base.CheckpointHistory = []runstate.CheckpointPlan{{
		Paths: []string{"composer.lock"}, BackupID: "backup-1", CandidateHash: "candidate",
		Steps: []runstate.CheckpointStep{
			{Name: runstate.CheckpointStepBackup, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "backup-command", OutputHash: "backup-output", Paths: []string{"composer.lock"}}},
			{Name: runstate.CheckpointStepValidation, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "command", OutputHash: "output", CandidateHash: "candidate", Paths: []string{"composer.lock"}}},
		},
	}}
	if _, err := BuildFromRun(base); err != nil {
		t.Fatalf("BuildFromRun rejected complete typed evidence: %v", err)
	}
}

func TestBuildFromRun_RequiresPatchEvidenceForPatchDeltas(t *testing.T) {
	baseline := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.0", Source: "composer.lock"}}
	final := baseline
	final.Patches = []inventory.Patch{{Package: "drupal/token", Description: "D11 fix", URL: "https://example.test/token.patch", Source: "composer.json"}}
	run := runstate.Run{
		ID: "patch-run", Root: "/project", InventoryBaseline: &baseline, InventoryFinal: &final,
		Evidence: []runstate.Evidence{
			{Kind: "checkpoint_commit", CommitHash: "abc123", CandidateHash: "candidate", Paths: []string{"composer.json"}},
			{Kind: "validation", ValidationHash: "validation", CandidateHash: "candidate", Target: "11", Paths: []string{"composer.json"}},
		},
		CheckpointHistory: []runstate.CheckpointPlan{{TargetMajor: 11, Paths: []string{"composer.json"}, BackupID: "backup-1", CandidateHash: "candidate", Steps: []runstate.CheckpointStep{
			{Name: runstate.CheckpointStepBackup, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "backup-command", OutputHash: "backup-output", Paths: []string{"composer.json"}}},
			{Name: runstate.CheckpointStepValidation, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "command", OutputHash: "output", CandidateHash: "candidate", Paths: []string{"composer.json"}}},
		}}},
	}
	if _, err := BuildFromRun(run); err == nil {
		t.Fatal("BuildFromRun accepted a patch delta without patch evidence")
	}
	run.Evidence = append(run.Evidence, runstate.Evidence{Kind: "patch", Summary: "token D11 fix", Paths: []string{"composer.json"}})
	if _, err := BuildFromRun(run); err == nil {
		t.Fatal("BuildFromRun accepted partial patch evidence")
	}
	run.Evidence[len(run.Evidence)-1].CandidateHash = "different-candidate"
	run.Evidence[len(run.Evidence)-1].Target = "11"
	if _, err := BuildFromRun(run); err == nil {
		t.Fatal("BuildFromRun accepted discordant patch evidence")
	}
	run.Evidence[len(run.Evidence)-1].CandidateHash = "candidate"
	if _, err := BuildFromRun(run); err != nil {
		t.Fatalf("BuildFromRun rejected patch evidence bound to the patch delta: %v", err)
	}
}

func TestBuildFromRun_ExposesChangesForEveryInventoryCategory(t *testing.T) {
	baseline := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "10.3", Source: "composer.lock"}, PHP: inventory.Version{Version: "8.1", Source: "composer.json"}, Packages: []inventory.Package{{Name: "vendor/package", Version: "1.0", Source: "composer.lock"}}, Extensions: []inventory.Package{{Name: "drupal/token", Version: "1.0", Source: "composer.lock"}}, Patches: []inventory.Patch{{Package: "drupal/token", Description: "old", URL: "https://example.test/old.patch", Source: "composer.json"}}, Config: []inventory.File{{Path: "config/sync/system.site.yml", Digest: "old-config", Source: "filesystem"}}, Tests: []inventory.File{{Path: "tests/src/Unit/ExampleTest.php", Digest: "old-test", Source: "filesystem"}}}
	final := inventory.Inventory{SchemaVersion: inventory.SchemaVersion, Core: inventory.Version{Version: "11.0", Source: "composer.lock"}, PHP: inventory.Version{Version: "8.3", Source: "composer.json"}, Packages: []inventory.Package{{Name: "vendor/package", Version: "1.1", Source: "composer.lock"}}, Extensions: []inventory.Package{{Name: "drupal/token", Version: "1.1", Source: "composer.lock"}}, Patches: []inventory.Patch{{Package: "drupal/token", Description: "new", URL: "https://example.test/new.patch", Source: "composer.json"}}, Config: []inventory.File{{Path: "config/sync/system.site.yml", Digest: "new-config", Source: "filesystem"}}, Tests: []inventory.File{{Path: "tests/src/Unit/ExampleTest.php", Digest: "new-test", Source: "filesystem"}}}
	paths := []string{"composer.lock", "composer.json", "config/sync/system.site.yml", "tests/src/Unit/ExampleTest.php"}
	run := runstate.Run{ID: "all-categories", Root: "/project", InventoryBaseline: &baseline, InventoryFinal: &final, Evidence: []runstate.Evidence{{Kind: "checkpoint_commit", CommitHash: "abc123", CandidateHash: "candidate", Paths: paths}, {Kind: "validation", ValidationHash: "validation", CandidateHash: "candidate", Target: "11", Paths: paths}, {Kind: "patch", CandidateHash: "candidate", Target: "11", Paths: []string{"composer.json"}}}, CheckpointHistory: []runstate.CheckpointPlan{{TargetMajor: 11, Paths: paths, BackupID: "backup-1", CandidateHash: "candidate", Steps: []runstate.CheckpointStep{{Name: runstate.CheckpointStepBackup, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "backup-command", OutputHash: "backup-output", Paths: paths}}, {Name: runstate.CheckpointStepValidation, Status: runstate.CheckpointStepSucceeded, Evidence: &runstate.CheckpointStepEvidence{CommandHash: "command", OutputHash: "output", CandidateHash: "candidate", Paths: paths}}}}}}
	data, err := BuildFromRun(run)
	if err != nil {
		t.Fatal(err)
	}
	if len(data.Changes) != 7 {
		t.Fatalf("Changes = %#v", data.Changes)
	}
	for _, kind := range []string{"core", "php", "package", "extension", "patch", "config", "test"} {
		found := false
		for _, change := range data.Changes {
			found = found || change.Kind == kind && change.Before != "" && change.After != "" && len(change.Evidence) >= 2
		}
		if !found {
			t.Errorf("ReportData.Changes missing %s before/after/evidence: %#v", kind, data.Changes)
		}
	}
}
