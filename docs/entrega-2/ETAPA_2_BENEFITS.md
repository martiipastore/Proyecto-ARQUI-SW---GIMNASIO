# Etapa 2 — Dominio y persistencia de Benefits

## Alcance implementado

Casos de uso `Acreditar`, `ConsultarSaldo` y `ListarMovimientos`, con puertos de entrada y salida, dominio sin infraestructura, adaptador PostgreSQL, migración SQL versionada y pruebas. Corresponden a HU-29/HU-32, RN-22/RN-26/RN-27/RN-42 y CL-08/08b/08c. La API HTTP y el cableado hacia el gateway son la etapa 3; el proceso actual sigue exponiendo salud, no estos casos de uso.

La migración inicial admite créditos de partner. Débitos, asistencia, penalizaciones, canjes y reversiones requerirán ampliaciones versionadas de esquema y dominio; no se presentan como implementados. D-52 sigue definiendo la suma de originales y compensatorios cuando se incorpore la reversión.

## Fundamento y aplicación de las decisiones aprobadas

| Decisión | Aplicación | Justificación |
|---|---|---|
| D1/D3 | Solo `benefits_db`, alumno representado por ID; partner/externalUserId resuelve una cuenta interna | Propiedad de datos por servicio y transacciones locales, teórico pp. 20–30. Ninguna FK ni consulta a otra base |
| D2 | `domain` con valores privados sin tags; `application` orquesta; `ports` expresa necesidades; adaptadores traducen SQL/JSON | Puertos y adaptadores, pp. 120–124. La autorización la decide aplicación a partir de los hechos que entrega el adaptador |
| D4/RN-22/42 | Bloqueo de la cuenta y secuencia creciente, actualización del saldo e inserción del movimiento en una transacción | Invariante local del agregado; pp. 27–28 y 120. Los movimientos de esta etapa no admiten UPDATE/DELETE |
| D7 | Saldo, partner, vinculación e idempotencia siempre en PostgreSQL | No usar copias obsoletas para permisos ni datos críticos |
| D-43/D10 | Presupuesto de 4 s por caso de uso, limitado también por el contexto del llamador | Presupuestos y cancelación, pp. 101–105; 4 s es valor aprobado del proyecto |
| RN-27 | Unicidad por partner/clave y resultado persistido en la misma transacción | Idempotencia y recuperación de resultado incierto, pp. 103–105 y 113–115 |

No se incorporaron frameworks o dependencias nuevas. pgx, testify y Testcontainers ya existían en el módulo. Testcontainers es una herramienta auxiliar adicional justificada para probar contra PostgreSQL real; no se atribuye su configuración al teórico.

## Transacción de acreditación

1. Validar la identidad de partner suministrada por el futuro adaptador autenticado y el formato de la clave.
2. Consultar y bloquear de forma compartida el estado del partner: una suspensión concurrente espera a la operación en curso; la siguiente solicitud ve el estado actualizado.
3. Intentar reservar `(partner_id, clave)` mediante unicidad SQL. Si existe una transacción concurrente con esa clave, PostgreSQL serializa la inserción dentro del presupuesto del caso de uso.
4. Comparar la huella: incluye operación, externalUserId, puntos, motivo y referencia. Cambiar cuenta con la misma clave también es conflicto. Es una representación canónica de los datos parseados, independiente del orden de propiedades del JSON de transporte.
5. Si existe resultado, recuperarlo sin recalcular el saldo histórico. Un éxito previo también comprueba que la vinculación siga vigente. Un rechazo registrado se reproduce aunque luego se cree la vinculación.
6. Para una operación nueva, validar los datos, resolver la vinculación y bloquear la cuenta. El dominio calcula nuevo saldo, movimiento y secuencia.
7. Insertar movimiento, actualizar cuenta y guardar el resultado de negocio antes del commit. Los rechazos de negocio también se confirman como resultados idempotentes. Los errores técnicos revierten la transacción y no se presentan como rechazos definitivos.

Los formatos y status HTTP, errores de autenticación y validación del JSON bruto, `Idempotent-Replayed` y el tratamiento de `IDEMPOTENCY_IN_PROGRESS` se completan en etapa 3. El resultado durable de esta etapa conserva movimiento, importe, fechas, saldo original y motivo estable de rechazo; no se afirma que ya se persistan respuestas HTTP completas.

## Restricciones de PostgreSQL

`001_ledger.sql` crea partners, cuentas, vinculaciones, movimientos e idempotencia. Tiene FK locales, unicidad, límites del contrato e índices para cuenta/partner/fecha/paginación. La API key se representa por su hash de 32 bytes; emitir y autenticar claves queda pendiente.

Las restricciones diferidas comprueban al confirmar que saldo y secuencia coinciden con el ledger y que ninguna operación quede incompleta. Los triggers impiden modificar/borrar movimientos, reemplazar resultados originales o eliminar claves antes de los 30 días mínimos. Son mecanismos concretos de SQL para sostener invariantes ya aprobadas, no patrones nuevos atribuidos al apunte.

El control diferido de saldo vuelve a sumar el historial de la cuenta: es una protección inicial explícita y puede ser costosa con historiales largos. Su impacto debe medirse en etapa 7 antes de optimizarla; no se declara resuelto el rendimiento. La cuenta serializa escrituras conforme al ADR-004 v2.

La retención no tiene todavía un job de limpieza: conservar más de 30 días satisface el mínimo. Cambios de políticas o limpieza se implementarán por separado. El dueño de la base conserva privilegios administrativos sobre sus tablas; los triggers no son defensa contra un administrador que modifique el esquema.

## Migraciones

Desde `services/benefits-service`, con `BENEFITS_DB_DSN` cargada de forma segura en el entorno:

```powershell
go run ./cmd/migrate
```

El comando es nuevo en esta etapa. Aplica SQL embebido, registra nombre y SHA-256 en `schema_migrations` y rechaza editar una migración ya aplicada. Versiones y SQL se guardan transaccionalmente. Ejecutar una única instancia del migrador al inicializar una base vacía; no forma parte del arranque HTTP ni se agregan endpoints administrativos ocultos. El bloqueo de la tabla de versiones serializa ejecuciones cuando esa tabla ya existe.

Los datos de prueba se crean en fixtures dentro de un PostgreSQL efímero, con claves sintéticas guardadas como hash. No se agregaron credenciales reales ni datos de alumnos al repositorio. El alta funcional de cuentas y vinculaciones sigue siendo responsabilidad de los flujos administrativos/eventos previstos, no de los partners.

## Evidencia y reproducción

Desde el módulo Benefits:

```powershell
go test -count=1 ./...
go vet ./...
```

Requiere Docker para la evidencia de integración; un skip no acredita persistencia. Las pruebas incluyen base vacía y migración repetida; cuenta y movimientos inmutables; créditos y replay desde otro pool; conflicto de payload y de cuenta; respuesta original de rechazo; saldo total e historial por partner; paginación; 12 créditos simultáneos con una clave y con claves distintas; payloads en conflicto concurrentes; rollback después del movimiento y antes de completar idempotencia; restricciones/retención SQL y usuario sin privilegios globales sin acceso a otra base.

Validación final del 16/10/2026: `go test -count=1 ./...` y `go vet ./...` terminaron con código 0 en el módulo Benefits. Pasaron dominio, aplicación, tests de arquitectura, handlers existentes e integración PostgreSQL real (33,235 s para el paquete de persistencia). Las integraciones se ejecutaron, no se omitieron. `go test -race` no está disponible con el compilador C actual de Windows: `cc1.exe: sorry, unimplemented: 64-bit mode not compiled in`. Esa limitación no reemplaza ni invalida la prueba concurrente real, pero no se presenta como detección de carreras ejecutada con éxito.

## Continuación en etapa 3

La etapa 3 agrega el adaptador de entrada HTTP, autenticación y un adaptador de salida que serializa el resultado para guardar sus bytes exactos en la transacción. El arranque de la API aplica las migraciones y D-53 agrega fecha de creación de cuenta. La serialización HTTP ya no se reconstruye solamente desde el resultado de negocio. Ver [ETAPA_3_API_BENEFITS.md](ETAPA_3_API_BENEFITS.md).
