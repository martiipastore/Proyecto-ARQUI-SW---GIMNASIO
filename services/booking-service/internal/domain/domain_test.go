// Etapa 4 de la entrega final — reglas de dominio de booking: RN-07 a RN-12,
// RN-17, RN-39, RN-40, CA-06.2, CA-07.1 a CA-07.5, CA-08.1 a CA-08.4,
// CA-13.1 a CA-13.3, CA-15.1 a CA-15.3 y D-37 a D-39.
package domain

import (
	"errors"
	"testing"
	"time"
)

func hora(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.ParseInLocation("2006-01-02 15:04", s, Zona)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCA06_2TopesDeCapacidad(t *testing.T) {
	casos := []struct {
		a  Actividad
		ok bool
	}{
		{Actividad{Nombre: "Musculación", CapacidadMaxima: 50, Musculacion: true}, true},
		{Actividad{Nombre: "Musculación", CapacidadMaxima: 51, Musculacion: true}, false},
		{Actividad{Nombre: "Zumba", CapacidadMaxima: 30}, true},
		{Actividad{Nombre: "Zumba", CapacidadMaxima: 31}, false},
		{Actividad{Nombre: "Zumba", CapacidadMaxima: 0}, false},
		{Actividad{Nombre: "   ", CapacidadMaxima: 10}, false},
	}
	for _, c := range casos {
		if err := c.a.Validar(); (err == nil) != c.ok {
			t.Errorf("%+v: error %v, se esperaba ok=%v", c.a, err, c.ok)
		}
	}
}

func TestCA07_3FranjasDeMusculacion(t *testing.T) {
	musc := Actividad{ID: "m", Nombre: "Musculación", CapacidadMaxima: 50, Musculacion: true, Activa: true}
	base := Horario{ActividadID: "m", DiaSemana: 1, DuracionMinutos: 120, ProfesorID: "p", Capacidad: 50}
	for _, inicio := range FranjasMusculacion {
		h := base
		h.InicioMinutos = inicio
		if err := ValidarHorario(musc, h); err != nil {
			t.Errorf("la franja %s debe aceptarse: %v", FormatHora(inicio), err)
		}
	}
	for _, h := range []Horario{
		{ActividadID: "m", DiaSemana: 1, InicioMinutos: 22*60 + 30, DuracionMinutos: 60, ProfesorID: "p", Capacidad: 50},
		{ActividadID: "m", DiaSemana: 1, InicioMinutos: 9 * 60, DuracionMinutos: 120, ProfesorID: "p", Capacidad: 50},
		{ActividadID: "m", DiaSemana: 1, InicioMinutos: 8 * 60, DuracionMinutos: 120, ProfesorID: "p", Capacidad: 51},
	} {
		if !errors.Is(ValidarHorario(musc, h), ErrValidacion) {
			t.Errorf("%+v debe rechazarse", h)
		}
	}
}

func TestRN08HorariosSuperpuestos(t *testing.T) {
	zumba := Horario{DiaSemana: 2, InicioMinutos: 19 * 60, DuracionMinutos: 60}
	if !zumba.SeSuperponeCon(Horario{DiaSemana: 2, InicioMinutos: 19*60 + 30, DuracionMinutos: 60}) {
		t.Error("19:00-20:00 y 19:30-20:30 se superponen (CA-07.4)")
	}
	if zumba.SeSuperponeCon(Horario{DiaSemana: 2, InicioMinutos: 20 * 60, DuracionMinutos: 60}) {
		t.Error("los horarios consecutivos no se superponen")
	}
	if zumba.SeSuperponeCon(Horario{DiaSemana: 3, InicioMinutos: 19 * 60, DuracionMinutos: 60}) {
		t.Error("días distintos no se superponen")
	}
}

func TestCA07_1Y07_2GeneracionDeClases(t *testing.T) {
	musc := Actividad{ID: "m", Nombre: "Musculación", CapacidadMaxima: 50, Musculacion: true, Activa: true}
	var horarios []Horario
	for dia := 1; dia <= 7; dia++ {
		for _, f := range FranjasMusculacion {
			horarios = append(horarios, Horario{ActividadID: "m", DiaSemana: dia, InicioMinutos: f, DuracionMinutos: 120, ProfesorID: "p", Capacidad: 50})
		}
	}
	antes := hora(t, "2026-11-01 00:00")
	clases, err := GenerarClases(musc, horarios, "2026-11-02", "2026-11-08", antes)
	if err != nil {
		t.Fatal(err)
	}
	if len(clases) != 49 {
		t.Fatalf("una semana de Musculación son 49 turnos, hubo %d", len(clases))
	}
	if f := clases[0].Franja(); f == nil || *f != "08:00-10:00" {
		t.Errorf("primera franja inesperada: %v", f)
	}
	zumba := Actividad{ID: "z", Nombre: "Zumba", CapacidadMaxima: 30, Activa: true}
	martes := []Horario{{ActividadID: "z", DiaSemana: 2, InicioMinutos: 19 * 60, DuracionMinutos: 60, ProfesorID: "p", Capacidad: 30}}
	clases, err = GenerarClases(zumba, martes, "2026-12-01", "2026-12-31", antes)
	if err != nil {
		t.Fatal(err)
	}
	if len(clases) != 5 {
		t.Fatalf("diciembre de 2026 tiene 5 martes, hubo %d", len(clases))
	}
	for _, c := range clases {
		if c.Inicio.In(Zona).Weekday() != time.Tuesday || c.Inicio.In(Zona).Hour() != 19 || c.Fin.Sub(c.Inicio) != time.Hour {
			t.Errorf("clase mal generada: %v", c.Inicio)
		}
	}
	if _, err = GenerarClases(zumba, martes, "2026-12-31", "2026-12-01", antes); !errors.Is(err, ErrValidacion) {
		t.Error("un período invertido se rechaza")
	}
	if _, err = GenerarClases(zumba, martes, "2026-01-01", "2026-12-31", antes); !errors.Is(err, ErrValidacion) {
		t.Error("un período demasiado largo se rechaza")
	}
	pasadas, _ := GenerarClases(zumba, martes, "2026-12-01", "2026-12-31", hora(t, "2026-12-15 20:00"))
	if len(pasadas) != 2 {
		t.Errorf("solo se generan clases futuras, hubo %d", len(pasadas))
	}
}

func TestEstadosYReservabilidad(t *testing.T) {
	c := Clase{Inicio: hora(t, "2026-11-10 19:00"), Fin: hora(t, "2026-11-10 20:00"), Capacidad: 30, Ocupacion: 21, Estado: ClaseProgramada}
	antes := hora(t, "2026-11-10 18:00")
	if c.Disponibles() != 9 || !c.Reservable(antes) {
		t.Error("CA-13.1: 21/30 ocupados deja 9 disponibles y se puede reservar")
	}
	if c.EstadoEn(hora(t, "2026-11-10 19:00")) != ClaseEnCurso || c.EstadoEn(hora(t, "2026-11-10 20:00")) != ClaseFinalizada {
		t.Error("los estados EN_CURSO y FINALIZADA se derivan del reloj")
	}
	if !errors.Is(c.ValidarReserva(hora(t, "2026-11-10 19:00")), ErrClaseNoFutura) {
		t.Error("CA-14.4: una clase iniciada no se reserva")
	}
	llena := c
	llena.Ocupacion = 30
	if llena.Reservable(antes) || !errors.Is(llena.ValidarReserva(antes), ErrSinCupo) {
		t.Error("CA-13.2/CA-14.5: una clase completa no se reserva")
	}
	cancelada := c
	cancelada.Estado = ClaseCancelada
	if !errors.Is(cancelada.ValidarReserva(antes), ErrClaseCancelada) {
		t.Error("CA-13.3: una clase cancelada no se reserva")
	}
}

func TestCA08Capacidad(t *testing.T) {
	c := Clase{Inicio: hora(t, "2026-11-10 19:00"), Fin: hora(t, "2026-11-10 20:00"), Capacidad: 30, Ocupacion: 21, Estado: ClaseProgramada}
	antes := hora(t, "2026-11-10 10:00")
	if !errors.Is(c.ValidarCapacidad(20, TopeGrupal, antes), ErrValidacion) {
		t.Error("CA-08.1: no se baja la capacidad por debajo de la ocupación")
	}
	if err := c.ValidarCapacidad(25, TopeGrupal, antes); err != nil {
		t.Errorf("CA-08.2: bajar a 25 se acepta: %v", err)
	}
	if !errors.Is(c.ValidarCapacidad(31, TopeGrupal, antes), ErrValidacion) {
		t.Error("RN-10: no se supera el tope de la actividad")
	}
	en := hora(t, "2026-11-10 19:10")
	if !errors.Is(c.ValidarCapacidad(25, TopeGrupal, en), ErrTransicionInvalida) || !errors.Is(c.ValidarCancelacion(en), ErrTransicionInvalida) {
		t.Error("CA-08.4: una clase en curso no se modifica ni se cancela")
	}
}

func TestRN40Superposicion(t *testing.T) {
	zumba := [2]time.Time{hora(t, "2026-11-10 19:00"), hora(t, "2026-11-10 20:00")}
	casos := []struct {
		inicio, fin string
		pisa        bool
	}{
		{"2026-11-10 19:30", "2026-11-10 20:30", true},  // Funcional (CA-14.11)
		{"2026-11-10 18:00", "2026-11-10 20:00", true},  // Musculación (CA-14.11)
		{"2026-11-10 20:00", "2026-11-10 21:00", false}, // GAP (CA-14.12)
		{"2026-11-10 20:00", "2026-11-10 22:00", false}, // Musculación (CA-14.12)
	}
	for _, c := range casos {
		if got := Superponen(zumba[0], zumba[1], hora(t, c.inicio), hora(t, c.fin)); got != c.pisa {
			t.Errorf("%s-%s: superpone=%v, se esperaba %v", c.inicio, c.fin, got, c.pisa)
		}
	}
}

func TestRN17CancelacionDeReserva(t *testing.T) {
	r := Reserva{Estado: ReservaConfirmada, Clase: Clase{Inicio: hora(t, "2026-11-10 19:00")}}
	if sin, err := r.ValidarCancelacion(hora(t, "2026-11-10 17:59")); sin || err != nil {
		t.Errorf("CA-15.1: con más de 1 hora se cancela: %v", err)
	}
	for _, s := range []string{"2026-11-10 18:00", "2026-11-10 18:50", "2026-11-10 19:05"} {
		if _, err := r.ValidarCancelacion(hora(t, s)); !errors.Is(err, ErrPlazoCancelacionVencido) {
			t.Errorf("CA-15.3: a las %s el plazo está vencido", s)
		}
	}
	r.Estado = ReservaCancelada
	if sin, err := r.ValidarCancelacion(hora(t, "2026-11-10 19:30")); !sin || err != nil {
		t.Error("CA-15.2: cancelar una cancelada no tiene efecto")
	}
	r.Estado = ReservaAsistida
	if _, err := r.ValidarCancelacion(hora(t, "2026-11-10 10:00")); !errors.Is(err, ErrTransicionInvalida) {
		t.Error("CU-02 3c: una asistida no se cancela")
	}
}

func TestRN12Vigencia(t *testing.T) {
	v := Vigencia{AlumnoActivo: true, VigenteHoy: true, VigenteEn: map[string]bool{"2026-11-20": true, "2026-12-20": false}}
	if v.ValidarParaReservar("2026-11-20") != nil {
		t.Error("CA-14.1: vigente hoy y en la fecha")
	}
	if !errors.Is(v.ValidarParaReservar("2026-12-20"), ErrMembresiaNoVigente) {
		t.Error("CA-14.3: vence antes de la clase")
	}
	v.VigenteHoy = false
	if !errors.Is(v.ValidarParaReservar("2026-11-20"), ErrMembresiaNoVigente) {
		t.Error("CA-14.2: sin membresía vigente hoy")
	}
	if !errors.Is(Vigencia{}.ValidarParaReservar("2026-11-20"), ErrUsuarioInactivo) {
		t.Error("RN-12 (a): alumno inactivo")
	}
}

func TestD37AD39Ingreso(t *testing.T) {
	musc := Reserva{ID: "m", Estado: ReservaConfirmada, Clase: Clase{Inicio: hora(t, "2026-11-10 08:00"), Fin: hora(t, "2026-11-10 10:00"), Musculacion: true, Estado: ClaseProgramada}}
	zumba := Reserva{ID: "z", Estado: ReservaConfirmada, Clase: Clase{Inicio: hora(t, "2026-11-10 10:00"), Fin: hora(t, "2026-11-10 11:00"), Estado: ClaseProgramada}}
	casos := []struct {
		ahora     string
		id        string
		reingreso bool
		err       error
	}{
		{"2026-11-10 07:44", "", false, ErrSinReservaParaHorario}, // antes de la ventana
		{"2026-11-10 07:45", "m", false, nil},                     // inicio −15 min
		{"2026-11-10 09:30", "m", false, nil},                     // fin −30 min (2 h)
		{"2026-11-10 09:50", "z", false, nil},                     // ejemplo de D-37
		{"2026-11-10 10:31", "", false, ErrSinReservaParaHorario}, // cerrada la de Zumba
	}
	for _, c := range casos {
		r, re, err := ElegirReservaParaIngreso([]Reserva{zumba, musc}, hora(t, c.ahora))
		if !errors.Is(err, c.err) || r.ID != c.id || re != c.reingreso {
			t.Errorf("%s: reserva %q reingreso=%v error=%v", c.ahora, r.ID, re, err)
		}
	}
	asistida := musc
	asistida.Estado = ReservaAsistida
	if r, re, err := ElegirReservaParaIngreso([]Reserva{asistida}, hora(t, "2026-11-10 09:45")); err != nil || !re || r.ID != "m" {
		t.Error("D-38: reingreso antes de que termine la clase, sin puntos")
	}
	if _, _, err := ElegirReservaParaIngreso([]Reserva{asistida}, hora(t, "2026-11-10 10:00")); !errors.Is(err, ErrSinReservaParaHorario) {
		t.Error("D-38: terminada la clase ya no hay reingreso")
	}
}
