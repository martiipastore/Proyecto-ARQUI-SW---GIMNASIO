package server

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/status"
)

// requestMetrics cuenta las requests que atraviesan el gateway por clase de
// respuesta (2xx, 3xx, 4xx, 5xx) y acumula su duración: tasa, errores y
// latencia media del método RED (etapa 14). No etiqueta por ruta ni por usuario
// para no crear series sin límite ni exponer datos personales.
type requestMetrics struct {
	count  [6]atomic.Int64 // índice = status / 100
	micros [6]atomic.Int64
}

func (m *requestMetrics) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if path == "/metrics" || strings.HasPrefix(path, "/health/") {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		class := c.Writer.Status() / 100
		if class < 1 || class > 5 {
			return
		}
		m.count[class].Add(1)
		m.micros[class].Add(time.Since(start).Microseconds())
	}
}

func (m *requestMetrics) text() string {
	var b strings.Builder
	b.WriteString("# TYPE gateway_http_requests_total counter\n")
	for class := 2; class <= 5; class++ {
		fmt.Fprintf(&b, "gateway_http_requests_total{code=\"%dxx\"} %d\n", class, m.count[class].Load())
	}
	b.WriteString("# TYPE gateway_http_request_seconds_sum counter\n")
	for class := 2; class <= 5; class++ {
		fmt.Fprintf(&b, "gateway_http_request_seconds_sum{code=\"%dxx\"} %.6f\n", class, float64(m.micros[class].Load())/1e6)
	}
	return b.String()
}

// servicesText expone el resultado de /api/v1/status como gauges: 1 si el
// servicio responde ready o degraded, 0 si no; y 1 si está degradado.
func servicesText(s status.Summary) string {
	var b strings.Builder
	b.WriteString("# TYPE gateway_upstream_up gauge\n")
	for _, svc := range s.Services {
		up := 0
		if svc.Status == status.Ready || svc.Status == status.Degraded {
			up = 1
		}
		fmt.Fprintf(&b, "gateway_upstream_up{service=%q} %d\n", svc.Name, up)
	}
	b.WriteString("# TYPE gateway_upstream_degraded gauge\n")
	for _, svc := range s.Services {
		d := 0
		if svc.Status == status.Degraded {
			d = 1
		}
		fmt.Fprintf(&b, "gateway_upstream_degraded{service=%q} %d\n", svc.Name, d)
	}
	b.WriteString("# TYPE gateway_upstream_latency_seconds gauge\n")
	for _, svc := range s.Services {
		fmt.Fprintf(&b, "gateway_upstream_latency_seconds{service=%q} %.3f\n", svc.Name, float64(svc.LatencyMS)/1000)
	}
	return b.String()
}
