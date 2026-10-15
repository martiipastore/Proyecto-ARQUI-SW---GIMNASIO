// Crea un usuario por base, con permisos solo sobre su propia base.
// Lo ejecuta la imagen oficial de mongo una única vez, al crear el volumen.

function createServiceUser(dbName, user, password) {
  if (!password) {
    throw new Error(`Falta la contraseña para ${user}`);
  }
  print(`Creando usuario ${user} en ${dbName}`);
  db.getSiblingDB(dbName).createUser({
    user: user,
    pwd: password,
    roles: [{ role: 'readWrite', db: dbName }],
  });
}

createServiceUser('training_db', 'training_user', process.env.TRAINING_DB_PASSWORD);
createServiceUser('notifications_db', 'notifications_user', process.env.NOTIFICATIONS_DB_PASSWORD);
