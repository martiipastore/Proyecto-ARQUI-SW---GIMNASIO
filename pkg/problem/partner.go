package problem

import (
	"encoding/json"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"net/http"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// PartnerRequest conserva solo IDs compatibles con el contrato UUID de partners.
func PartnerRequest(r *http.Request) *http.Request {
	id := correlation.FromContext(r.Context())
	if !uuidPattern.MatchString(id) {
		id = correlation.NewID()
	}
	r = r.WithContext(correlation.NewContext(r.Context(), id))
	r.Header.Set(correlation.Header, id)
	return r
}

// WritePartnerHTTP aplica la excepción camelCase del contrato público mediante
// la representación común RFC 7807, sin alterar errores de APIs internas.
func WritePartnerHTTP(w http.ResponseWriter, r *http.Request, p Problem) {
	if p.Instance == "" {
		p.Instance = r.URL.Path
	}
	if p.CorrelationID == "" {
		p.CorrelationID = correlation.FromContext(r.Context())
	}
	p.Type = "https://gimnasio.example.com/problems/" + strings.ReplaceAll(strings.ToLower(p.Code), "_", "-")
	raw, err := json.Marshal(p)
	var body map[string]any
	if err != nil {
		p.Status = 500
		body = map[string]any{"type": "https://gimnasio.example.com/problems/internal", "title": "Error interno", "status": 500, "code": "INTERNAL"}
	} else {
		_ = json.Unmarshal(raw, &body)
	}
	delete(body, "correlation_id")
	body["correlationId"] = p.CorrelationID
	w.Header().Set("Content-Type", ContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set(correlation.Header, correlation.FromContext(r.Context()))
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(body)
}
