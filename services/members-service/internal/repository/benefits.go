package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
)

// BenefitsHTTP no lee benefits_db: todo efecto cruza la API autenticada.
type BenefitsHTTP struct {
	URL, Secret string
	Client      *http.Client
}

func (b BenefitsHTTP) Fulfill(ctx context.Context, f model.Fulfillment) (int64, error) {
	data, err := json.Marshal(f)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.URL+"/internal/v1/external-registrations/fulfill", bytes.NewReader(data))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Key", b.Secret)
	req.Header.Set(correlation.Header, correlation.FromContext(ctx))
	res, err := b.Client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("benefits no disponible")
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != 200 {
		return 0, fmt.Errorf("benefits rechazó la operación: %d", res.StatusCode)
	}
	var out struct {
		Balance int64 `json:"balance"`
	}
	err = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&out)
	return out.Balance, err
}
