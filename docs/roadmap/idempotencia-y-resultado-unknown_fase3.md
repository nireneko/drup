# Fase 3: Idempotencia de mutaciones y resultado `unknown`

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 4.

## Estado verificado

Completada. El catálogo de `ToolSpec` declara los retries exclusivamente para lecturas; todos los mutadores exigen `request_id`. Un ledger versionado y fail-closed en `.drup/operations.v1.json` conserva la identidad semántica, la respuesta confirmada y la evidencia de reconciliación. Timeouts y cancelaciones de mutaciones se exponen como `unknown` y bloquean el relanzamiento equivalente hasta observar evidencia.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Impedir la repetición ciega de efectos cuando el cliente no puede saber si una mutación terminó.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- Descriptor de fase 2 como fuente de idempotencia.
- Ledger de operaciones persistido por run: request, tool, raíz, target, estado y evidencia.
- Reconciliadores por efecto (Git, lock, módulo, backup).
- Respuesta uniforme que distingue `failed`, `unknown` y `completed`.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: definir modelo `Operation` y transiciones started/completed/failed/unknown con pruebas.
2. WU2: exigir `request_id` en mutadores y devolver resultados confirmados al repetirlo.
3. WU3: mapear timeout/cancelación a `unknown`; nunca relanzar automáticamente.
4. WU4: añadir reconciliación observable y pruebas de timeout después del efecto.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] El mismo request confirmado no repite la mutación.
- [x] `unknown` bloquea un nuevo request equivalente hasta reconciliarlo.
- [x] Retries read-only siguen funcionando sin consumir cap de mutación.
- [x] La reconciliación registra evidencia, no confianza en un booleano del cliente.


## Verificación prevista

```bash
go test ./internal/mcp ./internal/app ./internal/audit
go test ./...
```

Estos comandos son el plan de evidencia, no resultados ejecutados. En sandbox o sin dependencias disponibles, registrar la limitación y NO afirmar que la suite pasa. Añadir readback de artefactos y `git diff --check` antes de revisión.

**Evidencia ejecutada (2026-08-29):**

```bash
GOCACHE=/tmp/drup-go-build go test ./internal/app -run 'TestGuardedCall_(RequiresRequestIDBeforeForcedDryRunPolicy|ConfirmedRequestIsDeduplicatedWithoutSecondAuditOrCapUse|RejectsRequestIDReusedForDifferentOperation|UnknownBlocksEquivalentUntilObservableReconciliation)|TestWireMCPTools_CallToolRequiresRequestIDBeforeGenerateReportEffect' -count=1
GOCACHE=/tmp/drup-go-build go test ./internal/mcp -run 'Test(MutatingDescriptorsRequireRequestIDAndOnlyReadOnlyDescriptorsRetry|WrapInEnvelope_DistinguishesUnknownAndPayloadFailure)' -count=1
GOCACHE=/tmp/drup-go-build go test ./internal/operation -count=1
GOCACHE=/tmp/drup-go-build go test ./...
git diff --check
```

Todos los comandos terminaron correctamente. La suite completa se ejecutó fuera del sandbox porque este bloquea listeners `httptest` IPv6.

## Riesgos, migración y rollback

- **Compatibilidad:** versionar contratos persistidos/MCP; mantener compatibilidad read-only solo cuando no debilite invariantes.
- **Datos incompletos:** fallar cerrado y conservar evidencia anterior; nunca inferir éxito.
- **Rollout:** introducir dominio y pruebas antes de hacerlo obligatorio en handlers.
- **Rollback:** retirar el wiring de esta fase y volver al contrato anterior; no borrar evidencia persistida y conservar lector/migración mientras existan runs compatibles.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración no aplicable: se introduce un ledger nuevo versionado sin datos previos; rollback conserva el archivo y sólo retira el wiring, por lo que no se destruye evidencia.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Tras cumplir esta definición, continuar con **{nxt}** según [`README.md`](README.md).
