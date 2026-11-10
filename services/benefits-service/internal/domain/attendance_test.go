package domain

import (
	"math"
	"testing"
	"time"
)

// RN-20/38/42: descuento acotado, saldo cero, orden y límites de entero.
func TestHechosBooking(t *testing.T) {
	h := HechoBooking{EventoID: "00000000-0000-4000-8000-000000000001", Tipo: PenalizacionInasistencia, AlumnoID: "00000000-0000-4000-8000-000000000002", OrigenID: "00000000-0000-4000-8000-000000000003", ReservaID: "00000000-0000-4000-8000-000000000003", ClaseID: "00000000-0000-4000-8000-000000000004", OcurridoEn: time.Now()}
	for _, balance := range []int64{0, 50, 100, 500} {
		c, _ := NuevaCuenta("cuenta", balance, 0)
		next, m, err := c.AplicarBooking(h, "mov", time.Now())
		if err != nil || next.Saldo() != max(int64(0), balance-100) || m.Datos().Puntos != -min(int64(100), balance) || next.Secuencia() != 1 {
			t.Fatalf("penalización inválida desde %d: %v", balance, err)
		}
	}
	c, _ := NuevaCuenta("cuenta", 0, 0)
	c, _, _ = c.AplicarBooking(h, "pen", time.Now())
	h.Tipo = Asistencia
	c, _, err := c.AplicarBooking(h, "asist", time.Now())
	if err != nil || c.Saldo() != 500 {
		t.Fatal("penalización cero generó deuda")
	}
	c, _ = NuevaCuenta("cuenta", 0, 0)
	c, _, _ = c.AplicarBooking(h, "asist", time.Now())
	h.Tipo = PenalizacionInasistencia
	c, _, err = c.AplicarBooking(h, "pen", time.Now())
	if err != nil || c.Saldo() != 400 {
		t.Fatal("orden de aplicación incorrecto")
	}
	c, _ = NuevaCuenta("cuenta", math.MaxInt64, 0)
	h.Tipo = Asistencia
	if _, _, err = c.AplicarBooking(h, "overflow", time.Now()); err == nil {
		t.Fatal("permite overflow")
	}
	h.EventoID = "inválido"
	if _, _, err = c.AplicarBooking(h, "invalido", time.Now()); err == nil {
		t.Fatal("acepta origen inválido")
	}
}
