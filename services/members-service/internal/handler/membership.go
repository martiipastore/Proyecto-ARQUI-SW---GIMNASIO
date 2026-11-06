package handler

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/gin-gonic/gin"
)

func membershipError(c *gin.Context, err error) {
	status, code, detail := 503, problem.CodeDependencyUnavailable, "No se pudo completar la operación de membresía."
	switch {
	case errors.Is(err, model.ErrInvalid):
		status, code, detail = 400, problem.CodeValidation, "Revisá tipo, fechas, cupo, motivo y clave de la operación."
	case errors.Is(err, model.ErrForbidden):
		status, code, detail = 403, "SIN_PERMISO", "Tu rol no permite esta operación."
	case errors.Is(err, model.ErrMembershipNotFound):
		status, code, detail = 404, "MEMBRESIA_NO_ENCONTRADA", "No existe el usuario, tipo o membresía solicitado."
	case errors.Is(err, model.ErrMembershipOverlap):
		status, code, detail = 409, "MEMBRESIAS_SUPERPUESTAS", "El período se superpone con otra membresía activa."
	case errors.Is(err, model.ErrMembershipTransition):
		status, code, detail = 409, "TRANSICION_MEMBRESIA_INVALIDA", "La membresía no permite esta transición."
	case errors.Is(err, model.ErrConflict):
		status, code, detail = 409, "CONFLICTO_IDEMPOTENCIA", "La clave ya identifica otra operación."
	case errors.Is(err, model.ErrMembershipInProgress):
		status, code, detail = 409, "OPERACION_EN_CURSO", "Reintentá la misma operación con la misma clave."
	}
	if status == 503 {
		c.Header("Retry-After", "1")
	}
	problem.Write(c, problem.New(status, code, "Operación de membresía no disponible", detail))
}
func membershipResponse(c *gin.Context, result model.MembershipResult, replay bool, err error, status int) {
	if err != nil {
		membershipError(c, err)
		return
	}
	c.Header("Idempotent-Replayed", strconv.FormatBool(replay))
	c.JSON(status, result)
}
func RegisterMemberships(r *gin.Engine, s *service.Memberships, secret string) {
	group := r.Group("/api/v1/memberships", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !trustedActor(c, secret) {
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	group.GET("/types", func(c *gin.Context) {
		if len(c.Request.URL.Query()) != 0 {
			membershipError(c, model.ErrInvalid)
			return
		}
		out, err := s.Types(c.Request.Context())
		if err != nil {
			membershipError(c, err)
			return
		}
		c.JSON(200, gin.H{"items": out})
	})
	list := func(c *gin.Context) {
		page, size := 1, 20
		query := c.Request.URL.Query()
		for key, values := range query {
			if (key != "pagina" && key != "tamano") || len(values) != 1 {
				membershipError(c, model.ErrInvalid)
				return
			}
		}
		var err error
		if query.Has("pagina") {
			page, err = strconv.Atoi(query.Get("pagina"))
			if err != nil {
				membershipError(c, model.ErrInvalid)
				return
			}
		}
		if query.Has("tamano") {
			size, err = strconv.Atoi(query.Get("tamano"))
			if err != nil {
				membershipError(c, model.ErrInvalid)
				return
			}
		}
		user := c.Param("userID")
		if user == "" {
			user = c.GetHeader("X-User-Id")
		}
		out, err := s.List(c.Request.Context(), c.GetHeader("X-User-Id"), c.GetHeader("X-User-Role"), user, page, size)
		if err != nil {
			membershipError(c, err)
			return
		}
		c.JSON(200, out)
	}
	group.GET("/me", list)
	group.GET("/users/:userID", list)
	write := func(c *gin.Context) bool {
		if len(c.Request.Header.Values("Idempotency-Key")) != 1 {
			membershipError(c, model.ErrInvalid)
			return false
		}
		return true
	}
	assign := func(renew bool) gin.HandlerFunc {
		return func(c *gin.Context) {
			if !write(c) {
				return
			}
			var in model.MembershipInput
			if !decode(c, &in) {
				membershipError(c, model.ErrInvalid)
				return
			}
			if renew {
				if in.UserID != "" && in.UserID != c.Param("userID") {
					membershipError(c, model.ErrInvalid)
					return
				}
				in.UserID = c.Param("userID")
			}
			out, replay, err := s.Assign(c.Request.Context(), c.GetHeader("X-User-Id"), c.GetHeader("X-User-Role"), c.GetHeader("Idempotency-Key"), in, renew)
			membershipResponse(c, out, replay, err, 201)
		}
	}
	group.POST("", assign(false))
	group.POST("/users/:userID/renew", assign(true))
	group.POST("/:id/cancel", func(c *gin.Context) {
		if !write(c) {
			return
		}
		var in struct {
			Reason string `json:"motivo"`
		}
		if !decode(c, &in) {
			membershipError(c, model.ErrInvalid)
			return
		}
		out, replay, err := s.Cancel(c.Request.Context(), c.GetHeader("X-User-Id"), c.GetHeader("X-User-Role"), c.GetHeader("Idempotency-Key"), c.Param("id"), in.Reason)
		membershipResponse(c, out, replay, err, 200)
	})
	group.PUT("/types/:code", func(c *gin.Context) {
		if !write(c) {
			return
		}
		var in struct {
			Name    string `json:"nombre"`
			Months  *int   `json:"duracion_meses"`
			Days    *int   `json:"duracion_dias"`
			Classes *int   `json:"clases"`
			Enabled *bool  `json:"habilitado"`
			Reason  string `json:"motivo"`
		}
		if !decode(c, &in) || in.Enabled == nil {
			membershipError(c, model.ErrInvalid)
			return
		}
		out, replay, err := s.Configure(c.Request.Context(), c.GetHeader("X-User-Id"), c.GetHeader("X-User-Role"), c.GetHeader("Idempotency-Key"), in.Reason, model.MembershipType{Code: c.Param("code"), Name: in.Name, Months: in.Months, Days: in.Days, Classes: in.Classes, Enabled: *in.Enabled})
		membershipResponse(c, out, replay, err, 200)
	})
}
