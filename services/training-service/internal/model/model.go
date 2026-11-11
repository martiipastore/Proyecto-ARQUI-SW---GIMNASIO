package model

import (
	"errors"
	"time"
)

// Roles que inyecta el gateway (ADR-006).
const (
	RolAlumno        = "ALUMNO"
	RolProfesor      = "PROFESOR"
	RolNutricionista = "NUTRICIONISTA"
)

// Estados (SPEC §7).
const (
	Vigente     = "VIGENTE"
	Finalizado  = "FINALIZADO"
	Pendiente   = "PENDIENTE"
	Respondida  = "RESPONDIDA"
	FechaLayout = "2006-01-02"
)

// Errores de negocio; el handler los traduce con el catálogo de ARCHITECTURE §12.1.
var (
	ErrValidacion       = errors.New("datos inválidos")
	ErrSinPermiso       = errors.New("sin permiso sobre el alumno")
	ErrNoEncontrado     = errors.New("no encontrado")
	ErrSinNutricionista = errors.New("el alumno no tiene nutricionista asignado")
	ErrYaRespondida     = errors.New("la consulta ya fue respondida")
	ErrNoDisponible     = errors.New("dependencia no disponible")
)

// Actor es la identidad verificada por el gateway.
type Actor struct{ ID, Rol string }

// Ejercicio planificado (SPEC §8, CA-17.1).
type Ejercicio struct {
	Nombre           string  `json:"nombre" bson:"nombre"`
	GrupoMuscular    string  `json:"grupo_muscular" bson:"grupo_muscular"`
	Series           int     `json:"series" bson:"series"`
	Repeticiones     int     `json:"repeticiones" bson:"repeticiones"`
	PesoSugerido     float64 `json:"peso_sugerido" bson:"peso_sugerido"`
	DescansoSegundos int     `json:"descanso_segundos" bson:"descanso_segundos"`
	Observaciones    string  `json:"observaciones" bson:"observaciones"`
}

// DiaRutina agrupa ejercicios ("Día A").
type DiaRutina struct {
	Nombre      string      `json:"nombre" bson:"nombre"`
	Descripcion string      `json:"descripcion" bson:"descripcion"`
	Ejercicios  []Ejercicio `json:"ejercicios" bson:"ejercicios"`
}

// PlanEntrenamiento (RN-28). EstadoEfectivo se calcula al responder.
type PlanEntrenamiento struct {
	ID            string      `json:"id" bson:"_id"`
	AlumnoID      string      `json:"alumno_id" bson:"alumno_id"`
	ProfesorID    string      `json:"profesor_id" bson:"profesor_id"`
	Objetivo      string      `json:"objetivo" bson:"objetivo"`
	VigenciaDesde string      `json:"vigencia_desde" bson:"vigencia_desde"`
	VigenciaHasta string      `json:"vigencia_hasta" bson:"vigencia_hasta"`
	Observaciones string      `json:"observaciones" bson:"observaciones"`
	Dias          []DiaRutina `json:"dias" bson:"dias"`
	Estado        string      `json:"estado" bson:"estado"`
	CreadoEn      time.Time   `json:"creado_en" bson:"creado_en"`
	FinalizadoEn  *time.Time  `json:"finalizado_en" bson:"finalizado_en,omitempty"`
}

// PlanAlimenticio (RN-29, CA-18.1).
type PlanAlimenticio struct {
	ID              string     `json:"id" bson:"_id"`
	AlumnoID        string     `json:"alumno_id" bson:"alumno_id"`
	NutricionistaID string     `json:"nutricionista_id" bson:"nutricionista_id"`
	Fecha           string     `json:"fecha" bson:"fecha"`
	Desayuno        string     `json:"desayuno" bson:"desayuno"`
	Colacion        string     `json:"colacion" bson:"colacion"`
	Almuerzo        string     `json:"almuerzo" bson:"almuerzo"`
	Merienda        string     `json:"merienda" bson:"merienda"`
	Cena            string     `json:"cena" bson:"cena"`
	Observaciones   string     `json:"observaciones" bson:"observaciones"`
	Estado          string     `json:"estado" bson:"estado"`
	CreadoEn        time.Time  `json:"creado_en" bson:"creado_en"`
	FinalizadoEn    *time.Time `json:"finalizado_en" bson:"finalizado_en,omitempty"`
}

// Medicion es inmutable (RN-31): no hay operación de modificación ni de borrado.
type Medicion struct {
	ID                 string    `json:"id" bson:"_id"`
	AlumnoID           string    `json:"alumno_id" bson:"alumno_id"`
	NutricionistaID    string    `json:"nutricionista_id" bson:"nutricionista_id"`
	Fecha              string    `json:"fecha" bson:"fecha"`
	Peso               float64   `json:"peso" bson:"peso"`
	PorcentajeGrasa    float64   `json:"porcentaje_grasa" bson:"porcentaje_grasa"`
	PorcentajeMuscular float64   `json:"porcentaje_muscular" bson:"porcentaje_muscular"`
	MasaMuscular       *float64  `json:"masa_muscular" bson:"masa_muscular,omitempty"`
	Observaciones      string    `json:"observaciones" bson:"observaciones"`
	RegistradaEn       time.Time `json:"registrada_en" bson:"registrada_en"`
}

// Consulta nutricional (RN-37). NutricionistaID es quien respondió; una
// consulta PENDIENTE no queda atada a un nutricionista, así pasa al nuevo si
// cambia la asignación.
type Consulta struct {
	ID              string     `json:"id" bson:"_id"`
	AlumnoID        string     `json:"alumno_id" bson:"alumno_id"`
	Pregunta        string     `json:"pregunta" bson:"pregunta"`
	EnviadaEn       time.Time  `json:"enviada_en" bson:"enviada_en"`
	Estado          string     `json:"estado" bson:"estado"`
	Respuesta       *string    `json:"respuesta" bson:"respuesta,omitempty"`
	RespondidaEn    *time.Time `json:"respondida_en" bson:"respondida_en,omitempty"`
	NutricionistaID *string    `json:"nutricionista_id" bson:"nutricionista_id,omitempty"`
}

// Asignacion es lo que members informa de una asignación vigente (RN-30).
type Asignacion struct {
	ProfesionalID, ProfesionalRol, AlumnoID string
	AlumnoNombre, ProfesionalNombre         string
}

// PlanesEntrenamiento es el vigente más el historial de un alumno.
type PlanesEntrenamiento struct {
	Vigente   *PlanEntrenamiento  `json:"vigente"`
	Historial []PlanEntrenamiento `json:"historial"`
}

// PlanesAlimenticios es el vigente más el historial de un alumno.
type PlanesAlimenticios struct {
	Vigente   *PlanAlimenticio  `json:"vigente"`
	Historial []PlanAlimenticio `json:"historial"`
}
