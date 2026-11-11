package application

import (
	"context"
	"errors"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

// EnviosMelt entrega a MELT las acreditaciones registradas junto con cada
// asistencia (etapa 13, D-92, ADR-009 v2). Corre en segundo plano: una falla
// del proveedor nunca afecta los puntos locales ni la asistencia.
type EnviosMelt struct {
	repo      ports.EnviosMelt
	proveedor ports.ProveedorMelt
	contactos ports.Contactos
	clock     ports.Clock
}

func NewEnviosMelt(r ports.EnviosMelt, p ports.ProveedorMelt, c ports.Contactos, clock ports.Clock) *EnviosMelt {
	return &EnviosMelt{repo: r, proveedor: p, contactos: c, clock: clock}
}

// Procesar intenta los envíos vencidos y devuelve cuántos procesó. Cada envío
// conserva su referencia: un reintento nunca acredita dos veces en MELT.
func (s *EnviosMelt) Procesar(ctx context.Context, limite int) (int, error) {
	ahora := s.clock.Now()
	envios, err := s.repo.EnviosMeltVencidos(ctx, ahora, limite)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range envios {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		r := s.intentar(ctx, e)
		if err := s.repo.GuardarEnvioMelt(ctx, e.Registrar(r, s.clock.Now())); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *EnviosMelt) intentar(ctx context.Context, e domain.EnvioMelt) domain.ResultadoMelt {
	email, err := s.contactos.EmailDe(ctx, e.AlumnoID)
	if errors.Is(err, domain.ErrCuentaNoVinculada) {
		return domain.ResultadoMelt{Tipo: domain.ResultadoRechazado, Motivo: "La persona ya no es un alumno activo."}
	}
	if err != nil {
		return domain.ResultadoMelt{Tipo: domain.ResultadoReintentar, Motivo: "No se pudo consultar el email en Members."}
	}
	r, err := s.proveedor.Acreditar(ctx, email, e.Puntos, e.Referencia, "Asistencia a clase en Metal Fitness")
	if err != nil {
		return domain.ResultadoMelt{Tipo: domain.ResultadoReintentar, Motivo: "MELT no respondió; se reintenta con la misma referencia."}
	}
	return r
}

// MisEnvios lista los envíos del alumno autenticado.
func (s *EnviosMelt) MisEnvios(ctx context.Context, alumno string) ([]domain.EnvioMelt, error) {
	if !domain.UUIDValido(alumno) {
		return nil, domain.ErrOperacionInvalida
	}
	return s.repo.EnviosMeltDeAlumno(ctx, alumno)
}
