// Package semver provides minimal semantic version parsing and constraint
// evaluation using only the standard library. It supports >=, ^, ~, and ||
// operators — sufficient for PHP and Drupal version constraint checks.
package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a semantic version with Major, Minor, and Patch components.
type Version struct {
	Major, Minor, Patch int
}

// Parse parses a version string into a Version struct.
// Accepts formats: "1.2.3", "1.2", "11", "v1.2.3", "1.2.3-beta1".
func Parse(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")

	// Strip pre-release suffix (e.g., "-beta1").
	if idx := strings.Index(s, "-"); idx >= 0 {
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Version{}, fmt.Errorf("invalid version: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Version{}, fmt.Errorf("invalid major in %q: %w", s, err)
	}
	minor := 0
	if len(parts) >= 2 {
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return Version{}, fmt.Errorf("invalid minor in %q: %w", s, err)
		}
	}
	patch := 0
	if len(parts) == 3 {
		patch, err = strconv.Atoi(parts[2])
		if err != nil {
			return Version{}, fmt.Errorf("invalid patch in %q: %w", s, err)
		}
	}

	if major < 0 || minor < 0 || patch < 0 {
		return Version{}, fmt.Errorf("negative version component in %q", s)
	}

	return Version{Major: major, Minor: minor, Patch: patch}, nil
}

// Compare returns -1, 0, or 1 comparing v to other.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return cmpInt(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return cmpInt(v.Minor, other.Minor)
	}
	return cmpInt(v.Patch, other.Patch)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// Satisfies checks if version v satisfies the given constraint string.
// Supported operators: >=, ^, ~, || (OR).
func Satisfies(v Version, constraint string) bool {
	// Split on || for OR conditions.
	parts := strings.Split(constraint, "||")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if satisfiesSingle(v, part) {
			return true
		}
	}
	return false
}

// satisfiesSingle checks a single constraint (no || operators).
func satisfiesSingle(v Version, constraint string) bool {
	constraint = strings.TrimSpace(constraint)

	switch {
	case strings.HasPrefix(constraint, ">="):
		minVer, err := Parse(strings.TrimPrefix(constraint, ">="))
		if err != nil {
			return false
		}
		return v.Compare(minVer) >= 0

	case strings.HasPrefix(constraint, "^"):
		// ^X.Y means >=X.Y.0, <(X+1).0.0
		base, err := Parse(strings.TrimPrefix(constraint, "^"))
		if err != nil {
			return false
		}
		upper := Version{Major: base.Major + 1, Minor: 0, Patch: 0}
		return v.Compare(base) >= 0 && v.Compare(upper) < 0

	case strings.HasPrefix(constraint, "~"):
		// ~X.Y means >=X.Y.0, <X.(Y+1).0
		base, err := Parse(strings.TrimPrefix(constraint, "~"))
		if err != nil {
			return false
		}
		upper := Version{Major: base.Major, Minor: base.Minor + 1, Patch: 0}
		return v.Compare(base) >= 0 && v.Compare(upper) < 0

	default:
		// Try as exact version match.
		exact, err := Parse(constraint)
		if err != nil {
			return false
		}
		return v.Compare(exact) == 0
	}
}
