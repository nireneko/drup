package scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ErrorClass classifies where a deprecation error originates.
type ErrorClass string

const (
	ClassContrib ErrorClass = "contrib"
	ClassCustom  ErrorClass = "custom"
	ClassTheme   ErrorClass = "theme"
	ClassCore    ErrorClass = "core"
)

// ScanResult is the top-level output of parsing upgrade_status JSON.
type ScanResult struct {
	Modules     []ModuleStatus `json:"modules"`
	TotalErrs   int            `json:"total_errors"`
	ProjectPath string         `json:"project_path"`
}

// ModuleStatus represents one project's scan results.
type ModuleStatus struct {
	Name   string     `json:"name"`
	Type   ErrorClass `json:"type"`
	Errors []DepError `json:"errors"`
	HasD11 *bool      `json:"has_d11_release,omitempty"`
}

// DepError is a single deprecation or compatibility error.
type DepError struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Source   string `json:"source"`
}

// normalizedFinding is the flattened, sortable shape EvidenceHash hashes
// over — one entry per (module, error) pair, independent of the nested
// Modules/Errors structure or the order Parse/ParseCheckstyle happened to
// produce them in.
type normalizedFinding struct {
	Module   string     `json:"module"`
	Type     ErrorClass `json:"type"`
	File     string     `json:"file"`
	Line     int        `json:"line"`
	Message  string     `json:"message"`
	Rule     string     `json:"rule"`
	Severity string     `json:"severity"`
	Source   string     `json:"source"`
}

// EvidenceHash computes a deterministic SHA256 hex digest over this scan
// result's normalized findings. Two scans whose findings are byte-identical
// (module, type, and every error field) hash identically regardless of the
// order modules or errors were produced in; any difference in a single
// error entry changes the hash. A scan with zero errors still hashes a
// valid, non-empty digest over its (empty) normalized finding list — the
// validate tool's expected_hash gate depends on evidence_hash never being
// empty or omitted, even for a clean scan.
func (r *ScanResult) EvidenceHash() string {
	var findings []normalizedFinding
	if r != nil {
		for _, mod := range r.Modules {
			for _, e := range mod.Errors {
				findings = append(findings, normalizedFinding{
					Module:   mod.Name,
					Type:     mod.Type,
					File:     e.File,
					Line:     e.Line,
					Message:  e.Message,
					Rule:     e.Rule,
					Severity: e.Severity,
					Source:   e.Source,
				})
			}
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Module != b.Module {
			return a.Module < b.Module
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})

	// json.Marshal never fails for this concrete, cycle-free struct slice.
	data, _ := json.Marshal(findings)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
