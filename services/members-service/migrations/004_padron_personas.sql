-- Etapa 3 de la entrega final: padrón de alumnos y profesionales, asignaciones,
-- activación de cuentas creadas por el administrador, bajas lógicas y auditoría
-- (HU-01, HU-02, RN-30, RN-33, RN-34, RN-36, D-78; contrato docs/api/users.v1.md).
-- Las personas externas del Club (D-63) siguen sin DNI.

ALTER TABLE usuarios
 ADD COLUMN apellido text CHECK (apellido IS NULL OR char_length(btrim(apellido)) BETWEEN 1 AND 80),
 ADD COLUMN dni text UNIQUE CHECK (dni IS NULL OR dni ~ '^[0-9]{7,8}$'),
 ADD COLUMN telefono text CHECK (telefono IS NULL OR telefono ~ '^[0-9+() -]{6,30}$'),
 ADD COLUMN fecha_nacimiento date CHECK (fecha_nacimiento IS NULL OR fecha_nacimiento >= DATE '1900-01-01'),
 ADD COLUMN matricula text,
 ADD COLUMN estado text NOT NULL DEFAULT 'ACTIVO' CHECK (estado IN ('ACTIVO','INACTIVO')),
 ADD COLUMN dado_de_baja_en timestamptz,
 ADD COLUMN actualizado_en timestamptz,
 ADD COLUMN actualizado_por uuid REFERENCES usuarios(id),
 -- La matrícula es opcional y solo corresponde a nutricionistas (SPEC §6.1).
 ADD CONSTRAINT matricula_solo_nutricionista CHECK (matricula IS NULL OR (rol='NUTRICIONISTA' AND char_length(btrim(matricula)) BETWEEN 1 AND 40)),
 -- Una persona del padrón (con DNI) tiene apellido; un alumno, además, fecha de nacimiento.
 ADD CONSTRAINT padron_completo CHECK (dni IS NULL OR (apellido IS NOT NULL AND (rol<>'ALUMNO' OR fecha_nacimiento IS NOT NULL))),
 ADD CONSTRAINT baja_coherente CHECK ((estado='INACTIVO') = (dado_de_baja_en IS NOT NULL));

-- Asignación profesional–alumno con vigencia; nunca se borra, se revoca (RN-30).
CREATE TABLE asignaciones (
 id uuid PRIMARY KEY,
 profesional_id uuid NOT NULL REFERENCES usuarios(id),
 alumno_id uuid NOT NULL REFERENCES usuarios(id),
 desde timestamptz NOT NULL,
 hasta timestamptz,
 asignada_por uuid NOT NULL REFERENCES usuarios(id),
 revocada_por uuid REFERENCES usuarios(id),
 CHECK (profesional_id <> alumno_id),
 CHECK (hasta IS NULL OR hasta >= desde),
 CHECK ((hasta IS NULL) = (revocada_por IS NULL))
);
-- Una sola asignación vigente por par profesional–alumno (SPEC §9).
CREATE UNIQUE INDEX asignacion_vigente_unica ON asignaciones(profesional_id, alumno_id) WHERE hasta IS NULL;
CREATE INDEX asignaciones_alumno ON asignaciones(alumno_id) WHERE hasta IS NULL;

-- Activación de cuentas creadas por el administrador (D-78): enlace de un solo
-- uso enviado por correo, mismo esquema de token que el registro externo (ADR-017).
CREATE TABLE activaciones_cuenta (
 id uuid PRIMARY KEY,
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 token_hash bytea NOT NULL UNIQUE CHECK (octet_length(token_hash)=32),
 token_cifrado bytea,
 token_expira timestamptz NOT NULL,
 consumida boolean NOT NULL DEFAULT false,
 correo_enviado_en timestamptz,
 intentos integer NOT NULL DEFAULT 0,
 proximo_intento timestamptz NOT NULL DEFAULT now(),
 creada_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX activaciones_correo_pendiente ON activaciones_cuenta(proximo_intento) WHERE correo_enviado_en IS NULL;
CREATE UNIQUE INDEX activacion_pendiente_unica ON activaciones_cuenta(usuario_id) WHERE NOT consumida;

-- Punto de extensión para la mensajería (etapa 5): los eventos se escriben en la
-- misma transacción que el cambio; todavía no hay relay que los publique.
CREATE TABLE outbox_usuarios (
 event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_type text NOT NULL CHECK (event_type IN ('alumno.creado','usuario.desactivado')),
 event_version integer NOT NULL DEFAULT 1,
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 data jsonb NOT NULL,
 occurred_at timestamptz NOT NULL,
 publicado_en timestamptz,
 UNIQUE (usuario_id, event_type)
);

-- Auditoría de escrituras sobre personas y asignaciones (RN-36), solo inserción.
CREATE TABLE auditoria_usuarios (
 id bigserial PRIMARY KEY,
 entidad text NOT NULL,
 entidad_id uuid NOT NULL,
 accion text NOT NULL,
 actor_id uuid REFERENCES usuarios(id),
 datos jsonb NOT NULL,
 creado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX auditoria_usuarios_entidad ON auditoria_usuarios(entidad, entidad_id, id);
CREATE FUNCTION proteger_auditoria_usuarios() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Auditoría de usuarios inmutable' USING ERRCODE='23514'; END;
$$;
CREATE TRIGGER auditoria_usuarios_inmutable BEFORE UPDATE OR DELETE ON auditoria_usuarios
 FOR EACH ROW EXECUTE FUNCTION proteger_auditoria_usuarios();
