# Hoja de ruta de mejoras pendientes

> Orden de implementación basado en dependencias verificadas. Cada fase enlaza una solución; el índice no sustituye sus criterios de terminado.

## Cómo usarla

1. Empezar por la primera fase no terminada.
2. Implementar por work units con Strict TDD.
3. No avanzar hasta cumplir la definición de terminado y conservar evidencia.

## Fases

| Fase | Solución | Estado de partida |
|---:|---|---|
| 1 | [Planificador numérico y parametrizado de versiones mayores](planificador-majors_fase1.md) | Parcial |
| 2 | [Descriptor único y separación real entre análisis y mutación](descriptor-y-frontera-readonly_fase2.md) | Parcial |
| 3 | [Idempotencia de mutaciones y resultado `unknown`](idempotencia-y-resultado-unknown_fase3.md) | Parcial |
| 4 | [Contratos mínimos y escenarios multiagente](contratos-multiagente-minimos_fase4.md) | Parcial |
| 5 | [Estado persistente y autoridad de transiciones](runstate-persistente_fase5.md) | Pendiente |
| 6 | [Commits y checkpoints autorizados por evidencia](commits-por-evidencia_fase6.md) | Pendiente |
| 7 | [Ejecutor determinista de checkpoints operativos](executor-checkpoints_fase7.md) | Pendiente |
| 8 | [Inventario inicial y reporte antes/después reconstruible](inventario-y-reporte_fase8.md) | Pendiente |
| 9 | [Planificador Composer para contrib](planner-contrib-composer_fase9.md) | Pendiente |
| 10 | [Restauración transaccional y ensayable](restore-transaccional_fase10.md) | Pendiente |
| 11 | [Cadena de suministro verificable para patches](supply-chain-patches_fase11.md) | Pendiente |
| 12 | [Catálogo MCP y documentación generados desde contratos](catalogo-mcp-generado_fase12.md) | Pendiente |

## Dependencias principales

- Fases 1–4 cierran reglas de decisión, efectos, idempotencia y contratos mínimos.
- Fases 5–6 convierten esas reglas en autoridad persistente y commits por evidencia.
- Fases 7–9 automatizan checkpoints, trazabilidad y planificación de contrib.
- Fases 10–12 endurecen recuperación, supply chain y eliminación de deriva documental.

El estado “parcial” NO implica que los cambios locales estén entregados ni verificados; cada documento enumera lo que falta.
