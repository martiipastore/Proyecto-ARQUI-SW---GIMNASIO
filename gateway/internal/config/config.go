// Package config carga la configuración del api-gateway: servicios destino,
// tabla de rutas por prefijo con su timeout, CORS y timeouts generales.
package config

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/config"
)

// Nombres de los servicios internos.
const (
	Members      = "members-service"
	Booking      = "booking-service"
	BookingIndex = "booking-indexer"
	Benefits     = "benefits-service"
	Training     = "training-service"
	Notification = "notification-worker"
)

// Service es un servicio interno al que el gateway enruta o del que consulta la salud.
type Service struct {
	Name string
	URL  *url.URL
}

// Route asocia un prefijo público con un servicio y un timeout.
type Route struct {
	Prefix  string
	Service string
	Timeout time.Duration
}

// Config del gateway.
type Config struct {
	HTTPAddr        string
	LogLevel        string
	ShutdownTimeout time.Duration
	StatusTimeout   time.Duration
	CORSOrigins     []string
	// Services en el orden en que aparecen en /api/v1/status.
	Services []Service
	// Routes ordenadas del prefijo más largo al más corto.
	Routes []Route
}

// serviceDefs: nombre, variable de entorno con la URL base y valor por defecto (red de Docker Compose).
var serviceDefs = []struct{ name, env, def string }{
	{Members, "GATEWAY_MEMBERS_URL", "http://members-service:8081"},
	{Booking, "GATEWAY_BOOKING_URL", "http://booking-service:8082"},
	{BookingIndex, "GATEWAY_BOOKING_INDEXER_URL", "http://booking-indexer:8086"},
	{Benefits, "GATEWAY_BENEFITS_URL", "http://benefits-service:8083"},
	{Training, "GATEWAY_TRAINING_URL", "http://training-service:8084"},
	{Notification, "GATEWAY_NOTIFICATION_URL", "http://notification-worker:8085"},
}

// routeDefs es la tabla de routing semántico (docs/ARCHITECTURE.md §5.2).
// key define la variable GATEWAY_TIMEOUT_<key> para su timeout.
var routeDefs = []struct {
	prefix, service, key string
	timeout              time.Duration // 0 = GATEWAY_TIMEOUT_DEFAULT
}{
	{"/api/v1/auth", Members, "AUTH", 0},
	{"/api/v1/users", Members, "USERS", 0},
	{"/api/v1/memberships", Members, "MEMBERSHIPS", 0},
	{"/api/v1/activities", Booking, "ACTIVITIES", 0},
	{"/api/v1/classes", Booking, "CLASSES", 0},
	{"/api/v1/bookings", Booking, "BOOKINGS", 5 * time.Second},
	{"/api/v1/check-ins", Booking, "CHECK_INS", 5 * time.Second},
	{"/api/v1/benefits", Benefits, "BENEFITS", 5 * time.Second},
	{"/api/v1/training", Training, "TRAINING", 0},
	{"/api/v1/nutrition", Training, "NUTRITION", 0},
	{"/partner-api/v1", Benefits, "PARTNER_API", 5 * time.Second},
}

// Load lee la configuración del entorno.
func Load() (Config, error) {
	return load(config.New())
}

// LoadFromMap lee la configuración de un mapa (para tests).
func LoadFromMap(values map[string]string) (Config, error) {
	return load(config.NewFromMap(values))
}

func load(l *config.Loader) (Config, error) {
	cfg := Config{
		HTTPAddr:        l.String("GATEWAY_HTTP_ADDR", ":8080"),
		LogLevel:        l.String("LOG_LEVEL", "info"),
		ShutdownTimeout: l.Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		StatusTimeout:   l.Duration("GATEWAY_STATUS_TIMEOUT", 2*time.Second),
		CORSOrigins:     l.List("GATEWAY_CORS_ORIGINS", []string{"http://localhost:5173"}),
	}

	var errs []error
	known := map[string]bool{}
	for _, s := range serviceDefs {
		raw := l.String(s.env, s.def)
		u, err := url.Parse(raw)
		if err != nil || u.Scheme == "" || u.Host == "" {
			errs = append(errs, fmt.Errorf("%s debe ser una URL absoluta (ej. http://host:puerto): %q", s.env, raw))
			continue
		}
		cfg.Services = append(cfg.Services, Service{Name: s.name, URL: u})
		known[s.name] = true
	}

	defaultTimeout := l.Duration("GATEWAY_TIMEOUT_DEFAULT", 3*time.Second)
	for _, r := range routeDefs {
		def := r.timeout
		if def == 0 {
			def = defaultTimeout
		}
		if !known[r.service] {
			continue
		}
		cfg.Routes = append(cfg.Routes, Route{
			Prefix:  r.prefix,
			Service: r.service,
			Timeout: l.Duration("GATEWAY_TIMEOUT_"+r.key, def),
		})
	}
	sort.SliceStable(cfg.Routes, func(i, j int) bool {
		return len(cfg.Routes[i].Prefix) > len(cfg.Routes[j].Prefix)
	})

	if err := l.Err(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return cfg, fmt.Errorf("configuración del gateway inválida: %s", strings.Join(msgs, "; "))
	}
	return cfg, nil
}

// Service devuelve el servicio con ese nombre.
func (c Config) Service(name string) (Service, bool) {
	for _, s := range c.Services {
		if s.Name == name {
			return s, true
		}
	}
	return Service{}, false
}
