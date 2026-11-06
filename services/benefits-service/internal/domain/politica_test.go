package domain

import (
	"errors"
	"strings"
	"testing"
)

// Club §5/6: presupuesto y permisos independientes de la membresía.
func TestPoliticaAcreditacion(t *testing.T) {
	diario := int64(150)
	for _, caso := range []struct {
		nombre   string
		politica PoliticaAcreditacion
		puntos   int64
		esperado error
	}{
		{"permiso_revocado", PoliticaAcreditacion{}, 0, ErrPartnerNoAutorizado},
		{"limite_exacto", PoliticaAcreditacion{true, 100, &diario, 50}, 100, nil},
		{"operacion_excesiva", PoliticaAcreditacion{true, 100, nil, 0}, 101, ErrOperacionInvalida},
		{"presupuesto_excedido", PoliticaAcreditacion{true, 100, &diario, 51}, 100, ErrOperacionInvalida},
		{"replay_presupuesto_agotado", PoliticaAcreditacion{true, 100, &diario, 150}, 0, nil},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			if err := caso.politica.Validar(caso.puntos); !errors.Is(err, caso.esperado) {
				t.Fatalf("error=%v, esperado=%v", err, caso.esperado)
			}
		})
	}
}

func TestEventoSegunIntegracion(t *testing.T) {
	for _, caso := range []struct {
		nombre    string
		requerido bool
		evento    string
		esperado  error
	}{
		{"cliente_previo_sin_evento", false, "", nil},
		{"integracion_nueva_sin_evento", true, "", ErrOperacionInvalida},
		{"evento_valido", true, "resumenes-0001", nil},
		{"evento_demasiado_largo", false, strings.Repeat("a", 129), ErrOperacionInvalida},
		{"evento_con_nulo", true, "evento\x00", ErrOperacionInvalida},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			if err := ValidarEvento(caso.requerido, caso.evento); !errors.Is(err, caso.esperado) {
				t.Fatalf("error=%v, esperado=%v", err, caso.esperado)
			}
		})
	}
}
