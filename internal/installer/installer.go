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

// AgentAdapter is the interface for agent-specific installation.
type AgentAdapter interface {
	ID() string
	Detect() bool
	SkillsDir() string
	AgentsDir() string
	MCPConfigPath() string
	WriteSkill(name, content string) error
	WriteAgent(name, content string) error
	WriteMCPConfig(content string) error
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
	return filepath.Join(a.homeDir, ".claude", "mcp", "drup.json")
}

func (a *ClaudeAdapter) AgentsDir() string {
	return filepath.Join(a.homeDir, ".claude", "agents")
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

func (a *ClaudeAdapter) WriteMCPConfig(content string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(a.MCPConfigPath(), []byte(content), 0o644)
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
	return filepath.Join(a.homeDir, ".config", "opencode", "opencode.json")
}

func (a *OpenCodeAdapter) AgentsDir() string {
	return filepath.Join(a.homeDir, ".config", "opencode", "agents")
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

func (a *CodexAdapter) AgentsDir() string {
	return filepath.Join(a.homeDir, ".codex", "agents")
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

func (a *CodexAdapter) WriteMCPConfig(content string) error {
	dir := filepath.Dir(a.MCPConfigPath())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(a.MCPConfigPath(), []byte(content), 0o644)
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
