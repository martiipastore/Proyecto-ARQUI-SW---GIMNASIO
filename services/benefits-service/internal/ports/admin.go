package ports

import (
	"context"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// SecretGenerator emite credenciales aleatorias; solo se devuelven una vez.
type SecretGenerator interface{ NewSecret() (string, error) }

// Auditoria identifica al administrador y el momento de cada cambio.
type Auditoria struct {
	AdministradorID, EventoID string
	Ahora                     time.Time
}

// ReversionPedida agrupa la identidad idempotente de una reversión.
type ReversionPedida struct {
	Auditoria
	Clave, Huella, MovimientoID string
}

// AplicarReversion recibe la cuenta bloqueada, el original y si ya tiene
// compensatorio; devuelve el estado nuevo según las reglas del dominio.
type AplicarReversion func(domain.CuentaBeneficios, domain.DatosMovimiento, bool) (domain.CuentaBeneficios, domain.MovimientoPuntos, error)

// AdministracionClub es el puerto de salida de la administración del Club.
// El adaptador garantiza atomicidad, bloqueo por cuenta y auditoría inmutable.
type AdministracionClub interface {
	ListarIntegraciones(context.Context) ([]domain.Integracion, error)
	CrearIntegracion(ctx context.Context, a Auditoria, id string, cfg domain.ConfiguracionIntegracion, hash [32]byte) (domain.Integracion, error)
	ActualizarIntegracion(ctx context.Context, a Auditoria, id string, cfg domain.ConfiguracionIntegracion) (domain.Integracion, error)
	RotarCredencial(ctx context.Context, a Auditoria, id string, hash [32]byte) error
	Vincular(ctx context.Context, a Auditoria, partnerID, usuarioID, externalUserID string) (creada bool, err error)
	CuentaDeUsuario(ctx context.Context, usuarioID string, pagina, tamano int) (domain.CuentaAdministrada, error)
	Revertir(ctx context.Context, p ReversionPedida, aplicar AplicarReversion) (resultado domain.ResultadoReversion, repetida bool, err error)
}
