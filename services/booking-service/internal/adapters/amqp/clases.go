// Package amqp es el adaptador de entrada del indexer: traduce clase.actualizada
// (docs/events/clase.actualizada.v1.json) y lo aplica al índice (ADR-014).
package amqp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// ErrEventoInvalido es permanente: el mensaje va a la DLQ sin reintentos.
var ErrEventoInvalido = errors.New("clase.actualizada inválido")

// Permanent clasifica errores para el consumidor de pkg/messaging.
func Permanent(err error) bool {
	return errors.Is(err, ErrEventoInvalido) || errors.Is(err, domain.ErrValidacion)
}

type sobre struct {
	EventID      string          `json:"event_id"`
	EventType    string          `json:"event_type"`
	EventVersion int             `json:"event_version"`
	OccurredAt   time.Time       `json:"occurred_at"`
	Producer     string          `json:"producer"`
	Data         json.RawMessage `json:"data"`
}

type datos struct {
	ClaseID         string    `json:"clase_id"`
	Version         int64     `json:"version"`
	ActividadID     string    `json:"actividad_id"`
	ActividadNombre string    `json:"actividad_nombre"`
	Musculacion     bool      `json:"musculacion"`
	Inicio          time.Time `json:"inicio"`
	Fin             time.Time `json:"fin"`
	ProfesorID      string    `json:"profesor_id"`
	ProfesorNombre  string    `json:"profesor_nombre"`
	Capacidad       int       `json:"capacidad"`
	Ocupacion       int       `json:"ocupacion"`
	Estado          string    `json:"estado"`
}

// Metricas son contadores de instancia expuestos en /metrics del indexer.
type Metricas struct {
	Aplicados, Descartados atomic.Int64
	mu                     sync.Mutex
	ultimo, maximo, suma   time.Duration
	cantidad               int64
}

func (m *Metricas) registrar(r application.ResultadoIndexado) {
	if !r.Aplicado {
		m.Descartados.Add(1)
		return
	}
	m.Aplicados.Add(1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ultimo, m.suma, m.cantidad = r.Retraso, m.suma+r.Retraso, m.cantidad+1
	if r.Retraso > m.maximo {
		m.maximo = r.Retraso
	}
}

// Retraso devuelve último, máximo y promedio desde el commit hasta indexar.
func (m *Metricas) Retraso() (ultimo, maximo, promedio time.Duration, cantidad int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cantidad > 0 {
		promedio = m.suma / time.Duration(m.cantidad)
	}
	return m.ultimo, m.maximo, promedio, m.cantidad
}

// Adapter decodifica y delega en el indexador.
type Adapter struct {
	App      *application.Indexador
	Metricas *Metricas
	// Espera reintenta dentro del mismo mensaje mientras OpenSearch no responde,
	// antes de devolverlo a la cola de reintentos de RabbitMQ.
	Espera time.Duration
}

func traducir(body []byte) (domain.EventoClase, error) {
	var s sobre
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&s); err != nil || s.EventType != "clase.actualizada" || s.EventVersion != 1 || s.Producer != "booking-service" || len(s.Data) == 0 {
		return domain.EventoClase{}, ErrEventoInvalido
	}
	var d datos
	if err := json.Unmarshal(s.Data, &d); err != nil {
		return domain.EventoClase{}, ErrEventoInvalido
	}
	e := domain.EventoClase{OcurridoEn: s.OccurredAt, Clase: domain.Clase{ID: d.ClaseID, Version: d.Version, ActividadID: d.ActividadID, ActividadNombre: d.ActividadNombre,
		Musculacion: d.Musculacion, Inicio: d.Inicio.UTC(), Fin: d.Fin.UTC(), ProfesorID: d.ProfesorID, ProfesorNombre: d.ProfesorNombre,
		Capacidad: d.Capacidad, Ocupacion: d.Ocupacion, Estado: d.Estado}}
	if !e.Valido() {
		return domain.EventoClase{}, ErrEventoInvalido
	}
	return e, nil
}

func (a Adapter) Handle(ctx context.Context, body []byte) error {
	e, err := traducir(body)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(a.Espera)
	for {
		r, err := a.App.Aplicar(ctx, e)
		if err == nil {
			if a.Metricas != nil {
				a.Metricas.registrar(r)
			}
			return nil
		}
		if !errors.Is(err, ports.ErrNoDisponible) || time.Now().After(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(time.Second):
		}
	}
}
