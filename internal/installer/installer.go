package installer

import (
	"fmt"
	"os"
	"path/filepath"
)

// homeDir returns the user's home directory. Package-level var for testability.
var homeDir = os.UserHomeDir

// AgentAdapter is the interface for agent-specific installation.
type AgentAdapter interface {
	ID() string
	Detect() bool
	SkillsDir() string
	MCPConfigPath() string
	WriteSkill(name, content string) error
	WriteMCPConfig(binaryPath string) error
}

// ClaudeAdapter handles Claude Code installation.
type ClaudeAdapter struct {
	homeDir string
}

func (a *ClaudeAdapter) ID() string { return "claude" }

func (a *ClaudeAdapter) Detect() bool {
	dir := filepath.Join(a.homeDir, ".claude")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *ClaudeAdapter) SkillsDir() string {
	return filepath.Join(a.homeDir, ".claude", "skills")
}

func (a *ClaudeAdapter) MCPConfigPath() string {
	return filepath.Join(a.homeDir, ".claude", "mcp.json")
}

func (a *ClaudeAdapter) WriteSkill(name, content string) error {
	dir := a.SkillsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *ClaudeAdapter) WriteMCPConfig(binaryPath string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	config := fmt.Sprintf(`{
  "mcpServers": {
    "drup": {
      "command": "%s",
      "args": ["mcp"]
    }
  }
}`, binaryPath)
	return os.WriteFile(a.MCPConfigPath(), []byte(config), 0o644)
}

// OpenCodeAdapter handles OpenCode installation.
type OpenCodeAdapter struct {
	homeDir string
}

func (a *OpenCodeAdapter) ID() string { return "opencode" }

func (a *OpenCodeAdapter) Detect() bool {
	dir := filepath.Join(a.homeDir, ".config", "opencode")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *OpenCodeAdapter) SkillsDir() string {
	return filepath.Join(a.homeDir, ".config", "opencode", "skills")
}

func (a *OpenCodeAdapter) MCPConfigPath() string {
	return filepath.Join(a.homeDir, ".config", "opencode", "mcp.json")
}

func (a *OpenCodeAdapter) WriteSkill(name, content string) error {
	dir := a.SkillsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *OpenCodeAdapter) WriteMCPConfig(binaryPath string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	config := fmt.Sprintf(`{
  "mcpServers": {
    "drup": {
      "command": "%s",
      "args": ["mcp"]
    }
  }
}`, binaryPath)
	return os.WriteFile(a.MCPConfigPath(), []byte(config), 0o644)
}

// CodexAdapter handles Codex installation.
type CodexAdapter struct {
	homeDir string
}

func (a *CodexAdapter) ID() string { return "codex" }

func (a *CodexAdapter) Detect() bool {
	dir := filepath.Join(a.homeDir, ".codex")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *CodexAdapter) SkillsDir() string {
	return filepath.Join(a.homeDir, ".codex", "skills")
}

func (a *CodexAdapter) MCPConfigPath() string {
	return filepath.Join(a.homeDir, ".codex", "mcp.json")
}

func (a *CodexAdapter) WriteSkill(name, content string) error {
	dir := a.SkillsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *CodexAdapter) WriteMCPConfig(binaryPath string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	config := fmt.Sprintf(`{
  "mcpServers": {
    "drup": {
      "command": "%s",
      "args": ["mcp"]
    }
  }
}`, binaryPath)
	return os.WriteFile(a.MCPConfigPath(), []byte(config), 0o644)
}

// DetectAgents returns all detected agent adapters.
func DetectAgents() []AgentAdapter {
	home, err := homeDir()
	if err != nil {
		return nil
	}

	adapters := []AgentAdapter{
		&ClaudeAdapter{homeDir: home},
		&OpenCodeAdapter{homeDir: home},
		&CodexAdapter{homeDir: home},
	}

	var detected []AgentAdapter
	for _, a := range adapters {
		if a.Detect() {
			detected = append(detected, a)
		}
	}
	return detected
}

// Install writes skill files and MCP config to all detected agents.
func Install(agents []AgentAdapter, binaryPath string, files map[string]string) error {
	for _, agent := range agents {
		for name, content := range files {
			if err := agent.WriteSkill(name, content); err != nil {
				return fmt.Errorf("write skill %s to %s: %w", name, agent.ID(), err)
			}
		}
		if err := agent.WriteMCPConfig(binaryPath); err != nil {
			return fmt.Errorf("write MCP config to %s: %w", agent.ID(), err)
		}
	}
	return nil
}
