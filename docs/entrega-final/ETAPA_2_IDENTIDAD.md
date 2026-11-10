# Etapa 2 — Identidad, autenticación y autorización

Fecha: 9 de noviembre de 2026. Base: `main` en `9bc5c11` (PR #16). Rama:
`feature/entrega-final-etapa-02-identidad`. Plan: [PLAN_ENTREGA_FINAL.md](PLAN_ENTREGA_FINAL.md).

**Criterio de aceptación:** ningún flujo privado opera sin identidad válida y los
permisos de RN-30 se verifican en el dueño del dato.

## 1. Qué ya estaba implementado (CB-03)

La mayor parte del backend de esta etapa se construyó con la ampliación del Club. Se
revisó contra el plan y no requiere cambios:

| Trabajo del plan | Implementación | Prueba |
|---|---|---|
| Usuarios, credenciales seguras y login | members-service: bcrypt 12, mismo error para correo inexistente y contraseña incorrecta, cuota de intentos | `services/members-service/internal/repository/identity_integration_test.go`, E2E CB-03 |
| Emitir y validar JWT | RS256 de 15 minutos, `iss`, `aud`, `sub` y `role`; solo RS256 (ADR-006) | `pkg/access/token_test.go`: rechaza `alg: none`, `HS256`, `RS512`, `jku`, claims y formato inválidos |
| Eliminar cabeceras de identidad externas e inyectarlas desde el token | Gateway borra `X-User-*` y `X-Internal-*` del cliente e inyecta la identidad verificada | `gateway/internal/server/access_test.go`: `X-User-Id` falsificado ignorado |
| Autorización por rol en el borde | Gateway: rutas propias para todos los roles y el resto solo para administración; sin clave pública, falla cerrado | `TestRutasPrivadasYRolesFallanCerrados`: 401 sin token, 403 por rol, doble `Authorization` rechazada |
| Autorización fina en el dueño del dato | Members y Benefits exigen el actor verificado y la clave interna; membresías: solo la propia o administración | Tests de membresías (`ErrForbidden`) y de `/benefits/me` |
| Expiración y token inválido | Token vencido o firmado con otra clave: 401 | `TestCuentaPropiaJWTEliminaIdentidadFalsificada` (token de hace 16 minutos) |

Booking y Training todavía no tienen endpoints de negocio; aplican la misma regla al
implementarse (etapas 4 y 10), según los contratos de [docs/api](../api/README.md).

## 2. Qué agrega esta etapa: login, sesión y rutas protegidas

El frontend era una única página que mezclaba login, activación y la cuenta del Club.
Se reemplazó por una aplicación con sesión y rutas por rol, sin dependencias nuevas:

| Archivo | Responsabilidad |
|---|---|
| `web/src/router.ts` | Ruteo por hash (`#/app`, `#/admin`…); reconoce el enlace de activación `#activate=` |
| `web/src/auth/session.tsx` | Sesión: JWT solo en memoria, vencimiento automático, cierre ante cualquier 401 y logout v1 |
| `web/src/App.tsx` | Tabla de rutas con roles permitidos, redirecciones y barra de navegación por rol |
| `web/src/pages/LoginPage.tsx` | Ingreso y reenvío del enlace de activación |
| `web/src/pages/ActivationPage.tsx` | Primera contraseña o confirmación de vinculación |
| `web/src/pages/StudentHomePage.tsx` | Inicio del alumno: membresía y saldo |
| `web/src/pages/AccountPage.tsx` | Mi cuenta y cierre de sesión (todos los roles) |
| `web/src/pages/InfoPages.tsx` | Acceso denegado, página inexistente e inicios pendientes de otras etapas |

Rutas disponibles al cerrar esta etapa (ver la tabla completa en la
[etapa 1 §2](ETAPA_1_CONTRATOS_RECORRIDOS.md#2-rutas-del-frontend)):

| Ruta | Roles | Contenido |
|---|---|---|
| `#/login`, `#/activar` | Públicas | Ingreso y activación |
| `#/estado` | Pública | Estado del sistema (como `/api/v1/status`); sirve para verificar el stack local con la identidad apagada |
| `#/app` | ALUMNO | Membresía y saldo |
| `#/cuenta` | Todos | Mi cuenta |
| `#/admin` | ADMINISTRADOR | Secciones por venir |
| `#/prof`, `#/nutri` | PROFESOR, NUTRICIONISTA | Secciones por venir, sin simular funcionalidad |

Comportamiento:

- Sin sesión, una ruta privada lleva al login; con sesión, el login lleva al inicio del rol.
- Una ruta de otro rol muestra "acceso denegado". El backend vuelve a autorizar siempre:
  ocultar una ruta no es la protección.
- Al vencer el JWT (15 minutos) o ante un 401, la sesión local se cierra y el login
  informa el motivo. No hay refresh tokens (ADR-006).
- El estado del sistema, que antes era la portada, pasa a `#/estado`. Sigue siendo
  público, igual que `/api/v1/status`, para poder verificar el stack local, donde la
  identidad viene apagada por defecto.

## 3. Decisión tomada en esta etapa (a confirmar por el equipo)

**Ruteo por hash, sin librería.** Era la decisión abierta 4 de la etapa 1; el equipo la aprobó como D-80. Se eligió la
opción que no agrega herramientas, por la regla académica del equipo. Además:

- funciona igual en Nginx, en el servidor de Vite y en Vercel, sin configurar el
  servidor para que devuelva `index.html` en cada ruta;
- conserva el enlace de activación que ya envía Members (`/#activate=<token>`).

El costo es que las direcciones llevan `#`. Si el equipo prefiere una librería de ruteo,
el cambio queda acotado a `router.ts` y `App.tsx`.

## 4. Verificación

| Verificación | Resultado |
|---|---|
| Backend (gateway, `pkg/access`, Members, Benefits) | Correcto en esta rama: los mismos tests de la línea base del 09/11/2026 |
| Revisión del código del frontend | Hecha: tipos explícitos, sin variables sin usar, hooks antes de cualquier retorno |
| **`npm run build` (TypeScript + Vite)** | **No ejecutado: el equipo donde se desarrolló no tiene Node.** Debe correrlo otro integrante antes de integrar |

Prueba manual pendiente, con `make up`, identidad habilitada y Mailpit:

- [ ] `npm --prefix web run build` sin errores.
- [ ] El enlace de activación del correo abre `#/activar`; la contraseña se guarda y el enlace no se puede reutilizar.
- [ ] Login de alumno → `#/app` con membresía y saldo; `#/admin` muestra "acceso denegado".
- [ ] Login de administrador → `#/admin`; `#/estado` muestra el estado del sistema, también sin sesión.
- [ ] Contraseña incorrecta muestra "Correo o contraseña incorrectos".
- [ ] "Salir" vuelve al login; volver atrás no muestra datos privados.
- [ ] A los 15 minutos de ingresar, la aplicación vuelve sola al login con el aviso de sesión vencida.
