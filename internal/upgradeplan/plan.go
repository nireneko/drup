// Package upgradeplan owns the numeric Drupal-major upgrade route.
//
// A plan is deliberately built from explicit per-step metadata. This keeps a
// caller from treating the 10-to-11 compatibility catalog as a generic rule
// for a later major such as 12.
package upgradeplan

import (
	"fmt"
	"strconv"
	"strings"
)

// Major is a positive Drupal major version.
type Major int

// NewMajor validates and returns a numeric major version.
func NewMajor(value int) (Major, error) {
	if value < 1 {
		return 0, fmt.Errorf("Drupal major must be positive, got %d", value)
	}
	return Major(value), nil
}

// ParseMajor parses a semver-like Drupal version or Composer constraint. It
// accepts a leading v, ^, or ~, but rejects ranges and non-numeric components.
func ParseMajor(value string) (Major, error) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "v")
	value = strings.TrimPrefix(value, "^")
	value = strings.TrimPrefix(value, "~")
	if value == "" {
		return 0, fmt.Errorf("Drupal major is empty")
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if part == "" {
			return 0, fmt.Errorf("invalid Drupal version %q", value)
		}
		if _, err := strconv.Atoi(part); err != nil {
			return 0, fmt.Errorf("invalid Drupal version %q: %w", value, err)
		}
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parse Drupal major %q: %w", value, err)
	}
	return NewMajor(major)
}

// Metadata identifies the versioned compatibility catalog governing one
// immediate upgrade. CatalogID must not be reused for a different jump.
type Metadata struct {
	From      Major
	To        Major
	CatalogID string
}

// KnownCatalog returns the offline compatibility metadata currently shipped
// with drup. Callers receive a copy so an invocation cannot extend a catalog
// for another target major by mutating shared state.
func KnownCatalog() []Metadata {
	return []Metadata{{From: 10, To: 11, CatalogID: "10-to-11"}}
}

// Step is one validated, immediate major upgrade.
type Step struct {
	From      Major
	To        Major
	CatalogID string
}

// Constraint returns the Composer constraint to use for this step's target.
func (s Step) Constraint() string { return fmt.Sprintf("^%d", s.To) }

// Validate confirms this step retains the domain's immediate-jump and
// metadata invariants when it crosses a package boundary.
func (s Step) Validate() error {
	if _, err := NewMajor(int(s.From)); err != nil {
		return fmt.Errorf("step from: %w", err)
	}
	if s.To != s.From+1 {
		return fmt.Errorf("step %d-to-%d is not the immediate next major jump", s.From, s.To)
	}
	if strings.TrimSpace(s.CatalogID) == "" {
		return fmt.Errorf("step %d-to-%d has no catalog ID", s.From, s.To)
	}
	return nil
}

// Plan is an ordered upgrade route. An empty plan is an explicit no-op.
type Plan struct {
	Current Major
	Target  Major
	Steps   []Step
}

// NoOp reports whether current already equals target.
func (p Plan) NoOp() bool { return len(p.Steps) == 0 }

// Build creates the complete consecutive route from current to target. Every
// step must have exact catalog metadata; missing or mismatched metadata is an
// error rather than a reason to reuse another major's rules.
func Build(current, target Major, metadata []Metadata) (Plan, error) {
	if _, err := NewMajor(int(current)); err != nil {
		return Plan{}, fmt.Errorf("current major: %w", err)
	}
	if _, err := NewMajor(int(target)); err != nil {
		return Plan{}, fmt.Errorf("target major: %w", err)
	}
	if target < current {
		return Plan{}, fmt.Errorf("target Drupal major %d is lower than current major %d", target, current)
	}
	plan := Plan{Current: current, Target: target}
	if target == current {
		return plan, nil
	}

	byJump := make(map[[2]Major]Metadata, len(metadata))
	for _, entry := range metadata {
		if err := (Step{From: entry.From, To: entry.To, CatalogID: entry.CatalogID}).Validate(); err != nil {
			return Plan{}, fmt.Errorf("metadata: %w", err)
		}
		key := [2]Major{entry.From, entry.To}
		if _, exists := byJump[key]; exists {
			return Plan{}, fmt.Errorf("duplicate metadata for %d-to-%d", entry.From, entry.To)
		}
		byJump[key] = entry
	}

	for from := current; from < target; from++ {
		to := from + 1
		entry, ok := byJump[[2]Major{from, to}]
		if !ok {
			return Plan{}, fmt.Errorf("missing compatibility metadata for Drupal %d-to-%d", from, to)
		}
		plan.Steps = append(plan.Steps, Step{From: from, To: to, CatalogID: entry.CatalogID})
	}
	return plan, nil
}
