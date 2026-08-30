// Package inventory captures a deterministic, read-only description of a Drupal project.
package inventory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SchemaVersion = 1

type Version struct {
	Version string `json:"version"`
	Source  string `json:"source"`
}
type Package struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
}
type File struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Source string `json:"source"`
}
type Patch struct {
	Package     string `json:"package"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Source      string `json:"source"`
}
type Inventory struct {
	SchemaVersion int       `json:"schema_version"`
	Core          Version   `json:"core"`
	PHP           Version   `json:"php"`
	Packages      []Package `json:"packages"`
	Extensions    []Package `json:"extensions"`
	Patches       []Patch   `json:"patches"`
	Config        []File    `json:"config"`
	Tests         []File    `json:"tests"`
}
type Change struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Before string `json:"before"`
	After  string `json:"after"`
	Source string `json:"source"`
}

// Capture only reads project files. It deliberately has no subprocess or write path.
func Capture(root string) (Inventory, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return Inventory{}, err
	}
	result := Inventory{SchemaVersion: SchemaVersion, Packages: []Package{}, Extensions: []Package{}, Patches: []Patch{}, Config: []File{}, Tests: []File{}}
	var composer struct {
		Config struct {
			Platform struct {
				PHP string `json:"php"`
			} `json:"platform"`
		} `json:"config"`
		Extra struct {
			Patches map[string]map[string]string `json:"patches"`
		} `json:"extra"`
	}
	raw, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return Inventory{}, fmt.Errorf("read composer.json: %w", err)
	}
	if err := json.Unmarshal(raw, &composer); err != nil {
		return Inventory{}, fmt.Errorf("decode composer.json: %w", err)
	}
	result.PHP = Version{Version: composer.Config.Platform.PHP, Source: "composer.json"}
	for pkg, entries := range composer.Extra.Patches {
		for description, url := range entries {
			result.Patches = append(result.Patches, Patch{Package: pkg, Description: description, URL: url, Source: "composer.json"})
		}
	}
	var lock struct {
		Packages []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages"`
		PackagesDev []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"packages-dev"`
	}
	lockRaw, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	if err != nil {
		return Inventory{}, fmt.Errorf("read composer.lock: %w", err)
	}
	if err := json.Unmarshal(lockRaw, &lock); err != nil {
		return Inventory{}, fmt.Errorf("decode composer.lock: %w", err)
	}
	for _, p := range append(lock.Packages, lock.PackagesDev...) {
		item := Package{Name: p.Name, Version: p.Version, Source: "composer.lock"}
		result.Packages = append(result.Packages, item)
		if p.Name == "drupal/core-recommended" || p.Name == "drupal/core" {
			result.Core = Version{Version: p.Version, Source: "composer.lock"}
		}
		if strings.HasPrefix(p.Name, "drupal/") && p.Name != "drupal/core" && p.Name != "drupal/core-recommended" {
			result.Extensions = append(result.Extensions, item)
		}
	}
	if err := addFiles(root, "config/sync", func(p string) bool { return strings.HasSuffix(p, ".yml") }, &result.Config); err != nil {
		return Inventory{}, err
	}
	if err := walkTests(root, &result.Tests); err != nil {
		return Inventory{}, err
	}
	sort.Slice(result.Packages, func(i, j int) bool { return result.Packages[i].Name < result.Packages[j].Name })
	sort.Slice(result.Extensions, func(i, j int) bool { return result.Extensions[i].Name < result.Extensions[j].Name })
	sort.Slice(result.Patches, func(i, j int) bool {
		a, b := result.Patches[i], result.Patches[j]
		if a.Package == b.Package {
			return a.Description < b.Description
		}
		return a.Package < b.Package
	})
	return result, nil
}
func addFiles(root, relative string, include func(string) bool, into *[]File) error {
	base := filepath.Join(root, relative)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	return filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !include(path) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		*into = append(*into, File{Path: filepath.ToSlash(rel), Digest: hex.EncodeToString(sum[:]), Source: "filesystem"})
		return nil
	})
}
func walkTests(root string, into *[]File) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != root && (info.Name() == "vendor" || info.Name() == ".git") {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slash := filepath.ToSlash(rel)
		if !strings.Contains(slash, "/tests/") && !strings.HasPrefix(slash, "tests/") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		*into = append(*into, File{Path: slash, Digest: hex.EncodeToString(sum[:]), Source: "filesystem"})
		return nil
	})
}
func Delta(before, after Inventory) []Change {
	var out []Change
	add := func(kind, name, b, a, source string) {
		if b != a {
			out = append(out, Change{Kind: kind, Name: name, Before: b, After: a, Source: source})
		}
	}
	add("core", "drupal/core", before.Core.Version, after.Core.Version, prefer(after.Core.Source, before.Core.Source))
	add("php", "php", before.PHP.Version, after.PHP.Version, prefer(after.PHP.Source, before.PHP.Source))
	diffPackages := func(kind string, b, a []Package) {
		bm := map[string]Package{}
		am := map[string]Package{}
		for _, p := range b {
			bm[p.Name] = p
		}
		for _, p := range a {
			am[p.Name] = p
		}
		names := map[string]bool{}
		for n := range bm {
			names[n] = true
		}
		for n := range am {
			names[n] = true
		}
		for n := range names {
			add(kind, n, bm[n].Version, am[n].Version, prefer(am[n].Source, bm[n].Source))
		}
	}
	diffPackages("package", before.Packages, after.Packages)
	diffPackages("extension", before.Extensions, after.Extensions)
	diffPatches := func(b, a []Patch) {
		bm := map[string][]Patch{}
		am := map[string][]Patch{}
		for _, patch := range b {
			bm[patch.Package] = append(bm[patch.Package], patch)
		}
		for _, patch := range a {
			am[patch.Package] = append(am[patch.Package], patch)
		}
		names := map[string]bool{}
		for name := range bm {
			names[name] = true
		}
		for name := range am {
			names[name] = true
		}
		for name := range names {
			add("patch", name, patchValue(bm[name]), patchValue(am[name]), prefer(patchSource(am[name]), patchSource(bm[name])))
		}
	}
	diffFiles := func(kind string, b, a []File) {
		bm := map[string]File{}
		am := map[string]File{}
		for _, file := range b {
			bm[file.Path] = file
		}
		for _, file := range a {
			am[file.Path] = file
		}
		names := map[string]bool{}
		for name := range bm {
			names[name] = true
		}
		for name := range am {
			names[name] = true
		}
		for name := range names {
			add(kind, name, bm[name].Digest, am[name].Digest, prefer(am[name].Source, bm[name].Source))
		}
	}
	diffPatches(before.Patches, after.Patches)
	diffFiles("config", before.Config, after.Config)
	diffFiles("test", before.Tests, after.Tests)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Name < out[j].Name
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func patchValue(patches []Patch) string {
	values := make([]string, len(patches))
	for i, patch := range patches {
		values[i] = patch.Description + "\x00" + patch.URL
	}
	sort.Strings(values)
	return strings.Join(values, "\x01")
}

func patchSource(patches []Patch) string {
	if len(patches) == 0 {
		return ""
	}
	sources := make([]string, len(patches))
	for i, patch := range patches {
		sources[i] = patch.Source
	}
	sort.Strings(sources)
	return sources[0]
}
func prefer(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
