# CLI reference

Run `drup help` for the authoritative surface of the installed binary. Paths below are project paths unless stated otherwise.

## Discovery and analysis

| Command | Purpose | Writes project files? |
|---|---|---:|
| `drup version` | Print binary version. | No |
| `drup help` | Print usage. | No |
| `drup scan <path>` | Run Upgrade Status analysis and print structured JSON. | No |
| `drup validate <path> [module]` | Re-scan and return an error status when findings remain. | No |
| `drup contrib <module>` | Query Drupal.org release compatibility information. | No local write |
| `drup issue <module_or_nid>` | Extract patch, diff, and merge-request links from Drupal.org issues. | No local write |
| `drup preflight [path] [--target-major=N] [--allow-dirty]` | Check project readiness and install missing analysis dependencies. | **Yes** |
| `drup report <path>` | Scan and write `drup-report.json` and `drup-report.md`. | **Yes** |

`preflight` defaults to the current directory and target major 11. It rejects unknown options. `--allow-dirty` changes the Git cleanliness gate; it does not make a later checkpoint commit safe.

## Scoped remediation and upgrade commands

| Command | Purpose | Important boundary |
|---|---|---|
| `drup fix <path>` | Run `vendor/bin/rector process` on custom modules and themes, then run `scan`. | Not an end-to-end pipeline; it does not update contrib or core. |
| `drup compat-fix <path> [--dry-run] [--target=VERSION]` | Widen existing custom extension core requirements when safe. | Never edits contributed extensions; missing declarations are reported rather than invented. |
| `drup contrib-patch <path> <module>` | Prepare/apply a compat patch and register it for a contrib module. | Requires workflow guards when invoked through MCP. |
| `drup allow-lenient <path> <package>...` | Allow a patched contrib package to install against the target core. | Use only with understood patch evidence. |
| `drup apply-patch <url> <path>` | Download and apply a Drupal.org allowlisted patch. | Redirects, size, hash, and patch safety are bounded. |
| `drup upgrade-core <version> [--dry-run] [--allow-dirty]` | Plan or perform one immediate core upgrade step. | A real upgrade is guarded and must not skip a major. |
| `drup cleanup <path> [--validate-passed\|--validate-failed]` | Remove Upgrade Status after validation. | Do not run before the evidence you need is retained. |
| `drup checkpoint-commit ...` | Publish an evidence-bound checkpoint diff. | Intended for the durable MCP workflow, not ad hoc shell use. |

## Backup commands

| Command | Purpose |
|---|---|
| `drup test-backup-create <path>` | Create a local database/files backup. |
| `drup test-backup-list <path>` | List backup manifests. |
| `drup test-backup-restore <path> <backup-id> --confirm` | Restore an existing backup with explicit confirmation. |
| `drup test-backup-delete <path> <backup-id>` | Delete a backup after the operator has decided it is no longer needed. |

The `test-` prefix is historical. These commands are real local recovery operations; treat restore and deletion as operational actions. See [Safety and recovery](safety-and-recovery.md).

## MCP and installer commands

| Command | Purpose |
|---|---|
| `drup mcp [--locked]` | Start the stdio JSON-RPC MCP server. `--locked` disables mutating tools for that process. |
| `drup install [--locked]` | Detect supported agents and install rendered skills, roles, commands, and MCP registration. |
| `drup sync [--locked]` | Reapply the agent assets and registration. |
| `drup uninstall [--dry-run\|--force]` | Remove drup-managed assets and MCP registration. |
| `drup init` | Initialize a Drupal project for upgrade automation. |
| `drup upgrade` | Self-update the binary. |

MCP schemas, effects, and guards are generated from `ToolSpec`; use [`mcp-tools.md`](mcp-tools.md) or the server’s `tools/list` response for agent-facing details.
