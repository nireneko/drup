# Per-Agent Model Configuration

`drup` lets you override which model each sub-agent runs on, per platform, without editing the installed templates by hand. Overrides live in `~/.config/drup/state.json`, under `model_assignments`, and are applied the next time you run `drup install` or `drup sync`.

## Shape

```json
{
  "model_assignments": {
    "<platform>": {
      "<agent>": {
        "default": "<model-id>",
        "escalation": "<model-id>"
      }
    }
  }
}
```

- `<platform>` is one of `claude`, `opencode`, `codex`.
- `<agent>` is one of `drup-preflight`, `drup-rector`, `drup-contrib`, `drup-custom`, `drup-theme`, `drup-validator`.
- `default` is the model dispatched on the first attempt.
- `escalation` is the model the orchestrator switches to after 2 failed attempts, for one final retry, before adding the item to the pending-human list (see each agent's "Model Routing" section).
- Both fields are optional. An unset field falls back to the built-in default for that platform/agent — you only need to name the field(s) you want to change.
- Any non-empty string is accepted as a model identifier — there is no allowlist, since model catalogs change faster than this tool would. The one restriction is structural: a value must not contain a newline, `"`, `\`, `#`, or leading/trailing whitespace, because it is substituted directly into generated YAML/TOML frontmatter.

## Per-platform naming examples

| Platform | Example model identifier |
|---|---|
| `claude` | `claude-haiku-4-5-20251001`, `claude-sonnet-5` |
| `opencode` | `openrouter/qwen/qwen3-30b-a3b:free` |
| `codex` | `gpt-4o-mini`, `gpt-4o` |

Use whatever identifier your platform/provider expects — `drup` does not validate it against a catalog.

## Example: move one agent to a stronger model

```json
{
  "model_assignments": {
    "claude": {
      "drup-rector": {
        "default": "claude-opus-4",
        "escalation": "claude-opus-4"
      }
    }
  }
}
```

After the next `drup sync`, `drup-rector`'s frontmatter, its "Default model:" prose, and the `claude/SKILL.md` roster row all reflect `claude-opus-4` — they are resolved from the same configured value, so they can never disagree with each other.

## Editing instructions

1. Run `drup install` at least once, so `~/.config/drup/state.json` exists.
2. Open `~/.config/drup/state.json` and add or edit the `model_assignments` key (create it if absent).
3. Run `drup sync` to re-render every installed agent with the new configuration. Only files whose content actually changed are rewritten.
4. Restart your agent (Claude Code, OpenCode, or Codex) so it picks up the updated agent definitions.

`drup-validator` always resolves to the strong tier by default (never the cheap default used by the fixer agents) — it is the gate every pipeline decision rests on. You can still override it explicitly if you want a different strong-tier model.

## Backward compatibility

- If `model_assignments` is absent or empty, `drup` renders exactly the same output as before this feature existed — no config means no behavior change.
- A partial config only overrides the platform/agent pairs it names; everything else keeps using the built-in defaults.
- An older `state.json` that still has the retired `model_overrides` key is read without error: `drup` prints a one-time warning and ignores it. There is no automatic migration — copy the values you want into `model_assignments` manually, using the shape above.
- An invalid platform or agent key (a typo, for example) makes `drup install`/`drup sync` skip that platform with a warning; other detected platforms still install normally, matching the existing per-agent failure isolation.

## Downgrade caveat

If you ever install an older `drup` build after configuring `model_assignments`, that build's own `Save()` call may rewrite `state.json` without the key, discarding your configuration. Keep a copy of your `model_assignments` block if you expect to downgrade, and reconfigure it after upgrading again.
