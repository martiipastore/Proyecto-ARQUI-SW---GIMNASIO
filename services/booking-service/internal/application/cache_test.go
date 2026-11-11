// Etapa 9 de la entrega final: cache-aside del catálogo de actividades
// (ADR-007, D-88). La fuente de verdad sigue siendo PostgreSQL.
package application

import (
	"context"
	"errors"
	"testing"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

type catalogoContado struct {
	catalogo
	lecturas int
	items    []domain.Actividad
	falla    error
}

func (c *catalogoContado) Actividades(context.Context) ([]domain.Actividad, error) {
	c.lecturas++
	return c.items, c.falla
}
func (c *catalogoContado) CrearActividad(_ context.Context, _ string, a domain.Actividad) (domain.Actividad, error) {
	a.ID = actID
	return a, c.falla
}
func (c *catalogoContado) ModificarActividad(_ context.Context, _, _ string, cambiar func(domain.Actividad, bool) (domain.Actividad, error)) (domain.Actividad, error) {
	if c.falla != nil {
		return domain.Actividad{}, c.falla
	}
	return cambiar(domain.Actividad{ID: actID, Nombre: "Zumba", CapacidadMaxima: 20, Activa: true}, false)
}

type cacheFalsa struct {
	items                  []domain.Actividad
	hay                    bool
	caida                  bool
	guardadas, invalidadas int
}

func (c *cacheFalsa) Leer(context.Context) ([]domain.Actividad, bool, error) {
	if c.caida {
		return nil, false, errors.New("redis caído")
	}
	return c.items, c.hay, nil
}
func (c *cacheFalsa) Guardar(_ context.Context, items []domain.Actividad) error {
	if c.caida {
		return errors.New("redis caído")
	}
	c.items, c.hay = items, true
	c.guardadas++
	return nil
}
func (c *cacheFalsa) Invalidar(context.Context) error {
	if c.caida {
		return errors.New("redis caído")
	}
	c.items, c.hay = nil, false
	c.invalidadas++
	return nil
}

func TestCatalogoConCache(t *testing.T) {
	ctx := context.Background()
	fuente := &catalogoContado{items: []domain.Actividad{{ID: actID, Nombre: "Zumba", CapacidadMaxima: 20, Activa: true}}}
	cache := &cacheFalsa{}
	s := New(fuente, &reservas{}, members{}, reloj{}).ConCache(cache)
	administrador := Actor{ID: admin, Rol: RolAdministrador}

	t.Run("miss_lee_la_fuente_y_completa_la_cache", func(t *testing.T) {
		items, err := s.Actividades(ctx)
		if err != nil || len(items) != 1 || fuente.lecturas != 1 || cache.guardadas != 1 {
			t.Fatalf("miss: items=%v err=%v lecturas=%d guardadas=%d", items, err, fuente.lecturas, cache.guardadas)
		}
	})
	t.Run("acierto_no_consulta_postgres", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			if _, err := s.Actividades(ctx); err != nil {
				t.Fatal(err)
			}
		}
		if fuente.lecturas != 1 {
			t.Fatalf("los aciertos no deben leer la base: %d lecturas", fuente.lecturas)
		}
	})
	t.Run("escribir_invalida_despues_de_confirmar", func(t *testing.T) {
		nombre := "Zumba Fitness"
		if _, err := s.ModificarActividad(ctx, administrador, actID, &nombre, nil, nil); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CrearActividad(ctx, administrador, "Pilates", 20); err != nil {
			t.Fatal(err)
		}
		if cache.invalidadas != 2 || cache.hay {
			t.Fatalf("alta y modificación deben invalidar: %d", cache.invalidadas)
		}
		if _, err := s.Actividades(ctx); err != nil || fuente.lecturas != 2 {
			t.Fatal("la siguiente lectura vuelve a la fuente")
		}
	})
	t.Run("escritura_rechazada_no_invalida", func(t *testing.T) {
		antes := cache.invalidadas
		fuente.falla = domain.ErrValidacion
		_, _ = s.CrearActividad(ctx, administrador, "Yoga", 20)
		fuente.falla = nil
		if _, err := s.CrearActividad(ctx, Actor{ID: alumno, Rol: RolAlumno}, "Yoga", 20); !errors.Is(err, domain.ErrSinPermiso) {
			t.Fatal("un alumno no crea actividades")
		}
		if cache.invalidadas != antes {
			t.Fatal("sin cambio confirmado no se invalida")
		}
	})
	t.Run("ADR-007_redis_caido_lee_la_fuente", func(t *testing.T) {
		cache.caida = true
		antes := fuente.lecturas
		items, err := s.Actividades(ctx)
		if err != nil || len(items) != 1 || fuente.lecturas != antes+1 {
			t.Fatalf("con Redis caído responde desde PostgreSQL: %v", err)
		}
		nombre := "Zumba"
		if _, err := s.ModificarActividad(ctx, administrador, actID, &nombre, nil, nil); err != nil {
			t.Fatal("una caché caída no impide escribir")
		}
	})
	t.Run("fuente_caida_no_se_oculta", func(t *testing.T) {
		cache.caida, cache.hay = false, false
		fuente.falla = errors.New("base caída")
		if _, err := s.Actividades(ctx); err == nil {
			t.Fatal("sin copia y sin base la lectura falla")
		}
		fuente.falla = nil
	})
}
