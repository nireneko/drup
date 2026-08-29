# Fase 11: Cadena de suministro verificable para patches

> **Decisión:** completar esta solución como un work unit verificable antes de avanzar a fase 12.

## Estado verificado

Pendiente. La URL inicial valida HTTPS/host y hay pruebas anti-bypass; redirecciones usan todavía política por defecto, el cuerpo no tiene límite y Composer registra URL sin identidad criptográfica del contenido.

No se debe reimplementar ninguna capacidad indicada como existente: debe reutilizarse y probarse en integración. El estado se basa en `MEJORAS-PROPUESTAS.md`, el árbol actual y los cambios locales visibles; no presupone que esos cambios estén entregados.

## Problema y objetivo

Verificar cada salto y conservar procedencia de los bytes incorporados al proyecto.

## Alcance y no alcance

**Incluye:** el comportamiento descrito, sus contratos, persistencia necesaria y pruebas. **No incluye:** reimplementar sesión canónica, kill switch, auditoría, backup con manifiesto, ejecución acotada ni staging seguro ya existentes; tampoco autoriza commits, releases o migraciones de datos fuera de la fase.

## Dependencias

- Requiere que las fases anteriores estén integradas y sus contratos sean estables.
- Debe reutilizar `session`, `audit`, `backup`, `exec` y `gitops` cuando corresponda.
- La fase siguiente no debe empezar hasta satisfacer la definición de terminado.

## Arquitectura propuesta

### Componentes y responsabilidades

- Cliente HTTP con `CheckRedirect`, límite de saltos y la misma allowlist.
- Descarga acotada por Content-Length + LimitReader.
- PatchEvidence: SHA-256, tamaño, URLs inicial/final, fecha e issue.
- Preflight `git apply --check` y validación de paths/tipos.

### Contratos y flujo

1. Validar identidad de raíz, run/fase y entrada tipada antes de cualquier efecto.
2. Ejecutar el servicio de dominio sin shell ni autoridad derivada del prompt.
3. Capturar resultado y evidencia sanitizada ligada al mismo candidato.
4. Persistir de forma atómica antes de exponer la siguiente acción permitida.
5. Ante resultado ambiguo, bloquear continuación ciega y ofrecer recuperación explícita.

## Plan de implementación por work units

1. WU1: pruebas RED de redirect host/scheme/loop y cuerpo sobredimensionado.
2. WU2: cliente/descarga limitada con limpieza garantizada.
3. WU3: evidencia criptográfica persistida en run/report.
4. WU4: validar paths dentro del paquete y cambios sensibles con confirmación.


Cada work unit incluye comportamiento, pruebas y documentación asociada; debe poder revisarse y revertirse sin retirar unidades ajenas. Si el cambio supera 400 líneas authored, dividirlo en PRs encadenadas por estas fronteras, no por tipo de archivo.

## Estrategia Strict TDD

1. **RED:** añadir primero la prueba enfocada de la invariante o fallo observable; registrar que falla por la razón esperada.
2. **GREEN:** implementar el mínimo comportamiento y ejecutar el comando enfocado.
3. **REFACTOR:** eliminar duplicación, conservar contratos y repetir pruebas enfocadas.
4. Ejecutar integración y suite completa solo después de GREEN; no declarar verde sin salida real.

## Criterios de aceptación

- [ ] Redirect a host no permitido se rechaza antes del body.
- [ ] Descarga grande se corta y elimina temporal.
- [ ] Bytes exactos y origen quedan auditables.
- [ ] Patch no escribe fuera del paquete declarado.


## Verificación prevista

```bash
go test ./internal/patch ./internal/patchreconcile ./internal/app
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
