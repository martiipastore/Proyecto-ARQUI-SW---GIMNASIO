package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/correlation"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/ports"
)

// Claves de los advisory locks de booking (segundo argumento de hashtextextended).
const (
	lockOperacion = 41
	lockAlumno    = 42
)

func previa(ctx context.Context, q consultor, op ports.Operacion, destino any) (bool, error) {
	var tipo, huella string
	var data []byte
	err := q.QueryRow(ctx, `SELECT tipo, huella, resultado FROM operaciones WHERE actor_id=$1 AND clave=$2`, op.Actor, op.Clave).Scan(&tipo, &huella, &data)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if tipo != op.Tipo || huella != op.Huella {
		return false, domain.ErrConflictoIdempotencia
	}
	return true, json.Unmarshal(data, destino)
}

// Previa devuelve el resultado guardado de una operación con la misma clave (RN-15).
func (d *DB) Previa(ctx context.Context, op ports.Operacion, destino any) (bool, error) {
	ok, err := previa(ctx, d.pool, op, destino)
	return ok, traducir(err)
}

// idempotente ejecuta fn una sola vez por actor y clave: serializa la misma clave
// (OPERACION_EN_CURSO si otra solicitud la está procesando), devuelve el resultado
// guardado si ya existía y guarda el nuevo en la misma transacción que el efecto.
func idempotente[T any](ctx context.Context, d *DB, op ports.Operacion, fn func(pgx.Tx) (T, error)) (T, bool, error) {
	var out T
	replay := false
	err := d.tx(ctx, func(tx pgx.Tx) error {
		var locked bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1, $2))`, op.Actor+":"+op.Clave, lockOperacion).Scan(&locked); err != nil {
			return err
		}
		if !locked {
			return domain.ErrOperacionEnCurso
		}
		ok, err := previa(ctx, tx, op, &out)
		if err != nil || ok {
			replay = ok
			return err
		}
		if out, err = fn(tx); err != nil {
			return err
		}
		data, err := json.Marshal(out)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO operaciones(actor_id, clave, tipo, huella, resultado) VALUES($1,$2,$3,$4,$5)`, op.Actor, op.Clave, op.Tipo, op.Huella, data)
		return err
	})
	if err != nil {
		var zero T
		return zero, false, traducir(err)
	}
	return out, replay, nil
}

func bloquearAlumno(ctx context.Context, tx pgx.Tx, alumnoID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, alumnoID, lockAlumno)
	return err
}

// Reservar aplica CU-01 pasos 4 a 6 en una transacción: bloquea la clase (el
// cupo se verifica y se toma de forma atómica, RN-14) y serializa las reservas
// del alumno para duplicado, turno diario y superposición (RN-13, RN-39, RN-40).
func (d *DB) Reservar(ctx context.Context, op ports.Operacion, claseID string, validar func(domain.Clase) error) (domain.Reserva, bool, error) {
	return idempotente(ctx, d, op, func(tx pgx.Tx) (domain.Reserva, error) {
		alumno := op.Actor
		if err := bloquearAlumno(ctx, tx, alumno); err != nil {
			return domain.Reserva{}, err
		}
		c, _, err := claseBloqueada(ctx, tx, claseID)
		if err != nil {
			return domain.Reserva{}, err
		}
		if err = validar(c); err != nil {
			return domain.Reserva{}, err
		}
		var duplicada, musculacion, superpuesta bool
		dia := domain.DiaMusculacion(c)
		err = tx.QueryRow(ctx, `SELECT
			EXISTS(SELECT 1 FROM reservas WHERE clase_id=$1 AND alumno_id=$2 AND estado='CONFIRMADA'),
			EXISTS(SELECT 1 FROM reservas WHERE alumno_id=$2 AND dia_musculacion=NULLIF($3::text,'')::date AND estado IN ('CONFIRMADA','ASISTIDA')),
			EXISTS(SELECT 1 FROM reservas WHERE alumno_id=$2 AND estado='CONFIRMADA' AND inicio < $5 AND $4 < fin)`,
			claseID, alumno, dia, c.Inicio, c.Fin).Scan(&duplicada, &musculacion, &superpuesta)
		switch {
		case err != nil:
			return domain.Reserva{}, err
		case duplicada:
			return domain.Reserva{}, domain.ErrReservaDuplicada
		case musculacion:
			return domain.Reserva{}, domain.ErrTurnoMusculacionDiario
		case superpuesta:
			return domain.Reserva{}, domain.ErrSuperposicionReserva
		}
		r := domain.Reserva{ClaseID: claseID, AlumnoID: alumno, Estado: domain.ReservaConfirmada}
		if err = tx.QueryRow(ctx, `INSERT INTO reservas(clase_id, alumno_id, estado, inicio, fin, dia_musculacion, creada_en, actualizada_en, actualizada_por)
			VALUES($1,$2,'CONFIRMADA',$3,$4,NULLIF($5::text,'')::date,now(),now(),$2) RETURNING id::text, creada_en`,
			claseID, alumno, c.Inicio, c.Fin, dia).Scan(&r.ID, &r.CreadaEn); err != nil {
			return domain.Reserva{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE clases SET ocupacion=ocupacion+1, actualizada_en=now() WHERE id=$1`, claseID); err != nil {
			return domain.Reserva{}, err
		}
		c.Ocupacion++
		c.ReservadaPorMi = true
		r.Clase = c
		return r, auditar(ctx, tx, alumno, "RESERVAR", "reserva", r.ID, "")
	})
}

// CancelarReserva aplica CU-02: bloquea la reserva y su clase, valida y libera el
// cupo una sola vez (RN-17, CA-15.1/2).
func (d *DB) CancelarReserva(ctx context.Context, op ports.Operacion, reservaID, motivo string, validar func(domain.Reserva) (bool, error)) (domain.Reserva, bool, error) {
	return idempotente(ctx, d, op, func(tx pgx.Tx) (domain.Reserva, error) {
		var r domain.Reserva
		if err := tx.QueryRow(ctx, `SELECT `+reservaCols+`, `+claseCols+reservaFrom+` WHERE r.id=$1 FOR UPDATE OF r, c`, reservaID).Scan(reservaDestinos(&r)...); err != nil {
			return r, traducir(err)
		}
		sinCambios, err := validar(r)
		if err != nil || sinCambios {
			return r, err
		}
		var cancelada time.Time
		if err = tx.QueryRow(ctx, `UPDATE reservas SET estado='CANCELADA', cancelada_en=now(), motivo_cancelacion=$2, actualizada_en=now(), actualizada_por=$3 WHERE id=$1 RETURNING cancelada_en`, reservaID, motivo, op.Actor).Scan(&cancelada); err != nil {
			return r, err
		}
		if _, err = tx.Exec(ctx, `UPDATE clases SET ocupacion=ocupacion-1, actualizada_en=now() WHERE id=$1`, r.ClaseID); err != nil {
			return r, err
		}
		r.Estado, r.CanceladaEn, r.MotivoCancelacion = domain.ReservaCancelada, &cancelada, &motivo
		r.Clase.Ocupacion--
		return r, auditar(ctx, tx, op.Actor, "CANCELAR", "reserva", reservaID, motivo)
	})
}

// MisReservas lista las reservas del alumno, más recientes primero (CA-16.1).
func (d *DB) MisReservas(ctx context.Context, alumnoID, estado string, pagina, tamano int) (domain.PaginaReservas, error) {
	out := domain.PaginaReservas{Items: []domain.Reserva{}, Pagina: pagina, Tamano: tamano}
	rows, err := d.pool.Query(ctx, `SELECT `+reservaCols+`, `+claseCols+`, count(*) OVER()`+reservaFrom+`
		WHERE r.alumno_id=$1 AND ($2::text='' OR r.estado=$2::text) ORDER BY r.inicio DESC, r.id LIMIT $3 OFFSET $4`,
		alumnoID, estado, tamano, (pagina-1)*tamano)
	if err != nil {
		return out, err
	}
	items, err := scanReservas(rows, &out.Total)
	if err != nil {
		return out, err
	}
	out.Items = items
	return out, nil
}

// RegistrarIngreso aplica CU-03: bloquea las reservas del alumno cercanas al
// ingreso, elige la reserva con completar y, si no es reingreso, registra la
// asistencia única con su evento en el outbox (RN-19 a RN-21, D-37/38).
func (d *DB) RegistrarIngreso(ctx context.Context, op ports.Operacion, alumnoID string, now time.Time, completar func([]domain.Reserva) (domain.Ingreso, error)) (domain.Ingreso, bool, error) {
	return idempotente(ctx, d, op, func(tx pgx.Tx) (domain.Ingreso, error) {
		if err := bloquearAlumno(ctx, tx, alumnoID); err != nil {
			return domain.Ingreso{}, err
		}
		rows, err := tx.Query(ctx, `SELECT `+reservaCols+`, `+claseCols+reservaFrom+`
			WHERE r.alumno_id=$1 AND r.estado IN ('CONFIRMADA','ASISTIDA') AND c.estado<>'CANCELADA'
			  AND c.inicio <= $2::timestamptz + interval '15 minutes' AND c.fin > $2::timestamptz
			FOR UPDATE OF r`, alumnoID, now)
		if err != nil {
			return domain.Ingreso{}, err
		}
		reservas, err := scanReservas(rows)
		if err != nil {
			return domain.Ingreso{}, err
		}
		ingreso, err := completar(reservas)
		if err != nil || ingreso.Reingreso {
			return ingreso, err
		}
		r := ingreso.Reserva
		tag, err := tx.Exec(ctx, `UPDATE reservas SET estado='ASISTIDA', ingreso_en=$2, actualizada_en=$2, actualizada_por=$3 WHERE id=$1 AND estado='CONFIRMADA'`, r.ID, now, op.Actor)
		if err != nil {
			return ingreso, err
		}
		if tag.RowsAffected() != 1 {
			return ingreso, domain.ErrSinReservaParaHorario
		}
		var asistenciaID string
		if err = tx.QueryRow(ctx, `INSERT INTO asistencias(reserva_id, clase_id, alumno_id, registrada_en, registrada_por) VALUES($1,$2,$3,$4,$5) RETURNING id::text`, r.ID, r.ClaseID, alumnoID, now, op.Actor).Scan(&asistenciaID); err != nil {
			return ingreso, err
		}
		data, err := json.Marshal(map[string]any{
			"asistencia_id": asistenciaID, "reserva_id": r.ID, "clase_id": r.ClaseID, "alumno_id": alumnoID,
			"actividad_id": r.Clase.ActividadID, "actividad_nombre": r.Clase.ActividadNombre,
			"clase_inicio": r.Clase.Inicio.UTC().Format(time.RFC3339), "resultado": "ASISTIO",
			"puntos": domain.PuntosAsistencia, "origen": "INGRESO_DNI", "registrada_en": now.UTC().Format(time.RFC3339),
		})
		if err != nil {
			return ingreso, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO outbox(event_type, occurred_at, correlation_id, agregado_id, data) VALUES('asistencia.registrada',$1,NULLIF($2,''),$3,$4)`, now, correlation.FromContext(ctx), r.ID, data); err != nil {
			return ingreso, err
		}
		ingresoEn := now
		ingreso.Reserva.Estado, ingreso.Reserva.IngresoEn = domain.ReservaAsistida, &ingresoEn
		return ingreso, auditar(ctx, tx, op.Actor, "INGRESO", "reserva", r.ID, "")
	})
}

// CerrarClases finaliza hasta 200 clases terminadas por llamada, pasa a AUSENTE
// sus reservas confirmadas y escribe un inasistencia.registrada por cada una
// (RN-38, CA-22.8). SKIP LOCKED permite varias instancias sin trabajo duplicado.
func (d *DB) CerrarClases(ctx context.Context, now time.Time) (int, int, error) {
	clases, ausentes := 0, 0
	err := d.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, actividad_id::text FROM clases WHERE estado='PROGRAMADA' AND fin <= $1 ORDER BY fin LIMIT 200 FOR UPDATE SKIP LOCKED`, now)
		if err != nil {
			return err
		}
		ids := []string{}
		actividad := map[string]string{}
		for rows.Next() {
			var id, act string
			if err = rows.Scan(&id, &act); err != nil {
				rows.Close()
				return err
			}
			ids = append(ids, id)
			actividad[id] = act
		}
		rows.Close()
		if err = rows.Err(); err != nil || len(ids) == 0 {
			return err
		}
		rows, err = tx.Query(ctx, `UPDATE reservas SET estado='AUSENTE', actualizada_en=$2 WHERE clase_id = ANY($1::uuid[]) AND estado='CONFIRMADA' RETURNING id::text, clase_id::text, alumno_id::text, inicio`, ids, now)
		if err != nil {
			return err
		}
		type ausencia struct {
			reserva, clase, alumno string
			inicio                 time.Time
		}
		var lista []ausencia
		for rows.Next() {
			var a ausencia
			if err = rows.Scan(&a.reserva, &a.clase, &a.alumno, &a.inicio); err != nil {
				rows.Close()
				return err
			}
			lista = append(lista, a)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
		for _, a := range lista {
			data, err := json.Marshal(map[string]any{
				"reserva_id": a.reserva, "clase_id": a.clase, "alumno_id": a.alumno, "actividad_id": actividad[a.clase],
				"clase_inicio": a.inicio.UTC().Format(time.RFC3339), "puntos_penalizacion": domain.PenalizacionAusencia,
				"registrada_en": now.UTC().Format(time.RFC3339),
			})
			if err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, `INSERT INTO outbox(event_type, occurred_at, agregado_id, data) VALUES('inasistencia.registrada',$1,$2,$3) ON CONFLICT (event_type, agregado_id) WHERE event_type IN ('asistencia.registrada', 'inasistencia.registrada') DO NOTHING`, now, a.reserva, data); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE clases SET estado='FINALIZADA', actualizada_en=$2 WHERE id = ANY($1::uuid[])`, ids, now); err != nil {
			return err
		}
		clases, ausentes = len(ids), len(lista)
		return nil
	})
	return clases, ausentes, traducir(err)
}
