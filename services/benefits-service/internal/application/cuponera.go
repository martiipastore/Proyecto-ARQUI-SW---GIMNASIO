package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

// CuponeraAPI es el puerto de entrada de la cuponera interna (D-94): catálogo
// (HU-09), canje (HU-20) y gestión de cupones por el administrador (RN-25).
type CuponeraAPI interface {
	Catalogo(context.Context, Actor) ([]domain.Beneficio, error)
	ListarBeneficios(context.Context, Actor) ([]domain.Beneficio, error)
	CrearBeneficio(context.Context, Actor, domain.Beneficio) (domain.Beneficio, error)
	ActualizarBeneficio(context.Context, Actor, string, domain.Beneficio) (domain.Beneficio, error)
	Canjear(ctx context.Context, actor Actor, beneficioID, clave string) (domain.ResultadoCanje, bool, error)
	MisCanjes(ctx context.Context, actor Actor, pagina, tamano int) (ports.PaginaCanjes, error)
	ListarCanjes(ctx context.Context, actor Actor, codigo, estado string, pagina, tamano int) (ports.PaginaCanjes, error)
	UsarCanje(ctx context.Context, actor Actor, id string) (domain.CanjeBeneficio, bool, error)
	AnularCanje(ctx context.Context, actor Actor, id, motivo string) (domain.ResultadoCanje, bool, error)
}

type Cuponera struct {
	store   ports.Cuponera
	clock   ports.Clock
	ids     ports.IDGenerator
	codigos ports.CodigoGenerator
	admin   *Administracion
}

func NewCuponera(store ports.Cuponera, clock ports.Clock, ids ports.IDGenerator, codigos ports.CodigoGenerator) *Cuponera {
	return &Cuponera{store: store, clock: clock, ids: ids, codigos: codigos, admin: &Administracion{clock: clock, ids: ids}}
}

func (c *Cuponera) ahora() time.Time { return c.clock.Now().UTC().Truncate(time.Microsecond) }

func alumno(actor Actor) error {
	if actor.Rol != domain.RolAlumno || !domain.UUIDValido(actor.ID) {
		return domain.ErrSinPermiso
	}
	return nil
}

func paginaValida(pagina, tamano int) bool {
	return pagina >= 1 && pagina <= 1000000 && tamano >= 1 && tamano <= 100
}

// Catalogo muestra solo lo canjeable hoy: ACTIVO y dentro de su vigencia (RN-24).
func (c *Cuponera) Catalogo(ctx context.Context, actor Actor) ([]domain.Beneficio, error) {
	if (actor.Rol != domain.RolAlumno && actor.Rol != domain.RolAdministrador) || !domain.UUIDValido(actor.ID) {
		return nil, domain.ErrSinPermiso
	}
	return c.store.ListarBeneficios(ctx, true, domain.DiaNegocio(c.ahora()))
}

// ListarBeneficios muestra al administrador todo el catálogo, incluidos los
// inactivos y vencidos, con la cantidad de canjes de cada uno.
func (c *Cuponera) ListarBeneficios(ctx context.Context, actor Actor) ([]domain.Beneficio, error) {
	if _, err := c.admin.auditoria(actor); err != nil {
		return nil, err
	}
	return c.store.ListarBeneficios(ctx, false, "")
}

func normalizarBeneficio(b domain.Beneficio) domain.Beneficio {
	b.Nombre = strings.TrimSpace(b.Nombre)
	b.Descripcion = strings.TrimSpace(b.Descripcion)
	return b
}

func (c *Cuponera) CrearBeneficio(ctx context.Context, actor Actor, b domain.Beneficio) (domain.Beneficio, error) {
	audit, err := c.admin.auditoria(actor)
	if err != nil {
		return domain.Beneficio{}, err
	}
	b = normalizarBeneficio(b)
	if err := b.Validar(); err != nil {
		return domain.Beneficio{}, err
	}
	if b.ID, err = c.ids.NewID(); err != nil {
		return domain.Beneficio{}, err
	}
	b.CreadoEn, b.ActualizadoEn = audit.Ahora, audit.Ahora
	return c.store.CrearBeneficio(ctx, audit, b)
}

// ActualizarBeneficio reemplaza nombre, descripción, costo, vigencia y estado.
// Los canjes previos conservan los puntos debitados (CA-09.3, CA-09.4).
func (c *Cuponera) ActualizarBeneficio(ctx context.Context, actor Actor, id string, b domain.Beneficio) (domain.Beneficio, error) {
	audit, err := c.admin.auditoria(actor)
	if err != nil {
		return domain.Beneficio{}, err
	}
	if !domain.UUIDValido(id) {
		return domain.Beneficio{}, domain.ErrBeneficioNoEncontrado
	}
	b = normalizarBeneficio(b)
	if err := b.Validar(); err != nil {
		return domain.Beneficio{}, err
	}
	b.ID, b.ActualizadoEn = strings.ToLower(id), audit.Ahora
	return c.store.ActualizarBeneficio(ctx, audit, b)
}

// Canjear aplica HU-20 con Idempotency-Key obligatoria (CA-20.4). La huella
// cubre el beneficio: la misma clave con otro beneficio es un conflicto. Si el
// código aleatorio choca con uno existente se reintenta con otro.
func (c *Cuponera) Canjear(ctx context.Context, actor Actor, beneficio, clave string) (domain.ResultadoCanje, bool, error) {
	if err := alumno(actor); err != nil {
		return domain.ResultadoCanje{}, false, err
	}
	if !claveID.MatchString(clave) {
		return domain.ResultadoCanje{}, false, domain.ErrOperacionInvalida
	}
	if !domain.UUIDValido(beneficio) {
		return domain.ResultadoCanje{}, false, domain.ErrBeneficioNoEncontrado
	}
	beneficio = strings.ToLower(beneficio)
	contenido, err := json.Marshal([]string{"CANJE", beneficio})
	if err != nil {
		return domain.ResultadoCanje{}, false, err
	}
	sum := sha256.Sum256(contenido)
	ahora := c.ahora()
	pedido := ports.PedidoCanje{UsuarioID: strings.ToLower(actor.ID), BeneficioID: beneficio, Clave: clave, Huella: hex.EncodeToString(sum[:]), Ahora: ahora}
	for intento := 0; ; intento++ {
		out, repetido, err := c.store.Canjear(ctx, pedido, func(cuenta domain.CuentaBeneficios, b domain.Beneficio) (domain.CuentaBeneficios, domain.MovimientoPuntos, domain.CanjeBeneficio, error) {
			movimiento, err := c.ids.NewID()
			if err != nil {
				return cuenta, domain.MovimientoPuntos{}, domain.CanjeBeneficio{}, err
			}
			canje, err := c.ids.NewID()
			if err != nil {
				return cuenta, domain.MovimientoPuntos{}, domain.CanjeBeneficio{}, err
			}
			codigo, err := c.codigos.NewCodigo()
			if err != nil {
				return cuenta, domain.MovimientoPuntos{}, domain.CanjeBeneficio{}, err
			}
			return cuenta.Canjear(b, pedido.UsuarioID, movimiento, canje, codigo, ahora)
		})
		if errors.Is(err, domain.ErrCodigoCanjeEnConflicto) && intento < 3 {
			continue
		}
		return out, repetido, err
	}
}

func (c *Cuponera) MisCanjes(ctx context.Context, actor Actor, pagina, tamano int) (ports.PaginaCanjes, error) {
	if err := alumno(actor); err != nil {
		return ports.PaginaCanjes{}, err
	}
	if !paginaValida(pagina, tamano) {
		return ports.PaginaCanjes{}, domain.ErrOperacionInvalida
	}
	return c.store.ListarCanjes(ctx, ports.FiltroCanjes{UsuarioID: strings.ToLower(actor.ID), Pagina: pagina, Tamano: tamano})
}

// ListarCanjes permite a recepción buscar un cupón por su código.
func (c *Cuponera) ListarCanjes(ctx context.Context, actor Actor, codigo, estado string, pagina, tamano int) (ports.PaginaCanjes, error) {
	if _, err := c.admin.auditoria(actor); err != nil {
		return ports.PaginaCanjes{}, err
	}
	codigo = domain.NormalizarCodigo(codigo)
	if !paginaValida(pagina, tamano) || (codigo != "" && !domain.CodigoCanjeValido(codigo)) {
		return ports.PaginaCanjes{}, domain.ErrOperacionInvalida
	}
	switch estado {
	case "", domain.CanjeConfirmado, domain.CanjeUsado, domain.CanjeAnulado:
	default:
		return ports.PaginaCanjes{}, domain.ErrOperacionInvalida
	}
	return c.store.ListarCanjes(ctx, ports.FiltroCanjes{Codigo: codigo, Estado: estado, Pagina: pagina, Tamano: tamano})
}

// UsarCanje registra la entrega del beneficio. Repetirlo no tiene efectos.
func (c *Cuponera) UsarCanje(ctx context.Context, actor Actor, id string) (domain.CanjeBeneficio, bool, error) {
	audit, err := c.admin.auditoria(actor)
	if err != nil {
		return domain.CanjeBeneficio{}, false, err
	}
	if !domain.UUIDValido(id) {
		return domain.CanjeBeneficio{}, false, domain.ErrCanjeNoEncontrado
	}
	return c.store.UsarCanje(ctx, audit, strings.ToLower(id), func(k domain.CanjeBeneficio) (domain.CanjeBeneficio, bool, error) {
		return k.Usar(audit.Ahora)
	})
}

// AnularCanje devuelve los puntos de un cupón no usado (RN-25). Anular uno ya
// ANULADO responde su estado sin nuevos efectos.
func (c *Cuponera) AnularCanje(ctx context.Context, actor Actor, id, motivo string) (domain.ResultadoCanje, bool, error) {
	audit, err := c.admin.auditoria(actor)
	if err != nil {
		return domain.ResultadoCanje{}, false, err
	}
	motivo = strings.TrimSpace(motivo)
	if !domain.UUIDValido(id) {
		return domain.ResultadoCanje{}, false, domain.ErrCanjeNoEncontrado
	}
	if !domain.TextoValido(motivo, 3, 200) {
		return domain.ResultadoCanje{}, false, domain.ErrOperacionInvalida
	}
	return c.store.AnularCanje(ctx, audit, strings.ToLower(id), func(cuenta domain.CuentaBeneficios, k domain.CanjeBeneficio, debito domain.DatosMovimiento) (domain.CuentaBeneficios, domain.MovimientoPuntos, domain.CanjeBeneficio, error) {
		mid, err := c.ids.NewID()
		if err != nil {
			return cuenta, domain.MovimientoPuntos{}, k, err
		}
		return cuenta.AnularCanje(k, debito, mid, motivo, audit.Ahora)
	})
}
