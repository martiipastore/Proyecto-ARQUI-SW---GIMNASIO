package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

type AcreditarPuntos struct {
	PartnerID, ExternalUserID, Clave string
	Puntos                           int64
	Motivo, Referencia               string
}

type RespuestaAcreditacion struct {
	Resultado domain.ResultadoOperacion
	Repetida  bool
}

type CasosDeUso interface {
	Acreditar(context.Context, AcreditarPuntos) (RespuestaAcreditacion, error)
	ConsultarSaldo(context.Context, string, string) (int64, error)
	ListarMovimientos(context.Context, string, string, int, int) (domain.PaginaMovimientos, error)
}

type Benefits struct {
	ledger ports.Ledger
	clock  ports.Clock
	ids    ports.IDGenerator
}

func New(ledger ports.Ledger, clock ports.Clock, ids ports.IDGenerator) *Benefits {
	return &Benefits{ledger: ledger, clock: clock, ids: ids}
}

var externalID = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,64}$`)
var claveID = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func (b *Benefits) Acreditar(ctx context.Context, entrada AcreditarPuntos) (RespuestaAcreditacion, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	var respuesta RespuestaAcreditacion
	if entrada.PartnerID == "" || !claveID.MatchString(entrada.Clave) {
		return respuesta, domain.ErrOperacionInvalida
	}
	// Incluye operación y cuenta: una key por partner no se reutiliza en otra ruta.
	contenido, err := json.Marshal([]any{"CREDIT", entrada.ExternalUserID, entrada.Puntos, entrada.Motivo, entrada.Referencia})
	if err != nil {
		return respuesta, err
	}
	huella := sha256.Sum256(contenido)
	hash := hex.EncodeToString(huella[:])
	err = b.ledger.EnTransaccion(ctx, func(tx ports.Transaction) error {
		activo, err := tx.PartnerActivo(ctx, entrada.PartnerID)
		if err != nil {
			return err
		}
		if !activo {
			return domain.ErrPartnerNoAutorizado
		}
		guardada, err := tx.TomarOperacion(ctx, entrada.PartnerID, entrada.Clave, hash, b.clock.Now().UTC())
		if err != nil {
			return err
		}
		if !guardada.Nueva {
			if guardada.Huella != hash {
				return domain.ErrConflictoIdempotencia
			}
			// RN-26: el replay no concede acceso si se revocó la vinculación.
			// No se reemplaza el resultado original persistido.
			if guardada.Resultado.Rechazo == "" {
				if _, err := tx.CuentaVinculada(ctx, entrada.PartnerID, entrada.ExternalUserID); err != nil {
					return err
				}
			}
			respuesta = RespuestaAcreditacion{Resultado: guardada.Resultado, Repetida: true}
			return nil
		}
		resultado := domain.ResultadoOperacion{}
		if !externalID.MatchString(entrada.ExternalUserID) || entrada.Puntos < 1 || entrada.Puntos > 1000000 || !domain.TextoValido(entrada.Motivo, 3, 200) || !domain.TextoValido(entrada.Referencia, 1, 100) {
			resultado.Rechazo = domain.OperacionInvalida
		} else {
			cuenta, err := tx.CuentaVinculada(ctx, entrada.PartnerID, entrada.ExternalUserID)
			if errors.Is(err, domain.ErrCuentaNoVinculada) {
				resultado.Rechazo = domain.CuentaNoEncontrada
			} else if err != nil {
				return err
			} else {
				id, err := b.ids.NewID()
				if err != nil {
					return err
				}
				nueva, movimiento, err := cuenta.Acreditar(id, entrada.PartnerID, entrada.Puntos, entrada.Motivo, entrada.Referencia, b.clock.Now().UTC())
				if err == domain.ErrOperacionInvalida {
					resultado.Rechazo = domain.OperacionInvalida
				} else if err != nil {
					return err
				} else {
					if err := tx.GuardarMovimiento(ctx, nueva, movimiento); err != nil {
						return err
					}
					resultado = domain.ResultadoOperacion{Movimiento: movimiento, Saldo: nueva.Saldo()}
				}
			}
		}
		if err := tx.CompletarOperacion(ctx, entrada.PartnerID, entrada.Clave, resultado); err != nil {
			return err
		}
		respuesta.Resultado = resultado
		return nil
	})
	if err != nil {
		return RespuestaAcreditacion{}, err
	}
	return respuesta, respuesta.Resultado.Error()
}

func (b *Benefits) ConsultarSaldo(ctx context.Context, partner, usuario string) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if partner == "" || !externalID.MatchString(usuario) {
		return 0, domain.ErrOperacionInvalida
	}
	lectura, err := b.ledger.ConsultarSaldo(ctx, partner, usuario)
	if err != nil {
		return 0, err
	}
	if !lectura.PartnerActivo {
		return 0, domain.ErrPartnerNoAutorizado
	}
	if !lectura.CuentaVinculada {
		return 0, domain.ErrCuentaNoVinculada
	}
	return lectura.Saldo, nil
}

func (b *Benefits) ListarMovimientos(ctx context.Context, partner, usuario string, pagina, tamanio int) (domain.PaginaMovimientos, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if partner == "" || !externalID.MatchString(usuario) || pagina < 1 || pagina > 100000 || tamanio < 1 || tamanio > 100 {
		return domain.PaginaMovimientos{}, domain.ErrOperacionInvalida
	}
	lectura, err := b.ledger.ListarMovimientos(ctx, partner, usuario, pagina, tamanio)
	if err != nil {
		return domain.PaginaMovimientos{}, err
	}
	if !lectura.PartnerActivo {
		return domain.PaginaMovimientos{}, domain.ErrPartnerNoAutorizado
	}
	if !lectura.CuentaVinculada {
		return domain.PaginaMovimientos{}, domain.ErrCuentaNoVinculada
	}
	return lectura.Pagina, nil
}
