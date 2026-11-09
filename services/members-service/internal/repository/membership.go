package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type membershipTx struct{ tx pgx.Tx }

func membershipError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrMembershipNotFound
	}
	var database *pgconn.PgError
	if errors.As(err, &database) {
		if database.Code == "23P01" {
			return model.ErrMembershipOverlap
		}
		if database.Code == "23514" {
			return model.ErrMembershipTransition
		}
		if database.Code == "23503" {
			return model.ErrMembershipNotFound
		}
	}
	return err
}
func (p *Postgres) RunMembership(ctx context.Context, actor, key, hash string, apply func(service.MembershipTx) (model.MembershipResult, error)) (result model.MembershipResult, replay bool, err error) {
	defer func() { err = membershipError(err) }()
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return result, false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var role string
	if err = tx.QueryRow(ctx, `SELECT rol FROM usuarios WHERE id=$1 FOR SHARE`, actor).Scan(&role); err != nil {
		return result, false, err
	}
	if role != access.Admin {
		return result, false, model.ErrForbidden
	}
	var locked bool
	if err = tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended($1,7))`, actor+":"+key).Scan(&locked); err != nil {
		return result, false, err
	}
	if !locked {
		return result, false, model.ErrMembershipInProgress
	}
	var storedHash string
	var data []byte
	err = tx.QueryRow(ctx, `SELECT huella,resultado FROM operaciones_membresia WHERE actor_id=$1 AND clave=$2`, actor, key).Scan(&storedHash, &data)
	if err == nil {
		if storedHash != hash {
			return result, false, model.ErrConflict
		}
		err = json.Unmarshal(data, &result)
		return result, true, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return result, false, err
	}
	result, err = apply(&membershipTx{tx})
	if err != nil {
		return result, false, err
	}
	data, err = json.Marshal(result)
	if err != nil {
		return result, false, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO operaciones_membresia(actor_id,clave,huella,resultado) VALUES($1,$2,$3,$4)`, actor, key, hash, data)
	if err != nil {
		return result, false, err
	}
	return result, false, tx.Commit(ctx)
}

const membershipColumns = `id::text,usuario_id::text,tipo_snapshot,fecha_inicio::text,fecha_vencimiento::text,estado,anterior_id::text,motivo`

func scanMembership(row interface{ Scan(...any) error }) (model.Membership, error) {
	var m model.Membership
	var config []byte
	err := row.Scan(&m.ID, &m.UserID, &config, &m.Start, &m.End, &m.State, &m.PreviousID, &m.Reason)
	if err == nil {
		err = json.Unmarshal(config, &m.Type)
	}
	return m, err
}

// LockUser serializa las operaciones del usuario y devuelve su rol; la regla
// sobre qué roles admiten membresía pertenece a service (patrón Capas).
func (t *membershipTx) LockUser(ctx context.Context, user string) (string, error) {
	var role string
	err := t.tx.QueryRow(ctx, `SELECT rol FROM usuarios WHERE id=$1 FOR UPDATE`, user).Scan(&role)
	return role, err
}
func (t *membershipTx) Type(ctx context.Context, code string) (model.MembershipType, error) {
	var v model.MembershipType
	err := t.tx.QueryRow(ctx, `SELECT codigo,nombre,duracion_meses,duracion_dias,clases,habilitado FROM tipos_membresia WHERE codigo=$1 FOR SHARE`, code).Scan(&v.Code, &v.Name, &v.Months, &v.Days, &v.Classes, &v.Enabled)
	return v, err
}
func (t *membershipTx) SaveType(ctx context.Context, v model.MembershipType, actor, reason string, now time.Time) error {
	tag, err := t.tx.Exec(ctx, `UPDATE tipos_membresia SET nombre=$2,duracion_meses=$3,duracion_dias=$4,clases=$5,habilitado=$6,modificada_por=$7,motivo=$8,actualizada_en=$9 WHERE codigo=$1`, v.Code, v.Name, v.Months, v.Days, v.Classes, v.Enabled, actor, reason, now)
	if err == nil && tag.RowsAffected() == 0 {
		return model.ErrMembershipNotFound
	}
	return err
}
func (t *membershipTx) Last(ctx context.Context, user string, active bool, today string) (*model.Membership, error) {
	where := ""
	if active {
		where = " AND estado='ACTIVA' AND fecha_vencimiento >= $2::date"
	}
	args := []any{user}
	if active {
		args = append(args, today)
	}
	m, err := scanMembership(t.tx.QueryRow(ctx, `SELECT `+membershipColumns+` FROM membresias WHERE usuario_id=$1`+where+` ORDER BY fecha_vencimiento DESC,id DESC LIMIT 1`, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &m, err
}
func (t *membershipTx) Get(ctx context.Context, id string) (model.Membership, error) {
	return scanMembership(t.tx.QueryRow(ctx, `SELECT `+membershipColumns+` FROM membresias WHERE id=$1`, id))
}
func (t *membershipTx) Insert(ctx context.Context, m model.Membership, actor string, now time.Time) error {
	snapshot, err := json.Marshal(m.Type)
	if err != nil {
		return err
	}
	_, err = t.tx.Exec(ctx, `INSERT INTO membresias(id,usuario_id,tipo_codigo,tipo_snapshot,fecha_inicio,fecha_vencimiento,estado,anterior_id,motivo,modificada_por,actualizada_en) VALUES($1,$2,$3,$4,$5,$6,'ACTIVA',$7,$8,$9,$10)`, m.ID, m.UserID, m.Type.Code, snapshot, m.Start, m.End, m.PreviousID, m.Reason, actor, now)
	return err
}
func (t *membershipTx) Cancel(ctx context.Context, id, actor, reason string, now time.Time) error {
	tag, err := t.tx.Exec(ctx, `UPDATE membresias SET estado='CANCELADA',modificada_por=$2,motivo=$3,actualizada_en=$4 WHERE id=$1 AND estado='ACTIVA'`, id, actor, reason, now)
	if err == nil && tag.RowsAffected() != 1 {
		return model.ErrMembershipTransition
	}
	return err
}
func (p *Postgres) MembershipTypes(ctx context.Context) ([]model.MembershipType, error) {
	rows, err := p.pool.Query(ctx, `SELECT codigo,nombre,duracion_meses,duracion_dias,clases,habilitado FROM tipos_membresia ORDER BY codigo`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MembershipType{}
	for rows.Next() {
		var t model.MembershipType
		if err = rows.Scan(&t.Code, &t.Name, &t.Months, &t.Days, &t.Classes, &t.Enabled); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
func (p *Postgres) MembershipPage(ctx context.Context, user, today string, page, size int) (out model.MembershipPage, err error) {
	defer func() { err = membershipError(err) }()
	out = model.MembershipPage{UserID: user, Page: page, Size: size, Items: []model.Membership{}}
	tx, err := p.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM usuarios WHERE id=$1)`, user).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, model.ErrMembershipNotFound
	}
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM membresias WHERE usuario_id=$1`, user).Scan(&out.Total); err != nil {
		return out, err
	}
	current, e := scanMembership(tx.QueryRow(ctx, `SELECT `+membershipColumns+` FROM membresias WHERE usuario_id=$1 AND estado='ACTIVA' AND fecha_inicio<=$2::date AND fecha_vencimiento>=$2::date LIMIT 1`, user, today))
	if e == nil {
		out.Current = &current
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return out, e
	}
	rows, err := tx.Query(ctx, `SELECT `+membershipColumns+` FROM membresias WHERE usuario_id=$1 ORDER BY fecha_inicio DESC,id DESC LIMIT $2 OFFSET $3`, user, size, (page-1)*size)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		m, e := scanMembership(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		out.Items = append(out.Items, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}
func (p *Postgres) ExpireMemberships(ctx context.Context, today string, now time.Time) (int64, error) {
	tag, err := p.pool.Exec(ctx, `UPDATE membresias SET estado='VENCIDA',modificada_por=NULL,motivo='Vencimiento automático',actualizada_en=$2 WHERE estado='ACTIVA' AND fecha_vencimiento<$1::date`, today, now)
	return tag.RowsAffected(), membershipError(err)
}
