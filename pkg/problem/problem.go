// Package problem implementa errores HTTP en formato RFC 7807
// (application/problem+json), con el campo "code" del catálogo de
// docs/ARCHITECTURE.md §12.1.
package problem

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
)

// ContentType es el media type de RFC 7807.
const ContentType = "application/problem+json"

// TypeBase es el prefijo de las URIs de tipo de problema.
const TypeBase = "https://gym.local/problems/"

// Códigos del catálogo usados por la infraestructura común.
const (
	CodeValidation            = "VALIDACION"
	CodeNotFound              = "NO_ENCONTRADO"
	CodeMethodNotAllowed      = "METODO_NO_PERMITIDO"
	CodeDependencyUnavailable = "DEPENDENCIA_NO_DISPONIBLE"
	CodeTimeout               = "TIMEOUT"
	CodeInternal              = "ERROR_INTERNO"
)

// FieldError describe un error de validación de un campo.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Problem es un error RFC 7807. Extensions agrega miembros extra al JSON.
type Problem struct {
	Type          string
	Title         string
	Status        int
	Detail        string
	Instance      string
	Code          string
	CorrelationID string
	Errors        []FieldError
	Extensions    map[string]any
}

// New crea un Problem con el type derivado del code (ej. SIN_CUPO → .../sin-cupo).
func New(status int, code, title, detail string) Problem {
	return Problem{
		Type:   TypeBase + strings.ReplaceAll(strings.ToLower(code), "_", "-"),
		Title:  title,
		Status: status,
		Detail: detail,
		Code:   code,
	}
}

// NotFound crea un 404 NO_ENCONTRADO.
func NotFound(detail string) Problem {
	return New(http.StatusNotFound, CodeNotFound, "Recurso no encontrado", detail)
}

// Internal crea un 500 ERROR_INTERNO sin exponer detalles.
func Internal() Problem {
	return New(http.StatusInternalServerError, CodeInternal, "Error interno",
		"Ocurrió un error inesperado. Informá el correlation_id si persiste.")
}

// MarshalJSON serializa los miembros estándar más las extensiones.
func (p Problem) MarshalJSON() ([]byte, error) {
	m := make(map[string]any, 8+len(p.Extensions))
	for k, v := range p.Extensions {
		m[k] = v
	}
	m["type"] = p.Type
	m["title"] = p.Title
	m["status"] = p.Status
	m["code"] = p.Code
	if p.Detail != "" {
		m["detail"] = p.Detail
	}
	if p.Instance != "" {
		m["instance"] = p.Instance
	}
	if p.CorrelationID != "" {
		m["correlation_id"] = p.CorrelationID
	}
	if len(p.Errors) > 0 {
		m["errors"] = p.Errors
	}
	return json.Marshal(m)
}

// WriteHTTP escribe p en w completando instance y correlation_id desde r.
func WriteHTTP(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.Instance == "" {
		p.Instance = r.URL.Path
	}
	if p.CorrelationID == "" {
		p.CorrelationID = correlation.FromContext(r.Context())
	}
	body, err := json.Marshal(p)
	if err != nil {
		body = []byte(`{"type":"` + TypeBase + `error-interno","title":"Error interno","status":500,"code":"` + CodeInternal + `"}`)
		p.Status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", ContentType)
	w.WriteHeader(p.Status)
	_, _ = w.Write(body)
}

// Write escribe p como respuesta de Gin y corta la cadena de handlers.
func Write(c *gin.Context, p Problem) {
	WriteHTTP(c.Writer, c.Request, p)
	c.Abort()
}
