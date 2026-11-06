package ports

import (
	"context"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

type RegistroMembers interface {
	Registrar(context.Context, domain.RegistroExterno) (domain.ReciboRegistro, error)
	ConsultarRegistro(context.Context, string, string) (domain.ReciboRegistro, error)
}
type CuentasExternas interface {
	SaldoUsuario(context.Context, string) (SaldoConsultado, error)
	PoliticaRegistro(context.Context, string, int64) (string, error)
	VincularExterno(context.Context, string, string, string) (int64, error)
}
