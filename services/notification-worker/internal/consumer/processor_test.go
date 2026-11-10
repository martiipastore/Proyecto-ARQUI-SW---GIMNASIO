package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/mailer"
	"github.com/GuadiViti/Arquitectura-de-Software/services/notification-worker/internal/store"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mongodb"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type testMailer struct {
	calls atomic.Int64
	fail  bool
}

func (m *testMailer) Send(context.Context, mailer.Email) error {
	m.calls.Add(1)
	if m.fail {
		return errors.New("SMTP caído")
	}
	return nil
}

func fixture() Event {
	e := Event{ID: "00000000-0000-4000-8000-000000000001", Type: "membresia.activada", Version: 1, Occurred: time.Now().UTC()}
	e.Data.Membership = "00000000-0000-4000-8000-000000000002"
	e.Data.Student = "00000000-0000-4000-8000-000000000003"
	e.Data.Name = "Fixture"
	e.Data.Email = "fixture@example.test"
	e.Data.MembershipType = "MENSUAL"
	e.Data.Start = "2026-11-10"
	e.Data.End = "2026-12-10"
	e.Data.Operation = "ASIGNACION"
	return e
}

// RN-05 y ADR-005: mensajes no procesables no deben tocar SMTP ni MongoDB.
func TestMensajesInvalidos(t *testing.T) {
	for _, change := range []func(*Event){func(e *Event) { e.Version = 99 }, func(e *Event) { e.ID = "invalido" }, func(e *Event) { e.Data.Email = "x@example.test\r\nBcc: otro@example.test" }, func(e *Event) { e.Data.End = "2026-01-01" }, func(e *Event) { e.Data.Operation = "OTRA" }} {
		e := fixture()
		change(&e)
		body, err := json.Marshal(e)
		require.NoError(t, err)
		require.ErrorIs(t, (Processor{}).Process(context.Background(), body), ErrInvalid)
	}
	require.ErrorIs(t, (Processor{}).Process(context.Background(), []byte("no-json")), ErrInvalid)
}

// RN-05: MongoDB real, error SMTP, recuperación, replay semántico y contenido
// diferente bajo el mismo ID. Ningún mock reemplaza la persistencia.
func TestNotificacionDurableMongo(t *testing.T) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Docker requerido: %v", r)
			}
		}()
		testcontainers.SkipIfProviderIsNotHealthy(t)
	}()
	ctx := context.Background()
	ctr, err := mongodb.Run(ctx, "mongo:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	uri, err := ctr.ConnectionString(ctx)
	require.NoError(t, err)
	db, err := store.NewMongo(uri, "notifications_db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close(context.Background()) })
	m := &testMailer{fail: true}
	p := Processor{Store: db, Mailer: m}
	e := fixture()
	body, err := json.Marshal(e)
	require.NoError(t, err)
	require.Error(t, p.Process(ctx, body))
	m.fail = false
	require.NoError(t, p.Process(ctx, body))
	require.NoError(t, p.Process(ctx, body))
	formatted, err := json.MarshalIndent(e, "", "  ")
	require.NoError(t, err)
	require.NoError(t, p.Process(ctx, formatted))
	e.Occurred = e.Occurred.In(time.FixedZone("fixture-offset", -3*60*60))
	withOffset, err := json.Marshal(e)
	require.NoError(t, err)
	require.NoError(t, p.Process(ctx, withOffset))
	require.EqualValues(t, 2, m.calls.Load())
	var history struct {
		Results []struct {
			State string `bson:"estado"`
		} `bson:"resultados"`
	}
	require.NoError(t, db.Database().Collection("notifications").FindOne(ctx, bson.M{"_id": e.ID}).Decode(&history))
	require.Len(t, history.Results, 2)
	require.Equal(t, "PENDIENTE", history.Results[0].State)
	require.Equal(t, "ENVIADA", history.Results[1].State)
	e.Data.Email = "different@example.test"
	changed, err := json.Marshal(e)
	require.NoError(t, err)
	require.Error(t, p.Process(ctx, changed))
	require.EqualValues(t, 2, m.calls.Load())
	e.ID = "00000000-0000-4000-8000-000000000004"
	body, err = json.Marshal(e)
	require.NoError(t, err)
	claimed, err := db.Claim(ctx, e.ID, body)
	require.NoError(t, err)
	require.True(t, claimed)
	claimed, err = db.Claim(ctx, e.ID, body)
	require.Error(t, err)
	require.False(t, claimed)
}
