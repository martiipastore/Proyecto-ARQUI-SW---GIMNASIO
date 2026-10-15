package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
)

func TestNew_EscribeJSONConServicioYCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, "members-service", "info")

	ctx := correlation.NewContext(context.Background(), "corr-12345678")
	log.InfoContext(ctx, "servicio iniciado", slog.String("addr", ":8081"))

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	assert.Equal(t, "members-service", line["service"])
	assert.Equal(t, "corr-12345678", line["correlation_id"])
	assert.Equal(t, "servicio iniciado", line["msg"])
	assert.Equal(t, "INFO", line["level"])
}

func TestNew_SinCorrelationIDNoAgregaElCampo(t *testing.T) {
	var buf bytes.Buffer
	NewWithWriter(&buf, "svc", "info").Info("hola")

	var line map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &line))
	_, ok := line["correlation_id"]
	assert.False(t, ok)
}

func TestNew_RespetaElNivel(t *testing.T) {
	var buf bytes.Buffer
	log := NewWithWriter(&buf, "svc", "warn")
	log.Info("no debe salir")
	assert.Empty(t, buf.String())
	log.Warn("sí debe salir")
	assert.NotEmpty(t, buf.String())
}

func TestParseLevel(t *testing.T) {
	assert.Equal(t, slog.LevelDebug, ParseLevel("DEBUG"))
	assert.Equal(t, slog.LevelError, ParseLevel("error"))
	assert.Equal(t, slog.LevelInfo, ParseLevel("cualquiera"))
}
