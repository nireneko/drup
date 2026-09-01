# Safety and recovery

A Drupal major upgrade changes code, dependencies, database state, and configuration. `drup` provides guardrails for that operational boundary; it does not replace a tested staging environment, a production change window, or review.

## Before mutation

For an agent-coordinated run, establish these facts before attempting a guarded change:

1. The target resolves to the correct canonical Drupal project root.
2. The project has a durable active run with the current action permitted.
3. An MCP session is bound to that same root.
4. A fresh project-local backup exists when the operation requires one.
5. The project’s mutation caps have not been exhausted.
6. No unresolved restore journal exists.

A rejected guard must not run the underlying mutation. Some tools with native dry-run semantics are forced into dry-run when no matching session is open; other tools are refused outright.

## Backups

Backups live at `.drup/backups/<backup-id>/`. Each backup includes:

- a compressed Drush database dump;
- a compressed project filesystem archive;
- a manifest containing the backup ID, canonical paths, database command, archive checksums, and deliberate archive exclusions.

The archive excludes drup’s own state and regenerable dependency directories. Verify the manifest and understand those exclusions before treating a backup as a complete disaster-recovery solution.

Create and inspect backups explicitly:

```bash
drup test-backup-create /absolute/path/to/project
drup test-backup-list /absolute/path/to/project
```

Backups are retained after successful runs. `drup` never deletes or restores one automatically.

## Restore and recovery

Restore requires an exact backup ID and explicit confirmation:

```bash
drup test-backup-restore /absolute/path/to/project <backup-id> --confirm
```

The restore path validates the backup, stages filesystem content, retains rescue material, records a journal, and verifies completion. Database restoration has an inherently non-atomic window. If a restore cannot complete cleanly, the journal enters recovery-required state and blocks new run-authorized mutations until an operator reconciles it through the guarded recovery path.

Do not remove a backup merely because a restore was attempted. Delete only after inspecting the project and making an explicit retention decision:

```bash
drup test-backup-delete /absolute/path/to/project <backup-id>
```

## Patch safety

Patch acquisition is bounded to Drupal.org-related HTTPS hosts, validates redirect hops, limits response size, records SHA-256 provenance, and rejects unsafe patch paths or symlink behavior. Those checks reduce risk; they do not establish that a patch is correct for your module or site. Review the patch and the Composer registration it changes.

## Commit and validation safety

A workflow checkpoint is not approved because an agent says it is complete. The durable checkpoint path binds the exact candidate paths and hash to validation evidence before committing. Use the installed validator role for scans and validation; fixer roles do not validate their own work.

## Locked mode

Use locked mode to inspect an integration without allowing MCP mutations:

```bash
drup mcp --locked
# or install the locked registration for supported agents
drup install --locked
```

Locked mode is process/configuration scoped. It is not a replacement for normal project permissions or a backup plan.
