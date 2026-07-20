package scan

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
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
	Rule    string `json:"rule"`
}
