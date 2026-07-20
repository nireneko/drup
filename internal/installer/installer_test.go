package installer

import (
	"os"
	"path/filepath"
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
