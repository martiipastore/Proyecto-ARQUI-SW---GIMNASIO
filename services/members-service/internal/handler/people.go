package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/gin-gonic/gin"
)

// peopleError traduce los errores del padrón a RFC 7807 (users.v1, ARCHITECTURE §12.1).
func peopleError(c *gin.Context, err error) {
	status, code, detail := 503, problem.CodeDependencyUnavailable, "No se pudo completar la operación."
	switch {
	case errors.Is(err, model.ErrInvalid):
		status, code, detail = 400, problem.CodeValidation, "Revisá los datos: nombre, apellido, DNI de 7 u 8 dígitos, email, teléfono, fecha de nacimiento y roles."
	case errors.Is(err, model.ErrUnauthorized):
		status, code, detail = 401, "NO_AUTENTICADO", "Ingresá para continuar."
	case errors.Is(err, model.ErrForbidden):
		status, code, detail = 403, "SIN_PERMISO", "Tu rol no permite esta operación."
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrMembershipNotFound):
		status, code, detail = 404, problem.CodeNotFound, "No existe la persona o la asignación solicitada."
	case errors.Is(err, model.ErrDuplicate):
		status, code, detail = 409, "PERSONA_DUPLICADA", "Ya existe una persona con ese DNI o email."
	case errors.Is(err, model.ErrAssignmentDuplicate):
		status, code, detail = 409, "ASIGNACION_DUPLICADA", "Ya existe una asignación vigente entre ese profesional y ese alumno."
	case errors.Is(err, model.ErrConflict):
		status, code, detail = 409, "TRANSICION_INVALIDA", "La persona ya estaba dada de baja."
	}
	if status == 503 {
		c.Header("Retry-After", "1")
	}
	problem.Write(c, problem.New(status, code, "No se pudo completar la solicitud", detail))
}

func withTimeout(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}

// internalOnly exige la credencial entre servicios; el gateway nunca expone
// rutas con un segmento "internal".
func internalOnly(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 || subtle.ConstantTimeCompare([]byte(c.GetHeader("X-Internal-Service-Key")), []byte(secret)) != 1 {
			peopleError(c, model.ErrUnauthorized)
			return
		}
		c.Next()
	}
}

func pagination(c *gin.Context, allowed ...string) (int, int, bool) {
	query := c.Request.URL.Query()
	keys := map[string]bool{"pagina": true, "tamano": true}
	for _, key := range allowed {
		keys[key] = true
	}
	for key, values := range query {
		if !keys[key] || len(values) != 1 {
			return 0, 0, false
		}
	}
	page, size := 1, 20
	var err error
	if query.Has("pagina") {
		if page, err = strconv.Atoi(query.Get("pagina")); err != nil {
			return 0, 0, false
		}
	}
	if query.Has("tamano") {
		if size, err = strconv.Atoi(query.Get("tamano")); err != nil {
			return 0, 0, false
		}
	}
	return page, size, true
}

// RegisterPeople expone el padrón (HU-01, HU-02, HU-21) y sus APIs internas.
func RegisterPeople(r *gin.Engine, s *service.People, secret string) {
	users := r.Group("/api/v1/users", func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if !trustedActor(c, secret) {
			return
		}
		c.Next()
	}, withTimeout)
	actor := func(c *gin.Context) (string, string) { return c.GetHeader("X-User-Id"), c.GetHeader("X-User-Role") }

	users.GET("", func(c *gin.Context) {
		page, size, ok := pagination(c, "rol", "estado", "q")
		if !ok {
			peopleError(c, model.ErrInvalid)
			return
		}
		query := c.Request.URL.Query()
		id, role := actor(c)
		out, err := s.List(c.Request.Context(), id, role, model.PersonFilter{Role: query.Get("rol"), State: query.Get("estado"), Query: query.Get("q"), Page: page, Size: size})
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})
	users.POST("", func(c *gin.Context) {
		var in model.PersonInput
		if !decode(c, &in) {
			peopleError(c, model.ErrInvalid)
			return
		}
		id, role := actor(c)
		out, err := s.Create(c.Request.Context(), id, role, in)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(201, out)
	})
	users.GET("/me/students", func(c *gin.Context) {
		id, role := actor(c)
		out, err := s.MyStudents(c.Request.Context(), id, role)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, gin.H{"items": out})
	})
	users.GET("/:id", func(c *gin.Context) {
		id, role := actor(c)
		person, assignments, err := s.Get(c.Request.Context(), id, role, c.Param("id"))
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, gin.H{"persona": person, "asignaciones": assignments})
	})
	users.PUT("/:id", func(c *gin.Context) {
		var in model.PersonUpdate
		if !decode(c, &in) {
			peopleError(c, model.ErrInvalid)
			return
		}
		id, role := actor(c)
		out, err := s.Update(c.Request.Context(), id, role, c.Param("id"), in)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})
	users.POST("/:id/deactivate", func(c *gin.Context) {
		var in struct {
			Reason string `json:"motivo"`
		}
		if !decode(c, &in) {
			peopleError(c, model.ErrInvalid)
			return
		}
		id, role := actor(c)
		out, err := s.Deactivate(c.Request.Context(), id, role, c.Param("id"), in.Reason)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})
	users.POST("/:id/assignments", func(c *gin.Context) {
		var in struct {
			StudentID string `json:"alumno_id"`
		}
		if !decode(c, &in) {
			peopleError(c, model.ErrInvalid)
			return
		}
		id, role := actor(c)
		out, err := s.Assign(c.Request.Context(), id, role, c.Param("id"), in.StudentID)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(201, out)
	})
	users.POST("/:id/assignments/:studentID/revoke", func(c *gin.Context) {
		id, role := actor(c)
		out, err := s.Revoke(c.Request.Context(), id, role, c.Param("id"), c.Param("studentID"))
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})

	internal := r.Group("/internal/v1", internalOnly(secret), withTimeout)
	summaries := func(find func(context.Context, []string) ([]model.StudentSummary, error)) gin.HandlerFunc {
		return func(c *gin.Context) {
			query := c.Request.URL.Query()
			if len(query) != 1 || len(query["ids"]) != 1 {
				peopleError(c, model.ErrInvalid)
				return
			}
			out, err := find(c.Request.Context(), strings.Split(query.Get("ids"), ","))
			if err != nil {
				peopleError(c, err)
				return
			}
			c.JSON(200, gin.H{"items": out})
		}
	}
	internal.GET("/alumnos", summaries(s.StudentsByID))
	// Etapa 13 (D-92): Benefits obtiene el email para acreditar en MELT.
	internal.GET("/alumnos/:id/contacto", func(c *gin.Context) {
		email, err := s.ContactEmail(c.Request.Context(), c.Param("id"))
		if err != nil {
			peopleError(c, err)
			return
		}
		c.Header("Cache-Control", "no-store")
		c.JSON(200, gin.H{"email": email})
	})
	internal.GET("/profesores", summaries(s.TeachersByID))
	// POST para que el DNI no viaje en la URL ni quede en logs de acceso.
	internal.POST("/alumnos/buscar-dni", func(c *gin.Context) {
		var in struct {
			DNI string `json:"dni"`
		}
		if !decode(c, &in) {
			peopleError(c, model.ErrInvalid)
			return
		}
		out, err := s.StudentByDNI(c.Request.Context(), in.DNI)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})
	internal.GET("/asignaciones", func(c *gin.Context) {
		query := c.Request.URL.Query()
		if len(query) != 2 || len(query["profesional_id"]) != 1 || len(query["alumno_id"]) != 1 {
			peopleError(c, model.ErrInvalid)
			return
		}
		ok, err := s.ActiveAssignment(c.Request.Context(), query.Get("profesional_id"), query.Get("alumno_id"))
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, gin.H{"vigente": ok})
	})
	// Etapa 10: listas vigentes para training (RN-30, RN-37), sin datos de contacto.
	internal.GET("/asignaciones/vigentes", func(c *gin.Context) {
		query := c.Request.URL.Query()
		if len(query) != 1 || (len(query["alumno_id"]) != 1 && len(query["profesional_id"]) != 1) {
			peopleError(c, model.ErrInvalid)
			return
		}
		out, err := s.ActiveAssignments(c.Request.Context(), query.Get("alumno_id"), query.Get("profesional_id"))
		if err != nil {
			peopleError(c, err)
			return
		}
		items := make([]gin.H, 0, len(out))
		for _, a := range out {
			item := gin.H{"profesional_id": a.ProfessionalID, "alumno_id": a.StudentID}
			if a.Professional != nil {
				item["profesional_rol"] = a.Professional.Role
				item["profesional_nombre"] = a.Professional.Name
				item["profesional_apellido"] = a.Professional.LastName
			}
			if a.Student != nil {
				item["alumno_nombre"] = a.Student.Name
				item["alumno_apellido"] = a.Student.LastName
			}
			items = append(items, item)
		}
		c.JSON(200, gin.H{"items": items})
	})
}

// RegisterCoverage expone la vigencia de membresías para booking (RN-12).
func RegisterCoverage(r *gin.Engine, s *service.Memberships, secret string) {
	r.GET("/internal/v1/alumnos/:id/vigencia", internalOnly(secret), withTimeout, func(c *gin.Context) {
		query := c.Request.URL.Query()
		var dates []string
		for key, values := range query {
			if key != "fechas" || len(values) != 1 {
				peopleError(c, model.ErrInvalid)
				return
			}
		}
		if raw := query.Get("fechas"); raw != "" {
			dates = strings.Split(raw, ",")
		}
		out, err := s.Coverage(c.Request.Context(), c.Param("id"), dates)
		if err != nil {
			peopleError(c, err)
			return
		}
		c.JSON(200, out)
	})
}
