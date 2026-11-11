package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
)

// Etapa 13 (D-92): envíos de puntos a MELT.

func insertarEnvioMelt(ctx context.Context, tx pgx.Tx, e domain.EnvioMelt) error {
	_, err := tx.Exec(ctx, `INSERT INTO envios_melt(id,alumno_id,origen_id,referencia,puntos,estado,proximo_intento,creado_en,actualizado_en)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT (origen_id) DO NOTHING`,
		e.ID, e.AlumnoID, e.OrigenID, e.Referencia, e.Puntos, e.Estado, e.ProximoIntento, e.CreadoEn)
	return err
}

const columnasEnvio = `id::text,alumno_id::text,origen_id::text,referencia,puntos,estado,intentos,proximo_intento,COALESCE(registro_url,''),registro_vence,motivo,creado_en,actualizado_en`

func escanearEnvios(rows pgx.Rows) ([]domain.EnvioMelt, error) {
	defer rows.Close()
	out := []domain.EnvioMelt{}
	for rows.Next() {
		var e domain.EnvioMelt
		var vence *time.Time
		if err := rows.Scan(&e.ID, &e.AlumnoID, &e.OrigenID, &e.Referencia, &e.Puntos, &e.Estado, &e.Intentos, &e.ProximoIntento, &e.RegistroURL, &vence, &e.Motivo, &e.CreadoEn, &e.ActualizadoEn); err != nil {
			return nil, err
		}
		if vence != nil {
			e.RegistroVence = *vence
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// EnviosMeltVencidos: pendientes cuyo próximo intento llegó y enlaces de
// registro vencidos (se renuevan repitiendo la misma solicitud).
func (d *DB) EnviosMeltVencidos(ctx context.Context, ahora time.Time, limite int) (out []domain.EnvioMelt, err error) {
	defer func() { err = storageError(err) }()
	rows, err := d.pool.Query(ctx, `SELECT `+columnasEnvio+` FROM envios_melt
		WHERE (estado='PENDIENTE_ENVIO' AND proximo_intento <= $1) OR (estado='PENDIENTE_REGISTRO' AND (proximo_intento <= $1 OR registro_vence < $1))
		ORDER BY proximo_intento LIMIT $2`, ahora, limite)
	if err != nil {
		return nil, err
	}
	return escanearEnvios(rows)
}

func (d *DB) GuardarEnvioMelt(ctx context.Context, e domain.EnvioMelt) (err error) {
	defer func() { err = storageError(err) }()
	var url, vence any
	if e.RegistroURL != "" {
		url = e.RegistroURL
	}
	if !e.RegistroVence.IsZero() {
		vence = e.RegistroVence
	}
	_, err = d.pool.Exec(ctx, `UPDATE envios_melt SET estado=$2,intentos=$3,proximo_intento=$4,registro_url=$5,registro_vence=$6,motivo=$7,actualizado_en=$8 WHERE id=$1`,
		e.ID, e.Estado, e.Intentos, e.ProximoIntento, url, vence, e.Motivo, e.ActualizadoEn)
	return err
}

func (d *DB) EnviosMeltDeAlumno(ctx context.Context, alumno string) (out []domain.EnvioMelt, err error) {
	defer func() { err = storageError(err) }()
	rows, err := d.pool.Query(ctx, `SELECT `+columnasEnvio+` FROM envios_melt WHERE alumno_id=$1 ORDER BY creado_en DESC LIMIT 50`, alumno)
	if err != nil {
		return nil, err
	}
	return escanearEnvios(rows)
}
