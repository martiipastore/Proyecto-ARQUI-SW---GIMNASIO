package application

import (
	"context"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"regexp"
)

type RegistroAPI interface {
	ConsultarCuenta(context.Context, string) (Balance, error)
	Registrar(context.Context, domain.RegistroExterno) (domain.ReciboRegistro, error)
	ConsultarRegistro(context.Context, string, string) (domain.ReciboRegistro, error)
	CompletarRegistro(context.Context, domain.CompletarRegistro) (int64, error)
}

func (r *Registro) ConsultarCuenta(ctx context.Context, user string) (Balance, error) {
	if !uuidRegistro.MatchString(user) {
		return Balance{}, domain.ErrOperacionInvalida
	}
	saldo, err := r.cuentas.SaldoUsuario(ctx, user)
	if err != nil {
		return Balance{}, err
	}
	return Balance{Saldo: saldo.Saldo, ActualizadoEn: saldo.ActualizadoEn}, nil
}

type Registro struct {
	members  ports.RegistroMembers
	cuentas  ports.CuentasExternas
	benefits *Benefits
}

func NewRegistro(m ports.RegistroMembers, c ports.CuentasExternas, b *Benefits) *Registro {
	return &Registro{m, c, b}
}

var uuidRegistro = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var eventoRegistro = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)

func (r *Registro) Registrar(ctx context.Context, in domain.RegistroExterno) (domain.ReciboRegistro, error) {
	if !claveID.MatchString(in.Clave) || !eventoRegistro.MatchString(in.EventoID) || in.Puntos < 0 || in.Puntos > 1000000 {
		return domain.ReciboRegistro{}, domain.ErrOperacionInvalida
	}
	name, err := r.cuentas.PoliticaRegistro(ctx, in.PartnerID, in.Puntos)
	if err != nil {
		return domain.ReciboRegistro{}, err
	}
	in.Plataforma = name
	return r.members.Registrar(ctx, in)
}
func (r *Registro) ConsultarRegistro(ctx context.Context, partner, event string) (domain.ReciboRegistro, error) {
	if !eventoRegistro.MatchString(event) {
		return domain.ReciboRegistro{}, domain.ErrOperacionInvalida
	}
	if _, err := r.cuentas.PoliticaRegistro(ctx, partner, 0); err != nil {
		return domain.ReciboRegistro{}, err
	}
	return r.members.ConsultarRegistro(ctx, partner, event)
}
func (r *Registro) CompletarRegistro(ctx context.Context, in domain.CompletarRegistro) (int64, error) {
	if !domain.TextoValido(in.EventoID, 1, 128) || !uuidRegistro.MatchString(in.RegistroID) || !uuidRegistro.MatchString(in.UsuarioID) || !uuidRegistro.MatchString(in.PartnerID) || !externalID.MatchString(in.ExternalUserID) || in.Puntos < 0 || in.Puntos > 1000000 {
		return 0, domain.ErrOperacionInvalida
	}
	if _, err := r.cuentas.PoliticaRegistro(ctx, in.PartnerID, in.Puntos); err != nil {
		return 0, err
	}
	balance, err := r.cuentas.VincularExterno(ctx, in.PartnerID, in.ExternalUserID, in.UsuarioID)
	if err != nil {
		return 0, err
	}
	if in.Puntos == 0 {
		return balance, nil
	}
	out, err := r.benefits.Acreditar(ctx, AcreditarPuntos{PartnerID: in.PartnerID, ExternalUserID: in.ExternalUserID, Clave: "registration-" + in.RegistroID, EventoID: in.EventoID, Puntos: in.Puntos, Motivo: in.Motivo, Referencia: in.Referencia})
	return out.Resultado.Saldo, err
}
