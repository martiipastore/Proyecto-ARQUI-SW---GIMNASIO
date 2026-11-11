package repository

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

var _ service.Repository = (*Mongo)(nil)

const (
	colEntrenamiento = "planes_entrenamiento"
	colAlimenticio   = "planes_alimenticios"
	colMediciones    = "mediciones"
	colConsultas     = "consultas"
)

// Validadores $jsonSchema: la base rechaza mediciones y consultas fuera de las
// reglas aunque un cliente esquive la capa service (CLAUDE.md regla 8, RN-31/37).
var validadores = map[string]bson.M{
	colMediciones: {"$jsonSchema": bson.M{
		"bsonType": "object",
		"required": bson.A{"_id", "alumno_id", "nutricionista_id", "fecha", "peso", "porcentaje_grasa", "porcentaje_muscular", "registrada_en"},
		"properties": bson.M{
			"fecha":               bson.M{"bsonType": "string", "pattern": `^\d{4}-\d{2}-\d{2}$`},
			"peso":                bson.M{"bsonType": "double", "minimum": 0, "exclusiveMinimum": true, "maximum": 500},
			"porcentaje_grasa":    bson.M{"bsonType": "double", "minimum": 0, "maximum": 100},
			"porcentaje_muscular": bson.M{"bsonType": "double", "minimum": 0, "maximum": 100},
			"masa_muscular":       bson.M{"bsonType": "double", "minimum": 0, "exclusiveMinimum": true},
		},
	}},
	colConsultas: {"$jsonSchema": bson.M{
		"bsonType": "object",
		"required": bson.A{"_id", "alumno_id", "pregunta", "enviada_en", "estado"},
		"properties": bson.M{
			"pregunta": bson.M{"bsonType": "string", "minLength": 1},
			"estado":   bson.M{"enum": bson.A{model.Pendiente, model.Respondida}},
		},
	}},
	colEntrenamiento: {"$jsonSchema": bson.M{"bsonType": "object", "required": bson.A{"_id", "alumno_id", "profesor_id", "estado", "dias"},
		"properties": bson.M{"estado": bson.M{"enum": bson.A{model.Vigente, model.Finalizado}}, "dias": bson.M{"bsonType": "array", "minItems": 1}}}},
	colAlimenticio: {"$jsonSchema": bson.M{"bsonType": "object", "required": bson.A{"_id", "alumno_id", "nutricionista_id", "estado"},
		"properties": bson.M{"estado": bson.M{"enum": bson.A{model.Vigente, model.Finalizado}}}}},
}

// EnsureSchema crea colecciones con validador e índices; es idempotente. El
// índice único parcial garantiza a lo sumo un plan VIGENTE por alumno (RN-28/29).
func (m *Mongo) EnsureSchema(ctx context.Context) error {
	for name, validator := range validadores {
		names, err := m.db.ListCollectionNames(ctx, bson.M{"name": name})
		if err != nil {
			return err
		}
		if len(names) == 0 {
			if err := m.db.CreateCollection(ctx, name, options.CreateCollection().SetValidator(validator)); err != nil {
				var cmd mongo.CommandError
				if !errors.As(err, &cmd) || cmd.Code != 48 { // 48 = ya existe (arranque concurrente)
					return err
				}
			}
		}
	}
	indices := map[string][]mongo.IndexModel{
		colEntrenamiento: {
			{Keys: bson.D{{Key: "alumno_id", Value: 1}}, Options: options.Index().SetName("un_vigente_por_alumno").SetUnique(true).SetPartialFilterExpression(bson.M{"estado": model.Vigente})},
			{Keys: bson.D{{Key: "alumno_id", Value: 1}, {Key: "creado_en", Value: -1}}},
		},
		colAlimenticio: {
			{Keys: bson.D{{Key: "alumno_id", Value: 1}}, Options: options.Index().SetName("un_vigente_por_alumno").SetUnique(true).SetPartialFilterExpression(bson.M{"estado": model.Vigente})},
			{Keys: bson.D{{Key: "alumno_id", Value: 1}, {Key: "creado_en", Value: -1}}},
		},
		colMediciones: {{Keys: bson.D{{Key: "alumno_id", Value: 1}, {Key: "fecha", Value: 1}, {Key: "registrada_en", Value: 1}}}},
		colConsultas:  {{Keys: bson.D{{Key: "alumno_id", Value: 1}, {Key: "enviada_en", Value: 1}}}},
	}
	for name, models := range indices {
		if _, err := m.db.Collection(name).Indexes().CreateMany(ctx, models); err != nil {
			return err
		}
	}
	return nil
}

// reemplazarVigente finaliza el vigente anterior e inserta el nuevo. Sin
// transacciones (Mongo standalone), el índice único resuelve la concurrencia:
// si otro alta ganó la carrera, se repite y la última confirmada queda VIGENTE.
func (m *Mongo) reemplazarVigente(ctx context.Context, coleccion, alumno string, doc any, ahora time.Time) error {
	col := m.db.Collection(coleccion)
	for intento := 0; intento < 3; intento++ {
		if _, err := col.UpdateMany(ctx, bson.M{"alumno_id": alumno, "estado": model.Vigente}, bson.M{"$set": bson.M{"estado": model.Finalizado, "finalizado_en": ahora}}); err != nil {
			return err
		}
		_, err := col.InsertOne(ctx, doc)
		if err == nil || !mongo.IsDuplicateKeyError(err) {
			return err
		}
	}
	return errors.New("no se pudo reemplazar el plan vigente por concurrencia")
}

func (m *Mongo) CrearPlanEntrenamiento(ctx context.Context, p model.PlanEntrenamiento, ahora time.Time) error {
	return m.reemplazarVigente(ctx, colEntrenamiento, p.AlumnoID, p, ahora)
}

func (m *Mongo) CrearPlanAlimenticio(ctx context.Context, p model.PlanAlimenticio, ahora time.Time) error {
	return m.reemplazarVigente(ctx, colAlimenticio, p.AlumnoID, p, ahora)
}

func listar[T any](ctx context.Context, col *mongo.Collection, filtro bson.M, orden bson.D) ([]T, error) {
	cur, err := col.Find(ctx, filtro, options.Find().SetSort(orden).SetLimit(500))
	if err != nil {
		return nil, err
	}
	out := []T{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Mongo) PlanesEntrenamiento(ctx context.Context, alumno string) ([]model.PlanEntrenamiento, error) {
	return listar[model.PlanEntrenamiento](ctx, m.db.Collection(colEntrenamiento), bson.M{"alumno_id": alumno}, bson.D{{Key: "creado_en", Value: -1}})
}

func (m *Mongo) PlanesAlimenticios(ctx context.Context, alumno string) ([]model.PlanAlimenticio, error) {
	return listar[model.PlanAlimenticio](ctx, m.db.Collection(colAlimenticio), bson.M{"alumno_id": alumno}, bson.D{{Key: "creado_en", Value: -1}})
}

// RegistrarMedicion solo inserta: el repositorio no ofrece modificar ni borrar (RN-31).
func (m *Mongo) RegistrarMedicion(ctx context.Context, med model.Medicion) error {
	_, err := m.db.Collection(colMediciones).InsertOne(ctx, med)
	return err
}

func (m *Mongo) Mediciones(ctx context.Context, alumno string) ([]model.Medicion, error) {
	return listar[model.Medicion](ctx, m.db.Collection(colMediciones), bson.M{"alumno_id": alumno}, bson.D{{Key: "fecha", Value: 1}, {Key: "registrada_en", Value: 1}})
}

func (m *Mongo) CrearConsulta(ctx context.Context, c model.Consulta) error {
	_, err := m.db.Collection(colConsultas).InsertOne(ctx, c)
	return err
}

func (m *Mongo) ConsultasDeAlumnos(ctx context.Context, alumnos []string, masRecientePrimero bool) ([]model.Consulta, error) {
	orden := 1
	if masRecientePrimero {
		orden = -1
	}
	return listar[model.Consulta](ctx, m.db.Collection(colConsultas), bson.M{"alumno_id": bson.M{"$in": alumnos}}, bson.D{{Key: "enviada_en", Value: orden}, {Key: "_id", Value: orden}})
}

func (m *Mongo) Consulta(ctx context.Context, id string) (model.Consulta, error) {
	var c model.Consulta
	err := m.db.Collection(colConsultas).FindOne(ctx, bson.M{"_id": id}).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return c, model.ErrNoEncontrado
	}
	return c, err
}

// Responder es una única actualización condicional: dos respuestas simultáneas
// no pueden pasar ambas (CA-34.4).
func (m *Mongo) Responder(ctx context.Context, id, nutricionista, respuesta string, ahora time.Time) (model.Consulta, error) {
	var c model.Consulta
	err := m.db.Collection(colConsultas).FindOneAndUpdate(ctx,
		bson.M{"_id": id, "estado": model.Pendiente},
		bson.M{"$set": bson.M{"estado": model.Respondida, "respuesta": respuesta, "respondida_en": ahora, "nutricionista_id": nutricionista}},
		options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&c)
	if errors.Is(err, mongo.ErrNoDocuments) {
		if _, err := m.Consulta(ctx, id); err != nil {
			return c, err
		}
		return c, model.ErrYaRespondida
	}
	return c, err
}
