package domain

import (
	"errors"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrCuentaNoVinculada     = errors.New("cuenta no vinculada al partner")
	ErrPartnerNoAutorizado   = errors.New("partner no autorizado")
	ErrConflictoIdempotencia = errors.New("clave de idempotencia usada con otros datos")
	ErrOperacionInvalida     = errors.New("operación inválida")
	ErrOperacionEnCurso      = errors.New("operación en curso")
)

// CuentaBeneficios protege el saldo y la secuencia del agregado (RN-22/42).
// Los valores se reemplazan al aplicar una operación; no se expone estado mutable.
type CuentaBeneficios struct {
	id        string
	saldo     int64
	secuencia int64
}

func NuevaCuenta(id string, saldo, secuencia int64) (CuentaBeneficios, error) {
	if id == "" || saldo < 0 || secuencia < 0 {
		return CuentaBeneficios{}, ErrOperacionInvalida
	}
	return CuentaBeneficios{id: id, saldo: saldo, secuencia: secuencia}, nil
}

func (c CuentaBeneficios) ID() string       { return c.id }
func (c CuentaBeneficios) Saldo() int64     { return c.saldo }
func (c CuentaBeneficios) Secuencia() int64 { return c.secuencia }

// DatosMovimiento es una copia para cruzar puertos, sin tags ni infraestructura.
type DatosMovimiento struct {
	ID, CuentaID, PartnerID string
	Puntos, Secuencia       int64
	Motivo, Referencia      string
	OcurridoEn, AplicadoEn  time.Time
}

type MovimientoPuntos struct{ datos DatosMovimiento }

func NuevoMovimiento(datos DatosMovimiento) (MovimientoPuntos, error) {
	if datos.ID == "" || datos.CuentaID == "" || datos.PartnerID == "" || datos.Secuencia < 1 || datos.Puntos < 1 || datos.Puntos > 1000000 || datos.OcurridoEn.IsZero() || datos.AplicadoEn.IsZero() || !TextoValido(datos.Motivo, 3, 200) || !TextoValido(datos.Referencia, 1, 100) {
		return MovimientoPuntos{}, ErrOperacionInvalida
	}
	return MovimientoPuntos{datos: datos}, nil
}

func (m MovimientoPuntos) Datos() DatosMovimiento { return m.datos }

func TextoValido(texto string, minimo, maximo int) bool {
	n := utf8.RuneCountInString(texto)
	return utf8.ValidString(texto) && !strings.ContainsRune(texto, 0) && n >= minimo && n <= maximo
}

// Acreditar crea un nuevo estado y movimiento, sin modificar el original (RN-22).
// El máximo es el límite técnico del contrato D-26, no un tope de saldo.
func (c CuentaBeneficios) Acreditar(id, partner string, puntos int64, motivo, referencia string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, error) {
	if puntos < 1 || puntos > 1000000 || c.saldo > math.MaxInt64-puntos || c.secuencia == math.MaxInt64 {
		return c, MovimientoPuntos{}, ErrOperacionInvalida
	}
	m, err := NuevoMovimiento(DatosMovimiento{ID: id, CuentaID: c.id, PartnerID: partner, Puntos: puntos, Secuencia: c.secuencia + 1, Motivo: motivo, Referencia: referencia, OcurridoEn: ahora, AplicadoEn: ahora})
	if err != nil {
		return c, MovimientoPuntos{}, err
	}
	c.saldo += puntos
	c.secuencia++
	return c, m, nil
}

// ResultadoOperacion conserva el resultado de negocio de la primera ejecución.
// Rechazo es estable; los adaptadores traducen códigos y formatos externos.
type ResultadoOperacion struct {
	Movimiento MovimientoPuntos
	Saldo      int64
	Rechazo    string
}

const (
	CuentaNoEncontrada = "cuenta_no_vinculada"
	OperacionInvalida  = "operacion_invalida"
)

func (r ResultadoOperacion) Error() error {
	switch r.Rechazo {
	case CuentaNoEncontrada:
		return ErrCuentaNoVinculada
	case OperacionInvalida:
		return ErrOperacionInvalida
	default:
		return nil
	}
}

type PaginaMovimientos struct {
	Movimientos []MovimientoPuntos
	Total       int64
}

type Partner struct {
	ID     string
	Activo bool
}
