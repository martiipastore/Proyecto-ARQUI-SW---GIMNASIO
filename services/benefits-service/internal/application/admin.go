package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

// Actor es la identidad inyectada por el gateway (ADR-006). Benefits vuelve a
// autorizar sobre el recurso: solo el administrador usa estos casos de uso.
type Actor struct{ ID, Rol string }

// IntegracionEmitida devuelve la API key una sola vez (CA-10.1); solo se
// conserva su hash SHA-256, igual que en la autenticación de partners.
type IntegracionEmitida struct {
	Integracion domain.Integracion
	APIKey      string
}

type AdministracionAPI interface {
	ListarIntegraciones(context.Context, Actor) ([]domain.Integracion, error)
	CrearIntegracion(context.Context, Actor, domain.ConfiguracionIntegracion) (IntegracionEmitida, error)
	ActualizarIntegracion(context.Context, Actor, string, domain.ConfiguracionIntegracion) (domain.Integracion, error)
	RotarCredencial(context.Context, Actor, string) (IntegracionEmitida, error)
	Vincular(context.Context, Actor, string, string, string) (bool, error)
	CuentaDeUsuario(context.Context, Actor, string, int, int) (domain.CuentaAdministrada, error)
	Revertir(context.Context, Actor, string, string, string) (domain.ResultadoReversion, bool, error)
}

type Administracion struct {
	store   ports.AdministracionClub
	clock   ports.Clock
	ids     ports.IDGenerator
	secrets ports.SecretGenerator
}

func NewAdministracion(store ports.AdministracionClub, clock ports.Clock, ids ports.IDGenerator, secrets ports.SecretGenerator) *Administracion {
	return &Administracion{store: store, clock: clock, ids: ids, secrets: secrets}
}

func (a *Administracion) auditoria(actor Actor) (ports.Auditoria, error) {
	if actor.Rol != domain.RolAdministrador || !domain.UUIDValido(actor.ID) {
		return ports.Auditoria{}, domain.ErrSinPermiso
	}
	id, err := a.ids.NewID()
	if err != nil {
		return ports.Auditoria{}, err
	}
	return ports.Auditoria{AdministradorID: strings.ToLower(actor.ID), EventoID: id, Ahora: a.clock.Now().UTC().Truncate(time.Microsecond)}, nil
}

func (a *Administracion) ListarIntegraciones(ctx context.Context, actor Actor) ([]domain.Integracion, error) {
	if _, err := a.auditoria(actor); err != nil {
		return nil, err
	}
	return a.store.ListarIntegraciones(ctx)
}

func (a *Administracion) emitir() (string, [32]byte, error) {
	key, err := a.secrets.NewSecret()
	if err != nil {
		return "", [32]byte{}, err
	}
	return key, sha256.Sum256([]byte(key)), nil
}

func (a *Administracion) CrearIntegracion(ctx context.Context, actor Actor, cfg domain.ConfiguracionIntegracion) (IntegracionEmitida, error) {
	audit, err := a.auditoria(actor)
	if err != nil {
		return IntegracionEmitida{}, err
	}
	cfg.Nombre = strings.TrimSpace(cfg.Nombre)
	// CA-10.1: el alta deja la integración ACTIVA.
	cfg.Activo = true
	if err := cfg.Validar(); err != nil {
		return IntegracionEmitida{}, err
	}
	id, err := a.ids.NewID()
	if err != nil {
		return IntegracionEmitida{}, err
	}
	key, hash, err := a.emitir()
	if err != nil {
		return IntegracionEmitida{}, err
	}
	created, err := a.store.CrearIntegracion(ctx, audit, id, cfg, hash)
	if err != nil {
		return IntegracionEmitida{}, err
	}
	return IntegracionEmitida{Integracion: created, APIKey: key}, nil
}

func (a *Administracion) ActualizarIntegracion(ctx context.Context, actor Actor, id string, cfg domain.ConfiguracionIntegracion) (domain.Integracion, error) {
	audit, err := a.auditoria(actor)
	if err != nil {
		return domain.Integracion{}, err
	}
	cfg.Nombre = strings.TrimSpace(cfg.Nombre)
	if !domain.UUIDValido(id) {
		return domain.Integracion{}, domain.ErrIntegracionNoEncontrada
	}
	if err := cfg.Validar(); err != nil {
		return domain.Integracion{}, err
	}
	return a.store.ActualizarIntegracion(ctx, audit, strings.ToLower(id), cfg)
}

// RotarCredencial reemplaza el hash: la key anterior deja de autenticar en la
// misma transacción (CA-10.2).
func (a *Administracion) RotarCredencial(ctx context.Context, actor Actor, id string) (IntegracionEmitida, error) {
	audit, err := a.auditoria(actor)
	if err != nil {
		return IntegracionEmitida{}, err
	}
	if !domain.UUIDValido(id) {
		return IntegracionEmitida{}, domain.ErrIntegracionNoEncontrada
	}
	key, hash, err := a.emitir()
	if err != nil {
		return IntegracionEmitida{}, err
	}
	if err := a.store.RotarCredencial(ctx, audit, strings.ToLower(id), hash); err != nil {
		return IntegracionEmitida{}, err
	}
	return IntegracionEmitida{Integracion: domain.Integracion{ID: strings.ToLower(id)}, APIKey: key}, nil
}

// Vincular asocia la cuenta existente de una persona con el identificador del
// partner (CA-10.3). Un identificador ya usado por otra cuenta se rechaza (CA-10.4).
func (a *Administracion) Vincular(ctx context.Context, actor Actor, partner, usuario, externo string) (bool, error) {
	audit, err := a.auditoria(actor)
	if err != nil {
		return false, err
	}
	if !domain.UUIDValido(partner) {
		return false, domain.ErrIntegracionNoEncontrada
	}
	if !domain.UUIDValido(usuario) || !domain.ExternalIDValido(externo) {
		return false, domain.ErrOperacionInvalida
	}
	return a.store.Vincular(ctx, audit, strings.ToLower(partner), strings.ToLower(usuario), externo)
}

func (a *Administracion) CuentaDeUsuario(ctx context.Context, actor Actor, usuario string, pagina, tamano int) (domain.CuentaAdministrada, error) {
	if _, err := a.auditoria(actor); err != nil {
		return domain.CuentaAdministrada{}, err
	}
	if !domain.UUIDValido(usuario) || pagina < 1 || pagina > 1000000 || tamano < 1 || tamano > 100 {
		return domain.CuentaAdministrada{}, domain.ErrOperacionInvalida
	}
	return a.store.CuentaDeUsuario(ctx, strings.ToLower(usuario), pagina, tamano)
}

// Revertir aplica HU-11 con Idempotency-Key obligatoria. La huella cubre
// movimiento y motivo: misma clave con otros datos es un conflicto.
func (a *Administracion) Revertir(ctx context.Context, actor Actor, movimiento, motivo, clave string) (domain.ResultadoReversion, bool, error) {
	audit, err := a.auditoria(actor)
	if err != nil {
		return domain.ResultadoReversion{}, false, err
	}
	motivo = strings.TrimSpace(motivo)
	if !claveID.MatchString(clave) {
		return domain.ResultadoReversion{}, false, domain.ErrOperacionInvalida
	}
	if !domain.UUIDValido(movimiento) {
		return domain.ResultadoReversion{}, false, domain.ErrMovimientoNoEncontrado
	}
	if !domain.TextoValido(motivo, 3, 200) {
		return domain.ResultadoReversion{}, false, domain.ErrOperacionInvalida
	}
	movimiento = strings.ToLower(movimiento)
	contenido, err := json.Marshal([]string{"REVERT", movimiento, motivo})
	if err != nil {
		return domain.ResultadoReversion{}, false, err
	}
	sum := sha256.Sum256(contenido)
	pedido := ports.ReversionPedida{Auditoria: audit, Clave: clave, Huella: hex.EncodeToString(sum[:]), MovimientoID: movimiento}
	return a.store.Revertir(ctx, pedido, func(c domain.CuentaBeneficios, original domain.DatosMovimiento, revertido bool) (domain.CuentaBeneficios, domain.MovimientoPuntos, error) {
		id, err := a.ids.NewID()
		if err != nil {
			return c, domain.MovimientoPuntos{}, err
		}
		return c.Revertir(original, revertido, id, motivo, audit.Ahora)
	})
}
