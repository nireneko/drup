# Cierre de la auditoría de `MEJORAS-PROPUESTAS.md`

Los tres pendientes detectados por la auditoría quedaron corregidos: la documentación refleja el árbol actual, las fases 4–8 y 10–11 registran su cierre, y restore dispone de una matriz table-driven para las fronteras de fallo que no estaban cubiertas.

## Resultado

| Alcance | Resultado |
|---|---|
| Implementación funcional | **12/12 fases implementadas** |
| Documentación de cierre | **Sin estados históricos presentados como actuales** |
| Cobertura de fallos de restore | **Rescue, extracción, journal, DB, swap y verificación cubiertos** |
| Catálogo generado | **Fuente única `mcp.ToolSpec` y check byte a byte disponible** |

## Correcciones realizadas

### Estado documental

- `MEJORAS-PROPUESTAS.md` abre con el estado final y conserva las propuestas detalladas únicamente como diagnóstico histórico.
- `docs/roadmap/README.md` muestra las doce fases terminadas, su evidencia principal y los commits que introdujeron cada capacidad.
- Las fases 4–8 y 10–11 marcan criterios y definición de terminado, registran comandos ejecutados y describen migración y rollback reales.

### Matriz de fallos de restore

`TestRestoreInjectedFailuresLeaveVerifiableRecoveryState` cubre mediante failpoints internos y controlados:

| Frontera | Salida comprobada |
|---|---|
| Creación del rescue | Journal `recovery_required`, backup original y árbol actual conservados |
| Extracción al staging | Original y rescue conservados; árbol actual intacto; nueva mutación bloqueada |
| Persistencia del journal tras crear rescue | El fallo se convierte en estado recuperable en el siguiente write; rescue identificado |
| Verificación posterior | Árbol restaurado verificable y árbol anterior preservado para `Recover` |

Las pruebas ya existentes completan la matriz con fallo de import de base de datos, fallo durante el swap, recuperación explícita y camino exitoso. Los seams son privados al paquete y no exponen failpoints en la API pública.

## Matriz final de las doce fases

| Fase | Resultado | Evidencia principal |
|---:|---|---|
| 1 | Terminada | `internal/upgradeplan`; majors consecutivos y selección numérica |
| 2 | Terminada | `mcp.ToolSpec`; catálogo único y frontera read-only |
| 3 | Terminada | ledger de idempotencia y reconciliación de `unknown` |
| 4 | Terminada | `internal/contracts`, `internal/multiharness`; corpus común |
| 5 | Terminada | `internal/runstate`; persistencia, exclusión por raíz y guard MCP |
| 6 | Terminada | checkpoints y commits ligados a evidencia independiente |
| 7 | Terminada | ejecutor determinista y progreso reanudable |
| 8 | Terminada | inventario y reporte reconstruible tras reinicio |
| 9 | Terminada | planner Composer determinista con explicación de conflictos |
| 10 | Terminada | plan, rescue, journal, failpoints, swap y recuperación |
| 11 | Terminada | redirects, límites, procedencia SHA-256 y paths seguros |
| 12 | Terminada | catálogo MCP generado y detección byte a byte de deriva |

## Verificación de la corrección

```bash
GOCACHE=/tmp/drup-go-build go test ./internal/backup \
  -run 'TestRestoreInjectedFailuresLeaveVerifiableRecoveryState|TestRestoreFailureKeepsOriginalAndRescueWithJournal|TestRestoreFilesystemSwapFailureRollsBackCurrentTree|TestRestoreSuccessCompletesJournalAndPreservesPriorTree' \
  -count=1
GOCACHE=/tmp/drup-go-build go test -count=1 ./...
GOCACHE=/tmp/drup-go-build go generate ./...
GOCACHE=/tmp/drup-go-build go run ./cmd/mcp-catalog-gen --check
git diff --check
```

Resultado: la prueba focalizada y la suite completa terminan en `ok`; los generadores y el check byte a byte del catálogo terminan con código 0; `git diff --check` no produce salida.

## Riesgo residual

La importación de base de datos sigue siendo deliberadamente no atómica. El contrato no promete rollback automático: persiste `recovery_required`, conserva rescue y árbol anterior, bloquea nuevas mutaciones y exige reconciliación operativa explícita.
