// Package store guarda en notifications_db (MongoDB) los mensajes procesados,
// para que el consumidor sea idempotente. Las colecciones se agregan junto con
// el consumidor de membresia.activada.
package store

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Mongo envuelve el cliente y la base notifications_db.
type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongo crea el cliente. El driver conecta en segundo plano, así el worker
// arranca aunque Mongo todavía no esté disponible (y /health/ready lo informa).
func NewMongo(uri, database string) (*Mongo, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("crear cliente de MongoDB: %w", err)
	}
	return &Mongo{client: client, db: client.Database(database)}, nil
}

// Ping verifica que MongoDB responde.
func (m *Mongo) Ping(ctx context.Context) error {
	return m.client.Ping(ctx, readpref.Primary())
}

// Database devuelve la base del worker.
func (m *Mongo) Database() *mongo.Database {
	return m.db
}

// Close cierra las conexiones.
func (m *Mongo) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}
