package installer

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nireneko/drup/internal/packaging"
)

// homeDir returns the user's home directory. Package-level var for testability.
var homeDir = os.UserHomeDir

// getCWD returns the current working directory. Package-level var for testability.
var getCWD = os.Getwd

// FileStatus represents the change detection result for a single file.
type FileStatus string

const (
	FileNew       FileStatus = "new"
	FileModified  FileStatus = "modified"
	FileUnchanged FileStatus = "unchanged"
)

// SyncFileResult holds the outcome for one synced file.
type SyncFileResult struct {
	Path   string     // Absolute path of the file
	Status FileStatus // new, modified, or unchanged
}

// AgentAdapter is the interface for agent-specific installation.
type AgentAdapter interface {
	ID() string
	Detect() bool
	SkillsDir() string
	AgentsDir() string
	CommandsDir() string
	MCPConfigPath() string
	WriteSkill(name, content string) error
	WriteAgent(name, content string) error
	WriteCommand(name, content string) error
	WriteMCPConfig(content string) error
	RenderMCPConfig(snippet string) (string, error)
	RemoveSkill(name string, dryRun bool) (string, error)
	RemoveAgent(name string, dryRun bool) (string, error)
	RemoveCommand(name string, dryRun bool) (string, error)
	RemoveMCPConfig(dryRun bool) (string, error)
}

// ClaudeAdapter handles Claude Code installation.
type ClaudeAdapter struct {
	HomeDir string
}

func (a *ClaudeAdapter) ID() string { return "claude" }

func (a *ClaudeAdapter) Detect() bool {
	dir := filepath.Join(a.HomeDir, ".claude")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *ClaudeAdapter) SkillsDir() string {
	return filepath.Join(a.HomeDir, ".claude", "skills")
}

func (a *ClaudeAdapter) MCPConfigPath() string {
	// Claude Code stores user-scoped MCP servers in ~/.claude.json. Installing
	// there makes drup available from every project instead of only the
	// directory where `drup install` happened to run.
	return filepath.Join(a.HomeDir, ".claude.json")
}

func (a *ClaudeAdapter) AgentsDir() string {
	return filepath.Join(a.HomeDir, ".claude", "agents")
}

func (a *ClaudeAdapter) CommandsDir() string {
	return "" // Claude Code does not support a commands directory
}

func (a *ClaudeAdapter) WriteSkill(name, content string) error {
	// Claude Code skills are directories: ~/.claude/skills/<name>/SKILL.md
	// The directory name becomes the slash command: /drup
	dir := filepath.Join(a.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

func (a *ClaudeAdapter) WriteAgent(name, content string) error {
	dir := a.AgentsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *ClaudeAdapter) WriteCommand(name, content string) error {
	// Claude Code does not support custom commands; commands are implicit via SKILL.md
	return nil
}

func (a *ClaudeAdapter) RenderMCPConfig(snippet string) (string, error) {
	configPath := a.MCPConfigPath()

	// Read existing .mcp.json or start fresh.
	var config map[string]any
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", configPath, err)
		}
		config = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("corrupt config %s: %w", configPath, err)
		}
	}

	// Parse the rendered snippet (command + args object).
	var snippetVal any
	if err := json.Unmarshal([]byte(snippet), &snippetVal); err != nil {
		return "", fmt.Errorf("invalid MCP snippet: %w", err)
	}

	// Ensure "mcpServers" key exists.
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	mcpServers["drup"] = snippetVal
	config["mcpServers"] = mcpServers

	merged, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal merged config: %w", err)
	}
	return string(merged), nil
}

func (a *ClaudeAdapter) WriteMCPConfig(content string) error {
	merged, err := a.RenderMCPConfig(content)
	if err != nil {
		return err
	}

	configPath := a.MCPConfigPath()
	dir := filepath.Dir(configPath)
	if dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create config dir: %w", err)
		}
	}
	tmp, err := os.CreateTemp(dir, ".mcp.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write([]byte(merged)); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpName, configPath)
}

func (a *ClaudeAdapter) RemoveSkill(name string, dryRun bool) (string, error) {
	dir := filepath.Join(a.SkillsDir(), name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", nil
	}
	if !dryRun {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (a *ClaudeAdapter) RemoveAgent(name string, dryRun bool) (string, error) {
	// Support glob patterns like "drup-*.md"
	pattern := filepath.Join(a.AgentsDir(), name)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if !dryRun {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return "", err
			}
		}
	}
	return matches[0], nil
}

func (a *ClaudeAdapter) RemoveCommand(name string, dryRun bool) (string, error) {
	// Claude Code does not support custom commands
	return "", nil
}

func (a *ClaudeAdapter) RemoveMCPConfig(dryRun bool) (string, error) {
	path := a.MCPConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}

	if dryRun {
		return path, nil
	}

	// Read existing .mcp.json.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("corrupt config %s: %w", path, err)
	}

	// Delete drup key from mcpServers.
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		return "", nil // no mcpServers key, nothing to remove
	}
	delete(mcpServers, "drup")

	// If mcpServers is now empty, remove the mcpServers key too.
	if len(mcpServers) == 0 {
		delete(config, "mcpServers")
	}

	// If config is now empty, delete the file.
	if len(config) == 0 {
		if err := os.Remove(path); err != nil {
			return "", err
		}
		return path, nil
	}

	// Marshal with indent.
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal updated config: %w", err)
	}

	// Atomic write: temp file + rename.
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".mcp.json.*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(updated); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}

// OpenCodeAdapter handles OpenCode installation.
type OpenCodeAdapter struct {
	HomeDir string
}

func (a *OpenCodeAdapter) ID() string { return "opencode" }

func (a *OpenCodeAdapter) Detect() bool {
	dir := filepath.Join(a.HomeDir, ".config", "opencode")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *OpenCodeAdapter) SkillsDir() string {
	return filepath.Join(a.HomeDir, ".config", "opencode", "skills")
}

func (a *OpenCodeAdapter) MCPConfigPath() string {
	return filepath.Join(a.HomeDir, ".config", "opencode", "opencode.json")
}

func (a *OpenCodeAdapter) AgentsDir() string {
	return filepath.Join(a.HomeDir, ".config", "opencode", "agents")
}

func (a *OpenCodeAdapter) CommandsDir() string {
	return filepath.Join(a.HomeDir, ".config", "opencode", "commands")
}

func (a *OpenCodeAdapter) WriteSkill(name, content string) error {
	// OpenCode skills are directories: ~/.config/opencode/skills/<name>/SKILL.md
	dir := filepath.Join(a.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

func (a *OpenCodeAdapter) WriteAgent(name, content string) error {
	dir := a.AgentsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *OpenCodeAdapter) WriteCommand(name, content string) error {
	dir := a.CommandsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *OpenCodeAdapter) RenderMCPConfig(snippet string) (string, error) {
	configPath := a.MCPConfigPath()

	// Read existing config or start fresh.
	var config map[string]any
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", configPath, err)
		}
		config = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return "", fmt.Errorf("corrupt config %s: %w", configPath, err)
		}
	}

	// Parse the rendered snippet.
	var snippetVal any
	if err := json.Unmarshal([]byte(snippet), &snippetVal); err != nil {
		return "", fmt.Errorf("invalid MCP snippet: %w", err)
	}

	// Ensure "mcp" key exists.
	mcp, ok := config["mcp"].(map[string]any)
	if !ok {
		mcp = make(map[string]any)
	}
	mcp["drup"] = snippetVal
	config["mcp"] = mcp

	merged, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal merged config: %w", err)
	}
	return string(merged), nil
}

func (a *OpenCodeAdapter) WriteMCPConfig(content string) error {
	merged, err := a.RenderMCPConfig(content)
	if err != nil {
		return err
	}

	configPath := a.MCPConfigPath()
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, "opencode.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write([]byte(merged)); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpName, configPath)
}

func (a *OpenCodeAdapter) RemoveSkill(name string, dryRun bool) (string, error) {
	dir := filepath.Join(a.SkillsDir(), name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", nil
	}
	if !dryRun {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (a *OpenCodeAdapter) RemoveAgent(name string, dryRun bool) (string, error) {
	// Support glob patterns like "drup-*.md"
	pattern := filepath.Join(a.AgentsDir(), name)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if !dryRun {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return "", err
			}
		}
	}
	return matches[0], nil
}

func (a *OpenCodeAdapter) RemoveCommand(name string, dryRun bool) (string, error) {
	pattern := filepath.Join(a.CommandsDir(), name)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", nil
	}
	if !dryRun {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return "", err
			}
		}
	}
	return matches[0], nil
}

func (a *OpenCodeAdapter) RemoveMCPConfig(dryRun bool) (string, error) {
	configPath := a.MCPConfigPath()
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return "", nil
	}

	if dryRun {
		return configPath, nil
	}

	// Read existing config.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}

	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return "", fmt.Errorf("corrupt config %s: %w", configPath, err)
	}

	// Delete drup key from mcp.
	mcp, ok := config["mcp"].(map[string]any)
	if !ok {
		return "", nil // no mcp key, nothing to remove
	}
	delete(mcp, "drup")

	// If mcp is now empty, remove the mcp key too.
	if len(mcp) == 0 {
		delete(config, "mcp")
	}

	// Marshal with indent.
	updated, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal updated config: %w", err)
	}

	// Atomic write: temp file + rename.
	dir := filepath.Dir(configPath)
	tmp, err := os.CreateTemp(dir, "opencode.json.*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(updated); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return "", fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return "", fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, configPath); err != nil {
		return "", err
	}
	return configPath, nil
}

// CodexAdapter handles Codex installation.
type CodexAdapter struct {
	HomeDir string
}

var codexAgentNames = []string{
	"drup-contrib",
	"drup-custom",
	"drup-preflight",
	"drup-rector",
	"drup-theme",
	"drup-validator",
}

func (a *CodexAdapter) ID() string { return "codex" }

func (a *CodexAdapter) Detect() bool {
	dir := filepath.Join(a.HomeDir, ".codex")
	_, err := os.Stat(dir)
	return err == nil
}

func (a *CodexAdapter) SkillsDir() string {
	return filepath.Join(a.HomeDir, ".codex", "skills")
}

func (a *CodexAdapter) MCPConfigPath() string {
	return filepath.Join(a.HomeDir, ".codex", "config.toml")
}

func (a *CodexAdapter) AgentsDir() string {
	return filepath.Join(a.HomeDir, ".codex", "agents")
}

func (a *CodexAdapter) CommandsDir() string {
	// Codex custom prompts are exposed as /prompts:<name>.
	return filepath.Join(a.HomeDir, ".codex", "prompts")
}

func (a *CodexAdapter) WriteSkill(name, content string) error {
	dir := filepath.Join(a.SkillsDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644)
}

func (a *CodexAdapter) WriteAgent(name, content string) error {
	dir := a.AgentsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *CodexAdapter) WriteCommand(name, content string) error {
	dir := a.CommandsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
}

func (a *CodexAdapter) RenderMCPConfig(snippet string) (string, error) {
	var parsed struct {
		Command    string   `json:"command"`
		Args       []string `json:"args"`
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(snippet), &parsed); err != nil {
		return "", fmt.Errorf("invalid Codex MCP snippet: %w", err)
	}
	drup, ok := parsed.MCPServers["drup"]
	if !ok {
		drup.Command = parsed.Command
		drup.Args = parsed.Args
	}
	if drup.Command == "" {
		return "", fmt.Errorf("invalid Codex MCP snippet: missing drup command")
	}

	configPath := a.MCPConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read %s: %w", configPath, err)
	}

	section := "[mcp_servers.drup]\ncommand = " + strconv.Quote(drup.Command) + "\nargs = ["
	for i, arg := range drup.Args {
		if i > 0 {
			section += ", "
		}
		section += strconv.Quote(arg)
	}
	section += "]\n"
	config := replaceCodexMCPSection(string(data), section)
	return replaceCodexAgentSections(config, a.AgentsDir()), nil
}

func (a *CodexAdapter) WriteMCPConfig(content string) error {
	merged, err := a.RenderMCPConfig(content)
	if err != nil {
		return err
	}
	return writeAtomic(a.MCPConfigPath(), []byte(merged))
}

func (a *CodexAdapter) RemoveSkill(name string, dryRun bool) (string, error) {
	dir := filepath.Join(a.SkillsDir(), name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return "", nil
	}
	if !dryRun {
		if err := os.RemoveAll(dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func (a *CodexAdapter) RemoveAgent(name string, dryRun bool) (string, error) {
	// Current Codex installs use TOML role configs. Also remove legacy Markdown
	// files left by older drup releases.
	names := []string{name}
	if strings.HasSuffix(name, ".md") {
		names = append(names, strings.TrimSuffix(name, ".md")+".toml")
	}
	var matches []string
	for _, candidate := range names {
		found, err := filepath.Glob(filepath.Join(a.AgentsDir(), candidate))
		if err != nil {
			return "", err
		}
		matches = append(matches, found...)
	}
	if len(matches) == 0 {
		return "", nil
	}
	if !dryRun {
		for _, match := range matches {
			if err := os.Remove(match); err != nil {
				return "", err
			}
		}
	}
	return matches[0], nil
}

func (a *CodexAdapter) RemoveCommand(name string, dryRun bool) (string, error) {
	path := filepath.Join(a.CommandsDir(), name)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	if !dryRun {
		if err := os.Remove(path); err != nil {
			return "", err
		}
	}
	return path, nil
}

func (a *CodexAdapter) RemoveMCPConfig(dryRun bool) (string, error) {
	path := a.MCPConfigPath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	}
	if dryRun {
		return path, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	updated := removeCodexMCPSection(string(data))
	updated = removeCodexAgentSections(updated)
	if updated == string(data) {
		return "", nil
	}
	if err := writeAtomic(path, []byte(updated)); err != nil {
		return "", err
	}
	return path, nil
}

// replaceCodexMCPSection updates only drup's TOML table, preserving every
// other Codex setting and MCP server.
func replaceCodexMCPSection(config, section string) string {
	config = removeCodexMCPSection(config)
	if config != "" && !strings.HasSuffix(config, "\n") {
		config += "\n"
	}
	if config != "" {
		config += "\n"
	}
	return config + section
}

func replaceCodexAgentSections(config, agentsDir string) string {
	config = removeCodexAgentSections(config)
	config = strings.TrimRight(config, "\n")
	for _, name := range codexAgentNames {
		if config != "" {
			config += "\n\n"
		}
		config += "[agents." + name + "]\n"
		config += "description = " + strconv.Quote("Drup "+strings.TrimPrefix(name, "drup-")+" migration agent") + "\n"
		config += "config_file = " + strconv.Quote(filepath.Join(agentsDir, name+".toml")) + "\n"
	}
	return config + "\n"
}

func removeCodexAgentSections(config string) string {
	lines := strings.SplitAfter(config, "\n")
	var kept []string
	removing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			removing = false
			for _, agentName := range codexAgentNames {
				if name == "agents."+agentName || strings.HasPrefix(name, "agents."+agentName+".") {
					removing = true
					break
				}
			}
		}
		if !removing {
			kept = append(kept, line)
		}
	}
	return strings.TrimRight(strings.Join(kept, ""), "\n") + "\n"
}

func removeCodexMCPSection(config string) string {
	lines := strings.SplitAfter(config, "\n")
	var kept []string
	removing := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "["), "]")
			removing = name == "mcp_servers.drup" || name == `mcp_servers."drup"` || strings.HasPrefix(name, "mcp_servers.drup.") || strings.HasPrefix(name, `mcp_servers."drup".`)
		}
		if !removing {
			kept = append(kept, line)
		}
	}
	return strings.TrimRight(strings.Join(kept, ""), "\n") + "\n"
}

func writeAtomic(path string, content []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".drup.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	return os.Rename(tmpName, path)
}

// DetectAgents returns all detected agent adapters.
func DetectAgents() []AgentAdapter {
	home, err := homeDir()
	if err != nil {
		return nil
	}

	adapters := []AgentAdapter{
		&ClaudeAdapter{HomeDir: home},
		&OpenCodeAdapter{HomeDir: home},
		&CodexAdapter{HomeDir: home},
	}

	var detected []AgentAdapter
	for _, a := range adapters {
		if a.Detect() {
			detected = append(detected, a)
		}
	}
	return detected
}

// maxBackups is the maximum number of config backups to retain.
var maxBackups = 5

// Backup naming. Directory backups are archives of drup-owned skill trees;
// file backups are copies of agent config files drup does not own.
const (
	dirBackupPrefix  = "drup-config-"
	dirBackupSuffix  = ".tar.gz"
	fileBackupPrefix = "drup-file-"
	fileBackupSuffix = ".bak"
)

// backupDir returns the backup directory path. Package-level var for testability.
var backupDir = func() string {
	home, err := homeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "drup", "backups")
}

// BackupConfig creates a tar.gz backup of the config directory before overwriting.
// It keeps the last maxBackups backups and skips if content is identical to the latest.
func BackupConfig(configDirPath string) error {
	if _, err := os.Stat(configDirPath); os.IsNotExist(err) {
		return nil // nothing to backup
	}

	bDir := backupDir()
	if bDir == "" {
		return fmt.Errorf("cannot determine backup directory")
	}
	if err := os.MkdirAll(bDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	// Check if content is identical to latest backup (dedup).
	latestBackup := findLatestBackup(bDir)
	if latestBackup != "" && isIdentical(configDirPath, latestBackup) {
		return nil // skip — identical content
	}

	// Create tar.gz backup.
	timestamp := time.Now().Format("20060102-150405.000000000")
	backupName := dirBackupPrefix + timestamp + dirBackupSuffix
	backupPath := filepath.Join(bDir, backupName)

	if err := createTarGz(backupPath, configDirPath); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("create backup: %w", err)
	}

	// Prune old backups beyond retention limit.
	pruneBackups(bDir, dirBackupPrefix, dirBackupSuffix, maxBackups)

	return nil
}

// BackupFile snapshots a single agent config file before drup rewrites it.
// These files (~/.claude.json, ~/.codex/config.toml) hold user settings drup
// does not own and cannot regenerate, unlike the skill trees BackupConfig
// archives. Retention and deduplication mirror BackupConfig, per file.
func BackupFile(path string) error {
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to backup
		}
		return fmt.Errorf("read %s: %w", path, err)
	}

	bDir := backupDir()
	if bDir == "" {
		return fmt.Errorf("cannot determine backup directory")
	}
	if err := os.MkdirAll(bDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}

	prefix := fileBackupPrefix + backupFileSlug(path) + "-"
	backups := listBackups(bDir, prefix, fileBackupSuffix)
	if len(backups) > 0 {
		latest, err := os.ReadFile(backups[len(backups)-1])
		if err == nil && bytes.Equal(latest, data) {
			return nil // skip — identical content
		}
	}

	timestamp := time.Now().Format("20060102-150405.000000000")
	backupPath := filepath.Join(bDir, prefix+timestamp+fileBackupSuffix)
	if err := writeAtomic(backupPath, data); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	pruneBackups(bDir, prefix, fileBackupSuffix, maxBackups)

	return nil
}

// backupFileSlug turns a config path into a filename-safe identifier so
// backups of different agent configs never collide.
func backupFileSlug(path string) string {
	base := strings.TrimPrefix(filepath.Base(path), ".")
	base = strings.ReplaceAll(base, string(os.PathSeparator), "-")
	if base == "" {
		return "config"
	}
	return base
}

func createTarGz(outputPath, sourceDir string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	gzWriter := gzip.NewWriter(outFile)
	defer gzWriter.Close()

	tarWriter := tar.NewWriter(gzWriter)
	defer tarWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Create relative path for tar entry.
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(tarWriter, file)
		return err
	})
}

// listBackups returns every backup in bDir matching prefix/suffix, oldest first.
// Timestamped names sort chronologically.
func listBackups(bDir, prefix, suffix string) []string {
	entries, err := os.ReadDir(bDir)
	if err != nil {
		return nil
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) && strings.HasSuffix(e.Name(), suffix) {
			backups = append(backups, filepath.Join(bDir, e.Name()))
		}
	}

	sort.Strings(backups)
	return backups
}

func findLatestBackup(bDir string) string {
	backups := listBackups(bDir, dirBackupPrefix, dirBackupSuffix)
	if len(backups) == 0 {
		return ""
	}
	return backups[len(backups)-1]
}

func isIdentical(sourceDir, backupPath string) bool {
	// Extract backup to temp dir and compare contents.
	// For simplicity, compare SHA256 of all files concatenated.
	sourceHash := hashDir(sourceDir)

	tmpDir, err := os.MkdirTemp("", "drup-backup-check-*")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmpDir)

	if err := extractTarGz(backupPath, tmpDir); err != nil {
		return false
	}

	backupHash := hashDir(tmpDir)
	return sourceHash == backupHash
}

func hashDir(dir string) string {
	h := sha256.New()
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h.Write(data)
		return nil
	})
	return fmt.Sprintf("%x", h.Sum(nil))
}

func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzReader, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(filepath.FromSlash(header.Name))
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive path escape: %s", header.Name)
		}
		target := filepath.Join(destDir, name)
		root := filepath.Clean(destDir)
		if target != root && !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("archive path escape: %s", header.Name)
		}
		if header.Typeflag != tar.TypeDir && header.Typeflag != tar.TypeReg {
			return fmt.Errorf("unsupported archive entry: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

func pruneBackups(bDir, prefix, suffix string, keep int) {
	backups := listBackups(bDir, prefix, suffix)
	if len(backups) <= keep {
		return
	}

	// Delete oldest backups beyond retention limit.
	for _, b := range backups[:len(backups)-keep] {
		os.Remove(b)
	}
}

// resolveFilePath maps a logical file path to an absolute path for the given agent.
func resolveFilePath(agent AgentAdapter, path string) string {
	switch {
	case path == "SKILL.md":
		return filepath.Join(agent.SkillsDir(), "drup", "SKILL.md")
	case path == ".mcp.json" || path == "mcp.json":
		return agent.MCPConfigPath()
	case path == "CLAUDE.md":
		projectDir, _ := getCWD()
		return filepath.Join(projectDir, "CLAUDE.md")
	case path == "copilot-instructions.md":
		projectDir, _ := getCWD()
		return filepath.Join(projectDir, ".github", "copilot-instructions.md")
	case strings.HasPrefix(path, "agents/"):
		name := strings.TrimPrefix(path, "agents/")
		return filepath.Join(agent.AgentsDir(), name)
	case strings.HasPrefix(path, "commands/"):
		name := strings.TrimPrefix(path, "commands/")
		return filepath.Join(agent.CommandsDir(), name)
	case strings.HasPrefix(path, "skills/"):
		// Strip all leading "skills/" prefixes to avoid nested skills/skills/ paths.
		rest := path
		for strings.HasPrefix(rest, "skills/") {
			rest = strings.TrimPrefix(rest, "skills/")
		}
		// Deduplicate SKILL.md appearing as a directory segment (SKILL.md/SKILL.md → SKILL.md).
		rest = strings.ReplaceAll(rest, "SKILL.md/", "")
		// If the path ends with SKILL.md, use it directly; otherwise append SKILL.md.
		if strings.HasSuffix(rest, "SKILL.md") {
			return filepath.Join(agent.SkillsDir(), rest)
		}
		return filepath.Join(agent.SkillsDir(), rest, "SKILL.md")
	default:
		return filepath.Join(agent.SkillsDir(), path, "SKILL.md")
	}
}

// computeIntendedContent returns the content to compare/write for a given file.
// For .mcp.json, it uses the adapter's RenderMCPConfig to get the post-merge content.
func computeIntendedContent(agent AgentAdapter, path, content string) (string, error) {
	if path == ".mcp.json" || path == "mcp.json" {
		return agent.RenderMCPConfig(content)
	}
	return content, nil
}

// writeFileContent writes content to the given path, creating parent directories as needed.
func writeFileContent(agent AgentAdapter, path, content string) error {
	absPath := resolveFilePath(agent, path)
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", absPath, err)
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// Install writes skill files, agent definitions, and MCP config to all detected agents.
// It compares intended content against existing files, skips unchanged files,
// and creates a backup only when at least one file is new or modified.
// Returns []SyncFileResult with one entry per file per agent.
func Install(agents []AgentAdapter, binaryPath string, files map[string]string) ([]SyncFileResult, error) {
	// Phase 1: Compute status for all files across all agents.
	type filePlan struct {
		agent   AgentAdapter
		path    string
		content string
		status  FileStatus
	}
	var allPlans []filePlan
	var allResults []SyncFileResult

	for _, agent := range agents {
		for path, content := range files {
			intended, err := computeIntendedContent(agent, path, content)
			if err != nil {
				return nil, fmt.Errorf("compute content for %s/%s: %w", agent.ID(), path, err)
			}

			absPath := resolveFilePath(agent, path)
			status := FileNew

			existing, err := os.ReadFile(absPath)
			if err == nil {
				if bytes.Equal(existing, []byte(intended)) {
					status = FileUnchanged
				} else {
					status = FileModified
				}
			}

			allPlans = append(allPlans, filePlan{agent: agent, path: path, content: intended, status: status})
			allResults = append(allResults, SyncFileResult{Path: absPath, Status: status})
		}
	}

	// Phase 2: Backup agents that have any new or modified files. The MCP
	// config is backed up separately because it lives outside the skills
	// directory and belongs to the user, not to drup.
	backedUp := make(map[string]bool)
	for _, p := range allPlans {
		if p.status != FileUnchanged && !backedUp[p.agent.ID()] {
			if err := BackupConfig(p.agent.SkillsDir()); err != nil {
				return nil, fmt.Errorf("backup config for %s: %w", p.agent.ID(), err)
			}
			if err := BackupFile(p.agent.MCPConfigPath()); err != nil {
				return nil, fmt.Errorf("backup MCP config for %s: %w", p.agent.ID(), err)
			}
			backedUp[p.agent.ID()] = true
		}
	}

	// Phase 3: Write only new or modified files.
	for _, p := range allPlans {
		if p.status == FileUnchanged {
			continue
		}
		if err := writeFileContent(p.agent, p.path, p.content); err != nil {
			return nil, fmt.Errorf("write %s to %s: %w", p.path, p.agent.ID(), err)
		}
	}

	return allResults, nil
}

// Uninstall removes skill files, agent definitions, and MCP config from all provided agents.
// It returns the list of paths removed (or would-be-removed in dry-run mode).
func Uninstall(agents []AgentAdapter, dryRun bool) ([]string, error) {
	var paths []string

	for _, agent := range agents {
		// Remove every skill directory drup installed, not just the main one.
		skills, err := packaging.SkillNames(agent.ID())
		if err != nil {
			return paths, fmt.Errorf("resolve skills for %s: %w", agent.ID(), err)
		}
		for _, skill := range skills {
			if path, err := agent.RemoveSkill(skill, dryRun); err != nil {
				return paths, fmt.Errorf("remove skill from %s: %w", agent.ID(), err)
			} else if path != "" {
				paths = append(paths, path)
			}
		}

		// Remove agent files using glob pattern.
		if path, err := agent.RemoveAgent("drup-*.md", dryRun); err != nil {
			return paths, fmt.Errorf("remove agents from %s: %w", agent.ID(), err)
		} else if path != "" {
			paths = append(paths, path)
		}

		// Remove command files.
		if path, err := agent.RemoveCommand("drup.md", dryRun); err != nil {
			return paths, fmt.Errorf("remove command from %s: %w", agent.ID(), err)
		} else if path != "" {
			paths = append(paths, path)
		}

		// Remove MCP config.
		if path, err := agent.RemoveMCPConfig(dryRun); err != nil {
			return paths, fmt.Errorf("remove MCP config from %s: %w", agent.ID(), err)
		} else if path != "" {
			paths = append(paths, path)
		}
	}

	return paths, nil
}
