package state

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// State holds the persisted installation state.
type State struct {
	Version         string                       `json:"version"`
	InstalledAgents []string                     `json:"installed_agents"`
	PendingSync     bool                         `json:"pending_sync,omitempty"`
	ModelOverrides  map[string]map[string]string `json:"model_overrides,omitempty"`
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

	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
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
