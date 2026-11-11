package members

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

// EmailDe consulta el email del alumno en Members (etapa 13, D-92). El email no
// se guarda en benefits_db ni se registra en logs: se usa solo para el envío.
func (c Client) EmailDe(ctx context.Context, alumno string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/internal/v1/alumnos/"+url.PathEscape(alumno)+"/contacto", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Internal-Service-Key", c.Secret)
	req.Header.Set(correlation.Header, correlation.FromContext(ctx))
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", ports.ErrAlmacenNoDisponible
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return "", domain.ErrCuentaNoVinculada
	default:
		return "", ports.ErrAlmacenNoDisponible
	}
	var out struct {
		Email string `json:"email"`
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&out); err != nil || out.Email == "" {
		return "", ports.ErrAlmacenNoDisponible
	}
	return out.Email, nil
}
