package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nireneko/drup/internal/semver"
)

// CompatChange records what happened to one custom extension's
// core_version_requirement.
type CompatChange struct {
	Name    string `json:"name"`
	File    string `json:"file"`
	Before  string `json:"before"`
	After   string `json:"after,omitempty"`
	Changed bool   `json:"changed"`
	Note    string `json:"note,omitempty"`
}

// CompatResult is the JSON output of the custom compatibility pass.
type CompatResult struct {
	ProjectPath       string         `json:"project_path"`
	TargetVersion     string         `json:"target_version"`
	DryRun            bool           `json:"dry_run"`
	Updated           int            `json:"updated"`
	AlreadyCompatible int            `json:"already_compatible"`
	NeedsAttention    int            `json:"needs_attention"`
	Changes           []CompatChange `json:"changes"`
}

// customExtensionDirs are the directories holding a project's own code.
// Contrib is deliberately excluded: it is composer-managed and belongs in a
// patch, never an in-place edit.
var customExtensionDirs = [][]string{
	{"modules", "custom"},
	{"themes", "custom"},
	{"profiles", "custom"},
}

// BumpCustomCoreCompat widens core_version_requirement in the project's own
// modules, themes and profiles so they declare support for the target Drupal
// major. These declarations are what preflight reports as upgrade blockers,
// and nothing else in the pipeline rewrites them.
func BumpCustomCoreCompat(projectPath, targetVersion string, dryRun bool) (*CompatResult, error) {
	major := majorOf(targetVersion)
	if major == "" {
		return nil, fmt.Errorf("invalid target version %q: use a major like 11", targetVersion)
	}
	target, err := semver.Parse(major + ".0.0")
	if err != nil {
		return nil, fmt.Errorf("invalid target version %q: %w", targetVersion, err)
	}

	result := &CompatResult{
		ProjectPath:   projectPath,
		TargetVersion: major,
		DryRun:        dryRun,
		Changes:       []CompatChange{},
	}

	root := resolveDrupalRoot(projectPath)
	for _, parts := range customExtensionDirs {
		dir := filepath.Join(append([]string{root}, parts...)...)
		for _, infoFile := range findInfoYMLFiles(dir) {
			change, err := bumpInfoFile(infoFile, major, target, dryRun)
			if err != nil {
				return nil, err
			}
			switch {
			case change.Changed:
				result.Updated++
			case change.Note != "":
				result.NeedsAttention++
			default:
				result.AlreadyCompatible++
			}
			result.Changes = append(result.Changes, change)
		}
	}

	return result, nil
}

func bumpInfoFile(infoFile, major string, target semver.Version, dryRun bool) (CompatChange, error) {
	change := CompatChange{
		Name: filepath.Base(filepath.Dir(infoFile)),
		File: infoFile,
	}

	data, err := os.ReadFile(infoFile)
	if err != nil {
		return change, fmt.Errorf("read %s: %w", infoFile, err)
	}
	content := string(data)

	constraint := parseCoreVersionRequirementFromInfo(content)
	change.Before = constraint

	if constraint == "" {
		// A missing declaration means the extension still uses the removed
		// "core:" key. Where to insert the replacement is a judgement call
		// about the file's structure, so leave it to a human.
		change.Note = "no core_version_requirement declared — add one manually"
		return change, nil
	}
	if semver.Satisfies(target, constraint) {
		return change, nil
	}

	change.After = widenConstraint(constraint, major)
	updated, ok := rewriteCoreVersionRequirement(content, change.After)
	if !ok {
		change.Note = "core_version_requirement found but could not be rewritten"
		change.After = ""
		return change, nil
	}
	change.Changed = true

	if dryRun {
		return change, nil
	}
	if err := os.WriteFile(infoFile, []byte(updated), 0o644); err != nil {
		return change, fmt.Errorf("write %s: %w", infoFile, err)
	}
	return change, nil
}

// widenConstraint adds the target major to an existing constraint rather than
// replacing it, so an extension keeps the versions it already supports.
func widenConstraint(constraint, major string) string {
	return strings.TrimSpace(constraint) + " || ^" + major
}

// rewriteCoreVersionRequirement replaces the declared value while preserving
// the line's indentation and quoting style.
func rewriteCoreVersionRequirement(content, newValue string) (string, bool) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "core_version_requirement:") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		value := strings.TrimSpace(strings.TrimPrefix(trimmed, "core_version_requirement:"))
		quote := ""
		if len(value) > 1 && (value[0] == '"' || value[0] == '\'') {
			quote = string(value[0])
		}
		if quote == "" {
			// Bare values containing "||" need quoting to stay valid YAML in
			// every parser, and Drupal's own core does quote them.
			quote = "'"
		}
		lines[i] = indent + "core_version_requirement: " + quote + newValue + quote
		return strings.Join(lines, "\n"), true
	}
	return content, false
}

// majorOf accepts "11", "11.1" or "^11" and returns the major digits.
func majorOf(version string) string {
	v := strings.TrimSpace(version)
	v = strings.TrimLeft(v, "^~>=< ")
	if idx := strings.IndexAny(v, ".-"); idx > 0 {
		v = v[:idx]
	}
	if v == "" {
		return ""
	}
	for _, r := range v {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return v
}

// RunCompatFix widens core_version_requirement across the project's own
// extensions and prints the result as JSON.
func RunCompatFix(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: drup compat-fix <path> [--target=11] [--dry-run]")
	}

	projectPath := args[0]
	target := "11"
	dryRun := false
	for _, arg := range args[1:] {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "--target="):
			target = strings.TrimPrefix(arg, "--target=")
		default:
			return fmt.Errorf("unknown option %q", arg)
		}
	}

	result, err := BumpCustomCoreCompat(projectPath, target, dryRun)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(data))

	if result.NeedsAttention > 0 {
		return fmt.Errorf("%d extension(s) need a core_version_requirement added by hand", result.NeedsAttention)
	}
	return nil
}
