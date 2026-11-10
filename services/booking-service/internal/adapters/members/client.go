// Package members es el adaptador de salida hacia las APIs internas de
// members-service (users.v1): vigencia de membresías, búsqueda por DNI y datos
// mínimos de alumnos y profesores. Cualquier respuesta inesperada se informa
// como indisponibilidad para que las validaciones fallen cerradas (ADR-005 v2).
package members

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Client llama a members-service con la clave interna entre servicios.
type Client struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

var _ ports.Members = Client{}

func (c Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, body)
	if err != nil {
		return err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Internal-Service-Key", c.Secret)
	req.Header.Set(correlation.Header, correlation.FromContext(ctx))
	res, err := c.HTTP.Do(req)
	if err != nil {
		return errors.Join(ports.ErrNoDisponible, err)
	}
	defer func() { _ = res.Body.Close() }()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return domain.ErrNoEncontrado
	default:
		return ports.ErrNoDisponible
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 64<<10)).Decode(out); err != nil {
		return errors.Join(ports.ErrNoDisponible, err)
	}
	return nil
}

type persona struct {
	ID       string  `json:"id"`
	Nombre   string  `json:"nombre"`
	Apellido *string `json:"apellido"`
	Estado   string  `json:"estado"`
}

func (p persona) dominio() domain.Persona {
	return domain.Persona{ID: p.ID, Nombre: p.Nombre, Apellido: p.Apellido, Estado: p.Estado}
}

// Vigencia consulta GET /internal/v1/alumnos/{id}/vigencia (RN-12).
func (c Client) Vigencia(ctx context.Context, alumnoID string, fechas []string) (domain.Vigencia, error) {
	var out struct {
		AlumnoActivo bool    `json:"alumno_activo"`
		VigenteHoy   bool    `json:"vigente_hoy"`
		VenceHoy     *string `json:"vence_hoy"`
		VigenteEn    map[string]struct {
			Vigente bool `json:"vigente"`
		} `json:"vigente_en"`
	}
	path := "/internal/v1/alumnos/" + url.PathEscape(alumnoID) + "/vigencia"
	if len(fechas) > 0 {
		path += "?fechas=" + url.QueryEscape(strings.Join(fechas, ","))
	}
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		if errors.Is(err, domain.ErrNoEncontrado) {
			// Un alumno que members no conoce no tiene membresía vigente.
			return domain.Vigencia{}, nil
		}
		return domain.Vigencia{}, err
	}
	v := domain.Vigencia{AlumnoActivo: out.AlumnoActivo, VigenteHoy: out.VigenteHoy, VenceHoy: out.VenceHoy, VigenteEn: map[string]bool{}}
	for fecha, cobertura := range out.VigenteEn {
		v.VigenteEn[fecha] = cobertura.Vigente
	}
	return v, nil
}

// AlumnoPorDNI usa POST para que el DNI no viaje en la URL (users.v1).
func (c Client) AlumnoPorDNI(ctx context.Context, dni string) (domain.Persona, error) {
	var out persona
	err := c.do(ctx, http.MethodPost, "/internal/v1/alumnos/buscar-dni", map[string]string{"dni": dni}, &out)
	return out.dominio(), err
}

func (c Client) resumen(ctx context.Context, path string, ids []string) (map[string]domain.Persona, error) {
	out := map[string]domain.Persona{}
	for inicio := 0; inicio < len(ids); inicio += 100 {
		fin := inicio + 100
		if fin > len(ids) {
			fin = len(ids)
		}
		var page struct {
			Items []persona `json:"items"`
		}
		if err := c.do(ctx, http.MethodGet, path+"?ids="+url.QueryEscape(strings.Join(ids[inicio:fin], ",")), nil, &page); err != nil {
			return nil, err
		}
		for _, p := range page.Items {
			out[p.ID] = p.dominio()
		}
	}
	return out, nil
}

// Alumnos devuelve nombre y estado de los alumnos pedidos (HU-21, D-22).
func (c Client) Alumnos(ctx context.Context, ids []string) (map[string]domain.Persona, error) {
	return c.resumen(ctx, "/internal/v1/alumnos", unicos(ids))
}

// Profesores devuelve solo personas con rol PROFESOR (RN-08, CA-07.6).
func (c Client) Profesores(ctx context.Context, ids []string) (map[string]domain.Persona, error) {
	return c.resumen(ctx, "/internal/v1/profesores", unicos(ids))
}

func unicos(ids []string) []string {
	vistos := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if !vistos[id] {
			vistos[id] = true
			out = append(out, id)
		}
	}
	return out
}
