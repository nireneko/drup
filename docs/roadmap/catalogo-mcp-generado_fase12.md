# Fase 12: Catálogo MCP y documentación generados desde contratos

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a cierre de la hoja de ruta.

## Estado verificado

Completada. `ToolSpec` sigue siendo la fuente única y ahora genera el registro JSON versionado, la tabla de `README`, el contrato de `docs/mcp-tools.md` y los metadatos de stub. La comprobación sin escritura `go run ./cmd/mcp-catalog-gen --check` y `TestBuildCatalogArtifactsMatchesCommittedOutputs` detectan cualquier deriva de bytes.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Convertir el descriptor y schemas consolidados en fuente única para catálogo, stubs y documentación verificable.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- Generador determinista desde `ToolSpec` y contratos versionados.
- Salidas: registry, stub metadata, tablas MCP y sección README.
- Texto largo manual con assertions de side effects.
- CI regenera en temporal y compara bytes.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: definir API del generador y ordenar output estable.
2. WU2: generar catálogo/schema/stub conservando handlers manuales.
3. WU3: generar tablas y corregir claims actual/objetivo.
4. WU4: check CI de deriva y guía para añadir/cambiar tools.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] Una tool se define en una única fuente: `internal/mcp.ToolSpec`.
- [x] CI puede fallar ante deriva entre schema, efecto, guard y docs mediante `mcp-catalog-gen --check` y la prueba de packaging.
- [x] Las docs muestran el catálogo implementado generado; las explicaciones manuales se mantienen fuera de los marcadores y no son contratos.
- [x] `TestBuildCatalogArtifactsIsDeterministic` prueba que dos renderizados producen los mismos bytes.


## Verificación prevista

```bash
go test ./internal/mcp ./internal/packaging
go generate ./... && go run ./cmd/mcp-catalog-gen --check
go test ./...
```

Resultado: las pruebas enfocadas y la suite completa pasaron. La primera suite completa dentro del sandbox no pudo abrir el listener IPv6 de `httptest`; la misma ejecución fuera del sandbox pasó. Se ejecutaron además `go generate ./...`, `mcp-catalog-gen --check` y `git diff --check`.

## Riesgos, migración y rollback

- **Compatibilidad:** versionar contratos persistidos/MCP; mantener compatibilidad read-only solo cuando no debilite invariantes.
- **Datos incompletos:** fallar cerrado y conservar evidencia anterior; nunca inferir éxito.
- **Rollout:** introducir dominio y pruebas antes de hacerlo obligatorio en handlers.
- **Rollback:** retirar el wiring de esta fase y volver al contrato anterior; no borrar evidencia persistida y conservar lector/migración mientras existan runs compatibles.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración no aplicable: no cambian contratos persistidos; rollback retira el wiring generado y conserva el registro como evidencia read-only.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Tras cumplir esta definición, continuar con **{nxt}** según [`README.md`](README.md).
