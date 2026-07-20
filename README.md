# drup — Drupal Upgrade Automation

**CLI + MCP harness que automatiza la migración de Drupal 8/9/10 → 11 combinando análisis determinista con agentes de IA.**

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25.10+-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/platform-linux%20%7C%20macOS%20%7C%20windows-lightgrey" alt="Platform">
  <img src="https://img.shields.io/badge/tests-72%2F72-brightgreen" alt="Tests">
</p>

---

## Qué hace

Migrar un sitio Drupal a la siguiente versión major es un proceso **mecánico pero manual** que se repite proyecto tras proyecto:

1. Instalar `upgrade_status` y `drupal-rector`
2. Correr análisis de deprecaciones
3. Revisar releases de cada módulo contrib en Drupal.org
4. Buscar parches en issues para los que no tienen release compatible
5. Refactorizar código custom (módulos y temas)
6. Validar que todo compile
7. Generar reporte

`drup` automatiza todo esto. **El 80% del trabajo es determinista** (rector, releases, parches) y se resuelve sin consumir un solo token de IA. El 20% restante (código custom complejo) lo resuelve un agente de IA con herramientas de validación y reintento.

```bash
# Pipeline completo con un solo comando:
drup fix /ruta/al/proyecto-drupal

# O paso a paso desde Claude Code:
/drup /ruta/al/proyecto-drupal
```

---

## Quick Start

### Instalación

```bash
# Opción 1: Go install
go install github.com/nireneko/drup/cmd/drup@latest

# Opción 2: desde source
git clone git@github.com:nireneko/drup.git
cd drup && go build -o /usr/local/bin/drup ./cmd/drup

# Opción 3: binario compilado (descarga directa)
# Disponible en https://github.com/nireneko/drup/releases (próximamente)
```

### Configurar agentes de IA

```bash
drup install
```

Esto detecta qué agentes tenés instalados (Claude Code, OpenCode, Codex) y escribe las skills, sub-agentes y configuración MCP en sus directorios nativos.

### Actualizar

```bash
drup upgrade      # actualiza el binario
drup sync         # re-aplica skills a los agentes (tras upgrade o cambio de templates)
```

---

## Integración con agentes de IA

Al ejecutar `drup install`, el binario detecta qué agentes tenés instalados y escribe los archivos necesarios en sus directorios nativos.

### Claude Code

| Qué se instala | Ruta | Propósito |
|---|---|---|
| **Skill orquestador** | `~/.claude/skills/drup/SKILL.md` | Pipeline de 7 fases. Se invoca con `/drup <ruta>` |
| **Sub-agentes** | `~/.claude/agents/drup-preflight.md` | Preflight: detecta entorno, instala dependencias |
| | `~/.claude/agents/drup-contrib.md` | Módulos contrib: releases, parches, commits |
| | `~/.claude/agents/drup-custom.md` | Código custom: refactor con reintento y escalado |
| | `~/.claude/agents/drup-theme.md` | Temas: deprecaciones twig/.theme |
| **MCP server** | `~/.claude/.mcp.json` | Registra `drup mcp` como servidor MCP con 7 tools |

**Uso**: abrí Claude Code en el proyecto Drupal y ejecutá:

```
/drup /ruta/al/proyecto
```

Claude Code carga el SKILL.md, se conecta al MCP server, y ejecuta las 7 fases del pipeline. Los sub-agentes aíslan el trabajo por módulo/archivo para no saturar el contexto.

**Modelo por defecto**: el skill usa el modelo activo de la sesión. Para forzar un modelo específico, configuralo en `~/.config/drup/config.yaml` (ver [Configuración](#configuración)).

### OpenCode

| Qué se instala | Ruta |
|---|---|
| **Skill orquestador** | `~/.config/opencode/skills/drup/SKILL.md` |
| **Sub-agentes** | `~/.config/opencode/agents/drup-*.md` |
| **MCP server** | `~/.config/opencode/mcp.json` |

**Uso**: en OpenCode, ejecutá `/drup <ruta>` o dejá que el skill se cargue automáticamente cuando menciones "Drupal upgrade" o "migrar Drupal".

### Codex

| Qué se instala | Ruta |
|---|---|
| **Skill orquestador** | `~/.codex/skills/drup/SKILL.md` |
| **Sub-agentes** | `~/.codex/agents/drup-*.md` |
| **MCP server** | `~/.codex/mcp.json` |

**Uso**: en Codex CLI, ejecutá `/drup <ruta>`.

### El MCP server

Los 3 agentes comparten la misma configuración MCP. El archivo `.mcp.json` (o `mcp.json`) registra el servidor:

```json
{
  "mcpServers": {
    "drup": {
      "command": "/ruta/al/binario/drup",
      "args": ["mcp"]
    }
  }
}
```

El servidor MCP se comunica por **stdio** (JSON-RPC 2.0). No necesita puerto, no necesita network — el agente lanza el proceso `drup mcp` y se comunica por stdin/stdout. Las 7 tools expuestas están documentadas en [MCP Tools](#mcp-tools).

### Verificar la instalación

```bash
# Ver qué agentes detectó drup
drup install
# Output: Installing drup to claude... done
#         Installing drup to opencode... done

# Forzar re-sincronización (tras update del binario)
drup sync
```

---

## Workflow completo: de Drupal 10 a Drupal 11

### Paso a paso con el CLI

```bash
# 1. Preflight: detecta versión, instala dependencias
drup preflight /ruta/proyecto

# 2. Scan: análisis inicial de deprecaciones
drup scan /ruta/proyecto

# 3. Fix: pipeline completo
drup fix /ruta/proyecto
#    ├── corre drupal-rector (autofix ~80%)
#    ├── por cada módulo contrib: busca release D11 o parche RTBC → aplica → commit
#    ├── por cada archivo custom: muestra errores para que el agente los resuelva
#    └── validación final → reporte

# 4. Reporte: resumen de todo lo hecho
drup report /ruta/proyecto
```

### Desde Claude Code (orquestado por skill)

```
/drup /ruta/proyecto
```

El skill `/drup` ejecuta el pipeline completo en 7 fases con **validation gates**: cada fase se valida antes de avanzar. Si algo falla, reintenta con modelo más potente. Si sigue fallando, va a la lista de pendientes para revisión humana.

### Comandos individuales

```bash
drup contrib check webform       # ¿tiene release compatible con D11?
drup issue patches 3412345       # parches de una issue de Drupal.org (JSON limpio)
drup mcp                         # servidor MCP (para agentes de IA)
```

---

## El Pipeline (7 fases)

```
[0. Preflight]      [1. Estático]         [2. Resolución]           [3. Self-healing]      [4. Salida]
git limpio          composer require      contrib:                  re-analyze + phpstan   rama + commits
drush status    →   upgrade_status    →   · release D11?        →   · ok → siguiente  →   reporte final
detectar core       drupal-rector         · parche issue?           · falla → reintento    lista p/ humano
versión             (autofix ~80%)        custom: agente edita      · ×2 → escala modelo   (PR opcional)
```

### Fase 0 — Preflight
Verifica git limpio, composer/drush disponibles, versión de core. Instala dependencias faltantes (`upgrade_status`, `drupal-rector`, `phpstan-drupal`).

### Fase 1 — Rector (0 tokens)
Ejecuta `drupal-rector` con los sets de reglas de D11 sobre módulos y temas custom. Resuelve ~80% de deprecaciones estándar de forma determinista. Commit atómico.

### Fase 2 — Módulos Contrib
Para cada módulo contrib con errores:
1. `contrib_check` → consulta `updates.drupal.org/release-history` (feed canónico del módulo Update de Drupal core)
2. ¿Release compatible con D11? → `composer require` → commit
3. ¿Sin release? → busca issues en Drupal.org (api-d7 + scraper HTML) → prioriza parches RTBC → descarga y aplica
4. ¿Sin parches? → el agente genera un `.patch` con la corrección
5. **Gate de validación**: `validate(scope=contrib, module=X)` → 0 errores = commit, >0 = reintentar

### Fase 3 — Código Custom
Para cada archivo custom con deprecaciones:
1. Agente lee el archivo + mensaje de error (±30 líneas)
2. Aplica la corrección mínima
3. `validate(scope=custom, file=Y)` → ¿0 errores? → commit
4. ¿Falla? → reintenta con feedback del validador (×2)
5. ¿Sigue fallando? → escala a modelo más potente (×1)
6. ¿Sigue fallando? → lista de pendientes para revisión humana

### Fase 4 — Validación Final
`validate(global)` → ¿`total_errors == 0`? → reporte final. Quedan errores → itera con el sub-agente correcto.

---

## Validation Gates (reglas estrictas)

El orquestador NUNCA confía en la auto-declaración de un sub-agente:

| Regla | Descripción |
|---|---|
| **Validación externa** | El orquestador ejecuta `validate` — el sub-agente nunca valida su propio trabajo |
| **Sin auto-aprobación** | Un sub-agente diciendo "listo" no significa nada. Solo `validate` == 0 cuenta |
| **Reintento con evidencia** | Si falla, el mismo sub-agente recibe el output del validador como feedback |
| **Máximo 2 reintentos** | Por scope. Luego escala modelo (haiku → sonnet). Luego lista humana |
| **Gate de fase** | Ninguna fase avanza hasta que TODOS los ítems pasan validación |
| **Commit solo post-gate** | Cada commit se ejecuta ÚNICAMENTE después de `validate` == 0 |

---

## Arquitectura

### El binario (`drup`)

```
cmd/drup/main.go              # entrypoint
internal/
  app/          # CLI dispatch (11 comandos) + MCP tool handlers
  exec/         # runner de subprocesos (composer, drush, rector, phpstan, git)
  scan/         # parser de upgrade_status:analyze JSON
  drupalorg/    # release-history XML + api-d7 + scraper de issues
  patch/        # descarga de .patch, git apply, registro en composer.json
  gitops/       # git clean check, commits atómicos, ramas
  report/       # generación de reportes JSON + Markdown
  mcp/          # servidor MCP (JSON-RPC 2.0, stdio)
  packaging/    # templates de skills/agentes/MCP (go:embed)
  installer/    # detección de agentes, escritura de assets, backup
  state/        # state.json con agentes instalados, pending_sync, modelos
  update/       # self-upgrade con checksum + reemplazo atómico
```

### El orquestador (skills de agente)

El binario solo hace trabajo determinista. El flujo completo lo ejecuta un **agente de IA** (Claude Code, OpenCode, Codex) siguiendo las instrucciones de un `SKILL.md`:

- **Skill `/drup`**: pipeline de 7 fases con validation gates
- **Sub-agentes**: `drup-preflight`, `drup-contrib`, `drup-custom`, `drup-theme` — aíslan contexto por módulo/archivo para no saturar la ventana del orquestador

### El puente (MCP)

El servidor MCP de `drup` expone 7 tools con tipos y esquemas JSON. Es el protocolo estándar que conecta el binario con cualquier agente compatible:

```
Claude Code ───┐
OpenCode ──────┼── MCP (stdio) ── drup mcp ── tools deterministas
Codex ─────────┘
```

---

## MCP Tools

| Tool | Input | Output |
|---|---|---|
| `scan` | `project_path` | JSON: errores clasificados (contrib/custom/theme/core) |
| `autofix` | `project_path` | JSON: resumen de rector + errores restantes |
| `contrib_check` | `module_machine_name` | `{ has_d11_release, latest_version, compatible_branches }` |
| `issue_patches` | `issue_nid` o `module_name` | `[{ url, status (RTBC/NR), date, is_patch }]` |
| `apply_patch` | `patch_url, project_path` | `{ applied, commit_hash, error }` |
| `validate` | `project_path` | `{ total_errors, errors[] }` |
| `create_patch` | `module_name, deprecation_details` | `{ patch_path, applied }` |

---

## Configuración

`~/.config/drup/config.yaml` (opcional):

```yaml
agents:
  claude-code:
    skills:
      drup:
        model: claude-sonnet-4
    agents:
      drup-contrib:
        model: claude-haiku-3-5
      drup-custom:
        model: claude-haiku-3-5
  opencode:
    profiles:
      drup-orchestrator:
        default: openrouter/qwen/qwen3-30b-a3b:free
```

Si no configurás nada, `drup` usa defaults sensatos (barato para mecánico, fuerte para razonamiento).

---

## Comandos

| Comando | Descripción |
|---|---|
| `drup init` | Genera `drup.yaml` en el directorio actual |
| `drup scan <path>` | Análisis inicial de deprecaciones (JSON) |
| `drup fix <path>` | Pipeline completo: preflight + rector + contrib + custom + validación |
| `drup preflight <path>` | Detecta versión, verifica git/composer/drush, instala dependencias |
| `drup contrib check <module>` | ¿Release D11 o parche disponible? |
| `drup issue patches <nid>` | Parches de una issue de Drupal.org |
| `drup report <path>` | Reporte del estado actual vs D11 |
| `drup mcp` | Servidor MCP por stdio (para agentes de IA) |
| `drup install` | Detecta agentes y escribe skills + MCP config |
| `drup sync` | Re-aplica skills a agentes instalados |
| `drup upgrade` | Actualiza el binario + sincroniza skills |
| `drup version` | Versión actual |

Flags globales: `--json`, `--force` (git sucio), `--dry-run`.

---

## Roadmap

| Versión | Alcance |
|---|---|
| **v0.1** ✅ | Binario Go: preflight + scan + fix + contrib + report. 72 tests. |
| v0.2 | Pipeline completo con skills de agente. Sub-agentes con isolation. Self-upgrade funcional. |
| v0.3 | Modo standalone con LLM (sin agente externo). RAG de change records de Drupal. |
| v0.4 | Encadenado 8→9→10→11. Creación de PR. Modo CI. |

---

## Desarrollo

```bash
git clone git@github.com:nireneko/drup.git
cd drup

go build ./cmd/drup     # compilar
go test ./...           # 72 tests
go vet ./...            # análisis estático
```

Estructura de tests: table-driven, fixtures en `testdata/`, variables a nivel paquete para mockear subprocesos.

---

## Licencia

MIT
