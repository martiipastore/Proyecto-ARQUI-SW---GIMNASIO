-- Etapa 4 de la entrega final: actividades, horarios, clases, reservas,
-- asistencias, idempotencia, outbox y auditoría de booking-service.
-- Las invariantes críticas también las garantiza la base (CLAUDE.md regla 8):
-- cupo (RN-14), unicidad (RN-13), turno diario de Musculación (RN-39),
-- superposición de reservas (RN-40) y de profesores (RN-08), asistencia única (RN-21).

-- Extensión confiable desde PostgreSQL 13: la crea el dueño de la base.
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE actividades (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  nombre text NOT NULL CHECK (length(btrim(nombre)) BETWEEN 1 AND 60),
  capacidad_maxima integer NOT NULL,
  musculacion boolean NOT NULL DEFAULT false,
  activa boolean NOT NULL DEFAULT true,
  creada_en timestamptz NOT NULL DEFAULT now(),
  actualizada_en timestamptz NOT NULL DEFAULT now(),
  actualizada_por uuid,
  CONSTRAINT tope_actividad CHECK (capacidad_maxima >= 1 AND capacidad_maxima <= CASE WHEN musculacion THEN 50 ELSE 30 END)
);
CREATE UNIQUE INDEX actividades_nombre_unico ON actividades (lower(nombre));
CREATE UNIQUE INDEX una_sola_musculacion ON actividades (musculacion) WHERE musculacion;

-- Catálogo inicial (CA-06.1, D-35).
INSERT INTO actividades (nombre, capacidad_maxima, musculacion) VALUES
  ('Musculación', 50, true),
  ('Funcional', 30, false),
  ('GAP', 30, false),
  ('Strong Nation', 30, false),
  ('Zumba', 30, false);

CREATE TABLE horarios (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actividad_id uuid NOT NULL REFERENCES actividades(id),
  dia_semana smallint NOT NULL CHECK (dia_semana BETWEEN 1 AND 7),
  inicio_minutos integer NOT NULL CHECK (inicio_minutos BETWEEN 0 AND 1439),
  duracion_minutos integer NOT NULL CHECK (duracion_minutos BETWEEN 15 AND 240),
  profesor_id uuid NOT NULL,
  profesor_nombre text NOT NULL,
  capacidad integer NOT NULL CHECK (capacidad BETWEEN 1 AND 50),
  creado_en timestamptz NOT NULL DEFAULT now(),
  creado_por uuid NOT NULL,
  CONSTRAINT horario_en_el_dia CHECK (inicio_minutos + duracion_minutos <= 1440),
  -- Un profesor no es responsable de dos horarios que se pisan (RN-08, CA-07.4).
  CONSTRAINT profesor_sin_horarios_superpuestos EXCLUDE USING gist (
    profesor_id WITH =, dia_semana WITH =,
    int4range(inicio_minutos, inicio_minutos + duracion_minutos) WITH &&)
);
CREATE INDEX horarios_actividad ON horarios (actividad_id, dia_semana, inicio_minutos);

CREATE TABLE clases (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actividad_id uuid NOT NULL REFERENCES actividades(id),
  horario_id uuid REFERENCES horarios(id),
  inicio timestamptz NOT NULL,
  fin timestamptz NOT NULL,
  profesor_id uuid NOT NULL,
  profesor_nombre text NOT NULL,
  capacidad integer NOT NULL CHECK (capacidad BETWEEN 1 AND 50),
  -- Reservas que tomaron cupo; antes del inicio coincide con las CONFIRMADA (RN-11).
  ocupacion integer NOT NULL DEFAULT 0,
  estado text NOT NULL DEFAULT 'PROGRAMADA' CHECK (estado IN ('PROGRAMADA', 'CANCELADA', 'FINALIZADA')),
  motivo_cancelacion text,
  cancelada_en timestamptz,
  creada_en timestamptz NOT NULL DEFAULT now(),
  actualizada_en timestamptz NOT NULL DEFAULT now(),
  actualizada_por uuid,
  CONSTRAINT clase_con_duracion CHECK (fin > inicio),
  -- La ocupación nunca supera la capacidad (RN-14).
  CONSTRAINT cupo CHECK (ocupacion >= 0 AND ocupacion <= capacidad),
  CONSTRAINT cancelacion_coherente CHECK ((estado = 'CANCELADA') = (cancelada_en IS NOT NULL AND motivo_cancelacion IS NOT NULL)),
  -- Regenerar el mismo período no duplica clases (RN-09, CA-07.5).
  CONSTRAINT clase_unica UNIQUE (actividad_id, inicio),
  CONSTRAINT profesor_sin_clases_superpuestas EXCLUDE USING gist (
    profesor_id WITH =, tstzrange(inicio, fin) WITH &&) WHERE (estado <> 'CANCELADA')
);
CREATE INDEX clases_inicio ON clases (inicio, id);
CREATE INDEX clases_profesor ON clases (profesor_id, inicio);
CREATE INDEX clases_por_cerrar ON clases (fin) WHERE estado = 'PROGRAMADA';

CREATE TABLE reservas (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  clase_id uuid NOT NULL REFERENCES clases(id),
  alumno_id uuid NOT NULL,
  estado text NOT NULL CHECK (estado IN ('CONFIRMADA', 'CANCELADA', 'ASISTIDA', 'AUSENTE')),
  -- Copias del horario de la clase para validar superposición en la base (RN-40).
  inicio timestamptz NOT NULL,
  fin timestamptz NOT NULL,
  -- Día calendario del turno de Musculación (RN-39); NULL en otras actividades.
  dia_musculacion date,
  creada_en timestamptz NOT NULL,
  ingreso_en timestamptz,
  cancelada_en timestamptz,
  motivo_cancelacion text,
  actualizada_en timestamptz NOT NULL,
  actualizada_por uuid,
  CONSTRAINT ingreso_coherente CHECK ((estado = 'ASISTIDA') = (ingreso_en IS NOT NULL)),
  CONSTRAINT cancelacion_coherente CHECK ((estado = 'CANCELADA') = (cancelada_en IS NOT NULL AND motivo_cancelacion IS NOT NULL)),
  CONSTRAINT alumno_sin_reservas_superpuestas EXCLUDE USING gist (
    alumno_id WITH =, tstzrange(inicio, fin) WITH &&) WHERE (estado = 'CONFIRMADA')
);
-- Una sola reserva activa por alumno y clase (RN-13, CA-14.6).
CREATE UNIQUE INDEX reserva_activa_unica ON reservas (clase_id, alumno_id) WHERE estado = 'CONFIRMADA';
-- Un turno de Musculación por día, reservado o asistido (RN-39, CA-14.9/10).
CREATE UNIQUE INDEX musculacion_un_turno_por_dia ON reservas (alumno_id, dia_musculacion)
  WHERE dia_musculacion IS NOT NULL AND estado IN ('CONFIRMADA', 'ASISTIDA');
CREATE INDEX reservas_alumno ON reservas (alumno_id, inicio DESC, id);
CREATE INDEX reservas_clase ON reservas (clase_id, estado);

CREATE FUNCTION proteger_historial_booking() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'El historial de booking es inmutable';
END;
$$;

-- Cada reserva tiene como máximo una asistencia, que no se modifica (RN-21).
CREATE TABLE asistencias (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  reserva_id uuid NOT NULL UNIQUE REFERENCES reservas(id),
  clase_id uuid NOT NULL REFERENCES clases(id),
  alumno_id uuid NOT NULL,
  origen text NOT NULL DEFAULT 'INGRESO_DNI' CHECK (origen = 'INGRESO_DNI'),
  puntos integer NOT NULL DEFAULT 500 CHECK (puntos = 500),
  registrada_en timestamptz NOT NULL,
  registrada_por uuid NOT NULL
);
CREATE TRIGGER asistencias_inmutables BEFORE UPDATE OR DELETE ON asistencias
  FOR EACH ROW EXECUTE FUNCTION proteger_historial_booking();

-- Resultados de escrituras con Idempotency-Key (RN-15).
CREATE TABLE operaciones (
  actor_id uuid NOT NULL,
  clave text NOT NULL,
  tipo text NOT NULL,
  huella text NOT NULL CHECK (length(huella) = 64),
  resultado jsonb NOT NULL,
  creada_en timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (actor_id, clave)
);
CREATE TRIGGER operaciones_inmutables BEFORE UPDATE OR DELETE ON operaciones
  FOR EACH ROW EXECUTE FUNCTION proteger_historial_booking();

-- Outbox (CLAUDE.md regla 6). El publicador a RabbitMQ llega en la etapa de
-- mensajería; cada reserva produce a lo sumo un evento de cada tipo.
CREATE TABLE outbox (
  event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  event_type text NOT NULL CHECK (event_type IN ('asistencia.registrada', 'inasistencia.registrada')),
  event_version integer NOT NULL DEFAULT 1,
  occurred_at timestamptz NOT NULL,
  correlation_id text,
  agregado_id uuid NOT NULL,
  data jsonb NOT NULL,
  publicado_en timestamptz,
  CONSTRAINT un_evento_por_reserva UNIQUE (event_type, agregado_id)
);
CREATE INDEX outbox_pendientes ON outbox (occurred_at) WHERE publicado_en IS NULL;

-- Quién hizo cada escritura y cuándo (RN-36).
CREATE TABLE auditoria (
  id bigserial PRIMARY KEY,
  actor_id uuid,
  accion text NOT NULL,
  entidad text NOT NULL,
  entidad_id uuid,
  detalle text,
  ocurrida_en timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER auditoria_inmutable BEFORE UPDATE OR DELETE ON auditoria
  FOR EACH ROW EXECUTE FUNCTION proteger_historial_booking();
