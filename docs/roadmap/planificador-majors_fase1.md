# Fase 1: Planificador numérico y parametrizado de versiones mayores

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 2.

## Estado verificado

Completado en el worktree, pendiente de la entrega normal del repositorio. Existe `internal/upgradeplan` con `Major`, `Step`, `Plan`, `Build`, parsing numérico, no-op y secuencias consecutivas ligadas a metadatos por salto. `coreupgrade` consume planes/pasos validados; el CLI, preflight y las superficies MCP de core aceptan o propagan `target_major` y fallan cerrados cuando falta el catálogo exacto.

El catálogo offline entregado contiene **solo** `10-to-11`. El dominio puede construir `9 -> 10 -> 11` si el llamador aporta metadatos para ambos pasos, pero ni `9-to-10` ni `11-to-12` están soportados por el catálogo enviado. La ejecución sigue siendo de un paso: no persiste ni reanuda automáticamente una ruta multi-major.

## Problema y objetivo

Convertir “no saltar majors” en una invariante reutilizable y parametrizar cada ciclo por el major objetivo.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `internal/upgradeplan`: `Major`, `Step`, `Plan`, `Build`.
- `internal/coreupgrade`: consume un único `Step` validado.
- `internal/app`: propaga `target_major` a los checks y las operaciones de core que el flujo actual expone; las herramientas de compatibilidad conservan su `target_version` ya parametrizado.
- Catálogo de compatibilidad por salto: el worktree incluye el identificador offline `10-to-11`; metadatos adicionales (incluidos origen/fecha) son necesarios antes de habilitar otros saltos.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: completado — `internal/upgradeplan/{plan.go,plan_test.go}` cubre parsing numérico, secuencias consecutivas, metadatos obligatorios y no-op.
2. WU2: completado — `internal/coreupgrade/{check.go,apply.go}` consume `Plan`/`Step`; `ApplyPlan` ejecuta solo un paso y el no-op no tiene efectos.
3. WU3: completado — `internal/app/mcp_tools.go` selecciona el major más alto de forma numérica para PHP 8.4.
4. WU4: completado dentro del alcance actual — `target_major` evita reutilizar reglas `10-to-11` para 12; textos y contratos de core son parametrizados. No se declara soporte para catálogos no enviados.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] Un objetivo 9→11 produce pasos 9→10 y 10→11 cuando ambos metadatos se aportan al dominio.
- [x] No se acepta un downgrade ni un major sin metadatos.
- [x] `target_major=12` no usa silenciosamente reglas 10→11.
- [x] El estado no-op no modifica archivos, backups, comandos Composer/Drush ni commits.


## Verificación prevista

```bash
go test ./internal/upgradeplan ./internal/coreupgrade ./internal/app
go test ./...
```

**Evidencia ejecutada:** pruebas enfocadas de `internal/upgradeplan`, `internal/coreupgrade`, `internal/app` e `internal/mcp`; `GOCACHE=/tmp/drup-go-build go test ./...`; y `git diff --check`. La suite completa pasó con acceso de loopback sin sandbox porque `httptest` no puede abrir su listener IPv6 dentro del sandbox.

## Riesgos, migración y rollback

- **Compatibilidad:** versionar contratos persistidos/MCP; mantener compatibilidad read-only solo cuando no debilite invariantes.
- **Datos incompletos:** fallar cerrado y conservar evidencia anterior; nunca inferir éxito.
- **Rollout:** introducir dominio y pruebas antes de hacerlo obligatorio en handlers.
- **Rollback:** retirar el wiring de esta fase y volver al contrato anterior; no borrar evidencia persistida y conservar lector/migración mientras existan runs compatibles.

**Migración:** N/A. No se persiste un formato nuevo de plan ni se migra estado; `target_version` sigue siendo un alias de compatibilidad del contrato MCP.

**Rollback ensayado:** N/A para el plan/no-op, porque no escriben archivos ni crean commits. El rollback de una mutación de un paso sigue usando el checkpoint existente; esta fase no lo modifica.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y superficies MCP de core permanecen coherentes.
- [x] Migración y rollback declarados N/A con razón.
- [x] El salto de major se decide en el dominio tipado, no en el prompt.

## Siguiente fase

Tras cumplir esta definición, continuar con **{nxt}** según [`README.md`](README.md).
