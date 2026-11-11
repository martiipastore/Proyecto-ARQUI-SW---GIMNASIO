package ports

import (
	"context"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// CodigoGenerator emite el código aleatorio que se muestra en el cupón (D-94).
type CodigoGenerator interface{ NewCodigo() (string, error) }

// PedidoCanje es la identidad idempotente de un canje del alumno (RN-15).
type PedidoCanje struct {
	UsuarioID, BeneficioID, Clave, Huella string
	Ahora                                 time.Time
}

// AplicarCanje recibe la cuenta bloqueada (o una vacía si todavía no existe) y
// el beneficio con su costo vigente leído en la misma transacción (CL-18).
type AplicarCanje func(domain.CuentaBeneficios, domain.Beneficio) (domain.CuentaBeneficios, domain.MovimientoPuntos, domain.CanjeBeneficio, error)

// AplicarAnulacion recibe la cuenta bloqueada, el cupón y su débito original.
type AplicarAnulacion func(domain.CuentaBeneficios, domain.CanjeBeneficio, domain.DatosMovimiento) (domain.CuentaBeneficios, domain.MovimientoPuntos, domain.CanjeBeneficio, error)

// AplicarUso decide la transición a USADO; cambio=false si ya estaba usado.
type AplicarUso func(domain.CanjeBeneficio) (domain.CanjeBeneficio, bool, error)

type FiltroCanjes struct {
	UsuarioID, Codigo, Estado string
	Pagina, Tamano            int
}

type PaginaCanjes struct {
	Items []domain.CanjeBeneficio
	Total int64
}

// Cuponera es el puerto de salida de catálogo y canjes. El adaptador garantiza
// atomicidad entre cupón, débito y saldo, bloqueo por cuenta y auditoría.
type Cuponera interface {
	ListarBeneficios(ctx context.Context, soloDisponibles bool, hoy string) ([]domain.Beneficio, error)
	CrearBeneficio(ctx context.Context, a Auditoria, b domain.Beneficio) (domain.Beneficio, error)
	ActualizarBeneficio(ctx context.Context, a Auditoria, b domain.Beneficio) (domain.Beneficio, error)
	Canjear(ctx context.Context, p PedidoCanje, aplicar AplicarCanje) (resultado domain.ResultadoCanje, repetido bool, err error)
	ListarCanjes(ctx context.Context, f FiltroCanjes) (PaginaCanjes, error)
	UsarCanje(ctx context.Context, a Auditoria, id string, usar AplicarUso) (canje domain.CanjeBeneficio, cambio bool, err error)
	AnularCanje(ctx context.Context, a Auditoria, id string, anular AplicarAnulacion) (resultado domain.ResultadoCanje, cambio bool, err error)
}
