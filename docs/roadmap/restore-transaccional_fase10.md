# Fase 10: Restauración transaccional y ensayable

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 11.

## Estado verificado

Pendiente. `backup.Manager.Restore` verifica checksums y usa staging, pero importa DB antes de reemplazar el árbol; un fallo puede dejar filesystem y DB de momentos diferentes. No hay journal ni backup de rescate.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Hacer la restauración verificable, recuperable y explícita respecto de ventanas no atómicas.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `restore_check` read-only y `RestorePlan` confirmado.
- Backup de rescate independiente del original.
- Swap de filesystem en mismo volumen cuando sea viable.
- Journal persistente con pasos y continuaciones concretas.
- Estrategia DB temporal o declaración de ventana recuperable.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: preflight de integridad, espacio, permisos, entorno y paths.
2. WU2: journal y máquina de continuación integrada en runstate.
3. WU3: staging/swap con inyección de fallos y conservación del árbol anterior.
4. WU4: adaptadores DB por entorno, verificación post-restore y recuperación.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [ ] Cada fallo inyectado conserva una salida verificable.
- [ ] Backup original y rescate nunca se borran por fallo.
- [ ] Run incompleto bloquea el upgrade.
- [ ] Cuando DB no es atómica, reporte y procedimiento lo declaran.


## Verificación prevista

```bash
go test ./internal/backup ./internal/runstate ./internal/app
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
