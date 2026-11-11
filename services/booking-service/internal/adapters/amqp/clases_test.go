package amqp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

type indiceStub struct {
	recibida domain.Clase
	fallas   int
	aplicado bool
}

func (s *indiceStub) Indexar(_ context.Context, c domain.Clase) (bool, error) {
	if s.fallas > 0 {
		s.fallas--
		return false, ports.ErrNoDisponible
	}
	s.recibida = c
	return s.aplicado, nil
}

type reloj struct{ t time.Time }

func (r reloj) Ahora() time.Time { return r.t }

const valido = `{"event_id":"6f1c2a8e-3b7d-4c1e-9a2b-1d2e3f4a5b6c","event_type":"clase.actualizada","event_version":1,"occurred_at":"2026-11-10T15:00:00Z","producer":"booking-service","correlation_id":"","traceparent":"",
"data":{"clase_id":"00000000-0000-4000-8000-000000000001","version":3,"actividad_id":"00000000-0000-4000-8000-000000000002","actividad_nombre":"Zumba","musculacion":false,
"inicio":"2026-11-12T12:00:00Z","fin":"2026-11-12T13:00:00Z","profesor_id":"00000000-0000-4000-8000-000000000003","profesor_nombre":"Lucía","capacidad":20,"ocupacion":5,"estado":"PROGRAMADA"}}`

// ADR-014: el adaptador traduce clase.actualizada y mide el retraso desde el commit.
func TestClaseActualizada(t *testing.T) {
	stub := &indiceStub{aplicado: true}
	metricas := &Metricas{}
	a := Adapter{App: application.NewIndexador(stub, reloj{time.Date(2026, 11, 10, 15, 0, 1, 500_000_000, time.UTC)}), Metricas: metricas}
	require.NoError(t, a.Handle(context.Background(), []byte(valido)))
	require.EqualValues(t, 3, stub.recibida.Version)
	require.Equal(t, 5, stub.recibida.Ocupacion)
	ultimo, _, _, n := metricas.Retraso()
	require.Equal(t, 1500*time.Millisecond, ultimo)
	require.EqualValues(t, 1, n)

	stub.aplicado = false
	require.NoError(t, a.Handle(context.Background(), []byte(valido)), "versión ya indexada: se confirma sin efecto")
	require.EqualValues(t, 1, metricas.Descartados.Load())
}

// Un mensaje que no cumple el esquema es permanente: va a la DLQ.
func TestClaseActualizadaInvalida(t *testing.T) {
	a := Adapter{App: application.NewIndexador(&indiceStub{}, reloj{time.Now()})}
	for name, body := range map[string]string{
		"json_roto":       `{`,
		"otro_tipo":       strings.Replace(valido, `"clase.actualizada"`, `"asistencia.registrada"`, 1),
		"otra_version":    strings.Replace(valido, `"event_version":1`, `"event_version":2`, 1),
		"otro_productor":  strings.Replace(valido, `"booking-service"`, `"members-service"`, 1),
		"version_cero":    strings.Replace(valido, `"version":3`, `"version":0`, 1),
		"sobreocupada":    strings.Replace(valido, `"ocupacion":5`, `"ocupacion":21`, 1),
		"estado_inválido": strings.Replace(valido, `"PROGRAMADA"`, `"EN_CURSO"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			err := a.Handle(context.Background(), []byte(body))
			require.Error(t, err)
			require.True(t, Permanent(err))
		})
	}
}

// OpenSearch caído: se reintenta dentro del mensaje y, si sigue caído, vuelve a
// la cola de reintentos (no es permanente).
func TestClaseActualizadaEsperaAlIndice(t *testing.T) {
	stub := &indiceStub{fallas: 1, aplicado: true}
	a := Adapter{App: application.NewIndexador(stub, reloj{time.Now()}), Espera: 3 * time.Second}
	require.NoError(t, a.Handle(context.Background(), []byte(valido)))
	stub.fallas = 100
	a.Espera = 0
	err := a.Handle(context.Background(), []byte(valido))
	require.True(t, errors.Is(err, ports.ErrNoDisponible))
	require.False(t, Permanent(err))
}
