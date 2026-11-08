package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/gin-gonic/gin"
)

func decodeRegistration(c *gin.Context, out any) bool {
	if c.ContentType() != "application/json" || len(c.Request.URL.Query()) != 0 {
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF
}
func RegisterRegistrations(r *gin.Engine, s application.RegistroAPI, auth Authenticator, secret string, logs ...*slog.Logger) {
	h := &PartnersAPI{}
	if len(logs) > 0 {
		h.log = logs[0]
	}
	r.GET("/api/v1/benefits/me", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || c.GetHeader("X-User-Id") == "" {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para consultar tus puntos."))
			return
		}
		out, err := s.ConsultarCuenta(c.Request.Context(), c.GetHeader("X-User-Id"))
		if err != nil {
			if errors.Is(err, domain.ErrCuentaNoVinculada) {
				problem.Write(c, problem.New(404, "CUENTA_NO_ENCONTRADA", "Cuenta pendiente", "Tu cuenta de puntos todavía no está disponible."))
			} else {
				problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudo consultar el saldo."))
			}
			return
		}
		c.JSON(200, gin.H{"balance": out.Saldo, "updatedAt": out.ActualizadoEn})
	})
	for _, version := range []string{"/v1", "/v2"} {
		group := r.Group(version, func(c *gin.Context) {
			c.Request = problem.PartnerRequest(c.Request)
			c.Header("Cache-Control", "no-store")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
			if len(c.Request.Header.Values("X-API-Key")) != 1 {
				h.error(c, application.ErrCredencialInvalida, "")
				return
			}
			id, err := auth.Autenticar(ctx, c.GetHeader("X-API-Key"))
			if err != nil {
				h.error(c, err, "")
				return
			}
			c.Set("partnerID", id)
			c.Next()
			if h.log != nil {
				h.log.InfoContext(c.Request.Context(), "solicitud de integración", slog.String("partner_id", id), slog.String("route", c.FullPath()), slog.Int("status", c.Writer.Status()))
			}
		})
		group.POST("/external-register", func(c *gin.Context) {
			var dto struct {
				Name      string `json:"name"`
				Email     string `json:"email"`
				External  string `json:"externalUserId"`
				Points    int64  `json:"points"`
				Event     string `json:"eventId"`
				Reason    string `json:"reason"`
				Reference string `json:"reference"`
			}
			if len(c.Request.Header.Values("Idempotency-Key")) != 1 || !decodeRegistration(c, &dto) {
				h.error(c, domain.ErrOperacionInvalida, "")
				return
			}
			if dto.Reason == "" {
				dto.Reason = "Registro externo"
			}
			if dto.Reference == "" {
				dto.Reference = dto.Event
				runes := []rune(dto.Reference)
				if len(runes) > 100 {
					dto.Reference = string(runes[:100])
				}
			}
			in := domain.RegistroExterno{PartnerID: c.GetString("partnerID"), Nombre: dto.Name, Email: dto.Email, ExternalUserID: dto.External, Puntos: dto.Points, EventoID: dto.Event, Clave: c.GetHeader("Idempotency-Key"), Motivo: dto.Reason, Referencia: dto.Reference}
			out, err := s.Registrar(c.Request.Context(), in)
			if err != nil {
				h.error(c, err, "")
				return
			}
			c.JSON(202, registrationReceipt(out))
		})
		group.GET("/external-registrations/:eventId", func(c *gin.Context) {
			if len(c.Request.URL.Query()) != 0 {
				h.error(c, domain.ErrOperacionInvalida, "")
				return
			}
			out, err := s.ConsultarRegistro(c.Request.Context(), c.GetString("partnerID"), c.Param("eventId"))
			if err != nil {
				h.error(c, err, "")
				return
			}
			c.JSON(200, registrationReceipt(out))
		})
	}
	r.POST("/internal/v1/external-registrations/fulfill", func(c *gin.Context) {
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Credencial de servicio inválida."))
			return
		}
		var dto struct {
			ID        string `json:"registration_id"`
			Event     string `json:"event_id"`
			Partner   string `json:"partner_id"`
			User      string `json:"user_id"`
			External  string `json:"external_user_id"`
			Points    int64  `json:"points"`
			Reason    string `json:"reason"`
			Reference string `json:"reference"`
		}
		if !decodeRegistration(c, &dto) {
			h.error(c, domain.ErrOperacionInvalida, "")
			return
		}
		balance, err := s.CompletarRegistro(c.Request.Context(), domain.CompletarRegistro{EventoID: dto.Event, RegistroID: dto.ID, PartnerID: dto.Partner, UsuarioID: dto.User, ExternalUserID: dto.External, Puntos: dto.Points, Motivo: dto.Reason, Referencia: dto.Reference})
		if err != nil {
			h.error(c, err, "")
			return
		}
		c.JSON(200, gin.H{"balance": balance})
	})
}
func registrationReceipt(r domain.ReciboRegistro) gin.H {
	return gin.H{"registrationId": r.ID, "eventId": r.EventoID, "externalUserId": r.ExternalUserID, "status": r.Estado}
}
