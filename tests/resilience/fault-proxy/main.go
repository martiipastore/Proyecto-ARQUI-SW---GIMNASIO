// Proxy exclusivo del ejercicio D10; no forma parte del stack normal.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
)

func faultProxy(target *url.URL, mode string, logger *slog.Logger) http.Handler {
	var withheld atomic.Bool
	p := httputil.NewSingleHostReverseProxy(target)
	p.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.WriteHeader(http.StatusBadGateway)
	}
	p.ModifyResponse = func(response *http.Response) error {
		if mode != "after-once" || response.Request.Method != http.MethodPost || response.StatusCode != 201 || !withheld.CompareAndSwap(false, true) {
			return nil
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			return err
		}
		digest := sha256.Sum256(body)
		fields := []any{"event", "response_withheld", "original_status", 201, "response_sha256", hex.EncodeToString(digest[:])}
		// Solo aceptar un trace ID hexadecimal de longitud fija; no loguear headers.
		parts := strings.Split(response.Request.Header.Get("traceparent"), "-")
		if len(parts) == 4 && len(parts[1]) == 32 {
			if _, err := hex.DecodeString(parts[1]); err == nil {
				fields = append(fields, "trace_id", parts[1])
			}
		}
		logger.Info("respuesta confirmada retenida por el ejercicio", fields...)
		<-response.Request.Context().Done()
		return response.Request.Context().Err()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if mode == "before" && r.Method == http.MethodPost {
			<-r.Context().Done()
			return
		}
		p.ServeHTTP(w, r)
	})
}

func main() {
	target, err := url.Parse(os.Getenv("TARGET_URL"))
	if err != nil || target.Scheme != "http" || target.Host == "" {
		log.Fatal("destino de pruebas inválido")
	}
	mode := os.Getenv("FAULT_MODE")
	if mode != "before" && mode != "after-once" {
		log.Fatal("modo de pruebas inválido")
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	log.Fatal(http.ListenAndServe(":9090", faultProxy(target, mode, logger)))
}
