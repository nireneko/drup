package installer

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// homeDir returns the user's home directory. Package-level var for testability.
var homeDir = os.UserHomeDir

// getCWD returns the current working directory. Package-level var for testability.
var getCWD = os.Getwd

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
	cwd, _ := getCWD()
	return filepath.Join(cwd, ".mcp.json")
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

func (a *ClaudeAdapter) WriteMCPConfig(content string) error {
	configPath := a.MCPConfigPath()

	// Read existing .mcp.json or start fresh.
	var config map[string]any
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", configPath, err)
		}
		config = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("corrupt config %s: %w", configPath, err)
		}
	}

	// Parse the rendered snippet (command + args object).
	var snippet any
	if err := json.Unmarshal([]byte(content), &snippet); err != nil {
		return fmt.Errorf("invalid MCP snippet: %w", err)
	}

	// Ensure "mcpServers" key exists.
	mcpServers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = make(map[string]any)
	}
	mcpServers["drup"] = snippet
	config["mcpServers"] = mcpServers

	// Marshal with indent.
	merged, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged config: %w", err)
	}

	// Atomic write: temp file + rename.
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
	if _, err := tmp.Write(merged); err != nil {
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

func (a *OpenCodeAdapter) WriteMCPConfig(content string) error {
	configPath := a.MCPConfigPath()
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Read existing config or start fresh.
	var config map[string]any
	data, err := os.ReadFile(configPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", configPath, err)
		}
		config = make(map[string]any)
	} else {
		if err := json.Unmarshal(data, &config); err != nil {
			return fmt.Errorf("corrupt config %s: %w", configPath, err)
		}
	}

	// Parse the rendered snippet.
	var snippet any
	if err := json.Unmarshal([]byte(content), &snippet); err != nil {
		return fmt.Errorf("invalid MCP snippet: %w", err)
	}

	// Ensure "mcp" key exists.
	mcp, ok := config["mcp"].(map[string]any)
	if !ok {
		mcp = make(map[string]any)
	}
	mcp["drup"] = snippet
	config["mcp"] = mcp

	// Marshal with indent.
	merged, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal merged config: %w", err)
	}

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(dir, "opencode.json.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(merged); err != nil {
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
	return filepath.Join(a.HomeDir, ".codex", "mcp.json")
}

func (a *CodexAdapter) AgentsDir() string {
	return filepath.Join(a.HomeDir, ".codex", "agents")
}

func (a *CodexAdapter) CommandsDir() string {
	return "" // Codex does not support a commands directory
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
	// Codex does not support custom commands
	return nil
}

func (a *CodexAdapter) WriteMCPConfig(content string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(a.MCPConfigPath(), []byte(content), 0o644)
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

func (a *CodexAdapter) RemoveCommand(name string, dryRun bool) (string, error) {
	// Codex does not support custom commands
	return "", nil
}

func (a *CodexAdapter) RemoveMCPConfig(dryRun bool) (string, error) {
	path := a.MCPConfigPath()
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
	backupName := fmt.Sprintf("drup-config-%s.tar.gz", timestamp)
	backupPath := filepath.Join(bDir, backupName)

	if err := createTarGz(backupPath, configDirPath); err != nil {
		os.Remove(backupPath)
		return fmt.Errorf("create backup: %w", err)
	}

	// Prune old backups beyond retention limit.
	pruneBackups(bDir, maxBackups)

	return nil
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

func findLatestBackup(bDir string) string {
	entries, err := os.ReadDir(bDir)
	if err != nil {
		return ""
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "drup-config-") && strings.HasSuffix(e.Name(), ".tar.gz") {
			backups = append(backups, filepath.Join(bDir, e.Name()))
		}
	}

	if len(backups) == 0 {
		return ""
	}

	sort.Strings(backups)
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

		target := filepath.Join(destDir, header.Name)
		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			outFile, err := os.Create(target)
			if err != nil {
				return err
			}
			io.Copy(outFile, tarReader)
			outFile.Close()
		}
	}
	return nil
}

func pruneBackups(bDir string, keep int) {
	entries, err := os.ReadDir(bDir)
	if err != nil {
		return
	}

	var backups []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "drup-config-") && strings.HasSuffix(e.Name(), ".tar.gz") {
			backups = append(backups, filepath.Join(bDir, e.Name()))
		}
	}

	if len(backups) <= keep {
		return
	}

	sort.Strings(backups)
	// Delete oldest backups beyond retention limit.
	for _, b := range backups[:len(backups)-keep] {
		os.Remove(b)
	}
}

// Install writes skill files, agent definitions, and MCP config to all detected agents.
// It creates a backup of existing configs before overwriting.
// The files map uses paths from packaging.Render (e.g. "SKILL.md", "agents/drup-preflight.md", ".mcp.json").
func Install(agents []AgentAdapter, binaryPath string, files map[string]string) error {
	// Backup existing configs before overwriting.
	for _, agent := range agents {
		skillsDir := agent.SkillsDir()
		if err := BackupConfig(skillsDir); err != nil {
			return fmt.Errorf("backup config for %s: %w", agent.ID(), err)
		}
	}

	for _, agent := range agents {
		for path, content := range files {
			switch {
			case path == "SKILL.md":
				// Main orchestrator skill → skills/drup/SKILL.md (command: /drup)
				if err := agent.WriteSkill("drup", content); err != nil {
					return fmt.Errorf("write orchestrator skill to %s: %w", agent.ID(), err)
				}
			case strings.HasPrefix(path, "agents/"):
				// Sub-agent definitions → agents/<name>.md
				agentName := strings.TrimPrefix(path, "agents/")
				if err := agent.WriteAgent(agentName, content); err != nil {
					return fmt.Errorf("write agent %s to %s: %w", agentName, agent.ID(), err)
				}
			case strings.HasPrefix(path, "commands/"):
				// Command definitions → commands/<name>.md
				commandName := strings.TrimPrefix(path, "commands/")
				if err := agent.WriteCommand(commandName, content); err != nil {
					return fmt.Errorf("write command %s to %s: %w", commandName, agent.ID(), err)
				}
			case path == ".mcp.json":
				// MCP config — use pre-rendered template content
				if err := agent.WriteMCPConfig(content); err != nil {
					return fmt.Errorf("write MCP config to %s: %w", agent.ID(), err)
				}
			default:
				// Legacy: write unknown files as flat skills (backward compat)
				if err := agent.WriteSkill(path, content); err != nil {
					return fmt.Errorf("write %s to %s: %w", path, agent.ID(), err)
				}
			}
		}
	}
	return nil
}

// Uninstall removes skill files, agent definitions, and MCP config from all provided agents.
// It returns the list of paths removed (or would-be-removed in dry-run mode).
func Uninstall(agents []AgentAdapter, dryRun bool) ([]string, error) {
	var paths []string

	for _, agent := range agents {
		// Remove skill directory.
		if path, err := agent.RemoveSkill("drup", dryRun); err != nil {
			return paths, fmt.Errorf("remove skill from %s: %w", agent.ID(), err)
		} else if path != "" {
			paths = append(paths, path)
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
