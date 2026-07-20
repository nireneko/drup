# Proposal: drup-mvp — Drupal Upgrade Automation System

## Intent

Drupal 8/9/10 → 11 migration is ~80% mechanical work done manually today. `drup` is a complete automation system: a Go binary (CLI + MCP server) that orchestrates deterministic analysis, combined with agent skill files that teach AI agents (Claude Code, OpenCode, Codex) how to run the full upgrade pipeline with sub-agent isolation, atomic commits, and self-healing validation. The binary handles deterministic work; agents handle reasoning; together they eliminate manual migration labor.

## Scope

### In Scope
- **Go binary** (`drup`): CLI with 6 commands + MCP server exposing 7 tools
- **Internal packages**: app, exec, scan, drupalorg, patch, gitops, report, mcp, state, packaging, installer, update
- **Orchestrator skill**: SKILL.md encoding the complete 7-stage pipeline for AI agents
- **4 sub-agents**: drup-preflight, drup-contrib, drup-custom, drup-theme with model routing
- **Agent templates**: skill + sub-agent definitions for Claude Code, OpenCode, Codex
- **Commit discipline**: atomic commits per unit of work via work-unit-commits skill
- **Install/sync system**: detect agents, write skills + MCP config, state.json tracking
- **Self-update**: binary upgrade with deferred sync pattern

### Out of Scope
- Chained 8→9→10→11 migration (v0.4)
- RAG of Drupal change records (v0.4)
- PR creation via `gh` (v0.4)
- CI mode (v0.4)

## Capabilities

### New Capabilities

- `cli-binary`: Go CLI with 6 commands (init, scan, fix, contrib, issue, report), manual dispatch via os.Args, stdlib only
- `mcp-server`: stdio MCP server wrapping all internal packages as 7 tools (scan, autofix, contrib_check, issue_patches, apply_patch, validate, create_patch)
- `preflight`: git clean check, composer/drush detection, core version detection from composer.lock, dev dependency installation (upgrade_status, drupal-rector, phpstan-drupal)
- `scan`: parse `upgrade_status:analyze` JSON into classified error model (contrib/custom/theme)
- `contrib-check`: Drupal.org release-history XML client for D11 compatibility lookup
- `issue-patches`: Drupal.org api-d7 + issue scraper for patch/diff/MR extraction with RTBC prioritization
- `apply-patch`: patch download, git apply, composer-patches registration in composer.json
- `gitops`: git clean verification, atomic commits, branch management (upgrade/drupal-11)
- `report`: final JSON + markdown with resolved/pending/token accounting
- `orchestrator-skill`: SKILL.md encoding the 7-stage pipeline (preflight → dep check → rector → contrib loop → custom loop → final validation → report)
- `sub-agents`: 4 specialized agents (preflight, contrib, custom, theme) with model routing (haiku → sonnet escalation)
- `agent-packaging`: templates for Claude Code, OpenCode, Codex — skills, sub-agent defs, MCP config per platform
- `installer`: detect installed agents, write assets to native directories, backup configs, state.json management
- `self-update`: GitHub Releases check, checksum verification, binary replacement, deferred sync pattern

### Modified Capabilities
None — greenfield project.

## Approach

**Architecture**: Go binary (CLI + MCP) + agent skill files. The binary is 100% deterministic — no LLM calls. Agents do the reasoning (custom code fixes, patch creation) using MCP tools for validation. Sub-agents isolate context per module/file to prevent window pollution.

**Go module**: `drup` (local), Go 1.25.10, stdlib only for MVP (os/exec, encoding/json, encoding/xml, net/http, flag, os, path/filepath). No cobra — manual dispatch. MCP SDK added in v0.2.

**Pipeline (orchestrator skill)**:
1. PREFLIGHT: detect Drupal version, check git clean, verify composer/drush
2. DEP CHECK: install upgrade_status, drupal-rector, phpstan-drupal if missing → `validate(scope=env)` gate
3. RECTOR: run drupal-rector on custom modules + themes → commit → `validate(scope=rector)` gate
4. CONTRIB LOOP: for each contrib module:
   a. Check D11 release → has it? → `composer require` → commit → `validate(scope=contrib,module=X)` gate
   b. No release? → search issues for RTBC patch → apply or create → commit → gate
   c. Gate fail? → re-enter loop (×2) → escalate model → gate → pass or human list
5. CUSTOM LOOP: for each custom file:
   a. Agent reads file + error → applies fix → `validate(scope=custom,file=Y)` gate
   b. Gate fail? → re-enter loop (×2) → escalate model → gate → pass or human list
6. FINAL VALIDATION: `validate(global)` → total_errors == 0? → done. Errors? → iterate remaining errors with correct sub-agent
7. REPORT: markdown + JSON of everything done

**Validation gates (HARD enforcement by orchestrator)**:

Cada sub-agente opera sobre un scope concreto (preflight → entorno, contrib → módulos contrib, custom → archivos custom, theme → temas). **El orquestador NO avanza al siguiente paso sin validación explícita e independiente del scope del sub-agente que acaba de terminar.**

Reglas de gate:

1. **Validación externa**: cuando un sub-agente termina, el orquestador —no el sub-agente— ejecuta `validate` sobre el scope correspondiente. Si el sub-agente reportó "listo" pero `validate` devuelve errores en su scope → **el sub-agente mintió o falló silenciosamente**.
2. **Sin auto-aprobación**: un sub-agente NUNCA valida su propio trabajo. `drup-contrib` no puede decir "parche aplicado, siguiente" — el orquestador llama a `validate` y solo si `errors[scope=contrib] == 0` procede.
3. **Reintento con evidencia**: si `validate` falla, el orquestador re-lanza EL MISMO sub-agente con el output del validador como feedback. Máximo 2 reintentos. Si sigue fallando → el orquestador escala el modelo (haiku → sonnet) y reintenta 1 vez más.
4. **Gate blocking**: ningún módulo/archivo del mismo scope avanza mientras haya un error pendiente en ese scope. El pipeline es secuencial dentro de cada scope.
5. **Gate de fase**: al terminar todos los sub-agentes de una fase (ej: todos los módulos contrib), el orquestador ejecuta `validate` global. Si `total_errors > 0` → no se pasa a la siguiente fase. Se itera sobre los errores restantes con el sub-agente correspondiente.

Flujo con gates:

```
SUBEJECUCIÓN                   GATE (orquestador)
─────────────                  ───────────────────
drup-preflight termina    →    validate(scope=preflight) → ¿0 errores?
                                  ├─ sí → siguiente fase
                                  └─ no → re-lanzar preflight con errores
drup-contrib módulo X     →    validate(scope=contrib, module=X) → ¿0?
                                  ├─ sí → commit + siguiente módulo
                                  └─ no → reintentar (×2) → escalar modelo → ¿0?
                                            ├─ sí → commit
                                            └─ no → lista humana
drup-custom archivo Y     →    validate(scope=custom, file=Y) → ¿0?
                                  └─ mismo ciclo que contrib
VALIDACIÓN GLOBAL         →    validate(global) → ¿total_errors == 0?
                                  ├─ sí → reporte final
                                  └─ no → iterar errores restantes
```

**Sub-agent model routing**: haiku/cheap for mechanical work (preflight, contrib, theme), haiku → sonnet escalation for custom code (2 retries on cheap model, then escalate). El escalado de modelo es orquestado por el gate, no por el sub-agente.

**Commit discipline**: ONE atomic commit per unit (rector run, each contrib module, each custom file). El commit SOLO se ejecuta después de que el gate de ese scope pasa (`validate` → 0 errores). Conventional commits format. Branch: `upgrade/drupal-11`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/drup/main.go` | New | CLI entrypoint, calls app.Run() |
| `internal/app/` | New | CLI dispatch (switch on args[0]), no cobra |
| `internal/exec/` | New | Subprocess runner (composer/drush/rector/phpstan/git) with output capture |
| `internal/scan/` | New | Parse upgrade_status:analyze JSON → internal model |
| `internal/drupalorg/` | New | release-history XML + api-d7 + issue scraper → JSON |
| `internal/patch/` | New | Download .patch, git apply, composer-patches integration |
| `internal/gitops/` | New | Git clean check, atomic commits, branch management |
| `internal/report/` | New | Markdown + JSON report generation |
| `internal/mcp/` | New | MCP server (stdio) wrapping above packages as 7 tools |
| `internal/state/` | New | state.json for install tracking |
| `internal/packaging/` | New | Templates for agent skills/sub-agents per platform |
| `internal/installer/` | New | Detect agents, write skills/MCP config to agent dirs |
| `internal/update/` | New | Self-update binary + deferred sync pattern |

## MCP Tools

| Tool | Input | Output | Used by |
|------|-------|--------|---------|
| scan | project_path | JSON: errors by type (contrib/custom/theme) | orchestrator, preflight |
| autofix | project_path | JSON: rector summary + remaining errors | orchestrator |
| contrib_check | module_machine_name | JSON: {has_d11_release, latest_version, compatible_branches} | drup-contrib |
| issue_patches | issue_nid or module_name | JSON: [{url, status, date, is_patch}] | drup-contrib |
| apply_patch | patch_url, project_path | JSON: {applied, commit_hash, error} | drup-contrib |
| validate | project_path | JSON: {total_errors, errors[]} | all sub-agents |
| create_patch | module_name, deprecation_details | JSON: {patch_path, applied} | drup-contrib |

## Sub-Agents

| Agent | Model | MCP Tools | Responsibility |
|-------|-------|-----------|----------------|
| drup-preflight | haiku/cheap | scan, validate | Detect version, check git/composer/drush, install dev deps |
| drup-contrib | haiku/cheap | contrib_check, issue_patches, apply_patch, validate | Check D11 releases, search/apply/create patches, commit per module |
| drup-custom | haiku → sonnet | validate, scan | Fix custom module deprecations, retry with escalation |
| drup-theme | haiku | validate, scan | Fix theme file deprecations (twig, .theme) |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| upgrade_status JSON format changes | Med | Parse defensively, fixture tests pin known versions |
| drupal.org issue markup changes | Med | api-d7 first, scraper as fallback, both with fixtures |
| Rector fails on some deprecations | Med | Falls to drup-custom sub-agent or pending human list |
| Agent creates broken patch | Med | validate tool catches it; max 2 retries, then escalate |
| Context window pollution across many modules | Med | Sub-agents isolate context per module; orchestrator stays lean |
| composer-patches format edge cases | Low | Use cweagans docs + test with real Drupal projects |

## Rollback Plan

**Binary rollback**: replace binary with previous version. No persistent state beyond state.json (delete to reset). **Project rollback**: each atomic commit is revertable via `git revert <hash>`. Full rollback: `git checkout main && git branch -D upgrade/drupal-11`. **Agent config rollback**: installer backs up previous configs (tar.gz, 5 latest). Restore from backup or re-run `drup install`.

## Dependencies

- Go 1.25.10 (build machine)
- `git`, `composer`, `drush` on user's PATH (Drupal project prerequisites)
- Target project must have `drupal/upgrade_status`, `palantirnet/drupal-rector`, `mglaman/phpstan-drupal` installable
- MCP SDK (`modelcontextprotocol/go-sdk` or `mark3labs/mcp-go`) for v0.2 MCP server
- GitHub Releases for self-update (v0.2)

## Success Criteria

- [ ] `drup scan /path` produces structured JSON identical to fixture test data
- [ ] `drup fix /path` on a Drupal 10 fixture → upgrade_status:analyze returns 0 errors for known cases
- [ ] `drup mcp` serves 7 tools with correct JSON schemas, testable via MCP Inspector
- [ ] Orchestrator skill loaded in Claude Code runs full pipeline without human intervention for standard cases
- [ ] Each sub-agent commits atomically, one unit per commit
- [ ] Pending human list is accurate and actionable when something can't be resolved
- [ ] All parsers have fixture-based unit tests (upgrade_status JSON, release-history XML, issue HTML)
- [ ] `go test ./...` and `go vet ./...` pass clean
- [ ] Binary builds with `go build ./cmd/drup` and runs on linux/darwin
- [ ] `drup install` detects agents and writes correct skills/MCP config to native directories
- [ ] `drup upgrade` replaces binary and triggers deferred sync

## Implementation Phases

| Version | What |
|---------|------|
| **v0.1** | Go binary: cmd/drup, internal/{app,exec,scan,drupalorg,patch,gitops,report}. CLI commands: init, scan, fix, contrib, issue, report. 100% deterministic, no LLM. |
| **v0.2** | MCP server (internal/mcp) with all 7 tools. Orchestrator skills + sub-agent templates for Claude Code, OpenCode, Codex. install/sync/upgrade commands. packaging/, installer/, state/ packages. |
| **v0.3** | LLM self-healing loop (internal/llm, internal/heal). Standalone mode `drup fix --auto` for CI without agent. RAG of Drupal change records. |
| **v0.4** | Chained 8→11 migration. PR creation via `gh`. CI mode. update/ package hardening. |
