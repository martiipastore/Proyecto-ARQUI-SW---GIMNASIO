package proxy

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// conexionesNuevas cuenta las conexiones TCP que abre un transporte para atender
// clientes × rondas solicitudes concurrentes.
func conexionesNuevas(t *testing.T, tr http.RoundTripper, clientes, rondas int) int64 {
	t.Helper()
	var nuevas atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			nuevas.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()
	client := &http.Client{Transport: tr}
	var wg sync.WaitGroup
	for i := 0; i < clientes; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < rondas; j++ {
				res, err := client.Get(srv.URL)
				if err != nil {
					t.Error(err)
					return
				}
				_, _ = io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}
		}()
	}
	wg.Wait()
	return nuevas.Load()
}

// Etapa 9 (D-88): con concurrencia mayor a 2 el gateway reutiliza conexiones
// hacia el servicio en lugar de abrir una por solicitud. Se compara contra el
// transporte por defecto de Go (2 ociosas por destino) en la misma corrida.
func TestTransportReutilizaConexiones(t *testing.T) {
	const clientes, rondas = 10, 50
	tr := NewTransport()
	require.Equal(t, MaxConexionesOciosasPorServicio, tr.MaxIdleConnsPerHost)
	propio := conexionesNuevas(t, tr, clientes, rondas)
	porDefecto := conexionesNuevas(t, http.DefaultTransport.(*http.Transport).Clone(), clientes, rondas)
	t.Logf("conexiones nuevas: pool del gateway=%d, transporte por defecto=%d", propio, porDefecto)
	// Al arrancar, varias solicitudes simultáneas pueden abrir alguna conexión de
	// más antes de que haya ociosas; el pool igual queda acotado por la concurrencia.
	require.LessOrEqual(t, propio, int64(3*clientes), "el pool queda acotado por la concurrencia")
	require.Less(t, propio*3, porDefecto, "el pool abre muchas menos conexiones que el transporte por defecto")
}
