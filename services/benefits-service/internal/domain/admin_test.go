package domain_test

import (
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/stretchr/testify/require"
)

var ahoraAdmin = time.Date(2026, 11, 10, 15, 0, 0, 0, time.UTC)

func original(puntos int64, tipo string) domain.DatosMovimiento {
	return domain.DatosMovimiento{ID: "original", CuentaID: "cuenta", Tipo: tipo, Puntos: puntos, Secuencia: 1}
}

// HU-11 / RN-22, RN-23, RN-25; CA-11.1, CA-11.2, CA-11.3, CA-35.3, CL-13.
func TestRevertirMovimiento(t *testing.T) {
	cuenta, err := domain.NuevaCuenta("cuenta", 10, 1)
	require.NoError(t, err)

	t.Run("CA-11.1_compensatorio_resta_el_importe", func(t *testing.T) {
		nueva, mov, err := cuenta.Revertir(original(10, ""), false, "rev", "Carga por error", ahoraAdmin)
		require.NoError(t, err)
		m := mov.Datos()
		require.Equal(t, int64(-10), m.Puntos)
		require.Equal(t, domain.Reversion, m.Tipo)
		require.Equal(t, "original", m.RevierteID)
		require.Equal(t, int64(2), m.Secuencia)
		require.Zero(t, nueva.Saldo())
		require.Equal(t, int64(10), cuenta.Saldo(), "el agregado original no cambia")
	})
	t.Run("CA-11.2_ya_revertido", func(t *testing.T) {
		_, _, err := cuenta.Revertir(original(10, ""), true, "rev", "Carga por error", ahoraAdmin)
		require.ErrorIs(t, err, domain.ErrMovimientoRevertido)
	})
	t.Run("una_reversion_no_se_revierte", func(t *testing.T) {
		o := original(-10, domain.Reversion)
		o.RevierteID = "otro"
		_, _, err := cuenta.Revertir(o, false, "rev", "Carga por error", ahoraAdmin)
		require.ErrorIs(t, err, domain.ErrMovimientoRevertido)
	})
	t.Run("CA-11.3_saldo_quedaria_negativo", func(t *testing.T) {
		pobre, err := domain.NuevaCuenta("cuenta", 4, 2)
		require.NoError(t, err)
		nueva, _, err := pobre.Revertir(original(10, ""), false, "rev", "Carga por error", ahoraAdmin)
		require.ErrorIs(t, err, domain.ErrSaldoInsuficiente)
		require.Equal(t, int64(4), nueva.Saldo())
	})
	t.Run("CA-35.3_asistencia_y_penalizacion", func(t *testing.T) {
		con, err := domain.NuevaCuenta("cuenta", 500, 1)
		require.NoError(t, err)
		_, mov, err := con.Revertir(original(500, domain.Asistencia), false, "rev", "Ingreso duplicado", ahoraAdmin)
		require.NoError(t, err)
		require.Equal(t, int64(-500), mov.Datos().Puntos)
		nueva, mov, err := con.Revertir(original(-100, domain.PenalizacionInasistencia), false, "rev", "Ausencia justificada", ahoraAdmin)
		require.NoError(t, err)
		require.Equal(t, int64(100), mov.Datos().Puntos)
		require.Equal(t, int64(600), nueva.Saldo())
	})
	t.Run("otra_cuenta_o_motivo_invalido", func(t *testing.T) {
		o := original(10, "")
		o.CuentaID = "ajena"
		_, _, err := cuenta.Revertir(o, false, "rev", "Carga por error", ahoraAdmin)
		require.ErrorIs(t, err, domain.ErrMovimientoNoEncontrado)
		_, _, err = cuenta.Revertir(original(10, ""), false, "rev", "no", ahoraAdmin)
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
	})
	t.Run("movimiento_de_reversion_requiere_original", func(t *testing.T) {
		_, err := domain.NuevoMovimiento(domain.DatosMovimiento{ID: "x", CuentaID: "c", Tipo: domain.Reversion, Puntos: -5, Secuencia: 1, Motivo: "Motivo", Referencia: "r", OcurridoEn: ahoraAdmin, AplicadoEn: ahoraAdmin})
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
		_, err = domain.NuevoMovimiento(domain.DatosMovimiento{ID: "x", CuentaID: "c", PartnerID: "p", RevierteID: "o", Puntos: 5, Secuencia: 1, Motivo: "Motivo", Referencia: "r", OcurridoEn: ahoraAdmin, AplicadoEn: ahoraAdmin})
		require.ErrorIs(t, err, domain.ErrOperacionInvalida, "un crédito no puede declarar revierte_id")
	})
}

// HU-10 / ADR-016: estado, permisos y límites configurables por integración.
func TestConfiguracionIntegracion(t *testing.T) {
	dia := int64(5000)
	base := domain.ConfiguracionIntegracion{Nombre: "Estudiario", Activo: true, MaxPuntosOperacion: 1000, MaxPuntosDia: &dia, MaxSolicitudesMinuto: 120}
	require.NoError(t, base.Validar())
	cero := int64(0)
	for name, mutate := range map[string]func(*domain.ConfiguracionIntegracion){
		"nombre_corto":             func(c *domain.ConfiguracionIntegracion) { c.Nombre = "ab" },
		"maximo_operacion_cero":    func(c *domain.ConfiguracionIntegracion) { c.MaxPuntosOperacion = 0 },
		"maximo_operacion_tecnico": func(c *domain.ConfiguracionIntegracion) { c.MaxPuntosOperacion = 1000001 },
		"limite_diario_cero":       func(c *domain.ConfiguracionIntegracion) { c.MaxPuntosDia = &cero },
		"solicitudes_cero":         func(c *domain.ConfiguracionIntegracion) { c.MaxSolicitudesMinuto = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			c := base
			mutate(&c)
			require.ErrorIs(t, c.Validar(), domain.ErrOperacionInvalida)
		})
	}
	sinLimite := base
	sinLimite.MaxPuntosDia = nil
	require.NoError(t, sinLimite.Validar())
}
