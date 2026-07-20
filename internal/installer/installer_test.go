package installer

import (
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
		"SKILL.md": "# Test Skill\n",
	}

	if err := Install(agents, "/usr/local/bin/drup", files); err != nil {
		t.Fatalf("Install error: %v", err)
	}

	// Verify file was written.
	skillPath := filepath.Join(agents[0].SkillsDir(), "SKILL.md")
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Errorf("skill file not written to %s", skillPath)
	}
}
