package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

type AcreditarPuntos struct {
	Correlacion                      string
	EntradaInvalida                  bool
	HuellaEntrada                    string
	PartnerID, ExternalUserID, Clave string
	Puntos                           int64
	Motivo, Referencia               string
}

type RespuestaAcreditacion struct {
	Representacion []byte
	Correlacion    string
	Resultado      domain.ResultadoOperacion
	Repetida       bool
}

type CasosDeUso interface {
	Acreditar(context.Context, AcreditarPuntos) (RespuestaAcreditacion, error)
	ConsultarSaldo(context.Context, string, string) (int64, error)
	ConsultarBalance(context.Context, string, string) (Balance, error)
	ListarMovimientos(context.Context, string, string, int, int) (domain.PaginaMovimientos, error)
}

type Balance struct {
	Saldo         int64
	ActualizadoEn time.Time
}

type Benefits struct {
	encoder ports.ResultadoEncoder
	ledger  ports.Ledger
	clock   ports.Clock
	ids     ports.IDGenerator
}

func New(ledger ports.Ledger, clock ports.Clock, ids ports.IDGenerator, encoders ...ports.ResultadoEncoder) *Benefits {
	b := &Benefits{ledger: ledger, clock: clock, ids: ids}
	if len(encoders) > 0 {
		b.encoder = encoders[0]
	}
	return b
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
	if entrada.EntradaInvalida {
		raw := sha256.Sum256([]byte(fmt.Sprintf("INVALID:%s:%s", entrada.ExternalUserID, entrada.HuellaEntrada)))
		hash = hex.EncodeToString(raw[:])
	}
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
			respuesta = RespuestaAcreditacion{Resultado: guardada.Resultado, Repetida: true, Correlacion: guardada.Correlacion, Representacion: guardada.Representacion}
			return nil
		}
		resultado := domain.ResultadoOperacion{}
		if entrada.EntradaInvalida || !externalID.MatchString(entrada.ExternalUserID) || entrada.Puntos < 1 || entrada.Puntos > 1000000 || !domain.TextoValido(entrada.Motivo, 3, 200) || !domain.TextoValido(entrada.Referencia, 1, 100) {
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
		var representacion []byte
		if b.encoder != nil {
			var err error
			representacion, err = b.encoder.Representar(resultado, entrada.ExternalUserID, entrada.Correlacion)
			if err != nil {
				return err
			}
		}
		if err := tx.CompletarOperacion(ctx, entrada.PartnerID, entrada.Clave, resultado, entrada.Correlacion, representacion); err != nil {
			return err
		}
		respuesta.Resultado = resultado
		respuesta.Representacion = representacion
		respuesta.Correlacion = entrada.Correlacion
		return nil
	})
	if err != nil {
		return RespuestaAcreditacion{}, err
	}
	return respuesta, respuesta.Resultado.Error()
}

func (b *Benefits) ConsultarSaldo(ctx context.Context, partner, usuario string) (int64, error) {
	r, err := b.ConsultarBalance(ctx, partner, usuario)
	return r.Saldo, err
}
func (b *Benefits) ConsultarBalance(ctx context.Context, partner, usuario string) (Balance, error) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if partner == "" || !externalID.MatchString(usuario) {
		return Balance{}, domain.ErrOperacionInvalida
	}
	r, err := b.ledger.ConsultarSaldo(ctx, partner, usuario)
	if err != nil {
		return Balance{}, err
	}
	if !r.PartnerActivo {
		return Balance{}, domain.ErrPartnerNoAutorizado
	}
	if !r.CuentaVinculada {
		return Balance{}, domain.ErrCuentaNoVinculada
	}
	return Balance{Saldo: r.Saldo, ActualizadoEn: r.ActualizadoEn}, nil
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
