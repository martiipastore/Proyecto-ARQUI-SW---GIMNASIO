-- La cuenta y su rol no dependen de contratar una membresía. Un registro externo
-- no puede elegir privilegios: las nuevas cuentas comienzan como ALUMNO.
ALTER TABLE usuarios ADD COLUMN rol text NOT NULL DEFAULT 'ALUMNO'
 CHECK (rol IN ('ADMINISTRADOR','PROFESOR','NUTRICIONISTA','ALUMNO'));
-- Sesiones opacas del borrador anterior ya no se usan. Conservar su tabla evita
-- borrar datos durante esta migración; sus tokens no autentican el nuevo código.
COMMENT ON TABLE sesiones IS 'Legado del borrador local: JWT RS256 sustituye su uso; no consultar para autorizar';
