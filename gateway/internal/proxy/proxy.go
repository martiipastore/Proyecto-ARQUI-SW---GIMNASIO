// Package proxy implementa el routing semántico del gateway: elige el servicio
// destino por prefijo de ruta, aplica el timeout de esa ruta, propaga el
// correlation ID y nunca expone rutas /internal/*.
package proxy

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	gwconfig "github.com/GuadiViti/Arquitectura-de-Software/gateway/internal/config"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/problem"
)

type route struct {
	prefix  string
	service string
	timeout time.Duration
	proxy   *httputil.ReverseProxy
}

// Proxy reenvía solicitudes a los servicios internos.
type Proxy struct {
	routes []route
	log    *slog.Logger
}

// New crea el proxy a partir de la configuración. transport puede ser nil (usa http.DefaultTransport).
func New(cfg gwconfig.Config, transport http.RoundTripper, log *slog.Logger) (*Proxy, error) {
	if transport == nil {
		transport = http.DefaultTransport
	}
	p := &Proxy{log: log}
	for _, r := range cfg.Routes {
		svc, ok := cfg.Service(r.Service)
		if !ok {
			return nil, errors.New("la ruta " + r.Prefix + " apunta a un servicio desconocido: " + r.Service)
		}
		target := svc.URL
		service := svc.Name
		rp := &httputil.ReverseProxy{
			Rewrite: func(pr *httputil.ProxyRequest) {
				pr.SetURL(target)
				pr.SetXForwarded()
				pr.Out.Host = target.Host
				if id := correlation.FromContext(pr.In.Context()); id != "" {
					pr.Out.Header.Set(correlation.Header, id)
				}
			},
			Transport:    transport,
			ErrorHandler: p.errorHandler(service),
		}
		p.routes = append(p.routes, route{prefix: r.Prefix, service: service, timeout: r.Timeout, proxy: rp})
	}
	return p, nil
}

// Handler devuelve el handler de Gin que atiende toda ruta no registrada explícitamente.
func (p *Proxy) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		p.ServeHTTP(c.Writer, c.Request)
		c.Abort()
	}
}

// ServeHTTP implementa http.Handler.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cleaned := cleanPath(r.URL.Path)
	if isInternal(cleaned) {
		problem.WriteHTTP(w, r, problem.NotFound("No existe el recurso "+r.URL.Path+"."))
		return
	}
	rt, ok := p.match(cleaned)
	if !ok {
		problem.WriteHTTP(w, r, problem.NotFound("No existe el recurso "+r.URL.Path+"."))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), rt.timeout)
	defer cancel()
	out := r.WithContext(ctx)
	u := *r.URL
	u.Path = cleaned
	u.RawPath = ""
	out.URL = &u
	rt.proxy.ServeHTTP(w, out)
}

func (p *Proxy) match(cleaned string) (route, bool) {
	for _, rt := range p.routes {
		if cleaned == rt.prefix || strings.HasPrefix(cleaned, rt.prefix+"/") {
			return rt, true
		}
	}
	return route{}, false
}

func (p *Proxy) errorHandler(service string) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			p.log.WarnContext(r.Context(), "timeout esperando al servicio",
				slog.String("upstream", service), slog.String("path", r.URL.Path))
			problem.WriteHTTP(w, r, problem.New(http.StatusGatewayTimeout, problem.CodeTimeout,
				"Tiempo de espera agotado", "El servicio "+service+" no respondió a tiempo."))
			return
		}
		if errors.Is(err, context.Canceled) {
			// El cliente cortó la conexión: no hay a quién responder.
			return
		}
		p.log.ErrorContext(r.Context(), "servicio no disponible",
			slog.String("upstream", service), slog.String("path", r.URL.Path), slog.Any("error", err))
		problem.WriteHTTP(w, r, problem.New(http.StatusBadGateway, problem.CodeDependencyUnavailable,
			"Servicio no disponible", "El servicio "+service+" no está disponible."))
	}
}

// cleanPath normaliza la ruta (resuelve "..", "//") para que el routing y lo que
// se reenvía sean exactamente lo mismo.
func cleanPath(p string) string {
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return path.Clean(p)
}

// isInternal indica si algún segmento de la ruta es "internal".
func isInternal(cleaned string) bool {
	for _, seg := range strings.Split(cleaned, "/") {
		if strings.EqualFold(seg, "internal") {
			return true
		}
	}
	return false
}
