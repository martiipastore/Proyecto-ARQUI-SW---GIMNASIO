// Package status implementa GET /api/v1/status: consulta en paralelo el
// /health/ready de cada servicio interno y devuelve un resumen.
//
// El resumen informa el estado de cada servicio y de cada una de sus
// dependencias, pero no reenvía los mensajes de error internos (que pueden
// incluir hosts o usuarios de base de datos) porque este endpoint es público.
package status

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/health"
)

// Estados de un servicio en el resumen.
const (
	Ready       = "ready"
	NotReady    = "not_ready"
	Unreachable = "unreachable"
	Degraded    = "degraded"
)

// Target es un servicio a consultar.
type Target struct {
	Name    string
	BaseURL string
}

// ServiceStatus es el estado de un servicio.
type ServiceStatus struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	LatencyMS int64             `json:"latency_ms"`
	Checks    map[string]string `json:"checks"`
}

// Summary es la respuesta de GET /api/v1/status.
type Summary struct {
	Status    string          `json:"status"`
	CheckedAt time.Time       `json:"checked_at"`
	Services  []ServiceStatus `json:"services"`
}

// Checker consulta la salud de los servicios.
type Checker struct {
	client  *http.Client
	targets []Target
	timeout time.Duration
	now     func() time.Time
}

// NewChecker crea un Checker. client debe propagar el correlation ID (correlation.Transport).
func NewChecker(client *http.Client, timeout time.Duration, targets ...Target) *Checker {
	return &Checker{client: client, targets: targets, timeout: timeout, now: time.Now}
}

// Check consulta todos los servicios en paralelo.
func (c *Checker) Check(ctx context.Context) Summary {
	results := make([]ServiceStatus, len(c.targets))
	var wg sync.WaitGroup
	for i, t := range c.targets {
		wg.Add(1)
		go func(i int, t Target) {
			defer wg.Done()
			results[i] = c.checkOne(ctx, t)
		}(i, t)
	}
	wg.Wait()

	return Summary{Status: aggregate(results), CheckedAt: c.now().UTC(), Services: results}
}

// aggregate aplica ADR-007: not_ready si algún servicio está not_ready o
// unreachable; si no, degraded si alguno está degraded; si no, ready.
func aggregate(results []ServiceStatus) string {
	overall := Ready
	for _, r := range results {
		switch r.Status {
		case NotReady, Unreachable:
			return NotReady
		case Degraded:
			overall = Degraded
		}
	}
	return overall
}

func (c *Checker) checkOne(ctx context.Context, t Target) ServiceStatus {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	res := ServiceStatus{Name: t.Name, Status: Unreachable, Checks: map[string]string{}}
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(t.BaseURL, "/")+"/health/ready", nil)
	if err != nil {
		return res
	}
	resp, err := c.client.Do(req)
	res.LatencyMS = time.Since(start).Milliseconds()
	if err != nil {
		return res
	}
	defer func() { _ = resp.Body.Close() }()

	var report health.Report
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err == nil {
		_ = json.Unmarshal(body, &report)
	}
	for name, chk := range report.Checks {
		res.Checks[name] = chk.Status
	}

	switch {
	case resp.StatusCode == http.StatusOK && report.Status == health.StatusReady:
		res.Status = Ready
	case resp.StatusCode == http.StatusOK && report.Status == health.StatusDegraded:
		res.Status = Degraded
	case resp.StatusCode == http.StatusServiceUnavailable:
		res.Status = NotReady
	default:
		res.Status = Unreachable
	}
	return res
}
