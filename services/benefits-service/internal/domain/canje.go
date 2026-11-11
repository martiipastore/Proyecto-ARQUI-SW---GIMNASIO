package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

// Cuponera interna del Club (D-94): catálogo de beneficios y canjes de la
// aplicación del gimnasio. HU-09, HU-20 y RN-22/23/24/25 del SPEC, sin stock
// (CA-09.5), más un código de cupón que recepción marca como USADO. No se
// expone por la API de partners: el contrato v2 sigue sin canjes (D-76).

const (
	Canje = "CANJE"

	CanjeConfirmado = "CONFIRMADO"
	CanjeUsado      = "USADO"
	CanjeAnulado    = "ANULADO"

	RolAlumno = "ALUMNO"

	// CostoMaximoBeneficio usa el mismo límite técnico que una operación de partner (D-26).
	CostoMaximoBeneficio = int64(1000000)
)

var (
	ErrBeneficioNoEncontrado  = errors.New("beneficio inexistente")
	ErrBeneficioNoDisponible  = errors.New("beneficio inactivo o fuera de vigencia")
	ErrCanjeNoEncontrado      = errors.New("canje inexistente")
	ErrTransicionCanje        = errors.New("el canje no admite esa transición")
	ErrCanjeSoloPorAnulacion  = errors.New("un canje se revierte anulándolo")
	ErrCodigoCanjeEnConflicto = errors.New("código de canje repetido")
)

// alfabetoCodigo evita caracteres que se confunden al dictarlos (0/O, 1/I/L).
const alfabetoCodigo = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// CodigoCanjeValido acepta el formato XXXX-XXXX que muestra el cupón.
func CodigoCanjeValido(s string) bool {
	if len(s) != 9 || s[4] != '-' {
		return false
	}
	for i, c := range s {
		if i != 4 && !strings.ContainsRune(alfabetoCodigo, c) {
			return false
		}
	}
	return true
}

// NormalizarCodigo admite el código escrito en minúsculas o sin guion.
func NormalizarCodigo(s string) string {
	s = strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	if len(s) == 8 && !strings.Contains(s, "-") {
		s = s[:4] + "-" + s[4:]
	}
	return s
}

// CodigoDesdeBytes arma un código a partir de bytes aleatorios del adaptador.
func CodigoDesdeBytes(b [8]byte) string {
	var out strings.Builder
	for i, v := range b {
		if i == 4 {
			out.WriteByte('-')
		}
		out.WriteByte(alfabetoCodigo[int(v)%len(alfabetoCodigo)])
	}
	return out.String()
}

// Beneficio es un ítem del catálogo canjeable por puntos (glosario del SPEC).
// Las fechas de vigencia son días calendario "AAAA-MM-DD" de Argentina; el
// final es inclusivo y opcional.
type Beneficio struct {
	ID, Nombre, Descripcion    string
	Costo                      int64
	VigenteDesde, VigenteHasta string
	Activo                     bool
	CreadoEn, ActualizadoEn    time.Time
	Canjes                     int64
}

func fechaValida(s string) bool {
	if len(s) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// Validar aplica los límites del alta y la modificación (HU-09).
func (b Beneficio) Validar() error {
	if !TextoValido(b.Nombre, 3, 100) || strings.TrimSpace(b.Nombre) != b.Nombre || !TextoValido(b.Descripcion, 0, 500) {
		return ErrOperacionInvalida
	}
	if b.Costo < 1 || b.Costo > CostoMaximoBeneficio || !fechaValida(b.VigenteDesde) {
		return ErrOperacionInvalida
	}
	if b.VigenteHasta != "" && (!fechaValida(b.VigenteHasta) || b.VigenteHasta < b.VigenteDesde) {
		return ErrOperacionInvalida
	}
	return nil
}

// Disponible indica si se puede canjear hoy: ACTIVO y dentro de su vigencia
// (RN-24). No hay stock: un beneficio disponible siempre se puede canjear.
func (b Beneficio) Disponible(hoy string) bool {
	return b.Activo && b.VigenteDesde <= hoy && (b.VigenteHasta == "" || hoy <= b.VigenteHasta)
}

// zonaNegocio es America/Argentina/Buenos_Aires; sin la base de zonas se usa
// su desplazamiento fijo (Argentina no aplica horario de verano desde 2009).
var zonaNegocio = func() *time.Location {
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		return time.FixedZone("ART", -3*60*60)
	}
	return loc
}()

// DiaNegocio convierte un instante al día calendario de Argentina.
func DiaNegocio(t time.Time) string { return t.In(zonaNegocio).Format("2006-01-02") }

// CanjeBeneficio es el cupón entregado al canjear. Conserva nombre y puntos del
// momento del canje: un cambio posterior del catálogo no lo altera (CA-09.4).
type CanjeBeneficio struct {
	ID, CuentaID, UsuarioID, BeneficioID, BeneficioNombre string
	Codigo, Estado, MovimientoID, AnulacionID             string
	MotivoAnulacion                                       string
	Puntos                                                int64
	CreadoEn                                              time.Time
	UsadoEn, AnuladoEn                                    *time.Time
}

// ResultadoCanje acompaña el cupón con el saldo resultante de la cuenta.
type ResultadoCanje struct {
	Canje CanjeBeneficio
	Saldo int64
}

// Canjear debita el costo vigente en un movimiento CANJE y crea el cupón
// CONFIRMADO, ambos o ninguno (RN-24). El saldo nunca queda negativo (RN-23).
func (c CuentaBeneficios) Canjear(b Beneficio, usuario, movimientoID, canjeID, codigo string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, CanjeBeneficio, error) {
	if !b.Disponible(DiaNegocio(ahora)) {
		return c, MovimientoPuntos{}, CanjeBeneficio{}, ErrBeneficioNoDisponible
	}
	if b.Costo < 1 || b.Costo > CostoMaximoBeneficio || !CodigoCanjeValido(codigo) || !UUIDValido(canjeID) || c.secuencia == math.MaxInt64 {
		return c, MovimientoPuntos{}, CanjeBeneficio{}, ErrOperacionInvalida
	}
	if c.saldo < b.Costo {
		return c, MovimientoPuntos{}, CanjeBeneficio{}, ErrSaldoInsuficiente
	}
	motivo := "Canje: " + b.Nombre
	m, err := NuevoMovimiento(DatosMovimiento{ID: movimientoID, CuentaID: c.id, Tipo: Canje, OrigenID: canjeID, Puntos: -b.Costo, Secuencia: c.secuencia + 1, Motivo: motivo, Referencia: codigo, OcurridoEn: ahora, AplicadoEn: ahora})
	if err != nil {
		return c, MovimientoPuntos{}, CanjeBeneficio{}, err
	}
	c.saldo -= b.Costo
	c.secuencia++
	canje := CanjeBeneficio{ID: canjeID, CuentaID: c.id, UsuarioID: usuario, BeneficioID: b.ID, BeneficioNombre: b.Nombre, Codigo: codigo, Estado: CanjeConfirmado, MovimientoID: movimientoID, Puntos: b.Costo, CreadoEn: ahora}
	return c, m, canje, nil
}

// Usar registra la entrega del beneficio en recepción. Solo un canje
// CONFIRMADO pasa a USADO; repetirlo sobre uno USADO no tiene efectos.
func (k CanjeBeneficio) Usar(ahora time.Time) (CanjeBeneficio, bool, error) {
	switch k.Estado {
	case CanjeUsado:
		return k, false, nil
	case CanjeConfirmado:
		k.Estado = CanjeUsado
		k.UsadoEn = &ahora
		return k, true, nil
	}
	return k, false, ErrTransicionCanje
}

// AnularCanje devuelve los puntos con un compensatorio del débito (RN-25:
// anular un canje revierte su débito). Un cupón ya USADO no se anula porque el
// beneficio fue entregado; uno ANULADO no se vuelve a anular.
func (c CuentaBeneficios) AnularCanje(k CanjeBeneficio, debito DatosMovimiento, id, motivo string, ahora time.Time) (CuentaBeneficios, MovimientoPuntos, CanjeBeneficio, error) {
	if k.Estado != CanjeConfirmado {
		return c, MovimientoPuntos{}, k, ErrTransicionCanje
	}
	if debito.ID != k.MovimientoID || debito.Tipo != Canje {
		return c, MovimientoPuntos{}, k, ErrOperacionInvalida
	}
	nueva, m, err := c.compensar(debito, false, id, motivo, ahora)
	if err != nil {
		return c, MovimientoPuntos{}, k, err
	}
	k.Estado = CanjeAnulado
	k.AnulacionID = m.Datos().ID
	k.MotivoAnulacion = motivo
	k.AnuladoEn = &ahora
	return nueva, m, k, nil
}
