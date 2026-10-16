package ports

import (
	"context"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID() (string, error) }

type OperacionGuardada struct {
	Huella    string
	Resultado domain.ResultadoOperacion
	Nueva     bool
}

// Transaction agrupa las necesidades de escritura para confirmar un único efecto.
// El adaptador decide cómo implementar la atomicidad, nunca las reglas del saldo.
type Transaction interface {
	PartnerActivo(context.Context, string) (bool, error)
	TomarOperacion(context.Context, string, string, string, time.Time) (OperacionGuardada, error)
	CuentaVinculada(context.Context, string, string) (domain.CuentaBeneficios, error)
	GuardarMovimiento(context.Context, domain.CuentaBeneficios, domain.MovimientoPuntos) error
	CompletarOperacion(context.Context, string, string, domain.ResultadoOperacion) error
}

type Ledger interface {
	EnTransaccion(context.Context, func(Transaction) error) error
	ConsultarSaldo(context.Context, string, string) (SaldoConsultado, error)
	ListarMovimientos(context.Context, string, string, int, int) (MovimientosConsultados, error)
}

type SaldoConsultado struct {
	PartnerActivo, CuentaVinculada bool
	Saldo                          int64
}

type MovimientosConsultados struct {
	PartnerActivo, CuentaVinculada bool
	Pagina                         domain.PaginaMovimientos
}
