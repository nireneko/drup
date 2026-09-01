# Fase 7: Ejecutor determinista de checkpoints operativos

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 8.

## Estado verificado

**Terminada.** `internal/app/checkpoint_executor.go` ejecuta planes deterministas, persiste progreso y conserva paridad entre CLI y MCP.

La arquitectura y el plan siguientes se conservan como contexto de la implementación realizada.

## Problema y objetivo

Ejecutar el límite transaccional repetido como una unidad explícita, verificable y reanudable.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `CheckpointPlan` dentro del run, sin segunda máquina de estados.
- Adaptadores argv-only para updb, cr, status, config export y tests allowlisted.
- Evidence por paso: exit code, duración, hash sanitizado y paths.
- Validador reejecuta checks read-only sobre el mismo candidato.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: modelo de plan/pasos y precondiciones en `internal/runstate`.
2. WU2: adaptadores deterministas en `internal/app`/`internal/exec`.
3. WU3: executor con pausa/reanudación e indisponible distinto de fallo.
4. WU4: integrar patch/minor por lote y major de un solo paquete.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] No se cierra sin backup, updb, validación y export requeridos.
- [x] Un major nunca agrupa más de un target.
- [x] Comandos no pasan por shell ni cwd implícito.
- [x] El fixer no puede reutilizar su propia afirmación como evidencia independiente.


## Verificación ejecutada

```bash
GOCACHE=/tmp/drup-go-build go test -count=1 ./internal/runstate ./internal/exec ./internal/app ./internal/e2e
```

Resultado de cierre: los paquetes indicados terminan en `ok`. La validación global se registra en el índice y en la auditoría de cierre.

## Riesgos, migración y rollback

- **Migración:** no se requiere migración destructiva; los contratos persistidos son versionados y los lectores fallan cerrado ante evidencia desconocida.
- **Rollout:** la fase quedó integrada y cubierta por pruebas enfocadas antes de habilitar sus handlers.
- **Rollback:** revertir `6acd8e8` retira el comportamiento de esta fase. La evidencia persistida no debe borrarse; debe conservarse el lector compatible o bloquearse explícitamente su consumo.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración y rollback han sido ensayados o declarados no aplicables con razón.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Cierre verificado; la dependencia siguiente es **fase 8** según [`README.md`](README.md).
