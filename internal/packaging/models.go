package packaging

import (
	"fmt"
	"slices"

	"github.com/nireneko/drup/internal/state"
)

// agentNames lists the sub-agents drup installs, in the same order the
// SKILL.md roster tables render them. All 3 supported platforms currently
// ship the same 6-agent roster.
var agentNames = []string{
	"drup-preflight",
	"drup-rector",
	"drup-contrib",
	"drup-custom",
	"drup-theme",
	"drup-validator",
}

// AgentNames returns the sub-agents drup installs for a platform.
func AgentNames(platform string) ([]string, error) {
	if !slices.Contains(Platforms(), platform) {
		return nil, fmt.Errorf("unsupported platform: %s", platform)
	}
	return agentNames, nil
}

// builtinModels holds the pre-configuration literal models, identical to
// what was hardcoded before this feature (REQ-003 — empty/missing
// ModelAssignments must render byte-identical output).
//
// Every fixer agent's (preflight/rector/contrib/custom/theme) escalation
// tier equals drup-validator's own default on that platform: the "strong"
// model already used for gate confirmation there. drup-validator's own
// escalation equals its default — it already runs on the strong tier and
// this catalog has no stronger fallback for it (design decision 7).
var builtinModels = map[string]map[string]state.ModelPhaseAssignment{
	"claude": {
		"drup-preflight": {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
		"drup-rector":    {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
		"drup-contrib":   {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
		"drup-custom":    {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
		"drup-theme":     {Default: "claude-haiku-4-5-20251001", Escalation: "claude-sonnet-5"},
		"drup-validator": {Default: "claude-sonnet-5", Escalation: "claude-sonnet-5"},
	},
	"opencode": {
		"drup-preflight": {Default: "openrouter/qwen/qwen3-30b-a3b:free", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
		"drup-rector":    {Default: "openrouter/qwen/qwen3-30b-a3b:free", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
		"drup-contrib":   {Default: "openrouter/qwen/qwen3-30b-a3b:free", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
		"drup-custom":    {Default: "openrouter/qwen/qwen3-30b-a3b:free", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
		"drup-theme":     {Default: "openrouter/qwen/qwen3-30b-a3b:free", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
		"drup-validator": {Default: "openrouter/qwen/qwen3-235b-a22b", Escalation: "openrouter/qwen/qwen3-235b-a22b"},
	},
	"codex": {
		"drup-preflight": {Default: "gpt-4o-mini", Escalation: "gpt-4o"},
		"drup-rector":    {Default: "gpt-4o-mini", Escalation: "gpt-4o"},
		"drup-contrib":   {Default: "gpt-4o-mini", Escalation: "gpt-4o"},
		"drup-custom":    {Default: "gpt-4o-mini", Escalation: "gpt-4o"},
		"drup-theme":     {Default: "gpt-4o-mini", Escalation: "gpt-4o"},
		"drup-validator": {Default: "gpt-4o", Escalation: "gpt-4o"},
	},
}

// resolveModel resolves the effective {default, escalation} pair for one
// agent on one platform: user-configured value (per field) wins, an
// unconfigured field falls back to the built-in literal (REQ-003).
func resolveModel(agent, platform string, assignments map[string]map[string]state.ModelPhaseAssignment) state.ModelPhaseAssignment {
	resolved := builtinModels[platform][agent]
	if perAgent, ok := assignments[platform]; ok {
		if configured, ok := perAgent[agent]; ok {
			if configured.Default != "" {
				resolved.Default = configured.Default
			}
			if configured.Escalation != "" {
				resolved.Escalation = configured.Escalation
			}
		}
	}
	return resolved
}

// validateAssignments fails closed on any platform/agent key not in the
// known table, and on any model string that could corrupt generated
// frontmatter/TOML (REQ-001 "unknown key rejected" scenario).
//
// An unknown top-level platform key is rejected regardless of which
// platform is being rendered — there is no legitimate render call it could
// ever satisfy, so it always signals a config-wide typo. An unknown agent
// key (or a bad value) nested under a *known* platform is scoped to the
// platform being rendered: installAgents calls Render once per detected
// platform with the SAME full assignments map, so a typo under "codex"
// must not also fail the unrelated "claude" render (design decision 4 —
// "one bad platform fails alone").
func validateAssignments(platform string, assignments map[string]map[string]state.ModelPhaseAssignment) error {
	for p := range assignments {
		if !slices.Contains(Platforms(), p) {
			return fmt.Errorf("model assignments: unknown platform %q", p)
		}
	}

	agents, ok := assignments[platform]
	if !ok {
		return nil
	}
	for agent, assignment := range agents {
		if !slices.Contains(agentNames, agent) {
			return fmt.Errorf("model assignments: unknown agent %q for platform %q", agent, platform)
		}
		if err := state.ValidateModelValue(assignment.Default); err != nil {
			return fmt.Errorf("model assignments %s.%s.default: %w", platform, agent, err)
		}
		if err := state.ValidateModelValue(assignment.Escalation); err != nil {
			return fmt.Errorf("model assignments %s.%s.escalation: %w", platform, agent, err)
		}
	}
	return nil
}
