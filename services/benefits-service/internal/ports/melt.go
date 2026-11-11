package ports

import (
	"context"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// EnviosMelt persiste los envíos al proveedor externo (etapa 13).
type EnviosMelt interface {
	// EnviosMeltVencidos devuelve envíos que deben intentarse ahora.
	EnviosMeltVencidos(ctx context.Context, ahora time.Time, limite int) ([]domain.EnvioMelt, error)
	GuardarEnvioMelt(ctx context.Context, e domain.EnvioMelt) error
	EnviosMeltDeAlumno(ctx context.Context, alumno string) ([]domain.EnvioMelt, error)
}

// ProveedorMelt acredita puntos en MELT. Devuelve error solo cuando el
// resultado es incierto o transitorio; el adaptador traduce el resto a
// domain.ResultadoMelt (capa anticorrupción, ADR-009 v2).
type ProveedorMelt interface {
	Acreditar(ctx context.Context, email string, puntos int64, referencia, descripcion string) (domain.ResultadoMelt, error)
}

// Contactos obtiene el email de un alumno desde Members, dueño del dato.
type Contactos interface {
	EmailDe(ctx context.Context, alumno string) (string, error)
}
