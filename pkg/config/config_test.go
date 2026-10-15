package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoader_ValoresPorDefectoYDefinidos(t *testing.T) {
	l := NewFromMap(map[string]string{
		"ADDR":    ":9000",
		"TIMEOUT": "5s",
		"WORKERS": "4",
		"ORIGINS": "http://a, ,http://b",
		"BLANCO":  "   ",
	})

	assert.Equal(t, ":9000", l.String("ADDR", ":8080"))
	assert.Equal(t, "def", l.String("NO_EXISTE", "def"))
	assert.Equal(t, "def", l.String("BLANCO", "def"), "un valor en blanco cuenta como no definido")
	assert.Equal(t, 5*time.Second, l.Duration("TIMEOUT", time.Second))
	assert.Equal(t, time.Second, l.Duration("NO_EXISTE", time.Second))
	assert.Equal(t, 4, l.Int("WORKERS", 1))
	assert.Equal(t, []string{"http://a", "http://b"}, l.List("ORIGINS", nil))
	require.NoError(t, l.Err())
}

func TestLoader_AcumulaErrores(t *testing.T) {
	l := NewFromMap(map[string]string{
		"TIMEOUT": "rapido",
		"WORKERS": "muchos",
	})

	l.Required("DB_DSN")
	l.Duration("TIMEOUT", time.Second)
	l.Int("WORKERS", 1)

	err := l.Err()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DB_DSN")
	assert.Contains(t, err.Error(), "TIMEOUT")
	assert.Contains(t, err.Error(), "WORKERS")
}

func TestLoader_DuracionNoPositivaEsInvalida(t *testing.T) {
	l := NewFromMap(map[string]string{"TIMEOUT": "0s"})
	assert.Equal(t, time.Second, l.Duration("TIMEOUT", time.Second))
	assert.Error(t, l.Err())
}
