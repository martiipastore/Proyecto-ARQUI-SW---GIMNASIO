// Package melt es el adaptador anticorrupción hacia la capacidad del grupo
// proveedor MELT (etapa 13, D-92, ADR-009 v2). Traduce el contrato externo
// «MELT - Acreditación externa de puntos» v1.1.0 (docs/contracts/providers/melt)
// a resultados del dominio; ningún nombre ni código de MELT sale de este paquete.
package melt

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// ErrIncierto indica timeout, red o 5xx: el resultado se desconoce y se
// reintenta con la misma referencia (contrato MELT: «un timeout no demuestra
// que la operación falló»).
var ErrIncierto = errors.New("resultado incierto en MELT")

// Client llama a POST {URL}/beneficios/acreditaciones con X-Partner-Key.
type Client struct {
	URL  string // https://melt-gateway-prueba.onrender.com/api/v1
	Key  string
	HTTP *http.Client
}

// Solicitud es el cuerpo exacto del contrato: un solo destinatario (email) y
// la referencia como alias de solicitudId.
type Solicitud struct {
	Email       string `json:"email"`
	Puntos      int64  `json:"puntos"`
	Referencia  string `json:"referencia"`
	Descripcion string `json:"descripcion,omitempty"`
}

type pendiente struct {
	Estado      string    `json:"estado"`
	RegistroURL string    `json:"registroUrl"`
	VenceEn     time.Time `json:"venceEn"`
}

type errorMelt struct {
	Error struct {
		Code string `json:"code"`
	} `json:"error"`
}

// Acreditar implementa ports.ProveedorMelt.
func (c Client) Acreditar(ctx context.Context, email string, puntos int64, referencia, descripcion string) (domain.ResultadoMelt, error) {
	body, err := json.Marshal(Solicitud{Email: strings.TrimSpace(email), Puntos: puntos, Referencia: referencia, Descripcion: descripcion})
	if err != nil {
		return domain.ResultadoMelt{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.URL, "/")+"/beneficios/acreditaciones", bytes.NewReader(body))
	if err != nil {
		return domain.ResultadoMelt{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Partner-Key", c.Key)
	if id := correlation.FromContext(ctx); id != "" {
		req.Header.Set(correlation.Header, id)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return domain.ResultadoMelt{}, ErrIncierto
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 16384))
	return Traducir(res.StatusCode, raw)
}

// Traducir convierte la respuesta HTTP de MELT en un resultado del dominio.
// Exportada para el test de contrato.
func Traducir(status int, body []byte) (domain.ResultadoMelt, error) {
	switch status {
	case http.StatusCreated, http.StatusOK:
		// 201 acreditó; 200 es la misma operación ya acreditada (idempotente).
		return domain.ResultadoMelt{Tipo: domain.ResultadoAcreditado}, nil
	case http.StatusAccepted:
		var p pendiente
		if err := json.Unmarshal(body, &p); err != nil || p.RegistroURL == "" || !strings.HasPrefix(p.RegistroURL, "https://") {
			return domain.ResultadoMelt{}, ErrIncierto
		}
		return domain.ResultadoMelt{Tipo: domain.ResultadoPendiente, RegistroURL: p.RegistroURL, RegistroVence: p.VenceEn,
			Motivo: "Registrate en MELT con tu mismo email para recibir los puntos."}, nil
	case http.StatusUnauthorized:
		// Clave ausente o incorrecta: es configuración nuestra, no un rechazo del
		// envío. Se reintenta para que se recupere al corregir la clave.
		return domain.ResultadoMelt{Tipo: domain.ResultadoReintentar, Motivo: "MELT rechazó la credencial de integración; revisar BENEFITS_MELT_PARTNER_KEY."}, nil
	case http.StatusBadRequest, http.StatusNotFound, http.StatusConflict, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		var e errorMelt
		_ = json.Unmarshal(body, &e)
		code := e.Error.Code
		if code == "" {
			code = http.StatusText(status)
		}
		return domain.ResultadoMelt{Tipo: domain.ResultadoRechazado, Motivo: "MELT rechazó la acreditación (" + code + ")."}, nil
	default:
		return domain.ResultadoMelt{}, ErrIncierto
	}
}
