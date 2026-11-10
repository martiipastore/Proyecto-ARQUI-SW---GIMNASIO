package amqp

import (
	"context"
	"encoding/json"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type inputStub struct {
	calls int
	last  domain.HechoBooking
}

func (s *inputStub) Aplicar(_ context.Context, h domain.HechoBooking) error {
	s.calls++
	s.last = h
	if !h.Valido() {
		return domain.ErrOperacionInvalida
	}
	return nil
}

// RN-20/38: valores del broker no pueden elegir el importe del ledger.
func TestValidarEnvelopeBooking(t *testing.T) {
	id := "00000000-0000-4000-8000-000000000001"
	base := map[string]any{"event_id": id, "event_type": "asistencia.registrada", "event_version": 1, "producer": "booking-service", "occurred_at": time.Now().UTC(), "data": map[string]any{"alumno_id": id, "reserva_id": id, "clase_id": id, "asistencia_id": id, "puntos": 500, "resultado": "ASISTIO", "origen": "INGRESO_DNI"}}
	stub := &inputStub{}
	adapter := Adapter{App: stub, EventType: "asistencia.registrada"}
	for _, value := range []int64{-100, 0, 499, 501, 2000} {
		base["data"].(map[string]any)["puntos"] = value
		body, err := json.Marshal(base)
		require.NoError(t, err)
		require.ErrorIs(t, adapter.Handle(context.Background(), body), domain.ErrOperacionInvalida)
	}
	require.Zero(t, stub.calls)
	base["data"].(map[string]any)["puntos"] = 500
	body, err := json.Marshal(base)
	require.NoError(t, err)
	require.NoError(t, adapter.Handle(context.Background(), body))
	require.Equal(t, domain.Asistencia, stub.last.Tipo)
	base["event_version"] = 2
	body, err = json.Marshal(base)
	require.NoError(t, err)
	require.ErrorIs(t, adapter.Handle(context.Background(), body), domain.ErrOperacionInvalida)
	require.True(t, Permanent(domain.ErrConflictoIdempotencia))
}
