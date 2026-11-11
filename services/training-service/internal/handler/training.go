package handler

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

// Register publica training.v1 (etapa 10). La identidad llega solo del gateway
// junto con la clave interna (ADR-006, regla 10); la autorización fina por
// asignación vigente la decide la capa service (RN-30).
func Register(r *gin.Engine, s *service.Training, secret string, log *slog.Logger) {
	api := r.Group("/api/v1", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 || c.GetHeader("X-User-Id") == "" {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para continuar."))
			c.Abort()
			return
		}
		c.Next()
	})
	h := handlers{s: s, log: log}

	api.GET("/training/plans/me", h.sinQuery(func(c *gin.Context) (any, error) { return s.MiPlanEntrenamiento(c.Request.Context(), actor(c)) }))
	api.GET("/training/plans", h.porAlumno(func(c *gin.Context, alumno string) (any, error) {
		return s.PlanesEntrenamientoDe(c.Request.Context(), actor(c), alumno)
	}))
	api.POST("/training/plans", func(c *gin.Context) {
		var in model.PlanEntrenamiento
		if !decode(c, &in) {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 201)(s.CrearPlanEntrenamiento(c.Request.Context(), actor(c), in))
	})

	api.GET("/nutrition/plans/me", h.sinQuery(func(c *gin.Context) (any, error) { return s.MiPlanAlimenticio(c.Request.Context(), actor(c)) }))
	api.GET("/nutrition/plans", h.porAlumno(func(c *gin.Context, alumno string) (any, error) {
		return s.PlanesAlimenticiosDe(c.Request.Context(), actor(c), alumno)
	}))
	api.POST("/nutrition/plans", func(c *gin.Context) {
		var in model.PlanAlimenticio
		if !decode(c, &in) {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 201)(s.CrearPlanAlimenticio(c.Request.Context(), actor(c), in))
	})

	api.GET("/nutrition/measurements/me", h.sinQuery(func(c *gin.Context) (any, error) {
		items, err := s.MisMediciones(c.Request.Context(), actor(c))
		return gin.H{"items": items}, err
	}))
	api.GET("/nutrition/measurements", h.porAlumno(func(c *gin.Context, alumno string) (any, error) {
		items, err := s.MedicionesDe(c.Request.Context(), actor(c), alumno)
		return gin.H{"items": items}, err
	}))
	api.POST("/nutrition/measurements", func(c *gin.Context) {
		var in model.Medicion
		if !decode(c, &in) {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 201)(s.RegistrarMedicion(c.Request.Context(), actor(c), in))
	})

	api.GET("/nutrition/questions/me", h.sinQuery(func(c *gin.Context) (any, error) {
		items, err := s.MisConsultas(c.Request.Context(), actor(c))
		return gin.H{"items": items}, err
	}))
	api.GET("/nutrition/questions", h.sinQuery(func(c *gin.Context) (any, error) {
		items, err := s.Bandeja(c.Request.Context(), actor(c))
		return gin.H{"items": items}, err
	}))
	api.POST("/nutrition/questions", func(c *gin.Context) {
		var in struct {
			Pregunta string `json:"pregunta"`
		}
		if !decode(c, &in) {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 201)(s.EnviarConsulta(c.Request.Context(), actor(c), in.Pregunta))
	})
	api.POST("/nutrition/questions/:id/answer", func(c *gin.Context) {
		var in struct {
			Respuesta string `json:"respuesta"`
		}
		if !decode(c, &in) {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 200)(s.Responder(c.Request.Context(), actor(c), c.Param("id"), in.Respuesta))
	})
}

type handlers struct {
	s   *service.Training
	log *slog.Logger
}

func actor(c *gin.Context) model.Actor {
	return model.Actor{ID: c.GetHeader("X-User-Id"), Rol: c.GetHeader("X-User-Role")}
}

// decode exige JSON, sin campos desconocidos ni query, hasta 64 KiB.
func decode(c *gin.Context, out any) bool {
	if c.ContentType() != "application/json" || len(c.Request.URL.Query()) != 0 {
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return false
	}
	var extra any
	return d.Decode(&extra) == io.EOF
}

func (h handlers) responder(c *gin.Context, status int) func(any, error) {
	return func(out any, err error) {
		if err != nil {
			h.fallo(c, err)
			return
		}
		c.JSON(status, out)
	}
}

func (h handlers) sinQuery(fn func(*gin.Context) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(c.Request.URL.Query()) != 0 {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 200)(fn(c))
	}
}

func (h handlers) porAlumno(fn func(*gin.Context, string) (any, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Request.URL.Query()
		if len(q) != 1 || len(q["alumno_id"]) != 1 {
			h.fallo(c, model.ErrValidacion)
			return
		}
		h.responder(c, 200)(fn(c, q.Get("alumno_id")))
	}
}

// fallo traduce con el catálogo de ARCHITECTURE §12.1 y training.v1.
func (h handlers) fallo(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrNoDisponible):
		c.Header("Retry-After", "1")
		problem.Write(c, problem.New(503, problem.CodeDependencyUnavailable, "Servicio no disponible", "No se pudo confirmar la operación. Reintentá en unos segundos."))
	case errors.Is(err, model.ErrValidacion):
		problem.Write(c, problem.New(400, problem.CodeValidation, "Solicitud inválida", "Revisá los datos enviados."))
	case errors.Is(err, model.ErrSinPermiso):
		problem.Write(c, problem.New(403, "SIN_PERMISO", "Sin permiso", "Tu rol o tu asignación no permiten esta operación sobre el alumno."))
	case errors.Is(err, model.ErrNoEncontrado):
		problem.Write(c, problem.NotFound("No hay un registro vigente."))
	case errors.Is(err, model.ErrSinNutricionista):
		problem.Write(c, problem.New(422, "SIN_NUTRICIONISTA", "Sin nutricionista", "No tenés un nutricionista asignado."))
	case errors.Is(err, model.ErrYaRespondida):
		problem.Write(c, problem.New(409, "TRANSICION_INVALIDA", "Consulta respondida", "La consulta ya tiene respuesta; podés enviar una nueva."))
	default:
		if h.log != nil {
			h.log.ErrorContext(c.Request.Context(), "falló una operación de training", slog.String("route", c.FullPath()))
		}
		problem.Write(c, problem.Internal())
	}
}
