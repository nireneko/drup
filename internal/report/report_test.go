package report

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/metrics"
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
