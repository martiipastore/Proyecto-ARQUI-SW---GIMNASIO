// Package telemetry contiene instrumentacion tecnica, sin reglas de negocio.
package telemetry

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// Setup habilita OTLP cuando se configura el endpoint; sin endpoint no exporta.
// Solo se propaga TraceContext: nunca baggage con datos provistos por clientes.
func Setup(ctx context.Context, service string, log *slog.Logger) (func(context.Context) error, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return func(context.Context) error { return nil }, nil
	}
	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", service))),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(error) {
		// El error crudo puede contener URLs o datos del exportador.
		log.Warn("no se pudieron exportar trazas", slog.String("component", "otlp"))
	}))
	return provider.Shutdown, nil
}

// Route evita URLs concretas, query strings e identificadores en los spans.
func Route(path string) string {
	prefix := ""
	if strings.HasPrefix(path, "/partner-api/") {
		prefix = "/partner-api"
		path = strings.TrimPrefix(path, prefix)
	}
	parts := strings.Split(path, "/")
	if len(parts) == 5 && parts[1] == "v1" && parts[2] == "accounts" {
		switch parts[4] {
		case "credits", "balance", "movements":
			return prefix + "/v1/accounts/{externalUserId}/" + parts[4]
		}
	}
	return "unmatched"
}

func method(value string) string {
	switch value {
	case "GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS":
		return value
	default:
		return "OTHER"
	}
}

// Middleware instrumenta solo el flujo minimo; los health checks no generan spans.
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		route := Route(c.Request.URL.Path)
		if route == "unmatched" {
			c.Next()
			return
		}
		parent := propagation.TraceContext{}.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		remote := trace.SpanContextFromContext(parent).WithTraceState(trace.TraceState{})
		parent = trace.ContextWithRemoteSpanContext(parent, remote)
		ctx, span := otel.Tracer("gym/http").Start(parent, method(c.Request.Method)+" "+route, trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.route", route), attribute.String("http.request.method", method(c.Request.Method))))
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
		span.SetAttributes(attribute.Int("http.response.status_code", c.Writer.Status()))
		if c.Writer.Status() >= 500 {
			span.SetStatus(codes.Error, "error HTTP")
		}
	}
}

// Transport crea el span cliente y propaga su contexto sin capturar headers/URL.
type Transport struct{ Base http.RoundTripper }

func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	route := Route(r.URL.Path)
	if route == "unmatched" {
		return base.RoundTrip(r)
	}
	ctx, span := otel.Tracer("gym/http").Start(r.Context(), method(r.Method)+" "+route, trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("http.route", route), attribute.String("http.request.method", method(r.Method))))
	defer span.End()
	out := r.Clone(ctx)
	// Evitar reenviar metadata arbitraria del cliente; solo nuestro traceparent.
	out.Header.Del("tracestate")
	out.Header.Del("baggage")
	out.Header.Del("traceparent")
	propagation.TraceContext{}.Inject(ctx, propagation.HeaderCarrier(out.Header))
	response, err := base.RoundTrip(out)
	if err != nil {
		span.SetStatus(codes.Error, "fallo de transporte")
	} else {
		span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
		if response.StatusCode >= 500 {
			span.SetStatus(codes.Error, "error HTTP")
		}
	}
	return response, err
}
