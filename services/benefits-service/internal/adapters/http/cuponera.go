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

// RegisterCuponera publica la cuponera interna de benefits.v1 (D-94): catálogo
// y canje para el alumno, gestión del catálogo y de los cupones para el
// administrador. Igual que la administración del Club, la identidad llega solo
// del gateway con la clave interna y la autorización fina la decide application.
func RegisterCuponera(r *gin.Engine, app application.CuponeraAPI, secret string, log *slog.Logger) {
	g := r.Group("/api/v1/benefits", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || !domain.UUIDValido(c.GetHeader("X-User-Id")) {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para usar el Club."))
			c.Abort()
			return
		}
		c.Set("actor", application.Actor{ID: c.GetHeader("X-User-Id"), Rol: c.GetHeader("X-User-Role")})
		c.Next()
	})
	h := cuponeraHandlers{app: app, log: log}
	g.GET("/catalog", h.catalogo)
	g.GET("/rewards", h.listarBeneficios)
	g.POST("/rewards", h.crearBeneficio)
	g.PUT("/rewards/:id", h.actualizarBeneficio)
	g.POST("/redemptions", h.canjear)
	g.GET("/me/redemptions", h.misCanjes)
	g.GET("/redemptions", h.listarCanjes)
	g.POST("/redemptions/:id/use", h.usar)
	g.POST("/redemptions/:id/cancel", h.anular)
}

type cuponeraHandlers struct {
	app application.CuponeraAPI
	log *slog.Logger
}

func (h cuponeraHandlers) fallo(c *gin.Context, err error) {
	switch {
	case errors.Is(err, domain.ErrSinPermiso):
		problem.Write(c, problem.New(403, "SIN_PERMISO", "Sin permiso", "Tu rol no permite esta operación del Club."))
	case errors.Is(err, domain.ErrOperacionInvalida):
		problem.Write(c, problem.New(400, problem.CodeValidation, "Solicitud inválida", "Revisá los campos y sus límites."))
	case errors.Is(err, domain.ErrBeneficioNoEncontrado):
		problem.Write(c, problem.NotFound("No existe el beneficio."))
	case errors.Is(err, domain.ErrCanjeNoEncontrado):
		problem.Write(c, problem.NotFound("No existe el canje."))
	case errors.Is(err, domain.ErrBeneficioNoDisponible):
		problem.Write(c, problem.New(422, "BENEFICIO_NO_DISPONIBLE", "Beneficio no disponible", "El beneficio está inactivo o fuera de su vigencia."))
	case errors.Is(err, domain.ErrSaldoInsuficiente):
		problem.Write(c, problem.New(422, "SALDO_INSUFICIENTE", "Saldo insuficiente", "No tenés puntos suficientes para este beneficio."))
	case errors.Is(err, domain.ErrTransicionCanje):
		problem.Write(c, problem.New(409, "TRANSICION_INVALIDA", "Canje no modificable", "El cupón ya fue usado o anulado."))
	case errors.Is(err, domain.ErrConflictoIdempotencia):
		problem.Write(c, problem.New(409, "CONFLICTO_IDEMPOTENCIA", "Clave reutilizada", "La Idempotency-Key ya identifica otra operación."))
	case errors.Is(err, ports.ErrAlmacenNoDisponible):
		c.Header("Retry-After", "1")
		problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudo completar la operación del Club."))
	default:
		if h.log != nil {
			h.log.ErrorContext(c.Request.Context(), "falló la cuponera del Club", slog.String("route", c.FullPath()))
		}
		problem.Write(c, problem.Internal())
	}
}

type beneficioDTO struct {
	Nombre       *string `json:"nombre"`
	Descripcion  *string `json:"descripcion"`
	Costo        *int64  `json:"costo"`
	VigenteDesde *string `json:"vigente_desde"`
	VigenteHasta *string `json:"vigente_hasta"`
	Activo       *bool   `json:"activo"`
}

// beneficio exige todos los campos salvo descripción y vigente_hasta (null =
// sin vencimiento). El alta puede omitir activo y queda ACTIVO.
func (d beneficioDTO) beneficio(alta bool) (domain.Beneficio, bool) {
	if d.Nombre == nil || d.Costo == nil || d.VigenteDesde == nil || (!alta && d.Activo == nil) {
		return domain.Beneficio{}, false
	}
	b := domain.Beneficio{Nombre: *d.Nombre, Costo: *d.Costo, VigenteDesde: *d.VigenteDesde, Activo: true}
	if d.Descripcion != nil {
		b.Descripcion = *d.Descripcion
	}
	if d.VigenteHasta != nil {
		b.VigenteHasta = *d.VigenteHasta
	}
	if d.Activo != nil {
		b.Activo = *d.Activo
	}
	return b, true
}

func beneficioJSON(b domain.Beneficio, admin bool) gin.H {
	var hasta any
	if b.VigenteHasta != "" {
		hasta = b.VigenteHasta
	}
	out := gin.H{"id": b.ID, "nombre": b.Nombre, "descripcion": b.Descripcion, "costo": b.Costo, "vigente_desde": b.VigenteDesde, "vigente_hasta": hasta}
	if admin {
		out["activo"] = b.Activo
		out["canjes"] = b.Canjes
		out["creado_en"] = b.CreadoEn
		out["actualizado_en"] = b.ActualizadoEn
	}
	return out
}

func canjeJSON(k domain.CanjeBeneficio, admin bool) gin.H {
	out := gin.H{"id": k.ID, "beneficio_id": k.BeneficioID, "beneficio": k.BeneficioNombre, "codigo": k.Codigo, "puntos": k.Puntos, "estado": k.Estado, "creado_en": k.CreadoEn, "usado_en": k.UsadoEn, "anulado_en": k.AnuladoEn}
	if k.MotivoAnulacion != "" {
		out["motivo_anulacion"] = k.MotivoAnulacion
	}
	if admin {
		out["usuario_id"] = k.UsuarioID
		out["movimiento_id"] = k.MovimientoID
		if k.AnulacionID != "" {
			out["anulacion_id"] = k.AnulacionID
		}
	}
	return out
}

func (h cuponeraHandlers) catalogo(c *gin.Context) {
	if !sinQuery(c) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.Catalogo(c.Request.Context(), actor(c))
	if err != nil {
		h.fallo(c, err)
		return
	}
	items := make([]gin.H, 0, len(out))
	for _, b := range out {
		items = append(items, beneficioJSON(b, false))
	}
	c.JSON(200, gin.H{"items": items})
}

func (h cuponeraHandlers) listarBeneficios(c *gin.Context) {
	if !sinQuery(c) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.ListarBeneficios(c.Request.Context(), actor(c))
	if err != nil {
		h.fallo(c, err)
		return
	}
	items := make([]gin.H, 0, len(out))
	for _, b := range out {
		items = append(items, beneficioJSON(b, true))
	}
	c.JSON(200, gin.H{"items": items})
}

func (h cuponeraHandlers) crearBeneficio(c *gin.Context) {
	var dto beneficioDTO
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	b, ok := dto.beneficio(true)
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.CrearBeneficio(c.Request.Context(), actor(c), b)
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.JSON(201, beneficioJSON(out, true))
}

func (h cuponeraHandlers) actualizarBeneficio(c *gin.Context) {
	var dto beneficioDTO
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	b, ok := dto.beneficio(false)
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.ActualizarBeneficio(c.Request.Context(), actor(c), c.Param("id"), b)
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.JSON(200, beneficioJSON(out, true))
}

// canjear exige Idempotency-Key (regla 9, CA-20.4): repetirla devuelve el
// mismo cupón con 200 e Idempotent-Replayed: true.
func (h cuponeraHandlers) canjear(c *gin.Context) {
	var dto struct {
		Beneficio string `json:"beneficio_id"`
	}
	if len(c.Request.Header.Values("Idempotency-Key")) != 1 || !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, repetido, err := h.app.Canjear(c.Request.Context(), actor(c), dto.Beneficio, c.GetHeader("Idempotency-Key"))
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.Header("Idempotent-Replayed", strconv.FormatBool(repetido))
	status := 201
	if repetido {
		status = 200
	}
	c.JSON(status, gin.H{"canje": canjeJSON(out.Canje, false), "saldo": out.Saldo})
}

// paginacion lee pagina y tamano, más los filtros permitidos de cada ruta.
func paginacion(c *gin.Context, permitidos ...string) (int, int, bool) {
	page, size := 1, 20
	for key, values := range c.Request.URL.Query() {
		ok := key == "pagina" || key == "tamano"
		for _, p := range permitidos {
			ok = ok || key == p
		}
		if len(values) != 1 || !ok || values[0] == "" {
			return 0, 0, false
		}
	}
	for key, dst := range map[string]*int{"pagina": &page, "tamano": &size} {
		if v, ok := c.GetQuery(key); ok {
			n, err := strconv.Atoi(v)
			if err != nil {
				n = 0
			}
			*dst = n
		}
	}
	return page, size, true
}

func paginaCanjesJSON(c *gin.Context, out ports.PaginaCanjes, page, size int, admin bool) {
	items := make([]gin.H, 0, len(out.Items))
	for _, k := range out.Items {
		items = append(items, canjeJSON(k, admin))
	}
	c.JSON(200, gin.H{"items": items, "total": out.Total, "pagina": page, "tamano": size})
}

func (h cuponeraHandlers) misCanjes(c *gin.Context) {
	page, size, ok := paginacion(c)
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.MisCanjes(c.Request.Context(), actor(c), page, size)
	if err != nil {
		h.fallo(c, err)
		return
	}
	paginaCanjesJSON(c, out, page, size, false)
}

func (h cuponeraHandlers) listarCanjes(c *gin.Context) {
	page, size, ok := paginacion(c, "codigo", "estado")
	if !ok {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, err := h.app.ListarCanjes(c.Request.Context(), actor(c), c.Query("codigo"), c.Query("estado"), page, size)
	if err != nil {
		h.fallo(c, err)
		return
	}
	paginaCanjesJSON(c, out, page, size, true)
}

func (h cuponeraHandlers) usar(c *gin.Context) {
	if !sinQuery(c) || c.Request.ContentLength > 0 {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, cambio, err := h.app.UsarCanje(c.Request.Context(), actor(c), c.Param("id"))
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.Header("Idempotent-Replayed", strconv.FormatBool(!cambio))
	c.JSON(200, gin.H{"canje": canjeJSON(out, true)})
}

func (h cuponeraHandlers) anular(c *gin.Context) {
	var dto struct {
		Motivo string `json:"motivo"`
	}
	if !decodeRegistration(c, &dto) {
		h.fallo(c, domain.ErrOperacionInvalida)
		return
	}
	out, cambio, err := h.app.AnularCanje(c.Request.Context(), actor(c), c.Param("id"), dto.Motivo)
	if err != nil {
		h.fallo(c, err)
		return
	}
	c.Header("Idempotent-Replayed", strconv.FormatBool(!cambio))
	c.JSON(200, gin.H{"canje": canjeJSON(out.Canje, true), "saldo": out.Saldo})
}
