CREATE TABLE partners (
    id uuid PRIMARY KEY,
    nombre text NOT NULL,
    api_key_hash bytea NOT NULL UNIQUE CHECK (octet_length(api_key_hash) = 32),
    activo boolean NOT NULL DEFAULT true
);
CREATE TABLE cuentas (
    id uuid PRIMARY KEY,
    alumno_id uuid NOT NULL UNIQUE,
    saldo bigint NOT NULL DEFAULT 0 CHECK (saldo >= 0),
    ultima_secuencia bigint NOT NULL DEFAULT 0 CHECK (ultima_secuencia >= 0)
);
CREATE TABLE vinculaciones (
    partner_id uuid NOT NULL REFERENCES partners(id),
    external_user_id text NOT NULL CHECK (external_user_id ~ '^[A-Za-z0-9._:@-]{1,64}$'),
    cuenta_id uuid NOT NULL REFERENCES cuentas(id),
    PRIMARY KEY (partner_id, external_user_id),
    UNIQUE (partner_id, cuenta_id)
);
CREATE INDEX vinculaciones_cuenta ON vinculaciones(cuenta_id);
CREATE TABLE movimientos_puntos (
    id uuid PRIMARY KEY,
    cuenta_id uuid NOT NULL REFERENCES cuentas(id),
    partner_id uuid NOT NULL REFERENCES partners(id),
    secuencia bigint NOT NULL CHECK (secuencia > 0),
    puntos bigint NOT NULL CHECK (puntos BETWEEN 1 AND 1000000),
    motivo text NOT NULL CHECK (char_length(motivo) BETWEEN 3 AND 200),
    referencia text NOT NULL CHECK (char_length(referencia) BETWEEN 1 AND 100),
    ocurrido_en timestamptz NOT NULL,
    aplicado_en timestamptz NOT NULL,
    UNIQUE (cuenta_id, secuencia),
    FOREIGN KEY (partner_id, cuenta_id) REFERENCES vinculaciones(partner_id, cuenta_id)
);
CREATE INDEX movimientos_historial ON movimientos_puntos(cuenta_id, partner_id, ocurrido_en DESC, secuencia DESC);
CREATE TABLE operaciones_partner (
    partner_id uuid NOT NULL REFERENCES partners(id),
    clave text NOT NULL CHECK (clave ~ '^[A-Za-z0-9._:-]{8,128}$'),
    huella text NOT NULL CHECK (length(huella) = 64),
    creada_en timestamptz NOT NULL,
    conservar_hasta timestamptz NOT NULL,
    resultado jsonb,
    PRIMARY KEY (partner_id, clave),
    CHECK (conservar_hasta >= creada_en + interval '30 days')
);
CREATE INDEX operaciones_retencion ON operaciones_partner(conservar_hasta);

-- RN-22: en esta etapa solo se agregan créditos. No se permite editar/borrar
-- movimientos. Los futuros tipos se incorporan mediante otra migración.
CREATE FUNCTION proteger_movimiento() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'Los movimientos son inmutables' USING ERRCODE = '23514';
END;
$$;
CREATE TRIGGER movimiento_inmutable BEFORE UPDATE OR DELETE ON movimientos_puntos
FOR EACH ROW EXECUTE FUNCTION proteger_movimiento();

-- Verificación diferida: saldo y secuencia deben explicar exactamente el ledger
-- al confirmar, aun si una escritura evita las validaciones de Go.
CREATE FUNCTION verificar_ledger() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE cuenta uuid; saldo_actual bigint; secuencia_actual bigint; suma numeric; cantidad bigint; maxima bigint;
BEGIN
    IF TG_TABLE_NAME = 'cuentas' THEN cuenta := NEW.id; ELSE cuenta := NEW.cuenta_id; END IF;
    SELECT saldo, ultima_secuencia INTO saldo_actual, secuencia_actual FROM cuentas WHERE id = cuenta;
    SELECT COALESCE(sum(puntos),0), count(*), COALESCE(max(secuencia),0)
      INTO suma, cantidad, maxima FROM movimientos_puntos WHERE cuenta_id = cuenta;
    IF saldo_actual <> suma OR secuencia_actual <> cantidad OR maxima <> cantidad THEN
        RAISE EXCEPTION 'Saldo o secuencia no coincide con el ledger' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER cuenta_ledger AFTER INSERT OR UPDATE ON cuentas
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_ledger();
CREATE CONSTRAINT TRIGGER movimiento_ledger AFTER INSERT ON movimientos_puntos
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_ledger();

CREATE FUNCTION verificar_operacion_completa() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF EXISTS (SELECT 1 FROM operaciones_partner WHERE partner_id = NEW.partner_id AND clave = NEW.clave AND resultado IS NULL) THEN
        RAISE EXCEPTION 'No se puede confirmar una operación incompleta' USING ERRCODE = '23514';
    END IF;
    RETURN NULL;
END;
$$;
CREATE CONSTRAINT TRIGGER operacion_completa AFTER INSERT ON operaciones_partner
DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION verificar_operacion_completa();

CREATE FUNCTION proteger_retencion() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF clock_timestamp() < OLD.conservar_hasta THEN
        RAISE EXCEPTION 'La operación debe conservarse al menos 30 días' USING ERRCODE = '23514';
    END IF;
    RETURN OLD;
END;
$$;
CREATE TRIGGER operacion_retencion BEFORE DELETE ON operaciones_partner
FOR EACH ROW EXECUTE FUNCTION proteger_retencion();

CREATE FUNCTION proteger_resultado() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.resultado IS NOT NULL OR NEW.partner_id <> OLD.partner_id OR NEW.clave <> OLD.clave
       OR NEW.huella <> OLD.huella OR NEW.creada_en <> OLD.creada_en OR NEW.conservar_hasta <> OLD.conservar_hasta THEN
        RAISE EXCEPTION 'El resultado original y su identidad son inmutables' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER operacion_resultado_inmutable BEFORE UPDATE ON operaciones_partner
FOR EACH ROW EXECUTE FUNCTION proteger_resultado();
