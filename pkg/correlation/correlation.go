// Package correlation maneja el correlation ID de cada solicitud: lo lee del
// header X-Correlation-ID (o lo genera si falta o es inválido), lo guarda en el
// context, lo devuelve en la respuesta y lo propaga en las llamadas salientes.
package correlation

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"regexp"

	"github.com/gin-gonic/gin"
)

// Header es el nombre del header HTTP que transporta el correlation ID.
const Header = "X-Correlation-ID"

type ctxKey struct{}

var validID = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

// Valid indica si id puede usarse como correlation ID (8 a 128 caracteres seguros).
func Valid(id string) bool {
	return validID.MatchString(id)
}

// NewID genera un UUID v4.
func NewID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("correlation: no se pudo generar un ID aleatorio: %v", err))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// NewContext devuelve un context con el correlation ID.
func NewContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext devuelve el correlation ID del context, o "" si no hay.
func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// FromRequest devuelve el ID del header si es válido; si no, genera uno nuevo.
func FromRequest(r *http.Request) string {
	if id := r.Header.Get(Header); Valid(id) {
		return id
	}
	return NewID()
}

// Middleware asegura que toda solicitud tenga un correlation ID: lo deja en el
// context, en el header de la solicitud (para que un proxy lo reenvíe) y en la respuesta.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := FromRequest(c.Request)
		c.Request.Header.Set(Header, id)
		c.Request = c.Request.WithContext(NewContext(c.Request.Context(), id))
		c.Header(Header, id)
		c.Next()
	}
}

// Transport propaga el correlation ID del context de la solicitud saliente.
type Transport struct {
	Base http.RoundTripper
}

// RoundTrip implementa http.RoundTripper.
func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	if id := FromContext(r.Context()); id != "" && r.Header.Get(Header) == "" {
		r = r.Clone(r.Context())
		r.Header.Set(Header, id)
	}
	return base.RoundTrip(r)
}
