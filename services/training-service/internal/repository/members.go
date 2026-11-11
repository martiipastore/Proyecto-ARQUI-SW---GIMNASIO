package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/training-service/internal/service"
)

// Members consulta las APIs internas de members-service (users.v1) en cada
// operación, sin caché (D-33). Cualquier respuesta inesperada es indisponibilidad
// para que la autorización falle cerrada (ADR-007).
type Members struct {
	URL, Secret string
	HTTP        *http.Client
}

var _ service.Members = Members{}

func (m Members) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(m.URL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Service-Key", m.Secret)
	req.Header.Set(correlation.Header, correlation.FromContext(ctx))
	res, err := m.HTTP.Do(req)
	if err != nil {
		return errors.Join(model.ErrNoDisponible, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: members respondió %d", model.ErrNoDisponible, res.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 256<<10)).Decode(out); err != nil {
		return errors.Join(model.ErrNoDisponible, err)
	}
	return nil
}

func (m Members) AsignacionVigente(ctx context.Context, profesional, alumno string) (bool, error) {
	var out struct {
		Vigente *bool `json:"vigente"`
	}
	q := url.Values{"profesional_id": {profesional}, "alumno_id": {alumno}}
	if err := m.get(ctx, "/internal/v1/asignaciones?"+q.Encode(), &out); err != nil {
		return false, err
	}
	if out.Vigente == nil {
		return false, model.ErrNoDisponible
	}
	return *out.Vigente, nil
}

func (m Members) vigentes(ctx context.Context, clave, id string) ([]model.Asignacion, error) {
	var out struct {
		Items []struct {
			ProfesionalID     string  `json:"profesional_id"`
			ProfesionalRol    string  `json:"profesional_rol"`
			ProfesionalNombre string  `json:"profesional_nombre"`
			AlumnoID          string  `json:"alumno_id"`
			AlumnoNombre      string  `json:"alumno_nombre"`
			AlumnoApellido    *string `json:"alumno_apellido"`
		} `json:"items"`
	}
	if err := m.get(ctx, "/internal/v1/asignaciones/vigentes?"+url.Values{clave: {id}}.Encode(), &out); err != nil {
		return nil, err
	}
	res := make([]model.Asignacion, 0, len(out.Items))
	for _, i := range out.Items {
		nombre := i.AlumnoNombre
		if i.AlumnoApellido != nil {
			nombre += " " + *i.AlumnoApellido
		}
		res = append(res, model.Asignacion{ProfesionalID: i.ProfesionalID, ProfesionalRol: i.ProfesionalRol, ProfesionalNombre: i.ProfesionalNombre, AlumnoID: i.AlumnoID, AlumnoNombre: nombre})
	}
	return res, nil
}

func (m Members) ProfesionalesDe(ctx context.Context, alumno string) ([]model.Asignacion, error) {
	return m.vigentes(ctx, "alumno_id", alumno)
}

func (m Members) AlumnosDe(ctx context.Context, profesional string) ([]model.Asignacion, error) {
	return m.vigentes(ctx, "profesional_id", profesional)
}
