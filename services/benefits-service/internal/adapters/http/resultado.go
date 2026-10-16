package http

import (
	"encoding/json"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"strings"
)

type respuestaPersistida struct {
	Estado int
	Cuerpo json.RawMessage
}

// ResultadoEncoder es un adaptador de salida. La aplicación trata los bytes
// como opacos: no conoce HTTP/JSON, pero los confirma junto al movimiento.
type ResultadoEncoder struct{}

func (ResultadoEncoder) Representar(r domain.ResultadoOperacion, usuario, correlacion string) ([]byte, error) {
	var valor any
	estado := 201
	if r.Rechazo != "" {
		estado, code, title, detail := 400, "INVALID_REQUEST", "Solicitud inválida", "La solicitud no cumple los campos o límites del contrato."
		if r.Rechazo == domain.CuentaNoEncontrada {
			estado = 404
			code = "ACCOUNT_NOT_FOUND"
			title = "Cuenta no encontrada"
			detail = "La cuenta no está vinculada a este partner."
		}
		valor = map[string]any{"type": "https://gimnasio.example.com/problems/" + strings.ReplaceAll(strings.ToLower(code), "_", "-"), "title": title, "status": estado, "detail": detail, "instance": "/v1/accounts/" + usuario + "/credits", "code": code, "correlationId": correlacion}
		body, err := json.Marshal(valor)
		if err != nil {
			return nil, err
		}
		return json.Marshal(respuestaPersistida{Estado: estado, Cuerpo: body})
	}
	m := r.Movimiento.Datos()
	valor = operationResult{MovementID: m.ID, ExternalUserID: usuario, Type: "CREDIT", Points: m.Puntos, Reason: m.Motivo, Reference: m.Referencia, OccurredAt: m.OcurridoEn, Balance: r.Saldo}
	body, err := json.Marshal(valor)
	if err != nil {
		return nil, err
	}
	return json.Marshal(respuestaPersistida{Estado: estado, Cuerpo: body})
}
