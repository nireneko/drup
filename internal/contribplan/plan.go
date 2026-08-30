// Package contribplan builds deterministic, read-only Composer update plans
// for Drupal contrib packages. It deliberately consumes typed Composer JSON;
// neither this domain package nor its parser can invoke a shell.
package contribplan

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/nireneko/drup/internal/semver"
)

type Package struct {
	Name      string            `json:"name"`
	Version   string            `json:"version"`
	Require   map[string]string `json:"require,omitempty"`
	Abandoned bool              `json:"abandoned,omitempty"`
}

type Lock struct {
	Packages []Package `json:"packages"`
}

// ParseLock accepts Composer's lock JSON and normalizes package ordering so a
// semantically identical lock always derives the same plan.
func ParseLock(raw []byte) (Lock, error) {
	var lock Lock
	if err := json.Unmarshal(raw, &lock); err != nil {
		return Lock{}, fmt.Errorf("parse composer.lock: %w", err)
	}
	if len(lock.Packages) == 0 {
		return Lock{}, fmt.Errorf("composer.lock has no packages")
	}
	seen := make(map[string]bool, len(lock.Packages))
	for i := range lock.Packages {
		p := &lock.Packages[i]
		if p.Name == "" || p.Version == "" {
			return Lock{}, fmt.Errorf("composer.lock package has empty name or version")
		}
		if seen[p.Name] {
			return Lock{}, fmt.Errorf("composer.lock has duplicate package %s", p.Name)
		}
		seen[p.Name] = true
		if p.Require == nil {
			p.Require = map[string]string{}
		}
	}
	sort.Slice(lock.Packages, func(i, j int) bool { return lock.Packages[i].Name < lock.Packages[j].Name })
	return lock, nil
}

type Class string

const (
	NoOp              Class = "no_op"
	Patch             Class = "patch"
	Minor             Class = "minor"
	Major             Class = "major"
	PatchRequired     Class = "patch_required"
	Abandoned         Class = "abandoned"
	BlockedDependency Class = "blocked_dependency"
)

type Blocker struct {
	Root       string `json:"root"`
	Constraint string `json:"constraint"`
}
type Item struct {
	Package   string    `json:"package"`
	Installed string    `json:"installed"`
	Available string    `json:"available,omitempty"`
	Class     Class     `json:"class"`
	Blockers  []Blocker `json:"blockers,omitempty"`
}
type GroupKind string

const (
	GroupPatch GroupKind = "patch"
	GroupMinor GroupKind = "minor"
	GroupMajor GroupKind = "major"
)

type Group struct {
	Kind    GroupKind `json:"kind"`
	Targets []string  `json:"targets"`
}
type Plan struct {
	Items  map[string]Item `json:"items"`
	Groups []Group         `json:"groups"`
}
type Input struct {
	Lock             Lock
	RootRequirements map[string]string
	Outdated         map[string]string
}

func Build(input Input) (Plan, error) {
	if len(input.Lock.Packages) == 0 {
		return Plan{}, fmt.Errorf("composer.lock has no packages")
	}
	byName := make(map[string]Package, len(input.Lock.Packages)+1)
	for _, pkg := range input.Lock.Packages {
		byName[pkg.Name] = pkg
	}
	if len(input.RootRequirements) > 0 {
		byName["root"] = Package{Name: "root", Require: input.RootRequirements}
	}
	plan := Plan{Items: make(map[string]Item, len(input.Lock.Packages))}
	for _, pkg := range input.Lock.Packages {
		if !strings.HasPrefix(pkg.Name, "drupal/") {
			continue
		}
		item := Item{Package: pkg.Name, Installed: pkg.Version, Class: NoOp}
		if pkg.Abandoned {
			item.Class = Abandoned
			plan.Items[pkg.Name] = item
			continue
		}
		available := strings.TrimSpace(input.Outdated[pkg.Name])
		item.Available = available
		if available == "" || available == pkg.Version {
			plan.Items[pkg.Name] = item
			continue
		}
		class, err := updateClass(pkg.Version, available)
		if err != nil {
			return Plan{}, fmt.Errorf("classify %s: %w", pkg.Name, err)
		}
		item.Class = class
		if class == Major {
			item.Blockers = blockersFor(pkg.Name, available, byName)
			if len(item.Blockers) > 0 {
				item.Class = BlockedDependency
			}
		}
		plan.Items[pkg.Name] = item
	}
	plan.Groups = groups(plan.Items)
	return plan, nil
}

func updateClass(installed, available string) (Class, error) {
	from, err := semver.Parse(installed)
	if err != nil {
		return "", err
	}
	to, err := semver.Parse(available)
	if err != nil {
		return "", err
	}
	if to.Compare(from) <= 0 {
		return NoOp, nil
	}
	if to.Major != from.Major {
		return Major, nil
	}
	if to.Minor != from.Minor {
		return Minor, nil
	}
	return Patch, nil
}
func blockersFor(target, available string, packages map[string]Package) []Blocker {
	to, err := semver.Parse(available)
	if err != nil {
		return nil
	}
	var blockers []Blocker
	for _, pkg := range packages {
		if c, ok := pkg.Require[target]; ok && !semver.Satisfies(to, c) {
			blockers = append(blockers, Blocker{Root: pkg.Name, Constraint: c})
		}
	}
	sort.Slice(blockers, func(i, j int) bool {
		if blockers[i].Root == blockers[j].Root {
			return blockers[i].Constraint < blockers[j].Constraint
		}
		return blockers[i].Root < blockers[j].Root
	})
	return blockers
}
func groups(items map[string]Item) []Group {
	var patch, minor, major []string
	for name, item := range items {
		switch item.Class {
		case Patch, PatchRequired:
			patch = append(patch, name)
		case Minor:
			minor = append(minor, name)
		case Major:
			major = append(major, name)
		}
	}
	sort.Strings(patch)
	sort.Strings(minor)
	sort.Strings(major)
	groups := make([]Group, 0, 2+len(major))
	if len(patch) > 0 {
		groups = append(groups, Group{Kind: GroupPatch, Targets: patch})
	}
	if len(minor) > 0 {
		groups = append(groups, Group{Kind: GroupMinor, Targets: minor})
	}
	for _, target := range major {
		groups = append(groups, Group{Kind: GroupMajor, Targets: []string{target}})
	}
	return groups
}
func Equal(left, right Plan) bool { return reflect.DeepEqual(left, right) }
