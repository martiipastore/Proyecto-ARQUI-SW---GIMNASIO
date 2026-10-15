// Package repository es la capa de acceso a datos de training-service (training_db en MongoDB).
// Solo esta capa conoce el driver de Mongo; la capa service depende de sus interfaces.
package repository

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// Mongo envuelve el cliente y la base training_db.
type Mongo struct {
	client *mongo.Client
	db     *mongo.Database
}

// NewMongo crea el cliente. El driver conecta en segundo plano, así el servicio
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

// Database devuelve la base del servicio.
func (m *Mongo) Database() *mongo.Database {
	return m.db
}

// Close cierra las conexiones.
func (m *Mongo) Close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}
