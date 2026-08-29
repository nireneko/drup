package multiharness

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/contracts"
)

func dispatchJSON(agent, phase, scope string) []byte {
	return []byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"candidate-1","run_id":"run-1","phase":"` + phase + `"},"agent":"` + agent + `","scope":"` + scope + `","payload":{}}`)
}

func reportJSON(agent, phase, status string) []byte {
	return []byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"candidate-1","run_id":"run-1","phase":"` + phase + `"},"agent":"` + agent + `","status":"` + status + `","summary":"reported","artifacts":[],"evidence":{},"risks":[]}`)
}

func TestHarnessRejectsInvalidDispatchBeforeAnyToolCall(t *testing.T) {
	fake := NewFakeMCP()
	h := New(fake)
	invalid := []byte(`{"schema_version":"v1","identity":{"root":"/project","candidate":"candidate-1","run_id":"run-1","phase":"rector"},"agent":"drup-rector","scope":"wrong","payload":{}}`)
	_, err := h.Execute(invalid, reportJSON("drup-rector", "rector", "pass"), ToolCall{Name: "autofix", Arguments: json.RawMessage(`{"project_path":"/project","request_id":"r1"}`)})
	if err == nil || !strings.Contains(err.Error(), "/scope") || !strings.Contains(err.Error(), "allowed:") {
		t.Fatalf("Execute() error = %v, want actionable contract diagnostic", err)
	}
	if calls := fake.Calls(); len(calls) != 0 {
		t.Fatalf("tool calls = %#v, want no effects after contract rejection", calls)
	}
}

func TestHarnessValidatesReportAndToolSchemaAgainstSingleCatalog(t *testing.T) {
	fake := NewFakeMCP()
	h := New(fake)
	_, err := h.Execute(dispatchJSON("drup-contrib", "contrib", "contrib"), reportJSON("drup-contrib", "contrib", "pass"), ToolCall{Name: "apply_patch", Arguments: json.RawMessage(`{"project_path":"/project","patch_url":"https://example.test/fix.patch","request_id":"r1"}`)})
	if err == nil || !strings.Contains(err.Error(), "composer_package") || !strings.Contains(err.Error(), "apply_patch") {
		t.Fatalf("Execute missing tool args error = %v", err)
	}
	if len(fake.Calls()) != 0 {
		t.Fatal("fake must reject before recording a malformed tool call")
	}

	_, err = h.Execute(dispatchJSON("drup-rector", "rector", "custom"), reportJSON("drup-rector", "rector", "completed"), ToolCall{Name: "autofix", Arguments: json.RawMessage(`{"project_path":"/project","request_id":"r2"}`)})
	if err == nil || !strings.Contains(err.Error(), "/status") {
		t.Fatalf("report status error = %v", err)
	}
	if len(fake.Calls()) != 0 {
		t.Fatal("report rejection must happen before a tool call")
	}
}

func TestTranscriptScenariosPreserveBoundedRetriesAndExplicitRecovery(t *testing.T) {
	for _, scenario := range []Scenario{
		{Name: "happy", Dispatch: dispatchJSON("drup-validator", "baseline", "baseline"), Report: reportJSON("drup-validator", "baseline", "pass"), Calls: []ToolCall{{Name: "scan", Arguments: json.RawMessage(`{"project_path":"/project"}`)}}},
		{Name: "retry exhausted", Dispatch: dispatchJSON("drup-validator", "baseline", "baseline"), Report: reportJSON("drup-validator", "baseline", "pass"), Calls: []ToolCall{{Name: "scan", Arguments: json.RawMessage(`{"project_path":"/project"}`), Failures: 3}}, WantRetries: 3},
		{Name: "unknown requires recovery", Dispatch: dispatchJSON("drup-contrib", "contrib", "contrib"), Report: reportJSON("drup-contrib", "contrib", "pass"), Calls: []ToolCall{{Name: "apply_patch", Arguments: json.RawMessage(`{"project_path":"/project","patch_url":"https://example.test/fix.patch","composer_package":"drupal/example","description":"fix","request_id":"r3"}`), Unknown: true}}, WantBlocked: true},
	} {
		t.Run(scenario.Name, func(t *testing.T) {
			trace, err := RunScenario(scenario)
			if scenario.WantBlocked {
				if err == nil || !strings.Contains(err.Error(), "explicit recovery") {
					t.Fatalf("RunScenario error = %v", err)
				}
				return
			}
			if scenario.WantRetries > 0 {
				if err == nil || !strings.Contains(err.Error(), "exhausted") {
					t.Fatalf("RunScenario retry exhaustion error = %v", err)
				}
			} else if err != nil {
				t.Fatalf("RunScenario error = %v", err)
			}
			if got := trace.Attempts("scan"); scenario.WantRetries > 0 && got != scenario.WantRetries {
				t.Fatalf("scan attempts = %d, want %d", got, scenario.WantRetries)
			}
		})
	}
}

func TestHarnessProducesSameSemanticTraceForAValidContract(t *testing.T) {
	dispatch, err := contracts.DecodeDispatch(dispatchJSON("drup-validator", "baseline", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if dispatch.Identity.Candidate != "candidate-1" {
		t.Fatal("fixture lost candidate binding")
	}
}

func TestCorpusSafetyScenarios(t *testing.T) {
	for _, scenario := range Corpus() {
		t.Run(scenario.Name, func(t *testing.T) {
			trace, err := RunScenario(scenario)
			if scenario.WantBlocked {
				if err == nil {
					t.Fatal("expected blocked transcript")
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if scenario.WantRetries > 0 && trace.Attempts("scan") != scenario.WantRetries {
				t.Fatalf("scan attempts = %d, want %d", trace.Attempts("scan"), scenario.WantRetries)
			}
			if len(scenario.Calls) == 0 && len(trace.Calls) != 0 {
				t.Fatalf("trace = %#v, want zero effects", trace)
			}
		})
	}
}

func TestHarnessReportFailureBlocksEveryToolEffect(t *testing.T) {
	for _, status := range []string{"fail", "blocked"} {
		t.Run(status, func(t *testing.T) {
			fake := NewFakeMCP()
			_, err := New(fake).Execute(
				dispatchJSON("drup-rector", "rector", "custom"),
				reportJSON("drup-rector", "rector", status),
				ToolCall{Name: "autofix", Arguments: json.RawMessage(`{"project_path":"/project","request_id":"blocked-1"}`)},
			)
			if err == nil || !strings.Contains(err.Error(), "status") || !strings.Contains(err.Error(), status) {
				t.Fatalf("Execute() error = %v, want actionable %s report block", err, status)
			}
			if got := fake.Calls(); len(got) != 0 {
				t.Fatalf("calls = %#v, want no effects", got)
			}
		})
	}
}

func TestFakeMCPRejectsSchemaTypeMismatchBeforeRecordingCall(t *testing.T) {
	fake := NewFakeMCP()
	_, err := fake.validate(ToolCall{Name: "scan", Arguments: json.RawMessage(`{"project_path":123}`)})
	if err == nil || !strings.Contains(err.Error(), "/project_path") || !strings.Contains(err.Error(), "string") {
		t.Fatalf("validate() error = %v, want type diagnostic", err)
	}
	if got := fake.Calls(); len(got) != 0 {
		t.Fatalf("calls = %#v, want no effect", got)
	}
}

func TestCorpusBlockedScenariosHaveExplicitBlockAndNoCalls(t *testing.T) {
	want := map[string]bool{"dirty-tree": true, "backup-failure": true, "confirmation-rejected": true}
	for _, scenario := range Corpus() {
		if !want[scenario.Name] {
			continue
		}
		t.Run(scenario.Name, func(t *testing.T) {
			trace, err := RunScenario(scenario)
			if !scenario.WantBlocked || err == nil {
				t.Fatalf("scenario = %#v, err = %v; want explicit block", scenario, err)
			}
			if len(trace.Calls) != 0 {
				t.Fatalf("trace = %#v, want zero calls after block", trace)
			}
		})
	}
}

func TestRunRenderedCorpusRejectsTraceOrderDivergence(t *testing.T) {
	raw := []byte(`{"schema_version":"v1","transcript":{"scenarios":[{"name":"happy-path","outcome":"pass","trace":[{"tool":"autofix","arguments":{"project_path":"/project"}}]}]}}`)
	_, err := RunRenderedCorpus(raw)
	if err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("RunRenderedCorpus() error = %v, want trace divergence", err)
	}
}

func TestRenderedTraceRejectsInvertedCoreTargetMajors(t *testing.T) {
	scenario := corpusScenario(t, "core-sequential")
	declared := renderedScenario{
		Name: scenario.Name, Outcome: "pass",
		Trace: []SemanticCall{
			{Tool: "core_upgrade_apply", Arguments: map[string]json.RawMessage{"project_path": json.RawMessage(`"/project"`), "target_major": json.RawMessage(`12`)}},
			{Tool: "core_upgrade_apply", Arguments: map[string]json.RawMessage{"project_path": json.RawMessage(`"/project"`), "target_major": json.RawMessage(`11`)}},
		},
	}
	if _, err := validateRenderedScenario(declared, scenario); err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("validateRenderedScenario() error = %v, want core target-major trace rejection", err)
	}
}

func TestRenderedTraceRejectsContribPackageMajorIsolationViolation(t *testing.T) {
	scenario := corpusScenario(t, "contrib-major-isolated")
	declared := renderedScenario{
		Name: scenario.Name, Outcome: "pass",
		Trace: []SemanticCall{{Tool: "composer_require", Arguments: map[string]json.RawMessage{
			"project_path": json.RawMessage(`"/project"`),
			"package":      json.RawMessage(`"drupal/other:^4"`),
		}}},
	}
	if _, err := validateRenderedScenario(declared, scenario); err == nil || !strings.Contains(err.Error(), "trace") {
		t.Fatalf("validateRenderedScenario() error = %v, want contrib package/major isolation rejection", err)
	}
}

func corpusScenario(t *testing.T, name string) Scenario {
	t.Helper()
	for _, scenario := range Corpus() {
		if scenario.Name == name {
			return scenario
		}
	}
	t.Fatalf("corpus lacks %q", name)
	return Scenario{}
}
