CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE tipos_membresia (
 codigo text PRIMARY KEY CHECK(codigo IN ('MENSUAL','TRIMESTRAL','ANUAL','CLASES')),
 nombre text NOT NULL CHECK(char_length(btrim(nombre)) BETWEEN 1 AND 120),
 duracion_meses integer CHECK(duracion_meses>0),
 duracion_dias integer CHECK(duracion_dias>0),
 clases integer CHECK(clases>0),
 habilitado boolean NOT NULL DEFAULT true,
 modificada_por uuid REFERENCES usuarios(id),
 motivo text NOT NULL,
 actualizada_en timestamptz NOT NULL DEFAULT now(),
 CHECK(NOT(duracion_meses IS NOT NULL AND duracion_dias IS NOT NULL)),
 CHECK(codigo='CLASES' OR clases IS NULL)
);
INSERT INTO tipos_membresia(codigo,nombre,duracion_meses,motivo) VALUES
 ('MENSUAL','Mensual',1,'Inicialización según SPEC'),
 ('TRIMESTRAL','Trimestral',3,'Inicialización según SPEC'),
 ('ANUAL','Anual',12,'Inicialización según SPEC'),
 ('CLASES','Por clases',NULL,'Cupo y duración pendientes de configuración comercial');

CREATE TABLE membresias (
 id uuid PRIMARY KEY,
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 tipo_codigo text NOT NULL REFERENCES tipos_membresia(codigo),
 tipo_snapshot jsonb NOT NULL,
 fecha_inicio date NOT NULL,
 fecha_vencimiento date NOT NULL CHECK(fecha_vencimiento>=fecha_inicio),
 estado text NOT NULL CHECK(estado IN ('ACTIVA','VENCIDA','CANCELADA')),
 anterior_id uuid REFERENCES membresias(id),
 motivo text NOT NULL CHECK(char_length(btrim(motivo)) BETWEEN 3 AND 200),
 modificada_por uuid REFERENCES usuarios(id),
 actualizada_en timestamptz NOT NULL,
 CHECK(COALESCE(tipo_snapshot->>'codigo'=tipo_codigo,false)),
 CHECK(tipo_codigo<>'CLASES' OR COALESCE((tipo_snapshot->>'clases')::integer,0)>0),
 EXCLUDE USING gist(usuario_id WITH =, daterange(fecha_inicio,fecha_vencimiento,'[]') WITH &&) WHERE(estado='ACTIVA')
);
CREATE INDEX membresias_historial ON membresias(usuario_id,fecha_inicio DESC,id);
CREATE INDEX membresias_vencimiento ON membresias(fecha_vencimiento) WHERE estado='ACTIVA';

CREATE TABLE historial_membresias (
 id bigserial PRIMARY KEY,
 entidad text NOT NULL,
 entidad_id text NOT NULL,
 accion text NOT NULL,
 actor_id uuid REFERENCES usuarios(id),
 motivo text NOT NULL,
 datos jsonb NOT NULL,
 creado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX historial_membresias_entidad ON historial_membresias(entidad,entidad_id,id);

CREATE TABLE outbox_membresias (
 event_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
 event_type text NOT NULL,
 event_version integer NOT NULL DEFAULT 1,
 membresia_id uuid NOT NULL REFERENCES membresias(id),
 data jsonb NOT NULL,
 occurred_at timestamptz NOT NULL,
 publicado_en timestamptz,
 UNIQUE(membresia_id,event_type)
);
CREATE TABLE operaciones_membresia (
 actor_id uuid NOT NULL REFERENCES usuarios(id),
 clave text NOT NULL,
 huella text NOT NULL CHECK(length(huella)=64),
 resultado jsonb NOT NULL,
 creada_en timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(actor_id,clave)
);

CREATE FUNCTION proteger_historial_membresias() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'Historial de membresías inmutable' USING ERRCODE='23514'; END;
$$;
CREATE TRIGGER historial_membresias_inmutable BEFORE UPDATE OR DELETE ON historial_membresias FOR EACH ROW EXECUTE FUNCTION proteger_historial_membresias();
CREATE TRIGGER operaciones_membresia_inmutables BEFORE UPDATE OR DELETE ON operaciones_membresia FOR EACH ROW EXECUTE FUNCTION proteger_historial_membresias();

CREATE FUNCTION guardar_historial_membresias() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'No eliminar historial' USING ERRCODE='23514'; END IF;
 IF TG_OP='UPDATE' AND TG_TABLE_NAME='membresias' THEN
  IF OLD.estado<>'ACTIVA' OR NEW.estado NOT IN ('CANCELADA','VENCIDA') OR
     (to_jsonb(NEW)-ARRAY['estado','motivo','modificada_por','actualizada_en']) IS DISTINCT FROM
     (to_jsonb(OLD)-ARRAY['estado','motivo','modificada_por','actualizada_en']) THEN
   RAISE EXCEPTION 'Membresía histórica inmutable' USING ERRCODE='23514';
  END IF;
 END IF;
 IF TG_OP='UPDATE' AND TG_TABLE_NAME='tipos_membresia' AND (to_jsonb(NEW)->>'codigo')<>(to_jsonb(OLD)->>'codigo') THEN
  RAISE EXCEPTION 'Código de tipo inmutable' USING ERRCODE='23514';
 END IF;
 INSERT INTO historial_membresias(entidad,entidad_id,accion,actor_id,motivo,datos)
 VALUES(TG_TABLE_NAME,CASE WHEN TG_TABLE_NAME='membresias' THEN to_jsonb(NEW)->>'id' ELSE to_jsonb(NEW)->>'codigo' END,
 TG_OP,NEW.modificada_por,NEW.motivo,to_jsonb(NEW));
 RETURN NEW;
END;
$$;
CREATE TRIGGER membresia_historial BEFORE INSERT OR UPDATE OR DELETE ON membresias FOR EACH ROW EXECUTE FUNCTION guardar_historial_membresias();
CREATE TRIGGER tipo_membresia_historial BEFORE INSERT OR UPDATE OR DELETE ON tipos_membresia FOR EACH ROW EXECUTE FUNCTION guardar_historial_membresias();

CREATE FUNCTION encolar_evento_membresia() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tipo text; nombre text; correo text;
BEGIN
 IF TG_OP='UPDATE' AND NEW.estado='VENCIDA' THEN RETURN NEW; END IF;
 tipo:=CASE WHEN TG_OP='INSERT' THEN 'membresia.activada' WHEN NEW.estado='CANCELADA' THEN 'membresia.cancelada' ELSE 'membresia.vencida' END;
 SELECT u.nombre,u.email INTO nombre,correo FROM usuarios u WHERE u.id=NEW.usuario_id;
 INSERT INTO outbox_membresias(event_type,membresia_id,data,occurred_at)
 VALUES(tipo,NEW.id,jsonb_build_object('membresia_id',NEW.id,'alumno_id',NEW.usuario_id,'alumno_nombre',nombre,'alumno_email',correo,
 'tipo_membresia',NEW.tipo_codigo,'fecha_inicio',NEW.fecha_inicio,'fecha_vencimiento',NEW.fecha_vencimiento,
 'operacion',CASE WHEN NEW.anterior_id IS NULL THEN 'ASIGNACION' ELSE 'RENOVACION' END,'membresia_anterior_id',NEW.anterior_id,'motivo',NEW.motivo,
 'cancelada_en',CASE WHEN NEW.estado='CANCELADA' THEN NEW.actualizada_en ELSE NULL END),NEW.actualizada_en);
 RETURN NEW;
END;
$$;
CREATE TRIGGER membresia_outbox AFTER INSERT OR UPDATE ON membresias FOR EACH ROW EXECUTE FUNCTION encolar_evento_membresia();
