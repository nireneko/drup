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
		// Global installation must not modify whichever repository happens to
		// be the current working directory. Skills are discovered from their
		// native user directories, so project bootstrap files are unnecessary.
		if relPath == "CLAUDE.md" || relPath == "copilot-instructions.md" {
			return nil
		}

		// Replace binary path placeholder in MCP config.
		s := string(content)
		s = strings.ReplaceAll(s, "{{BINARY_PATH}}", binaryPath)

		// Replace skill path placeholder in bootstrap templates.
		// Uses "." (current directory) as default — SKILL.md is co-located with the bootstrap.
		skillDir := "."
		s = strings.ReplaceAll(s, "{{SKILL_PATH}}", skillDir)
		if platform == "codex" && strings.HasPrefix(relPath, "agents/") && strings.HasSuffix(relPath, ".md") {
			var err error
			s, err = renderCodexAgentConfig(relPath, s)
			if err != nil {
				return err
			}
			relPath = strings.TrimSuffix(relPath, ".md") + ".toml"
		}
		if platform == "codex" && (relPath == "SKILL.md" || strings.HasSuffix(relPath, "/SKILL.md")) {
			if err := validateCodexSkill(relPath, s); err != nil {
				return err
			}
		}

		files[relPath] = s
		return nil
	})

	return files, err
}

// renderCodexAgentConfig converts the portable Markdown agent template into
// the TOML role config consumed by Codex's [agents.<name>].config_file.
func renderCodexAgentConfig(path, content string) (string, error) {
	if !strings.HasPrefix(content, "+++\n") {
		return "", fmt.Errorf("invalid Codex agent %s: missing TOML frontmatter", path)
	}
	parts := strings.SplitN(strings.TrimPrefix(content, "+++\n"), "\n+++\n", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid Codex agent %s: unclosed TOML frontmatter", path)
	}

	var description string
	for _, line := range strings.Split(parts[0], "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "description = ") {
			description = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "description = "))
			break
		}
	}
	if description == "" {
		return "", fmt.Errorf("invalid Codex agent %s: missing description", path)
	}

	body := strings.TrimSpace(parts[1])
	// A quoted basic string is sufficient for current descriptions. The agent
	// instructions use a literal multiline string so Markdown remains intact.
	return "description = " + description + "\n" +
		"developer_instructions = '''\n" + body + "\n'''\n", nil
}

// validateCodexSkill catches invalid assets before drup install writes them.
// Codex requires YAML frontmatter with both name and description metadata.
func validateCodexSkill(path, content string) error {
	lines := strings.Split(content, "\n")
	if len(lines) < 4 || lines[0] != "---" {
		return fmt.Errorf("invalid Codex skill %s: missing YAML frontmatter delimited by ---", path)
	}

	end := 0
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}
	if end == 0 {
		return fmt.Errorf("invalid Codex skill %s: unclosed YAML frontmatter", path)
	}
	metadata := strings.Join(lines[1:end], "\n")
	if !strings.Contains(metadata, "name:") || !strings.Contains(metadata, "description:") {
		return fmt.Errorf("invalid Codex skill %s: frontmatter requires name and description", path)
	}
	return nil
}

// Platforms returns the list of supported agent platforms.
func Platforms() []string {
	return []string{"claude", "opencode", "codex"}
}
