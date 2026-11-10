package store

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func (m *Mongo) notifications() *mongo.Collection {
	return m.db.Collection("notifications", options.Collection().SetWriteConcern(writeconcern.Majority()))
}

// Claim usa _id único y un documento atómico (no requiere replica set).
// Solo el proceso que gana el lease puede enviar. La identidad se conserva
// indefinidamente para impedir que el replay cree otra notificación lógica.
func (m *Mongo) Claim(ctx context.Context, id string, body []byte) (bool, error) {
	c := m.notifications()
	now := time.Now().UTC()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	_, err := c.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$setOnInsert": bson.M{"estado": "PENDIENTE", "intentos": 0, "creada_en": now, "huella": digest}}, options.UpdateOne().SetUpsert(true))
	if err != nil && !mongo.IsDuplicateKeyError(err) {
		return false, err
	}
	var record struct {
		State       string `bson:"estado"`
		Fingerprint string `bson:"huella"`
	}
	if err = c.FindOne(ctx, bson.M{"_id": id}).Decode(&record); err != nil {
		return false, err
	}
	if record.Fingerprint != digest {
		return false, errors.New("event_id con contenido diferente")
	}
	if record.State == "ENVIADA" {
		return false, nil
	}
	res, err := c.UpdateOne(ctx, bson.M{"_id": id, "estado": bson.M{"$ne": "ENVIADA"}, "$or": bson.A{bson.M{"lease_hasta": bson.M{"$exists": false}}, bson.M{"lease_hasta": bson.M{"$lte": now}}}}, bson.M{"$set": bson.M{"estado": "PROCESANDO", "lease_hasta": now.Add(20 * time.Second), "actualizada_en": now}, "$inc": bson.M{"intentos": 1}})
	if err != nil {
		return false, err
	}
	if res.ModifiedCount == 0 {
		return false, errors.New("notificación en procesamiento")
	}
	return true, nil
}

func (m *Mongo) Finish(ctx context.Context, id string, sent bool) error {
	state := "PENDIENTE"
	if sent {
		state = "ENVIADA"
	}
	now := time.Now().UTC()
	res, err := m.notifications().UpdateOne(ctx, bson.M{"_id": id, "estado": "PROCESANDO"}, bson.M{"$set": bson.M{"estado": state, "actualizada_en": now}, "$unset": bson.M{"lease_hasta": ""}, "$push": bson.M{"resultados": bson.M{"estado": state, "registrada_en": now}}})
	if err != nil {
		return err
	}
	if res.MatchedCount != 1 {
		return errors.New("notificación sin procesamiento activo")
	}
	return nil
}

// RecordFailure evita guardar cuerpos ni información personal en el diagnóstico.
func (m *Mongo) RecordFailure(ctx context.Context, body []byte, attempt int) error {
	id := fmt.Sprintf("%x", sha256.Sum256(body))
	_, err := m.db.Collection("failed_messages", options.Collection().SetWriteConcern(writeconcern.Majority())).UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": bson.M{"estado": "DLQ", "intentos": attempt, "actualizada_en": time.Now().UTC()}}, options.UpdateOne().SetUpsert(true))
	return err
}
