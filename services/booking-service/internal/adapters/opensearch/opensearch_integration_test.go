// Etapa 8 contra OpenSearch 2 real (testcontainers): versionado externo, filtros
// de HU-13 con orden estable (ADR-014), profundidad y reindexado con alias.
package opensearch

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

func nuevoOpenSearch(t *testing.T) *Client {
	t.Helper()
	if exec.Command("docker", "info").Run() != nil {
		t.Skip("Docker no disponible")
	}
	ctx := context.Background()
	ctr, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{Started: true, ContainerRequest: testcontainers.ContainerRequest{
		Image:        "opensearchproject/opensearch:2.17.1",
		ExposedPorts: []string{"9200/tcp"},
		Env: map[string]string{"discovery.type": "single-node", "DISABLE_SECURITY_PLUGIN": "true", "DISABLE_INSTALL_DEMO_CONFIG": "true",
			"OPENSEARCH_JAVA_OPTS": "-Xms512m -Xmx512m", "action.auto_create_index": "false"},
		WaitingFor: wait.ForHTTP("/_cluster/health").WithPort("9200/tcp").WithStartupTimeout(3 * time.Minute),
	}})
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)
	endpoint, err := ctr.PortEndpoint(ctx, "9200/tcp", "http")
	require.NoError(t, err)
	return &Client{URL: endpoint, HTTP: &http.Client{Timeout: 10 * time.Second}}
}

func uuidN(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func zona(t *testing.T, fecha string, hora, minuto int) time.Time {
	t.Helper()
	d, err := domain.ParseFecha(fecha)
	require.NoError(t, err)
	return time.Date(d.Year(), d.Month(), d.Day(), hora, minuto, 0, 0, domain.Zona)
}

func claseN(n int, actividad string, inicio time.Time, capacidad, ocupacion int) domain.Clase {
	return domain.Clase{ID: uuidN(n), ActividadID: actividad, ActividadNombre: "Act " + actividad[len(actividad)-2:], Inicio: inicio.UTC(), Fin: inicio.Add(time.Hour).UTC(),
		ProfesorID: uuidN(500), ProfesorNombre: "Profe", Capacidad: capacidad, Ocupacion: ocupacion, Estado: domain.ClaseProgramada, Version: 1}
}

func refrescar(t *testing.T, c *Client) {
	t.Helper()
	_, err := c.do(context.Background(), http.MethodPost, "/"+Alias+"/_refresh", nil, nil)
	require.NoError(t, err)
}

func ids(p domain.PaginaClases) []string {
	out := []string{}
	for _, c := range p.Items {
		out = append(out, c.ID)
	}
	return out
}

func TestOpenSearchReal(t *testing.T) {
	c := nuevoOpenSearch(t)
	ctx := context.Background()
	require.NoError(t, c.Ping(ctx))
	require.NoError(t, c.Asegurar(ctx, "clases-1"))
	require.NoError(t, c.Asegurar(ctx, "clases-2"), "con el alias ya creado no hace nada")

	actA, actB := uuidN(100), uuidN(101)
	dia := time.Now().In(domain.Zona).AddDate(0, 0, 3).Format(time.DateOnly)
	base := []domain.Clase{
		claseN(1, actA, zona(t, dia, 9, 0), 10, 0),
		claseN(2, actA, zona(t, dia, 9, 0), 10, 10), // mismo inicio: desempata el id
		claseN(3, actB, zona(t, dia, 18, 0), 10, 3),
		claseN(4, actB, zona(t, dia, 7, 30), 10, 0),
	}
	cancelada := claseN(5, actA, zona(t, dia, 12, 0), 10, 0)
	cancelada.Estado = domain.ClaseCancelada
	base = append(base, cancelada)
	for _, cl := range base {
		aplicado, err := c.Indexar(ctx, cl)
		require.NoError(t, err)
		require.True(t, aplicado)
	}
	refrescar(t, c)
	desde, _ := domain.ParseFecha(dia)
	filtro := domain.FiltroClases{Desde: desde, Hasta: desde.AddDate(0, 0, 1), Pagina: 1, Tamano: 20}

	t.Run("orden_inicio_e_id_y_paginacion", func(t *testing.T) {
		page, err := c.Buscar(ctx, filtro, time.Now())
		require.NoError(t, err)
		require.Equal(t, 5, page.Total)
		require.Equal(t, []string{uuidN(4), uuidN(1), uuidN(2), uuidN(5), uuidN(3)}, ids(page))
		f := filtro
		f.Tamano, f.Pagina = 2, 2
		page, err = c.Buscar(ctx, f, time.Now())
		require.NoError(t, err)
		require.Equal(t, []string{uuidN(2), uuidN(5)}, ids(page))
		require.Equal(t, "Act 00", page.Items[0].ActividadNombre)
	})

	t.Run("filtros_actividad_franja_y_disponibles", func(t *testing.T) {
		f := filtro
		f.ActividadID = actA
		page, err := c.Buscar(ctx, f, time.Now())
		require.NoError(t, err)
		require.Equal(t, []string{uuidN(1), uuidN(2), uuidN(5)}, ids(page))
		f = filtro
		f.Franja = "09:00"
		page, err = c.Buscar(ctx, f, time.Now())
		require.NoError(t, err)
		require.Equal(t, []string{uuidN(1), uuidN(2)}, ids(page), "franja en hora de Argentina")
		f = filtro
		f.SoloDisponibles = true
		page, err = c.Buscar(ctx, f, time.Now())
		require.NoError(t, err)
		require.Equal(t, []string{uuidN(4), uuidN(1), uuidN(3)}, ids(page), "sin la llena ni la cancelada")
		f.Desde, f.Hasta = desde.AddDate(0, 0, 1), desde.AddDate(0, 0, 2)
		page, err = c.Buscar(ctx, f, time.Now())
		require.NoError(t, err)
		require.Zero(t, page.Total, "fuera de rango")
	})

	t.Run("version_vieja_no_pisa_nueva", func(t *testing.T) {
		nueva := base[0]
		nueva.Version, nueva.Ocupacion = 3, 4
		aplicado, err := c.Indexar(ctx, nueva)
		require.NoError(t, err)
		require.True(t, aplicado)
		vieja := base[0]
		vieja.Version, vieja.Ocupacion = 2, 1
		aplicado, err = c.Indexar(ctx, vieja)
		require.NoError(t, err)
		require.False(t, aplicado, "fuera de orden se descarta")
		aplicado, err = c.Indexar(ctx, nueva)
		require.NoError(t, err)
		require.False(t, aplicado, "reentrega de la misma versión se descarta")
		refrescar(t, c)
		page, err := c.Buscar(ctx, filtro, time.Now())
		require.NoError(t, err)
		require.Equal(t, 4, page.Items[1].Ocupacion)
		require.EqualValues(t, 3, page.Items[1].Version)
	})

	t.Run("reindexado_con_alias_y_cambios_concurrentes", func(t *testing.T) {
		fuente := &fuenteMemoria{clases: map[string]domain.Clase{}}
		for i := 0; i < 1200; i++ {
			cl := claseN(1000+i, actB, zona(t, dia, 6, 0).Add(time.Duration(i)*30*time.Second), 30, 0)
			fuente.clases[cl.ID] = cl
		}
		// Mientras se reconstruye, "booking" sigue cambiando clases y el indexer
		// las escribe por el alias, como en producción.
		stop := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			for v := int64(2); ; v++ {
				select {
				case <-stop:
					return
				default:
				}
				cl := fuente.cambiar(uuidN(1000+int(v%50)), v)
				_, _ = c.Indexar(ctx, cl)
				time.Sleep(5 * time.Millisecond)
			}
		}()
		out, err := application.Reindexar(ctx, fuente, c, "clases-3")
		close(stop)
		wg.Wait()
		require.NoError(t, err)
		require.Equal(t, "clases-1", out.Anterior)
		// Converge también después de los últimos cambios concurrentes.
		require.Eventually(t, func() bool {
			n, s, _ := fuente.ResumenClases(ctx)
			ni, si, err := c.Resumen(ctx, "clases-3")
			return err == nil && n == ni && s == si
		}, 15*time.Second, 200*time.Millisecond)
		status, err := c.do(ctx, http.MethodHead, "/clases-1", nil, nil)
		require.NoError(t, err)
		require.Equal(t, 404, status, "el índice anterior se elimina")
		page, err := c.Buscar(ctx, domain.FiltroClases{Desde: desde, Hasta: desde.AddDate(0, 0, 1), ActividadID: actB, Pagina: 1, Tamano: 100}, time.Now())
		require.NoError(t, err)
		require.Equal(t, 1200, page.Total, "la búsqueda lee el índice nuevo por el alias")
	})

	t.Run("sin_motor_responde_no_disponible", func(t *testing.T) {
		caido := &Client{URL: "http://127.0.0.1:1", HTTP: &http.Client{Timeout: time.Second}}
		_, err := caido.Buscar(ctx, filtro, time.Now())
		require.ErrorIs(t, err, ports.ErrNoDisponible)
		_, err = caido.Indexar(ctx, base[0])
		require.ErrorIs(t, err, ports.ErrNoDisponible)
	})
}

// fuenteMemoria simula booking_db con versiones que crecen.
type fuenteMemoria struct {
	mu     sync.Mutex
	clases map[string]domain.Clase
}

func (f *fuenteMemoria) cambiar(id string, ocupacion int64) domain.Clase {
	f.mu.Lock()
	defer f.mu.Unlock()
	cl := f.clases[id]
	cl.Version++
	cl.Ocupacion = int(ocupacion % 30)
	f.clases[id] = cl
	return cl
}

func (f *fuenteMemoria) RecorrerClases(_ context.Context, lote int, fn func([]domain.Clase) error) error {
	f.mu.Lock()
	todas := make([]domain.Clase, 0, len(f.clases))
	for _, cl := range f.clases {
		todas = append(todas, cl)
	}
	f.mu.Unlock()
	for i := 0; i < len(todas); i += lote {
		end := min(i+lote, len(todas))
		if err := fn(todas[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (f *fuenteMemoria) ResumenClases(context.Context) (int64, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var suma int64
	for _, cl := range f.clases {
		suma += cl.Version
	}
	return int64(len(f.clases)), suma, nil
}
