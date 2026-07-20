package scan

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// rawProject is the JSON structure from upgrade_status:analyze.
type rawProject struct {
	Project string     `json:"project"`
	Version *string    `json:"version"`
	Path    string     `json:"path"`
	Errors  []rawError `json:"errors"`
}

type rawError struct {
	Message string `json:"message"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Rule    string `json:"rule"`
}

// Parse reads upgrade_status:analyze JSON and returns a classified ScanResult.
func Parse(r io.Reader) (*ScanResult, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	var raw map[string]rawProject
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse upgrade_status JSON: %w", err)
	}

	result := &ScanResult{}
	for _, proj := range raw {
		mod := ModuleStatus{
			Name: proj.Project,
			Type: classifyPath(proj.Path),
		}
		for _, e := range proj.Errors {
			mod.Errors = append(mod.Errors, DepError{
				File:    e.File,
				Line:    e.Line,
				Message: e.Message,
				Rule:    e.Rule,
			})
		}
		result.TotalErrs += len(mod.Errors)
		result.Modules = append(result.Modules, mod)
	}

	return result, nil
}

// classifyPath determines the ErrorClass from a file path.
func classifyPath(path string) ErrorClass {
	switch {
	case strings.Contains(path, "modules/contrib/"):
		return ClassContrib
	case strings.Contains(path, "modules/custom/"):
		return ClassCustom
	case strings.HasPrefix(path, "themes/"):
		return ClassTheme
	case strings.HasPrefix(path, "core/"):
		return ClassCore
	default:
		return ClassCustom // fallback per spec
	}
}
