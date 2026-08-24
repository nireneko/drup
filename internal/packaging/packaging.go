package packaging

import (
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nireneko/drup/internal/state"
)

//go:embed templates/*
var templateFS embed.FS

// Render returns the set of files to write for a given platform.
// binaryPath is injected into MCP config templates. assignments configures
// per-platform/per-agent model overrides (REQ-004); nil/empty renders the
// built-in defaults byte-identically to pre-change output (REQ-003).
// Locked mode is never selected through this entry point — use RenderLocked
// to render the mcp.json `--locked` argument.
func Render(platform, binaryPath string, assignments map[string]map[string]state.ModelPhaseAssignment) (map[string]string, error) {
	return renderInternal(platform, binaryPath, false, assignments)
}

// RenderLocked is Render plus locked-mode selection: when locked is true,
// every rendered mcp.json launches `drup mcp --locked` instead of `drup
// mcp`, equivalent to setting DRUP_DISABLE_MUTATIONS=1 for that agent's MCP
// server process. locked=false renders byte-identically to Render.
func RenderLocked(platform, binaryPath string, locked bool, assignments map[string]map[string]state.ModelPhaseAssignment) (map[string]string, error) {
	return renderInternal(platform, binaryPath, locked, assignments)
}

// renderInternal is the shared implementation behind Render and
// RenderLocked, kept as a single function (D2-style rationale: adding a
// locked bool here churns zero existing Render call sites — see
// internal/packaging/packaging_test.go's 27 pre-existing Render(...) calls
// and internal/app/commands.go's installAgents).
func renderInternal(platform, binaryPath string, locked bool, assignments map[string]map[string]state.ModelPhaseAssignment) (map[string]string, error) {
	platformDir := platform
	switch platform {
	case "claude", "opencode", "codex":
		// valid
	default:
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}

	// Fail closed before writing anything: an unknown platform/agent key or
	// an injection-unsafe model string must not produce partial output.
	if err := validateAssignments(platform, assignments); err != nil {
		return nil, err
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
		// The templates are gone; this guard keeps them from creeping back in.
		if relPath == "CLAUDE.md" || relPath == "copilot-instructions.md" {
			return nil
		}

		// Replace binary path placeholder in MCP config.
		s := string(content)
		s = strings.ReplaceAll(s, "{{BINARY_PATH}}", binaryPath)

		// Replace the locked-mode arg placeholder in MCP config. Omitted
		// (empty string) by default so unlocked output stays byte-identical
		// to before this placeholder existed.
		lockedArg := ""
		if locked {
			lockedArg = `, "--locked"`
		}
		s = strings.ReplaceAll(s, "{{LOCKED_ARG}}", lockedArg)

		// Replace skill path placeholder in bootstrap templates.
		// Uses "." (current directory) as default — SKILL.md is co-located with the bootstrap.
		skillDir := "."
		s = strings.ReplaceAll(s, "{{SKILL_PATH}}", skillDir)

		// Substitute model placeholders BEFORE the Codex Markdown->TOML
		// conversion below, so the resolved `model = "..."` line survives
		// the conversion instead of being computed from an already
		// converted asset (REQ-004, REQ-005).
		var modelErr error
		s, modelErr = substituteModels(s, platform, assignments)
		if modelErr != nil {
			return fmt.Errorf("%s: %w", relPath, modelErr)
		}

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

// substituteModels replaces qualified model placeholders
// `{{MODEL_DEFAULT:<agent>}}` / `{{MODEL_ESCALATION:<agent>}}` for every
// known agent (design decision 2 — qualified placeholders, since a single
// SKILL.md roster names all 6 agents). Any residual `{{MODEL_` after
// substitution fails closed rather than shipping a half-rendered asset
// (REQ-004 "zero placeholders survive render").
func substituteModels(content, platform string, assignments map[string]map[string]state.ModelPhaseAssignment) (string, error) {
	for _, agent := range agentNames {
		resolved := resolveModel(agent, platform, assignments)
		content = strings.ReplaceAll(content, fmt.Sprintf("{{MODEL_DEFAULT:%s}}", agent), resolved.Default)
		content = strings.ReplaceAll(content, fmt.Sprintf("{{MODEL_ESCALATION:%s}}", agent), resolved.Escalation)
	}
	if strings.Contains(content, "{{MODEL_") {
		return "", fmt.Errorf("unresolved model placeholder remains after substitution")
	}
	return content, nil
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

	var description, model string
	for _, line := range strings.Split(parts[0], "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "description = "):
			description = strings.TrimSpace(strings.TrimPrefix(trimmed, "description = "))
		case strings.HasPrefix(trimmed, "model = "):
			model = strings.TrimSpace(strings.TrimPrefix(trimmed, "model = "))
		}
	}
	if description == "" {
		return "", fmt.Errorf("invalid Codex agent %s: missing description", path)
	}
	// The description is copied verbatim into TOML, so it must already be a
	// single-line quoted string.
	if len(description) < 2 || description[0] != '"' || description[len(description)-1] != '"' {
		return "", fmt.Errorf("invalid Codex agent %s: description must be a quoted single-line string, got %s", path, description)
	}
	// model uses the same quoting check as description (REQ-005) — it went
	// through substituteModels already, so this catches an unquoted or
	// multi-line template regression rather than a user-supplied value.
	if model != "" && (len(model) < 2 || model[0] != '"' || model[len(model)-1] != '"') {
		return "", fmt.Errorf("invalid Codex agent %s: model must be a quoted single-line string, got %s", path, model)
	}

	body := strings.TrimSpace(parts[1])
	// The agent instructions use a literal multiline string so Markdown
	// remains intact, which the delimiter itself cannot appear inside.
	if strings.Contains(body, "'''") {
		return "", fmt.Errorf("invalid Codex agent %s: instructions must not contain ''', it closes the TOML literal string", path)
	}

	out := "description = " + description + "\n"
	if model != "" {
		out += "model = " + model + "\n"
	}
	out += "developer_instructions = '''\n" + body + "\n'''\n"
	return out, nil
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

// SkillNames returns the skill directories drup installs for a platform.
// It is derived from the embedded templates so uninstall removes exactly what
// install wrote, including skills added later.
func SkillNames(platform string) ([]string, error) {
	if !slices.Contains(Platforms(), platform) {
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}

	// The platform-level SKILL.md is installed as the "drup" skill.
	names := []string{"drup"}

	entries, err := fs.ReadDir(templateFS, filepath.Join("templates", platform, "skills"))
	if err != nil {
		return names, nil // platform ships no extra skills
	}
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// Platforms returns the list of supported agent platforms.
func Platforms() []string {
	return []string{"claude", "opencode", "codex"}
}
