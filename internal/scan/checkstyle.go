package scan

import (
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// checkstyleReport mirrors the XML produced by upgrade_status:checkstyle.
type checkstyleReport struct {
	Files []checkstyleFile `xml:"file"`
}

type checkstyleFile struct {
	Name   string            `xml:"name,attr"`
	Errors []checkstyleError `xml:"error"`
}

type checkstyleError struct {
	Line     int    `xml:"line,attr"`
	Message  string `xml:"message,attr"`
	Severity string `xml:"severity,attr"`
	Source   string `xml:"source,attr"`
}

// ParseCheckstyle reads upgrade_status:checkstyle XML and returns a classified
// ScanResult. The XML schema is stable across upgrade_status releases, unlike
// the human-readable table that upgrade_status:analyze prints.
func ParseCheckstyle(r io.Reader) (*ScanResult, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read checkstyle output: %w", err)
	}

	// Drush prefixes its own notices to the report, so the XML rarely starts
	// at byte zero.
	text := string(raw)
	start := strings.Index(text, "<?xml")
	if start < 0 {
		start = strings.Index(text, "<checkstyle")
	}
	if start < 0 {
		// Missing report is only acceptable when drush said there was nothing
		// to report. Treating any other output as zero findings would hide
		// every deprecation in the project, which is how the previous
		// text-format parser failed silently.
		if isNoFindingsOutput(text) {
			return &ScanResult{}, nil
		}
		return nil, fmt.Errorf("no checkstyle report in upgrade_status output: %.200s", text)
	}

	var report checkstyleReport
	if err := xml.Unmarshal([]byte(text[start:]), &report); err != nil {
		return nil, fmt.Errorf("parse checkstyle XML: %w", err)
	}

	byModule := make(map[string]*ModuleStatus)
	result := &ScanResult{}

	for _, file := range report.Files {
		name := moduleNameFromPath(file.Name)
		mod, ok := byModule[name]
		if !ok {
			mod = &ModuleStatus{Name: name, Type: classifyPath(file.Name)}
			byModule[name] = mod
		}
		for _, e := range file.Errors {
			severity := e.Severity
			if severity == "" {
				severity = "warning"
			}
			source := e.Source
			if source == "" {
				source = "upgrade_status"
			}
			mod.Errors = append(mod.Errors, DepError{
				File:     file.Name,
				Line:     e.Line,
				Message:  e.Message,
				Severity: severity,
				Source:   source,
			})
			// upgrade_status emits informational rows alongside real
			// findings; counting them inflated every total by dozens.
			if severity != "info" {
				result.TotalErrs++
			}
		}
	}

	names := make([]string, 0, len(byModule))
	for name := range byModule {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result.Modules = append(result.Modules, *byModule[name])
	}

	return result, nil
}

// isNoFindingsOutput reports whether output without a checkstyle report still
// means "nothing found": empty output, drush notices, or the explicit
// no-errors message. Anything else is an unrecognized format.
func isNoFindingsOutput(text string) bool {
	if strings.TrimSpace(text) == "" {
		return true
	}
	if strings.Contains(strings.ToLower(text), "no errors found") {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "[") {
			continue
		}
		return false
	}
	return true
}

// moduleNameFromPath extracts the project name from a checkstyle file path such
// as "modules/custom/repository_sync/src/Foo.php". Paths outside a module or
// theme directory fall back to their first segment.
func moduleNameFromPath(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "./"), "/")
	for i, seg := range segments {
		if seg != "modules" && seg != "themes" && seg != "profiles" {
			continue
		}
		// Skip the optional contrib/custom grouping directory.
		next := i + 1
		if next < len(segments) && (segments[next] == "contrib" || segments[next] == "custom") {
			next++
		}
		if next < len(segments) {
			return segments[next]
		}
	}
	if len(segments) > 0 {
		return segments[0]
	}
	return path
}
