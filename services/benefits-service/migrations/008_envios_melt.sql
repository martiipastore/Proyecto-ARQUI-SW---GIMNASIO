-- Etapa 13 (D-92, ADR-009 v2): acreditación de puntos en MELT por cada asistencia.
-- El envío se registra en la misma transacción que el movimiento local y lo
-- entrega un proceso en segundo plano. La base garantiza un envío por asistencia
-- y una referencia única que nunca cambia entre reintentos.
CREATE TABLE envios_melt (
 id uuid PRIMARY KEY,
 alumno_id uuid NOT NULL,
 origen_id uuid NOT NULL UNIQUE,
 referencia text NOT NULL UNIQUE CHECK (length(referencia) BETWEEN 1 AND 128),
 puntos bigint NOT NULL CHECK (puntos > 0),
 estado text NOT NULL CHECK (estado IN ('PENDIENTE_ENVIO','ACREDITADO','PENDIENTE_REGISTRO','RECHAZADO')),
 intentos integer NOT NULL DEFAULT 0 CHECK (intentos >= 0),
 proximo_intento timestamptz NOT NULL,
 registro_url text,
 registro_vence timestamptz,
 motivo text NOT NULL DEFAULT '',
 creado_en timestamptz NOT NULL,
 actualizado_en timestamptz NOT NULL,
 CHECK ((estado = 'PENDIENTE_REGISTRO') = (registro_url IS NOT NULL))
);
CREATE INDEX envios_melt_vencidos ON envios_melt(proximo_intento) WHERE estado IN ('PENDIENTE_ENVIO','PENDIENTE_REGISTRO');
CREATE INDEX envios_melt_alumno ON envios_melt(alumno_id, creado_en DESC);

-- La referencia, los puntos y el origen no cambian después del alta: un
-- reintento siempre repite la misma operación en el proveedor.
CREATE FUNCTION proteger_envio_melt() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' THEN
  RAISE EXCEPTION 'Los envíos a MELT no se borran' USING ERRCODE='23514';
 END IF;
 IF NEW.referencia <> OLD.referencia OR NEW.puntos <> OLD.puntos OR NEW.origen_id <> OLD.origen_id OR NEW.alumno_id <> OLD.alumno_id THEN
  RAISE EXCEPTION 'La operación enviada a MELT es inmutable' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER envio_melt_inmutable BEFORE UPDATE OR DELETE ON envios_melt FOR EACH ROW EXECUTE FUNCTION proteger_envio_melt();
