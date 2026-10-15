# Cómo contribuir

Este repositorio usa **GitHub Flow**: `main` siempre está en estado entregable y todo cambio entra por Pull Request.

## Ramas

| Rama | Uso |
|---|---|
| `main` | Protegida. Sin pushes directos. Siempre compila y pasa CI. |
| `feature/etapa-NN-nombre` | Trabajo de una etapa o funcionalidad. `NN` con dos dígitos. |

Ejemplos: `feature/etapa-02-arquitectura`, `feature/etapa-03-members-service`, `feature/etapa-04-reservas-concurrencia`.

Para correcciones puntuales fuera de una etapa: `fix/descripcion-corta`. Para documentación sola: `docs/descripcion-corta`.

### Protección de `main` (configurar en GitHub → Settings → Branches)

- Require a pull request before merging, con **1 aprobación** de otro integrante.
- Dismiss stale approvals when new commits are pushed.
- Require status checks to pass (workflows de CI).
- Require branches to be up to date before merging.
- Require conversation resolution before merging.
- Sin force push ni borrado.

## Flujo de trabajo

1. Actualizá `main`: `git switch main && git pull`.
2. Creá la rama: `git switch -c feature/etapa-NN-nombre`.
3. Commiteá en pasos chicos con **Conventional Commits**.
4. Antes de abrir el PR: `make lint` y `make test` en verde localmente.
5. Abrí el PR contra `main` con la plantilla (abajo).
6. **Otro integrante** revisa. El autor no aprueba su propio PR.
7. Con **CI verde** y aprobación: *Squash and merge* (el título del PR, en formato Conventional Commits, queda como commit en `main`).
8. Borrá la rama.

## Conventional Commits

```
<tipo>(<alcance>): <descripción en imperativo, minúscula, sin punto final>

[cuerpo opcional: qué y por qué]

[pie opcional: BREAKING CHANGE: …, Refs: HU-14, RN-14]
```

| Tipo | Cuándo |
|---|---|
| `feat` | Nueva funcionalidad |
| `fix` | Corrección de un error |
| `docs` | Solo documentación (SPEC, ADR, README) |
| `refactor` | Cambio interno sin cambiar comportamiento |
| `test` | Agregar o corregir tests |
| `perf` | Mejora de rendimiento |
| `build` | Docker, Makefile, dependencias |
| `ci` | GitHub Actions |
| `chore` | Mantenimiento que no encaja en lo anterior |

**Alcances sugeridos:** `gateway`, `members`, `booking`, `benefits`, `training`, `notification`, `web`, `pkg`, `deploy`, `docs`, `adr`, `spec`.

Ejemplos:

```
feat(booking): rechazar reservas superpuestas del mismo alumno
fix(benefits): evitar saldo negativo en débitos concurrentes
docs(adr): agregar ADR-005 de comunicación entre servicios
test(booking): cubrir último lugar con dos alumnos simultáneos
```

Un cambio que rompe un contrato (API pública, API de partners o esquema de evento) lleva `!` y `BREAKING CHANGE:` en el pie: `feat(benefits)!: …`.

## Plantilla de Pull Request

```markdown
## Qué cambia
<!-- Resumen breve -->

## Por qué
<!-- HU / RN del SPEC o ADR relacionados: HU-14, RN-14, ADR-003 -->

## Cómo se probó
- [ ] Tests unitarios
- [ ] Tests de integración (testcontainers)
- [ ] Prueba manual / E2E (indicar pasos)

## Checklist
- [ ] Respeta el patrón del servicio (capas / hexagonal)
- [ ] No accede a la base de otro servicio
- [ ] Errores en formato RFC 7807
- [ ] Si publica eventos: usa outbox; si consume: es idempotente
- [ ] Si cambia un contrato (API o evento): documentado en docs/contracts, docs/api o docs/events
- [ ] Si es una decisión estructural: ADR nuevo o actualizado
```

## Revisión de código

Quien revisa verifica, como mínimo:

- Que el cambio cumple las HU/RN que menciona y no contradice el SPEC.
- Que respeta los límites de servicio (ADR-001) y la comunicación definida (ADR-005).
- Que hay tests para las reglas de negocio nuevas, en especial los casos límite del SPEC §10.
- Que no hay secretos, datos personales en logs ni código muerto.

Comentarios: prefijo `bloqueante:` para lo que impide aprobar, `sugerencia:` para lo opcional.

## Cambios en la arquitectura o el SPEC

- **SPEC.md** se modifica solo por PR con alcance `spec`, aprobado por al menos otro integrante, actualizando el anexo de decisiones.
- Toda decisión estructural nueva (tecnología, límite de servicio, patrón, contrato) requiere un **ADR** basado en [docs/adr/ADR-000-plantilla.md](docs/adr/ADR-000-plantilla.md). Un ADR aceptado no se edita: se reemplaza por otro.
