// Package opensearch es el adaptador de salida hacia el índice de búsqueda de
// clases (etapa 8, ADR-014). Usa la API REST con net/http, sin cliente externo:
// alias clases-read, versionado externo y consultas por filtros.
package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Alias es el nombre estable que leen la búsqueda y el indexer.
const Alias = "clases-read"

var (
	_ ports.IndiceClases    = (*Client)(nil)
	_ ports.IndiceEscritura = (*Client)(nil)
	_ ports.Reindexado      = (*Client)(nil)
)

// Client habla con un nodo OpenSearch. URL sin barra final.
type Client struct {
	URL  string
	HTTP *http.Client
}

// documento es la forma indexada: campos de filtro y orden más los datos que
// devuelve GET /classes. Las etiquetas viven acá, nunca en domain.
type documento struct {
	ID              string    `json:"id"`
	ActividadID     string    `json:"actividad_id"`
	ActividadNombre string    `json:"actividad_nombre"`
	Musculacion     bool      `json:"musculacion"`
	Inicio          time.Time `json:"inicio"`
	Fin             time.Time `json:"fin"`
	Fecha           string    `json:"fecha"`
	HoraInicio      string    `json:"hora_inicio"`
	ProfesorID      string    `json:"profesor_id"`
	ProfesorNombre  string    `json:"profesor_nombre"`
	Capacidad       int       `json:"capacidad"`
	Ocupacion       int       `json:"ocupacion"`
	Disponibles     int       `json:"disponibles"`
	Estado          string    `json:"estado"`
	Version         int64     `json:"version"`
}

func aDocumento(c domain.Clase) documento {
	return documento{ID: c.ID, ActividadID: c.ActividadID, ActividadNombre: c.ActividadNombre, Musculacion: c.Musculacion, Inicio: c.Inicio.UTC(), Fin: c.Fin.UTC(),
		Fecha: c.Fecha(), HoraInicio: c.HoraInicio(), ProfesorID: c.ProfesorID, ProfesorNombre: c.ProfesorNombre, Capacidad: c.Capacidad, Ocupacion: c.Ocupacion,
		Disponibles: c.Disponibles(), Estado: c.Estado, Version: c.Version}
}

func (d documento) clase() domain.Clase {
	return domain.Clase{ID: d.ID, ActividadID: d.ActividadID, ActividadNombre: d.ActividadNombre, Musculacion: d.Musculacion, Inicio: d.Inicio, Fin: d.Fin,
		ProfesorID: d.ProfesorID, ProfesorNombre: d.ProfesorNombre, Capacidad: d.Capacidad, Ocupacion: d.Ocupacion, Estado: d.Estado, Version: d.Version}
}

// Mapping estricto: un campo inesperado es un error, no un tipo adivinado.
const mapping = `{"settings":{"number_of_shards":1,"number_of_replicas":0},"mappings":{"dynamic":"strict","properties":{` +
	`"id":{"type":"keyword"},"actividad_id":{"type":"keyword"},"actividad_nombre":{"type":"keyword"},"musculacion":{"type":"boolean"},` +
	`"inicio":{"type":"date"},"fin":{"type":"date"},"fecha":{"type":"keyword"},"hora_inicio":{"type":"keyword"},` +
	`"profesor_id":{"type":"keyword"},"profesor_nombre":{"type":"keyword"},"capacidad":{"type":"integer"},"ocupacion":{"type":"integer"},` +
	`"disponibles":{"type":"integer"},"estado":{"type":"keyword"},"version":{"type":"long"}}}}`

// do ejecuta una solicitud. Las fallas de red y los 5xx/429 son indisponibilidad.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		switch b := body.(type) {
		case []byte:
			reader = bytes.NewReader(b)
		case string:
			reader = strings.NewReader(b)
		default:
			data, err := json.Marshal(b)
			if err != nil {
				return 0, err
			}
			reader = bytes.NewReader(data)
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, reader)
	if err != nil {
		return 0, err
	}
	if body != nil {
		ct := "application/json"
		if strings.HasPrefix(path, "/_bulk") || strings.Contains(path, "/_bulk") {
			ct = "application/x-ndjson"
		}
		req.Header.Set("Content-Type", ct)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return 0, errors.Join(ports.ErrNoDisponible, err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if err != nil {
		return res.StatusCode, errors.Join(ports.ErrNoDisponible, err)
	}
	if res.StatusCode >= 500 || res.StatusCode == 429 {
		return res.StatusCode, fmt.Errorf("%w: OpenSearch respondió %d", ports.ErrNoDisponible, res.StatusCode)
	}
	if out != nil && len(data) > 0 && res.StatusCode < 300 {
		if err := json.Unmarshal(data, out); err != nil {
			return res.StatusCode, err
		}
	}
	return res.StatusCode, nil
}

// Ping es el chequeo de readiness (dependencia parcial, ADR-007).
func (c *Client) Ping(ctx context.Context) error {
	status, err := c.do(ctx, http.MethodGet, "/_cluster/health", nil, nil)
	if err == nil && status != 200 {
		err = fmt.Errorf("%w: OpenSearch respondió %d", ports.ErrNoDisponible, status)
	}
	return err
}

// Asegurar crea el primer índice con el alias si todavía no existe.
func (c *Client) Asegurar(ctx context.Context, nombre string) error {
	status, err := c.do(ctx, http.MethodHead, "/_alias/"+Alias, nil, nil)
	if err != nil {
		return err
	}
	if status == 200 {
		return nil
	}
	if err := c.CrearIndice(ctx, nombre); err != nil {
		return err
	}
	_, err = c.Apuntar(ctx, nombre)
	return err
}

func (c *Client) CrearIndice(ctx context.Context, nombre string) error {
	status, err := c.do(ctx, http.MethodPut, "/"+url.PathEscape(nombre), mapping, nil)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("crear índice %s: HTTP %d", nombre, status)
	}
	return nil
}

// Indexar escribe la versión con version_type=external: OpenSearch rechaza con
// 409 una versión menor o igual a la guardada, que se trata como descartada.
func (c *Client) Indexar(ctx context.Context, cl domain.Clase) (bool, error) {
	path := fmt.Sprintf("/%s/_doc/%s?version=%d&version_type=external", Alias, url.PathEscape(cl.ID), cl.Version)
	status, err := c.do(ctx, http.MethodPut, path, aDocumento(cl), nil)
	if err != nil {
		return false, err
	}
	switch status {
	case 200, 201:
		return true, nil
	case 409:
		return false, nil
	default:
		return false, fmt.Errorf("indexar clase: HTTP %d", status)
	}
}

// Cargar envía un lote con _bulk y versionado externo. Los conflictos de versión
// son esperables en el repaso; cualquier otro error por ítem falla el lote.
func (c *Client) Cargar(ctx context.Context, indice string, clases []domain.Clase) error {
	if len(clases) == 0 {
		return nil
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, cl := range clases {
		if err := enc.Encode(map[string]any{"index": map[string]any{"_index": indice, "_id": cl.ID, "version": cl.Version, "version_type": "external"}}); err != nil {
			return err
		}
		if err := enc.Encode(aDocumento(cl)); err != nil {
			return err
		}
	}
	var out struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int `json:"status"`
		} `json:"items"`
	}
	status, err := c.do(ctx, http.MethodPost, "/_bulk", buf.Bytes(), &out)
	if err != nil {
		return err
	}
	if status != 200 {
		return fmt.Errorf("carga masiva: HTTP %d", status)
	}
	for _, item := range out.Items {
		for _, r := range item {
			if r.Status >= 300 && r.Status != 409 {
				return fmt.Errorf("carga masiva: ítem con HTTP %d", r.Status)
			}
		}
	}
	return nil
}

// Apuntar mueve el alias en una sola acción atómica de _aliases.
func (c *Client) Apuntar(ctx context.Context, nuevo string) (string, error) {
	var actuales map[string]any
	status, err := c.do(ctx, http.MethodGet, "/_alias/"+Alias, nil, &actuales)
	if err != nil {
		return "", err
	}
	anterior := ""
	acciones := []any{}
	if status == 200 {
		for indice := range actuales {
			anterior = indice
			acciones = append(acciones, map[string]any{"remove": map[string]any{"index": indice, "alias": Alias}})
		}
	}
	acciones = append(acciones, map[string]any{"add": map[string]any{"index": nuevo, "alias": Alias, "is_write_index": true}})
	status, err = c.do(ctx, http.MethodPost, "/_aliases", map[string]any{"actions": acciones}, nil)
	if err != nil {
		return anterior, err
	}
	if status != 200 {
		return anterior, fmt.Errorf("mover alias: HTTP %d", status)
	}
	return anterior, nil
}

// Resumen devuelve cantidad y suma de versiones del índice, después de refrescar.
func (c *Client) Resumen(ctx context.Context, indice string) (int64, int64, error) {
	if _, err := c.do(ctx, http.MethodPost, "/"+url.PathEscape(indice)+"/_refresh", nil, nil); err != nil {
		return 0, 0, err
	}
	var out struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
		} `json:"hits"`
		Aggregations struct {
			Suma struct {
				Value float64 `json:"value"`
			} `json:"suma"`
		} `json:"aggregations"`
	}
	body := `{"size":0,"track_total_hits":true,"aggs":{"suma":{"sum":{"field":"version"}}}}`
	status, err := c.do(ctx, http.MethodPost, "/"+url.PathEscape(indice)+"/_search", body, &out)
	if err != nil {
		return 0, 0, err
	}
	if status != 200 {
		return 0, 0, fmt.Errorf("resumen del índice: HTTP %d", status)
	}
	return out.Hits.Total.Value, int64(out.Aggregations.Suma.Value), nil
}

func (c *Client) Eliminar(ctx context.Context, indice string) error {
	status, err := c.do(ctx, http.MethodDelete, "/"+url.PathEscape(indice), nil, nil)
	if err != nil {
		return err
	}
	if status != 200 && status != 404 {
		return fmt.Errorf("eliminar índice: HTTP %d", status)
	}
	return nil
}

// Buscar aplica los filtros de HU-13 con la misma semántica que la consulta de
// PostgreSQL de la etapa 4: rango de inicio [desde, hasta), actividad, hora de
// inicio local, disponibilidad; orden inicio asc e id como desempate (ADR-014).
func (c *Client) Buscar(ctx context.Context, f domain.FiltroClases, now time.Time) (domain.PaginaClases, error) {
	out := domain.PaginaClases{Items: []domain.Clase{}, Pagina: f.Pagina, Tamano: f.Tamano}
	filtros := []any{map[string]any{"range": map[string]any{"inicio": map[string]any{"gte": f.Desde.UTC().Format(time.RFC3339), "lt": f.Hasta.UTC().Format(time.RFC3339)}}}}
	if f.ActividadID != "" {
		filtros = append(filtros, map[string]any{"term": map[string]any{"actividad_id": strings.ToLower(f.ActividadID)}})
	}
	if f.Franja != "" {
		filtros = append(filtros, map[string]any{"term": map[string]any{"hora_inicio": f.Franja}})
	}
	if f.SoloDisponibles {
		filtros = append(filtros,
			map[string]any{"term": map[string]any{"estado": domain.ClaseProgramada}},
			map[string]any{"range": map[string]any{"inicio": map[string]any{"gt": now.UTC().Format(time.RFC3339Nano)}}},
			map[string]any{"range": map[string]any{"disponibles": map[string]any{"gt": 0}}})
	}
	consulta := map[string]any{
		"from": (f.Pagina - 1) * f.Tamano, "size": f.Tamano, "track_total_hits": true,
		"query": map[string]any{"bool": map[string]any{"filter": filtros}},
		"sort":  []any{map[string]any{"inicio": "asc"}, map[string]any{"id": "asc"}},
	}
	var res struct {
		Hits struct {
			Total struct {
				Value int `json:"value"`
			} `json:"total"`
			Hits []struct {
				Source documento `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	status, err := c.do(ctx, http.MethodPost, "/"+Alias+"/_search", consulta, &res)
	if err != nil {
		return out, err
	}
	if status != 200 {
		// Sin alias o con el índice roto la búsqueda no puede responder.
		return out, fmt.Errorf("%w: búsqueda HTTP %d", ports.ErrNoDisponible, status)
	}
	out.Total = res.Hits.Total.Value
	for _, h := range res.Hits.Hits {
		out.Items = append(out.Items, h.Source.clase())
	}
	return out, nil
}
