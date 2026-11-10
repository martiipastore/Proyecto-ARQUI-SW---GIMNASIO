-- RN-20/22/38/42: extender el mismo ledger, sin modificar migraciones históricas.
ALTER TABLE movimientos_puntos ALTER COLUMN partner_id DROP NOT NULL;
ALTER TABLE movimientos_puntos DROP CONSTRAINT movimientos_puntos_puntos_check;
ALTER TABLE movimientos_puntos ADD COLUMN tipo text NOT NULL DEFAULT 'CREDITO_PARTNER';
ALTER TABLE movimientos_puntos ADD COLUMN origen_id uuid;
ALTER TABLE movimientos_puntos ADD CONSTRAINT importe_por_tipo CHECK (
 (tipo='CREDITO_PARTNER' AND partner_id IS NOT NULL AND origen_id IS NULL AND puntos BETWEEN 1 AND 1000000)
 OR (tipo='ASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND puntos=500)
 OR (tipo='PENALIZACION_INASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND puntos BETWEEN -100 AND 0)
);
CREATE UNIQUE INDEX hecho_booking_unico ON movimientos_puntos(tipo,origen_id) WHERE origen_id IS NOT NULL;
CREATE TABLE mensajes_booking_procesados (
 event_id uuid PRIMARY KEY,
 tipo text NOT NULL CHECK(tipo IN ('ASISTENCIA','PENALIZACION_INASISTENCIA')),
 origen_id uuid NOT NULL,
 huella text NOT NULL CHECK(length(huella)=64),
 movimiento_id uuid NOT NULL REFERENCES movimientos_puntos(id),
 procesado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mensaje_booking_origen ON mensajes_booking_procesados(tipo,origen_id);
CREATE TRIGGER mensaje_booking_inmutable BEFORE UPDATE OR DELETE ON mensajes_booking_procesados FOR EACH ROW EXECUTE FUNCTION proteger_movimiento();

CREATE FUNCTION verificar_recibo_booking() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE mid uuid;
BEGIN
 IF TG_TABLE_NAME='movimientos_puntos' THEN
  IF NEW.origen_id IS NULL THEN RETURN NULL; END IF;
  mid:=NEW.id;
 ELSE mid:=NEW.movimiento_id;
 END IF;
 IF NOT EXISTS(SELECT 1 FROM movimientos_puntos m JOIN mensajes_booking_procesados p
   ON p.movimiento_id=m.id AND p.tipo=m.tipo AND p.origen_id=m.origen_id WHERE m.id=mid) THEN
  RAISE EXCEPTION 'Movimiento Booking sin recibo compatible' USING ERRCODE='23514';
 END IF;
 RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER movimiento_booking_completo AFTER INSERT ON movimientos_puntos DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_recibo_booking();
CREATE CONSTRAINT TRIGGER recibo_booking_completo AFTER INSERT ON mensajes_booking_procesados DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_recibo_booking();
