package installer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectAgents_Claude(t *testing.T) {
	home := t.TempDir()
	// Create Claude config dir.
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	agents := DetectAgents()
	found := false
	for _, a := range agents {
		if a.ID() == "claude" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Claude to be detected")
	}
}

func TestDetectAgents_OpenCode(t *testing.T) {
	home := t.TempDir()
	// Create OpenCode config dir.
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	agents := DetectAgents()
	found := false
	for _, a := range agents {
		if a.ID() == "opencode" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected OpenCode to be detected")
	}
}

func TestDetectAgents_None(t *testing.T) {
	home := t.TempDir()

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	agents := DetectAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestClaudeAdapter_Paths(t *testing.T) {
	home := t.TempDir()
	adapter := &ClaudeAdapter{HomeDir: home}

	if adapter.ID() != "claude" {
		t.Errorf("ID = %q, want %q", adapter.ID(), "claude")
	}
	if !adapter.Detect() {
		// Create the dir and try again.
		os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
		if !adapter.Detect() {
			t.Error("Detect = false after creating .claude dir")
		}
	}

	skillsDir := adapter.SkillsDir()
	if skillsDir == "" {
		t.Error("SkillsDir is empty")
	}

	// Mock CWD to home dir so .mcp.json resolves predictably.
	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	want := filepath.Join(home, ".mcp.json")
	if got := adapter.MCPConfigPath(); got != want {
		t.Errorf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestOpenCodeAdapter_Paths(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	want := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got := adapter.MCPConfigPath(); got != want {
		t.Errorf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestOpenCodeAdapter_WriteMCPConfig_MergesExisting(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// Pre-populate opencode.json with existing MCP servers and other keys.
	configDir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(configDir, 0o755)
	existing := `{
  "agent": {"default": "test"},
  "mcp": {
    "context7": {"type": "remote", "url": "https://example.com"},
    "engram": {"type": "local", "command": ["engram", "mcp"]}
  },
  "permission": {"bash": {"*": "allow"}}
}`
	configPath := filepath.Join(configDir, "opencode.json")
	os.WriteFile(configPath, []byte(existing), 0o644)

	// Write MCP config with drup snippet.
	snippet := `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`
	if err := adapter.WriteMCPConfig(snippet); err != nil {
		t.Fatalf("WriteMCPConfig error: %v", err)
	}

	// Read back and verify.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Existing top-level keys preserved.
	if _, ok := result["agent"]; !ok {
		t.Error("existing 'agent' key not preserved")
	}
	if _, ok := result["permission"]; !ok {
		t.Error("existing 'permission' key not preserved")
	}

	// Existing MCP entries preserved.
	mcp, ok := result["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp key missing or not an object")
	}
	if _, ok := mcp["context7"]; !ok {
		t.Error("existing 'context7' MCP entry not preserved")
	}
	if _, ok := mcp["engram"]; !ok {
		t.Error("existing 'engram' MCP entry not preserved")
	}

	// Drup entry added.
	drup, ok := mcp["drup"].(map[string]any)
	if !ok {
		t.Fatal("drup MCP entry missing or not an object")
	}
	if drup["type"] != "local" {
		t.Errorf("drup type = %v, want 'local'", drup["type"])
	}
}

func TestOpenCodeAdapter_WriteMCPConfig_CreatesNew(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	snippet := `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`
	if err := adapter.WriteMCPConfig(snippet); err != nil {
		t.Fatalf("WriteMCPConfig error: %v", err)
	}

	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	mcp, ok := result["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp key missing or not an object")
	}
	drup, ok := mcp["drup"].(map[string]any)
	if !ok {
		t.Fatal("drup entry missing or not an object")
	}
	if drup["type"] != "local" {
		t.Errorf("drup type = %v, want 'local'", drup["type"])
	}
}

func TestOpenCodeAdapter_WriteMCPConfig_CorruptFile(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	configDir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(configDir, 0o755)
	configPath := filepath.Join(configDir, "opencode.json")
	corruptContent := `{this is not valid json!!!`
	os.WriteFile(configPath, []byte(corruptContent), 0o644)

	snippet := `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`
	err := adapter.WriteMCPConfig(snippet)
	if err == nil {
		t.Fatal("expected error for corrupt JSON, got nil")
	}

	// Verify file was NOT overwritten.
	data, _ := os.ReadFile(configPath)
	if string(data) != corruptContent {
		t.Error("corrupt file was overwritten — it should have been left untouched")
	}
}

func TestOpenCodeAdapter_WriteMCPConfig_OverwritesExistingDrup(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	configDir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(configDir, 0o755)
	configPath := filepath.Join(configDir, "opencode.json")
	existing := `{
  "mcp": {
    "drup": {"type": "local", "command": ["/old/path/drup", "mcp"]},
    "engram": {"type": "local", "command": ["engram", "mcp"]}
  }
}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	snippet := `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`
	if err := adapter.WriteMCPConfig(snippet); err != nil {
		t.Fatalf("WriteMCPConfig error: %v", err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}

	mcp := result["mcp"].(map[string]any)
	drup := mcp["drup"].(map[string]any)
	cmd := drup["command"].([]any)
	if cmd[0] != "/usr/local/bin/drup" {
		t.Errorf("drup command[0] = %v, want '/usr/local/bin/drup'", cmd[0])
	}
	// Other MCP entries preserved.
	if _, ok := mcp["engram"]; !ok {
		t.Error("existing 'engram' entry not preserved during drup overwrite")
	}
}

func TestBackupConfig_CreatesTarGz(t *testing.T) {
	// Create a source config dir with some files.
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "mcp.json"), []byte(`{"test": true}`), 0o644)
	os.MkdirAll(filepath.Join(srcDir, "skills"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "skills", "SKILL.md"), []byte("# skill"), 0o644)

	// Set backup dir to a temp location.
	bDir := t.TempDir()
	orig := backupDir
	backupDir = func() string { return bDir }
	defer func() { backupDir = orig }()

	if err := BackupConfig(srcDir); err != nil {
		t.Fatalf("BackupConfig error: %v", err)
	}

	// Verify backup file exists.
	entries, err := os.ReadDir(bDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(entries))
	}
	if !strings.HasSuffix(entries[0].Name(), ".tar.gz") {
		t.Errorf("backup file = %q, want .tar.gz suffix", entries[0].Name())
	}
}

func TestBackupConfig_Retention5(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "config.json"), []byte(`{"v": 1}`), 0o644)

	bDir := t.TempDir()
	orig := backupDir
	backupDir = func() string { return bDir }
	defer func() { backupDir = orig }()

	// Create 6 backups with different content each time.
	for i := 0; i < 6; i++ {
		os.WriteFile(filepath.Join(srcDir, "config.json"), []byte(fmt.Sprintf(`{"v": %d}`, i)), 0o644)
		if err := BackupConfig(srcDir); err != nil {
			t.Fatalf("BackupConfig #%d error: %v", i, err)
		}
	}

	// Should keep only 5.
	entries, err := os.ReadDir(bDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 backups (retention), got %d", len(entries))
	}
}

func TestBackupConfig_DeduplicatesIdentical(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "config.json"), []byte(`{"same": true}`), 0o644)

	bDir := t.TempDir()
	orig := backupDir
	backupDir = func() string { return bDir }
	defer func() { backupDir = orig }()

	// First backup.
	if err := BackupConfig(srcDir); err != nil {
		t.Fatal(err)
	}
	// Second backup with same content — should be skipped.
	if err := BackupConfig(srcDir); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(bDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 backup (dedup), got %d", len(entries))
	}
}

func TestBackupConfig_NoSourceDir(t *testing.T) {
	bDir := t.TempDir()
	orig := backupDir
	backupDir = func() string { return bDir }
	defer func() { backupDir = orig }()

	// Non-existent source dir — should succeed silently.
	if err := BackupConfig("/nonexistent/path"); err != nil {
		t.Fatalf("BackupConfig should not error for missing dir: %v", err)
	}
}

func TestInstall_WritesFiles(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	// Mock CWD so Claude's .mcp.json resolves predictably.
	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":                 "# Test Orchestrator\n",
		".mcp.json":                `{"command":"drup","args":["mcp"]}`,
		"agents/drup-preflight.md": "# Test Preflight Agent\n",
	}

	results, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// All files should be "new" on first install.
	if len(results) != len(files) {
		t.Fatalf("expected %d results, got %d", len(files), len(results))
	}
	for _, r := range results {
		if r.Status != FileNew {
			t.Errorf("file %s: expected status %q, got %q", r.Path, FileNew, r.Status)
		}
		if r.Path == "" {
			t.Error("result has empty path")
		}
	}

	// Orchestrator skill: SKILL.md → skills/drup/SKILL.md (directory + file)
	skillPath := filepath.Join(agents[0].SkillsDir(), "drup", "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Errorf("orchestrator skill not written to %s", skillPath)
	}

	// Sub-agent: agents/drup-preflight.md → agents/drup-preflight.md
	agentPath := filepath.Join(agents[0].AgentsDir(), "drup-preflight.md")
	if _, err := os.Stat(agentPath); os.IsNotExist(err) {
		t.Errorf("agent file not written to %s", agentPath)
	}

	// MCP config: .mcp.json
	mcpPath := agents[0].MCPConfigPath()
	if _, err := os.Stat(mcpPath); os.IsNotExist(err) {
		t.Errorf("MCP config not written to %s", mcpPath)
	}
}

// Phase 1: Adapter Remove* methods tests

func TestClaudeAdapter_RemoveSkill(t *testing.T) {
	home := t.TempDir()
	adapter := &ClaudeAdapter{HomeDir: home}

	// Create skill directory.
	skillDir := filepath.Join(home, ".claude", "skills", "drup")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill"), 0o644)

	// Remove it.
	path, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("RemoveSkill error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify deleted.
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory still exists after RemoveSkill")
	}

	// Idempotent: remove again should succeed.
	path2, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("second RemoveSkill error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveSkill should return empty path, got %q", path2)
	}
}

func TestClaudeAdapter_RemoveAgent(t *testing.T) {
	home := t.TempDir()
	adapter := &ClaudeAdapter{HomeDir: home}

	// Create agent files.
	agentsDir := filepath.Join(home, ".claude", "agents")
	os.MkdirAll(agentsDir, 0o755)
	os.WriteFile(filepath.Join(agentsDir, "drup-preflight.md"), []byte("# preflight"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "drup-contrib.md"), []byte("# contrib"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "other-agent.md"), []byte("# other"), 0o644)

	// Remove all drup agents using glob pattern.
	path, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("RemoveAgent error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify drup agents deleted.
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-preflight.md")); !os.IsNotExist(err) {
		t.Error("drup-preflight.md still exists")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-contrib.md")); !os.IsNotExist(err) {
		t.Error("drup-contrib.md still exists")
	}

	// Other agent preserved.
	if _, err := os.Stat(filepath.Join(agentsDir, "other-agent.md")); os.IsNotExist(err) {
		t.Error("other-agent.md was deleted — should be preserved")
	}

	// Idempotent.
	path2, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("second RemoveAgent error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveAgent should return empty path, got %q", path2)
	}
}

func TestClaudeAdapter_RemoveMCPConfig(t *testing.T) {
	home := t.TempDir()
	adapter := &ClaudeAdapter{HomeDir: home}

	// Mock CWD to home dir.
	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	// Create MCP config.
	mcpPath := filepath.Join(home, ".mcp.json")
	os.WriteFile(mcpPath, []byte(`{"mcpServers":{"drup":{"command":"drup"}}}`), 0o644)

	// Remove it.
	path, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("RemoveMCPConfig error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify deleted.
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("MCP config still exists after RemoveMCPConfig")
	}

	// Idempotent.
	path2, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("second RemoveMCPConfig error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveMCPConfig should return empty path, got %q", path2)
	}
}

func TestOpenCodeAdapter_RemoveSkill(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// Create skill directory.
	skillDir := filepath.Join(home, ".config", "opencode", "skills", "drup")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill"), 0o644)

	// Remove it.
	path, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("RemoveSkill error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify deleted.
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory still exists after RemoveSkill")
	}

	// Idempotent.
	path2, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("second RemoveSkill error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveSkill should return empty path, got %q", path2)
	}
}

func TestOpenCodeAdapter_RemoveAgent(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// Create agent files.
	agentsDir := filepath.Join(home, ".config", "opencode", "agents")
	os.MkdirAll(agentsDir, 0o755)
	os.WriteFile(filepath.Join(agentsDir, "drup-preflight.md"), []byte("# preflight"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "drup-contrib.md"), []byte("# contrib"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "other-agent.md"), []byte("# other"), 0o644)

	// Remove all drup agents using glob pattern.
	path, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("RemoveAgent error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify drup agents deleted.
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-preflight.md")); !os.IsNotExist(err) {
		t.Error("drup-preflight.md still exists")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-contrib.md")); !os.IsNotExist(err) {
		t.Error("drup-contrib.md still exists")
	}

	// Other agent preserved.
	if _, err := os.Stat(filepath.Join(agentsDir, "other-agent.md")); os.IsNotExist(err) {
		t.Error("other-agent.md was deleted — should be preserved")
	}

	// Idempotent.
	path2, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("second RemoveAgent error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveAgent should return empty path, got %q", path2)
	}
}

func TestOpenCodeAdapter_RemoveMCPConfig_PreservesOtherKeys(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// Pre-populate opencode.json with multiple MCP servers.
	configDir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(configDir, 0o755)
	configPath := filepath.Join(configDir, "opencode.json")
	existing := `{
  "agent": {"default": "test"},
  "mcp": {
    "context7": {"type": "remote", "url": "https://example.com"},
    "engram": {"type": "local", "command": ["engram", "mcp"]},
    "drup": {"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}
  },
  "permission": {"bash": {"*": "allow"}}
}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	// Remove drup MCP config.
	path, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("RemoveMCPConfig error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Read back and verify.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// Other top-level keys preserved.
	if _, ok := result["agent"]; !ok {
		t.Error("existing 'agent' key not preserved")
	}
	if _, ok := result["permission"]; !ok {
		t.Error("existing 'permission' key not preserved")
	}

	// Other MCP entries preserved.
	mcp, ok := result["mcp"].(map[string]any)
	if !ok {
		t.Fatal("mcp key missing or not an object")
	}
	if _, ok := mcp["context7"]; !ok {
		t.Error("existing 'context7' MCP entry not preserved")
	}
	if _, ok := mcp["engram"]; !ok {
		t.Error("existing 'engram' MCP entry not preserved")
	}

	// Drup entry removed.
	if _, ok := mcp["drup"]; ok {
		t.Error("drup MCP entry still exists after RemoveMCPConfig")
	}
}

func TestOpenCodeAdapter_RemoveMCPConfig_RemovesEmptyMCP(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// Config with only drup in mcp.
	configDir := filepath.Join(home, ".config", "opencode")
	os.MkdirAll(configDir, 0o755)
	configPath := filepath.Join(configDir, "opencode.json")
	existing := `{
  "mcp": {
    "drup": {"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}
  }
}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	// Remove drup MCP config.
	_, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("RemoveMCPConfig error: %v", err)
	}

	// Read back and verify mcp key removed.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	// mcp key should be removed when empty.
	if _, ok := result["mcp"]; ok {
		t.Error("mcp key should be removed when empty")
	}
}

func TestOpenCodeAdapter_RemoveMCPConfig_Idempotent(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	// No config file exists.
	path, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("RemoveMCPConfig error: %v", err)
	}
	if path != "" {
		t.Errorf("RemoveMCPConfig on missing file should return empty path, got %q", path)
	}
}

func TestCodexAdapter_RemoveSkill(t *testing.T) {
	home := t.TempDir()
	adapter := &CodexAdapter{HomeDir: home}

	// Create skill directory.
	skillDir := filepath.Join(home, ".codex", "skills", "drup")
	os.MkdirAll(skillDir, 0o755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# skill"), 0o644)

	// Remove it.
	path, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("RemoveSkill error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify deleted.
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory still exists after RemoveSkill")
	}

	// Idempotent.
	path2, err := adapter.RemoveSkill("drup", false)
	if err != nil {
		t.Fatalf("second RemoveSkill error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveSkill should return empty path, got %q", path2)
	}
}

func TestCodexAdapter_RemoveAgent(t *testing.T) {
	home := t.TempDir()
	adapter := &CodexAdapter{HomeDir: home}

	// Create agent files.
	agentsDir := filepath.Join(home, ".codex", "agents")
	os.MkdirAll(agentsDir, 0o755)
	os.WriteFile(filepath.Join(agentsDir, "drup-preflight.md"), []byte("# preflight"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "drup-contrib.md"), []byte("# contrib"), 0o644)
	os.WriteFile(filepath.Join(agentsDir, "other-agent.md"), []byte("# other"), 0o644)

	// Remove all drup agents using glob pattern.
	path, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("RemoveAgent error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify drup agents deleted.
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-preflight.md")); !os.IsNotExist(err) {
		t.Error("drup-preflight.md still exists")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-contrib.md")); !os.IsNotExist(err) {
		t.Error("drup-contrib.md still exists")
	}

	// Other agent preserved.
	if _, err := os.Stat(filepath.Join(agentsDir, "other-agent.md")); os.IsNotExist(err) {
		t.Error("other-agent.md was deleted — should be preserved")
	}

	// Idempotent.
	path2, err := adapter.RemoveAgent("drup-*.md", false)
	if err != nil {
		t.Fatalf("second RemoveAgent error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveAgent should return empty path, got %q", path2)
	}
}

func TestCodexAdapter_RemoveMCPConfig(t *testing.T) {
	home := t.TempDir()
	adapter := &CodexAdapter{HomeDir: home}

	// Create MCP config.
	mcpPath := filepath.Join(home, ".codex", "mcp.json")
	os.MkdirAll(filepath.Dir(mcpPath), 0o755)
	os.WriteFile(mcpPath, []byte(`{"command":"drup"}`), 0o644)

	// Remove it.
	path, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("RemoveMCPConfig error: %v", err)
	}
	if path == "" {
		t.Error("expected path returned, got empty")
	}

	// Verify deleted.
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("MCP config still exists after RemoveMCPConfig")
	}

	// Idempotent.
	path2, err := adapter.RemoveMCPConfig(false)
	if err != nil {
		t.Fatalf("second RemoveMCPConfig error: %v", err)
	}
	if path2 != "" {
		t.Errorf("second RemoveMCPConfig should return empty path, got %q", path2)
	}
}

// Phase 2: Uninstall orchestration tests

func TestUninstall_CallsAllRemoveMethods(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	// Mock CWD so Claude's .mcp.json resolves predictably.
	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	// Install something first.
	files := map[string]string{
		"SKILL.md":                 "# Test\n",
		".mcp.json":                `{"command":"drup"}`,
		"agents/drup-preflight.md": "# Preflight\n",
		"agents/drup-contrib.md":   "# Contrib\n",
	}
	if _, err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Verify files exist.
	skillDir := filepath.Join(home, ".claude", "skills", "drup")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Fatal("skill dir not created")
	}

	// Uninstall.
	paths, err := Uninstall(agents, false)
	if err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	// Verify paths returned.
	if len(paths) == 0 {
		t.Error("expected paths returned, got empty")
	}

	// Verify files deleted.
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Error("skill directory still exists after Uninstall")
	}
	agentsDir := filepath.Join(home, ".claude", "agents")
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-preflight.md")); !os.IsNotExist(err) {
		t.Error("drup-preflight.md still exists after Uninstall")
	}
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-contrib.md")); !os.IsNotExist(err) {
		t.Error("drup-contrib.md still exists after Uninstall")
	}
	mcpPath := filepath.Join(home, ".mcp.json")
	if _, err := os.Stat(mcpPath); !os.IsNotExist(err) {
		t.Error("MCP config still exists after Uninstall")
	}
}

func TestUninstall_DryRun(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	// Mock CWD so Claude's .mcp.json resolves predictably.
	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	// Install something first.
	files := map[string]string{
		"SKILL.md":                 "# Test\n",
		".mcp.json":                `{"command":"drup"}`,
		"agents/drup-preflight.md": "# Preflight\n",
	}
	if _, err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Uninstall with dry-run.
	paths, err := Uninstall(agents, true)
	if err != nil {
		t.Fatalf("Uninstall error: %v", err)
	}

	// Verify paths returned.
	if len(paths) == 0 {
		t.Error("expected paths returned, got empty")
	}

	// Verify files NOT deleted (dry-run).
	skillDir := filepath.Join(home, ".claude", "skills", "drup")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Error("skill directory deleted in dry-run mode — should be preserved")
	}
	agentsDir := filepath.Join(home, ".claude", "agents")
	if _, err := os.Stat(filepath.Join(agentsDir, "drup-preflight.md")); os.IsNotExist(err) {
		t.Error("drup-preflight.md deleted in dry-run mode — should be preserved")
	}
}

func TestUninstall_Idempotent(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	// First uninstall (nothing installed).
	paths1, err := Uninstall(agents, false)
	if err != nil {
		t.Fatalf("first Uninstall error: %v", err)
	}

	// Second uninstall (should be idempotent).
	paths2, err := Uninstall(agents, false)
	if err != nil {
		t.Fatalf("second Uninstall error: %v", err)
	}

	// Both should succeed without error.
	_ = paths1
	_ = paths2
}

// WriteSkill tests — verify directory structure creation.

func TestWriteSkill_CreatesDirectoryStructure(t *testing.T) {
	home := t.TempDir()

	tests := []struct {
		name    string
		adapter AgentAdapter
		wantDir string
	}{
		{
			name:    "Claude creates skills/<name>/SKILL.md",
			adapter: &ClaudeAdapter{HomeDir: home},
			wantDir: filepath.Join(home, ".claude", "skills", "drup"),
		},
		{
			name:    "OpenCode creates skills/<name>/SKILL.md",
			adapter: &OpenCodeAdapter{HomeDir: home},
			wantDir: filepath.Join(home, ".config", "opencode", "skills", "drup"),
		},
		{
			name:    "Codex creates skills/<name>/SKILL.md",
			adapter: &CodexAdapter{HomeDir: home},
			wantDir: filepath.Join(home, ".codex", "skills", "drup"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := "# Test Skill\nTrigger: test\n"
			if err := tt.adapter.WriteSkill("drup", content); err != nil {
				t.Fatalf("WriteSkill error: %v", err)
			}

			// Verify directory was created.
			info, err := os.Stat(tt.wantDir)
			if err != nil {
				t.Fatalf("skill directory not created at %s: %v", tt.wantDir, err)
			}
			if !info.IsDir() {
				t.Errorf("expected directory at %s, got file", tt.wantDir)
			}

			// Verify SKILL.md content.
			skillFile := filepath.Join(tt.wantDir, "SKILL.md")
			got, err := os.ReadFile(skillFile)
			if err != nil {
				t.Fatalf("read SKILL.md: %v", err)
			}
			if string(got) != content {
				t.Errorf("SKILL.md content = %q, want %q", got, content)
			}
		})
	}
}

// WriteCommand tests — verify adapter-specific behavior.

func TestWriteCommand_OpenCode(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{HomeDir: home}

	content := "# drup command\nTrigger: drup\n"
	if err := adapter.WriteCommand("drup.md", content); err != nil {
		t.Fatalf("WriteCommand error: %v", err)
	}

	// Verify file written to commands directory.
	cmdPath := filepath.Join(home, ".config", "opencode", "commands", "drup.md")
	got, err := os.ReadFile(cmdPath)
	if err != nil {
		t.Fatalf("read command file: %v", err)
	}
	if string(got) != content {
		t.Errorf("command content = %q, want %q", got, content)
	}
}

func TestWriteCommand_ClaudeIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := &ClaudeAdapter{HomeDir: home}

	// Claude does not support commands directory — WriteCommand should be a no-op.
	if err := adapter.WriteCommand("drup.md", "# test"); err != nil {
		t.Fatalf("WriteCommand should not error for Claude: %v", err)
	}

	// Verify no commands directory was created.
	cmdDir := filepath.Join(home, ".claude", "commands")
	if _, err := os.Stat(cmdDir); !os.IsNotExist(err) {
		t.Errorf("Claude should not create a commands directory, but %s exists", cmdDir)
	}
}

func TestWriteCommand_CodexIsNoop(t *testing.T) {
	home := t.TempDir()
	adapter := &CodexAdapter{HomeDir: home}

	// Codex does not support commands directory — WriteCommand should be a no-op.
	if err := adapter.WriteCommand("drup.md", "# test"); err != nil {
		t.Fatalf("WriteCommand should not error for Codex: %v", err)
	}

	// Verify no commands directory was created.
	cmdDir := filepath.Join(home, ".codex", "commands")
	if _, err := os.Stat(cmdDir); !os.IsNotExist(err) {
		t.Errorf("Codex should not create a commands directory, but %s exists", cmdDir)
	}
}

// --- Phase 3: Bootstrap file installation tests ---

func TestInstall_BootstrapFiles_Claude(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":  "# drup pipeline\n",
		"CLAUDE.md": "# Bootstrap\nLoad SKILL.md\n",
		".mcp.json": `{"command":"drup","args":["mcp"]}`,
	}

	if _, err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// CLAUDE.md should be written to project root (CWD in this test).
	claudeBootstrap := filepath.Join(home, "CLAUDE.md")
	data, err := os.ReadFile(claudeBootstrap)
	if err != nil {
		t.Fatalf("CLAUDE.md not written to project root: %v", err)
	}
	if !strings.Contains(string(data), "SKILL.md") {
		t.Error("CLAUDE.md should reference SKILL.md")
	}
}

func TestInstall_BootstrapFiles_Codex(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".codex"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":                "# drup pipeline\n",
		"copilot-instructions.md": "# Bootstrap\nLoad SKILL.md\n",
		".mcp.json":               `{"command":"drup","args":["mcp"]}`,
	}

	if _, err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// copilot-instructions.md should be written to .github/.
	codexBootstrap := filepath.Join(home, ".github", "copilot-instructions.md")
	data, err := os.ReadFile(codexBootstrap)
	if err != nil {
		t.Fatalf("copilot-instructions.md not written to .github/: %v", err)
	}
	if !strings.Contains(string(data), "SKILL.md") {
		t.Error("copilot-instructions.md should reference SKILL.md")
	}
}

// --- Phase 4: SyncFileResult and change detection tests ---

func TestInstall_AllUnchanged(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":                 "# Test Orchestrator\n",
		".mcp.json":                `{"command":"drup","args":["mcp"]}`,
		"agents/drup-preflight.md": "# Test Preflight Agent\n",
	}

	// First install — all new.
	results1, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("first Install error: %v", err)
	}
	for _, r := range results1 {
		if r.Status != FileNew {
			t.Errorf("first install: file %s expected %q, got %q", r.Path, FileNew, r.Status)
		}
	}

	// Record modtimes after first install.
	modtimes := make(map[string]int64)
	for _, r := range results1 {
		info, err := os.Stat(r.Path)
		if err != nil {
			t.Fatalf("stat %s: %v", r.Path, err)
		}
		modtimes[r.Path] = info.ModTime().UnixNano()
	}

	// Second install — all unchanged.
	results2, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("second Install error: %v", err)
	}
	if len(results2) != len(results1) {
		t.Fatalf("expected %d results, got %d", len(results1), len(results2))
	}
	for _, r := range results2 {
		if r.Status != FileUnchanged {
			t.Errorf("second install: file %s expected %q, got %q", r.Path, FileUnchanged, r.Status)
		}
	}

	// Verify files were NOT rewritten (modtimes unchanged).
	for _, r := range results2 {
		info, err := os.Stat(r.Path)
		if err != nil {
			t.Fatalf("stat %s: %v", r.Path, err)
		}
		if info.ModTime().UnixNano() != modtimes[r.Path] {
			t.Errorf("file %s was rewritten despite being unchanged", r.Path)
		}
	}
}

func TestInstall_MixedStatus(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":                 "# Test Orchestrator\n",
		".mcp.json":                `{"command":"drup","args":["mcp"]}`,
		"agents/drup-preflight.md": "# Test Preflight Agent\n",
	}

	// First install to create all files.
	_, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("first Install error: %v", err)
	}

	// Modify one file (agent definition) to have different content.
	agentPath := filepath.Join(agents[0].AgentsDir(), "drup-preflight.md")
	os.WriteFile(agentPath, []byte("# MODIFIED content\n"), 0o644)

	// Second install — should detect mixed statuses.
	results, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("second Install error: %v", err)
	}

	statusByPath := make(map[string]FileStatus)
	for _, r := range results {
		statusByPath[r.Path] = r.Status
	}

	// The modified agent file should be "modified".
	if statusByPath[agentPath] != FileModified {
		t.Errorf("agent file %s: expected %q, got %q", agentPath, FileModified, statusByPath[agentPath])
	}

	// Other files should be "unchanged".
	skillPath := filepath.Join(agents[0].SkillsDir(), "drup", "SKILL.md")
	if statusByPath[skillPath] != FileUnchanged {
		t.Errorf("skill file %s: expected %q, got %q", skillPath, FileUnchanged, statusByPath[skillPath])
	}

	mcpPath := agents[0].MCPConfigPath()
	if statusByPath[mcpPath] != FileUnchanged {
		t.Errorf("MCP file %s: expected %q, got %q", mcpPath, FileUnchanged, statusByPath[mcpPath])
	}
}

func TestInstall_MCPPostMergeComparison(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".config", "opencode"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	adapter := &OpenCodeAdapter{HomeDir: home}
	agents := []AgentAdapter{adapter}

	// Pre-populate opencode.json with other MCP entries.
	configPath := filepath.Join(home, ".config", "opencode", "opencode.json")
	existing := `{
  "mcp": {
    "context7": {"type": "remote", "url": "https://example.com"},
    "engram": {"type": "local", "command": ["engram", "mcp"]}
  },
  "permission": {"bash": {"*": "allow"}}
}`
	os.WriteFile(configPath, []byte(existing), 0o644)

	// First install — merges drup into existing config.
	snippet := `{"type": "local", "command": ["/usr/local/bin/drup", "mcp"]}`
	files := map[string]string{
		"SKILL.md":  "# Test\n",
		".mcp.json": snippet,
	}
	results1, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("first Install error: %v", err)
	}

	// Find the MCP config result.
	var mcpResult *SyncFileResult
	for i := range results1 {
		if results1[i].Path == configPath {
			mcpResult = &results1[i]
			break
		}
	}
	if mcpResult == nil {
		t.Fatal("MCP config result not found")
	}
	// First install: MCP config should be "modified" (existing file with different content).
	if mcpResult.Status != FileModified && mcpResult.Status != FileNew {
		t.Errorf("first install MCP: expected new or modified, got %q", mcpResult.Status)
	}

	// Read the merged config that was written.
	mergedData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	// Second install — merged output should match existing file byte-for-byte → unchanged.
	results2, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("second Install error: %v", err)
	}

	for _, r := range results2 {
		if r.Path == configPath {
			if r.Status != FileUnchanged {
				t.Errorf("second install MCP: expected %q, got %q", FileUnchanged, r.Status)
				// Debug: show what differs.
				rendered, _ := adapter.RenderMCPConfig(snippet)
				t.Logf("existing file:\n%s", string(mergedData))
				t.Logf("rendered merge:\n%s", rendered)
			}
			break
		}
	}

	// Verify other keys are still preserved in the file.
	var result map[string]any
	json.Unmarshal(mergedData, &result)
	if _, ok := result["permission"]; !ok {
		t.Error("'permission' key not preserved after merge")
	}
	mcp := result["mcp"].(map[string]any)
	if _, ok := mcp["context7"]; !ok {
		t.Error("'context7' MCP entry not preserved after merge")
	}
}

func TestInstall_BackupSkippedWhenUnchanged(t *testing.T) {
	home := t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)

	orig := homeDir
	homeDir = func() (string, error) { return home, nil }
	defer func() { homeDir = orig }()

	origCWD := getCWD
	getCWD = func() (string, error) { return home, nil }
	defer func() { getCWD = origCWD }()

	// Use a temp dir for backups.
	bDir := t.TempDir()
	origBackup := backupDir
	backupDir = func() string { return bDir }
	defer func() { backupDir = origBackup }()

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":  "# Test\n",
		".mcp.json": `{"command":"drup","args":["mcp"]}`,
	}

	// First install — creates files (backup may or may not happen since skills dir may not exist yet).
	_, err := Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("first Install error: %v", err)
	}

	// Count backups after first install.
	entries1, _ := os.ReadDir(bDir)
	backupCount1 := len(entries1)

	// Second install — all unchanged, no backup should be created.
	_, err = Install(agents, "/usr/local/bin/drup", files)
	if err != nil {
		t.Fatalf("second Install error: %v", err)
	}

	entries2, _ := os.ReadDir(bDir)
	backupCount2 := len(entries2)

	if backupCount2 != backupCount1 {
		t.Errorf("backup created on unchanged install: had %d, now %d", backupCount1, backupCount2)
	}
}
