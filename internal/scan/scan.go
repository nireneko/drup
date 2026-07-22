package scan

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
)

var (
	projectLineRe = regexp.MustCompile(`^Project:\s+(\S+)`)
	fileLineRe    = regexp.MustCompile(`^\s*-\s+(.+):(\d+)\s*$`)
	ruleLineRe    = regexp.MustCompile(`^\s*Rule:\s+(\S+)`)
	separatorRe   = regexp.MustCompile(`^[=\-]{3,}`)
)

// Parse reads plain-text upgrade_status:analyze output and returns a classified ScanResult.
func Parse(r io.Reader) (*ScanResult, error) {
	result := &ScanResult{}

	var currentMod *ModuleStatus
	var pendingErr *DepError

	flushError := func() {
		if pendingErr != nil && currentMod != nil {
			if pendingErr.Message == "" {
				pendingErr.Message = "(no message)"
			}
			currentMod.Errors = append(currentMod.Errors, *pendingErr)
			result.TotalErrs++
		}
		pendingErr = nil
	}

	flushModule := func() {
		flushError()
		if currentMod != nil {
			result.Modules = append(result.Modules, *currentMod)
		}
		currentMod = nil
	}

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip blank, warning, and separator lines.
		if trimmed == "" || strings.HasPrefix(trimmed, "[warning]") || separatorRe.MatchString(trimmed) {
			continue
		}

		// Project header starts a new module block.
		if m := projectLineRe.FindStringSubmatch(trimmed); m != nil {
			flushModule()
			name := m[1]
			currentMod = &ModuleStatus{
				Name: name,
				Type: classifyPath(name),
			}
			continue
		}

		// File:line starts a new error entry.
		if m := fileLineRe.FindStringSubmatch(line); m != nil {
			flushError()
			lineNum := 0
			fmt.Sscanf(m[2], "%d", &lineNum)
			pendingErr = &DepError{
				File:     m[1],
				Line:     lineNum,
				Severity: "warning",
				Source:   "upgrade_status",
			}
			continue
		}

		// Rule line completes the current error.
		if m := ruleLineRe.FindStringSubmatch(trimmed); m != nil {
			if pendingErr != nil {
				pendingErr.Rule = m[1]
			}
			continue
		}

		// Otherwise: message text for the pending error.
		if pendingErr != nil && pendingErr.Message == "" {
			pendingErr.Message = trimmed
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}

	// Flush any remaining state.
	flushModule()

	// Classify modules by their file paths when the project name didn't contain a path hint.
	for i := range result.Modules {
		mod := &result.Modules[i]
		if len(mod.Errors) > 0 {
			mod.Type = classifyPath(mod.Errors[0].File)
		}
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
