-- Conservar compatibilidad de integraciones existentes. Las nuevas requieren
-- identidad de evento por defecto; una excepción se configura explícitamente.
ALTER TABLE partners ADD COLUMN requiere_evento boolean NOT NULL DEFAULT false;
ALTER TABLE partners ALTER COLUMN requiere_evento SET DEFAULT true;
