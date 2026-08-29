// Package contracts defines the versioned, fail-closed envelopes exchanged by
// the coordinator and drup's specialist agents. It validates communication
// only; it does not own workflow transitions or execute domain effects.
package contracts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const SchemaVersion = "v1"

var (
	phases      = map[string]struct{}{"preflight": {}, "backup": {}, "baseline": {}, "rector": {}, "contrib": {}, "custom": {}, "theme": {}, "core": {}, "cleanup": {}, "final": {}}
	scopes      = map[string]struct{}{"env": {}, "backup": {}, "baseline": {}, "rector": {}, "contrib": {}, "custom": {}, "theme": {}, "core": {}, "global": {}}
	agents      = map[string]struct{}{"drup-preflight": {}, "drup-rector": {}, "drup-contrib": {}, "drup-custom": {}, "drup-theme": {}, "drup-validator": {}}
	statuses    = map[string]struct{}{"pass": {}, "fail": {}, "blocked": {}}
	checkpoints = map[string]struct{}{"backup": {}, "validation": {}, "confirmation": {}, "recovery": {}}
)

// Identity binds a message to one immutable candidate and one run phase.
type Identity struct {
	Root      string `json:"root"`
	Candidate string `json:"candidate"`
	RunID     string `json:"run_id"`
	Phase     string `json:"phase"`
}

// Dispatch is the typed input accepted by an agent before it makes a tool call.
type Dispatch struct {
	SchemaVersion string          `json:"schema_version"`
	Identity      Identity        `json:"identity"`
	Agent         string          `json:"agent"`
	Scope         string          `json:"scope"`
	Payload       json.RawMessage `json:"payload"`
}

// AgentReport is the typed outcome returned by an agent. Specific outcomes
// belong in Evidence, while Status only expresses transport/run state.
type AgentReport struct {
	SchemaVersion string          `json:"schema_version"`
	Identity      Identity        `json:"identity"`
	Agent         string          `json:"agent"`
	Status        string          `json:"status"`
	Summary       string          `json:"summary"`
	Artifacts     []string        `json:"artifacts"`
	Evidence      json.RawMessage `json:"evidence"`
	Risks         []string        `json:"risks"`
}

// ValidationEvidence records independently observed checks for one candidate.
type ValidationEvidence struct {
	SchemaVersion string   `json:"schema_version"`
	Identity      Identity `json:"identity"`
	Checks        []Check  `json:"checks"`
}

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

// CheckpointEvidence records a checkpoint result without authorizing a next
// transition; callers retain their existing session/audit authority.
type CheckpointEvidence struct {
	SchemaVersion string   `json:"schema_version"`
	Identity      Identity `json:"identity"`
	Checkpoint    string   `json:"checkpoint"`
	Status        string   `json:"status"`
	Detail        string   `json:"detail,omitempty"`
}

// Error points at the invalid JSON member and lists closed-enum values where
// applicable, so an agent can repair the request instead of retrying blindly.
type Error struct {
	Contract string
	Pointer  string
	Value    string
	Allowed  []string
	Reason   string
}

func (e *Error) Error() string {
	message := fmt.Sprintf("%s contract error at %s", e.Contract, e.Pointer)
	if e.Reason != "" {
		message += ": " + e.Reason
	}
	if e.Value != "" {
		message += fmt.Sprintf(" (value %q)", e.Value)
	}
	if len(e.Allowed) > 0 {
		message += fmt.Sprintf("; allowed: %s", strings.Join(e.Allowed, ", "))
	}
	return message
}

func DecodeDispatch(raw []byte) (Dispatch, error) {
	var value Dispatch
	if err := decodeStrict(raw, &value); err != nil {
		return value, decodeError("dispatch", err)
	}
	if err := validateDispatch(value); err != nil {
		return value, err
	}
	return value, nil
}

func DecodeAgentReport(dispatch Dispatch, raw []byte) (AgentReport, error) {
	var value AgentReport
	if err := decodeStrict(raw, &value); err != nil {
		return value, decodeError("agent_report", err)
	}
	if err := validateVersionIdentity("agent_report", value.SchemaVersion, value.Identity); err != nil {
		return value, err
	}
	if err := validateEnum("agent_report", "/agent", value.Agent, agents); err != nil {
		return value, err
	}
	if err := validateEnum("agent_report", "/status", value.Status, statuses); err != nil {
		return value, err
	}
	if value.Agent != dispatch.Agent {
		return value, &Error{Contract: "agent_report", Pointer: "/agent", Value: value.Agent, Reason: "must match dispatch agent"}
	}
	if err := requireSameIdentity(value.Identity, dispatch.Identity); err != nil {
		return value, err
	}
	if value.Summary == "" {
		return value, required("agent_report", "/summary")
	}
	if value.Artifacts == nil || value.Evidence == nil || value.Risks == nil {
		return value, &Error{Contract: "agent_report", Pointer: "/", Reason: "artifacts, evidence, and risks are required"}
	}
	return value, nil
}

func DecodeValidationEvidence(raw []byte) (ValidationEvidence, error) {
	var value ValidationEvidence
	if err := decodeStrict(raw, &value); err != nil {
		return value, decodeError("validation_evidence", err)
	}
	if err := validateVersionIdentity("validation_evidence", value.SchemaVersion, value.Identity); err != nil {
		return value, err
	}
	if len(value.Checks) == 0 {
		return value, required("validation_evidence", "/checks")
	}
	for i, check := range value.Checks {
		if check.Name == "" {
			return value, required("validation_evidence", fmt.Sprintf("/checks/%d/name", i))
		}
		if err := validateEnum("validation_evidence", fmt.Sprintf("/checks/%d/status", i), check.Status, statuses); err != nil {
			return value, err
		}
	}
	return value, nil
}

func DecodeCheckpointEvidence(raw []byte) (CheckpointEvidence, error) {
	var value CheckpointEvidence
	if err := decodeStrict(raw, &value); err != nil {
		return value, decodeError("checkpoint_evidence", err)
	}
	if err := validateVersionIdentity("checkpoint_evidence", value.SchemaVersion, value.Identity); err != nil {
		return value, err
	}
	if err := validateEnum("checkpoint_evidence", "/checkpoint", value.Checkpoint, checkpoints); err != nil {
		return value, err
	}
	if err := validateEnum("checkpoint_evidence", "/status", value.Status, statuses); err != nil {
		return value, err
	}
	return value, nil
}

func requireSameIdentity(actual, expected Identity) error {
	for _, field := range []struct{ name, actual, expected string }{
		{"root", actual.Root, expected.Root},
		{"candidate", actual.Candidate, expected.Candidate},
		{"run_id", actual.RunID, expected.RunID},
		{"phase", actual.Phase, expected.Phase},
	} {
		if field.actual != field.expected {
			return &Error{Contract: "agent_report", Pointer: "/identity/" + field.name, Value: field.actual, Allowed: []string{field.expected}, Reason: "must match dispatch identity"}
		}
	}
	return nil
}

func validateDispatch(value Dispatch) error {
	if err := validateVersionIdentity("dispatch", value.SchemaVersion, value.Identity); err != nil {
		return err
	}
	if err := validateEnum("dispatch", "/agent", value.Agent, agents); err != nil {
		return err
	}
	if err := validateEnum("dispatch", "/scope", value.Scope, scopes); err != nil {
		return err
	}
	if value.Payload == nil {
		return required("dispatch", "/payload")
	}
	return nil
}

func validateVersionIdentity(contract, version string, identity Identity) error {
	if version != SchemaVersion {
		return &Error{Contract: contract, Pointer: "/schema_version", Value: version, Allowed: []string{SchemaVersion}, Reason: "unsupported schema version"}
	}
	for pointer, value := range map[string]string{"/identity/root": identity.Root, "/identity/candidate": identity.Candidate, "/identity/run_id": identity.RunID} {
		if value == "" {
			return required(contract, pointer)
		}
	}
	return validateEnum(contract, "/identity/phase", identity.Phase, phases)
}

func validateEnum(contract, pointer, value string, values map[string]struct{}) error {
	if _, ok := values[value]; ok {
		return nil
	}
	allowed := make([]string, 0, len(values))
	for value := range values {
		allowed = append(allowed, value)
	}
	// Stable ordering keeps CI diagnostics deterministic.
	sort.Strings(allowed)
	return &Error{Contract: contract, Pointer: pointer, Value: value, Allowed: allowed, Reason: "unknown enum value"}
}

func required(contract, pointer string) error {
	return &Error{Contract: contract, Pointer: pointer, Reason: "required field is missing or empty"}
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("unexpected trailing JSON value")
	}
	return nil
}

func decodeError(contract string, err error) error {
	message := err.Error()
	pointer := "/"
	if strings.HasPrefix(message, "json: unknown field ") {
		field := strings.Trim(strings.TrimPrefix(message, "json: unknown field "), "\"")
		return &Error{Contract: contract, Pointer: "/" + field, Value: field, Reason: "unknown field"}
	}
	return &Error{Contract: contract, Pointer: pointer, Reason: message}
}
