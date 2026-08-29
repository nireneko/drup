# Fase 9: Planificador Composer para contrib

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 10.

## Estado verificado

Pendiente. `drupalorg.UpgradePath` aporta releases, pero no analiza `composer.lock`, `show --outdated`, `prohibits` o `why-not`; clasificación y orden continúan en el agente.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Detectar bloqueos causales antes de tocar Composer y producir lotes deterministas por major inmediato.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `internal/contribplan`: paquete, clasificación, causalidad y grupos.
- Adaptador Composer read-only con JSON, sin shell.
- Fusión con Drupal.org e Inventory del mismo ciclo.
- Ledger recalculado después de cada core major.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: parser de lock y modelo de clasificación con fixtures.
2. WU2: ejecutar `show --outdated --direct --format=json` y consultas why-not/prohibits.
3. WU3: resolver cadenas causales y ordenar grupos patch/minor; major aislado.
4. WU4: tool `contrib_plan`, persistencia y escenarios de conflicto.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [ ] Conflictos se detectan antes de modificar json/lock.
- [ ] Cada bloqueo identifica raíz y restricción.
- [ ] Lock idéntico produce plan idéntico.
- [ ] Un grupo major contiene exactamente un paquete.


## Verificación prevista

```bash
go test ./internal/contribplan ./internal/drupalorg ./internal/app ./internal/mcp
go test ./...
```

Estos comandos son el plan de evidencia, no resultados ejecutados. En sandbox o sin dependencias disponibles, registrar la limitación y NO afirmar que la suite pasa. Añadir readback de artefactos y `git diff --check` antes de revisión.

## Riesgos, migración y rollback

- **Compatibilidad:** versionar contratos persistidos/MCP; mantener compatibilidad read-only solo cuando no debilite invariantes.
- **Datos incompletos:** fallar cerrado y conservar evidencia anterior; nunca inferir éxito.
- **Rollout:** introducir dominio y pruebas antes de hacerlo obligatorio en handlers.
- **Rollback:** retirar el wiring de esta fase y volver al contrato anterior; no borrar evidencia persistida y conservar lector/migración mientras existan runs compatibles.

## Definición de terminado

- [ ] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [ ] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [ ] Contratos, docs y tres superficies MCP permanecen coherentes.
- [ ] Migración y rollback han sido ensayados o declarados no aplicables con razón.
- [ ] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Tras cumplir esta definición, continuar con **{nxt}** según [`README.md`](README.md).
