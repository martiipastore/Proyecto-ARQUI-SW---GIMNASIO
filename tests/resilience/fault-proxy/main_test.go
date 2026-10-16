package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
)

type notifiedBuffer struct {
	bytes.Buffer
	ready chan struct{}
	once  sync.Once
}

func (b *notifiedBuffer) Write(p []byte) (int, error) {
	n, err := b.Buffer.Write(p)
	b.once.Do(func() { close(b.ready) })
	return n, err
}

// D10 / RN-27 / CL-08b: el inyector retiene una sola respuesta ya recibida.
func TestResponseWithheldOnce(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(201)
		_, _ = io.WriteString(w, `{"confirmed":true}`)
	}))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	output := &notifiedBuffer{ready: make(chan struct{})}
	h := faultProxy(target, "after-once", slog.New(slog.NewJSONHandler(output, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest("POST", "/v1/accounts/synthetic/credits", nil).WithContext(ctx)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { h.ServeHTTP(response, request); close(done) }()
	<-output.ready
	// La respuesta del backend ya fue consumida; ahora el gateway corta la espera.
	cancel()
	<-done
	var event struct {
		Hash   string `json:"response_sha256"`
		Status int    `json:"original_status"`
	}
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	retry := httptest.NewRecorder()
	h.ServeHTTP(retry, httptest.NewRequest("POST", "/v1/accounts/synthetic/credits", nil))
	digest := sha256.Sum256(retry.Body.Bytes())
	if calls.Load() != 2 || retry.Code != 201 || event.Status != 201 || event.Hash != hex.EncodeToString(digest[:]) {
		t.Fatal("no se conservó la respuesta original o la falla no fue de un solo intento")
	}
}

func TestBeforeModeDoesNotReachBenefits(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer upstream.Close()
	target, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	h := faultProxy(target, "before", slog.New(slog.NewJSONHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/accounts/synthetic/credits", nil).WithContext(ctx))
	if calls.Load() != 0 {
		t.Fatal("la falla previa alcanzó el backend")
	}
}
