# Drupal Upgrade Automation — Agent Retrospective

**Proyecto**: `/home/borja/sites/drupal/upgrade-test`  
**Fecha**: 2026-07-23  
**Modelo**: deepseek-v4-flash-free  
**Rol**: Orchestrator (7-stage pipeline) + manual execution  

---

## 1. Resumen

La prueba consistió en ejecutar el pipeline completo de upgrade a Drupal 11 usando agentes autónomos (MCP + CLI tools) sobre un proyecto Drupal 10.6 con webform 6.2.0 como único módulo contrib. Se completó exitosamente la actualización a Drupal 11.4.4 y webform 6.3.0.

---

## 2. Qué funcionó bien

### 2.1 Pipeline stages
- **Preflight**: `drup preflight` detectó correctamente la versión, dependencias, y estado del working tree.
- **Rector / Custom loops**: correctamente identificó que no había módulos custom, evitando trabajo innecesario (YAGNI).
- **Composer updates**: `ddev composer require` funcionó sin problemas para webform 6.3.0 y el core upgrade.
- **DB updates**: `drush updb -y` ejecutó 12 hook_updates de webform sin errores.
- **Core upgrade**: `drup upgrade-core 11.4` actualizó el constraint, creó backup del composer.json, y verificó la versión resultante (11.4.4).

### 2.2 Git workflow
- Los commits en conventional commits format quedaron limpios y separados por responsabilidad:
  - `fix(contrib): update webform to 6.3.0`
  - `fix(core): upgrade Drupal core to ^11.0 (11.4.4)`
  - `docs(report): update upgrade report with D11.4 upgrade details`

### 2.3 MCP Engram
- `mem_save` persistió decisiones clave entre turnos sin intervención manual.
- `mem_context` recuperó estado de sesiones anteriores.

---

## 3. Qué falló / Problemas encontrados

### 3.1 CRÍTICO — `drup scan` / `drup validate` no funcionan
**Síntoma**: ambos comandos ejecutan `drush -r <path> upgrade_status:analyze --all`. El comando drush sale con exit code 3 cuando hay hallazgos (que es el caso normal). `drup` interpreta exit code != 0 como fallo, abortando el pipeline.

**Causa raíz**: la herramienta `drup` trata exit code 3 de drush como error, pero `upgrade_status:analyze` devuelve exit code 3 para reportar hallazgos — es su comportamiento por diseño.

**Impacto**: TODO el pipeline basado en `drup scan`/`drup validate` es intransitable. Tuve que bypassear todo con `drush upgrade_status:analyze --all --format=codeclimate` directamente.

**Solución aplicada**: ejecución directa de drush vía `ddev exec` con `--format=codeclimate` para obtener JSON parseable. Procesamiento con Python inline para extraer métricas.

### 3.2 PHP 8.4 deprecation warnings bloquean drush
**Síntoma**: `drush` fallaba con exit code 3 porque PHP 8.4 emitía deprecation notices por "implicitly marking parameter as nullable" en cada función de webform que usaba `?Type $param = null`. El stderr se llenaba de cientos de líneas y drush abortaba.

**Causa raíz**: DrupalKernel::__construct() hace `error_reporting(E_ALL)` sobrescribiendo cualquier configuración de php.ini.

**Solución aplicada**: agregué `error_reporting(E_ALL & ~E_DEPRECATED & ~E_USER_DEPRECATED)` en `settings.php` DESPUÉS del include de DDEV. Esto corre después de que DrupalKernel setea E_ALL.

**Lección**: Cualquier proyecto en PHP 8.4 con módulos que usan parámetros nullable implícitos va a tener este problema. Debería ser parte del preflight detectarlo y solucionarlo.

### 3.3 `drup contrib` devuelve falsos negativos
**Síntoma**: `drup contrib webform` retornó `has_d11_release: false` y `compatible_branches: []`, a pesar de que webform 6.3.0 oficialmente soporta D11 (`^10.3 || ^11.0`).

**Causa probable**: el chequeo probablemente busca `^11` en el release, pero webform 6.3.0 tiene `^10.3 || ^11.0`, y el parser no interpreta correctamente rangos compuestos.

**Solución aplicada**: verificación manual via `composer show drupal/webform` + websearch. Luego upgrade manual con `composer require`.

### 3.4 MCP tools sin parámetros expuestos
**Síntoma**: Varios MCP tools como `drup_scan`, `drup_contrib_check`, `drup_issue_patches`, `drup_apply_patch` se anuncian sin parámetros en su schema, pero requieren parámetros implícitos (module name, path, URL). Esto causó errores  -32603 (invalid params) al invocarlos.

**Ejemplos**:
- `drup_contrib_check` requiere `module` pero el schema no lo expone
- `drup_issue_patches` requiere `module_name` o `issue_nid`
- `drup_apply_patch` requiere `url` y `project`
- `drup_composer_require` sin parámetros pero necesita saber qué módulo

**Solución aplicada**: usar `drup` CLI directamente con argumentos de línea de comandos.

### 3.5 `drup fix` (autofix) reportó error sin custom modules
**Síntoma**: `drup_autofix` MCP tool devolvió "no custom modules or themes found" como error MCP, cuando debería ser un caso exitoso (0 cosas que fixear).

**Impacto**: el pipeline de Rector no puede diferenciar entre "no hay nada que hacer" (OK) y "falló el tool" (error).

### 3.6 `drup` CLI usa `-r` flag que no funciona con DDEV
**Síntoma**: `drup scan <path>` ejecuta `drush -r <path> upgrade_status:analyze --all`. El `-r <path>` pasa la ruta del host al wrapper `ddev drush`, pero dentro del contenedor DDEV esa ruta no existe.

**Solución aplicada**: ejecutar drush via `ddev exec drush` en lugar de `drush -r <path>`.

---

## 4. Qué se podría mejorar

### 4.1 Pipeline architecture
| Issue | Propuesta |
|-------|-----------|
| Dependencia total en `drup scan`/`drup validate` | Que el orchestrator pueda leer JSON directamente de `upgrade_status:analyze --format=codeclimate` |
| Sin manejo de exit code 3 | Diferenciar entre "error de herramienta" (exit 3) y "hallazgos encontrados" (exit 1 con datos útiles en stdout) |
| Validación solo por exit code | Agregar validación semántica: analizar el JSON de salida para decidir si hay blockers reales |

### 4.2 MCP tooling
| Issue | Propuesta |
|-------|-----------|
| Tools sin schema de parámetros | Exponer `module`, `url`, `path` como parámetros requeridos en el schema |
| Errores como MCP errors vs resultados | `drup_autofix` sin custom modules no debería ser error MCP |
| `drup_contrib_check` falso negativo | Mejorar parser de `core_version_requirement` para rangos compuestos (`^10.3 \|\| ^11.0`) |

### 4.3 PHP 8.4 compatibilidad
| Issue | Propuesta |
|-------|-----------|
| DrupalKernel sobreescribe error_reporting | Agregar paso de preflight que detecte y parchee settings.php automáticamente |
| Deprecations de módulos contrib bloquean herramientas | El pipeline debería suprimir E_DEPRECATED temprano, no reactivamente |

### 4.4 Sub-agentes
| Issue | Propuesta |
|-------|-----------|
| No se usaron en esta prueba | El pipeline está diseñado para delegar módulos custom a sub-agentes, pero este proyecto no tenía. Probar con un proyecto que sí tenga código custom. |
| No hay fallback tools si los MCP fallan | Cuando `drup_scan` MCP falla, el orchestrador debería tener un plan B (ejecutar drush directo) sin depender del humano |

### 4.5 DDEV integration
| Issue | Propuesta |
|-------|-----------|
| `drup` CLI ejecuta comandos en host, no en contenedor | Agregar flag `--ddev` o detectar automáticamente entorno DDEV |
| Paths host vs container | `-r` con ruta host se rompe dentro del contenedor DDEV |

---

## 5. Experiencia general

El concepto del pipeline es sólido: stages secuenciales con validación obligatoria entre cada uno, commits atómicos, y gates que evitan avanzar con errores. La separación en contrib loop, custom loop, y theme loop tiene sentido.

Sin embargo, la implementación actual depende demasiado de `drup` CLI, y `drup` CLI tiene bugs que bloquean el pipeline completo:
- No maneja exit code 3 de drush
- No funciona con DDEV out of the box
- Sus sub-comandos (scan, validate) no producen output estructurado que el orchestrator pueda consumir

El resultado final fue bueno (D11.4.4 funcionando), pero requirió bypassear ~60% de las herramientas automatizadas y ejecutar comandos drush/composer manualmente. Para un proyecto real con 10+ módulos contrib y código custom, esto no escalaría sin las herramientas funcionando correctamente.

**Nota final**: 10/10 volvería a intentarlo — el concepto es correcto, solo necesita madurar la capa de herramientas CLI/MCP.
