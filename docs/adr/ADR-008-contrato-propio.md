# ADR-008 — Contrato propio: API de fidelización del Club de Beneficios (v1)

| Campo | Valor |
|---|---|
| **Estado** | Aceptado |
| **Fecha** | 2026-10-07 |
| **Decisión del TP** | D8 |
| **Autores** | Pastore, Martina · Schaffer, Matías · Viti, María Guadalupe |
| **Relacionado con** | SPEC HU-10, HU-29 a HU-32, RN-22 a RN-27; [ADR-001](ADR-001-limites-de-servicios.md); [ADR-005](ADR-005-comunicacion.md); contrato [benefits-api.v1.yaml](../contracts/benefits-api.v1.yaml); guía [docs/contracts/README.md](../contracts/README.md) |

## Contexto

El TP pide publicar una **capacidad propia** que otros grupos puedan integrar en el flujo principal de su sistema **sin explicaciones privadas**: el contrato tiene que alcanzar para entender qué hace cada operación, cómo autenticarse, cómo reintentar y cómo interpretar cada error.

Fuerzas en juego:

- Los consumidores son equipos con stacks distintos y sin acceso a nuestro código.
- Las operaciones mueven puntos: un reintento por timeout **no** puede acreditar dos veces (RN-27).
- El contrato se publica **antes** de implementar el servicio, para que los otros grupos avancen en paralelo con un mock.
- Hay que poder evolucionar la API sin romper a quienes ya la integraron.

### Por qué esta capacidad

| Candidata | ¿Sirve a otros sistemas? | Decisión |
|---|---|---|
| **Club de Beneficios (puntos)** | Sí: cualquier negocio puede premiar compras, turnos o visitas con puntos y dejar que se usen como descuento o canje. Es genérica y autocontenida. | **Elegida** |
| Reservas de clases | Poco: depende de la agenda, cupos y membresías propias del gimnasio. | Descartada |
| Planes y mediciones | Datos de salud de alumnos; exponerlos a terceros es sensible y de poco valor para otros dominios. | Descartada |
| Membresías | Muy específica del gimnasio. | Descartada |

Además, el SPEC ya define el rol de **partner** (HU-29 a HU-32) y benefits-service es el dueño único del ledger (ADR-001), así que la API no exige coordinar con otros servicios.

## Alternativas consideradas

### Alternativa A — REST + OpenAPI 3.1 (elegida)

| Pros | Contras |
|---|---|
| Lo puede consumir cualquier lenguaje con un cliente HTTP | El cliente debe implementar reintentos e idempotencia (lo guiamos en la documentación) |
| OpenAPI es legible por personas y por herramientas: mock (Prism), validación (Spectral), generación de clientes | Menos eficiente que un protocolo binario (irrelevante para este volumen) |
| Errores estándar con RFC 7807 | |

### Alternativa B — gRPC + Protocol Buffers

| Pros | Contras |
|---|---|
| Contrato tipado, eficiente | Exige tooling específico y HTTP/2 extremo a extremo; más fricción para otros grupos |
| | No se prueba con un simple `curl` |

### Alternativa C — GraphQL

| Pros | Contras |
|---|---|
| El cliente elige los campos | Las operaciones son comandos simples; la flexibilidad no aporta |
| | Idempotencia, caché y rate limiting más difíciles de expresar |

### Alternativa D — Integración asíncrona (eventos o webhooks)

| Pros | Contras |
|---|---|
| Desacople temporal | El partner necesita la respuesta en el momento (¿alcanzó el saldo?) |
| | Exige que cada grupo opere un broker o un endpoint receptor |

## Decisión

Publicamos la capacidad como **API REST descrita en OpenAPI 3.1**: [docs/contracts/benefits-api.v1.yaml](../contracts/benefits-api.v1.yaml), versión **1.0.0**.

### Diseño

| Aspecto | Decisión |
|---|---|
| Recursos | Todo cuelga de la cuenta: `/v1/accounts/{externalUserId}/credits`, `/debits`, `/balance`, `/movements`, `/redemptions`; el catálogo en `/v1/benefits`. Los comandos que crean un movimiento son `POST` sobre una colección (sustantivos en plural) y devuelven `201`. |
| Identidad de la cuenta | Par **(partner, `externalUserId`)**. El partner usa su propio identificador de usuario y nunca ve IDs internos. El partner sale de la API key, no de la URL. |
| Vinculación | Solo opera sobre cuentas de alumnos que el administrador vinculó a ese partner (SPEC S-11, HU-10). Otra cuenta → `404 ACCOUNT_NOT_FOUND`, sin revelar si existe para otro partner (RN-26). |
| Visibilidad | `balance` devuelve el saldo total; `movements` solo los movimientos originados por ese partner (RN-26). |
| Autenticación | Header `X-API-Key`, una clave por partner, guardada como hash y revocable. |
| Idioma | Rutas, campos y códigos en **inglés** por ser un contrato para terceros. Es una excepción documentada a la convención interna (dominio en español). |
| Campos JSON | `camelCase` (excepción a la convención interna `snake_case`, por el mismo motivo). |
| Errores | RFC 7807 con `code` estable: `INVALID_REQUEST`, `UNAUTHORIZED`, `FORBIDDEN`, `ACCOUNT_NOT_FOUND`, `BENEFIT_NOT_FOUND`, `IDEMPOTENCY_CONFLICT`, `IDEMPOTENCY_IN_PROGRESS`, `INSUFFICIENT_BALANCE`, `BENEFIT_NOT_AVAILABLE`, `RATE_LIMITED`, `INTERNAL`, `UNAVAILABLE`. La guía indica cuáles se reintentan. |
| Límites | 20 req/s por API key (ráfaga 40), 1 a 1.000.000 puntos por operación (límite técnico de validación, no tope de negocio), páginas de hasta 100. |

### Idempotencia

- `Idempotency-Key` **obligatorio** en `credits`, `debits` y `redemptions`.
- Misma key + mismo body → misma respuesta original (status y body), header `Idempotent-Replayed: true`, sin nuevo efecto.
- Misma key + body distinto → `409 IDEMPOTENCY_CONFLICT`. Misma key en curso → `409 IDEMPOTENCY_IN_PROGRESS`.
- Se guardan también las respuestas de error: reintentar un rechazo con la misma key devuelve el mismo rechazo.
- Unicidad **por partner**; retención **30 días** (RN-27, D-08).
- Implementación prevista: tabla `operaciones_partner` con `UNIQUE (partner_id, idempotency_key)`, hash del body y respuesta guardada, escrita en la misma transacción que el movimiento (ADR-003).

### Publicación

| Qué | Dónde |
|---|---|
| Contrato | `docs/contracts/benefits-api.v1.yaml` en este repositorio |
| Guía para consumidores | `docs/contracts/README.md` |
| Mock ejecutable | Prism (`stoplight/prism`) con `deploy/docker-compose.mock.yml`, en `http://localhost:4010` |
| Producción | `https://<host>/partner-api/v1/...` a través del api-gateway (placeholder hasta elegir proveedor, D-23) |

### Compatibilidad y versionado

- **Semantic Versioning** en `info.version`; la versión **mayor** va en la ruta (`/v1`).
- Cambios compatibles (endpoints nuevos, campos opcionales nuevos, campos nuevos en respuestas, códigos de error nuevos, valores de enum nuevos) → versión menor dentro de `/v1`.
- Cambios incompatibles (quitar/renombrar, volver obligatorio, cambiar tipos o semántica, achicar límites) → `/v2`, conviviendo con `/v1`.
- Deprecación con aviso: `/v1` se mantiene al menos 60 días tras publicar `/v2` (o hasta el fin de la cursada), con headers `Deprecation` y `Sunset` y anuncio en el CHANGELOG.
- Pedimos a los consumidores ignorar campos y valores de enum desconocidos (lector tolerante).

## Consecuencias

### Positivas

- Los otros grupos pueden integrarse hoy contra el mock, sin esperar la implementación.
- Reintentos seguros de punta a punta: un timeout nunca duplica puntos.
- El contrato es la fuente de verdad: valida el mock, validará al servicio real y sirve para generar clientes.
- El partner no depende de identificadores internos del gimnasio.

### Negativas

- Dos convenciones conviven: interna (español, `snake_case`) y pública (inglés, `camelCase`). benefits-service debe mapear entre ambas en su adaptador HTTP de partners.
- La vinculación previa de usuarios exige coordinación manual con cada partner.
- Guardar respuestas de idempotencia 30 días ocupa almacenamiento.
- El mock no tiene estado: no sirve para probar flujos (el saldo no cambia entre llamadas).

### Riesgos aceptados

| Riesgo | Probabilidad | Impacto | Mitigación |
|---|---|---|---|
| El servicio real se desvía del contrato | Media | Alto | Validación de respuestas reales contra el contrato (Prism en modo proxy) en los tests de integración |
| Un partner reutiliza keys entre operaciones distintas | Media | Medio | `409 IDEMPOTENCY_CONFLICT` + guía explícita |
| Filtración de una API key | Baja | Alto | Hash en base, revocación inmediata, rate limit por key |
| Cambios de catálogo rompen canjes ya ofrecidos por el partner | Media | Bajo | `BENEFIT_NOT_AVAILABLE` y recomendación de leer el catálogo antes de ofrecer un canje |

## Cómo se verificará

- **Lint del contrato:** Spectral con [docs/contracts/.spectral.yaml](../contracts/.spectral.yaml) (OpenAPI válido, ejemplos válidos contra esquemas, `Idempotency-Key` en todo POST, errores `problem+json`, `Retry-After` en 429/503, rutas versionadas) en CI para todo PR que toque `docs/contracts/`. Umbral: 0 errores y 0 advertencias.
- **Mock:** el job de CI levanta Prism y ejecuta los ejemplos curl de la guía; todos deben responder con el status documentado.
- **Conformidad del servicio real** (cuando exista): tests de integración que pasan por Prism en modo `proxy` con validación de respuestas; cualquier desvío falla el test.
- **Idempotencia:** test que envía la misma key 10 veces en paralelo → 1 movimiento y 10 respuestas iguales; misma key con otro body → 409.
- **Compatibilidad:** en CI, comparación del contrato contra la última versión publicada (`oasdiff breaking`); un cambio incompatible dentro de `/v1` falla el build.
- **Usabilidad:** al menos un grupo externo integra crédito, saldo y débito usando solo el contrato y la guía.
