package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/httpx"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/logger"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/telemetry"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// D11: contexto comun entre HTTP entrante, cliente, servicio y logs sin secretos.
func TestTracePropagationAndSanitizedLogs(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := trace.NewTracerProvider(trace.WithSyncer(exporter), trace.WithSampler(trace.AlwaysSample()))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() {
		otel.SetTracerProvider(previous)
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	var gatewayLogs, benefitsLogs bytes.Buffer
	service := httpx.NewRouter(logger.NewWithWriter(&benefitsLogs, "benefits-service", "info"))
	service.GET("/v1/accounts/:externalUserId/balance", func(c *gin.Context) {
		_, span := otel.Tracer("test/postgres").Start(c.Request.Context(), "postgres.transaccion")
		span.End()
		c.Status(200)
	})
	upstream := httptest.NewServer(service)
	defer upstream.Close()
	client := &http.Client{Transport: telemetry.Transport{}}
	gateway := httpx.NewRouter(logger.NewWithWriter(&gatewayLogs, "api-gateway", "info"))
	gateway.GET("/partner-api/v1/accounts/:externalUserId/balance", func(c *gin.Context) {
		req, err := http.NewRequestWithContext(c.Request.Context(), "GET", upstream.URL+"/v1/accounts/private-user/balance?secret=query-secret", nil)
		require.NoError(t, err)
		req.Header.Set("X-Correlation-ID", c.GetHeader("X-Correlation-ID"))
		req.Header.Set("X-API-Key", "secret-api-key")
		req.Header.Set("baggage", "password=secret-baggage")
		req.Header.Set("tracestate", "vendor=secret-state")
		resp, err := client.Do(req)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		c.Status(resp.StatusCode)
	})
	req := httptest.NewRequest("GET", "/partner-api/v1/accounts/private-user/balance", nil)
	req.Header.Set("X-Correlation-ID", "a286172f-7cbd-41e1-bfa0-6042f834611c")
	req.Header.Set("traceparent", "00-0123456789abcdef0123456789abcdef-0123456789abcdef-01")
	req.Header.Set("tracestate", "vendor=secret-state")
	w := httptest.NewRecorder()
	gateway.ServeHTTP(w, req)
	require.Equal(t, 200, w.Code)
	spans := exporter.GetSpans()
	require.Len(t, spans, 4)
	for _, span := range spans {
		require.Equal(t, "0123456789abcdef0123456789abcdef", span.SpanContext.TraceID().String())
		require.Empty(t, span.SpanContext.TraceState().String())
		require.True(t, span.Parent.IsValid())
	}
	// Orden de cierre: persistencia, servidor Benefits, cliente, servidor Gateway.
	require.Equal(t, spans[1].SpanContext.SpanID(), spans[0].Parent.SpanID())
	require.Equal(t, spans[2].SpanContext.SpanID(), spans[1].Parent.SpanID())
	require.Equal(t, spans[3].SpanContext.SpanID(), spans[2].Parent.SpanID())
	encoded, err := json.Marshal(spans)
	require.NoError(t, err)
	all := string(encoded) + gatewayLogs.String() + benefitsLogs.String()
	for _, secret := range []string{"private-user", "secret-api-key", "query-secret", "secret-baggage", "secret-state"} {
		require.NotContains(t, all, secret)
	}
	for _, output := range []string{gatewayLogs.String(), benefitsLogs.String()} {
		var line map[string]any
		require.NoError(t, json.Unmarshal([]byte(output), &line))
		require.Equal(t, "0123456789abcdef0123456789abcdef", line["trace_id"])
		require.Equal(t, "a286172f-7cbd-41e1-bfa0-6042f834611c", line["correlation_id"])
		require.NotEmpty(t, line["span_id"])
	}
}

func TestSetupDisabledAndRoute(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	shutdown, err := telemetry.Setup(context.Background(), "test", logger.NewWithWriter(io.Discard, "test", "info"))
	require.NoError(t, err)
	require.NoError(t, shutdown(context.Background()))
	for _, path := range []string{"/health/ready", "/v1/accounts/private-user/unknown", "/private-user"} {
		require.Equal(t, "unmatched", telemetry.Route(path))
	}
	require.Equal(t, "/v1/accounts/{externalUserId}/credits", telemetry.Route("/v1/accounts/secret/credits"))
	require.False(t, strings.Contains(telemetry.Route("/partner-api/v1/accounts/secret/movements"), "secret"))
}
