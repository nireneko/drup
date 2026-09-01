# Hoja de ruta de mejoras completadas

Las doce fases están implementadas. Este índice enlaza el diseño histórico de cada solución y la evidencia que permite verificar su estado final.

## Estado final

| Fase | Solución | Estado | Evidencia principal | Commit inicial de implementación |
|---:|---|---|---|---|
| 1 | [Planificador numérico y parametrizado de versiones mayores](planificador-majors_fase1.md) | Terminada | `internal/upgradeplan`; pruebas de saltos consecutivos y selección numérica | `a314ac3` |
| 2 | [Descriptor único y frontera read-only](descriptor-y-frontera-readonly_fase2.md) | Terminada | `mcp.ToolSpec`; pruebas de catálogo único y análisis sin mutación | `b55b55d` |
| 3 | [Idempotencia y resultado `unknown`](idempotencia-y-resultado-unknown_fase3.md) | Terminada | ledger persistente; deduplicación y reconciliación observable | `f9026a6` |
| 4 | [Contratos mínimos y escenarios multiagente](contratos-multiagente-minimos_fase4.md) | Terminada | `internal/contracts`, `internal/multiharness`; corpus común para tres plataformas | `cd454d1` |
| 5 | [Estado persistente y autoridad de transiciones](runstate-persistente_fase5.md) | Terminada | `internal/runstate`; reinicio, exclusión por raíz y guard MCP | `228d4d1`, `2a334f8` |
| 6 | [Commits y checkpoints autorizados por evidencia](commits-por-evidencia_fase6.md) | Terminada | `internal/app/checkpoint.go`, `internal/gitops`; paridad CLI/MCP | `7d3fee2`, `655a076` |
| 7 | [Ejecutor determinista de checkpoints](executor-checkpoints_fase7.md) | Terminada | `internal/app/checkpoint_executor.go`; planes y progreso persistente | `6acd8e8` |
| 8 | [Inventario y reporte reconstruible](inventario-y-reporte_fase8.md) | Terminada | `internal/inventory`, `report.BuildFromRun`; snapshot estable tras reinicio | `b361307` |
| 9 | [Planificador Composer para contrib](planner-contrib-composer_fase9.md) | Terminada | `internal/contribplan`; conflictos explicables y grupos deterministas | `62024a8` |
| 10 | [Restauración transaccional y ensayable](restore-transaccional_fase10.md) | Terminada | `internal/backup/restore.go`; journal, rescue, failpoints, recuperación e interlock | `52e7eb2`–`0107b6a` |
| 11 | [Cadena de suministro verificable para patches](supply-chain-patches_fase11.md) | Terminada | redirects acotados, límite de cuerpo, SHA-256 y validación de paths | `1d0de84` |
| 12 | [Catálogo MCP generado desde contratos](catalogo-mcp-generado_fase12.md) | Terminada | catálogo JSON/Markdown determinista y check byte a byte | `54306b9` |

## Dependencias implementadas

- Las fases 1–4 fijan reglas de decisión, efectos, idempotencia y contratos mínimos.
- Las fases 5–6 convierten esas reglas en autoridad persistente y publicación autorizada.
- Las fases 7–9 aportan ejecución, trazabilidad y planificación de contrib.
- Las fases 10–12 endurecen recuperación, supply chain y coherencia documental.

Para verificar el cierre completo se ejecuta `go test -count=1 ./...`, seguido de `go generate ./...`, `go run ./cmd/mcp-catalog-gen --check` y `git diff --check`.
