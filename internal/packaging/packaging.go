package packaging

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed templates/*
var templateFS embed.FS

// Render returns the set of files to write for a given platform.
// binaryPath is injected into MCP config templates.
func Render(platform, binaryPath string) (map[string]string, error) {
	platformDir := platform
	switch platform {
	case "claude", "opencode", "codex":
		// valid
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}

	files := make(map[string]string)

	// Walk the platform's template directory.
	root := filepath.Join("templates", platformDir)
	err := fs.WalkDir(templateFS, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		content, err := templateFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		// Relative path from platform dir.
		relPath, _ := filepath.Rel(root, path)

		// Replace binary path placeholder in MCP config.
		s := string(content)
		s = strings.ReplaceAll(s, "{{BINARY_PATH}}", binaryPath)

		// Replace skill path placeholder in bootstrap templates.
		// Uses "." (current directory) as default — SKILL.md is co-located with the bootstrap.
		skillDir := "."
		s = strings.ReplaceAll(s, "{{SKILL_PATH}}", skillDir)

		files[relPath] = s
		return nil
	})

	return files, err
}

// Platforms returns the list of supported agent platforms.
func Platforms() []string {
	return []string{"claude", "opencode", "codex"}
}
