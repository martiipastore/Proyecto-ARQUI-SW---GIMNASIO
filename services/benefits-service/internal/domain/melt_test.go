package domain

import (
	"testing"
	"time"
)

// D-92: 100 puntos MELT por asistencia; las ausencias no se envían; la
// referencia es estable para reintentos (contrato MELT 1.1.0).
func TestEnvioMeltDeAsistencia(t *testing.T) {
	ahora := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	h := HechoBooking{EventoID: "11111111-1111-4111-8111-111111111111", Tipo: Asistencia, AlumnoID: "22222222-2222-4222-8222-222222222222",
		OrigenID: "33333333-3333-4333-8333-33333333AAAA", ReservaID: "44444444-4444-4444-8444-444444444444", ClaseID: "55555555-5555-4555-8555-555555555555", OcurridoEn: ahora}
	e, ok := EnvioMeltDeAsistencia(h, "66666666-6666-4666-8666-666666666666", ahora)
	if !ok || e.Puntos != 100 || e.Estado != EnvioPendiente {
		t.Fatalf("envío inesperado: %+v", e)
	}
	if e.Referencia != "metalfitness-asistencia-33333333-3333-4333-8333-33333333aaaa" {
		t.Fatalf("referencia no estable: %s", e.Referencia)
	}
	if !e.RequiereEnvio(ahora) {
		t.Fatal("un envío nuevo se intenta enseguida")
	}
	h.Tipo = PenalizacionInasistencia
	if _, ok := EnvioMeltDeAsistencia(h, "66666666-6666-4666-8666-666666666666", ahora); ok {
		t.Fatal("una ausencia no acredita en MELT")
	}
}

func TestEnvioMeltRegistrarResultados(t *testing.T) {
	ahora := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	base := EnvioMelt{Estado: EnvioPendiente, Referencia: "ref", ProximoIntento: ahora}

	r := base.Registrar(ResultadoMelt{Tipo: ResultadoReintentar, Motivo: "MELT no respondió"}, ahora)
	if r.Estado != EnvioPendiente || r.Intentos != 1 || !r.ProximoIntento.Equal(ahora.Add(10*time.Second)) || r.Referencia != "ref" {
		t.Fatalf("reintento: %+v", r)
	}
	if r.RequiereEnvio(ahora.Add(5 * time.Second)) {
		t.Fatal("respeta la espera")
	}
	r = r.Registrar(ResultadoMelt{Tipo: ResultadoReintentar}, ahora)
	if !r.ProximoIntento.Equal(ahora.Add(20 * time.Second)) {
		t.Fatalf("espera creciente: %v", r.ProximoIntento.Sub(ahora))
	}

	vence := ahora.Add(7 * 24 * time.Hour)
	p := base.Registrar(ResultadoMelt{Tipo: ResultadoPendiente, RegistroURL: "https://melt.example/registro#t", RegistroVence: vence}, ahora)
	if p.Estado != EnvioPendienteRegistro || p.RegistroURL == "" || p.RequiereEnvio(ahora) {
		t.Fatalf("pendiente de registro: %+v", p)
	}
	if p.RequiereEnvio(ahora.Add(time.Minute)) || !p.RequiereEnvio(ahora.Add(ReconsultaRegistroMelt)) {
		t.Fatal("se reconsulta cada 2 minutos para detectar el registro")
	}
	if !p.RequiereEnvio(vence.Add(time.Second)) {
		t.Fatal("un enlace vencido se renueva repitiendo la misma solicitud")
	}

	a := p.Registrar(ResultadoMelt{Tipo: ResultadoAcreditado}, ahora)
	if a.Estado != EnvioAcreditado || a.RegistroURL != "" || a.RequiereEnvio(ahora.Add(time.Hour)) {
		t.Fatalf("acreditado: %+v", a)
	}
	x := base.Registrar(ResultadoMelt{Tipo: ResultadoRechazado, Motivo: "DATOS_INVALIDOS"}, ahora)
	if x.Estado != EnvioRechazado || x.RequiereEnvio(ahora.Add(time.Hour)) {
		t.Fatalf("rechazado no se reintenta: %+v", x)
	}
}

func TestEsperaMeltAcotada(t *testing.T) {
	for intentos, want := range map[int]time.Duration{1: 10 * time.Second, 2: 20 * time.Second, 4: 80 * time.Second, 20: EsperaMaximaMelt} {
		if got := EsperaMelt(intentos); got != want {
			t.Fatalf("intentos %d: %v, quería %v", intentos, got, want)
		}
	}
}
