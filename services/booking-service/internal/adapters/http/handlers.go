package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// ---- Errores (booking.v1, ARCHITECTURE §12.1) ----

type mapeo struct {
	err    error
	status int
	code   string
	detail string
}

var errores = []mapeo{
	{domain.ErrValidacion, 400, problem.CodeValidation, "Revisá los datos enviados."},
	{domain.ErrSinPermiso, 403, "SIN_PERMISO", "Tu rol no permite esta operación sobre el recurso."},
	{domain.ErrNoEncontrado, 404, problem.CodeNotFound, "No existe el recurso solicitado."},
	{domain.ErrSinCupo, 409, "SIN_CUPO", "La clase no tiene lugares disponibles."},
	{domain.ErrReservaDuplicada, 409, "RESERVA_DUPLICADA", "Ya tenés una reserva activa en esta clase."},
	{domain.ErrTurnoMusculacionDiario, 409, "TURNO_MUSCULACION_DIARIO", "Ya tenés un turno de Musculación ese día."},
	{domain.ErrSuperposicionReserva, 409, "SUPERPOSICION_RESERVA", "Tenés otra reserva que se superpone en horario."},
	{domain.ErrSuperposicionProfesor, 409, "SUPERPOSICION_PROFESOR", "El profesor ya es responsable de una clase superpuesta."},
	{domain.ErrTransicionInvalida, 409, "TRANSICION_INVALIDA", "El estado actual no permite esta operación."},
	{domain.ErrConflictoIdempotencia, 409, "CONFLICTO_IDEMPOTENCIA", "La clave ya identifica otra operación."},
	{domain.ErrOperacionEnCurso, 409, "OPERACION_EN_CURSO", "Reintentá con la misma clave en unos segundos."},
	{domain.ErrClaseNoFutura, 422, "CLASE_NO_FUTURA", "La clase ya comenzó."},
	{domain.ErrClaseCancelada, 422, "CLASE_CANCELADA", "La clase fue cancelada."},
	{domain.ErrPlazoCancelacionVencido, 422, "PLAZO_CANCELACION_VENCIDO", "Solo se puede cancelar hasta 1 hora antes del inicio."},
	{domain.ErrMembresiaNoVigente, 422, "MEMBRESIA_NO_VIGENTE", "No tenés una membresía vigente para esa fecha."},
	{domain.ErrUsuarioInactivo, 422, "USUARIO_INACTIVO", "La persona está dada de baja."},
	{domain.ErrSinReservaParaHorario, 422, "SIN_RESERVA_PARA_HORARIO", "No hay una reserva para este horario."},
}

func responderError(c *gin.Context, err error) {
	status, code, detail := 503, problem.CodeDependencyUnavailable, "No se pudo completar la operación. Reintentá en unos segundos."
	if !errors.Is(err, ports.ErrNoDisponible) {
		for _, m := range errores {
			if errors.Is(err, m.err) {
				status, code, detail = m.status, m.code, m.detail
				break
			}
		}
	}
	p := problem.New(status, code, "No se pudo completar la solicitud", detail)
	var rechazo application.RechazoIngreso
	if errors.As(err, &rechazo) {
		// D-36: la recepción muestra el nombre en rojo con el motivo.
		p.Extensions = map[string]any{"resultado": "ROJO", "nombre": rechazo.Nombre}
	}
	if status == 503 {
		c.Header("Retry-After", "1")
	}
	problem.Write(c, p)
}

// ---- Identidad y entrada ----

func actor(c *gin.Context) application.Actor {
	return application.Actor{ID: c.GetHeader("X-User-Id"), Rol: c.GetHeader("X-User-Role")}
}

// confiable exige la clave interna que inyecta el gateway junto con la identidad
// verificada (ADR-006, CLAUDE.md regla 10).
func confiable(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		if len(secret) < 32 || len(c.Request.Header.Values("X-Internal-Service-Key")) != 1 ||
			subtle.ConstantTimeCompare([]byte(secret), []byte(c.GetHeader("X-Internal-Service-Key"))) != 1 ||
			len(c.Request.Header.Values("X-User-Id")) != 1 || !access.ValidRole(c.GetHeader("X-User-Role")) {
			problem.Write(c, problem.New(401, "NO_AUTENTICADO", "No autorizado", "Ingresá para acceder."))
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 4*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func decodificar(c *gin.Context, out any) bool {
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

func clave(c *gin.Context) (string, bool) {
	if len(c.Request.Header.Values("Idempotency-Key")) != 1 {
		responderError(c, domain.ErrValidacion)
		return "", false
	}
	return c.GetHeader("Idempotency-Key"), true
}

// consulta valida que solo vengan parámetros conocidos y sin repetir.
func consulta(c *gin.Context, permitidos ...string) bool {
	for key, values := range c.Request.URL.Query() {
		ok := len(values) == 1
		if ok {
			ok = false
			for _, p := range permitidos {
				ok = ok || p == key
			}
		}
		if !ok {
			responderError(c, domain.ErrValidacion)
			return false
		}
	}
	return true
}

func entero(c *gin.Context, nombre string, defecto int) (int, bool) {
	raw := c.Query(nombre)
	if raw == "" {
		return defecto, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		responderError(c, domain.ErrValidacion)
		return 0, false
	}
	return n, true
}

// ---- Representaciones (snake_case, booking.v1) ----

type ref struct {
	ID     string `json:"id"`
	Nombre string `json:"nombre"`
}

type claseJSON struct {
	ID             string    `json:"id"`
	Actividad      ref       `json:"actividad"`
	Inicio         time.Time `json:"inicio"`
	Fin            time.Time `json:"fin"`
	Franja         *string   `json:"franja"`
	Profesor       ref       `json:"profesor"`
	Capacidad      int       `json:"capacidad"`
	Ocupacion      int       `json:"ocupacion"`
	Disponibles    int       `json:"disponibles"`
	Estado         string    `json:"estado"`
	Reservable     bool      `json:"reservable"`
	ReservadaPorMi bool      `json:"reservada_por_mi"`
}

func claseDTO(c domain.Clase, now time.Time) claseJSON {
	return claseJSON{
		ID: c.ID, Actividad: ref{c.ActividadID, c.ActividadNombre}, Inicio: c.Inicio.UTC(), Fin: c.Fin.UTC(), Franja: c.Franja(),
		Profesor: ref{c.ProfesorID, c.ProfesorNombre}, Capacidad: c.Capacidad, Ocupacion: c.Ocupacion, Disponibles: c.Disponibles(),
		Estado: c.EstadoEn(now), Reservable: c.Reservable(now), ReservadaPorMi: c.ReservadaPorMi,
	}
}

type claseResumen struct {
	ID        string    `json:"id"`
	Actividad string    `json:"actividad"`
	Inicio    time.Time `json:"inicio"`
	Fin       time.Time `json:"fin"`
}

type reservaJSON struct {
	ID                string       `json:"id"`
	Clase             claseResumen `json:"clase"`
	Estado            string       `json:"estado"`
	CreadaEn          time.Time    `json:"creada_en"`
	IngresoEn         *time.Time   `json:"ingreso_en"`
	CanceladaEn       *time.Time   `json:"cancelada_en"`
	MotivoCancelacion *string      `json:"motivo_cancelacion"`
}

func reservaDTO(r domain.Reserva) reservaJSON {
	return reservaJSON{
		ID: r.ID, Clase: claseResumen{r.Clase.ID, r.Clase.ActividadNombre, r.Clase.Inicio.UTC(), r.Clase.Fin.UTC()},
		Estado: r.Estado, CreadaEn: r.CreadaEn.UTC(), IngresoEn: r.IngresoEn, CanceladaEn: r.CanceladaEn, MotivoCancelacion: r.MotivoCancelacion,
	}
}

type actividadJSON struct {
	ID               string `json:"id"`
	Nombre           string `json:"nombre"`
	CapacidadMaxima  int    `json:"capacidad_maxima"`
	Musculacion      bool   `json:"musculacion"`
	Activa           bool   `json:"activa"`
	PuntosAsistencia int    `json:"puntos_asistencia"`
}

func actividadDTO(a domain.Actividad) actividadJSON {
	return actividadJSON{a.ID, a.Nombre, a.CapacidadMaxima, a.Musculacion, a.Activa, domain.PuntosAsistencia}
}

type horarioJSON struct {
	ID              string `json:"id"`
	ActividadID     string `json:"actividad_id"`
	DiaSemana       int    `json:"dia_semana"`
	Inicio          string `json:"inicio"`
	Fin             string `json:"fin"`
	DuracionMinutos int    `json:"duracion_minutos"`
	Profesor        ref    `json:"profesor"`
	Capacidad       int    `json:"capacidad"`
}

func horarioDTO(h domain.Horario) horarioJSON {
	return horarioJSON{h.ID, h.ActividadID, h.DiaSemana, domain.FormatHora(h.InicioMinutos), domain.FormatHora(h.FinMinutos()), h.DuracionMinutos, ref{h.ProfesorID, h.ProfesorNombre}, h.Capacidad}
}

func pagina[T any](items []T, total, pagina, tamano int) gin.H {
	return gin.H{"items": items, "total": total, "pagina": pagina, "tamano": tamano}
}

func replay(c *gin.Context, r bool) {
	c.Header("Idempotent-Replayed", strconv.FormatBool(r))
}
