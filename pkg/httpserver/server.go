// Package httpserver arranca un servidor HTTP y lo apaga ordenadamente
// (graceful shutdown) cuando se cancela el context, típicamente por SIGINT o SIGTERM.
package httpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Options configura el servidor.
type Options struct {
	Addr            string
	Handler         http.Handler
	ShutdownTimeout time.Duration
	// ReadHeaderTimeout protege contra clientes lentos (Slowloris). Por defecto 5s.
	ReadHeaderTimeout time.Duration
}

// Run atiende en opts.Addr hasta que ctx se cancela; entonces deja de aceptar
// conexiones y espera hasta ShutdownTimeout a que terminen las solicitudes en curso.
func Run(ctx context.Context, log *slog.Logger, opts Options) error {
	ln, err := net.Listen("tcp", opts.Addr)
	if err != nil {
		return fmt.Errorf("escuchar en %s: %w", opts.Addr, err)
	}
	return Serve(ctx, log, ln, opts)
}

// Serve es como Run pero sobre un listener ya abierto (útil en tests).
func Serve(ctx context.Context, log *slog.Logger, ln net.Listener, opts Options) error {
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	if opts.ReadHeaderTimeout <= 0 {
		opts.ReadHeaderTimeout = 5 * time.Second
	}
	srv := &http.Server{
		Handler:           opts.Handler,
		ReadHeaderTimeout: opts.ReadHeaderTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("servidor HTTP escuchando", slog.String("addr", ln.Addr().String()))
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Info("apagando servidor HTTP", slog.Duration("timeout", opts.ShutdownTimeout))
	shutdownCtx, cancel := context.WithTimeout(context.Background(), opts.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	log.Info("servidor HTTP detenido")
	return nil
}
