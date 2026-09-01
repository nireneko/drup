# Fase 6: Commits y checkpoints autorizados por evidencia

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 7.

## Estado verificado

**Terminada.** `internal/app/checkpoint.go` e `internal/gitops` separan mutación y publicación y exigen evidencia independiente ligada al candidato.

La arquitectura y el plan siguientes se conservan como contexto de la implementación realizada.

## Problema y objetivo

Separar mutación de publicación y aplicar `commit_strategy` como política de dominio verificable.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- `checkpoint_commit` recibe run, scope, paths, estrategia y hash de validación.
- Servicio compartido CLI/MCP para semántica de core.
- Binding entre evidencia, target, revisión del diff y paths actuales.
- `gitops.Commit` permanece como adaptador de staging acotado.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: hacer `apply_patch` y cleanup commit-free manteniendo reporte de paths.
2. WU2: modelar estrategia none/single/per-fix en runstate.
3. WU3: implementar `checkpoint_commit` y rechazo de evidencia obsoleta.
4. WU4: unificar core CLI/MCP en servicio de aplicación con pruebas contractuales.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] `none` produce cero commits.
- [x] No se acepta evidencia anterior al diff, de otro target o con paths extra.
- [x] `single` difiere hasta el cierre y `per-fix` solo abre su checkpoint.
- [x] CLI y MCP ejecutan la misma secuencia observable.


## Verificación ejecutada

```bash
GOCACHE=/tmp/drup-go-build go test -count=1 ./internal/gitops ./internal/app
```

Resultado de cierre: los paquetes indicados terminan en `ok`. La validación global se registra en el índice y en la auditoría de cierre.

## Riesgos, migración y rollback

- **Migración:** no se requiere migración destructiva; los contratos persistidos son versionados y los lectores fallan cerrado ante evidencia desconocida.
- **Rollout:** la fase quedó integrada y cubierta por pruebas enfocadas antes de habilitar sus handlers.
- **Rollback:** revertir `7d3fee2` y `655a076` retira el comportamiento de esta fase. La evidencia persistida no debe borrarse; debe conservarse el lector compatible o bloquearse explícitamente su consumo.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración y rollback han sido ensayados o declarados no aplicables con razón.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Cierre verificado; la dependencia siguiente es **fase 7** según [`README.md`](README.md).
