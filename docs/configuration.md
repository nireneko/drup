# Configuration

`drup` has two distinct configuration scopes: global installer state and optional project guardrails. They should not be confused.

## Global state

The installer stores state at:

```text
~/.config/drup/state.json
```

It records installed agents, pending synchronization state, and optional per-platform model assignments. An older `model_overrides` key is ignored with a warning; use `model_assignments` instead.

```json
{
  "model_assignments": {
    "codex": {
      "drup-validator": {
        "default": "gpt-4o",
        "escalation": "gpt-4o"
      }
    }
  }
}
```

After editing model assignments, run `drup sync` and restart the agent. Valid platforms are `claude`, `opencode`, and `codex`; valid roles are the installed `drup-*` specialist roles. See [Model configuration](model-configuration.md) for complete rules and examples.

The self-update backup location is separate and historical:

```text
~/.drup/backups/
```

Do not mistake it for project backup state.

## Project configuration

Optional project configuration lives at:

```text
<project>/.drup/config.json
```

All fields have safe defaults. Missing, empty, malformed, or invalid individual values fall back to defaults rather than blocking a tool call.

```json
{
  "mutation_cap_per_session": 50,
  "mutation_cap_per_day": 200,
  "backup_freshness_window": "24h",
  "allowlist_mode": "strict",
  "checkpoint_smoke_commands": [
    ["vendor/bin/phpunit"],
    ["drush", "status"]
  ]
}
```

| Field | Default | Meaning |
|---|---:|---|
| `mutation_cap_per_session` | `50` | Maximum guarded mutations while an MCP session is open. |
| `mutation_cap_per_day` | `200` | Maximum guarded mutations for the fallback daily budget. |
| `backup_freshness_window` | `24h` | How long a backup manifest satisfies the freshness guard. Use a Go duration string. |
| `allowlist_mode` | `strict` | Currently only strict behavior is implemented: HTTPS Drupal.org-related patch hosts. Other values do not loosen the policy. |
| `checkpoint_smoke_commands` | none | Optional argv vectors allowed only for `vendor/bin/phpunit`/`phpunit`, `composer test`, `drush status`, or `drush core:status`. Shell syntax is rejected. |

Do not put credentials, database URLs, or arbitrary shell commands in this file. The smoke-command allowlist deliberately accepts argv vectors, not a shell string.

## Agent installation paths

| Host | Global configuration modified by installer |
|---|---|
| Claude Code | `~/.claude.json` |
| OpenCode | `~/.config/opencode/opencode.json` |
| Codex | `~/.codex/config.toml` |

`drup install`, `sync`, and `uninstall` only manage the drup-owned registrations and generated assets; they preserve other MCP servers when parsing succeeds. Run them with the host application closed, then restart it.
