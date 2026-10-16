package application

import (
	"context"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/stretchr/testify/require"
)

type fixedClock struct{}

func (fixedClock) Now() time.Time { return time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC) }

type fixedIDs struct{}

func (fixedIDs) NewID() (string, error) { return "movimiento", nil }

type puertoPrueba struct {
	operacion ports.OperacionGuardada
	lectura   ports.SaldoConsultado
	hash      string
	guardados int
	final     domain.ResultadoOperacion
}

func (p *puertoPrueba) EnTransaccion(ctx context.Context, fn func(ports.Transaction) error) error {
	return fn(p)
}
func (p *puertoPrueba) PartnerActivo(context.Context, string) (bool, error) { return true, nil }
func (p *puertoPrueba) TomarOperacion(_ context.Context, _, _, hash string, _ time.Time) (ports.OperacionGuardada, error) {
	p.hash = hash
	return p.operacion, nil
}
func (p *puertoPrueba) CuentaVinculada(context.Context, string, string) (domain.CuentaBeneficios, error) {
	return domain.CuentaBeneficios{}, domain.ErrCuentaNoVinculada
}
func (p *puertoPrueba) GuardarMovimiento(context.Context, domain.CuentaBeneficios, domain.MovimientoPuntos) error {
	p.guardados++
	return nil
}
func (p *puertoPrueba) CompletarOperacion(_ context.Context, _, _ string, r domain.ResultadoOperacion, _ string, _ []byte) error {
	p.final = r
	return nil
}
func (p *puertoPrueba) ConsultarSaldo(context.Context, string, string) (ports.SaldoConsultado, error) {
	return p.lectura, nil
}
func (p *puertoPrueba) ListarMovimientos(context.Context, string, string, int, int) (ports.MovimientosConsultados, error) {
	return ports.MovimientosConsultados{}, nil
}

func comando() AcreditarPuntos {
	return AcreditarPuntos{PartnerID: "partner", ExternalUserID: "alumno-1", Clave: "clave-0001", Puntos: 100, Motivo: "Compra", Referencia: "ref-1"}
}

// RN-26: el adaptador entrega hechos; la aplicación decide el permiso.
func TestRN26_LecturaSoloDeCuentaVinculada(t *testing.T) {
	p := &puertoPrueba{lectura: ports.SaldoConsultado{PartnerActivo: true}}
	b := New(p, fixedClock{}, fixedIDs{})
	_, err := b.ConsultarSaldo(context.Background(), "partner", "alumno-1")
	require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
	p.lectura = ports.SaldoConsultado{PartnerActivo: false, CuentaVinculada: true, Saldo: 100}
	_, err = b.ConsultarSaldo(context.Background(), "partner", "alumno-1")
	require.ErrorIs(t, err, domain.ErrPartnerNoAutorizado)
}

// RN-27/CL-08b: un rechazo también queda disponible para replay.
func TestRN27_RechazoYReplaySinNuevosEfectos(t *testing.T) {
	p := &puertoPrueba{operacion: ports.OperacionGuardada{Nueva: true}}
	b := New(p, fixedClock{}, fixedIDs{})
	primera, err := b.Acreditar(context.Background(), comando())
	require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
	require.Equal(t, domain.CuentaNoEncontrada, p.final.Rechazo)
	p.operacion = ports.OperacionGuardada{Huella: p.hash, Resultado: p.final}
	repetida, err := b.Acreditar(context.Background(), comando())
	require.ErrorIs(t, err, domain.ErrCuentaNoVinculada)
	require.True(t, repetida.Repetida)
	require.Equal(t, primera.Resultado, repetida.Resultado)
	require.Zero(t, p.guardados)
	otro := comando()
	otro.Puntos++
	_, err = b.Acreditar(context.Background(), otro)
	require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
}

func TestRN27_ClaveInvalidaNoIniciaOperacion(t *testing.T) {
	p := &puertoPrueba{}
	entrada := comando()
	entrada.Clave = "corta"
	_, err := New(p, fixedClock{}, fixedIDs{}).Acreditar(context.Background(), entrada)
	require.ErrorIs(t, err, domain.ErrOperacionInvalida)
	require.Empty(t, p.hash)
}
