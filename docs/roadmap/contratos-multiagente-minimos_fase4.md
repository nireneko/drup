# Fase 4: Contratos mínimos y escenarios multiagente

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 5.

## Estado verificado

**Terminada.** `internal/contracts` e `internal/multiharness` implementan schemas versionados, validación de reportes y un corpus compartido para Claude, Codex y OpenCode.

La arquitectura y el plan siguientes se conservan como contexto de la implementación realizada.

## Problema y objetivo

Detectar en CI estados, scopes, argumentos y secuencias incompatibles antes de que un agente intente el workflow real.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- Schemas versionados: `Dispatch`, `AgentReport`, `ValidationEvidence`, `CheckpointEvidence`.
- Adaptador de cada plataforma hacia el contrato común.
- MCP falso que valida calls y reproduce fallos deterministas.
- Escenarios compartidos renderizados desde `internal/packaging/templates`.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: inventariar estados/scopes/campos actuales y fijar schemas mínimos compatibles.
2. WU2: validar reportes antes del dispatch y producir errores accionables.
3. WU3: escenarios de happy path, dirty tree, backup fail, retry, rechazo y restart.
4. WU4: ejecutar el mismo corpus sobre Claude/Codex/OpenCode y probar orden completo.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [x] Estados o scopes desconocidos fallan en CI.
- [x] Cada llamada satisface el schema real de la tool.
- [x] Las tres plataformas recorren la misma semántica.
- [x] Los escenarios demuestran límites de retry y aislamiento major, no solo frases.


## Verificación ejecutada

```bash
GOCACHE=/tmp/drup-go-build go test -count=1 ./internal/contracts ./internal/multiharness ./internal/mcp ./internal/packaging
```

Resultado de cierre: los paquetes indicados terminan en `ok`. La validación global se registra en el índice y en la auditoría de cierre.

## Riesgos, migración y rollback

- **Migración:** no se requiere migración destructiva; los contratos persistidos son versionados y los lectores fallan cerrado ante evidencia desconocida.
- **Rollout:** la fase quedó integrada y cubierta por pruebas enfocadas antes de habilitar sus handlers.
- **Rollback:** revertir `cd454d1` retira el comportamiento de esta fase. La evidencia persistida no debe borrarse; debe conservarse el lector compatible o bloquearse explícitamente su consumo.

## Definición de terminado

- [x] Todos los criterios tienen prueba enfocada y evidencia registrada.
- [x] Suite aplicable y `git diff --check` ejecutados con resultado explícito.
- [x] Contratos, docs y tres superficies MCP permanecen coherentes.
- [x] Migración y rollback han sido ensayados o declarados no aplicables con razón.
- [x] No quedan decisiones del workflow confiadas únicamente al prompt.

## Siguiente fase

Cierre verificado; la dependencia siguiente es **fase 5** según [`README.md`](README.md).
