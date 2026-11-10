// Etapa 4 de la entrega final — invariantes de booking contra PostgreSQL real
// (testcontainers): RN-08, RN-09, RN-13, RN-14, RN-15, RN-17, RN-21, RN-38,
// RN-39, RN-40, CA-07.4, CA-07.5, CA-08.1/3, CA-14.6 a CA-14.12, CA-15.1/2,
// CA-22.1/6/8 y SPEC §10 (concurrencia).
package postgres

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

func uuidN(n int) string { return fmt.Sprintf("00000000-0000-4000-8000-%012d", n) }

func nuevaBase(t *testing.T) *DB {
	t.Helper()
	requireDocker(t)
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("booking_db"), tcpostgres.WithUsername("booking_user"), tcpostgres.WithPassword("fixture"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	t.Cleanup(func() {
		if ctr != nil {
			_ = ctr.Terminate(context.Background())
		}
	})
	require.NoError(t, err)
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := Open(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Migrate(ctx), "las migraciones son idempotentes")
	return db
}

func actividadPorNombre(t *testing.T, db *DB, nombre string) domain.Actividad {
	t.Helper()
	items, err := db.Actividades(context.Background())
	require.NoError(t, err)
	for _, a := range items {
		if a.Nombre == nombre {
			return a
		}
	}
	t.Fatalf("no existe la actividad %s", nombre)
	return domain.Actividad{}
}

// clase crea una clase concreta de la actividad, con el inicio indicado.
func clase(t *testing.T, db *DB, a domain.Actividad, profesor string, inicio time.Time, minutos, capacidad int) domain.Clase {
	t.Helper()
	ctx := context.Background()
	c := domain.Clase{ActividadID: a.ID, Inicio: inicio.UTC(), Fin: inicio.Add(time.Duration(minutos) * time.Minute).UTC(), ProfesorID: profesor, ProfesorNombre: "Lucía Gómez", Capacidad: capacidad}
	n, err := db.GuardarClases(ctx, uuidN(1), []domain.Clase{c})
	require.NoError(t, err)
	require.Equal(t, 1, n)
	page, err := db.BuscarClases(ctx, domain.FiltroClases{Desde: c.Inicio, Hasta: c.Inicio.Add(time.Second), ActividadID: a.ID, Pagina: 1, Tamano: 10}, "")
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	return page.Items[0]
}

func op(actor, key string) ports.Operacion {
	return ports.Operacion{Actor: actor, Clave: key, Tipo: "RESERVAR", Huella: fmt.Sprintf("%064d", len(key))}
}

func validarAhora(c domain.Clase) error { return c.ValidarReserva(time.Now()) }

func reservar(db *DB, alumno, key, claseID string) (domain.Reserva, bool, error) {
	return db.Reservar(context.Background(), op(alumno, key), claseID, validarAhora)
}

func cancelar(db *DB, alumno, key, reservaID string) (domain.Reserva, bool, error) {
	o := ports.Operacion{Actor: alumno, Clave: key, Tipo: "CANCELAR_RESERVA", Huella: fmt.Sprintf("%064d", 7)}
	return db.CancelarReserva(context.Background(), o, reservaID, domain.MotivoCancelacionAlumno, func(r domain.Reserva) (bool, error) {
		return r.ValidarCancelacion(time.Now())
	})
}

func TestBookingPostgres(t *testing.T) {
	db := nuevaBase(t)
	ctx := context.Background()
	zumba := actividadPorNombre(t, db, "Zumba")
	funcional := actividadPorNombre(t, db, "Funcional")
	musc := actividadPorNombre(t, db, "Musculación")
	manana := time.Now().In(domain.Zona).AddDate(0, 0, 2)
	at := func(h, m int) time.Time {
		return time.Date(manana.Year(), manana.Month(), manana.Day(), h, m, 0, 0, domain.Zona)
	}

	t.Run("CA06_1_catalogo_inicial", func(t *testing.T) {
		items, err := db.Actividades(ctx)
		require.NoError(t, err)
		require.Len(t, items, 5)
		require.True(t, musc.Musculacion)
		require.Equal(t, 50, musc.CapacidadMaxima)
	})

	t.Run("CA07_4_CA07_5_horarios_y_generacion", func(t *testing.T) {
		// 21:00 no coincide con las clases que crean los demás subtests.
		_, err := db.CrearHorario(ctx, uuidN(1), domain.Horario{ActividadID: zumba.ID, DiaSemana: 2, InicioMinutos: 21 * 60, DuracionMinutos: 60, ProfesorID: uuidN(900), ProfesorNombre: "P", Capacidad: 30})
		require.NoError(t, err)
		_, err = db.CrearHorario(ctx, uuidN(1), domain.Horario{ActividadID: funcional.ID, DiaSemana: 2, InicioMinutos: 21*60 + 30, DuracionMinutos: 60, ProfesorID: uuidN(900), ProfesorNombre: "P", Capacidad: 30})
		require.ErrorIs(t, err, domain.ErrSuperposicionProfesor)
		horarios, err := db.Horarios(ctx, zumba.ID)
		require.NoError(t, err)
		clases, err := domain.GenerarClases(zumba, horarios, manana.Format(time.DateOnly), manana.AddDate(0, 0, 27).Format(time.DateOnly), time.Now())
		require.NoError(t, err)
		require.Len(t, clases, 4)
		n, err := db.GuardarClases(ctx, uuidN(1), clases)
		require.NoError(t, err)
		require.Equal(t, 4, n)
		n, err = db.GuardarClases(ctx, uuidN(1), clases)
		require.NoError(t, err)
		require.Equal(t, 0, n, "regenerar el mismo período no duplica")
	})

	t.Run("CA14_8_RN14_concurrencia_ultimo_lugar", func(t *testing.T) {
		c := clase(t, db, funcional, uuidN(901), at(9, 0), 60, 1)
		var wg sync.WaitGroup
		var mu sync.Mutex
		ok, sinCupo := 0, 0
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				_, _, err := reservar(db, uuidN(100+i), "concurrente-"+uuidN(i), c.ID)
				mu.Lock()
				defer mu.Unlock()
				switch {
				case err == nil:
					ok++
				case errors.Is(err, domain.ErrSinCupo):
					sinCupo++
				default:
					t.Errorf("error inesperado: %v", err)
				}
			}(i)
		}
		wg.Wait()
		require.Equal(t, 1, ok, "exactamente uno obtiene el lugar")
		require.Equal(t, 9, sinCupo)
		actual, err := db.Clase(ctx, c.ID, "")
		require.NoError(t, err)
		require.Equal(t, 1, actual.Ocupacion, "la ocupación nunca supera la capacidad")
	})

	alumno := uuidN(200)
	zumbaClase := clase(t, db, zumba, uuidN(902), at(19, 0), 60, 30)

	t.Run("CA14_1_CA14_6_CA14_7_reserva_duplicado_e_idempotencia", func(t *testing.T) {
		r, replay, err := reservar(db, alumno, "reserva-zumba-1", zumbaClase.ID)
		require.NoError(t, err)
		require.False(t, replay)
		require.Equal(t, domain.ReservaConfirmada, r.Estado)
		require.Equal(t, 1, r.Clase.Ocupacion)
		again, replay, err := reservar(db, alumno, "reserva-zumba-1", zumbaClase.ID)
		require.NoError(t, err)
		require.True(t, replay, "misma clave: misma reserva sin efectos")
		require.Equal(t, r.ID, again.ID)
		_, _, err = reservar(db, alumno, "reserva-zumba-2", zumbaClase.ID)
		require.ErrorIs(t, err, domain.ErrReservaDuplicada)
		o := op(alumno, "reserva-zumba-1")
		o.Huella = fmt.Sprintf("%064d", 99)
		_, _, err = db.Reservar(ctx, o, zumbaClase.ID, validarAhora)
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
	})

	t.Run("CA14_11_CA14_12_superposicion", func(t *testing.T) {
		pisa := clase(t, db, funcional, uuidN(903), at(19, 30), 60, 30)
		_, _, err := reservar(db, alumno, "superpuesta-1", pisa.ID)
		require.ErrorIs(t, err, domain.ErrSuperposicionReserva)
		musc1820 := clase(t, db, musc, uuidN(904), at(18, 0), 120, 50)
		_, _, err = reservar(db, alumno, "superpuesta-2", musc1820.ID)
		require.ErrorIs(t, err, domain.ErrSuperposicionReserva)
		gap := clase(t, db, actividadPorNombre(t, db, "GAP"), uuidN(905), at(20, 0), 60, 30)
		_, _, err = reservar(db, alumno, "consecutiva-1", gap.ID)
		require.NoError(t, err, "una clase que empieza a las 20:00 no pisa a una que termina a las 20:00")
	})

	t.Run("CA14_9_CA14_10_un_turno_de_musculacion_por_dia", func(t *testing.T) {
		otro := uuidN(201)
		t08 := clase(t, db, musc, uuidN(906), at(8, 0), 120, 50)
		t16 := clase(t, db, musc, uuidN(907), at(16, 0), 120, 50)
		r, _, err := reservar(db, otro, "musc-08", t08.ID)
		require.NoError(t, err)
		_, _, err = reservar(db, otro, "musc-16", t16.ID)
		require.ErrorIs(t, err, domain.ErrTurnoMusculacionDiario)
		_, _, err = cancelar(db, otro, "cancela-08", r.ID)
		require.NoError(t, err)
		_, _, err = reservar(db, otro, "musc-16-bis", t16.ID)
		require.NoError(t, err, "cancelado el turno, puede reservar otro del mismo día")
	})

	t.Run("CA15_1_CA15_2_cancelacion_libera_cupo_una_vez", func(t *testing.T) {
		tercero := uuidN(202)
		c := clase(t, db, zumba, uuidN(908), at(10, 0), 60, 30)
		r, _, err := reservar(db, tercero, "cancelable", c.ID)
		require.NoError(t, err)
		cancelada, _, err := cancelar(db, tercero, "cancelar-1", r.ID)
		require.NoError(t, err)
		require.Equal(t, domain.ReservaCancelada, cancelada.Estado)
		otra, _, err := cancelar(db, tercero, "cancelar-2", r.ID)
		require.NoError(t, err)
		require.Equal(t, domain.ReservaCancelada, otra.Estado)
		actual, err := db.Clase(ctx, c.ID, "")
		require.NoError(t, err)
		require.Equal(t, 0, actual.Ocupacion, "el cupo se libera una sola vez")
		_, _, err = reservar(db, tercero, "nueva-tras-cancelar", c.ID)
		require.NoError(t, err, "CA-15.5: puede volver a reservar")
	})

	t.Run("CA08_1_CA08_3_capacidad_y_cancelacion_de_clase", func(t *testing.T) {
		c := clase(t, db, zumba, uuidN(909), at(12, 0), 60, 30)
		for i := 0; i < 3; i++ {
			_, _, err := reservar(db, uuidN(300+i), "cap-"+uuidN(i), c.ID)
			require.NoError(t, err)
		}
		_, err := db.CambiarCapacidad(ctx, uuidN(1), c.ID, 2, func(cl domain.Clase, a domain.Actividad) error {
			return cl.ValidarCapacidad(2, a.CapacidadMaxima, time.Now())
		})
		require.ErrorIs(t, err, domain.ErrValidacion)
		cancelada, err := db.CancelarClase(ctx, uuidN(1), c.ID, "Profesor enfermo", func(cl domain.Clase) error { return cl.ValidarCancelacion(time.Now()) })
		require.NoError(t, err)
		require.Equal(t, domain.ClaseCancelada, cancelada.Estado)
		reservas, err := db.ReservasDeClase(ctx, c.ID)
		require.NoError(t, err)
		require.Empty(t, reservas, "todas las reservas activas pasaron a CANCELADA")
	})

	t.Run("CA22_1_CA22_6_CA22_8_ingreso_y_ausencias", func(t *testing.T) {
		asiste, falta := uuidN(400), uuidN(401)
		c := clase(t, db, funcional, uuidN(910), at(15, 0), 60, 30)
		_, _, err := reservar(db, asiste, "ingreso-1", c.ID)
		require.NoError(t, err)
		_, _, err = reservar(db, falta, "ingreso-2", c.ID)
		require.NoError(t, err)
		ingresar := func(key string, now time.Time) (domain.Ingreso, error) {
			o := ports.Operacion{Actor: uuidN(1), Clave: key, Tipo: "INGRESO", Huella: fmt.Sprintf("%064d", 1)}
			in, _, err := db.RegistrarIngreso(ctx, o, asiste, now, func(rs []domain.Reserva) (domain.Ingreso, error) {
				r, re, err := domain.ElegirReservaParaIngreso(rs, now)
				return domain.Ingreso{Reserva: r, Reingreso: re, Nombre: "Martina"}, err
			})
			return in, err
		}
		_, err = ingresar("ingreso-temprano", c.Inicio.Add(-20*time.Minute))
		require.ErrorIs(t, err, domain.ErrSinReservaParaHorario, "antes de la ventana no pasa")
		in, err := ingresar("ingreso-ok", c.Inicio.Add(5*time.Minute))
		require.NoError(t, err)
		require.False(t, in.Reingreso)
		require.Equal(t, domain.ReservaAsistida, in.Reserva.Estado)
		in, err = ingresar("ingreso-de-nuevo", c.Inicio.Add(20*time.Minute))
		require.NoError(t, err)
		require.True(t, in.Reingreso, "D-38: reingreso sin puntos")
		var asistencias, eventos int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM asistencias WHERE alumno_id=$1`, asiste).Scan(&asistencias))
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE event_type='asistencia.registrada'`).Scan(&eventos))
		require.Equal(t, 1, asistencias, "RN-21: una asistencia por reserva")
		require.Equal(t, 1, eventos)

		clases, ausentes, err := db.CerrarClases(ctx, c.Fin.Add(time.Minute))
		require.NoError(t, err)
		require.GreaterOrEqual(t, clases, 1)
		require.GreaterOrEqual(t, ausentes, 1)
		reservas, err := db.ReservasDeClase(ctx, c.ID)
		require.NoError(t, err)
		estados := map[string]string{}
		for _, r := range reservas {
			estados[r.AlumnoID] = r.Estado
		}
		require.Equal(t, domain.ReservaAsistida, estados[asiste])
		require.Equal(t, domain.ReservaAusente, estados[falta])
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE event_type='inasistencia.registrada' AND data->>'alumno_id'=$1`, falta).Scan(&eventos))
		require.Equal(t, 1, eventos)
		_, _, err = db.CerrarClases(ctx, c.Fin.Add(2*time.Minute))
		require.NoError(t, err)
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM outbox WHERE event_type='inasistencia.registrada' AND data->>'alumno_id'=$1`, falta).Scan(&eventos))
		require.Equal(t, 1, eventos, "RN-38: la ausencia se registra una sola vez")
	})

	t.Run("RN13_la_base_rechaza_duplicados_aunque_falle_el_codigo", func(t *testing.T) {
		var claseID string
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT clase_id::text FROM reservas WHERE alumno_id=$1 AND estado='CONFIRMADA' LIMIT 1`, alumno).Scan(&claseID))
		_, err := db.pool.Exec(ctx, `INSERT INTO reservas(clase_id,alumno_id,estado,inicio,fin,creada_en,actualizada_en) SELECT id,$2,'CONFIRMADA',inicio,fin,now(),now() FROM clases WHERE id=$1`, claseID, alumno)
		require.Error(t, err)
		// Lo rechaza el índice único o la exclusión de superposición, según cuál evalúe primero.
		traducido := traducir(err)
		require.True(t, errors.Is(traducido, domain.ErrReservaDuplicada) || errors.Is(traducido, domain.ErrSuperposicionReserva), traducido)
	})
}
