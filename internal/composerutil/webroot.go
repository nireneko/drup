// Package composerutil provides helpers for reading Drupal composer.json configuration.
package composerutil

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ReadWebRoot reads the web root directory name from composer.json's
// extra.drupal-scaffold.locations.web-root. Falls back to "web" if the
// scaffold config is absent, malformed, or the file doesn't exist.
func ReadWebRoot(projectPath string) string {
	data, err := os.ReadFile(filepath.Join(projectPath, "composer.json"))
	if err != nil {
		return "web"
	}

	var doc struct {
		Extra struct {
			DrupalScaffold struct {
				Locations struct {
					WebRoot string `json:"web-root"`
				} `json:"locations"`
			} `json:"drupal-scaffold"`
		} `json:"extra"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return "web"
	}

	if doc.Extra.DrupalScaffold.Locations.WebRoot == "" {
		return "web"
	}
	return doc.Extra.DrupalScaffold.Locations.WebRoot
}
