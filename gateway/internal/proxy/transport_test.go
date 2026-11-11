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

// Etapa 9 (D-88): con concurrencia mayor a 2 el gateway reutiliza conexiones
// hacia el servicio en lugar de abrir una por solicitud.
func TestTransportReutilizaConexiones(t *testing.T) {
	var nuevas atomic.Int64
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			nuevas.Add(1)
		}
	}
	srv.Start()
	defer srv.Close()

	tr := NewTransport()
	require.Equal(t, MaxConexionesOciosasPorServicio, tr.MaxIdleConnsPerHost)
	client := &http.Client{Transport: tr}
	const clientes, rondas = 10, 50
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
	// Con el transporte por defecto serían cientos de conexiones nuevas.
	require.LessOrEqual(t, nuevas.Load(), int64(clientes), "una conexión por cliente concurrente como máximo")
}
