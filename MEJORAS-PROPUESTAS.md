# Mejoras implementadas en `drup`

## Resumen ejecutivo

Las doce mejoras están implementadas y disponen de pruebas enfocadas. El estado persistente vive en `internal/runstate`, las herramientas `run_*` están registradas en el catálogo MCP y los efectos, checkpoints y commits se autorizan mediante evidencia persistida. La restauración conserva un journal recuperable incluso en sus fronteras de fallo inyectadas.

La tabla de cierre y los documentos de `docs/roadmap/` son la referencia de estado actual. Las descripciones detalladas que siguen se conservan como diagnóstico histórico: explican el problema original y la solución propuesta, pero no describen trabajo pendiente.

## Estado final verificable

| Fases | Estado | Evidencia de cierre |
|---|---|---|
| 1–3 | Terminadas | `internal/upgradeplan`, descriptores `mcp.ToolSpec` e idempotencia persistente |
| 4–6 | Terminadas | `internal/contracts`, `internal/multiharness`, `internal/runstate` y checkpoints autorizados |
| 7–9 | Terminadas | ejecutor determinista, inventario/reporte reconstruible y planner Composer |
| 10–12 | Terminadas | restore recuperable, patches con procedencia criptográfica y catálogo MCP generado |

## Alcance y método de revisión

Se revisaron:

- CLI y composición: `cmd/drup/main.go`, `internal/app/`.
- MCP: `internal/mcp/server.go`, `internal/mcp/tools.go`, `internal/app/mcp_tools.go`.
- Guardarraíles: `internal/session/session.go`, `internal/app/guard.go`, `internal/audit/audit.go`, `internal/projectconfig/config.go`.
- Git, copias y recuperación: `internal/gitops/`, `internal/backup/`, `internal/coreupgrade/`, `internal/patch/`.
- Drupal.org, versiones y Composer: `internal/drupalorg/`, `internal/semver/`, `internal/composerutil/`.
- Informes y métricas: `internal/report/`, `internal/metrics/`.
- Harness y agentes: `internal/packaging/templates/{claude,codex,opencode}/`.
- Pruebas unitarias, de integración simulada y CI: `internal/**/*_test.go`, `internal/e2e/pipeline_test.go`, `.github/workflows/ci.yml`.
- Documentación operativa: `README.md`, `docs/workflow.md`, `docs/mcp-tools.md` y el diseño local `docs/workflow-state-machine.md`.

## Capacidades existentes que conviene preservar

Estas propuestas no deben reimplementar mecanismos que ya existen:

| Capacidad actual | Evidencia |
|---|---|
| Raíz de proyecto canónica y sesión MCP ligada a ella | `session.CanonicalRoot`, `session.Open`, `session.EvaluateGuard` en `internal/session/session.go` |
| Kill switch, modo inseguro explícito, copia reciente y límite de mutaciones | `guardedCall` en `internal/app/guard.go`; `audit.CheckCap` en `internal/audit/audit.go` |
| Auditoría sin persistir argumentos sensibles en claro | `audit.Record` y `audit.HashArgs` |
| Copias con manifiesto, checksums y protección contra escapes de archivo | `backup.Manifest`, `backup.Manager.Restore`, `backup.extract` |
| Timeouts de subprocesos y terminación de grupos de procesos | `resolveExecTimeout` y `internal/exec.runBounded` |
| Validación contra evidencia obsoleta | `expected_hash` y `scan.ScanResult.EvidenceHash()` en `realHandleValidate` |
| Commits con rutas declaradas y rechazo de contenido ya preparado fuera de alcance | `gitops.Commit` |
| Allowlist de URL basada en host, no en subcadenas | `patch.defaultCheckAllowedURL` y `TestDefaultCheckAllowedURL` |
| Normalización y bloqueo de alias peligrosos de Drush | `normalizeDrushCommand`, `drushAliasMap` y `TestDrushExec_BlocklistViaAlias` |

## Estado final de los tres hallazgos críticos del informe anterior

| Hallazgo | Estado actual | Evidencia |
|---|---|---|
| Allowlist de parches mediante `strings.Contains` | **Corregido** | La URL inicial y cada redirect se validan por esquema y hostname; `TestDownloadPatch_RejectsUnsafeRedirectBeforeBody` prueba el límite. |
| Comparación lexicográfica de versiones | **Corregido** | `internal/upgradeplan` opera con majors numéricos y `TestDrupalVersionMatrix_SelectsHighestNumericMajorForPHP84` comprueba el valor exacto. |
| Alias de comandos Drush peligrosos | **Corregido** | `drushAliasMap` normaliza `sqlq`, `sql-cli`, `sqlc`, `scr`, `ev`, `exec` y `core:execute`; la blocklist incluye `sql:query` y `php:script`. |

## Hoja de ruta priorizada

| Prioridad | Propuesta | Impacto | Esfuerzo estimado |
|---|---|---|---|
| P0 | 1. Planificador numérico y parametrizado de versiones mayores | Evita saltos de major y decisiones incorrectas | Medio-alto |
| P0 | 2. Estado persistente de ejecución y autoridad de transiciones | Hace el workflow recuperable y exigible | Alto |
| P0 | 3. Separación real entre análisis y mutación | Conserva la independencia del validador | Medio |
| P0 | 4. Commits y checkpoints autorizados por evidencia | Impide commits prematuros o contrarios a la estrategia | Alto |
| P0 | 5. Reintentos idempotentes y estado `unknown` | Evita duplicar mutaciones tras timeouts | Medio |
| P1 | 6. Ejecutor determinista de checkpoints operativos | Reduce Bash ad hoc y omisiones de `updb`, export o smoke tests | Alto |
| P1 | 7. Inventario inicial y reporte antes/después reconstruible | Aporta trazabilidad real y auditoría | Medio-alto |
| P1 | 8. Planificador de dependencias Composer para contrib | Detecta bloqueos antes de mutar | Medio-alto |
| P1 | 9. Restauración transaccional y ensayable | Reduce estados mixtos durante recuperación | Alto |
| P1 | 10. Harness de contratos y escenarios multiagente | Detecta contradicciones entre prompts, schemas y handlers | Medio-alto |
| P2 | 11. Cadena de suministro verificable para parches | Cierra redirecciones, descargas ilimitadas y falta de procedencia | Medio |
| P2 | 12. Documentación y catálogo MCP generados desde contratos | Evita deriva entre código, README y agentes | Medio |

## 1. Planificador numérico y parametrizado de versiones mayores

**Prioridad:** P0  
**Esfuerzo:** medio-alto  
**Dependencias:** debería preceder al estado persistente, porque sus transiciones necesitan una fuente fiable para `current_major`, `next_major` y `target_major`.

**Funcionalidad:** incorporar un planificador de actualización que calcule una secuencia explícita de majors inmediatos, por ejemplo `9 -> 10 -> 11`, y parametrizar todas las operaciones por `target_major` en lugar de asumir Drupal 11.

**Por qué sería aconsejable:** `docs/workflow.md` establece que nunca se debe saltar una versión mayor y generaliza el proceso para Drupal 12, 13 y posteriores. Esa regla debe ser una propiedad del dominio, no una recomendación del prompt.

**Problema que resolvería:**

- `coreupgrade.NextMajor` compara el major actual con el último major publicado y devuelve este último; no obliga a que sea `currentMajor + 1`.
- `coreupgrade.Apply` acepta cualquier `targetVersion` válido sin comprobar que sea el siguiente major inmediato.
- `realHandleDrupalVersionMatrix` compara claves como strings, por lo que `"9"` puede ganar a `"11"`.
- La producción mantiene referencias fijas a D11 en `drupalorg.ReleaseInfo.HasD11`, `checkCoreReadiness`, `patch.CommitSubject`, defaults de compatibilidad y descripciones MCP. El agente recibe `target_major`, pero varios handlers lo ignoran o lo sustituyen por `"11"`.

**Cómo podría implementarse:**

1. Crear `internal/upgradeplan` con valores numéricos y tipos `Major`, `Step` y `Plan`.
2. Construir el plan como una lista de pasos consecutivos desde el major instalado hasta el objetivo; rechazar objetivos inferiores, saltos directos y majors sin metadatos compatibles.
3. Cambiar `core_upgrade_check`, `contrib_check`, preflight, compatibilidad, nombres de commits y reglas de Rector para recibir el major del paso actual.
4. Sustituir `HasD11` por un resultado general, por ejemplo `CompatibleWithTarget` y `TargetMajor`.
5. Mantener catálogos de deprecaciones versionados por salto (`10-to-11`, `11-to-12`) en vez de presentar un catálogo D11 como universal.
6. Reemplazar la matriz estática por datos versionados y actualizables, con una copia embebida para funcionamiento offline y fecha/origen visibles.

**Criterios de éxito:**

- Una ejecución `9 -> 11` produce dos ciclos y nunca ofrece `11` como primer paso.
- PHP 8.4 selecciona el major compatible numéricamente esperado, con una prueba que aserte el valor exacto.
- `target_major=12` no reutiliza silenciosamente reglas o textos de D11.
- Un proyecto ya en el objetivo devuelve un no-op explícito y no crea commits ni copias innecesarias.

## 2. Estado persistente de ejecución y autoridad de transiciones

**Prioridad:** P0  
**Esfuerzo:** alto  
**Dependencias:** planificador de majors de la propuesta 1.

**Funcionalidad:** implementar un registro persistente por ejecución que determine la fase actual, evidencias aceptadas, acción permitida, backups, commits, bloqueos y trabajo pendiente.

**Por qué sería aconsejable:** una sesión MCP actual solo vive en memoria y `pipeline_status` resume llamadas de mutación. Ninguno de los dos mecanismos puede demostrar que se respetaron Git, tooling, baseline, orden de contrib, validación, exportación de configuración o el bucle de core.

**Problema que resolvería:** después de reiniciar el servidor, el orquestador debe reconstruir el progreso a partir de la conversación. Además, un handler mutante no sabe si la fase normativa que lo autoriza ha pasado. Las plantillas dicen qué hacer, pero Go no lo exige.

**Cómo podría implementarse:**

1. Implementar el diseño de `docs/workflow-state-machine.md` en `internal/runstate`, añadiendo `schema_version`, persistencia atómica, bloqueo por proyecto y evidencias append-only.
2. Exponer `run_create`, `run_status`, `run_record`, `run_confirm`, `run_block` y `run_abandon` en las tres superficies MCP: handler real, stub y schema.
3. Añadir `run_id` a los mutadores y componer el guard de ejecución con sesión, backup reciente y límite de mutación dentro de `guardedCall`.
4. Hacer que `run_status.allowed_actions` sea la única fuente para el siguiente dispatch.
5. Persistir identificadores y hashes de evidencia, no stdout completo ni secretos.

**Criterios de éxito:**

- Reiniciar MCP conserva exactamente la siguiente acción.
- Un mutador sin `run_id`, en una fase incorrecta o para otra raíz canónica no ejecuta el handler subyacente.
- No pueden existir dos runs activos para la misma raíz.
- El reporte final puede reconstruirse sin usar el historial conversacional.

**Nota de alcance:** esta funcionalidad está definida en un Markdown local y en artefactos OpenSpec locales, pero no debe contabilizarse como implementada hasta que existan el paquete, las herramientas y las pruebas de guard.

## 3. Separación real entre análisis y mutación

**Prioridad:** P0  
**Esfuerzo:** medio  
**Dependencias:** el guard de fase de la propuesta 2 simplifica la autorización, pero la separación semántica puede iniciarse antes.

**Funcionalidad:** convertir las herramientas del validador en operaciones estrictamente read-only y trasladar toda preparación de Upgrade Status a una acción mutante propiedad de preflight.

**Por qué sería aconsejable:** la independencia del validador es el principal guardarraíl del harness. Si su herramienta de análisis instala paquetes, borra configuración o habilita módulos, la frontera de responsabilidad es solo nominal.

**Problema que resolvería:**

- `realHandleScan` llama a `ensureUpgradeStatusEnabled`.
- `realHandleUpgradeScan` puede ejecutar `composer require`, `config:delete update.settings`, `drush en` y `drush cr` antes de analizar.
- `ensureUpgradeStatusEnabled` ignora errores de `config:delete` y `cr`.
- `realHandleAutofix` vuelve a ejecutar `upgrade_status:analyze`, aunque las plantillas reservan scan/validate al validador.
- `drup-validator` declara que no toca Composer, pero su primera instrucción es llamar a una herramienta que puede hacerlo.

**Cómo podría implementarse:**

1. Crear una operación mutante `prepare_upgrade_status`, asignada a `drup-preflight`, que instale, habilite y registre cada cambio.
2. Hacer que `scan`, `validate` y `upgrade_scan` fallen con una salida accionable si falta la preparación, sin modificar nada.
3. Eliminar el rescan interno de `autofix`; debe devolver únicamente salida de Rector, paths modificados y diff/resumen.
4. Etiquetar las herramientas como `read_only`, `dry_run_capable` o `mutating` en un único descriptor y probar que el registro, el guard y la documentación coinciden.
5. Usar el estado de ejecución para autorizar la herramienta por fase, en lugar de confiar en que el agente recuerde su allowlist.

**Criterios de éxito:**

- Ejecutar cualquier herramienta permitida al validador deja iguales `composer.json`, `composer.lock`, configuración activa y árbol de trabajo.
- La preparación aparece como mutación auditada y con backup previo.
- El agente Rector no produce una validación propia ni un contador que pueda confundirse con el gate independiente.

## 4. Commits y checkpoints autorizados por evidencia

**Prioridad:** P0  
**Esfuerzo:** alto  
**Dependencias:** estado persistente de la propuesta 2.

**Funcionalidad:** desacoplar mutación y commit, y añadir una única operación de commit que solo acepte rutas declaradas después de validar evidencia independiente para el mismo candidato.

**Por qué sería aconsejable:** el usuario elige `per-fix`, `single` o `none`; esa decisión debe controlar todos los commits. También debe cumplirse la regla “no commit antes del gate del validador”.

**Problema que resolvería:**

- `patch.Apply` aplica, registra y hace commit antes de que `drup-validator` inspeccione el resultado.
- `RunCleanup` crea un commit cuando recibe `--validate-passed`; el MCP confía en el booleano `validate_passed` y no acepta `commit_strategy`.
- La estrategia `none` no puede impedir esos commits desde el handler.
- `coreupgrade.Apply` crea un checkpoint vacío, pero el MCP no ejecuta el ciclo completo de Composer, `updb` y verificación que realiza el comando CLI `RunUpgradeCore`; las dos superficies tienen semánticas distintas.

**Cómo podría implementarse:**

1. Hacer que `apply_patch` y `cleanup` modifiquen y reporten paths, pero nunca creen commits.
2. Añadir una operación de dominio `checkpoint_commit` que reciba `run_id`, scope, paths, mensaje y `validation_evidence_hash`.
3. Verificar que el hash corresponde al diff actual, al mismo target y a una evidencia producida por el validador después de la mutación.
4. Reutilizar `gitops.Commit` para el staging acotado y persistir el hash resultante en el run.
5. Aplicar `commit_strategy` dentro de Go: `none` rechaza commits, `single` difiere hasta el final y `per-fix` habilita el checkpoint correspondiente.
6. Unificar la semántica CLI/MCP del core en un servicio de aplicación compartido; evitar que uno solo reescriba `composer.json` mientras el otro ejecuta la actualización completa.

**Criterios de éxito:**

- `commit_strategy=none` produce cero commits en un escenario completo.
- Ningún commit de cambios funcionales se crea con evidencia anterior al diff actual.
- Un hash obsoleto, otro target o paths adicionales son rechazados.
- CLI y MCP producen los mismos pasos y resultados para el core.

## 5. Reintentos idempotentes y estado de resultado desconocido

**Prioridad:** P0  
**Esfuerzo:** medio  
**Dependencias:** almacenamiento de operaciones de la propuesta 2.

**Funcionalidad:** introducir claves idempotentes y distinguir `failed` de `unknown` cuando un timeout puede haber ocurrido después de una mutación parcial.

**Por qué sería aconsejable:** reintentar lecturas HTTP es razonable; reintentar automáticamente un mutador no lo es si no se sabe si el primer intento terminó en el proceso externo.

**Problema que resolvería:** `mcp.Server.retryLoop` aplica hasta tres intentos a cualquier handler cuando el error parece transitorio. Un timeout de Composer, Drush o Git puede devolver el control antes de que todo el árbol de procesos haya terminado o después de haber escrito parte del estado. Repetir el mismo handler puede duplicar instalaciones, commits o cambios de configuración.

**Cómo podría implementarse:**

1. Clasificar cada herramienta en el descriptor único propuesto en la sección 3.
2. Mantener retries automáticos solo para herramientas read-only e HTTP idempotente.
3. Exigir `request_id` para mutadores; registrar `started`, `completed`, `failed` o `unknown` antes y después del efecto.
4. Ante timeout, consultar evidencia observable — Git, Composer lock, estado del módulo, run record — antes de permitir una continuación.
5. Reutilizar un resultado ya confirmado cuando se repite el mismo `request_id`.

**Criterios de éxito:**

- Una prueba de timeout después del efecto demuestra que solo existe una mutación.
- Un estado `unknown` bloquea otro intento ciego y ofrece una verificación concreta.
- Los retries de Drupal.org continúan funcionando sin consumir presupuesto de mutación.

## 6. Ejecutor determinista de checkpoints operativos

**Prioridad:** P1  
**Esfuerzo:** alto  
**Dependencias:** propuestas 2, 4 y 5.

**Funcionalidad:** ejecutar como una unidad de dominio los checkpoints repetidos del workflow: backup, mutación acotada, `updb`, cache rebuild, status, smoke tests, exportación de configuración, validación y commit opcional.

**Por qué sería aconsejable:** `docs/workflow.md` repite el mismo límite transaccional en custom/theme, contrib patch/minor/major, core y cleanup. Encargar cada comando a texto libre aumenta la posibilidad de omitir un paso o aceptar evidencia del target equivocado.

**Problema que resolvería:** hoy no existen herramientas deterministas para `config:export`, selección de tests del proyecto o smoke checks. Las plantillas ordenan al agente contrib ejecutar Composer, base de datos, status y export, pero el agente recurre a Bash. El validador, por su parte, solo puede “reportar evidencia suministrada” y no ejecutarla con sus herramientas permitidas.

**Cómo podría implementarse:**

1. Modelar un `CheckpointPlan` dentro de `internal/runstate` o un paquete de aplicación, evitando una segunda máquina de estados paralela.
2. Añadir adaptadores deterministas para `updb`, `cr`, `status`, `config:export` y comandos de prueba configurados por proyecto.
3. Capturar por paso comando lógico, exit code, duración, hash de salida sanitizada y paths modificados.
4. Permitir que el validador reejecute únicamente los checks read-only sobre el candidato, sin reutilizar la afirmación del fixer.
5. Aplicar el mismo checkpoint a una fase completa patch/minor y a un único paquete major.

**Criterios de éxito:**

- No se puede cerrar un checkpoint si falta backup, `updb`, validación o export requerido.
- Un package major no admite más de un target.
- Un check no disponible se distingue de uno fallido y queda visible en el reporte.
- Los comandos no se ejecutan mediante shell ni dependen del directorio ambiente.

## 7. Inventario inicial y reporte antes/después reconstruible

**Prioridad:** P1  
**Esfuerzo:** medio-alto  
**Dependencias:** run state y checkpoint executor.

**Funcionalidad:** capturar un inventario estructurado al inicio y compararlo con el estado final para generar el informe desde evidencia persistida.

**Por qué sería aconsejable:** un informe de actualización debe responder qué cambió, por qué, cómo se verificó y cómo volver atrás. Un scan final no puede reconstruir esa historia.

**Problema que resolvería:**

- `report.ReportData` solo contiene errores, items resueltos/pendientes, tokens y métricas.
- `realHandleGenerateReport` inicializa `Resolved` vacío y solo agrega pendientes desde un scan vivo.
- `include_patch_list` incrementa `TokenAccounting.Total` para contar parches, mezclando dos conceptos distintos.
- No se persisten versiones anteriores/posteriores, branch, commits, backups, exportaciones, tooling temporal, checks runtime ni patches retirados.
- La plantilla del validador pide esa información, pero el handler `generate_report` no acepta la evidencia acumulada.

**Cómo podría implementarse:**

1. Añadir `inventory_capture` read-only con core/PHP, paquetes exactos, módulos/temas habilitados, restricciones, patches, estado de configuración y comandos de test detectados.
2. Persistir el baseline como evidencia del run y capturar un inventario final del mismo schema.
3. Extender `ReportData` con `Before`, `After`, `Changes`, `Checkpoints`, `Backups`, `Commits`, `ConfigExports`, `Validations`, `Skipped` y `PendingHuman`.
4. Generar JSON y Markdown a partir del run, no a partir de parámetros booleanos que vuelven a escanear de manera implícita.
5. Mantener métricas y token accounting como secciones independientes.

**Criterios de éxito:**

- El reporte enumera versiones exactas antes/después y la procedencia de cada cambio.
- Cada commit, patch y backup enlaza con el checkpoint que lo produjo.
- El informe se regenera de forma idéntica después de reiniciar MCP.
- Parches aplicados no alteran el contador de tokens.

## 8. Planificador de dependencias Composer para contrib

**Prioridad:** P1  
**Esfuerzo:** medio-alto  
**Dependencias:** planificador de majors e inventario inicial.

**Funcionalidad:** crear un plan read-only de compatibilidad y orden de actualización basado en el grafo Composer real del proyecto, además de los metadatos de Drupal.org.

**Por qué sería aconsejable:** saber que existe una release compatible no significa que el proyecto pueda instalarla. Dependencias cruzadas, paquetes abandonados, plugins y restricciones raíz suelen ser el verdadero bloqueo de una actualización mayor.

**Problema que resolvería:** `drupalorg.UpgradePath` recomienda una release por módulo según Drupal.org, pero no evalúa el lock local ni ejecuta `composer prohibits/why-not`. La clasificación patch/minor/major y el orden siguen dependiendo del agente.

**Cómo podría implementarse:**

1. Añadir `contrib_plan` read-only que lea `composer.lock` y ejecute Composer en modo no mutante (`show --outdated --direct --format=json`, `prohibits`/`why-not`).
2. Combinar el resultado con `module_release_info` y `contrib_upgrade_path`.
3. Clasificar cada paquete como no-op, patch, minor, major, patch requerido, abandonado o bloqueado por dependencia.
4. Construir grupos seguros para patch/minor y targets aislados para major, con la cadena causal del bloqueo.
5. Guardar el plan como ledger del ciclo del major inmediato y recalcularlo tras cada core major.

**Criterios de éxito:**

- Un conflicto conocido se detecta antes de modificar `composer.json` o `composer.lock`.
- Cada bloqueo identifica el paquete raíz y la restricción que lo causa.
- Ningún lote major contiene más de un paquete.
- El mismo lock produce un plan determinista.

## 9. Restauración transaccional y ensayable

**Prioridad:** P1  
**Esfuerzo:** alto  
**Dependencias:** estado persistente y confirmaciones explícitas.

**Funcionalidad:** separar verificación y aplicación de una restauración, mantener un diario de recuperación y reducir la posibilidad de terminar con base de datos y archivos de momentos distintos.

**Por qué sería aconsejable:** la recuperación es la última barrera de seguridad. Sus fallos deben ser más controlados que los de una mutación ordinaria.

**Problema que resolvería:** `backup.Manager.Restore` verifica checksums y extrae a staging, pero después importa primero la base de datos y `replaceProject` elimina el árbol actual antes de copiar los archivos. Si la copia falla, el proyecto queda parcialmente sustituido; si falla después de importar la base, base y filesystem pueden quedar desalineados.

**Cómo podría implementarse:**

1. Añadir `restore_check` read-only: compatibilidad del manifiesto, espacio libre, permisos, integridad completa, entorno, base de datos y plan de paths.
2. Después de la confirmación humana, crear una copia de rescate previa a restaurar y registrar su ID.
3. Preparar el filesystem en el mismo volumen y usar renames atómicos de directorios cuando el layout lo permita, conservando el árbol anterior hasta completar la verificación.
4. Para la base de datos, preferir importación a una base temporal y cambio controlado cuando el entorno lo soporte; en caso contrario, registrar explícitamente la ventana no atómica y el procedimiento de recuperación.
5. Persistir un journal `restore_started/files_swapped/db_imported/verified/completed` y ofrecer una continuación concreta tras cada fallo.

**Criterios de éxito:**

- Pruebas de inyección de fallo en cada paso dejan una ruta de recuperación verificable.
- Ningún fallo borra el backup original ni el backup de rescate.
- Una restauración incompleta aparece como tal en `run_status` y no permite continuar el upgrade.

## 10. Harness de contratos y escenarios multiagente

**Prioridad:** P1  
**Esfuerzo:** medio-alto  
**Dependencias:** ninguna para comenzar; debe evolucionar junto con las propuestas 2 y 6.

**Funcionalidad:** validar automáticamente que prompts, dispatches, respuestas de agentes, schemas MCP y handlers forman un protocolo coherente en Claude, Codex y OpenCode.

**Por qué sería aconsejable:** las pruebas actuales verifican presencia de frases y paridad parcial, pero no demuestran que un orquestador pueda completar el workflow con los contratos reales.

**Problema que resolvería:** existen contradicciones actuales que las pruebas no detectan:

- El orquestador exige `status: pass|fail|blocked`, mientras Rector devuelve `completed|failed`, custom/theme `fixed|failed` y contrib `updated|patched|created|failed`.
- `drup-validator` usa `baseline` y una fase de reporte, pero su lista de scopes permitidos solo incluye `env`, `rector`, `contrib`, `custom`, `theme` y `global`.
- El agente Rector envía `target_paths`, aunque el schema y `realHandleAutofix` solo leen `project_path` y procesan todos los paths encontrados.
- El agente contrib llama a `apply_patch` sin `composer_package` ni `description`, pese a que esos datos determinan la raíz del paquete y el registro Composer.
- `internal/e2e/pipeline_test.go` es una integración simulada del CLI; `TestPipeline_StageOrdering` no aserta el orden completo y no ejecuta agentes ni el flujo normativo.

**Cómo podría implementarse:**

1. Definir JSON Schema versionado para `Dispatch`, `AgentReport`, `ValidationEvidence` y `CheckpointEvidence`.
2. Validar cada reporte antes de que el orquestador lo use y rechazar estados/campos desconocidos con una salida accionable.
3. Generar los fragmentos de contrato de las plantillas desde el schema común, conservando solo instrucciones específicas por rol.
4. Crear escenarios transcript-driven con un MCP falso: happy path, árbol sucio, backup fallido, dos retries, contrib major aislado, core multi-major, confirmación rechazada y restart.
5. Ejecutar los mismos escenarios sobre los artefactos renderizados de las tres plataformas.

**Criterios de éxito:**

- Un estado no permitido o un scope imposible falla en CI.
- Cada tool call de un agente satisface el schema MCP real.
- Los escenarios prueban el orden completo y los límites de retry, no solo la existencia de palabras en `SKILL.md`.
- La paridad semántica entre plataformas se verifica después del render.

## 11. Cadena de suministro verificable para parches

**Prioridad:** P2  
**Esfuerzo:** medio  
**Dependencias:** el inventario y el run state mejoran la trazabilidad, pero el hardening de red puede realizarse de forma independiente.

**Funcionalidad:** validar cada salto HTTP, limitar el tamaño descargado y registrar la identidad criptográfica y procedencia de cada patch.

**Por qué sería aconsejable:** la corrección de la allowlist inicial es importante, pero el contenido remoto sigue siendo una entrada de código que se incorpora al proyecto.

**Problema que resolvería:** `patch.httpClient` usa el comportamiento de redirección predeterminado de `net/http`; `downloadPatch` valida la URL original, pero no vuelve a aplicar la allowlist al destino de una redirección. Además, copia `resp.Body` sin límite y Composer registra la URL, no el hash del contenido revisado.

**Cómo podría implementarse:**

1. Configurar `CheckRedirect` para validar esquema y host en todos los saltos y limitar su cantidad.
2. Aplicar un máximo de bytes mediante `Content-Length` y `io.LimitReader`.
3. Calcular SHA-256, tamaño, URL inicial/final, fecha y issue de origen; persistirlos como evidencia.
4. Ejecutar `git apply --check` antes de aplicar y registrar las rutas afectadas para compararlas con el paquete esperado.
5. Rechazar patches que escriban fuera del paquete declarado o que cambien tipos de archivo no autorizados sin decisión humana.

**Criterios de éxito:**

- Una redirección de `drupal.org` a un host no permitido se rechaza antes de descargar el cuerpo.
- Una respuesta sobredimensionada se corta y elimina el temporal.
- El reporte permite demostrar qué bytes exactos se aplicaron.

## 12. Documentación y catálogo MCP generados desde contratos

**Prioridad:** P2  
**Esfuerzo:** medio  
**Dependencias:** descriptor único de herramientas y schemas del harness.

**Funcionalidad:** generar el catálogo de herramientas, propiedades de seguridad y fragmentos operativos desde una única definición versionada.

**Por qué sería aconsejable:** la documentación es parte del harness: un agente ejecuta lo que lee. La deriva documental puede producir una operación incorrecta aunque el código aislado sea seguro.

**Problema que resolvería:**

- `README.md` todavía presenta un pipeline de siete stages que no coincide con la secuencia normativa actual.
- `README.md` afirma que cleanup elimina Upgrade Status y Rector, pero `RunCleanup` solo desinstala/elimina `drupal/upgrade_status`.
- `docs/mcp-tools.md` afirma que `core_upgrade_apply` crea “dos commits (checkpoint + bump)”, mientras `coreupgrade.Apply` crea un checkpoint vacío y deja `composer.json` modificado sin commit de bump.
- Las definiciones de tools se mantienen por triplicado en `toolRegistry`, `defaultTools` y `WireMCPTools`; las pruebas cubren simetría de nombres, pero no que descripción, efectos y contrato de respuesta coincidan con la implementación.

**Cómo podría implementarse:**

1. Crear un descriptor `ToolSpec` con schema, clase de efecto, timeout, requisitos, rol propietario y documentación breve.
2. Generar `toolRegistry`, stubs, tablas de `docs/mcp-tools.md` y la sección MCP del README.
3. Mantener las explicaciones largas manuales, pero verificar sus claims de side effects mediante tests de contrato.
4. Añadir un check de CI que regenere en temporal y falle si el árbol versionado difiere.
5. Marcar explícitamente capacidades implementadas, planificadas y obsoletas.

**Criterios de éxito:**

- Añadir o cambiar una herramienta exige modificar una sola fuente.
- CI detecta cualquier diferencia entre schema, handler, guard y documentación.
- README y `docs/workflow.md` describen el mismo proceso efectivo o distinguen claramente “actual” de “objetivo”.

## Orden de implementación recomendado

### Entrega 1 — cerrar riesgos de decisión

1. Planificador numérico de majors y corrección de comparación de versiones.
2. Separación de análisis/mutación.
3. Clasificación de herramientas y desactivación de retries automáticos en mutadores.
4. Pruebas de contrato para los hallazgos actuales.

### Entrega 2 — convertir el workflow en autoridad de código

1. `internal/runstate` y herramientas `run_*`.
2. Guard de fase y confirmaciones.
3. Commit autorizado por evidencia y estrategia.
4. Reanudación tras restart e idempotencia por `request_id`.

### Entrega 3 — automatización operativa completa

1. Checkpoint executor.
2. Inventario y reporte antes/después.
3. Plan de Composer para contrib.
4. Escenarios multi-major y multiagente.

### Entrega 4 — recuperación y endurecimiento

1. Restore ensayable con journal.
2. Procedencia e integridad de patches.
3. Generación de documentación y catálogo MCP.

## Riesgos y decisiones de producto pendientes

- **Compatibilidad hacia atrás:** añadir `run_id` obligatorio rompe clientes anteriores. Conviene versionar el contrato MCP o mantener un periodo de compatibilidad solo para operaciones read-only.
- **Política offline:** una matriz dinámica necesita cache embebida y una política explícita cuando Drupal.org no está disponible.
- **Tests de proyecto:** ejecutar PHPUnit, Behat, Playwright o comandos propios requiere una allowlist/configuración por proyecto; no debe aceptarse shell arbitrario desde el LLM.
- **Atomicidad de base de datos:** no todos los entornos permiten importar a una base temporal y conmutarla. El informe debe declarar cuándo la restauración solo puede ser recuperable, no estrictamente atómica.
- **Alcance de Rector:** los catálogos D11 actuales siguen siendo útiles para el salto 10→11; deben versionarse, no borrarse ni generalizarse artificialmente.

## Resultado esperado

Al completar las propuestas P0 y P1, el LLM seguirá coordinando y resolviendo trabajo no determinista, pero dejará de ser la autoridad sobre el orden, los permisos, los commits y la evidencia. Go podrá responder de forma verificable: qué fase está activa, qué se completó, qué falló, qué acción exacta es válida y cómo recuperar el proyecto.
