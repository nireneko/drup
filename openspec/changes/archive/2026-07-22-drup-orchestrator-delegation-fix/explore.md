## Exploration: drup-orchestrator-delegation-fix

### Current State

The drup skill defines a 7-stage pipeline with a "pure coordinator" orchestrator that has ZERO execute permission. It delegates all work to 6 sub-agents (drup-preflight, drup-rector, drup-contrib, drup-custom, drup-theme, drup-validator) via OpenCode's `task()` tool. The skill is installed at `~/.config/opencode/skills/drup/SKILL.md` and the sub-agent markdown files are installed at `~/.config/opencode/agents/drup-*.md`.

However, the `gentle-orchestrator` agent in `opencode.json` has a permission block that uses `"*": "deny"` for task delegation and only explicitly allows SDD and review sub-agents. The drup sub-agents are NOT in the allow list. The orchestrator also has `bash: true, edit: true, write: true` — full execution capability — which contradicts the SKILL.md's "zero execute permission" rule.

### Affected Areas

- `~/.config/opencode/opencode.json` — `gentle-orchestrator.permission.task` blocks drup sub-agent dispatch; `gentle-orchestrator.tools` grants execution capability that contradicts the skill's coordinator-only rule
- `~/.config/opencode/skills/drup/SKILL.md` — the installed orchestrator skill (template source: `internal/packaging/templates/opencode/SKILL.md`)
- `~/.config/opencode/agents/drup-*.md` — 6 sub-agent definition files (installed from templates)
- `internal/packaging/templates/opencode/SKILL.md` — template source for the orchestrator skill
- `internal/packaging/templates/opencode/agents/*.md` — template sources for all 6 sub-agents
- `openspec/specs/orchestrator-skill/spec.md` — spec encoding the zero-execute rule
- `openspec/specs/agent-packaging/spec.md` — spec for agent template generation

### Root Cause Analysis

**Three distinct gaps compound to produce both reported symptoms:**

#### Gap 1: Permission block blocks drup sub-agent dispatch (PRIMARY)

The `gentle-orchestrator` agent's `permission.task` in `opencode.json`:

```json
{
  "task": {
    "*": "deny",
    "explore": "allow",
    "general": "allow",
    "sdd-*": "allow",
    "review-*": "allow",
    "jd-*": "allow"
  }
}
```

No `drup-*` entries exist. When the SKILL.md says "dispatch drup-contrib", the orchestrator's `task()` call is denied by the permission system. The orchestrator then has two fallback paths, both wrong:
1. **Execute directly** — it has `bash: true, edit: true, write: true`, so it runs `composer require` itself, violating its own "zero execute" rule.
2. **Tell the user to do it manually** — it gives up and outputs manual instructions.

The drup sub-agent markdown files exist in `~/.config/opencode/agents/` and are auto-discovered by OpenCode as available agents, but the orchestrator's permission block prevents it from dispatching them.

#### Gap 2: Orchestrator tools grant execution capability

The `gentle-orchestrator` agent has:

```json
"tools": {
  "bash": true,
  "edit": true,
  "question": true,
  "read": true,
  "task": true,
  "write": true
}
```

The SKILL.md says "you have ZERO execute permission" and "you MUST NEVER call... or run Bash" — but the agent config gives it full bash/edit/write tools. The LLM sees it CAN execute, and when delegation fails (Gap 1), it falls back to direct execution. The tool config and the skill instructions are in direct contradiction.

#### Gap 3: Core upgrade has no explicit pipeline stage

The SKILL.md assigns `core_upgrade_check` and `core_upgrade_apply` to `drup-contrib` in the sub-agent roster, and mentions core upgrade in the User Confirmation Gates section ("Stage 3/4 involves a `core_upgrade_apply`"). But there is no dedicated pipeline stage (e.g., "Stage 4b: CORE UPGRADE") that tells the orchestrator:
1. WHEN to check for a core upgrade (after contrib loop? before?)
2. HOW to dispatch it (which sub-agent, what scope/target)
3. WHAT the validation gate looks like

The core upgrade is implicitly buried in the contrib loop, but the orchestrator has no clear trigger to dispatch it. This is why the orchestrator told the user to "manually modify composer.json" — it never had a clear instruction to dispatch `drup-contrib` for a core version bump.

### Approaches

1. **Fix permission block + restrict tools + add core upgrade stage** — Add `drup-*` entries to `gentle-orchestrator.permission.task`, remove `bash`/`edit`/`write` from the orchestrator's tools (keep only `read`, `task`, `question`), and add an explicit "Stage 4b: CORE UPGRADE" to the SKILL.md pipeline with clear dispatch instructions and validation gate.
   - Pros: Fixes all three root causes. Enforces the coordinator-only rule at the config level, not just the prompt level. Makes core upgrade an explicit pipeline step.
   - Cons: Requires changes in 3 places (opencode.json, SKILL.md template, spec). The tool restriction is a breaking change for the orchestrator's SDD workflow (it currently uses bash for git state checks inline).
   - Effort: Medium

2. **Fix permission block only** — Add `drup-*` to the allow list and leave everything else as-is.
   - Pros: Minimal change. Unblocks delegation immediately.
   - Cons: Doesn't fix the tool contradiction (orchestrator can still execute directly). Doesn't fix the missing core upgrade stage. Band-aid, not a fix.
   - Effort: Low

3. **Fix permission block + add core upgrade stage (leave tools as-is)** — Add `drup-*` to the allow list and add the core upgrade stage to SKILL.md, but don't restrict the orchestrator's tools.
   - Pros: Fixes delegation and the core upgrade gap. Doesn't break the SDD orchestrator's inline git state checks.
   - Cons: The tool contradiction remains — the orchestrator CAN still execute directly if it chooses to ignore the SKILL.md. Relies on prompt discipline rather than config enforcement.
   - Effort: Low-Medium

### Recommendation

**Approach 1** is the correct fix, but with a nuance: the `gentle-orchestrator` agent serves double duty as both the SDD orchestrator AND the drup orchestrator. The SDD workflow needs `bash` for `gentle-ai review` lifecycle commands and `git` state checks. Removing bash entirely would break SDD.

The recommended solution is a **split agent config**:

1. **Keep `gentle-orchestrator` as-is for SDD** (it needs bash/edit/write for review lifecycle).
2. **Add a new `drup-orchestrator` agent** in `opencode.json` with:
   - `tools: { bash: false, edit: false, write: false, read: true, task: true, question: true }` — true zero-execute config
   - `permission.task` allowing only `drup-*` agents
   - `prompt` referencing the drup SKILL.md
3. **Add the drup sub-agents to the allow list** for whichever orchestrator agent runs the drup skill.
4. **Add "Stage 4b: CORE UPGRADE"** to the SKILL.md between the contrib loop and custom loop, with explicit dispatch instructions for `drup-contrib` with `{scope: "contrib", target: "core"}` and a validation gate via `drup-validator`.

This approach enforces the coordinator-only rule at the config level (tools can't be called even if the LLM wants to), fixes delegation, and makes core upgrade an explicit pipeline step.

### Risks

- **Split agent config increases maintenance burden**: Two orchestrator agents (SDD + drup) means two sets of instructions to maintain. Mitigation: the drup orchestrator prompt can be minimal — just "load the drup skill and follow it" — since the SKILL.md already encodes all the rules.
- **OpenCode agent discovery**: The drup sub-agents are installed as markdown files in `~/.config/opencode/agents/` but are NOT registered in `opencode.json` as named agents with `mode: "subagent"`. Need to verify whether OpenCode auto-discovers agents from the `agents/` directory or requires explicit registration. If explicit registration is needed, all 6 drup sub-agents must be added to `opencode.json`.
- **Backward compatibility**: Existing users who ran `drup install` already have the old config. The fix needs to be deployable via `drup install` (re-run) or a migration step.

### Ready for Proposal

**Yes.** The root causes are clearly identified:
1. Permission block denies drup sub-agent dispatch → orchestrator falls back to direct execution or manual instructions
2. Orchestrator tools grant execution capability that contradicts the SKILL.md's zero-execute rule
3. Core upgrade has no explicit pipeline stage, so the orchestrator doesn't know when/how to dispatch it

The fix is well-scoped: split the orchestrator config (or add drup entries to the existing permission block), restrict tools for the drup orchestrator, and add an explicit core upgrade stage to the pipeline. All changes are in configuration and skill templates — no Go code changes needed.
