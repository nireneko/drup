package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// lenientPlugin relaxes the core requirement composer reads from a package's
// own metadata. It is the ecosystem's answer to a module whose released
// metadata still caps at the previous major while a patch already makes the
// code compatible: composer resolves against the metadata, never the patched
// files, so without this the patch can never be installed.
const lenientPlugin = "mglaman/composer-drupal-lenient"

// lenientPluginInstalled reports whether the plugin is actually on disk.
// Declared-but-absent is the state composer leaves when allow-plugins blocks
// the install, and an allow list without the plugin behind it changes nothing.
func lenientPluginInstalled(projectPath string) bool {
	if info, err := os.Stat(filepath.Join(projectPath, "vendor", filepath.FromSlash(lenientPlugin))); err == nil && info.IsDir() {
		return true
	}

	data, err := os.ReadFile(filepath.Join(projectPath, "vendor", "composer", "installed.json"))
	if err != nil {
		return false
	}
	var installed struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(data, &installed); err != nil {
		return false
	}
	for _, pkg := range installed.Packages {
		if pkg.Name == lenientPlugin {
			return true
		}
	}
	return false
}

// LenientResult is the JSON output of the lenient pass.
type LenientResult struct {
	ProjectPath   string   `json:"project_path"`
	PluginPresent bool     `json:"plugin_present"`
	PluginAdded   bool     `json:"plugin_added"`
	AllowedList   []string `json:"allowed_list"`
	Added         []string `json:"added"`
	DryRun        bool     `json:"dry_run"`
}

// AllowLenient adds packages to composer's drupal-lenient allow list,
// installing the plugin when the project does not have it yet. Only the named
// packages are affected: the relaxation is per package, never project-wide.
func AllowLenient(projectPath string, packages []string, dryRun bool) (*LenientResult, error) {
	if len(packages) == 0 {
		return nil, fmt.Errorf("name at least one package, e.g. drupal/switch_page_theme")
	}
	for _, pkg := range packages {
		if !strings.Contains(pkg, "/") {
			return nil, fmt.Errorf("invalid package %q: use the composer name, e.g. drupal/%s", pkg, pkg)
		}
	}

	composerPath := filepath.Join(projectPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil, fmt.Errorf("read composer.json: %w", err)
	}

	result := &LenientResult{
		ProjectPath: projectPath,
		DryRun:      dryRun,
		// Presence means installed, not declared. composer.json can name the
		// plugin while vendor/ has nothing, which is what a require that got
		// blocked by allow-plugins leaves behind — and an allow list without
		// the plugin on disk does nothing at all.
		PluginPresent: lenientPluginInstalled(projectPath),
		Added:         []string{},
	}

	var composer map[string]any
	if err := json.Unmarshal(data, &composer); err != nil {
		return nil, fmt.Errorf("parse composer.json: %w", err)
	}

	extra, _ := composer["extra"].(map[string]any)
	if extra == nil {
		extra = map[string]any{}
	}
	lenient, _ := extra["drupal-lenient"].(map[string]any)
	if lenient == nil {
		lenient = map[string]any{}
	}

	existing := map[string]bool{}
	if list, ok := lenient["allowed-list"].([]any); ok {
		for _, item := range list {
			if name, ok := item.(string); ok {
				existing[name] = true
			}
		}
	}
	for _, pkg := range packages {
		if !existing[pkg] {
			existing[pkg] = true
			result.Added = append(result.Added, pkg)
		}
	}

	allowed := make([]string, 0, len(existing))
	for name := range existing {
		allowed = append(allowed, name)
	}
	sort.Strings(allowed)
	result.AllowedList = allowed

	if dryRun {
		return result, nil
	}

	// The plugin has to be installed before composer will honour the list, and
	// composer must be told to trust it.
	if !result.PluginPresent {
		// Authorize first. composer refuses to install an unlisted plugin, and
		// it writes composer.json and the lock before that refusal, leaving a
		// package declared but absent.
		if _, stderr, exitCode, err := cliRun(projectPath, "composer", "config", "--no-plugins",
			"allow-plugins."+lenientPlugin, "true"); err != nil {
			return nil, fmt.Errorf("allow %s: %w", lenientPlugin, err)
		} else if exitCode != 0 {
			return nil, fmt.Errorf("allow %s failed (exit %d): %s", lenientPlugin, exitCode, stderr)
		}

		if _, stderr, exitCode, err := cliRun(projectPath, "composer", "require", "--dev", lenientPlugin, "-W"); err != nil {
			return nil, fmt.Errorf("install %s: %w", lenientPlugin, err)
		} else if exitCode != 0 {
			return nil, fmt.Errorf("install %s failed (exit %d): %s", lenientPlugin, exitCode, stderr)
		}
		if !lenientPluginInstalled(projectPath) {
			return nil, fmt.Errorf("%s is declared in composer.json but not present in vendor/; the allow list would have no effect", lenientPlugin)
		}
		result.PluginAdded = true

		// composer require rewrote the file; work from the new content.
		if data, err = os.ReadFile(composerPath); err != nil {
			return nil, fmt.Errorf("re-read composer.json: %w", err)
		}
		composer = map[string]any{}
		if err := json.Unmarshal(data, &composer); err != nil {
			return nil, fmt.Errorf("re-parse composer.json: %w", err)
		}
		if e, ok := composer["extra"].(map[string]any); ok {
			extra = e
		}
	}

	lenient["allowed-list"] = allowed
	extra["drupal-lenient"] = lenient
	composer["extra"] = extra

	updated, err := json.MarshalIndent(composer, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("marshal composer.json: %w", err)
	}
	if err := os.WriteFile(composerPath, append(updated, '\n'), 0o644); err != nil {
		return nil, fmt.Errorf("write composer.json: %w", err)
	}

	return result, nil
}

// RunAllowLenient exposes the lenient pass on the command line.
func RunAllowLenient(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: drup allow-lenient <path> <package>... [--dry-run]")
	}

	projectPath := args[0]
	dryRun := false
	packages := []string{}
	for _, arg := range args[1:] {
		switch {
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "-"):
			return fmt.Errorf("unknown option %q", arg)
		default:
			packages = append(packages, arg)
		}
	}

	result, err := AllowLenient(projectPath, packages, dryRun)
	if err != nil {
		return err
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
