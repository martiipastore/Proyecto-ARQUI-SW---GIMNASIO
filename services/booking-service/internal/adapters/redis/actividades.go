// Package redis es el adaptador de salida de la caché del catálogo de
// actividades (etapa 9, ADR-007, D-88). La copia no es autoritativa: si Redis
// falla, el caso de uso lee PostgreSQL.
package redis

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Clave versionada: un cambio de formato usa otra clave y no lee datos viejos.
const Clave = "booking:actividades:v1"

var _ ports.CacheActividades = (*Actividades)(nil)

// Metricas son contadores de instancia para /metrics (hits, misses, errores y
// tiempo acumulado de las operaciones contra Redis).
type Metricas struct {
	Aciertos, Fallos, Errores, Invalidaciones atomic.Int64
	Operaciones, MicrosegundosRedis           atomic.Int64
}

// Actividades implementa ports.CacheActividades sobre Redis.
type Actividades struct {
	Client   *goredis.Client
	TTL      time.Duration
	Timeout  time.Duration
	Metricas *Metricas
}

type actividad struct {
	ID              string `json:"id"`
	Nombre          string `json:"nombre"`
	CapacidadMaxima int    `json:"capacidad_maxima"`
	Musculacion     bool   `json:"musculacion"`
	Activa          bool   `json:"activa"`
}

// medir acota cada operación: una caché lenta no debe demorar más que la base.
func (a *Actividades) medir(ctx context.Context, fn func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()
	inicio := time.Now()
	err := fn(ctx)
	a.Metricas.Operaciones.Add(1)
	a.Metricas.MicrosegundosRedis.Add(time.Since(inicio).Microseconds())
	if err != nil && !errors.Is(err, goredis.Nil) {
		a.Metricas.Errores.Add(1)
	}
	return err
}

func (a *Actividades) Leer(ctx context.Context) ([]domain.Actividad, bool, error) {
	var raw []byte
	err := a.medir(ctx, func(ctx context.Context) error {
		var err error
		raw, err = a.Client.Get(ctx, Clave).Bytes()
		return err
	})
	if errors.Is(err, goredis.Nil) {
		a.Metricas.Fallos.Add(1)
		return nil, false, nil
	}
	if err != nil {
		a.Metricas.Fallos.Add(1)
		return nil, false, err
	}
	var copia []actividad
	if err := json.Unmarshal(raw, &copia); err != nil {
		// Una copia ilegible se trata como miss y se reemplaza.
		a.Metricas.Fallos.Add(1)
		return nil, false, nil
	}
	a.Metricas.Aciertos.Add(1)
	out := make([]domain.Actividad, 0, len(copia))
	for _, c := range copia {
		out = append(out, domain.Actividad{ID: c.ID, Nombre: c.Nombre, CapacidadMaxima: c.CapacidadMaxima, Musculacion: c.Musculacion, Activa: c.Activa})
	}
	return out, true, nil
}

func (a *Actividades) Guardar(ctx context.Context, items []domain.Actividad) error {
	copia := make([]actividad, 0, len(items))
	for _, i := range items {
		copia = append(copia, actividad{ID: i.ID, Nombre: i.Nombre, CapacidadMaxima: i.CapacidadMaxima, Musculacion: i.Musculacion, Activa: i.Activa})
	}
	raw, err := json.Marshal(copia)
	if err != nil {
		return err
	}
	return a.medir(ctx, func(ctx context.Context) error { return a.Client.Set(ctx, Clave, raw, a.TTL).Err() })
}

func (a *Actividades) Invalidar(ctx context.Context) error {
	err := a.medir(ctx, func(ctx context.Context) error { return a.Client.Del(ctx, Clave).Err() })
	if err == nil {
		a.Metricas.Invalidaciones.Add(1)
	}
	return err
}

// Ping es el chequeo de readiness: Redis es dependencia parcial (ADR-007).
func (a *Actividades) Ping(ctx context.Context) error { return a.Client.Ping(ctx).Err() }
