package packaging

import (
	"testing"
)

func TestRender_Claude(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for claude")
	}
}

func TestRender_OpenCode(t *testing.T) {
	files, err := Render("opencode", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for opencode")
	}
}

func TestRender_Codex(t *testing.T) {
	files, err := Render("codex", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for codex")
	}
}

func TestRender_UnsupportedPlatform(t *testing.T) {
	_, err := Render("unknown", "/usr/local/bin/drup")
	if err == nil {
		t.Error("expected error for unsupported platform, got nil")
	}
}

func TestPlatforms(t *testing.T) {
	platforms := Platforms()
	if len(platforms) != 3 {
		t.Errorf("len(platforms) = %d, want 3", len(platforms))
	}
}
