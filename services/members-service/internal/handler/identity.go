package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/gin-gonic/gin"
)

func decode(c *gin.Context, out any) bool {
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
func identityError(c *gin.Context, err error) {
	status, code, detail := 500, problem.CodeInternal, "No se pudo completar la operación."
	switch {
	case errors.Is(err, model.ErrRateLimited):
		status, code, detail = 429, "LIMITE_EXCEDIDO", "Demasiados intentos. Reintentá en un minuto."
		c.Header("Retry-After", "60")
	case errors.Is(err, model.ErrInvalid):
		status, code, detail = 400, problem.CodeValidation, "Revisá los datos enviados; la contraseña debe tener entre 12 y 72 caracteres."
	case errors.Is(err, model.ErrUnauthorized):
		status, code, detail = 401, "NO_AUTENTICADO", "Credencial o enlace inválido, utilizado o vencido."
	case errors.Is(err, model.ErrNotFound):
		status, code, detail = 404, problem.CodeNotFound, "No existe un registro con ese evento para la integración."
	case errors.Is(err, model.ErrConflict):
		status, code, detail = 409, "CONFLICTO_IDENTIDAD", "La solicitud no coincide con una vinculación o evento anterior."
	}
	problem.Write(c, problem.New(status, code, "No se pudo completar la solicitud", detail))
}

func trustedActor(c *gin.Context, secret string) bool {
	if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || len(c.Request.Header.Values("X-User-Id")) != 1 || !access.ValidRole(c.GetHeader("X-User-Role")) {
		identityError(c, model.ErrUnauthorized)
		return false
	}
	return true
}

func RegisterIdentity(r *gin.Engine, s *service.Identity, secret string) {
	r.Use(func(c *gin.Context) { c.Header("Cache-Control", "no-store"); c.Next() })
	r.POST("/api/v1/auth/resend-activation", func(c *gin.Context) {
		var in struct {
			Email string `json:"email"`
		}
		if !decode(c, &in) {
			identityError(c, model.ErrInvalid)
			return
		}
		if err := s.Resend(c.Request.Context(), in.Email); err != nil {
			identityError(c, err)
			return
		}
		c.JSON(202, gin.H{"status": "accepted"})
	})
	internal := r.Group("/internal/v1", func(c *gin.Context) {
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Service-Key")), []byte(secret)) != 1 {
			identityError(c, model.ErrUnauthorized)
			return
		}
		c.Next()
	})
	internal.POST("/external-registrations", func(c *gin.Context) {
		var in model.ExternalRegistration
		if !decode(c, &in) {
			identityError(c, model.ErrInvalid)
			return
		}
		out, err := s.RegisterExternal(c.Request.Context(), in)
		if err != nil {
			identityError(c, err)
			return
		}
		c.JSON(202, out)
	})
	internal.GET("/external-registrations/:partner/:event", func(c *gin.Context) {
		out, err := s.Registration(c.Request.Context(), c.Param("partner"), c.Param("event"))
		if err != nil {
			identityError(c, err)
			return
		}
		c.JSON(200, out)
	})
	r.POST("/api/v1/auth/activate", func(c *gin.Context) {
		var in struct {
			Token    string `json:"token"`
			Password string `json:"password"`
		}
		if !decode(c, &in) {
			identityError(c, model.ErrInvalid)
			return
		}
		if err := s.Activate(c.Request.Context(), in.Token, in.Password); err != nil {
			identityError(c, err)
			return
		}
		c.JSON(200, gin.H{"status": "activated"})
	})
	r.POST("/api/v1/auth/logout", func(c *gin.Context) {
		if !trustedActor(c, secret) {
			return
		}
		if err := s.Logout(c.Request.Context(), c.GetHeader("X-User-Id")); err != nil {
			identityError(c, err)
			return
		}
		c.Status(204)
	})
	r.POST("/api/v1/auth/login", func(c *gin.Context) {
		var in struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if !decode(c, &in) {
			identityError(c, model.ErrInvalid)
			return
		}
		token, err := s.Login(c.Request.Context(), in.Email, in.Password)
		if err != nil {
			identityError(c, err)
			return
		}
		c.JSON(200, gin.H{"access_token": token, "token_type": "Bearer", "expires_in": int(access.Lifetime.Seconds())})
	})
	r.GET("/api/v1/users/me", func(c *gin.Context) {
		if !trustedActor(c, secret) {
			return
		}
		out, err := s.CurrentUser(c.Request.Context(), c.GetHeader("X-User-Id"))
		if err != nil {
			identityError(c, err)
			return
		}
		c.JSON(200, out)
	})
}
