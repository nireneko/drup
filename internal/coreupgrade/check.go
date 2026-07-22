// Package coreupgrade provides deterministic, read-only and git-safe
// operations to check for and apply the next Drupal core major version bump
// in composer.json. It performs no LLM parsing — every decision is derived
// from drupal.org release data and local composer.json content.
package coreupgrade

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nireneko/drup/internal/drupalorg"
)

// drupalCorePackage is the composer package name drup uses to look up the
// upstream release history for Drupal core itself.
const drupalCorePackage = "drupal/core"

// checkRelease is the drupal.org release lookup used by NextMajor.
// Package-level var for testability — tests override it to avoid real HTTP calls.
var checkRelease = drupalorg.CheckRelease

// CheckResult is the read-only outcome of a next-major-version check.
type CheckResult struct {
	CurrentVersion string `json:"current_version"`
	NextVersion    string `json:"next_version"`
	Available      bool   `json:"available"`
	Constraint     string `json:"constraint"`
}

// NextMajor checks drupal.org's release history for drupal/core and reports
// whether a major version newer than currentVersion is available. It performs
// no writes and no git operations.
func NextMajor(currentVersion string) (*CheckResult, error) {
	currentMajor, err := MajorVersion(currentVersion)
	if err != nil {
		return nil, fmt.Errorf("parse current version %q: %w", currentVersion, err)
	}

	info, err := checkRelease(drupalCorePackage)
	if err != nil {
		return nil, fmt.Errorf("check drupal/core release: %w", err)
	}

	latestMajor, err := MajorVersion(info.Latest)
	if err != nil {
		return nil, fmt.Errorf("parse latest release %q: %w", info.Latest, err)
	}

	result := &CheckResult{CurrentVersion: currentVersion}
	if latestMajor > currentMajor {
		result.NextVersion = info.Latest
		result.Available = true
		result.Constraint = fmt.Sprintf("^%d.0", latestMajor)
	}
	return result, nil
}

// MajorVersion extracts the leading major version number from a semver-like
// or composer-constraint-like string (e.g. "10.1.5" -> 10, "^11.0" -> 11).
func MajorVersion(version string) (int, error) {
	version = strings.TrimPrefix(version, "^")
	version = strings.TrimPrefix(version, "~")
	if version == "" {
		return 0, fmt.Errorf("empty version")
	}
	parts := strings.SplitN(version, ".", 2)
	return strconv.Atoi(parts[0])
}

// PreviewComposerPatch computes the exact composer.json changes an Apply call
// would make, without writing to disk. It reports every require entry whose
// package name is "drupal/core" or starts with "drupal/core-" that would
// change to newConstraint. changed is false when there is nothing to update
// (already at target, or no drupal/core requirement present).
func PreviewComposerPatch(composerJSON []byte, newConstraint string) (diff string, changed bool, err error) {
	var doc struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal(composerJSON, &doc); err != nil {
		return "", false, fmt.Errorf("parse composer.json: %w", err)
	}

	pkgs := make([]string, 0, len(doc.Require))
	for pkg := range doc.Require {
		if pkg == drupalCorePackage || strings.HasPrefix(pkg, drupalCorePackage+"-") {
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)

	var lines []string
	for _, pkg := range pkgs {
		constraint := doc.Require[pkg]
		if constraint == newConstraint {
			continue
		}
		lines = append(lines, fmt.Sprintf("-\"%s\": \"%s\"", pkg, constraint))
		lines = append(lines, fmt.Sprintf("+\"%s\": \"%s\"", pkg, newConstraint))
		changed = true
	}

	return strings.Join(lines, "\n"), changed, nil
}
