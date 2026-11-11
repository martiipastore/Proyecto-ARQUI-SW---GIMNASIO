package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// Etapa 13 (D-92): el caso de uso entrega los envíos vencidos y conserva la
// referencia; las fallas de MELT o de Members solo reprograman el envío.
type repoMelt struct {
	envios   []domain.EnvioMelt
	guardado map[string]domain.EnvioMelt
}

func (r *repoMelt) EnviosMeltVencidos(context.Context, time.Time, int) ([]domain.EnvioMelt, error) {
	return r.envios, nil
}
func (r *repoMelt) GuardarEnvioMelt(_ context.Context, e domain.EnvioMelt) error {
	r.guardado[e.ID] = e
	return nil
}
func (r *repoMelt) EnviosMeltDeAlumno(context.Context, string) ([]domain.EnvioMelt, error) {
	return r.envios, nil
}

type proveedorMelt struct {
	r       domain.ResultadoMelt
	err     error
	llamado []string
}

func (p *proveedorMelt) Acreditar(_ context.Context, email string, puntos int64, ref, _ string) (domain.ResultadoMelt, error) {
	p.llamado = append(p.llamado, email+"|"+ref)
	return p.r, p.err
}

type contactos struct{ err error }

func (c contactos) EmailDe(context.Context, string) (string, error) { return "alumno@gym.local", c.err }

type reloj struct{ t time.Time }

func (r reloj) Now() time.Time { return r.t }

func TestProcesarEnviosMelt(t *testing.T) {
	ahora := time.Date(2026, 11, 11, 16, 0, 0, 0, time.UTC)
	nuevo := func() *repoMelt {
		return &repoMelt{envios: []domain.EnvioMelt{{ID: "e1", AlumnoID: "a", Referencia: "metalfitness-asistencia-r1", Puntos: 100, Estado: domain.EnvioPendiente, ProximoIntento: ahora}}, guardado: map[string]domain.EnvioMelt{}}
	}
	cases := []struct {
		nombre string
		prov   *proveedorMelt
		cont   contactos
		estado string
		llama  bool
	}{
		{"acreditado", &proveedorMelt{r: domain.ResultadoMelt{Tipo: domain.ResultadoAcreditado}}, contactos{}, domain.EnvioAcreditado, true},
		{"pendiente de registro", &proveedorMelt{r: domain.ResultadoMelt{Tipo: domain.ResultadoPendiente, RegistroURL: "https://x/#t", RegistroVence: ahora.Add(time.Hour)}}, contactos{}, domain.EnvioPendienteRegistro, true},
		{"MELT caído", &proveedorMelt{err: errors.New("timeout")}, contactos{}, domain.EnvioPendiente, true},
		{"Members caído", &proveedorMelt{}, contactos{err: errors.New("caído")}, domain.EnvioPendiente, false},
		{"alumno dado de baja", &proveedorMelt{}, contactos{err: domain.ErrCuentaNoVinculada}, domain.EnvioRechazado, false},
	}
	for _, c := range cases {
		t.Run(c.nombre, func(t *testing.T) {
			repo := nuevo()
			n, err := NewEnviosMelt(repo, c.prov, c.cont, reloj{ahora}).Procesar(context.Background(), 20)
			if err != nil || n != 1 {
				t.Fatalf("n=%d err=%v", n, err)
			}
			e := repo.guardado["e1"]
			if e.Estado != c.estado || e.Intentos != 1 || e.Referencia != "metalfitness-asistencia-r1" {
				t.Fatalf("envío guardado: %+v", e)
			}
			if c.llama != (len(c.prov.llamado) == 1) {
				t.Fatalf("llamadas a MELT: %v", c.prov.llamado)
			}
			if c.llama && c.prov.llamado[0] != "alumno@gym.local|metalfitness-asistencia-r1" {
				t.Fatalf("solicitud: %v", c.prov.llamado)
			}
			if c.estado == domain.EnvioPendiente && !e.ProximoIntento.After(ahora) {
				t.Fatal("una falla transitoria reprograma el envío")
			}
		})
	}
}

func TestMisEnviosValidaAlumno(t *testing.T) {
	s := NewEnviosMelt(&repoMelt{}, &proveedorMelt{}, contactos{}, reloj{})
	if _, err := s.MisEnvios(context.Background(), "no-uuid"); !errors.Is(err, domain.ErrOperacionInvalida) {
		t.Fatalf("err=%v", err)
	}
}
