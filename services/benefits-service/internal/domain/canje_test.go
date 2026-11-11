package domain

import (
	"errors"
	"testing"
	"time"
)

// Cuponera interna (D-94): HU-09, HU-20, RN-23, RN-24 y RN-25.

const (
	canjeID1 = "11111111-1111-4111-8111-111111111111"
	canjeID2 = "22222222-2222-4222-8222-222222222222"
)

// 12/11/2026 15:00 UTC = 12:00 en Buenos Aires.
var ahoraCanje = time.Date(2026, 11, 12, 15, 0, 0, 0, time.UTC)

func beneficio(costo int64) Beneficio {
	return Beneficio{ID: "b1", Nombre: "Botella", Costo: costo, VigenteDesde: "2026-11-01", Activo: true}
}

func cuenta(t *testing.T, saldo int64) CuentaBeneficios {
	t.Helper()
	c, err := NuevaCuenta("cuenta", saldo, 3)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// CA-20.1: saldo 50 y beneficio de 30 → canje CONFIRMADO, movimiento −30, saldo 20.
func TestCanjearDebitaYCreaCupon(t *testing.T) {
	c := cuenta(t, 50)
	nueva, mov, canje, err := c.Canjear(beneficio(30), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
	if err != nil {
		t.Fatal(err)
	}
	if nueva.Saldo() != 20 || nueva.Secuencia() != 4 {
		t.Fatalf("saldo %d secuencia %d", nueva.Saldo(), nueva.Secuencia())
	}
	m := mov.Datos()
	if m.Tipo != Canje || m.Puntos != -30 || m.OrigenID != canjeID1 || m.Referencia != "ABCD-EF23" || m.Secuencia != 4 {
		t.Fatalf("movimiento inesperado: %+v", m)
	}
	if canje.Estado != CanjeConfirmado || canje.Puntos != 30 || canje.MovimientoID != "mov" || canje.BeneficioNombre != "Botella" {
		t.Fatalf("canje inesperado: %+v", canje)
	}
	if c.Saldo() != 50 {
		t.Fatal("la cuenta original no debe cambiar (RN-22)")
	}
}

// CA-20.2 / CL-07: saldo 20 y beneficio de 30 → saldo insuficiente, sin cambios.
func TestCanjearConSaldoInsuficiente(t *testing.T) {
	c := cuenta(t, 20)
	nueva, _, _, err := c.Canjear(beneficio(30), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
	if !errors.Is(err, ErrSaldoInsuficiente) || nueva.Saldo() != 20 {
		t.Fatalf("err %v saldo %d", err, nueva.Saldo())
	}
	// Saldo justo: queda en cero y nunca negativo (RN-23).
	nueva, _, _, err = c.Canjear(beneficio(20), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
	if err != nil || nueva.Saldo() != 0 {
		t.Fatalf("err %v saldo %d", err, nueva.Saldo())
	}
}

// CA-20.3: un beneficio inactivo, vencido o que todavía no empezó no se canjea.
func TestCanjearBeneficioNoDisponible(t *testing.T) {
	casos := map[string]Beneficio{
		"inactivo":       {Nombre: "Botella", Costo: 10, VigenteDesde: "2026-11-01"},
		"vencido ayer":   {Nombre: "Botella", Costo: 10, VigenteDesde: "2026-11-01", VigenteHasta: "2026-11-11", Activo: true},
		"empieza mañana": {Nombre: "Botella", Costo: 10, VigenteDesde: "2026-11-13", Activo: true},
	}
	for nombre, b := range casos {
		t.Run(nombre, func(t *testing.T) {
			_, _, _, err := cuenta(t, 100).Canjear(b, "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
			if !errors.Is(err, ErrBeneficioNoDisponible) {
				t.Fatalf("err %v", err)
			}
		})
	}
	// El último día de vigencia todavía se canjea (vigencia inclusiva, hora de Argentina).
	b := Beneficio{Nombre: "Botella", Costo: 10, VigenteDesde: "2026-11-12", VigenteHasta: "2026-11-12", Activo: true}
	if _, _, _, err := cuenta(t, 100).Canjear(b, "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje); err != nil {
		t.Fatalf("último día vigente: %v", err)
	}
	// 12/11 a las 02:00 UTC todavía es 11/11 en Argentina.
	if b.Disponible(DiaNegocio(time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC))) {
		t.Fatal("el día de negocio se calcula en America/Argentina/Buenos_Aires")
	}
}

// CA-09.5: sin stock, un beneficio disponible se canjea las veces que alcance el saldo.
func TestCanjearSinStock(t *testing.T) {
	c := cuenta(t, 100)
	for i := 0; i < 3; i++ {
		var err error
		c, _, _, err = c.Canjear(beneficio(30), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
		if err != nil {
			t.Fatalf("canje %d: %v", i+1, err)
		}
	}
	if c.Saldo() != 10 {
		t.Fatalf("saldo %d", c.Saldo())
	}
}

func TestUsarCanje(t *testing.T) {
	k := CanjeBeneficio{Estado: CanjeConfirmado}
	usado, cambio, err := k.Usar(ahoraCanje)
	if err != nil || !cambio || usado.Estado != CanjeUsado || usado.UsadoEn == nil {
		t.Fatalf("%+v %v %v", usado, cambio, err)
	}
	// Repetir sobre uno USADO no tiene efectos.
	if again, cambio, err := usado.Usar(ahoraCanje.Add(time.Hour)); err != nil || cambio || !again.UsadoEn.Equal(ahoraCanje) {
		t.Fatalf("repetir: %v %v", cambio, err)
	}
	if _, _, err := (CanjeBeneficio{Estado: CanjeAnulado}).Usar(ahoraCanje); !errors.Is(err, ErrTransicionCanje) {
		t.Fatalf("un canje anulado no se usa: %v", err)
	}
}

// RN-25: anular un canje revierte su débito con un compensatorio.
func TestAnularCanjeDevuelvePuntos(t *testing.T) {
	c := cuenta(t, 50)
	c, mov, k, err := c.Canjear(beneficio(30), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
	if err != nil {
		t.Fatal(err)
	}
	nueva, comp, anulado, err := c.AnularCanje(k, mov.Datos(), canjeID2, "Error de recepción", ahoraCanje)
	if err != nil {
		t.Fatal(err)
	}
	m := comp.Datos()
	if nueva.Saldo() != 50 || m.Tipo != Reversion || m.Puntos != 30 || m.RevierteID != "mov" {
		t.Fatalf("saldo %d movimiento %+v", nueva.Saldo(), m)
	}
	if anulado.Estado != CanjeAnulado || anulado.AnulacionID != canjeID2 || anulado.MotivoAnulacion != "Error de recepción" {
		t.Fatalf("canje %+v", anulado)
	}
	for _, estado := range []string{CanjeUsado, CanjeAnulado} {
		k.Estado = estado
		if _, _, _, err := c.AnularCanje(k, mov.Datos(), canjeID2, "Otro motivo", ahoraCanje); !errors.Is(err, ErrTransicionCanje) {
			t.Fatalf("anular %s: %v", estado, err)
		}
	}
}

// La reversión genérica no alcanza a un débito de canje: se anula el cupón.
func TestRevertirCanjeSoloPorAnulacion(t *testing.T) {
	c := cuenta(t, 50)
	c, mov, _, err := c.Canjear(beneficio(30), "alumno", "mov", canjeID1, "ABCD-EF23", ahoraCanje)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Revertir(mov.Datos(), false, "rev", "Corrección", ahoraCanje); !errors.Is(err, ErrCanjeSoloPorAnulacion) {
		t.Fatalf("err %v", err)
	}
}

func TestValidarBeneficio(t *testing.T) {
	ok := Beneficio{Nombre: "Botella", Costo: 25, VigenteDesde: "2026-11-01", VigenteHasta: "2026-12-31"}
	if err := ok.Validar(); err != nil {
		t.Fatal(err)
	}
	invalidos := map[string]func(*Beneficio){
		"nombre corto":      func(b *Beneficio) { b.Nombre = "ab" },
		"costo cero":        func(b *Beneficio) { b.Costo = 0 },
		"costo excesivo":    func(b *Beneficio) { b.Costo = CostoMaximoBeneficio + 1 },
		"fecha inválida":    func(b *Beneficio) { b.VigenteDesde = "2026-13-01" },
		"hasta antes":       func(b *Beneficio) { b.VigenteHasta = "2026-10-31" },
		"descripción larga": func(b *Beneficio) { b.Descripcion = string(make([]rune, 501)) },
	}
	for nombre, cambiar := range invalidos {
		b := ok
		cambiar(&b)
		if err := b.Validar(); !errors.Is(err, ErrOperacionInvalida) {
			t.Errorf("%s: %v", nombre, err)
		}
	}
}

func TestCodigoCanje(t *testing.T) {
	codigo := CodigoDesdeBytes([8]byte{0, 1, 2, 3, 250, 251, 252, 253})
	if !CodigoCanjeValido(codigo) {
		t.Fatalf("código generado inválido: %s", codigo)
	}
	if NormalizarCodigo(" abcdef23 ") != "ABCD-EF23" {
		t.Fatal("normalizar sin guion")
	}
	for _, malo := range []string{"ABCD-EF2", "ABCDEF234", "ABCD-EFO1", "abcd-ef23"} {
		if CodigoCanjeValido(malo) {
			t.Errorf("%s no debería ser válido", malo)
		}
	}
}
