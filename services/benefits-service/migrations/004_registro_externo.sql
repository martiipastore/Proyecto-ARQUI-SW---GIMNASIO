-- El permiso nuevo se concede explícitamente, sin abrir registro a partners existentes.
ALTER TABLE partners ADD COLUMN permite_registro boolean NOT NULL DEFAULT false;
ALTER TABLE partners ADD COLUMN max_solicitudes_minuto integer NOT NULL DEFAULT 120 CHECK(max_solicitudes_minuto BETWEEN 1 AND 60000);
CREATE TABLE solicitudes_integracion (
 partner_id uuid NOT NULL REFERENCES partners(id),
 ventana timestamptz NOT NULL,
 cantidad integer NOT NULL,
 PRIMARY KEY(partner_id,ventana)
);
CREATE INDEX solicitudes_integracion_retencion ON solicitudes_integracion(ventana);
