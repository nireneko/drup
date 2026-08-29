package packaging

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AgentContract is the platform-neutral, machine-readable subset of the
// coordinator/agent envelope. It is rendered with every platform package so
// transcript tests do not need to infer contracts from Markdown prose.
type AgentContract struct {
	SchemaVersion string `json:"schema_version"`
	Identity      struct {
		Required []string `json:"required"`
		Phases   []string `json:"phases"`
	} `json:"identity"`
	Dispatch struct {
		Required []string `json:"required"`
		Agents   []string `json:"agents"`
		Scopes   []string `json:"scopes"`
	} `json:"dispatch"`
	AgentReport struct {
		Required []string `json:"required"`
		Status   []string `json:"status"`
	} `json:"agent_report"`
	ValidationEvidence struct {
		Required []string `json:"required"`
	} `json:"validation_evidence"`
	Transcript struct {
		Scenarios []struct {
			Name    string `json:"name"`
			Outcome string `json:"outcome"`
			Trace   []struct {
				Tool      string                     `json:"tool"`
				Arguments map[string]json.RawMessage `json:"arguments"`
			} `json:"trace"`
		} `json:"scenarios"`
	} `json:"transcript"`
	CheckpointEvidence struct {
		Required []string `json:"required"`
	} `json:"checkpoint_evidence"`
}

// RenderContract decodes the contract emitted by Render. Keeping this input as
// rendered files makes the test surface match what each agent actually gets.
func RenderContract(files map[string]string) (AgentContract, error) {
	const path = "contracts/agent-contract.json"
	raw, ok := files[path]
	if !ok {
		return AgentContract{}, fmt.Errorf("rendered package is missing %s", path)
	}
	var contract AgentContract
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return AgentContract{}, fmt.Errorf("decode %s: %w", path, err)
	}
	if contract.SchemaVersion != "v1" {
		return AgentContract{}, fmt.Errorf("decode %s: unsupported schema_version %q", path, contract.SchemaVersion)
	}
	return contract, nil
}
