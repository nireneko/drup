package packaging

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/nireneko/drup/internal/state"
)

func TestRender_Claude(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for claude")
	}
}

func TestRender_OpenCode(t *testing.T) {
	files, err := Render("opencode", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for opencode")
	}
}

func TestRender_Codex(t *testing.T) {
	files, err := Render("codex", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	if _, ok := files["SKILL.md"]; !ok {
		t.Error("missing SKILL.md for codex")
	}
	if _, ok := files["mcp.json"]; !ok {
		t.Error("missing MCP template for codex")
	}
}

// --- RenderLocked / --locked mcp.json arg (specs/mcp-server Kill Switch and
// Dry-Run Partition; installer parity for the --locked flag) ---

func TestRenderLocked_OmitsFlagByDefault(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			unlocked, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			explicit, err := RenderLocked(platform, "/usr/local/bin/drup", false, nil)
			if err != nil {
				t.Fatalf("RenderLocked(locked=false) error: %v", err)
			}
			if unlocked["mcp.json"] != explicit["mcp.json"] {
				t.Errorf("%s: Render and RenderLocked(false) must render mcp.json byte-identically\nRender:       %q\nRenderLocked: %q", platform, unlocked["mcp.json"], explicit["mcp.json"])
			}
			if strings.Contains(unlocked["mcp.json"], "--locked") {
				t.Errorf("%s: mcp.json must omit --locked by default, got %q", platform, unlocked["mcp.json"])
			}
			if !json.Valid([]byte(unlocked["mcp.json"])) {
				t.Errorf("%s: unlocked mcp.json is not valid JSON: %q", platform, unlocked["mcp.json"])
			}
		})
	}
}

func TestRenderLocked_IncludesFlagWhenSelected(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := RenderLocked(platform, "/usr/local/bin/drup", true, nil)
			if err != nil {
				t.Fatalf("RenderLocked(locked=true) error: %v", err)
			}
			mcpJSON, ok := files["mcp.json"]
			if !ok {
				t.Fatalf("%s: missing mcp.json", platform)
			}
			if !strings.Contains(mcpJSON, "--locked") {
				t.Errorf("%s: locked mcp.json must contain --locked, got %q", platform, mcpJSON)
			}
			if !json.Valid([]byte(mcpJSON)) {
				t.Errorf("%s: locked mcp.json is not valid JSON: %q", platform, mcpJSON)
			}
		})
	}
}

// TestRenderLocked_ParityAcrossPlatforms guards the parity requirement: all
// three platforms must agree on whether --locked is present for a given
// locked selection (mirrors TestSKILLMD_CrossPlatformIdentical's pattern).
func TestRenderLocked_ParityAcrossPlatforms(t *testing.T) {
	for _, locked := range []bool{false, true} {
		var presence []bool
		for _, platform := range Platforms() {
			files, err := RenderLocked(platform, "/usr/local/bin/drup", locked, nil)
			if err != nil {
				t.Fatalf("RenderLocked(%v) error for %s: %v", locked, platform, err)
			}
			presence = append(presence, strings.Contains(files["mcp.json"], "--locked"))
		}
		for i := 1; i < len(presence); i++ {
			if presence[i] != presence[0] {
				t.Errorf("locked=%v: platform %s presence=%v disagrees with %s presence=%v", locked, Platforms()[i], presence[i], Platforms()[0], presence[0])
			}
		}
	}
}

func TestValidateCodexSkill(t *testing.T) {
	if err := validateCodexSkill("SKILL.md", "---\nname: test\ndescription: test\n---\n# Test\n"); err != nil {
		t.Fatalf("valid Codex skill rejected: %v", err)
	}
	if err := validateCodexSkill("SKILL.md", "# Test\n"); err == nil {
		t.Fatal("invalid Codex skill accepted")
	}
}

func TestRender_UnsupportedPlatform(t *testing.T) {
	_, err := Render("unknown", "/usr/local/bin/drup", nil)
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
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			content, ok := files["SKILL.md"]
			if !ok {
				t.Fatal("missing SKILL.md")
			}

			// Must NOT contain platform-specific primitives.
			forbidden := []string{"task("}
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
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			content := files["SKILL.md"]

			// Must contain the orchestrated pipeline and backup safety gate.
			required := []string{"Stage 0: SAFETY BACKUP", "Stage 1: PREFLIGHT", "Stage 6: CORE UPGRADE", "Stage 9: BACKUP FINALIZATION", "test-backup-create"}
			for _, r := range required {
				if !strings.Contains(content, r) {
					t.Errorf("SKILL.md for %s missing required CLI stage %q", platform, r)
				}
			}
		})
	}
}

func TestSKILLMD_CrossPlatformIdentical(t *testing.T) {
	for _, platform := range Platforms() {
		files, _ := Render(platform, "/usr/local/bin/drup", nil)
		content := files["SKILL.md"]
		for _, required := range []string{"Stage 0: SAFETY BACKUP", "test-backup-create", "test-backup-restore", "manual `test-backup-delete`", "report its `backup_id` and path"} {
			if !strings.Contains(content, required) {
				t.Errorf("%s SKILL.md missing shared lifecycle rule %q", platform, required)
			}
		}
		for _, forbidden := range []string{
			"Successful run and final validation has zero errors: run `drup test-backup-delete",
			"delete it only after successful final validation",
		} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s SKILL.md still automates backup deletion: %q", platform, forbidden)
			}
		}
	}
}

func TestRender_AgentFiles(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			agents := 0
			for key := range files {
				if strings.HasPrefix(key, "agents/") {
					agents++
				}
			}
			if agents == 0 {
				t.Errorf("platform %s should include orchestrator agent files", platform)
			}
			if platform == "codex" {
				for key, content := range files {
					if strings.HasPrefix(key, "agents/") {
						if !strings.HasSuffix(key, ".toml") {
							t.Errorf("Codex agent %s must be installed as TOML", key)
						}
						if !strings.Contains(content, "developer_instructions = '''") {
							t.Errorf("Codex agent %s lacks developer_instructions", key)
						}
					}
				}
			}
		})
	}
}

func TestRender_ClaudeBootstrap(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if _, ok := files["CLAUDE.md"]; ok {
		t.Error("global Claude install must not overwrite project CLAUDE.md")
	}
}

func TestRender_CodexBootstrap(t *testing.T) {
	files, err := Render("codex", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if _, ok := files["copilot-instructions.md"]; ok {
		t.Error("global Codex install must not overwrite project instructions")
	}
}

func TestRender_BootstrapSkillPathSubstitution(t *testing.T) {
	files, err := Render("claude", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	for path, content := range files {
		if strings.Contains(content, "{{SKILL_PATH}}") {
			t.Errorf("%s should have {{SKILL_PATH}} substituted", path)
		}
	}
}

// Task 5.4: Verify new skill files exist and contain trigger phrases.

func TestRender_D11FixesSkillExists(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
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
			files, err := Render(platform, "/usr/local/bin/drup", nil)
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

func TestRenderCodexAgentConfig_RejectsUnquotableContent(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "unquoted description",
			content: "+++\ndescription = bare words\n+++\n\nInstructions.\n",
			wantErr: "quoted single-line string",
		},
		{
			name:    "instructions close the literal string",
			content: "+++\ndescription = \"Agent\"\n+++\n\nUse ''' carefully.\n",
			wantErr: "must not contain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := renderCodexAgentConfig("agents/drup-test.md", tt.content)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRenderCodexAgentConfig_ValidTemplate(t *testing.T) {
	got, err := renderCodexAgentConfig("agents/drup-test.md", "+++\ndescription = \"Agent\"\n+++\n\n# Title\n\nDo the work.\n")
	if err != nil {
		t.Fatalf("renderCodexAgentConfig error: %v", err)
	}
	want := "description = \"Agent\"\ndeveloper_instructions = '''\n# Title\n\nDo the work.\n'''\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderCodexAgentConfig_PreservesModel(t *testing.T) {
	got, err := renderCodexAgentConfig("agents/drup-test.md", "+++\ndescription = \"Agent\"\nmodel = \"gpt-4o-mini\"\n+++\n\n# Title\n\nDo the work.\n")
	if err != nil {
		t.Fatalf("renderCodexAgentConfig error: %v", err)
	}
	want := "description = \"Agent\"\nmodel = \"gpt-4o-mini\"\ndeveloper_instructions = '''\n# Title\n\nDo the work.\n'''\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderCodexAgentConfig_RejectsUnquotedModel(t *testing.T) {
	_, err := renderCodexAgentConfig("agents/drup-test.md", "+++\ndescription = \"Agent\"\nmodel = bare\n+++\n\nInstructions.\n")
	if err == nil {
		t.Fatal("expected an error for an unquoted model field, got nil")
	}
	if !strings.Contains(err.Error(), "quoted single-line string") {
		t.Errorf("error = %q, want it to mention quoting", err.Error())
	}
}

// --- Configurable per-phase models (Phase 2) ---

func TestRender_NilAssignments_ByteIdentical(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			withNil, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render(nil) error: %v", err)
			}
			withEmpty, err := Render(platform, "/usr/local/bin/drup", map[string]map[string]state.ModelPhaseAssignment{})
			if err != nil {
				t.Fatalf("Render(empty) error: %v", err)
			}
			if len(withNil) != len(withEmpty) {
				t.Fatalf("file count differs: nil=%d empty=%d", len(withNil), len(withEmpty))
			}
			for path, content := range withNil {
				if withEmpty[path] != content {
					t.Errorf("%s differs between nil and empty assignments", path)
				}
			}
		})
	}
}

func TestRender_SubstitutionCorrectness(t *testing.T) {
	assignments := map[string]map[string]state.ModelPhaseAssignment{
		"claude": {
			"drup-rector": {Default: "claude-opus-4", Escalation: "claude-opus-4-max"},
		},
	}
	files, err := Render("claude", "/usr/local/bin/drup", assignments)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	content, ok := files["agents/drup-rector.md"]
	if !ok {
		t.Fatal("missing agents/drup-rector.md")
	}
	if !strings.Contains(content, "model: claude-opus-4\n") {
		t.Errorf("frontmatter model not substituted:\n%s", content)
	}
	if !strings.Contains(content, "claude-opus-4") || !strings.Contains(content, "claude-opus-4-max") {
		t.Errorf("prose does not mention configured default/escalation:\n%s", content)
	}

	// Untouched agent must still resolve to its built-in default.
	other, ok := files["agents/drup-contrib.md"]
	if !ok {
		t.Fatal("missing agents/drup-contrib.md")
	}
	if !strings.Contains(other, "model: claude-haiku-4-5-20251001\n") {
		t.Errorf("unconfigured agent should keep the built-in default:\n%s", other)
	}
}

func TestRender_UnknownPlatformKeyRejected(t *testing.T) {
	assignments := map[string]map[string]state.ModelPhaseAssignment{
		"chatgpt": {"drup-rector": {Default: "gpt-5"}},
	}
	files, err := Render("claude", "/usr/local/bin/drup", assignments)
	if err == nil {
		t.Fatal("expected an error for an unknown platform key, got nil")
	}
	if len(files) != 0 {
		t.Errorf("expected zero files written, got %d", len(files))
	}
}

func TestRender_UnknownAgentKeyRejected(t *testing.T) {
	assignments := map[string]map[string]state.ModelPhaseAssignment{
		"claude": {"drup-not-a-real-agent": {Default: "claude-opus-4"}},
	}
	files, err := Render("claude", "/usr/local/bin/drup", assignments)
	if err == nil {
		t.Fatal("expected an error for an unknown agent key, got nil")
	}
	if len(files) != 0 {
		t.Errorf("expected zero files written, got %d", len(files))
	}
}

func TestRender_InjectionCharsRejected(t *testing.T) {
	tests := []string{"claude-opus\n4", `claude"opus`, `claude\opus`, "claude#opus", " claude-opus"}
	for _, v := range tests {
		t.Run(v, func(t *testing.T) {
			assignments := map[string]map[string]state.ModelPhaseAssignment{
				"claude": {"drup-rector": {Default: v}},
			}
			files, err := Render("claude", "/usr/local/bin/drup", assignments)
			if err == nil {
				t.Fatalf("expected an error for injection-unsafe model %q, got nil", v)
			}
			if len(files) != 0 {
				t.Errorf("expected zero files written, got %d", len(files))
			}
		})
	}
}

func TestRender_ArbitraryModelStringAccepted(t *testing.T) {
	assignments := map[string]map[string]state.ModelPhaseAssignment{
		"claude": {"drup-rector": {Escalation: "some-future-model-id"}},
	}
	files, err := Render("claude", "/usr/local/bin/drup", assignments)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(files["agents/drup-rector.md"], "some-future-model-id") {
		t.Error("arbitrary valid model string should pass through unmodified")
	}
}

func TestRender_NoResidualPlaceholders(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			for path, content := range files {
				if strings.Contains(content, "{{MODEL_") {
					t.Errorf("%s/%s still contains an unsubstituted {{MODEL_ placeholder", platform, path)
				}
			}
		})
	}
}

// foreignModelVocab holds distinguishing substrings from each platform's
// built-in model literals (see models.go builtinModels). A platform's
// rendered output must never contain another platform's vocabulary — that
// pattern (a hardcoded literal that bypassed the {{MODEL_ placeholder
// grammar entirely) is exactly how CRITICAL-1 survived TestRender_NoResidualPlaceholders:
// a literal that was never converted to a placeholder has no "{{MODEL_" to
// detect.
var foreignModelVocab = map[string][]string{
	"claude":   {"haiku", "sonnet"},
	"opencode": {"qwen3"},
	"codex":    {"gpt-4o"},
}

func TestRender_NoForeignPlatformModelLiterals(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			for otherPlatform, words := range foreignModelVocab {
				if otherPlatform == platform {
					continue
				}
				for _, word := range words {
					for path, content := range files {
						if strings.Contains(content, word) {
							t.Errorf("%s/%s contains foreign vocabulary %q belonging to platform %q — a hardcoded literal likely bypassed the {{MODEL_ substitution", platform, path, word, otherPlatform)
						}
					}
				}
			}
		})
	}
}

func TestRender_OpenCodeFrontmatterProseAgree(t *testing.T) {
	files, err := Render("opencode", "/usr/local/bin/drup", nil)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	modelLine := regexp.MustCompile(`(?m)^model: (.+)$`)
	for path, content := range files {
		if !strings.HasPrefix(path, "agents/") {
			continue
		}
		m := modelLine.FindStringSubmatch(content)
		if m == nil {
			t.Errorf("%s: no frontmatter model line found", path)
			continue
		}
		frontmatterModel := m[1]
		if !strings.Contains(content, frontmatterModel) {
			continue
		}
		if !strings.Contains(content, "Default model:") {
			t.Errorf("%s: missing 'Default model:' prose", path)
			continue
		}
		prose := content[strings.Index(content, "Default model:"):]
		if !strings.Contains(prose, frontmatterModel) {
			t.Errorf("%s: frontmatter model %q not reachable from prose:\n%s", path, frontmatterModel, prose)
		}
	}
}

func TestRender_RosterReflectsOverride(t *testing.T) {
	assignments := map[string]map[string]state.ModelPhaseAssignment{
		"claude": {
			"drup-rector": {Default: "claude-opus-4", Escalation: "claude-opus-4"},
		},
	}
	files, err := Render("claude", "/usr/local/bin/drup", assignments)
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	skill, ok := files["SKILL.md"]
	if !ok {
		t.Fatal("missing SKILL.md")
	}
	// The overridden agent's own roster row must reflect the configured
	// value, not the built-in literal — other, unconfigured agent rows are
	// unaffected and keep showing their own built-in default.
	rosterLine := ""
	for _, line := range strings.Split(skill, "\n") {
		if strings.Contains(line, "| drup-rector |") {
			rosterLine = line
			break
		}
	}
	if rosterLine == "" {
		t.Fatal("missing drup-rector roster row")
	}
	if strings.Contains(rosterLine, "claude-haiku-4-5-20251001") {
		t.Errorf("drup-rector roster row still shows the built-in literal: %q", rosterLine)
	}
	if !strings.Contains(rosterLine, "claude-opus-4") {
		t.Errorf("drup-rector roster row does not reflect the override: %q", rosterLine)
	}
	if !strings.Contains(rosterLine, "2 retries") {
		t.Errorf("drup-rector roster row lost its retry annotation: %q", rosterLine)
	}
}

func TestRender_DrupValidatorProseMatchesFrontmatter(t *testing.T) {
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			var content string
			for path, c := range files {
				if strings.Contains(path, "drup-validator") {
					content = c
					break
				}
			}
			if content == "" {
				t.Fatal("missing drup-validator agent file")
			}
			if strings.Contains(content, "Default model: haiku") {
				t.Error("drup-validator prose still contradicts its non-cheap frontmatter default")
			}
		})
	}
}

// --- REQ-5: Sub-agent templates must reference result.payload ---

func TestSubAgentTemplates_ContainPayloadReference(t *testing.T) {
	agents := []string{"drup-preflight", "drup-rector", "drup-contrib", "drup-custom", "drup-theme", "drup-validator"}
	for _, platform := range Platforms() {
		t.Run(platform, func(t *testing.T) {
			files, err := Render(platform, "/usr/local/bin/drup", nil)
			if err != nil {
				t.Fatalf("Render error: %v", err)
			}
			for _, agent := range agents {
				path := "agents/" + agent + ".md"
				if platform == "codex" {
					path = "agents/" + agent + ".toml"
				}
				content, ok := files[path]
				if !ok {
					t.Errorf("missing %s", path)
					continue
				}
				if !strings.Contains(content, "result.payload") {
					t.Errorf("%s does not reference result.payload — sub-agents must read tool responses from the envelope payload", path)
				}
			}
		})
	}
}
