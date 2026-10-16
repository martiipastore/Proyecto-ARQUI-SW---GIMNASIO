package http

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type Authenticator interface {
	Autenticar(context.Context, string) (string, error)
}
type PartnersAPI struct {
	casos application.CasosDeUso
	auth  Authenticator
	log   *slog.Logger
}

func RegisterPartners(r *gin.Engine, casos application.CasosDeUso, auth Authenticator, logs ...*slog.Logger) {
	r.RedirectTrailingSlash = false
	h := &PartnersAPI{casos: casos, auth: auth}
	if len(logs) > 0 {
		h.log = logs[0]
	}
	grupo := r.Group("/v1", func(c *gin.Context) {
		c.Request = problem.PartnerRequest(c.Request)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Header(correlation.Header, correlation.FromContext(c.Request.Context()))
		c.Header("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				h.fallo(c, http.StatusInternalServerError, "INTERNAL", "Ocurrió un error inesperado.", "")
			}
		}()
		if len(c.Request.Header.Values("X-API-Key")) != 1 {
			h.fallo(c, 401, "UNAUTHORIZED", "Falta una credencial válida.", "")
			return
		}
		id, err := auth.Autenticar(c.Request.Context(), c.GetHeader("X-API-Key"))
		if err != nil {
			h.error(c, err, "")
			return
		}
		c.Set("partnerID", id)
		c.Next()
	})
	grupo.POST("/accounts/:externalUserId/credits", h.acreditar)
	grupo.GET("/accounts/:externalUserId/balance", h.balance)
	grupo.GET("/accounts/:externalUserId/movements", h.movimientos)
}

func (h *PartnersAPI) fallo(c *gin.Context, status int, code, detail, originalCorrelation string) {
	if status >= 500 && h.log != nil {
		h.log.ErrorContext(c.Request.Context(), "falló operación de Benefits", slog.Int("status", status), slog.String("code", code))
	}
	if status == 503 {
		c.Header("Retry-After", "1")
	}
	titles := map[int]string{400: "Solicitud inválida", 401: "No autenticado", 403: "Sin permiso", 404: "Cuenta no encontrada", 409: "Conflicto de idempotencia", 500: "Error interno", 503: "Servicio no disponible"}
	p := problem.New(status, code, titles[status], detail)
	p.CorrelationID = originalCorrelation
	problem.WritePartnerHTTP(c.Writer, c.Request, p)
	c.Abort()
}

func (h *PartnersAPI) error(c *gin.Context, err error, correlacion string) {
	switch {
	case errors.Is(err, application.ErrCredencialInvalida):
		h.fallo(c, 401, "UNAUTHORIZED", "Credencial inválida o revocada.", correlacion)
	case errors.Is(err, application.ErrPartnerInactivo), errors.Is(err, domain.ErrPartnerNoAutorizado):
		h.fallo(c, 403, "FORBIDDEN", "El partner no está habilitado.", correlacion)
	case errors.Is(err, domain.ErrCuentaNoVinculada):
		h.fallo(c, 404, "ACCOUNT_NOT_FOUND", "La cuenta no está vinculada a este partner.", correlacion)
	case errors.Is(err, domain.ErrConflictoIdempotencia):
		h.fallo(c, 409, "IDEMPOTENCY_CONFLICT", "La clave ya identifica otra operación.", correlacion)
	case errors.Is(err, domain.ErrOperacionEnCurso):
		h.fallo(c, 409, "IDEMPOTENCY_IN_PROGRESS", "La operación original sigue en curso; reintentá con la misma clave.", correlacion)
	case errors.Is(err, domain.ErrOperacionInvalida):
		h.fallo(c, 400, "INVALID_REQUEST", "La solicitud no cumple los campos o límites del contrato.", correlacion)
	case errors.Is(err, ports.ErrAlmacenNoDisponible):
		h.fallo(c, 503, "UNAVAILABLE", "El Club de Beneficios no está disponible temporalmente.", correlacion)
	default:
		h.fallo(c, 500, "INTERNAL", "Ocurrió un error inesperado.", correlacion)
	}
}

type creditRequest struct {
	Points    *int64  `json:"points"`
	Reason    *string `json:"reason"`
	Reference *string `json:"reference"`
}
type operationResult struct {
	MovementID     string    `json:"movementId"`
	ExternalUserID string    `json:"externalUserId"`
	Type           string    `json:"type"`
	Points         int64     `json:"points"`
	Reason         string    `json:"reason"`
	Reference      string    `json:"reference"`
	OccurredAt     time.Time `json:"occurredAt"`
	Balance        int64     `json:"balance"`
}

func (h *PartnersAPI) acreditar(c *gin.Context) {
	if len(c.Request.Header.Values("Idempotency-Key")) != 1 {
		h.error(c, domain.ErrOperacionInvalida, "")
		return
	}
	entrada := application.AcreditarPuntos{PartnerID: c.GetString("partnerID"), ExternalUserID: c.Param("externalUserId"), Clave: c.GetHeader("Idempotency-Key"), Correlacion: correlation.FromContext(c.Request.Context())}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		h.error(c, domain.ErrOperacionInvalida, "")
		return
	}
	contentType, _, _ := mime.ParseMediaType(c.GetHeader("Content-Type"))
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var dto *creditRequest
	entrada.EntradaInvalida = len(c.Request.URL.Query()) != 0 || contentType != "application/json" || decoder.Decode(&dto) != nil || dto == nil
	var extra any
	if !entrada.EntradaInvalida && decoder.Decode(&extra) != io.EOF {
		entrada.EntradaInvalida = true
	}
	if dto == nil || dto.Points == nil || dto.Reason == nil || dto.Reference == nil {
		entrada.EntradaInvalida = true
	}
	if !entrada.EntradaInvalida {
		entrada.Puntos = *dto.Points
		entrada.Motivo = *dto.Reason
		entrada.Referencia = *dto.Reference
	}
	// Las entradas rechazadas también conservan su identidad sin guardar el body.
	if entrada.EntradaInvalida {
		var canonical any
		if json.Unmarshal(body, &canonical) == nil {
			body, _ = json.Marshal(canonical)
		}
		hash := sha256.Sum256(append([]byte(contentType+":"+c.Request.URL.Query().Encode()+":"), body...))
		entrada.HuellaEntrada = hex.EncodeToString(hash[:])
	}
	respuesta, err := h.casos.Acreditar(c.Request.Context(), entrada)
	trace.SpanFromContext(c.Request.Context()).SetAttributes(attribute.Bool("benefits.idempotent_replayed", respuesta.Repetida))
	if err == nil && respuesta.Resultado.Rechazo == "" && h.log != nil {
		h.log.InfoContext(c.Request.Context(), "acreditación procesada", slog.Bool("idempotent_replayed", respuesta.Repetida))
	}
	if respuesta.Repetida {
		c.Header("Idempotent-Replayed", "true")
	} else {
		c.Header("Idempotent-Replayed", "false")
	}
	if len(respuesta.Representacion) > 0 {
		var original respuestaPersistida
		if json.Unmarshal(respuesta.Representacion, &original) != nil {
			h.fallo(c, 500, "INTERNAL", "Ocurrió un error inesperado.", "")
			return
		}
		media := "application/json"
		if original.Estado >= 400 {
			media = problem.ContentType
		}
		c.Data(original.Estado, media, original.Cuerpo)
		return
	}
	if err != nil {
		h.error(c, err, respuesta.Correlacion)
		return
	}
	m := respuesta.Resultado.Movimiento.Datos()
	c.JSON(201, operationResult{MovementID: m.ID, ExternalUserID: entrada.ExternalUserID, Type: "CREDIT", Points: m.Puntos, Reason: m.Motivo, Reference: m.Referencia, OccurredAt: m.OcurridoEn, Balance: respuesta.Resultado.Saldo})
}

func (h *PartnersAPI) balance(c *gin.Context) {
	if len(c.Request.URL.Query()) != 0 {
		h.error(c, domain.ErrOperacionInvalida, "")
		return
	}
	b, err := h.casos.ConsultarBalance(c.Request.Context(), c.GetString("partnerID"), c.Param("externalUserId"))
	if err != nil {
		h.error(c, err, "")
		return
	}
	c.JSON(200, gin.H{"externalUserId": c.Param("externalUserId"), "balance": b.Saldo, "updatedAt": b.ActualizadoEn})
}

func (h *PartnersAPI) movimientos(c *gin.Context) {
	q := c.Request.URL.Query()
	for key, values := range q {
		if (key != "page" && key != "size") || len(values) != 1 {
			h.error(c, domain.ErrOperacionInvalida, "")
			return
		}
	}
	page, size := 1, 20
	var err error
	if q.Has("page") {
		page, err = strconv.Atoi(q.Get("page"))
		if err != nil {
			h.error(c, domain.ErrOperacionInvalida, "")
			return
		}
	}
	if q.Has("size") {
		size, err = strconv.Atoi(q.Get("size"))
		if err != nil {
			h.error(c, domain.ErrOperacionInvalida, "")
			return
		}
	}
	r, err := h.casos.ListarMovimientos(c.Request.Context(), c.GetString("partnerID"), c.Param("externalUserId"), page, size)
	if err != nil {
		h.error(c, err, "")
		return
	}
	items := make([]gin.H, 0, len(r.Movimientos))
	for _, mov := range r.Movimientos {
		m := mov.Datos()
		items = append(items, gin.H{"id": m.ID, "type": "CREDIT", "points": m.Puntos, "reason": m.Motivo, "reference": m.Referencia, "status": "CONFIRMED", "occurredAt": m.OcurridoEn})
	}
	pages := (r.Total + int64(size) - 1) / int64(size)
	c.JSON(200, gin.H{"items": items, "page": page, "size": size, "totalItems": r.Total, "totalPages": pages})
}

// PartnerFallback conserva salud y errores internos, y adapta las rutas públicas.
func PartnerFallback(c *gin.Context) {
	if strings.HasPrefix(c.Request.URL.Path, "/v1/") {
		c.Request = problem.PartnerRequest(c.Request)
		problem.WritePartnerHTTP(c.Writer, c.Request, problem.New(400, "INVALID_REQUEST", "Solicitud inválida", "Ruta o método no disponible en el flujo publicado."))
		c.Abort()
		return
	}
	problem.Write(c, problem.NotFound("No existe el recurso solicitado."))
}
