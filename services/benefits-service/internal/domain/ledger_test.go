package domain

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// HU-29, RN-22/23/42: cada acreditación extiende el ledger sin editar el pasado.
func TestCuenta_AcreditacionesExplicanSaldo(t *testing.T) {
	c, err := NuevaCuenta("cuenta", 0, 0)
	require.NoError(t, err)
	primera, m1, err := c.Acreditar("m1", "partner", 100, "Compra", "ref-1", time.Now())
	require.NoError(t, err)
	segunda, m2, err := primera.Acreditar("m2", "partner", 250, "Compra", "ref-2", time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 0, c.Saldo())
	require.EqualValues(t, 100, primera.Saldo())
	require.Equal(t, m1.Datos().Puntos+m2.Datos().Puntos, segunda.Saldo())
	require.EqualValues(t, 2, segunda.Secuencia())
	copia := m1.Datos()
	copia.Puntos = 999
	require.EqualValues(t, 100, m1.Datos().Puntos)
}

// RN-22/23 y D-26: no aceptar importes inválidos ni overflow silencioso.
func TestCuenta_OperacionInvalidaNoModificaSaldo(t *testing.T) {
	for _, puntos := range []int64{-1, 0, 1000001} {
		c, _ := NuevaCuenta("cuenta", 0, 0)
		actual, _, err := c.Acreditar("m", "p", puntos, "Compra", "ref", time.Now())
		require.ErrorIs(t, err, ErrOperacionInvalida)
		require.Equal(t, c, actual)
	}
	c, _ := NuevaCuenta("cuenta", math.MaxInt64, 1)
	_, _, err := c.Acreditar("m", "p", 1, "Compra", "ref", time.Now())
	require.ErrorIs(t, err, ErrOperacionInvalida)
	_, err = NuevaCuenta("cuenta", -1, 0)
	require.ErrorIs(t, err, ErrOperacionInvalida)
	_, _, err = c.Acreditar("m", "p", 1, "ab", "ref", time.Now())
	require.ErrorIs(t, err, ErrOperacionInvalida)
}
