package application

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/stretchr/testify/require"
)

type secretoFijo struct{}

func (secretoFijo) NewSecret() (string, error) { return "clave-emitida-una-vez", nil }

type idsUUID struct{}

func (idsUUID) NewID() (string, error) { return "6f1c2a8e-3b7d-4c1e-9a2b-1d2e3f4a5b6c", nil }

type adminStore struct {
	ports.AdministracionClub
	hash    [32]byte
	creada  domain.ConfiguracionIntegracion
	pedido  ports.ReversionPedida
	llamado bool
}

func (s *adminStore) CrearIntegracion(_ context.Context, a ports.Auditoria, id string, c domain.ConfiguracionIntegracion, h [32]byte) (domain.Integracion, error) {
	s.llamado, s.hash, s.creada = true, h, c
	return domain.Integracion{ID: id, Config: c}, nil
}
func (s *adminStore) RotarCredencial(_ context.Context, _ ports.Auditoria, _ string, h [32]byte) error {
	s.llamado, s.hash = true, h
	return nil
}
func (s *adminStore) ListarIntegraciones(context.Context) ([]domain.Integracion, error) {
	s.llamado = true
	return nil, nil
}
func (s *adminStore) Revertir(_ context.Context, p ports.ReversionPedida, _ ports.AplicarReversion) (domain.ResultadoReversion, bool, error) {
	s.llamado, s.pedido = true, p
	return domain.ResultadoReversion{}, false, nil
}

const adminID = "0b6a1f2e-9c8d-4e7f-8a6b-5c4d3e2f1a0b"

var admin = Actor{ID: adminID, Rol: domain.RolAdministrador}

// ADR-006 / regla 10: Benefits vuelve a autorizar; otro rol no llega al puerto.
func TestAdministracion_SoloAdministrador(t *testing.T) {
	for _, actor := range []Actor{{ID: adminID, Rol: "ALUMNO"}, {ID: adminID, Rol: "PROFESOR"}, {ID: "no-uuid", Rol: domain.RolAdministrador}, {}} {
		store := &adminStore{}
		app := NewAdministracion(store, fixedClock{}, idsUUID{}, secretoFijo{})
		_, err := app.ListarIntegraciones(context.Background(), actor)
		require.ErrorIs(t, err, domain.ErrSinPermiso)
		_, _, err = app.Revertir(context.Background(), actor, idsUUIDValue, "Carga por error", "clave-0001")
		require.ErrorIs(t, err, domain.ErrSinPermiso)
		require.False(t, store.llamado)
	}
}

const idsUUIDValue = "6f1c2a8e-3b7d-4c1e-9a2b-1d2e3f4a5b6c"

// CA-10.1: el alta queda ACTIVA y la key se devuelve una vez; se guarda solo su hash.
func TestAdministracion_AltaYRotacionEmitenKeyUnaVez(t *testing.T) {
	store := &adminStore{}
	app := NewAdministracion(store, fixedClock{}, idsUUID{}, secretoFijo{})
	out, err := app.CrearIntegracion(context.Background(), admin, domain.ConfiguracionIntegracion{Nombre: "  Estudiario  ", MaxPuntosOperacion: 1000, MaxSolicitudesMinuto: 60})
	require.NoError(t, err)
	require.Equal(t, "clave-emitida-una-vez", out.APIKey)
	require.Equal(t, sha256.Sum256([]byte("clave-emitida-una-vez")), store.hash)
	require.True(t, store.creada.Activo)
	require.Equal(t, "Estudiario", store.creada.Nombre)

	_, err = app.CrearIntegracion(context.Background(), admin, domain.ConfiguracionIntegracion{Nombre: "x", MaxPuntosOperacion: 1000, MaxSolicitudesMinuto: 60})
	require.ErrorIs(t, err, domain.ErrOperacionInvalida)

	store.hash = [32]byte{}
	rot, err := app.RotarCredencial(context.Background(), admin, idsUUIDValue)
	require.NoError(t, err)
	require.Equal(t, sha256.Sum256([]byte(rot.APIKey)), store.hash)
	_, err = app.RotarCredencial(context.Background(), admin, "no-uuid")
	require.ErrorIs(t, err, domain.ErrIntegracionNoEncontrada)
}

// HU-11 / regla 9: Idempotency-Key obligatoria y huella sobre movimiento y motivo.
func TestAdministracion_ReversionIdempotente(t *testing.T) {
	store := &adminStore{}
	app := NewAdministracion(store, fixedClock{}, idsUUID{}, secretoFijo{})
	_, _, err := app.Revertir(context.Background(), admin, idsUUIDValue, "Carga por error", "corta")
	require.ErrorIs(t, err, domain.ErrOperacionInvalida)
	_, _, err = app.Revertir(context.Background(), admin, idsUUIDValue, "no", "clave-0001")
	require.ErrorIs(t, err, domain.ErrOperacionInvalida)
	require.False(t, store.llamado)

	_, _, err = app.Revertir(context.Background(), admin, "6F1C2A8E-3B7D-4C1E-9A2B-1D2E3F4A5B6C", "  Carga por error ", "clave-0001")
	require.NoError(t, err)
	primera := store.pedido
	require.Equal(t, idsUUIDValue, primera.MovimientoID)
	require.Equal(t, adminID, primera.AdministradorID)
	_, _, err = app.Revertir(context.Background(), admin, idsUUIDValue, "Carga por error", "clave-0001")
	require.NoError(t, err)
	require.Equal(t, primera.Huella, store.pedido.Huella, "mismos datos, misma huella")
	_, _, err = app.Revertir(context.Background(), admin, idsUUIDValue, "Otro motivo", "clave-0001")
	require.NoError(t, err)
	require.NotEqual(t, primera.Huella, store.pedido.Huella)
}
