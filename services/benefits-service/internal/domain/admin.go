package domain

import (
	"errors"
	"math"
	"regexp"
	"time"
)

// Administración del Club (etapa 7 de la entrega final): HU-10, HU-11, HU-35 y
// HU-CB-07 con RN-22/23/25/26. Los débitos y la API de partners siguen sin
// canjes (D-76); la cuponera interna está en canje.go (D-94).

const (
	Reversion        = "REVERSION"
	RolAdministrador = "ADMINISTRADOR"
)

var (
	ErrSinPermiso              = errors.New("solo el administrador puede operar")
	ErrSaldoInsuficiente       = errors.New("la operación dejaría el saldo negativo")
	ErrMovimientoRevertido     = errors.New("el movimiento ya fue revertido o es una reversión")
	ErrMovimientoNoEncontrado  = errors.New("movimiento inexistente")
	ErrIntegracionNoEncontrada = errors.New("integración inexistente")
	ErrVinculoEnConflicto      = errors.New("identificador externo o cuenta ya vinculados")
)

var identificadorExterno = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,64}$`)

// ExternalIDValido aplica el mismo formato que la API de partners y la base.
func ExternalIDValido(s string) bool { return identificadorExterno.MatchString(s) }

// Revertir crea el compensatorio de un movimiento CONFIRMADO de esta cuenta.
// El original no cambia (RN-22); el saldo nunca queda negativo (RN-23); un
// movimiento se revierte una sola vez y una reversión no se revierte (RN-25).
// Revertir una asistencia no modifica la asistencia de Booking (CA-35.3).
// El débito de un canje solo se compensa anulando el canje (D-94), para que el
// cupón y su movimiento no queden en estados distintos.
func (c CuentaBeneficios) Revertir(original DatosMovimiento, yaRevertido bool, id, motivo string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, error) {
	if original.Tipo == Canje && original.CuentaID == c.id {
		return c, MovimientoPuntos{}, ErrCanjeSoloPorAnulacion
	}
	return c.compensar(original, yaRevertido, id, motivo, ahora)
}

func (c CuentaBeneficios) compensar(original DatosMovimiento, yaRevertido bool, id, motivo string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, error) {
	if original.ID == "" || original.CuentaID != c.id {
		return c, MovimientoPuntos{}, ErrMovimientoNoEncontrado
	}
	if yaRevertido || original.Tipo == Reversion {
		return c, MovimientoPuntos{}, ErrMovimientoRevertido
	}
	if !TextoValido(motivo, 3, 200) || c.secuencia == math.MaxInt64 {
		return c, MovimientoPuntos{}, ErrOperacionInvalida
	}
	puntos := -original.Puntos
	if c.saldo+puntos < 0 {
		return c, MovimientoPuntos{}, ErrSaldoInsuficiente
	}
	if puntos > 0 && c.saldo > math.MaxInt64-puntos {
		return c, MovimientoPuntos{}, ErrOperacionInvalida
	}
	m, err := NuevoMovimiento(DatosMovimiento{ID: id, CuentaID: c.id, Tipo: Reversion, RevierteID: original.ID, Puntos: puntos, Secuencia: c.secuencia + 1, Motivo: motivo, Referencia: original.ID, OcurridoEn: ahora, AplicadoEn: ahora})
	if err != nil {
		return c, MovimientoPuntos{}, err
	}
	c.saldo += puntos
	c.secuencia++
	return c, m, nil
}

// EstadoMovimiento deriva el estado de RN-22: REVERTIDO conserva su importe en
// el saldo y el compensatorio aplica el contrario (D-52).
func (p PaginaMovimientos) EstadoMovimiento(id string) string {
	if p.RevertidoPor[id] != "" {
		return "REVERTIDO"
	}
	return "CONFIRMADO"
}

type Vinculacion struct {
	PartnerID, PartnerNombre, ExternalUserID string
}

// CuentaAdministrada es la vista completa de una persona para el administrador.
type CuentaAdministrada struct {
	UsuarioID, CuentaID string
	Saldo               int64
	CreadaEn            time.Time
	Pagina              PaginaMovimientos
	Vinculaciones       []Vinculacion
}

// ConfiguracionIntegracion reúne estado, permisos y límites de una integración
// (ADR-016). La credencial no forma parte de la configuración.
type ConfiguracionIntegracion struct {
	Nombre                                                  string
	Activo, PermiteConsulta, PermitePuntos, PermiteRegistro bool
	RequiereEvento                                          bool
	MaxPuntosOperacion                                      int64
	MaxPuntosDia                                            *int64
	MaxSolicitudesMinuto                                    int
}

func (c ConfiguracionIntegracion) Validar() error {
	if !TextoValido(c.Nombre, 3, 100) || c.MaxPuntosOperacion < 1 || c.MaxPuntosOperacion > 1000000 || c.MaxSolicitudesMinuto < 1 || c.MaxSolicitudesMinuto > 60000 {
		return ErrOperacionInvalida
	}
	if c.MaxPuntosDia != nil && *c.MaxPuntosDia < 1 {
		return ErrOperacionInvalida
	}
	return nil
}

type Integracion struct {
	ID            string
	Config        ConfiguracionIntegracion
	Vinculaciones int64
	CreadaEn      time.Time
}

// ResultadoReversion es lo que se devuelve también ante un reintento idempotente.
type ResultadoReversion struct {
	Movimiento MovimientoPuntos
	Saldo      int64
}
