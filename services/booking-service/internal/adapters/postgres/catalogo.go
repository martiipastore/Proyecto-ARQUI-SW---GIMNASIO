package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/GuadiViti/Arquitectura-de-Software/services/booking-service/internal/domain"
)

const actividadCols = `id::text, nombre, capacidad_maxima, musculacion, activa`

func scanActividad(row pgx.Row) (domain.Actividad, error) {
	var a domain.Actividad
	err := row.Scan(&a.ID, &a.Nombre, &a.CapacidadMaxima, &a.Musculacion, &a.Activa)
	return a, traducir(err)
}

// Actividades lista el catálogo, Musculación primero.
func (d *DB) Actividades(ctx context.Context) ([]domain.Actividad, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+actividadCols+` FROM actividades ORDER BY musculacion DESC, nombre`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Actividad{}
	for rows.Next() {
		a, err := scanActividad(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Actividad devuelve una actividad o domain.ErrNoEncontrado.
func (d *DB) Actividad(ctx context.Context, id string) (domain.Actividad, error) {
	return scanActividad(d.pool.QueryRow(ctx, `SELECT `+actividadCols+` FROM actividades WHERE id=$1`, id))
}

// CrearActividad da de alta una actividad grupal.
func (d *DB) CrearActividad(ctx context.Context, actor string, a domain.Actividad) (domain.Actividad, error) {
	var out domain.Actividad
	err := d.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanActividad(tx.QueryRow(ctx, `INSERT INTO actividades(nombre,capacidad_maxima,musculacion,activa,actualizada_por) VALUES($1,$2,false,true,$3) RETURNING `+actividadCols, a.Nombre, a.CapacidadMaxima, actor))
		if err != nil {
			return err
		}
		return auditar(ctx, tx, actor, "CREAR", "actividad", out.ID, "")
	})
	return out, traducir(err)
}

// ModificarActividad bloquea la actividad y aplica cambiar (CA-06.2, CA-06.3).
func (d *DB) ModificarActividad(ctx context.Context, actor, id string, cambiar func(domain.Actividad, bool) (domain.Actividad, error)) (domain.Actividad, error) {
	var out domain.Actividad
	err := d.tx(ctx, func(tx pgx.Tx) error {
		actual, err := scanActividad(tx.QueryRow(ctx, `SELECT `+actividadCols+` FROM actividades WHERE id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		var conReservas bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM clases c JOIN reservas r ON r.clase_id=c.id AND r.estado='CONFIRMADA' WHERE c.actividad_id=$1 AND c.estado='PROGRAMADA' AND c.inicio>now())`, id).Scan(&conReservas); err != nil {
			return err
		}
		nueva, err := cambiar(actual, conReservas)
		if err != nil {
			return err
		}
		out, err = scanActividad(tx.QueryRow(ctx, `UPDATE actividades SET nombre=$2,capacidad_maxima=$3,activa=$4,actualizada_en=now(),actualizada_por=$5 WHERE id=$1 RETURNING `+actividadCols, id, nueva.Nombre, nueva.CapacidadMaxima, nueva.Activa, actor))
		if err != nil {
			return err
		}
		return auditar(ctx, tx, actor, "MODIFICAR", "actividad", id, "")
	})
	return out, traducir(err)
}

const horarioCols = `id::text, actividad_id::text, dia_semana, inicio_minutos, duracion_minutos, profesor_id::text, profesor_nombre, capacidad`

func scanHorario(row pgx.Row) (domain.Horario, error) {
	var h domain.Horario
	err := row.Scan(&h.ID, &h.ActividadID, &h.DiaSemana, &h.InicioMinutos, &h.DuracionMinutos, &h.ProfesorID, &h.ProfesorNombre, &h.Capacidad)
	return h, traducir(err)
}

// Horarios lista los horarios de una actividad.
func (d *DB) Horarios(ctx context.Context, actividadID string) ([]domain.Horario, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+horarioCols+` FROM horarios WHERE actividad_id=$1 ORDER BY dia_semana, inicio_minutos`, actividadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Horario{}
	for rows.Next() {
		h, err := scanHorario(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// CrearHorario guarda el horario; la restricción de exclusión rechaza un
// profesor con horarios superpuestos (RN-08, CA-07.4).
func (d *DB) CrearHorario(ctx context.Context, actor string, h domain.Horario) (domain.Horario, error) {
	var out domain.Horario
	err := d.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanHorario(tx.QueryRow(ctx, `INSERT INTO horarios(actividad_id,dia_semana,inicio_minutos,duracion_minutos,profesor_id,profesor_nombre,capacidad,creado_por) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING `+horarioCols,
			h.ActividadID, h.DiaSemana, h.InicioMinutos, h.DuracionMinutos, h.ProfesorID, h.ProfesorNombre, h.Capacidad, actor))
		if err != nil {
			return err
		}
		return auditar(ctx, tx, actor, "CREAR", "horario", out.ID, "")
	})
	return out, traducir(err)
}

// GuardarClases inserta las clases sin duplicar por actividad e inicio (CA-07.5).
func (d *DB) GuardarClases(ctx context.Context, actor string, clases []domain.Clase) (int, error) {
	creadas := 0
	err := d.tx(ctx, func(tx pgx.Tx) error {
		batch := &pgx.Batch{}
		for _, c := range clases {
			batch.Queue(`INSERT INTO clases(actividad_id,horario_id,inicio,fin,profesor_id,profesor_nombre,capacidad,actualizada_por) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8) ON CONFLICT ON CONSTRAINT clase_unica DO NOTHING`,
				c.ActividadID, c.HorarioID, c.Inicio, c.Fin, c.ProfesorID, c.ProfesorNombre, c.Capacidad, actor)
		}
		results := tx.SendBatch(ctx, batch)
		for range clases {
			tag, err := results.Exec()
			if err != nil {
				_ = results.Close()
				return err
			}
			creadas += int(tag.RowsAffected())
		}
		if err := results.Close(); err != nil {
			return err
		}
		return auditar(ctx, tx, actor, "GENERAR", "clases", "", strconv.Itoa(creadas)+" clases nuevas")
	})
	return creadas, traducir(err)
}

const claseCols = `c.id::text, c.actividad_id::text, a.nombre, a.musculacion, coalesce(c.horario_id::text,''), c.inicio, c.fin, c.profesor_id::text, c.profesor_nombre, c.capacidad, c.ocupacion, c.estado`
const claseFrom = ` FROM clases c JOIN actividades a ON a.id=c.actividad_id`

// reservadaPorMi marca "ya reservada" para el alumno del parámetro indicado (CA-13.4).
func reservadaPorMi(param int) string {
	p := "$" + strconv.Itoa(param)
	return `, EXISTS(SELECT 1 FROM reservas r WHERE r.clase_id=c.id AND r.estado='CONFIRMADA' AND r.alumno_id=NULLIF(` + p + `::text,'')::uuid)`
}

func claseDestinos(c *domain.Clase) []any {
	return []any{&c.ID, &c.ActividadID, &c.ActividadNombre, &c.Musculacion, &c.HorarioID, &c.Inicio, &c.Fin, &c.ProfesorID, &c.ProfesorNombre, &c.Capacidad, &c.Ocupacion, &c.Estado}
}

// BuscarClases filtra y pagina por inicio e id (HU-13, ADR-014).
func (d *DB) BuscarClases(ctx context.Context, f domain.FiltroClases, alumnoID string) (domain.PaginaClases, error) {
	out := domain.PaginaClases{Items: []domain.Clase{}, Pagina: f.Pagina, Tamano: f.Tamano}
	rows, err := d.pool.Query(ctx, `SELECT `+claseCols+reservadaPorMi(1)+`, count(*) OVER()`+claseFrom+`
		WHERE c.inicio >= $2 AND c.inicio < $3
		  AND ($4::text = '' OR c.actividad_id = NULLIF($4::text,'')::uuid)
		  AND ($5::text = '' OR to_char(c.inicio AT TIME ZONE 'America/Argentina/Buenos_Aires','HH24:MI') = $5::text)
		  AND (NOT $6::boolean OR (c.estado='PROGRAMADA' AND c.inicio > now() AND c.ocupacion < c.capacidad))
		ORDER BY c.inicio, c.id LIMIT $7 OFFSET $8`,
		alumnoID, f.Desde, f.Hasta, f.ActividadID, f.Franja, f.SoloDisponibles, f.Tamano, (f.Pagina-1)*f.Tamano)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c domain.Clase
		if err = rows.Scan(append(claseDestinos(&c), &c.ReservadaPorMi, &out.Total)...); err != nil {
			return out, err
		}
		out.Items = append(out.Items, c)
	}
	return out, rows.Err()
}

// Clase devuelve una clase con su ocupación actual.
func (d *DB) Clase(ctx context.Context, id, alumnoID string) (domain.Clase, error) {
	var c domain.Clase
	err := d.pool.QueryRow(ctx, `SELECT `+claseCols+reservadaPorMi(2)+claseFrom+` WHERE c.id=$1`, id, alumnoID).Scan(append(claseDestinos(&c), &c.ReservadaPorMi)...)
	return c, traducir(err)
}

// ClasesDeProfesor lista las clases del profesor desde una fecha (HU-21).
func (d *DB) ClasesDeProfesor(ctx context.Context, profesorID string, desde time.Time) ([]domain.Clase, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+claseCols+claseFrom+` WHERE c.profesor_id=$1 AND c.inicio >= $2 ORDER BY c.inicio, c.id LIMIT 200`, profesorID, desde)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Clase{}
	for rows.Next() {
		var c domain.Clase
		if err = rows.Scan(claseDestinos(&c)...); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const reservaCols = `r.id::text, r.clase_id::text, r.alumno_id::text, r.estado, r.creada_en, r.ingreso_en, r.cancelada_en, r.motivo_cancelacion`
const reservaFrom = ` FROM reservas r JOIN clases c ON c.id=r.clase_id JOIN actividades a ON a.id=c.actividad_id`

func reservaDestinos(r *domain.Reserva) []any {
	return append([]any{&r.ID, &r.ClaseID, &r.AlumnoID, &r.Estado, &r.CreadaEn, &r.IngresoEn, &r.CanceladaEn, &r.MotivoCancelacion}, claseDestinos(&r.Clase)...)
}

func scanReservas(rows pgx.Rows, extra ...any) ([]domain.Reserva, error) {
	defer rows.Close()
	out := []domain.Reserva{}
	for rows.Next() {
		var r domain.Reserva
		if err := rows.Scan(append(reservaDestinos(&r), extra...)...); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ReservasDeClase lista las reservas no canceladas: activas, asistidas y
// ausentes (CA-21.1, CA-35.1/2).
func (d *DB) ReservasDeClase(ctx context.Context, claseID string) ([]domain.Reserva, error) {
	rows, err := d.pool.Query(ctx, `SELECT `+reservaCols+`, `+claseCols+reservaFrom+` WHERE r.clase_id=$1 AND r.estado<>'CANCELADA' ORDER BY r.creada_en, r.id`, claseID)
	if err != nil {
		return nil, err
	}
	return scanReservas(rows)
}

func claseBloqueada(ctx context.Context, tx pgx.Tx, id string) (domain.Clase, domain.Actividad, error) {
	var c domain.Clase
	var a domain.Actividad
	err := tx.QueryRow(ctx, `SELECT `+claseCols+`, a.capacidad_maxima`+claseFrom+` WHERE c.id=$1 FOR UPDATE OF c`, id).Scan(append(claseDestinos(&c), &a.CapacidadMaxima)...)
	a.ID, a.Nombre, a.Musculacion = c.ActividadID, c.ActividadNombre, c.Musculacion
	return c, a, traducir(err)
}

// CambiarCapacidad bloquea la clase, valida y guarda (RN-10, CA-08.1/2/4).
func (d *DB) CambiarCapacidad(ctx context.Context, actor, id string, capacidad int, validar func(domain.Clase, domain.Actividad) error) (domain.Clase, error) {
	var out domain.Clase
	err := d.tx(ctx, func(tx pgx.Tx) error {
		c, a, err := claseBloqueada(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = validar(c, a); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE clases SET capacidad=$2, actualizada_en=now(), actualizada_por=$3 WHERE id=$1`, id, capacidad, actor); err != nil {
			return err
		}
		c.Capacidad = capacidad
		out = c
		return auditar(ctx, tx, actor, "CAMBIAR_CAPACIDAD", "clase", id, strconv.Itoa(capacidad))
	})
	return out, traducir(err)
}

// CancelarClase cancela la clase y todas sus reservas activas con el motivo
// "clase cancelada" (RN-32, CA-08.3).
func (d *DB) CancelarClase(ctx context.Context, actor, id, motivo string, validar func(domain.Clase) error) (domain.Clase, error) {
	var out domain.Clase
	err := d.tx(ctx, func(tx pgx.Tx) error {
		c, _, err := claseBloqueada(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = validar(c); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE reservas SET estado='CANCELADA', cancelada_en=now(), motivo_cancelacion=$2, actualizada_en=now(), actualizada_por=$3 WHERE clase_id=$1 AND estado='CONFIRMADA'`, id, domain.MotivoClaseCancelada, actor); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE clases SET estado='CANCELADA', ocupacion=0, cancelada_en=now(), motivo_cancelacion=$2, actualizada_en=now(), actualizada_por=$3 WHERE id=$1`, id, motivo, actor); err != nil {
			return err
		}
		c.Estado, c.Ocupacion = domain.ClaseCancelada, 0
		out = c
		return auditar(ctx, tx, actor, "CANCELAR", "clase", id, motivo)
	})
	return out, traducir(err)
}
