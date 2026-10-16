-- La cuenta sin movimientos informa su fecha de creación (aclaración aprobada).
ALTER TABLE cuentas ADD COLUMN creada_en timestamptz NOT NULL DEFAULT now();
