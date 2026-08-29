// Package multiharness provides a deterministic, effect-free contract harness
// for agent transcript tests. It deliberately does not implement workflow
// transitions: production session/audit/backup code remains authoritative.
package multiharness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/nireneko/drup/internal/contracts"
	"github.com/nireneko/drup/internal/mcp"
)

const maxReadOnlyAttempts = 3

// ToolCall is a declared MCP request in a transcript. Failures and Unknown
// model deterministic server outcomes without touching a project.
type ToolCall struct {
	Name      string
	Arguments json.RawMessage
	Failures  int
	Unknown   bool
}

// Call records one semantically valid call attempt.
type Call struct {
	Name      string
	Arguments json.RawMessage
}

// SemanticCall is the stable identity used to compare transcript traces. It
// intentionally records only effect-relevant arguments, never request IDs or
// arbitrary descriptive text. ToolSpecs remains the schema authority.
type SemanticCall struct {
	Tool      string                     `json:"tool"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

func semanticCall(call Call) (SemanticCall, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(call.Arguments, &fields); err != nil {
		return SemanticCall{}, fmt.Errorf("semantic arguments for %q: %w", call.Name, err)
	}
	keys := []string{"project_path"}
	switch call.Name {
	case "core_upgrade_apply":
		keys = append(keys, "target_major")
	case "apply_patch":
		keys = append(keys, "composer_package")
	case "composer_require":
		keys = append(keys, "package")
	}
	args := make(map[string]json.RawMessage)
	for _, key := range keys {
		raw, ok := fields[key]
		if !ok {
			continue
		}
		canonical, err := canonicalJSON(raw)
		if err != nil {
			return SemanticCall{}, fmt.Errorf("semantic argument /%s for %q: %w", key, call.Name, err)
		}
		args[key] = canonical
	}
	return SemanticCall{Tool: call.Name, Arguments: args}, nil
}

func canonicalJSON(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(value)
	return json.RawMessage(canonical), err
}

// FakeMCP validates calls against mcp.ToolSpecs. It intentionally reads the
// production catalog instead of maintaining a second, drifting schema.
type FakeMCP struct {
	specs map[string]mcp.ToolSpec
	calls []Call
}

func NewFakeMCP() *FakeMCP {
	specs := make(map[string]mcp.ToolSpec)
	for _, spec := range mcp.ToolSpecs() {
		specs[spec.Name] = spec
	}
	return &FakeMCP{specs: specs}
}

func (f *FakeMCP) Calls() []Call { return append([]Call(nil), f.calls...) }

func (f *FakeMCP) validate(call ToolCall) (mcp.ToolSpec, error) {
	spec, ok := f.specs[call.Name]
	if !ok {
		return mcp.ToolSpec{}, fmt.Errorf("MCP tool %q is not present in mcp.ToolSpecs", call.Name)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(call.Arguments, &fields); err != nil {
		return mcp.ToolSpec{}, fmt.Errorf("MCP tool %q arguments: %w", call.Name, err)
	}
	for _, required := range spec.Required {
		if raw, ok := fields[required]; !ok || string(raw) == "null" || string(raw) == `""` {
			return mcp.ToolSpec{}, fmt.Errorf("MCP tool %q invalid at /%s: required by ToolSpecs", call.Name, required)
		}
	}
	for field, raw := range fields {
		property, ok := spec.Properties[field]
		if !ok {
			return mcp.ToolSpec{}, fmt.Errorf("MCP tool %q invalid at /%s: unknown field; allowed: %s", call.Name, field, propertyNames(spec))
		}
		if err := validateJSONType(raw, property.Type); err != nil {
			return mcp.ToolSpec{}, fmt.Errorf("MCP tool %q invalid at /%s: %w", call.Name, field, err)
		}
	}
	return spec, nil
}

func propertyNames(spec mcp.ToolSpec) string {
	names := make([]string, 0, len(spec.Properties))
	for name := range spec.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Sprintf("%v", names)
}

func validateJSONType(raw json.RawMessage, want string) error {
	if bytes.Equal(raw, []byte("null")) {
		return fmt.Errorf("must be %s, got null", want)
	}
	switch want {
	case "string":
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("must be string")
		}
	case "boolean":
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("must be boolean")
		}
	case "integer":
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var value json.Number
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("must be integer")
		}
		if _, err := value.Int64(); err != nil {
			return fmt.Errorf("must be integer")
		}
	case "array":
		var value []json.RawMessage
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("must be array")
		}
	default:
		return fmt.Errorf("unsupported ToolSpecs JSON type %q", want)
	}
	return nil
}

// Harness accepts a dispatch/report pair and only then allows its declared
// call. Invalid communication therefore produces zero fake effects.
type Harness struct{ fake *FakeMCP }

func New(fake *FakeMCP) *Harness { return &Harness{fake: fake} }

func (h *Harness) Execute(dispatchRaw, reportRaw []byte, call ToolCall) (Trace, error) {
	dispatch, err := contracts.DecodeDispatch(dispatchRaw)
	if err != nil {
		return Trace{}, err
	}
	report, err := contracts.DecodeAgentReport(dispatch, reportRaw)
	if err != nil {
		return Trace{}, err
	}
	if err := reportAllowsTools(report); err != nil {
		return Trace{}, err
	}
	return h.executeCall(call)
}

func (h *Harness) executeCall(call ToolCall) (Trace, error) {
	spec, err := h.fake.validate(call)
	if err != nil {
		return Trace{}, err
	}
	attempts := 1
	if call.Failures > 0 && spec.RetryEligible {
		attempts = min(maxReadOnlyAttempts, call.Failures+1)
	}
	trace := Trace{Calls: make([]Call, 0, attempts)}
	for i := 0; i < attempts; i++ {
		h.fake.calls = append(h.fake.calls, Call{Name: call.Name, Arguments: call.Arguments})
		trace.Calls = append(trace.Calls, h.fake.calls[len(h.fake.calls)-1])
	}
	if call.Unknown {
		return trace, fmt.Errorf("MCP tool %q returned unknown; explicit recovery is required before continuation", call.Name)
	}
	if call.Failures >= maxReadOnlyAttempts && spec.RetryEligible {
		return trace, fmt.Errorf("MCP tool %q exhausted %d bounded attempts", call.Name, maxReadOnlyAttempts)
	}
	if call.Failures > 0 && !spec.RetryEligible {
		return trace, fmt.Errorf("MCP tool %q is mutating and cannot retry without explicit recovery", call.Name)
	}
	return trace, nil
}

func reportAllowsTools(report contracts.AgentReport) error {
	if report.Status == "pass" {
		return nil
	}
	return fmt.Errorf("agent report status %q blocks MCP tool calls; inspect evidence and use explicit recovery before continuation", report.Status)
}

// Scenario is a compact transcript fixture shared by all packaging surfaces.
type Scenario struct {
	Name             string
	Dispatch, Report []byte
	Calls            []ToolCall
	WantRetries      int
	WantBlocked      bool
}

// Trace is the normalized semantic trace, intentionally independent of each
// platform's Markdown/TOML/YAML packaging syntax.
type Trace struct{ Calls []Call }

func (t Trace) Attempts(name string) int {
	n := 0
	for _, call := range t.Calls {
		if call.Name == name {
			n++
		}
	}
	return n
}

// Semantic returns the canonical trace identity of every call in order.
func (t Trace) Semantic() ([]SemanticCall, error) {
	semantic := make([]SemanticCall, 0, len(t.Calls))
	for _, call := range t.Calls {
		entry, err := semanticCall(call)
		if err != nil {
			return nil, err
		}
		semantic = append(semantic, entry)
	}
	return semantic, nil
}

func RunScenario(scenario Scenario) (Trace, error) {
	fake := NewFakeMCP()
	harness := New(fake)
	dispatch, err := contracts.DecodeDispatch(scenario.Dispatch)
	if err != nil {
		return Trace{}, err
	}
	report, err := contracts.DecodeAgentReport(dispatch, scenario.Report)
	if err != nil {
		return Trace{}, err
	}
	if err := reportAllowsTools(report); err != nil {
		return Trace{}, err
	}
	var result Trace
	for _, call := range scenario.Calls {
		trace, err := harness.executeCall(call)
		result.Calls = append(result.Calls, trace.Calls...)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// Corpus is the shared transcript-driven regression set. Its scenarios make
// safety properties executable without creating a second workflow authority.
func Corpus() []Scenario {
	return []Scenario{
		{Name: "happy-path", Dispatch: fixtureDispatch("drup-validator", "baseline", "baseline"), Report: fixtureReport("drup-validator", "baseline", "pass"), Calls: []ToolCall{{Name: "scan", Arguments: json.RawMessage(`{"project_path":"/project"}`)}}},
		{Name: "dirty-tree", Dispatch: fixtureDispatch("drup-preflight", "preflight", "env"), Report: fixtureReport("drup-preflight", "preflight", "blocked"), Calls: []ToolCall{{Name: "composer_require", Arguments: json.RawMessage(`{"project_path":"/project","package":"drupal/upgrade_status","request_id":"dirty-tree-1"}`)}}, WantBlocked: true},
		{Name: "backup-failure", Dispatch: fixtureDispatch("drup-preflight", "backup", "backup"), Report: fixtureReport("drup-preflight", "backup", "blocked"), Calls: []ToolCall{{Name: "test_backup_create", Arguments: json.RawMessage(`{"project_path":"/project","request_id":"backup-failure-1"}`)}}, WantBlocked: true},
		{Name: "two-retries-then-success", Dispatch: fixtureDispatch("drup-validator", "baseline", "baseline"), Report: fixtureReport("drup-validator", "baseline", "pass"), Calls: []ToolCall{{Name: "scan", Arguments: json.RawMessage(`{"project_path":"/project"}`), Failures: 2}}, WantRetries: 3},
		{Name: "retries-exhausted", Dispatch: fixtureDispatch("drup-validator", "baseline", "baseline"), Report: fixtureReport("drup-validator", "baseline", "pass"), Calls: []ToolCall{{Name: "scan", Arguments: json.RawMessage(`{"project_path":"/project"}`), Failures: 3}}, WantRetries: 3, WantBlocked: true},
		{Name: "contrib-major-isolated", Dispatch: fixtureDispatch("drup-contrib", "contrib", "contrib"), Report: fixtureReport("drup-contrib", "contrib", "pass"), Calls: []ToolCall{{Name: "composer_require", Arguments: json.RawMessage(`{"project_path":"/project","package":"drupal/example:^3","request_id":"contrib-major-1"}`)}}},
		{Name: "core-sequential", Dispatch: fixtureDispatch("drup-contrib", "core", "core"), Report: fixtureReport("drup-contrib", "core", "pass"), Calls: []ToolCall{{Name: "core_upgrade_apply", Arguments: json.RawMessage(`{"project_path":"/project","target_major":11,"request_id":"core-11"}`)}, {Name: "core_upgrade_apply", Arguments: json.RawMessage(`{"project_path":"/project","target_major":12,"request_id":"core-12"}`)}}},
		{Name: "confirmation-rejected", Dispatch: fixtureDispatch("drup-contrib", "core", "core"), Report: fixtureReport("drup-contrib", "core", "blocked"), Calls: []ToolCall{{Name: "core_upgrade_apply", Arguments: json.RawMessage(`{"project_path":"/project","target_major":11,"request_id":"confirmation-rejected-1"}`)}}, WantBlocked: true},
		{Name: "ambiguous-restart-requires-recovery", Dispatch: fixtureDispatch("drup-contrib", "contrib", "contrib"), Report: fixtureReport("drup-contrib", "contrib", "pass"), Calls: []ToolCall{{Name: "apply_patch", Arguments: json.RawMessage(`{"project_path":"/project","patch_url":"https://example.test/fix.patch","composer_package":"drupal/example","description":"retry only after reconciliation","request_id":"ambiguous-1"}`), Unknown: true}}, WantBlocked: true},
	}
}

func fixtureDispatch(agent, phase, scope string) []byte {
	return []byte(fmt.Sprintf(`{"schema_version":"v1","identity":{"root":"/project","candidate":"candidate-1","run_id":"run-1","phase":"%s"},"agent":"%s","scope":"%s","payload":{}}`, phase, agent, scope))
}
func fixtureReport(agent, phase, status string) []byte {
	return []byte(fmt.Sprintf(`{"schema_version":"v1","identity":{"root":"/project","candidate":"candidate-1","run_id":"run-1","phase":"%s"},"agent":"%s","status":"%s","summary":"reported","artifacts":[],"evidence":{},"risks":[]}`, phase, agent, status))
}

// renderedTranscript is the executable portion of a platform package's
// machine-readable contract. Its trace is compared with the real corpus,
// rather than reconstructed from platform-neutral test code.
type renderedTranscript struct {
	SchemaVersion string `json:"schema_version"`
	Transcript    struct {
		Scenarios []renderedScenario `json:"scenarios"`
	} `json:"transcript"`
}
type renderedScenario struct {
	Name    string         `json:"name"`
	Outcome string         `json:"outcome"`
	Trace   []SemanticCall `json:"trace"`
}

// RunRenderedCorpus validates and executes the corpus selected by one
// rendered platform artifact. It returns the normalized call sequence so
// callers can compare platform traces without bypassing their contracts.
func RunRenderedCorpus(raw []byte) (Trace, error) {
	var rendered renderedTranscript
	if err := json.Unmarshal(raw, &rendered); err != nil {
		return Trace{}, fmt.Errorf("rendered contract JSON: %w", err)
	}
	if rendered.SchemaVersion != "v1" {
		return Trace{}, fmt.Errorf("rendered contract schema_version %q is unsupported", rendered.SchemaVersion)
	}
	corpus := Corpus()
	limit := len(rendered.Transcript.Scenarios)
	if len(corpus) < limit {
		limit = len(corpus)
	}
	var combined Trace
	for i := 0; i < limit; i++ {
		declared, scenario := rendered.Transcript.Scenarios[i], corpus[i]
		if declared.Name != scenario.Name {
			return Trace{}, fmt.Errorf("rendered transcript scenario %d is %q, want %q", i, declared.Name, scenario.Name)
		}
		trace, err := validateRenderedScenario(declared, scenario)
		if err != nil {
			return Trace{}, err
		}
		combined.Calls = append(combined.Calls, trace.Calls...)
	}
	if len(rendered.Transcript.Scenarios) != len(corpus) {
		return Trace{}, fmt.Errorf("rendered transcript scenario count = %d, want %d", len(rendered.Transcript.Scenarios), len(corpus))
	}
	return combined, nil
}

func validateRenderedScenario(declared renderedScenario, scenario Scenario) (Trace, error) {
	trace, err := RunScenario(scenario)
	blocked := err != nil
	if declared.Outcome == "blocked" && !blocked {
		return Trace{}, fmt.Errorf("rendered transcript %q expected blocked outcome", declared.Name)
	}
	if declared.Outcome == "pass" && blocked {
		return Trace{}, fmt.Errorf("rendered transcript %q unexpectedly blocked: %w", declared.Name, err)
	}
	if declared.Outcome != "pass" && declared.Outcome != "blocked" {
		return Trace{}, fmt.Errorf("rendered transcript %q has invalid outcome %q", declared.Name, declared.Outcome)
	}
	actual, semanticErr := trace.Semantic()
	if semanticErr != nil {
		return Trace{}, semanticErr
	}
	if !sameSemanticCalls(actual, declared.Trace) {
		return Trace{}, fmt.Errorf("rendered transcript %q semantic trace = %#v, want %#v", declared.Name, declared.Trace, actual)
	}
	return trace, nil
}

func sameSemanticCalls(left, right []SemanticCall) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].Tool != right[i].Tool || len(left[i].Arguments) != len(right[i].Arguments) {
			return false
		}
		for key, value := range left[i].Arguments {
			other, ok := right[i].Arguments[key]
			if !ok || !bytes.Equal(value, other) {
				return false
			}
		}
	}
	return true
}
