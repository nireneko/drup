package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ModelPhaseAssignment holds the default and escalation model identifiers
// for one agent on one platform. Empty fields fall back to the built-in
// per-platform/per-agent defaults (internal/packaging/models.go) — see
// REQ-003. Any non-empty string is accepted (no allowlist, see design
// decision 3); only structural corruption of the generated frontmatter is
// rejected, by ValidateModelValue (called per-field from
// internal/packaging.validateAssignments).
type ModelPhaseAssignment struct {
	Default    string `json:"default,omitempty"`
	Escalation string `json:"escalation,omitempty"`
}

// State holds the persisted installation state.
type State struct {
	Version         string   `json:"version"`
	InstalledAgents []string `json:"installed_agents"`
	PendingSync     bool     `json:"pending_sync,omitempty"`

	// ModelAssignments maps platform -> agent -> {default, escalation} model
	// identifiers. Replaces the dead `model_overrides` field (REQ-008).
	// Nil/empty resolves to built-in defaults; partial config only
	// overrides the platform/agent pairs it names.
	ModelAssignments map[string]map[string]ModelPhaseAssignment `json:"model_assignments,omitempty"`
}

// configDir returns the user's config directory. Package-level var for testability.
var configDir = os.UserConfigDir

// statePath returns the path to state.json for the given config directory.
func statePath(configBase string) string {
	return filepath.Join(configBase, "drup", "state.json")
}

// Load reads the state from ~/.config/drup/state.json.
// Returns a default (empty) state if the file doesn't exist.
func Load() (*State, error) {
	base, err := configDir()
	if err != nil {
		return nil, err
	}

	path := statePath(base)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &State{}, nil
		}
		return nil, err
	}

	// Decode into State plus the retired `model_overrides` key so a legacy
	// state.json can be read without erroring, warned about once, and
	// dropped — no migration path (REQ-008, design decision 8).
	var raw struct {
		State
		LegacyModelOverrides map[string]map[string]string `json:"model_overrides,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if len(raw.LegacyModelOverrides) > 0 {
		fmt.Fprintln(os.Stderr, "Warning: state.json contains the legacy \"model_overrides\" key; ignoring it. Configure model assignments with \"model_assignments\" instead (see docs/model-configuration.md).")
	}
	s := raw.State
	return &s, nil
}

// ValidateModelValue accepts any non-empty string that cannot corrupt
// generated frontmatter/TOML: rejects newlines, quotes, backslashes, comment
// markers, or leading/trailing whitespace (design decision 3 — structure is
// validated, not the model name itself). Empty means "unset" (falls through
// to the built-in default) and always passes. Callers validate platform/agent
// keys separately: that requires the known-agent table owned by
// internal/packaging and is enforced in packaging.validateAssignments
// instead, so a bad key can never brick an unrelated command such as
// `drup uninstall` by failing at load time (design decision 4).
func ValidateModelValue(v string) error {
	if v == "" {
		return nil
	}
	if strings.TrimSpace(v) != v {
		return fmt.Errorf("must not have leading/trailing whitespace: %q", v)
	}
	for _, bad := range []string{"\n", "\"", "\\", "#"} {
		if strings.Contains(v, bad) {
			return fmt.Errorf("must not contain %q: %q", bad, v)
		}
	}
	return nil
}

// Save persists the state to ~/.config/drup/state.json with atomic write.
func Save(s *State) error {
	base, err := configDir()
	if err != nil {
		return err
	}

	dir := filepath.Join(base, "drup")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	path := statePath(base)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// Remove removes the state directory (~/.config/drup/) and legacy directory (~/.drup/).
// It is idempotent — missing directories are silently skipped.
func Remove() error {
	base, err := configDir()
	if err != nil {
		return err
	}

	// Remove ~/.config/drup/
	drupDir := filepath.Join(base, "drup")
	if _, err := os.Stat(drupDir); err == nil {
		if err := os.RemoveAll(drupDir); err != nil {
			return fmt.Errorf("remove %s: %w", drupDir, err)
		}
	}

	// Remove legacy ~/.drup/
	legacyDir := filepath.Join(base, ".drup")
	if _, err := os.Stat(legacyDir); err == nil {
		if err := os.RemoveAll(legacyDir); err != nil {
			return fmt.Errorf("remove %s: %w", legacyDir, err)
		}
	}

	return nil
}
