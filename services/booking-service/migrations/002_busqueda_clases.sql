-- Etapa 8 de la entrega final: búsqueda de clases en OpenSearch (ADR-014, D-45).
-- PostgreSQL sigue siendo la fuente de verdad; el índice es un modelo de lectura
-- derivado de clase.actualizada, publicado por outbox (CLAUDE.md regla 6).

-- Versión monótona por clase: el indexer usa versionado externo y descarta
-- cualquier evento con versión menor o igual a la ya indexada.
ALTER TABLE clases ADD COLUMN version bigint NOT NULL DEFAULT 1 CHECK (version >= 1);

-- El outbox admite varios clase.actualizada por clase. La unicidad de un evento
-- por reserva se conserva para asistencia/inasistencia (etapas 4 y 6).
ALTER TABLE outbox DROP CONSTRAINT un_evento_por_reserva;
CREATE UNIQUE INDEX un_evento_por_reserva ON outbox (event_type, agregado_id)
  WHERE event_type IN ('asistencia.registrada', 'inasistencia.registrada');
ALTER TABLE outbox DROP CONSTRAINT outbox_event_type_check;
ALTER TABLE outbox ADD CONSTRAINT outbox_event_type_check
  CHECK (event_type IN ('asistencia.registrada', 'inasistencia.registrada', 'clase.actualizada'));

-- Toda escritura de una clase incrementa su versión, en la misma fila.
CREATE FUNCTION versionar_clase() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  NEW.version := OLD.version + 1;
  RETURN NEW;
END;
$$;
CREATE TRIGGER clase_versionada BEFORE UPDATE ON clases
  FOR EACH ROW EXECUTE FUNCTION versionar_clase();

-- Captura de cambios en la misma transacción que la escritura: ningún camino
-- (generar, reservar, cancelar, capacidad, cancelación o cierre) puede cambiar
-- una clase sin dejar su evento. occurred_at usa el reloj real para medir el
-- retraso hasta que el cambio es visible en la búsqueda.
CREATE FUNCTION publicar_clase() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a actividades%ROWTYPE;
BEGIN
  SELECT * INTO a FROM actividades WHERE id = NEW.actividad_id;
  INSERT INTO outbox (event_type, occurred_at, agregado_id, data) VALUES (
    'clase.actualizada', clock_timestamp(), NEW.id,
    jsonb_build_object(
      'clase_id', NEW.id, 'version', NEW.version,
      'actividad_id', NEW.actividad_id, 'actividad_nombre', a.nombre, 'musculacion', a.musculacion,
      'inicio', to_char(NEW.inicio AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
      'fin', to_char(NEW.fin AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
      'profesor_id', NEW.profesor_id, 'profesor_nombre', NEW.profesor_nombre,
      'capacidad', NEW.capacidad, 'ocupacion', NEW.ocupacion, 'estado', NEW.estado));
  RETURN NULL;
END;
$$;
CREATE TRIGGER clase_publicada AFTER INSERT OR UPDATE ON clases
  FOR EACH ROW EXECUTE FUNCTION publicar_clase();

-- Renombrar una actividad cambia el documento de sus clases: se vuelven a
-- publicar con una versión nueva para que el índice no quede con el nombre viejo.
CREATE FUNCTION republicar_clases_de_actividad() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.nombre IS DISTINCT FROM OLD.nombre OR NEW.musculacion IS DISTINCT FROM OLD.musculacion THEN
    UPDATE clases SET actualizada_en = actualizada_en WHERE actividad_id = NEW.id;
  END IF;
  RETURN NULL;
END;
$$;
CREATE TRIGGER actividad_republicada AFTER UPDATE ON actividades
  FOR EACH ROW EXECUTE FUNCTION republicar_clases_de_actividad();
