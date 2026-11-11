-- Cuponera interna del Club (D-94): HU-09, HU-20, RN-22/23/24/25.
-- Extiende el mismo ledger con el tipo CANJE. Los beneficios no tienen stock
-- (CA-09.5); el canje conserva nombre y puntos del momento (CA-09.4).

ALTER TABLE movimientos_puntos DROP CONSTRAINT importe_por_tipo;
ALTER TABLE movimientos_puntos ADD CONSTRAINT importe_por_tipo CHECK (
 (tipo='CREDITO_PARTNER' AND partner_id IS NOT NULL AND origen_id IS NULL AND revierte_id IS NULL AND puntos BETWEEN 1 AND 1000000)
 OR (tipo='ASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND revierte_id IS NULL AND puntos=500)
 OR (tipo='PENALIZACION_INASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND revierte_id IS NULL AND puntos BETWEEN -100 AND 0)
 OR (tipo='CANJE' AND partner_id IS NULL AND origen_id IS NOT NULL AND revierte_id IS NULL AND puntos BETWEEN -1000000 AND -1)
 OR (tipo='REVERSION' AND partner_id IS NULL AND origen_id IS NULL AND revierte_id IS NOT NULL AND puntos BETWEEN -1000000 AND 1000000)
);

-- El recibo de mensajes de Booking solo corresponde a sus dos tipos: un canje
-- también usa origen_id (su id), pero no viene de un evento.
CREATE OR REPLACE FUNCTION verificar_recibo_booking() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE mid uuid;
BEGIN
 IF TG_TABLE_NAME='movimientos_puntos' THEN
  IF NEW.origen_id IS NULL OR NEW.tipo NOT IN ('ASISTENCIA','PENALIZACION_INASISTENCIA') THEN RETURN NULL; END IF;
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

CREATE TABLE beneficios (
 id uuid PRIMARY KEY,
 nombre text NOT NULL CHECK (char_length(nombre) BETWEEN 3 AND 100),
 descripcion text NOT NULL DEFAULT '' CHECK (char_length(descripcion) <= 500),
 costo bigint NOT NULL CHECK (costo BETWEEN 1 AND 1000000),
 vigente_desde date NOT NULL,
 vigente_hasta date CHECK (vigente_hasta >= vigente_desde),
 activo boolean NOT NULL DEFAULT true,
 creado_en timestamptz NOT NULL,
 actualizado_en timestamptz NOT NULL
);
CREATE INDEX beneficios_catalogo ON beneficios(activo, costo, nombre);

CREATE TABLE canjes (
 id uuid PRIMARY KEY,
 cuenta_id uuid NOT NULL REFERENCES cuentas(id),
 usuario_id uuid NOT NULL,
 beneficio_id uuid NOT NULL REFERENCES beneficios(id),
 beneficio_nombre text NOT NULL CHECK (char_length(beneficio_nombre) BETWEEN 3 AND 100),
 puntos bigint NOT NULL CHECK (puntos BETWEEN 1 AND 1000000),
 codigo text NOT NULL UNIQUE CHECK (codigo ~ '^[ABCDEFGHJKMNPQRSTUVWXYZ2-9]{4}-[ABCDEFGHJKMNPQRSTUVWXYZ2-9]{4}$'),
 estado text NOT NULL CHECK (estado IN ('CONFIRMADO','USADO','ANULADO')),
 movimiento_id uuid NOT NULL UNIQUE REFERENCES movimientos_puntos(id),
 anulacion_id uuid UNIQUE REFERENCES movimientos_puntos(id),
 motivo_anulacion text CHECK (char_length(motivo_anulacion) BETWEEN 3 AND 200),
 -- Idempotency-Key del canje (regla 9 de CLAUDE.md), única por alumno.
 clave text NOT NULL CHECK (clave ~ '^[A-Za-z0-9._:-]{8,128}$'),
 huella text NOT NULL CHECK (length(huella) = 64),
 -- Saldo después del débito: lo que se devuelve ante un reintento idempotente.
 saldo_resultante bigint NOT NULL CHECK (saldo_resultante >= 0),
 creado_en timestamptz NOT NULL,
 usado_en timestamptz,
 anulado_en timestamptz,
 UNIQUE (usuario_id, clave),
 CHECK ((estado = 'USADO') = (usado_en IS NOT NULL)),
 CHECK ((estado = 'ANULADO') = (anulado_en IS NOT NULL AND anulacion_id IS NOT NULL AND motivo_anulacion IS NOT NULL))
);
CREATE INDEX canjes_usuario ON canjes(usuario_id, creado_en DESC);
CREATE INDEX canjes_estado ON canjes(estado, creado_en DESC);

-- Un canje nunca se borra y solo admite CONFIRMADO -> USADO o CONFIRMADO -> ANULADO.
-- Lo que se debitó, a quién y por qué beneficio no cambia (RN-22, CA-09.4).
CREATE FUNCTION proteger_canje() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP = 'DELETE' THEN
  RAISE EXCEPTION 'Los canjes no se borran' USING ERRCODE='23514';
 END IF;
 IF NEW.id <> OLD.id OR NEW.cuenta_id <> OLD.cuenta_id OR NEW.usuario_id <> OLD.usuario_id OR NEW.beneficio_id <> OLD.beneficio_id
    OR NEW.beneficio_nombre <> OLD.beneficio_nombre OR NEW.puntos <> OLD.puntos OR NEW.codigo <> OLD.codigo
    OR NEW.movimiento_id <> OLD.movimiento_id OR NEW.clave <> OLD.clave OR NEW.huella <> OLD.huella
    OR NEW.saldo_resultante <> OLD.saldo_resultante OR NEW.creado_en <> OLD.creado_en THEN
  RAISE EXCEPTION 'Los datos del canje son inmutables' USING ERRCODE='23514';
 END IF;
 IF NEW.estado <> OLD.estado AND NOT (OLD.estado = 'CONFIRMADO' AND NEW.estado IN ('USADO','ANULADO')) THEN
  RAISE EXCEPTION 'Transición de canje inválida' USING ERRCODE='23514';
 END IF;
 IF NEW.estado = OLD.estado AND (NEW.usado_en IS DISTINCT FROM OLD.usado_en OR NEW.anulado_en IS DISTINCT FROM OLD.anulado_en
    OR NEW.anulacion_id IS DISTINCT FROM OLD.anulacion_id OR NEW.motivo_anulacion IS DISTINCT FROM OLD.motivo_anulacion) THEN
  RAISE EXCEPTION 'Los datos del canje son inmutables' USING ERRCODE='23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER canje_protegido BEFORE UPDATE OR DELETE ON canjes FOR EACH ROW EXECUTE FUNCTION proteger_canje();

-- Al confirmar la transacción: todo débito CANJE tiene su cupón con la misma
-- cuenta e importe, y toda reversión de un CANJE corresponde a su anulación.
CREATE FUNCTION verificar_canje() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o_tipo text;
BEGIN
 IF TG_TABLE_NAME = 'movimientos_puntos' THEN
  IF NEW.tipo = 'CANJE' THEN
   IF NOT EXISTS (SELECT 1 FROM canjes k WHERE k.id = NEW.origen_id AND k.movimiento_id = NEW.id AND k.cuenta_id = NEW.cuenta_id AND k.puntos = -NEW.puntos) THEN
    RAISE EXCEPTION 'Débito de canje sin cupón compatible' USING ERRCODE='23514';
   END IF;
  ELSIF NEW.tipo = 'REVERSION' THEN
   SELECT tipo INTO o_tipo FROM movimientos_puntos WHERE id = NEW.revierte_id;
   IF o_tipo = 'CANJE' AND NOT EXISTS (SELECT 1 FROM canjes k WHERE k.movimiento_id = NEW.revierte_id AND k.estado = 'ANULADO' AND k.anulacion_id = NEW.id) THEN
    RAISE EXCEPTION 'Un canje solo se revierte anulándolo' USING ERRCODE='23514';
   END IF;
  END IF;
 ELSE
  IF NOT EXISTS (SELECT 1 FROM movimientos_puntos m WHERE m.id = NEW.movimiento_id AND m.tipo = 'CANJE' AND m.origen_id = NEW.id AND m.cuenta_id = NEW.cuenta_id AND m.puntos = -NEW.puntos) THEN
   RAISE EXCEPTION 'Cupón sin débito compatible' USING ERRCODE='23514';
  END IF;
  IF NEW.estado = 'ANULADO' AND NOT EXISTS (SELECT 1 FROM movimientos_puntos m WHERE m.id = NEW.anulacion_id AND m.tipo = 'REVERSION' AND m.revierte_id = NEW.movimiento_id) THEN
   RAISE EXCEPTION 'Anulación sin compensatorio compatible' USING ERRCODE='23514';
  END IF;
 END IF;
 RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER movimiento_canje_completo AFTER INSERT ON movimientos_puntos
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_canje();
CREATE CONSTRAINT TRIGGER canje_completo AFTER INSERT OR UPDATE ON canjes
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_canje();

-- Auditoría del catálogo y de la gestión de cupones (HU-09, HU-CB-07).
ALTER TABLE auditoria_club DROP CONSTRAINT auditoria_club_accion_check;
ALTER TABLE auditoria_club ADD CONSTRAINT auditoria_club_accion_check CHECK (accion IN (
 'ALTA_INTEGRACION','CAMBIO_INTEGRACION','ROTACION_CREDENCIAL','VINCULACION','REVERSION',
 'ALTA_BENEFICIO','CAMBIO_BENEFICIO','USO_CANJE','ANULACION_CANJE'));
