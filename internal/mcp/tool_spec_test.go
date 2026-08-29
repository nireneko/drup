package mcp

import "testing"

func TestToolSpecsAreTheSingleCatalogForSchemasAndStubs(t *testing.T) {
	specs := ToolSpecs()
	if len(specs) == 0 {
		t.Fatal("ToolSpecs returned no tools")
	}

	seen := map[string]bool{}
	stubs := defaultTools()
	for _, spec := range specs {
		if spec.Name == "" {
			t.Fatal("tool spec has no name")
		}
		if seen[spec.Name] {
			t.Fatalf("duplicate tool spec: %s", spec.Name)
		}
		seen[spec.Name] = true
		if len(spec.Properties) == 0 {
			t.Errorf("%s has no input schema properties", spec.Name)
		}
		if spec.Effect == "" {
			t.Errorf("%s has no effect class", spec.Name)
		}
		if spec.Stub && stubs[spec.Name] == nil {
			t.Errorf("stub tool %s is absent from defaultTools", spec.Name)
		}
		if !spec.Stub {
			if len(spec.Name) < len("test_backup_") || spec.Name[:len("test_backup_")] != "test_backup_" {
				t.Errorf("non-stub tool %s is not an intentional backup asymmetry", spec.Name)
			}
		}
	}
	for name := range stubs {
		if !seen[name] {
			t.Errorf("stub tool %s has no descriptor", name)
		}
	}
}

func TestMutatingDescriptorsRequireRequestIDAndOnlyReadOnlyDescriptorsRetry(t *testing.T) {
	for _, spec := range ToolSpecs() {
		hasRequestID := false
		for _, required := range spec.Required {
			hasRequestID = hasRequestID || required == "request_id"
		}
		if spec.Effect == EffectMutating && !hasRequestID {
			t.Errorf("mutating tool %s does not require request_id", spec.Name)
		}
		if spec.Effect == EffectReadOnly && hasRequestID {
			t.Errorf("read-only tool %s unexpectedly requires request_id", spec.Name)
		}
		if spec.RetryEligible && spec.Effect != EffectReadOnly {
			t.Errorf("mutating tool %s is retry eligible", spec.Name)
		}
	}
}

func TestToolSpecs_RunBoundMutationsAndWorkflowToolsHaveDistinctContracts(t *testing.T) {
	workflow := map[string]bool{
		"run_create": true, "run_status": true, "run_record": true,
		"run_confirm": true, "run_block": true, "run_abandon": true,
	}
	for _, spec := range ToolSpecs() {
		if workflow[spec.Name] {
			if spec.Effect != EffectWorkflow || spec.RequiresRun {
				t.Errorf("workflow tool %s = effect %q requires_run %v", spec.Name, spec.Effect, spec.RequiresRun)
			}
			continue
		}
		if spec.RequiresRun {
			hasRunID := false
			for _, required := range spec.Required {
				hasRunID = hasRunID || required == "run_id"
			}
			if !hasRunID {
				t.Errorf("run-bound tool %s does not require run_id", spec.Name)
			}
		}
	}
}
