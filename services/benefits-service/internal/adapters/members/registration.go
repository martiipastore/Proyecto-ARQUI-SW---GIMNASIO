package members

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
)

type Client struct {
	URL, Secret string
	HTTP        *http.Client
}
type receipt struct {
	ID       string `json:"registration_id"`
	Event    string `json:"event_id"`
	External string `json:"external_user_id"`
	State    string `json:"status"`
}

func (c Client) request(ctx context.Context, method, path string, in any) (domain.ReciboRegistro, error) {
	var body []byte
	var err error
	if in != nil {
		body, err = json.Marshal(in)
		if err != nil {
			return domain.ReciboRegistro{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, bytes.NewReader(body))
	if err != nil {
		return domain.ReciboRegistro{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Service-Key", c.Secret)
	req.Header.Set(correlation.Header, correlation.FromContext(ctx))
	res, err := c.HTTP.Do(req)
	if err != nil {
		return domain.ReciboRegistro{}, ports.ErrAlmacenNoDisponible
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case 200, 202:
	case 400:
		return domain.ReciboRegistro{}, domain.ErrOperacionInvalida
	case 409:
		return domain.ReciboRegistro{}, domain.ErrConflictoIdempotencia
	case 401:
		return domain.ReciboRegistro{}, domain.ErrCuentaNoVinculada
	default:
		return domain.ReciboRegistro{}, ports.ErrAlmacenNoDisponible
	}
	var out receipt
	if err = json.NewDecoder(io.LimitReader(res.Body, 8192)).Decode(&out); err != nil {
		return domain.ReciboRegistro{}, ports.ErrAlmacenNoDisponible
	}
	return domain.ReciboRegistro{ID: out.ID, EventoID: out.Event, ExternalUserID: out.External, Estado: out.State}, nil
}
func (c Client) Registrar(ctx context.Context, in domain.RegistroExterno) (domain.ReciboRegistro, error) {
	dto := struct {
		PartnerID string `json:"partner_id"`
		Platform  string `json:"platform"`
		Event     string `json:"event_id"`
		Key       string `json:"key"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		External  string `json:"external_user_id"`
		Points    int64  `json:"points"`
		Reason    string `json:"reason"`
		Reference string `json:"reference"`
	}{in.PartnerID, in.Plataforma, in.EventoID, in.Clave, in.Nombre, in.Email, in.ExternalUserID, in.Puntos, in.Motivo, in.Referencia}
	return c.request(ctx, http.MethodPost, "/internal/v1/external-registrations", dto)
}
func (c Client) ConsultarRegistro(ctx context.Context, partner, event string) (domain.ReciboRegistro, error) {
	return c.request(ctx, http.MethodGet, "/internal/v1/external-registrations/"+url.PathEscape(partner)+"/"+url.PathEscape(event), nil)
}
