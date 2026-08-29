# Fase 2: Descriptor único y separación real entre análisis y mutación

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 3.

## Estado verificado

Completado. `internal/mcp.ToolSpec` es el catálogo canónico de nombre, schema, efecto, timeout, rol, precondiciones y visibilidad de stub. `defaultTools()` y `WireMCPTools()` se derivan de ese catálogo; el guard consume su política de sesión y backup. `scan`, `upgrade_scan` y `validate` permanecen en la frontera read-only, mientras que `prepare_upgrade_status` conserva la preparación auditada y protegida.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Hacer ejecutable la frontera read-only/mutating y evitar que un agente validador prepare o modifique el proyecto.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `internal/mcp.ToolSpec`: nombre, schema, clase de efecto, timeout, rol, precondiciones.
- Registro MCP y wiring derivados del descriptor.
- `internal/app/guard.go`: autoriza por efecto y fase, no por listas duplicadas.
- Handlers de scan/validate: solo inspección; preparación propiedad de preflight.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: introducir `ToolSpec` y tests de unicidad/paridad en `internal/mcp`.
2. WU2: generar/derivar registro, stub y wiring sin cambiar contratos públicos.
3. WU3: retirar efectos laterales residuales de `realHandleScan`, `realHandleUpgradeScan` y `autofix`.
4. WU4: pruebas de árbol/config/Composer inmutables para todas las tools del validador.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] Cada herramienta tiene exactamente una clase de efecto.
- [x] Las tools read-only no escriben Composer, configuración ni worktree.
- [x] Preparación es auditada, protegida por backup/cap y ajena al validador.
- [x] Rector no produce una validación que pueda pasar por gate independiente.


## Verificación prevista

```bash
go test ./internal/mcp ./internal/app
go test ./...
```

Estos comandos son el plan de evidencia, no resultados ejecutados. En sandbox o sin dependencias disponibles, registrar la limitación y NO afirmar que la suite pasa. Añadir readback de artefactos y `git diff --check` antes de revisión.

## Riesgos, migración y rollback

- **Compatibilidad:** versionar contratos persistidos/MCP; mantener compatibilidad read-only solo cuando no debilite invariantes.
- **Datos incompletos:** fallar cerrado y conservar evidencia anterior; nunca inferir éxito.
- **Rollout:** introducir dominio y pruebas antes de hacerlo obligatorio en handlers.
- **Rollback:** retirar el wiring de esta fase y volver al contrato anterior; no borrar evidencia persistida y conservar lector/migración mientras existan runs compatibles.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito: `go test ./...` pasa fuera del sandbox (el sandbox bloquea los listeners IPv6 de `httptest`).
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración y rollback no aplican: no se persiste un formato nuevo; revertir el wiring restituye el catálogo anterior sin eliminar evidencia existente.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Tras cumplir esta definición, continuar con **{nxt}** según [`README.md`](README.md).
