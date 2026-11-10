package http

import (
	"crypto/subtle"
	"errors"
	"log/slog"
	"strconv"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/gin-gonic/gin"
)

// RegisterAdministration publica la administración del Club de benefits.v1
// (etapa 7: HU-10, HU-11, HU-35, HU-CB-07). La identidad llega solo del
// gateway con la clave interna; la autorización fina la decide application.
func RegisterAdministration(r *gin.Engine, app application.AdministracionAPI, secret string, log *slog.Logger) {
	g := r.Group("/api/v1/benefits", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || !domain.UUIDValido(c.GetHeader("X-User-Id")) {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para administrar el Club."))
			c.Abort()
			return
		}
		c.Set("actor", application.Actor{ID: c.GetHeader("X-User-Id"), Rol: c.GetHeader("X-User-Role")})
		c.Next()
	})
	h := adminHandlers{app: app, log: log}
	g.GET("/partners", h.listar)
	g.POST("/partners", h.crear)
	g.PUT("/partners/:id", h.actualizar)
	g.POST("/partners/:id/rotate-key", h.rotar)
	g.POST("/partners/:id/links", h.vincular)
	g.GET("/accounts/:usuarioId", h.cuenta)
	g.POST("/movements/:id/revert", h.revertir)
}

type adminHandlers struct {
	app application.AdministracionAPI
	log *slog.Logger
}

func actor(c *gin.Context) application.Actor {
	a, _ := c.Get("actor")
	out, _ := a.(application.Actor)
	return out
}

func (h adminHandlers) fallo(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSinPermiso):
		problem.Write(c, problem.New(403, "SIN_PERMISO", "Sin permiso", "Solo el administrador gestiona el Club."))
	case errors.Is(err, domain.ErrOperacionInvalida):
		problem.Write(c, problem.New(400, problem.CodeValidation, "Solicitud inválida", "Revisá los campos y sus límites."))
	case errors.Is(err, domain.ErrIntegracionNoEncontrada):
		problem.Write(c, problem.NotFound("No existe la integración."))
	case errors.Is(err, domain.ErrMovimientoNoEncontrado):
		problem.Write(c, problem.NotFound("No existe el movimiento."))
	case errors.Is(err, domain.ErrCuentaNoVinculada):
		problem.Write(c, problem.New(404, "CUENTA_NO_ENCONTRADA", "Cuenta inexistente", "La persona todavía no tiene cuenta de puntos."))
	case errors.Is(err, domain.ErrVinculoEnConflicto):
		problem.Write(c, problem.New(409, "CONFLICTO_IDENTIDAD", "Vínculo en conflicto", "El identificador externo o la cuenta ya están vinculados de otra forma para esta integración."))
	case errors.Is(err, domain.ErrMovimientoRevertido):
		problem.Write(c, problem.New(409, "TRANSICION_INVALIDA", "Movimiento no reversible", "El movimiento ya fue revertido o es una reversión."))
	case errors.Is(err, domain.ErrSaldoInsuficiente):
		problem.Write(c, problem.New(422, "SALDO_INSUFICIENTE", "Saldo insuficiente", "La reversión dejaría el saldo negativo."))
	case errors.Is(err, domain.ErrConflictoIdempotencia):
		problem.Write(c, problem.New(409, "CONFLICTO_IDEMPOTENCIA", "Clave reutilizada", "La Idempotency-Key ya identifica otra operación."))
	case errors.Is(err, ports.ErrAlmacenNoDisponible):
		c.Header("Retry-After", "1")
		problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudo completar la operación del Club."))
	default:
		if h.log != nil {
			h.log.ErrorContext(c.Request.Context(), "falló la administración del Club", slog.String("route", c.FullPath()))
		}
		problem.Write(c, problem.Internal())
	}
}

type integracionDTO struct {
	Nombre               *string `json:"nombre"`
	Activo               *bool   `json:"activo"`
	PermiteConsulta      *bool   `json:"permite_consulta"`
	PermitePuntos        *bool   `json:"permite_puntos"`
	PermiteRegistro      *bool   `json:"permite_registro"`
	RequiereEvento       *bool   `json:"requiere_evento"`
	MaxPuntosOperacion   *int64  `json:"max_puntos_operacion"`
	MaxPuntosDia         *int64  `json:"max_puntos_dia"`
	MaxSolicitudesMinuto *int    `json:"max_solicitudes_minuto"`
}

// configuracion completa el alta con valores seguros (ADR-016) y exige todos
// los campos al reemplazar con PUT, salvo max_puntos_dia (null = sin límite).
func (d integracionDTO) configuracion(alta bool) (domain.ConfiguracionIntegracion, bool) {
	cfg := domain.ConfiguracionIntegracion{Activo: true, PermiteConsulta: true, PermitePuntos: true, RequiereEvento: true, MaxPuntosOperacion: 1000000, MaxSolicitudesMinuto: 120, MaxPuntosDia: d.MaxPuntosDia}
	if d.Nombre == nil {
		return cfg, false
	}
	cfg.Nombre = *d.Nombre
	if alta {
		if d.Activo != nil {
			return cfg, false // El alta siempre queda ACTIVA (CA-10.1).
		}
	} else if d.Activo == nil || d.PermiteConsulta == nil || d.PermitePuntos == nil || d.PermiteRegistro == nil || d.RequiereEvento == nil || d.MaxPuntosOperacion == nil || d.MaxSolicitudesMinuto == nil {
		return cfg, false
	}
	for dst, src := range map[*bool]*bool{&cfg.Activo: d.Activo, &cfg.PermiteConsulta: d.PermiteConsulta, &cfg.PermitePuntos: d.PermitePuntos, &cfg.PermiteRegistro: d.PermiteRegistro, &cfg.RequiereEvento: d.RequiereEvento} {
		if src != nil {
			*dst = *src
		}
	}
	if d.MaxPuntosOperacion != nil {
		cfg.MaxPuntosOperacion = *d.MaxPuntosOperacion
	}
	if d.MaxSolicitudesMinuto != nil {
		cfg.MaxSolicitudesMinuto = *d.MaxSolicitudesMinuto
	}
	return cfg, true
}

func integracionJSON(i domain.Integracion) gin.H {
	c := i.Config
	estado := "INACTIVO"
	if c.Activo {
		estado = "ACTIVO"
	}
	return gin.H{"id": i.ID, "nombre": c.Nombre, "activo": c.Activo, "estado": estado, "permite_consulta": c.PermiteConsulta, "permite_puntos": c.PermitePuntos, "permite_registro": c.PermiteRegistro, "requiere_evento": c.RequiereEvento, "max_puntos_operacion": c.MaxPuntosOperacion, "max_puntos_dia": c.MaxPuntosDia, "max_solicitudes_minuto": c.MaxSolicitudesMinuto, "vinculaciones": i.Vinculaciones, "creada_en": i.CreadaEn}
}

func sinQuery(c *gin.Context) bool { return len(c.Request.URL.Query()) == 0 }

func (h adminHandlers) listar(c *gin.Context) {
	if !sinQuery(c) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.ListarIntegraciones(c.Request.Context(), actor(c))
	if err != nil {
		h.fallo(c, err)
		return
	}
	items := make([]gin.H, 0, len(out))
	for _, i := range out {
		items = append(items, integracionJSON(i))
	}
	c.JSON(200, gin.H{"items": items})
}

func (h adminHandlers) crear(c *gin.Context) {
	var dto integracionDTO
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	cfg, ok := dto.configuracion(true)
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.CrearIntegracion(c.Request.Context(), actor(c), cfg)
	if err != nil {
		h.fallo(c, err)
		return
	}
	body := integracionJSON(out.Integracion)
	body["api_key"] = out.APIKey
	c.JSON(201, body)
}

func (h adminHandlers) actualizar(c *gin.Context) {
	var dto integracionDTO
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	cfg, ok := dto.configuracion(false)
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.ActualizarIntegracion(c.Request.Context(), actor(c), c.Param("id"), cfg)
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.JSON(200, integracionJSON(out))
}

func (h adminHandlers) rotar(c *gin.Context) {
	if !sinQuery(c) || c.Request.ContentLength > 0 {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.RotarCredencial(c.Request.Context(), actor(c), c.Param("id"))
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.JSON(200, gin.H{"id": out.Integracion.ID, "api_key": out.APIKey})
}

func (h adminHandlers) vincular(c *gin.Context) {
	var dto struct {
		Usuario  string `json:"usuario_id"`
		External string `json:"external_user_id"`
	}
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	creada, err := h.app.Vincular(c.Request.Context(), actor(c), c.Param("id"), dto.Usuario, dto.External)
	if err != nil {
		h.fallo(c, err)
		return
	}
	status := 200
	if creada {
		status = 201
	}
	c.JSON(status, gin.H{"partner_id": c.Param("id"), "usuario_id": dto.Usuario, "external_user_id": dto.External})
}

func (h adminHandlers) cuenta(c *gin.Context) {
	page, size := 1, 20
	for key, values := range c.Request.URL.Query() {
		if len(values) != 1 || (key != "pagina" && key != "tamano") || values[0] == "" {
			h.fallo(c, domain.ErrOperacionInvalida)
			return
		}
	}
	if v, ok := c.GetQuery("pagina"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			page = n
		} else {
			page = 0
		}
	}
	if v, ok := c.GetQuery("tamano"); ok {
		if n, err := strconv.Atoi(v); err == nil {
			size = n
		} else {
			size = 0
		}
	}
	out, err := h.app.CuentaDeUsuario(c.Request.Context(), actor(c), c.Param("usuarioId"), page, size)
	if err != nil {
		h.fallo(c, err)
		return
	}
	links := make([]gin.H, 0, len(out.Vinculaciones))
	for _, v := range out.Vinculaciones {
		links = append(links, gin.H{"partner_id": v.PartnerID, "partner": v.PartnerNombre, "external_user_id": v.ExternalUserID})
	}
	c.JSON(200, gin.H{"usuario_id": out.UsuarioID, "saldo": out.Saldo, "creada_en": out.CreadaEn, "vinculaciones": links, "items": movimientosJSON(out.Pagina, true), "total": out.Pagina.Total, "pagina": page, "tamano": size})
}

func (h adminHandlers) revertir(c *gin.Context) {
	var dto struct {
		Motivo string `json:"motivo"`
	}
	if len(c.Request.Header.Values("Idempotency-Key")) != 1 || !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, repetida, err := h.app.Revertir(c.Request.Context(), actor(c), c.Param("id"), dto.Motivo, c.GetHeader("Idempotency-Key"))
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.Header("Idempotent-Replayed", strconv.FormatBool(repetida))
	m := out.Movimiento.Datos()
	status := 201
	if repetida {
		status = 200
	}
	c.JSON(status, gin.H{"movimiento": gin.H{"id": m.ID, "fecha": m.OcurridoEn, "motivo": m.Motivo, "puntos": m.Puntos, "estado": "CONFIRMADO", "origen": "REVERSION", "revierte_id": m.RevierteID, "secuencia": m.Secuencia}, "saldo": out.Saldo})
}
