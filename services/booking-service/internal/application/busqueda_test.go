// Etapa 8 de la entrega final: GET /classes desde el índice (ADR-014, D-45).
package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

type indiceFalso struct {
	pagina domain.PaginaClases
	err    error
	filtro domain.FiltroClases
}

func (i *indiceFalso) Buscar(_ context.Context, f domain.FiltroClases, _ time.Time) (domain.PaginaClases, error) {
	i.filtro = f
	return i.pagina, i.err
}

type marcasFalsas struct {
	alumno string
	err    error
}

func (m *marcasFalsas) ReservadasPor(_ context.Context, a string, ids []string) (map[string]bool, error) {
	m.alumno = a
	return map[string]bool{ids[0]: true}, m.err
}

func TestBusquedaIndexada(t *testing.T) {
	ahora := time.Date(2026, 11, 10, 15, 0, 0, 0, domain.Zona)
	indice := &indiceFalso{pagina: domain.PaginaClases{Items: []domain.Clase{{ID: "a"}, {ID: "b"}}, Total: 2, Pagina: 1, Tamano: 20}}
	marcas := &marcasFalsas{}
	s := New(&catalogo{}, &reservas{}, members{}, reloj{ahora}).ConBusqueda(indice, marcas)
	ctx := context.Background()

	t.Run("CA-13.4_ya_reservada_desde_la_fuente", func(t *testing.T) {
		page, err := s.BuscarClases(ctx, Actor{ID: alumno, Rol: RolAlumno}, domain.FiltroClases{Pagina: 1, Tamano: 20})
		if err != nil {
			t.Fatal(err)
		}
		if !page.Items[0].ReservadaPorMi || page.Items[1].ReservadaPorMi || marcas.alumno != alumno {
			t.Fatalf("marcas incorrectas: %+v", page.Items)
		}
		if indice.filtro.Hasta.Sub(indice.filtro.Desde) != 15*24*time.Hour {
			t.Fatal("no aplica el rango por defecto antes de consultar el índice")
		}
	})
	t.Run("administrador_no_consulta_marcas", func(t *testing.T) {
		marcas.alumno = ""
		if _, err := s.BuscarClases(ctx, Actor{ID: admin, Rol: RolAdministrador}, domain.FiltroClases{Pagina: 1, Tamano: 20}); err != nil || marcas.alumno != "" {
			t.Fatal("el administrador no tiene reservas propias que marcar")
		}
	})
	t.Run("D-45_profundidad_maxima_10000", func(t *testing.T) {
		if _, err := s.BuscarClases(ctx, Actor{ID: admin, Rol: RolAdministrador}, domain.FiltroClases{Pagina: 100, Tamano: 100}); err != nil {
			t.Fatalf("10.000 exactos se permiten: %v", err)
		}
		if _, err := s.BuscarClases(ctx, Actor{ID: admin, Rol: RolAdministrador}, domain.FiltroClases{Pagina: 101, Tamano: 100}); !errors.Is(err, domain.ErrValidacion) {
			t.Fatalf("más de 10.000 debe ser VALIDACION: %v", err)
		}
	})
	t.Run("indice_caido_es_503_sin_respaldo", func(t *testing.T) {
		indice.err = errors.New("conexión rechazada")
		_, err := s.BuscarClases(ctx, Actor{ID: alumno, Rol: RolAlumno}, domain.FiltroClases{Pagina: 1, Tamano: 20})
		if !errors.Is(err, ports.ErrNoDisponible) {
			t.Fatalf("debe ser dependencia no disponible: %v", err)
		}
		indice.err = nil
		marcas.err = errors.New("base caída")
		_, err = s.BuscarClases(ctx, Actor{ID: alumno, Rol: RolAlumno}, domain.FiltroClases{Pagina: 1, Tamano: 20})
		if !errors.Is(err, ports.ErrNoDisponible) {
			t.Fatalf("sin marcas confiables tampoco responde: %v", err)
		}
	})
}
