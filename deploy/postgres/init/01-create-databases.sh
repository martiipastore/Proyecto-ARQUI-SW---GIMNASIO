#!/bin/sh
# Crea una base y un usuario propio por servicio (Database per Service, variante
# "una instancia, una base por servicio"). Cada usuario es dueño solo de su base y
# PUBLIC no puede conectarse a ninguna: un servicio no puede abrir la base de otro.
#
# Lo ejecuta la imagen oficial de postgres una única vez, al crear el volumen.
# (Sin "set -u": si el archivo no es ejecutable, la imagen lo incluye con "source"
# en su propio shell y "-u" podría romper el entrypoint.)
set -e

create_db() {
  db="$1"
  user="$2"
  password="$3"
  echo "Creando base ${db} con el usuario ${user}"
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname postgres \
    -v db="$db" -v usr="$user" -v pwd="$password" <<'SQL'
CREATE ROLE :"usr" LOGIN PASSWORD :'pwd';
CREATE DATABASE :"db" OWNER :"usr";
REVOKE ALL ON DATABASE :"db" FROM PUBLIC;
GRANT CONNECT, TEMPORARY ON DATABASE :"db" TO :"usr";
SQL
  psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$db" \
    -v usr="$user" <<'SQL'
REVOKE ALL ON SCHEMA public FROM PUBLIC;
ALTER SCHEMA public OWNER TO :"usr";
SQL
}

create_db members_db  members_user  "$MEMBERS_DB_PASSWORD"
create_db booking_db  booking_user  "$BOOKING_DB_PASSWORD"
create_db benefits_db benefits_user "$BENEFITS_DB_PASSWORD"
