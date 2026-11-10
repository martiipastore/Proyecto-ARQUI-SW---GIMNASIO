-- Etapa 7 de la entrega final (HU-10, HU-11, HU-35, HU-CB-07; RN-22/23/25/26).
-- Extiende el mismo ledger: la reversión es un movimiento compensatorio más,
-- con secuencia propia, y nunca modifica el original (RN-22). El estado
-- REVERTIDO se deriva de la existencia de su compensatorio, único por original.
ALTER TABLE movimientos_puntos ADD COLUMN revierte_id uuid REFERENCES movimientos_puntos(id);
CREATE UNIQUE INDEX reversion_unica ON movimientos_puntos(revierte_id) WHERE revierte_id IS NOT NULL;
ALTER TABLE movimientos_puntos DROP CONSTRAINT importe_por_tipo;
ALTER TABLE movimientos_puntos ADD CONSTRAINT importe_por_tipo CHECK (
 (tipo='CREDITO_PARTNER' AND partner_id IS NOT NULL AND origen_id IS NULL AND revierte_id IS NULL AND puntos BETWEEN 1 AND 1000000)
 OR (tipo='ASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND revierte_id IS NULL AND puntos=500)
 OR (tipo='PENALIZACION_INASISTENCIA' AND partner_id IS NULL AND origen_id IS NOT NULL AND revierte_id IS NULL AND puntos BETWEEN -100 AND 0)
 OR (tipo='REVERSION' AND partner_id IS NULL AND origen_id IS NULL AND revierte_id IS NOT NULL AND puntos BETWEEN -1000000 AND 100)
);

-- El compensatorio pertenece a la misma cuenta, aplica el importe contrario y
-- no revierte otra reversión. El saldo no negativo lo sigue garantizando
-- cuentas.saldo >= 0 junto con verificar_ledger (RN-23).
CREATE FUNCTION verificar_reversion() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE o_cuenta uuid; o_tipo text; o_puntos bigint;
BEGIN
 IF NEW.tipo <> 'REVERSION' THEN RETURN NEW; END IF;
 SELECT cuenta_id, tipo, puntos INTO o_cuenta, o_tipo, o_puntos FROM movimientos_puntos WHERE id = NEW.revierte_id;
 IF NOT FOUND OR o_cuenta <> NEW.cuenta_id OR o_tipo = 'REVERSION' OR NEW.puntos <> -o_puntos THEN
  RAISE EXCEPTION 'Reversión incompatible con el movimiento original' USING ERRCODE = '23514';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER movimiento_reversion BEFORE INSERT ON movimientos_puntos
FOR EACH ROW EXECUTE FUNCTION verificar_reversion();

-- Idempotency-Key de la reversión (regla 9 de CLAUDE.md): única por
-- administrador; misma clave y mismos datos devuelven el resultado original.
CREATE TABLE reversiones_admin (
 administrador_id uuid NOT NULL,
 clave text NOT NULL CHECK (clave ~ '^[A-Za-z0-9._:-]{8,128}$'),
 huella text NOT NULL CHECK (length(huella) = 64),
 movimiento_id uuid NOT NULL UNIQUE REFERENCES movimientos_puntos(id),
 saldo_resultante bigint NOT NULL CHECK (saldo_resultante >= 0),
 creada_en timestamptz NOT NULL,
 PRIMARY KEY (administrador_id, clave)
);
CREATE TRIGGER reversion_admin_inmutable BEFORE UPDATE OR DELETE ON reversiones_admin
FOR EACH ROW EXECUTE FUNCTION proteger_movimiento();

-- Auditoría inmutable de la administración del Club (HU-CB-07). Nunca guarda
-- la API key ni su hash: solo quién, qué, cuándo y la configuración resultante.
CREATE TABLE auditoria_club (
 id uuid PRIMARY KEY,
 administrador_id uuid NOT NULL,
 accion text NOT NULL CHECK (accion IN ('ALTA_INTEGRACION','CAMBIO_INTEGRACION','ROTACION_CREDENCIAL','VINCULACION','REVERSION')),
 partner_id uuid REFERENCES partners(id),
 movimiento_id uuid REFERENCES movimientos_puntos(id),
 detalle jsonb NOT NULL DEFAULT '{}'::jsonb,
 ocurrido_en timestamptz NOT NULL
);
CREATE INDEX auditoria_club_partner ON auditoria_club(partner_id, ocurrido_en DESC);
CREATE TRIGGER auditoria_club_inmutable BEFORE UPDATE OR DELETE ON auditoria_club
FOR EACH ROW EXECUTE FUNCTION proteger_movimiento();

ALTER TABLE partners ADD COLUMN creada_en timestamptz NOT NULL DEFAULT now();
ALTER TABLE partners ADD CONSTRAINT partner_nombre_valido CHECK (char_length(nombre) BETWEEN 1 AND 100) NOT VALID;
