CREATE TABLE usuarios (
 id uuid PRIMARY KEY,
 nombre text NOT NULL CHECK (char_length(nombre) BETWEEN 1 AND 120),
 email text NOT NULL UNIQUE CHECK (email=lower(btrim(email))),
 password_hash bytea,
 registro_externo boolean NOT NULL DEFAULT false,
 creado_en timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE identidades_externas (
 partner_id uuid NOT NULL,
 external_user_id text NOT NULL CHECK(external_user_id ~ '^[A-Za-z0-9._:@-]{1,64}$'),
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 verificada boolean NOT NULL DEFAULT false,
 PRIMARY KEY(partner_id, external_user_id),
 UNIQUE(partner_id,external_user_id,usuario_id),
 UNIQUE(partner_id, usuario_id)
);
CREATE TABLE registros_externos (
 id uuid PRIMARY KEY,
 partner_id uuid NOT NULL,
 evento_id text NOT NULL,
 clave text NOT NULL,
 huella bytea NOT NULL CHECK(octet_length(huella)=32),
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 external_user_id text NOT NULL,
 plataforma text NOT NULL,
 puntos bigint NOT NULL CHECK(puntos BETWEEN 0 AND 1000000),
 motivo text NOT NULL,
 referencia text NOT NULL,
 estado text NOT NULL CHECK(estado IN ('pending_verification','ready','completed')),
 token_hash bytea UNIQUE CHECK(octet_length(token_hash)=32),
 token_cifrado bytea,
 token_expira timestamptz,
 token_consumido boolean NOT NULL DEFAULT false,
 saldo_acreditado bigint,
 intentos integer NOT NULL DEFAULT 0,
 proximo_intento timestamptz NOT NULL DEFAULT now(),
 creado_en timestamptz NOT NULL DEFAULT now(),
 UNIQUE(partner_id,evento_id),
 UNIQUE(partner_id,clave),
 FOREIGN KEY(partner_id,external_user_id,usuario_id) REFERENCES identidades_externas(partner_id,external_user_id,usuario_id)
);
CREATE INDEX registros_pendientes ON registros_externos(proximo_intento) WHERE estado='ready';
CREATE TABLE claves_registro (
 partner_id uuid NOT NULL,
 clave text NOT NULL,
 registro_id uuid NOT NULL REFERENCES registros_externos(id),
 PRIMARY KEY(partner_id,clave)
);
CREATE TABLE correos_pendientes (
 id bigserial PRIMARY KEY,
 registro_id uuid NOT NULL REFERENCES registros_externos(id),
 tipo text NOT NULL CHECK(tipo IN ('verification','welcome')),
 enviado_en timestamptz,
 intentos integer NOT NULL DEFAULT 0,
 proximo_intento timestamptz NOT NULL DEFAULT now(),
 UNIQUE(registro_id,tipo)
);
CREATE TABLE sesiones (
 token_hash bytea PRIMARY KEY CHECK(octet_length(token_hash)=32),
 usuario_id uuid NOT NULL REFERENCES usuarios(id),
 expira_en timestamptz NOT NULL
);
CREATE TABLE cuotas_autenticacion (
 clave text NOT NULL,
 ventana timestamptz NOT NULL,
 solicitudes integer NOT NULL,
 PRIMARY KEY(clave,ventana)
);
