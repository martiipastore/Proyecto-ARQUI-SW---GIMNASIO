-- Conserva las autorizaciones actuales; las nuevas instalaciones pueden ajustar
-- estos valores por integración antes de entregar su credencial.
ALTER TABLE partners
 ADD COLUMN permite_consulta boolean NOT NULL DEFAULT true,
 ADD COLUMN permite_puntos boolean NOT NULL DEFAULT true,
 ADD COLUMN max_puntos_operacion bigint NOT NULL DEFAULT 1000000 CHECK (max_puntos_operacion BETWEEN 1 AND 1000000),
 ADD COLUMN max_puntos_dia bigint CHECK (max_puntos_dia > 0);

-- Identidad del evento independiente de Idempotency-Key. No se purga junto
-- a la retención de respuestas: un evento no vuelve a generar puntos.
CREATE TABLE eventos_acreditados (
 partner_id uuid NOT NULL REFERENCES partners(id),
 evento_id text NOT NULL CHECK (char_length(evento_id) BETWEEN 1 AND 128),
 huella text NOT NULL CHECK (length(huella)=64),
 saldo_resultante bigint NOT NULL CHECK (saldo_resultante >= 0),
 movimiento_id uuid NOT NULL UNIQUE REFERENCES movimientos_puntos(id),
 PRIMARY KEY (partner_id, evento_id)
);
CREATE TRIGGER evento_inmutable BEFORE UPDATE OR DELETE ON eventos_acreditados
 FOR EACH ROW EXECUTE FUNCTION proteger_movimiento();
CREATE INDEX movimientos_limite_diario ON movimientos_puntos(partner_id, aplicado_en);
