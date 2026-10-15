package httpserver

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServe_EsperaLasSolicitudesEnCursoAlApagar(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	started := make(chan struct{})
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "terminado")
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Serve(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), ln,
			Options{Handler: handler, ShutdownTimeout: 2 * time.Second})
	}()

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String())
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		resCh <- result{body: string(b), err: err}
	}()

	<-started
	cancel() // llega la señal de apagado con una solicitud en curso

	res := <-resCh
	require.NoError(t, res.err)
	assert.Equal(t, "terminado", res.body, "la solicitud en curso debe completarse")
	require.NoError(t, <-done)

	_, err = net.DialTimeout("tcp", ln.Addr().String(), 200*time.Millisecond)
	assert.Error(t, err, "después del apagado no acepta conexiones")
}

func TestRun_ErrorSiElPuertoEstaOcupado(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()

	err = Run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)),
		Options{Addr: ln.Addr().String(), Handler: http.NotFoundHandler()})
	assert.Error(t, err)
}
