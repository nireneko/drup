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
	adapter := &ClaudeAdapter{homeDir: home}

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

	want := filepath.Join(home, ".claude", "mcp", "drup.json")
	if got := adapter.MCPConfigPath(); got != want {
		t.Errorf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestOpenCodeAdapter_Paths(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{homeDir: home}

	want := filepath.Join(home, ".config", "opencode", "opencode.json")
	if got := adapter.MCPConfigPath(); got != want {
		t.Errorf("MCPConfigPath() = %q, want %q", got, want)
	}
}

func TestOpenCodeAdapter_WriteMCPConfig_MergesExisting(t *testing.T) {
	home := t.TempDir()
	adapter := &OpenCodeAdapter{homeDir: home}

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
	adapter := &OpenCodeAdapter{homeDir: home}

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
	adapter := &OpenCodeAdapter{homeDir: home}

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
	adapter := &OpenCodeAdapter{homeDir: home}

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

	agents := DetectAgents()
	if len(agents) == 0 {
		t.Fatal("no agents detected")
	}

	files := map[string]string{
		"SKILL.md":                 "# Test Orchestrator\n",
		".mcp.json":                `{"command":"drup","args":["mcp"]}`,
		"agents/drup-preflight.md": "# Test Preflight Agent\n",
	}

	if err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
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
