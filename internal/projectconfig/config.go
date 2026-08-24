// Package projectconfig loads per-project drup configuration from
// <project>/.drup/config.json. Every field has a safe built-in default so a
// project with no config file (the overwhelmingly common case) behaves
// identically to one that explicitly wrote the defaults out.
package projectconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Config holds the guardrail knobs a project can override via
// .drup/config.json. Load always returns a fully populated Config, whether
// or not the file exists.
type Config struct {
	// MutationCapPerSession bounds how many mutating tool calls a single
	// session_open-bound session may make before the guard refuses further
	// mutations (see internal/audit).
	MutationCapPerSession int
	// MutationCapPerDay is the cap applied when no session is open, for a
	// project that opts out of session-scoped counting.
	MutationCapPerDay int
	// BackupFreshnessWindow is how long a backup manifest remains "fresh"
	// for the backup-freshness guard: a manifest older than this (and older
	// than the session-open time) no longer satisfies the gate.
	BackupFreshnessWindow time.Duration
	// AllowlistMode selects how strict the patch URL allowlist is: "strict"
	// (default) allows only *.drupal.org over https. Any other value is
	// reserved for a future opt-in extension and currently behaves
	// identically to "strict".
	AllowlistMode string
}

// rawConfig mirrors the on-disk JSON shape. BackupFreshnessWindow is a Go
// duration string (e.g. "24h") on disk rather than the raw nanosecond count
// encoding/json would otherwise (de)serialize time.Duration as.
type rawConfig struct {
	MutationCapPerSession int    `json:"mutation_cap_per_session"`
	MutationCapPerDay     int    `json:"mutation_cap_per_day"`
	BackupFreshnessWindow string `json:"backup_freshness_window"`
	AllowlistMode         string `json:"allowlist_mode"`
}

// Defaults returns the built-in configuration applied whenever
// .drup/config.json is absent, empty, unparsable, or omits a field.
func Defaults() Config {
	return Config{
		MutationCapPerSession: 50,
		MutationCapPerDay:     200,
		BackupFreshnessWindow: 24 * time.Hour,
		AllowlistMode:         "strict",
	}
}

// Load reads <projectPath>/.drup/config.json and overlays it onto
// Defaults(). A missing file, an empty file, or a file that fails to parse
// all resolve to the plain defaults — a malformed config must never block a
// tool call, only fail to customize it. Fields present in the file but
// invalid (zero/negative caps, an unparsable duration) are likewise ignored
// in favor of the default for that field alone.
func Load(projectPath string) Config {
	cfg := Defaults()

	data, err := os.ReadFile(filepath.Join(projectPath, ".drup", "config.json"))
	if err != nil || len(data) == 0 {
		return cfg
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg
	}

	if raw.MutationCapPerSession > 0 {
		cfg.MutationCapPerSession = raw.MutationCapPerSession
	}
	if raw.MutationCapPerDay > 0 {
		cfg.MutationCapPerDay = raw.MutationCapPerDay
	}
	if raw.BackupFreshnessWindow != "" {
		if d, err := time.ParseDuration(raw.BackupFreshnessWindow); err == nil && d > 0 {
			cfg.BackupFreshnessWindow = d
		}
	}
	if raw.AllowlistMode != "" {
		cfg.AllowlistMode = raw.AllowlistMode
	}

	return cfg
}
