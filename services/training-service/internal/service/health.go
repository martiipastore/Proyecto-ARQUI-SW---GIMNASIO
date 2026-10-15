// Package service contiene las reglas de negocio de training-service (planes de
// entrenamiento, planes alimenticios, mediciones y consultas, en etapas posteriores).
// Depende de interfaces de la capa repository, nunca del driver concreto.
package service

import (
	"context"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
)

// Pinger es lo que la capa service necesita de un repositorio para saber si está disponible.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Readiness reúne las dependencias propias del servicio.
type Readiness struct {
	db Pinger
}

// NewReadiness crea el servicio de disponibilidad.
func NewReadiness(db Pinger) *Readiness {
	return &Readiness{db: db}
}

// Checks devuelve los chequeos de /health/ready.
func (r *Readiness) Checks() []health.Check {
	return []health.Check{{Name: "mongodb", Fn: r.db.Ping}}
}
