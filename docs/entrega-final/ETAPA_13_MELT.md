# Etapa 13 — Integración externa con MELT

**Estado al 10/11/2026:** documentación recibida y analizada; implementación y
verificación contra el proveedor pendientes. El equipo confirmó que este PDF es
toda la información que recibió. Se prepara esta referencia para continuar el
[plan final](PLAN_ENTREGA_FINAL.md) en orden; no se adelanta la implementación.

## 1. Fuente y alcance de esta recepción

- Proveedor: **MELT**.
- Fuente: [Manual_integracion_MELT_Grupo_Gimnasio.pdf](../contracts/providers/melt/Manual_integracion_MELT_Grupo_Gimnasio.pdf), 5 páginas.
- Copia recibida el 10/11/2026, sin número de versión o fecha de emisión explícitos.
- SHA-256: `7e150c2a7790ed4baedfeb1eb8e7a30deed1e4bf873b35fdb23d19c2fd3d6ff9`.
- Base declarada por el proveedor: `https://melt-gateway.onrender.com`.
- Prefijo: `/api/v1`; HTTPS y JSON cuando corresponda.

Este documento resume el manual, no certifica disponibilidad ni compatibilidad
del despliegue actual. No se hicieron llamadas a MELT, altas de usuarios ni envíos
de datos. El manual describe el contrato externo; sus recomendaciones no aprueban
por sí solas cambios en el SPEC, la arquitectura o el alcance de METALFITNESS.

## 2. APIs documentadas

| Método y ruta externa | Autenticación MELT | Información confirmada y página |
|---|---|---|
| `GET /api/v1/tratamientos` | Pública | Listado, búsqueda, filtros y paginación; pp. 1–2 |
| `GET /api/v1/tratamientos/{id}` | Pública | Detalle por ID de texto; 200, 400, 404 y 503; p. 2 |
| `POST /api/v1/auth/registro` | Pública | Alta de cliente con referido opcional; 201 y estado del beneficio; pp. 2–3 |
| `POST /api/v1/auth/login` | Credenciales personales MELT | 200 con token Bearer, expiración y usuario; nombres exactos de campos no especificados; p. 3 |
| `GET /api/v1/beneficios/mis-puntos` | Bearer MELT, rol `cliente` | Saldo de la identidad MELT autenticada; sin puntos: cero y movimientos vacíos; p. 3 |

No se documentan operaciones administrativas que debamos consumir. Los IDs de
tratamientos se conservan como strings, exactamente como llegan. `TRAT-001` es
un ejemplo ilustrativo, no un dato confirmado del catálogo real.

### Catálogo: parámetros y respuesta (pp. 1–2)

Todos los parámetros son opcionales y deben codificarse en la URL.

| Parámetro | Tipo | Valores, límites o default documentados |
|---|---|---|
| `q` | string | Coincidencias en nombre y keywords |
| `categoria` | string | `Facial`, `Corporal`, `Depilación`, `Manos y uñas` |
| `precioMax` | number | Mayor o igual a cero |
| `page` | integer | Default 1; no se precisa tratamiento de valores inválidos |
| `limit` | integer | Default 10; máximo 50 |
| `sort` | string | `nombre` o `precio`; default `nombre` |
| `order` | string | `asc` o `desc`; default `asc` |

El ejemplo de listado contiene `tratamientos` y `pagination`, con `page`, `limit`,
`total` y `totalPages`. Cada tratamiento del ejemplo contiene `id`, `nombre`,
`descripcion`, `categoria`, `precio`, `moneda`, `duracionMinutos`, `imagenes`,
`keywords` y `profesionalesIds`. El ejemplo usa ARS, pero no establece una regla
universal de moneda, campos obligatorios o nulabilidad. Un listado vacío es válido.
El esquema exacto de detalle no está explicitado: no asumir que sea idéntico al listado.

### Registro referido y beneficio (pp. 2–3)

El cuerpo documentado contiene `nombre`, `email`, `password` y `codigoReferido`.
El alta pública crea un usuario MELT con rol `cliente`; no permite elegir roles.
No se precisan validaciones completas de longitud o complejidad de contraseña.

`CODIGO_A_COORDINAR` es un placeholder: **el PDF no entrega el código definitivo**.
Debe enviarse el valor acordado sin alteraciones. Si falta o no coincide, el
beneficio no aplica. La configuración descrita otorga **100 puntos por alta nueva
en MELT con código válido**, sin conversión a pesos ni aplicación retroactiva.

La respuesta 201 incluye el usuario y `beneficioRegistro`:

| Valor | Tratamiento descrito por MELT |
|---|---|
| `acreditado` | Beneficio acreditado |
| `pendiente_reintento_en_login` | Cuenta creada; Auth de MELT reintenta el beneficio al iniciar sesión; no volver a registrar |
| `no_aplica` | No correspondía el beneficio, por ejemplo por código ausente o inválido |

MELT declara que la acreditación es idempotente, pero no documenta una
`Idempotency-Key` ni una garantía de replay del POST de registro. Un timeout no
autoriza repetir automáticamente la creación de cuenta. Un 409 puede indicar
email ya existente; no equivale a demostrar si se acreditó el beneficio.

Los 100 puntos de este flujo pertenecen al sistema de MELT según el manual. No
hay un contrato de callback que ordene acreditarlos en el ledger del gimnasio.
No reemplazan los 500 puntos por asistencia del gimnasio, no se suman a ellos por
suposición y no se equiparan las cuentas ni las membresías de ambos sistemas.
Si se pretende un efecto en nuestra cuenta, se deberá coordinar explícitamente.

### Login y puntos MELT (p. 3)

Login recibe `email` y `password`. Para consultar puntos se usa
`Authorization: Bearer <token>` emitido por MELT para un cliente. El JWT de
METALFITNESS no se supone válido en MELT. No se reciben credenciales reales en el PDF.
El manual no define refresh, duración numérica del token ni el JSON completo de
login o saldo; no inventar campos para construir un cliente o un mock.

## 3. Errores, disponibilidad y coordinación

El manual usa el HTTP status y el campo externo `error`; los cuerpos pueden
variar por endpoint. No afirma que cumplan RFC 7807. Documenta (pp. 3–4):

| HTTP | Significado habitual indicado por MELT |
|---|---|
| 200 / 201 | Consulta/login/replay exitoso o recurso creado, según endpoint |
| 400 | JSON, parámetros o campos inválidos |
| 401 | Token ausente/inválido o credenciales inválidas |
| 403 | Permisos insuficientes o código no autorizado |
| 404 | Tratamiento o ruta inexistente |
| 409 | Conflicto, por ejemplo email duplicado |
| 422 | Plan/membresía sin configuración, si aplica a otro registro externo |
| 500 / 503 | Error interno o indisponibilidad temporal |

Esta tabla es general: no implica que cada endpoint devuelva todos los códigos.
El 422 no basta para inferir un endpoint privado, cuerpo de membresía ni flujo
MELT → gimnasio que el manual no especifica.

MELT advierte que Render puede demorar el primer acceso si el servicio estaba
inactivo (p. 5). Recomienda espera creciente y reintentos limitados ante 503; no
fija timeouts, cantidad de intentos, cuotas ni SLA. Esos parámetros y la
degradación visible del gimnasio se definirán según el flujo elegido.

La p. 4 distingue dos coordinaciones:

- El origen HTTPS del frontend se solicita para CORS **si** hay llamadas directas
  desde navegador. El manual contempla esa alternativa, pero la arquitectura del
  gimnasio exige frontend → gateway propio → servicio consumidor → gateway MELT
  mediante puerto/adaptador (ADR-009 propuesto). No se adopta acceso directo desde
  la SPA por leer el PDF. CORS no reemplaza autenticación.
- Si MELT necesita invocar una API nuestra, se deben acordar URL backend HTTPS,
  método, ruta, JSON, autenticación, headers, respuestas, errores y reintentos.
  El PDF describe qué intercambiar, **no especifica esa llamada**. Nuestro
  contrato v2 existente no queda automáticamente acordado por este manual.

## 4. Pendientes para implementar la etapa 13

| Pendiente | Impacto |
|---|---|
| Código de referido definitivo y confirmación de los 100 puntos vigentes | Impide demostrar el beneficio referido; no impide una futura consulta pública del catálogo |
| Flujo de negocio, actor, pantalla y efecto observable en METALFITNESS | Un catálogo aislado no demuestra por sí solo la integración relevante requerida por SPEC §1.5 |
| Servicio consumidor, HU/RN, puerto y modelo propio | Completar ADR-009 antes de implementar; esta recepción no los decide |
| Esquemas completos y errores representativos de detalle, registro, login y saldo | El PDF da ejemplos y descripciones parciales; falta validar compatibilidad |
| Semántica ante resultado indeterminado del registro | Evitar duplicación de altas y beneficios; no asumir replay del POST |
| Timeouts, reintentos, límites, costos y disponibilidad | Justificar con el flujo y pruebas; no inventar valores atribuidos al proveedor |
| Necesidad real de login MELT y manejo de sus credenciales/sesión | El catálogo no requiere token; no incorporar captura de contraseñas por defecto |
| Llamada inversa MELT → gimnasio, si se necesita | Acordar explícitamente contrato y efecto; no inventar callback ni secreto |
| CORS, si se propone cambiar a consumo directo | Evaluar primero la arquitectura propia; no es requisito de una llamada servidor a servidor |

No se pide al equipo un segundo documento que no recibió. Esta lista conserva
lo que el PDF deja abierto para coordinar cuando se implemente la etapa.

## 5. Verificación prevista, todavía no ejecutada

1. Confirmar contrato vigente y disponibilidad por el gateway público de MELT;
   registrar versión recibida, entorno y fecha de la prueba.
2. Probar listado, filtros combinados, acentos/URL encoding, paginación, orden,
   listado vacío, campos adicionales y preservación de IDs de texto.
3. Probar detalle existente, ID inválido, inexistente, timeout, 500 y 503; la UI
   debe distinguir vacío de indisponibilidad y traducir errores al contrato propio.
4. Si se aprueba el flujo referido: registrar un usuario de prueba coordinado con
   código real, observar los tres estados de `beneficioRegistro` y no registrar
   otra vez ante beneficio pendiente. Validar email duplicado y resultado ambiguo.
5. Si se necesita sesión MELT: probar login, rechazo de credenciales, expiración,
   consulta de puntos propios y cuenta sin puntos. No reutilizar el JWT local.
6. Demostrar idempotencia del beneficio y recuperación sin altas ni acreditaciones
   duplicadas; los mocks se derivan solo de partes conocidas del contrato.
7. Integrar el flujo aprobado desde el frontend a través del servicio consumidor,
   con degradación visible y sin alterar reservas, membresías o puntos locales
   por una falla externa. Completar ADR-009, trazas y evidencia de D9.

No se agregan endpoints, variables de entorno, secretos, dependencias ni reglas
de negocio con esta preparación documental. Continúa el trabajo por etapas desde
la 6; la 13 sigue pendiente de implementación.
