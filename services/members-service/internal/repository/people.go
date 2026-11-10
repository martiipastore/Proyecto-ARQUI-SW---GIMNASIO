package repository

import (
	"context"
	"errors"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/jackc/pgx/v5"
)

// Columnas de una persona. password_hash nunca sale del repositorio: solo se
// informa si todavía falta establecerla.
const personColumns = `u.id::text,u.rol,u.nombre,u.apellido,u.dni,u.email,u.telefono,u.fecha_nacimiento::text,u.matricula,u.estado,u.registro_externo,u.creado_en,u.dado_de_baja_en,u.password_hash IS NULL`

func scanPerson(row pgx.Row) (model.Person, error) {
	var p model.Person
	var created time.Time
	var deactivated *time.Time
	err := row.Scan(&p.ID, &p.Role, &p.Name, &p.LastName, &p.DNI, &p.Email, &p.Phone, &p.BirthDate, &p.License, &p.State, &p.External, &created, &deactivated, &p.PendingPassword)
	if errors.Is(err, pgx.ErrNoRows) {
		return p, model.ErrNotFound
	}
	if err != nil {
		return p, err
	}
	p.CreatedAt = created.UTC().Format(time.RFC3339)
	if deactivated != nil {
		value := deactivated.UTC().Format(time.RFC3339)
		p.DeactivatedAt = &value
	}
	return p, nil
}

func uniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	return errors.As(err, &pgErr) && pgErr.SQLState() == "23505"
}

func audit(ctx context.Context, tx pgx.Tx, entity, id, action, actor string, data map[string]any) error {
	var actorID any
	if actor != "" {
		actorID = actor
	}
	_, err := tx.Exec(ctx, `INSERT INTO auditoria_usuarios(entidad,entidad_id,accion,actor_id,datos) VALUES($1,$2,$3,$4,$5)`, entity, id, action, actorID, data)
	return err
}

func personTx(ctx context.Context, tx pgx.Tx, id, lock string) (model.Person, error) {
	return scanPerson(tx.QueryRow(ctx, `SELECT `+personColumns+` FROM usuarios u WHERE u.id=$1 `+lock, id))
}

func (p *Postgres) Person(ctx context.Context, id string) (model.Person, error) {
	return scanPerson(p.pool.QueryRow(ctx, `SELECT `+personColumns+` FROM usuarios u WHERE u.id=$1`, id))
}

func (p *Postgres) CreatePerson(ctx context.Context, actor, id string, in model.PersonInput, canComplete func(model.Person) bool, activation func(string) ([]byte, []byte, error), now time.Time) (model.Person, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Person{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	// Mismo bloqueo por email que el registro externo, más uno por DNI: dos
	// altas simultáneas con el mismo dato se serializan (RN-33).
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,4)), pg_advisory_xact_lock(hashtextextended($2,5))`, in.Email, in.DNI); err != nil {
		return model.Person{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM usuarios WHERE dni=$1)`, in.DNI).Scan(&exists); err != nil {
		return model.Person{}, err
	}
	if exists {
		return model.Person{}, model.ErrDuplicate
	}
	action := "ALTA"
	existing, err := scanPerson(tx.QueryRow(ctx, `SELECT `+personColumns+` FROM usuarios u WHERE u.email=$1 FOR UPDATE`, in.Email))
	switch {
	case err == nil:
		if !canComplete(existing) {
			return model.Person{}, model.ErrDuplicate
		}
		id, action = existing.ID, "ALTA_DESDE_REGISTRO_EXTERNO"
		_, err = tx.Exec(ctx, `UPDATE usuarios SET nombre=$2,apellido=$3,dni=$4,telefono=NULLIF($5,''),fecha_nacimiento=NULLIF($6,'')::date,estado='ACTIVO',dado_de_baja_en=NULL,actualizado_en=$7,actualizado_por=$8 WHERE id=$1`, id, in.Name, in.LastName, in.DNI, in.Phone, in.BirthDate, now, actor)
	case errors.Is(err, model.ErrNotFound):
		_, err = tx.Exec(ctx, `INSERT INTO usuarios(id,nombre,apellido,dni,email,telefono,fecha_nacimiento,matricula,rol,registro_externo,creado_en,actualizado_en,actualizado_por) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,'')::date,NULLIF($8,''),$9,false,$10,$10,$11)`, id, in.Name, in.LastName, in.DNI, in.Email, in.Phone, in.BirthDate, in.License, in.Role, now, actor)
	}
	if err != nil {
		if uniqueViolation(err) {
			return model.Person{}, model.ErrDuplicate
		}
		return model.Person{}, err
	}
	person, err := personTx(ctx, tx, id, "")
	if err != nil {
		return model.Person{}, err
	}
	if person.PendingPassword {
		var activationID string
		if err = tx.QueryRow(ctx, `SELECT gen_random_uuid()::text`).Scan(&activationID); err != nil {
			return model.Person{}, err
		}
		hash, encrypted, err := activation(activationID)
		if err != nil {
			return model.Person{}, err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO activaciones_cuenta(id,usuario_id,token_hash,token_cifrado,token_expira) VALUES($1,$2,$3,$4,$5) ON CONFLICT (usuario_id) WHERE NOT consumida DO UPDATE SET token_hash=EXCLUDED.token_hash,token_cifrado=EXCLUDED.token_cifrado,token_expira=EXCLUDED.token_expira,correo_enviado_en=NULL,proximo_intento=now()`, activationID, id, hash, encrypted, now.Add(24*time.Hour)); err != nil {
			return model.Person{}, err
		}
	}
	if person.Role == "ALUMNO" {
		// S-10/CA-01.1: el hecho queda durable; la etapa 5 lo publicará a Benefits.
		if _, err = tx.Exec(ctx, `INSERT INTO outbox_usuarios(event_type,usuario_id,data,occurred_at) VALUES('alumno.creado',$1::uuid,jsonb_build_object('alumno_id',$1::uuid),$2) ON CONFLICT (usuario_id,event_type) DO NOTHING`, id, now); err != nil {
			return model.Person{}, err
		}
	}
	if err = audit(ctx, tx, "usuario", id, action, actor, map[string]any{"rol": person.Role}); err != nil {
		return model.Person{}, err
	}
	return person, tx.Commit(ctx)
}

func (p *Postgres) UpdatePerson(ctx context.Context, actor, id string, in model.PersonUpdate, now time.Time) (model.Person, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Person{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	tag, err := tx.Exec(ctx, `UPDATE usuarios SET nombre=$2,apellido=$3,telefono=NULLIF($4,''),fecha_nacimiento=NULLIF($5,'')::date,matricula=NULLIF($6,''),actualizado_en=$7,actualizado_por=$8 WHERE id=$1 AND dni IS NOT NULL`, id, in.Name, in.LastName, in.Phone, in.BirthDate, in.License, now, actor)
	if err != nil {
		return model.Person{}, err
	}
	if tag.RowsAffected() != 1 {
		return model.Person{}, model.ErrNotFound
	}
	if err = audit(ctx, tx, "usuario", id, "MODIFICACION", actor, map[string]any{}); err != nil {
		return model.Person{}, err
	}
	person, err := personTx(ctx, tx, id, "")
	if err != nil {
		return model.Person{}, err
	}
	return person, tx.Commit(ctx)
}

func (p *Postgres) DeactivatePerson(ctx context.Context, actor, id, reason string, check func(model.Person) error, now time.Time) (model.Person, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Person{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	person, err := personTx(ctx, tx, id, "FOR UPDATE")
	if err != nil {
		return model.Person{}, err
	}
	if err = check(person); err != nil {
		return model.Person{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE usuarios SET estado='INACTIVO',dado_de_baja_en=$2,actualizado_en=$2,actualizado_por=$3 WHERE id=$1`, id, now, actor); err != nil {
		return model.Person{}, err
	}
	// Punto de extensión de la etapa 5: booking cancelará reservas futuras (D-20).
	if _, err = tx.Exec(ctx, `INSERT INTO outbox_usuarios(event_type,usuario_id,data,occurred_at) VALUES('usuario.desactivado',$1::uuid,jsonb_build_object('usuario_id',$1::uuid,'rol',$2::text,'desactivado_en',$3::timestamptz),$3) ON CONFLICT (usuario_id,event_type) DO NOTHING`, id, person.Role, now); err != nil {
		return model.Person{}, err
	}
	if err = audit(ctx, tx, "usuario", id, "BAJA", actor, map[string]any{"motivo": reason}); err != nil {
		return model.Person{}, err
	}
	person, err = personTx(ctx, tx, id, "")
	if err != nil {
		return model.Person{}, err
	}
	return person, tx.Commit(ctx)
}

func (p *Postgres) People(ctx context.Context, f model.PersonFilter) (model.PersonPage, error) {
	out := model.PersonPage{Items: []model.Person{}, Page: f.Page, Size: f.Size}
	where := ` FROM usuarios u WHERE ($1='' OR u.rol=$1) AND ($2='' OR u.estado=$2) AND ($3='' OR u.dni=$3 OR position(lower($3) in lower(u.email))>0 OR position(lower($3) in lower(u.nombre))>0 OR position(lower($3) in lower(COALESCE(u.apellido,'')))>0)`
	if err := p.pool.QueryRow(ctx, `SELECT count(*)`+where, f.Role, f.State, f.Query).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := p.pool.Query(ctx, `SELECT `+personColumns+where+` ORDER BY u.apellido NULLS LAST,u.nombre,u.id LIMIT $4 OFFSET $5`, f.Role, f.State, f.Query, f.Size, int64(f.Page-1)*int64(f.Size))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		person, err := scanPerson(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, person)
	}
	return out, rows.Err()
}

const assignmentColumns = `a.id::text,a.profesional_id::text,a.alumno_id::text,a.desde,a.hasta,
 s.id::text,s.nombre,s.apellido,s.email,s.telefono,s.rol,s.estado,
 pr.id::text,pr.nombre,pr.apellido,pr.email,pr.telefono,pr.rol,pr.estado`

const assignmentFrom = ` FROM asignaciones a JOIN usuarios s ON s.id=a.alumno_id JOIN usuarios pr ON pr.id=a.profesional_id`

func scanAssignment(row pgx.Row) (model.Assignment, error) {
	var a model.Assignment
	var since time.Time
	var until *time.Time
	student, professional := &model.PersonDetail{}, &model.PersonDetail{}
	err := row.Scan(&a.ID, &a.ProfessionalID, &a.StudentID, &since, &until,
		&student.ID, &student.Name, &student.LastName, &student.Email, &student.Phone, &student.Role, &student.State,
		&professional.ID, &professional.Name, &professional.LastName, &professional.Email, &professional.Phone, &professional.Role, &professional.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, model.ErrNotFound
	}
	if err != nil {
		return a, err
	}
	a.Since = since.UTC().Format(time.RFC3339)
	if until != nil {
		value := until.UTC().Format(time.RFC3339)
		a.Until = &value
	}
	a.Student, a.Professional = student, professional
	return a, nil
}

func (p *Postgres) assignmentList(ctx context.Context, query string, args ...any) ([]model.Assignment, error) {
	rows, err := p.pool.Query(ctx, `SELECT `+assignmentColumns+assignmentFrom+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p *Postgres) Assign(ctx context.Context, actor, id, professional, student string, check func(model.Person, model.Person) error, now time.Time) (model.Assignment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Assignment{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	// FOR SHARE impide que una baja concurrente deje una asignación con un inactivo.
	prof, err := personTx(ctx, tx, professional, "FOR SHARE")
	if err != nil {
		return model.Assignment{}, err
	}
	stud, err := personTx(ctx, tx, student, "FOR SHARE")
	if err != nil {
		return model.Assignment{}, err
	}
	if err = check(prof, stud); err != nil {
		return model.Assignment{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO asignaciones(id,profesional_id,alumno_id,desde,asignada_por) VALUES($1,$2,$3,$4,$5)`, id, professional, student, now, actor); err != nil {
		if uniqueViolation(err) {
			return model.Assignment{}, model.ErrAssignmentDuplicate
		}
		return model.Assignment{}, err
	}
	if err = audit(ctx, tx, "asignacion", id, "ASIGNACION", actor, map[string]any{"profesional_id": professional, "alumno_id": student}); err != nil {
		return model.Assignment{}, err
	}
	a, err := scanAssignment(tx.QueryRow(ctx, `SELECT `+assignmentColumns+assignmentFrom+` WHERE a.id=$1`, id))
	if err != nil {
		return model.Assignment{}, err
	}
	return a, tx.Commit(ctx)
}

func (p *Postgres) Revoke(ctx context.Context, actor, professional, student string, now time.Time) (model.Assignment, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Assignment{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var id string
	err = tx.QueryRow(ctx, `UPDATE asignaciones SET hasta=$3,revocada_por=$4 WHERE profesional_id=$1 AND alumno_id=$2 AND hasta IS NULL RETURNING id::text`, professional, student, now, actor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Assignment{}, model.ErrNotFound
	}
	if err != nil {
		return model.Assignment{}, err
	}
	if err = audit(ctx, tx, "asignacion", id, "REVOCACION", actor, map[string]any{}); err != nil {
		return model.Assignment{}, err
	}
	a, err := scanAssignment(tx.QueryRow(ctx, `SELECT `+assignmentColumns+assignmentFrom+` WHERE a.id=$1`, id))
	if err != nil {
		return model.Assignment{}, err
	}
	return a, tx.Commit(ctx)
}

func (p *Postgres) Assignments(ctx context.Context, person string) ([]model.Assignment, error) {
	return p.assignmentList(ctx, ` WHERE a.profesional_id=$1 OR a.alumno_id=$1 ORDER BY a.hasta NULLS FIRST,a.desde DESC`, person)
}

func (p *Postgres) StudentsOf(ctx context.Context, professional string) ([]model.Assignment, error) {
	return p.assignmentList(ctx, ` WHERE a.profesional_id=$1 AND a.hasta IS NULL AND s.estado='ACTIVO' ORDER BY s.apellido,s.nombre`, professional)
}

func (p *Postgres) SummariesByID(ctx context.Context, ids []string, role string) ([]model.StudentSummary, error) {
	rows, err := p.pool.Query(ctx, `SELECT id::text,nombre,apellido,estado FROM usuarios WHERE id=ANY($1::uuid[]) AND rol=$2 ORDER BY id`, ids, role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.StudentSummary{}
	for rows.Next() {
		var s model.StudentSummary
		if err = rows.Scan(&s.ID, &s.Name, &s.LastName, &s.State); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *Postgres) StudentByDNI(ctx context.Context, dni string) (model.StudentSummary, error) {
	var s model.StudentSummary
	err := p.pool.QueryRow(ctx, `SELECT id::text,nombre,apellido,estado FROM usuarios WHERE dni=$1 AND rol='ALUMNO'`, dni).Scan(&s.ID, &s.Name, &s.LastName, &s.State)
	if errors.Is(err, pgx.ErrNoRows) {
		return s, model.ErrNotFound
	}
	return s, err
}

func (p *Postgres) ActiveAssignment(ctx context.Context, professional, student string) (bool, error) {
	var ok bool
	err := p.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1`+assignmentFrom+` WHERE a.profesional_id=$1 AND a.alumno_id=$2 AND a.hasta IS NULL AND s.estado='ACTIVO' AND pr.estado='ACTIVO')`, professional, student).Scan(&ok)
	return ok, err
}

func (p *Postgres) EnsureAdmin(ctx context.Context, id, email, name string, hash []byte) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	tag, err := tx.Exec(ctx, `INSERT INTO usuarios(id,nombre,email,rol,password_hash,registro_externo) VALUES($1,$2,$3,'ADMINISTRADOR',$4,false) ON CONFLICT (email) DO NOTHING`, id, name, email, hash)
	if err != nil {
		return false, err
	}
	created := tag.RowsAffected() == 1
	if created {
		if err = audit(ctx, tx, "usuario", id, "ALTA_ADMIN_INICIAL", "", map[string]any{"rol": "ADMINISTRADOR"}); err != nil {
			return false, err
		}
	}
	return created, tx.Commit(ctx)
}
