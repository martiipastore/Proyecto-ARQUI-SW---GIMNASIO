// Package http es el adaptador de entrada HTTP (Gin) de booking-service. Traduce
// solicitudes a llamadas a los puertos de entrada de internal/application y sus
// errores a RFC 7807. No contiene reglas de negocio.
package http

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

// NewRouter crea el router HTTP. checks son las dependencias propias que verifica /health/ready.
func NewRouter(log *slog.Logger, healthTimeout time.Duration, checks ...health.Check) *gin.Engine {
	r := httpx.NewRouter(log)
	health.Register(r, healthTimeout, checks...)
	return r
}

// Register expone booking.v1 detrás del gateway. ahora es el reloj del servicio,
// que define los estados derivados (EN_CURSO, FINALIZADA) en las respuestas.
func Register(r *gin.Engine, s *application.Servicio, secret string, ahora func() time.Time) {
	api := r.Group("/api/v1", confiable(secret))

	// Actividades y horarios (HU-06, HU-07).
	api.GET("/activities", func(c *gin.Context) {
		if !consulta(c) {
			return
		}
		items, err := s.Actividades(c.Request.Context())
		if err != nil {
			responderError(c, err)
			return
		}
		out := make([]actividadJSON, 0, len(items))
		for _, a := range items {
			out = append(out, actividadDTO(a))
		}
		c.JSON(200, gin.H{"items": out})
	})
	api.POST("/activities", func(c *gin.Context) {
		var in struct {
			Nombre    string `json:"nombre"`
			Capacidad int    `json:"capacidad_maxima"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		a, err := s.CrearActividad(c.Request.Context(), actor(c), in.Nombre, in.Capacidad)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(201, actividadDTO(a))
	})
	api.PUT("/activities/:id", func(c *gin.Context) {
		var in struct {
			Nombre    *string `json:"nombre"`
			Capacidad *int    `json:"capacidad_maxima"`
			Activa    *bool   `json:"activa"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		a, err := s.ModificarActividad(c.Request.Context(), actor(c), c.Param("id"), in.Nombre, in.Capacidad, in.Activa)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, actividadDTO(a))
	})
	api.GET("/activities/:id/schedules", func(c *gin.Context) {
		if !consulta(c) {
			return
		}
		items, err := s.Horarios(c.Request.Context(), actor(c), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		out := make([]horarioJSON, 0, len(items))
		for _, h := range items {
			out = append(out, horarioDTO(h))
		}
		c.JSON(200, gin.H{"items": out})
	})
	api.POST("/activities/:id/schedules", func(c *gin.Context) {
		var in struct {
			DiaSemana  int    `json:"dia_semana"`
			Inicio     string `json:"inicio"`
			Duracion   int    `json:"duracion_minutos"`
			ProfesorID string `json:"profesor_id"`
			Capacidad  int    `json:"capacidad"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		h, err := s.CrearHorario(c.Request.Context(), actor(c), c.Param("id"), application.NuevoHorario{
			DiaSemana: in.DiaSemana, Inicio: in.Inicio, DuracionMinutos: in.Duracion, ProfesorID: in.ProfesorID, Capacidad: in.Capacidad,
		})
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(201, horarioDTO(h))
	})
	api.POST("/classes/generate", func(c *gin.Context) {
		var in struct {
			Desde string `json:"desde"`
			Hasta string `json:"hasta"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		n, err := s.GenerarClases(c.Request.Context(), actor(c), in.Desde, in.Hasta)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, gin.H{"creadas": n})
	})

	// Clases (HU-08, HU-13, HU-21).
	api.GET("/classes", func(c *gin.Context) {
		if !consulta(c, "fecha_desde", "fecha_hasta", "actividad_id", "franja", "solo_disponibles", "pagina", "tamano") {
			return
		}
		f := domain.FiltroClases{ActividadID: c.Query("actividad_id"), Franja: c.Query("franja")}
		var err error
		if v := c.Query("fecha_desde"); v != "" {
			if f.Desde, err = domain.ParseFecha(v); err != nil {
				responderError(c, err)
				return
			}
		}
		if v := c.Query("fecha_hasta"); v != "" {
			if f.Hasta, err = domain.ParseFecha(v); err != nil {
				responderError(c, err)
				return
			}
			f.Hasta = f.Hasta.AddDate(0, 0, 1) // fecha_hasta es inclusiva
		}
		switch c.Query("solo_disponibles") {
		case "", "false":
		case "true":
			f.SoloDisponibles = true
		default:
			responderError(c, domain.ErrValidacion)
			return
		}
		var ok bool
		if f.Pagina, ok = entero(c, "pagina", 1); !ok {
			return
		}
		if f.Tamano, ok = entero(c, "tamano", 20); !ok {
			return
		}
		page, err := s.BuscarClases(c.Request.Context(), actor(c), f)
		if err != nil {
			responderError(c, err)
			return
		}
		now := ahora()
		out := make([]claseJSON, 0, len(page.Items))
		for _, cl := range page.Items {
			out = append(out, claseDTO(cl, now))
		}
		c.JSON(200, pagina(out, page.Total, page.Pagina, page.Tamano))
	})
	api.GET("/classes/mine", func(c *gin.Context) {
		if !consulta(c) {
			return
		}
		items, err := s.MisClases(c.Request.Context(), actor(c))
		if err != nil {
			responderError(c, err)
			return
		}
		now := ahora()
		out := make([]claseJSON, 0, len(items))
		for _, cl := range items {
			out = append(out, claseDTO(cl, now))
		}
		c.JSON(200, gin.H{"items": out})
	})
	api.GET("/classes/:id", func(c *gin.Context) {
		if !consulta(c) {
			return
		}
		d, err := s.DetalleClase(c.Request.Context(), actor(c), c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		body := gin.H{"clase": claseDTO(d.Clase, ahora())}
		if d.Completo {
			type reservaClase struct {
				ID           string     `json:"id"`
				AlumnoID     string     `json:"alumno_id"`
				AlumnoNombre *string    `json:"alumno_nombre"`
				Estado       string     `json:"estado"`
				CreadaEn     time.Time  `json:"creada_en"`
				IngresoEn    *time.Time `json:"ingreso_en"`
			}
			reservas := make([]reservaClase, 0, len(d.Reservas))
			for _, r := range d.Reservas {
				reservas = append(reservas, reservaClase{r.ID, r.AlumnoID, r.AlumnoNombre, r.Estado, r.CreadaEn.UTC(), r.IngresoEn})
			}
			body["reservas"] = reservas
		}
		c.JSON(200, body)
	})
	api.PUT("/classes/:id/capacity", func(c *gin.Context) {
		var in struct {
			Capacidad int `json:"capacidad"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		cl, err := s.CambiarCapacidad(c.Request.Context(), actor(c), c.Param("id"), in.Capacidad)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, claseDTO(cl, ahora()))
	})
	api.POST("/classes/:id/cancel", func(c *gin.Context) {
		var in struct {
			Motivo string `json:"motivo"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		cl, err := s.CancelarClase(c.Request.Context(), actor(c), c.Param("id"), in.Motivo)
		if err != nil {
			responderError(c, err)
			return
		}
		c.JSON(200, claseDTO(cl, ahora()))
	})

	// Reservas (HU-14 a HU-16).
	api.POST("/bookings", func(c *gin.Context) {
		key, ok := clave(c)
		if !ok {
			return
		}
		var in struct {
			ClaseID string `json:"clase_id"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		r, re, err := s.Reservar(c.Request.Context(), actor(c), key, in.ClaseID)
		if err != nil {
			responderError(c, err)
			return
		}
		replay(c, re)
		c.JSON(201, gin.H{"reserva": reservaDTO(r), "clase": claseDTO(r.Clase, ahora())})
	})
	api.GET("/bookings/me", func(c *gin.Context) {
		if !consulta(c, "estado", "pagina", "tamano") {
			return
		}
		p, ok := entero(c, "pagina", 1)
		if !ok {
			return
		}
		t, ok := entero(c, "tamano", 20)
		if !ok {
			return
		}
		page, err := s.MisReservas(c.Request.Context(), actor(c), c.Query("estado"), p, t)
		if err != nil {
			responderError(c, err)
			return
		}
		out := make([]reservaJSON, 0, len(page.Items))
		for _, r := range page.Items {
			out = append(out, reservaDTO(r))
		}
		c.JSON(200, pagina(out, page.Total, page.Pagina, page.Tamano))
	})
	api.POST("/bookings/:id/cancel", func(c *gin.Context) {
		key, ok := clave(c)
		if !ok {
			return
		}
		if c.Request.ContentLength > 0 {
			var in struct{}
			if !decodificar(c, &in) {
				responderError(c, domain.ErrValidacion)
				return
			}
		}
		r, re, err := s.CancelarReserva(c.Request.Context(), actor(c), key, c.Param("id"))
		if err != nil {
			responderError(c, err)
			return
		}
		replay(c, re)
		c.JSON(200, reservaDTO(r))
	})

	// Ingreso por DNI (HU-22, D-36 a D-39, D-82).
	api.POST("/check-ins", func(c *gin.Context) {
		key, ok := clave(c)
		if !ok {
			return
		}
		var in struct {
			DNI string `json:"dni"`
		}
		if !decodificar(c, &in) {
			responderError(c, domain.ErrValidacion)
			return
		}
		ingreso, re, err := s.RegistrarIngreso(c.Request.Context(), actor(c), key, in.DNI)
		if err != nil {
			responderError(c, err)
			return
		}
		replay(c, re)
		status, mensaje := 201, "Bienvenido"
		if ingreso.Reingreso {
			status, mensaje = 200, "Bienvenido de nuevo"
		}
		r := ingreso.Reserva
		c.JSON(status, gin.H{
			"resultado":  "VERDE",
			"mensaje":    mensaje,
			"nombre":     ingreso.Nombre,
			"membresia":  gin.H{"estado": "VIGENTE", "vence": ingreso.Vence},
			"clase":      claseResumen{r.Clase.ID, r.Clase.ActividadNombre, r.Clase.Inicio.UTC(), r.Clase.Fin.UTC()},
			"reserva_id": r.ID,
			"puntos":     map[bool]int{true: 0, false: domain.PuntosAsistencia}[ingreso.Reingreso],
		})
	})
}
