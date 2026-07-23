package packaging

import (
	"strings"
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

// --- Cross-platform SKILL.md content tests (Phase 1) ---

func TestSKILLMD_NoPlatformPrimitives(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup")
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			content, ok := files["SKILL.md"]
			if !ok {
				t.Fatal("missing SKILL.md")
			}

			// Must NOT contain platform-specific primitives.
			forbidden := []string{"task(", "Sub-Agent Roster", "drup-preflight", "drup-validator", "drup-rector", "drup-contrib", "drup-custom", "drup-theme"}
			for _, f := range forbidden {
				if strings.Contains(content, f) {
					t.Errorf("SKILL.md for %s contains forbidden platform primitive %q", platform, f)
				}
			}
		})
	}
}

func TestSKILLMD_ContainsDrupCLIPipeline(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup")
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			content := files["SKILL.md"]

			// Must contain drup CLI pipeline stages.
			required := []string{"drup preflight", "drup scan", "drup fix", "drup contrib", "drup upgrade-core"}
			for _, r := range required {
				if !strings.Contains(content, r) {
					t.Errorf("SKILL.md for %s missing required CLI stage %q", platform, r)
				}
			}
		})
	}
}

func TestSKILLMD_CrossPlatformIdentical(t *testing.T) {
	opencodeFiles, _ := Render("opencode", "/usr/local/bin/drup")
	claudeFiles, _ := Render("claude", "/usr/local/bin/drup")
	codexFiles, _ := Render("codex", "/usr/local/bin/drup")

	opencodeSKILL := opencodeFiles["SKILL.md"]
	claudeSKILL := claudeFiles["SKILL.md"]
	codexSKILL := codexFiles["SKILL.md"]

	if opencodeSKILL != claudeSKILL {
		t.Error("opencode and claude SKILL.md should be identical")
	}
	if opencodeSKILL != codexSKILL {
		t.Error("opencode and codex SKILL.md should be identical")
	}
}

func TestRender_NoAgentFiles(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup")
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			for key := range files {
				if strings.HasPrefix(key, "agents/") {
					t.Errorf("platform %s should have no agent files, found %q", platform, key)
				}
			}
		})
	}
}

func TestRender_ClaudeBootstrap(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	content, ok := files["CLAUDE.md"]
	if !ok {
		t.Fatal("missing CLAUDE.md bootstrap for claude")
	}
	if !strings.Contains(content, "SKILL.md") {
		t.Error("CLAUDE.md must reference SKILL.md")
	}
}

func TestRender_CodexBootstrap(t *testing.T) {
	files, err := Render("codex", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	content, ok := files["copilot-instructions.md"]
	if !ok {
		t.Fatal("missing copilot-instructions.md bootstrap for codex")
	}
	if !strings.Contains(content, "SKILL.md") {
		t.Error("copilot-instructions.md must reference SKILL.md")
	}
}

func TestRender_BootstrapSkillPathSubstitution(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	content := files["CLAUDE.md"]
	if strings.Contains(content, "{{SKILL_PATH}}") {
		t.Error("CLAUDE.md should have {{SKILL_PATH}} substituted")
	}
}

// Task 5.4: Verify new skill files exist and contain trigger phrases.

func TestRender_D11FixesSkillExists(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup")
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			key := "skills/drupal-custom-d11-fixes/SKILL.md"
			content, ok := files[key]
			if !ok {
				t.Fatalf("missing %s for %s", key, platform)
			}
			if !strings.Contains(content, "drupal-custom-d11-fixes") {
				t.Error("D11 fixes skill should contain its name")
			}
			if !strings.Contains(content, "deprecation") {
				t.Error("D11 fixes skill should contain deprecation patterns")
			}
		})
	}
}

func TestRender_ContribPatchSkillExists(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup")
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			key := "skills/drupal-contrib-patch-writer/SKILL.md"
			content, ok := files[key]
			if !ok {
				t.Fatalf("missing %s for %s", key, platform)
			}
			if !strings.Contains(content, "drupal-contrib-patch-writer") {
				t.Error("Contrib patch skill should contain its name")
			}
			// Verify all 4 categories are present.
			for _, cat := range []string{"Category A", "Category B", "Category C", "Category D"} {
				if !strings.Contains(content, cat) {
					t.Errorf("Contrib patch skill missing %s", cat)
				}
			}
		})
	}
}
