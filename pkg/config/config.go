// Package config lee la configuración de los servicios desde variables de entorno.
//
// Uso típico: se crea un Loader, se leen todas las variables y al final se
// consulta Err(), que reúne todos los problemas encontrados (variables
// obligatorias ausentes o con formato inválido) para informarlos juntos.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Loader acumula los errores de lectura de variables de entorno.
type Loader struct {
	lookup func(string) (string, bool)
	errs   []error
}

// New crea un Loader que lee del entorno del proceso.
func New() *Loader {
	return &Loader{lookup: os.LookupEnv}
}

// NewFromMap crea un Loader que lee de un mapa (útil en tests).
func NewFromMap(values map[string]string) *Loader {
	return &Loader{lookup: func(k string) (string, bool) {
		v, ok := values[k]
		return v, ok
	}}
}

func (l *Loader) get(key string) (string, bool) {
	v, ok := l.lookup(key)
	if !ok {
		return "", false
	}
	v = strings.TrimSpace(v)
	return v, v != ""
}

// String devuelve el valor de key o def si no está definida.
func (l *Loader) String(key, def string) string {
	if v, ok := l.get(key); ok {
		return v
	}
	return def
}

// Required devuelve el valor de key y registra un error si no está definida.
func (l *Loader) Required(key string) string {
	v, ok := l.get(key)
	if !ok {
		l.errs = append(l.errs, fmt.Errorf("la variable de entorno %s es obligatoria", key))
	}
	return v
}

// Int devuelve key como entero o def si no está definida.
func (l *Loader) Int(key string, def int) int {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.errs = append(l.errs, fmt.Errorf("la variable de entorno %s debe ser un entero: %q", key, v))
		return def
	}
	return n
}

// Duration devuelve key como duración (ej. "3s", "500ms") o def si no está definida.
func (l *Loader) Duration(key string, def time.Duration) time.Duration {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		l.errs = append(l.errs, fmt.Errorf("la variable de entorno %s debe ser una duración positiva (ej. 3s): %q", key, v))
		return def
	}
	return d
}

// List devuelve key separada por comas, sin elementos vacíos, o def si no está definida.
func (l *Loader) List(key string, def []string) []string {
	v, ok := l.get(key)
	if !ok {
		return def
	}
	var out []string
	for _, item := range strings.Split(v, ",") {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

// Err devuelve todos los errores acumulados, o nil.
func (l *Loader) Err() error {
	return errors.Join(l.errs...)
}
