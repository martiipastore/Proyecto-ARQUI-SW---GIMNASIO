package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/migrations"
	"github.com/jackc/pgx/v5"
)

func (p *Postgres) Migrate(ctx context.Context) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(620261106)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations(version text PRIMARY KEY,huella text NOT NULL); LOCK TABLE schema_migrations IN EXCLUSIVE MODE`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations.Files, "*.sql")
	if err != nil {
		return err
	}
	for _, file := range files {
		data, err := migrations.Files.ReadFile(file)
		if err != nil {
			return err
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		var old string
		err = tx.QueryRow(ctx, `SELECT huella FROM schema_migrations WHERE version=$1`, file).Scan(&old)
		if err == nil {
			if old != hash {
				return fmt.Errorf("migración modificada: %s", file)
			}
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err = tx.Exec(ctx, string(data)); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations VALUES ($1,$2)`, file, hash); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (p *Postgres) TakeAuthQuota(ctx context.Context, key string, max int, now time.Time) error {
	var count int
	err := p.pool.QueryRow(ctx, `INSERT INTO cuotas_autenticacion(clave,ventana,solicitudes) VALUES($1,date_trunc('minute',$2::timestamptz),1) ON CONFLICT(clave,ventana) DO UPDATE SET solicitudes=LEAST(cuotas_autenticacion.solicitudes+1,$3+1) RETURNING solicitudes`, key, now, max).Scan(&count)
	if err != nil {
		return err
	}
	if count > max {
		return model.ErrRateLimited
	}
	return nil
}

func (p *Postgres) RenewActivation(ctx context.Context, email string, generate func(string) ([]byte, []byte, error), now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var id, state string
	err = tx.QueryRow(ctx, `SELECT r.id::text,r.estado FROM registros_externos r JOIN usuarios u ON u.id=r.usuario_id WHERE u.email=$1 AND NOT r.token_consumido AND (r.estado='pending_verification' OR (r.estado='completed' AND u.password_hash IS NULL)) ORDER BY r.creado_en DESC LIMIT 1 FOR UPDATE OF r`, email).Scan(&id, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	hash, encrypted, err := generate(id)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE registros_externos SET token_hash=$2,token_cifrado=$3,token_expira=$4 WHERE id=$1`, id, hash, encrypted, now.Add(24*time.Hour)); err != nil {
		return err
	}
	kind := "welcome"
	if state == "pending_verification" {
		kind = "verification"
	}
	_, err = tx.Exec(ctx, `INSERT INTO correos_pendientes(registro_id,tipo) VALUES($1,$2) ON CONFLICT(registro_id,tipo) DO UPDATE SET enviado_en=NULL,proximo_intento=now()`, id, kind)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) RegisterExternal(ctx context.Context, in model.ExternalRegistration, fingerprint, tokenHash, encrypted []byte, expires time.Time) (model.Receipt, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return model.Receipt{}, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	// Serializa todas las altas del partner para que evento y key no se crucen.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,3))`, in.PartnerID); err != nil {
		return model.Receipt{}, err
	}
	var receipt model.Receipt
	var aliasEvent string
	aliasErr := tx.QueryRow(ctx, `SELECT r.evento_id FROM claves_registro k JOIN registros_externos r ON r.id=k.registro_id WHERE k.partner_id=$1 AND k.clave=$2`, in.PartnerID, in.Key).Scan(&aliasEvent)
	if aliasErr == nil && aliasEvent != in.EventID {
		return model.Receipt{}, model.ErrConflict
	}
	if aliasErr != nil && !errors.Is(aliasErr, pgx.ErrNoRows) {
		return model.Receipt{}, aliasErr
	}
	var stored []byte
	var key string
	err = tx.QueryRow(ctx, `SELECT id::text,evento_id,external_user_id,huella,clave FROM registros_externos WHERE partner_id=$1 AND (evento_id=$2 OR clave=$3) ORDER BY creado_en LIMIT 1`, in.PartnerID, in.EventID, in.Key).Scan(&receipt.ID, &receipt.EventID, &receipt.ExternalUserID, &stored, &key)
	if err == nil {
		if !bytes.Equal(stored, fingerprint) || receipt.EventID != in.EventID {
			return model.Receipt{}, model.ErrConflict
		}
		receipt.Status = "accepted"
		if _, err = tx.Exec(ctx, `INSERT INTO claves_registro VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, in.PartnerID, in.Key, receipt.ID); err != nil {
			return model.Receipt{}, err
		}
		return receipt, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Receipt{}, err
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,4))`, in.Email); err != nil {
		return model.Receipt{}, err
	}
	// La identidad interna es independiente del recibo público del registro.
	tag, err := tx.Exec(ctx, `INSERT INTO usuarios(id,nombre,email,registro_externo) VALUES($1,$2,$3,true) ON CONFLICT(email) DO NOTHING`, in.NewUserID, in.Name, in.Email)
	if err != nil {
		return model.Receipt{}, err
	}
	created := tag.RowsAffected() == 1
	var userID string
	if err = tx.QueryRow(ctx, `SELECT id::text FROM usuarios WHERE email=$1 FOR UPDATE`, in.Email).Scan(&userID); err != nil {
		return model.Receipt{}, err
	}
	var linkedUser string
	var verified bool
	err = tx.QueryRow(ctx, `SELECT usuario_id::text,verificada FROM identidades_externas WHERE partner_id=$1 AND external_user_id=$2 FOR UPDATE`, in.PartnerID, in.ExternalUserID).Scan(&linkedUser, &verified)
	if err == nil && linkedUser != userID {
		return model.Receipt{}, model.ErrConflict
	}
	if errors.Is(err, pgx.ErrNoRows) {
		var other string
		otherErr := tx.QueryRow(ctx, `SELECT external_user_id FROM identidades_externas WHERE partner_id=$1 AND usuario_id=$2`, in.PartnerID, userID).Scan(&other)
		if otherErr == nil {
			return model.Receipt{}, model.ErrConflict
		}
		if !errors.Is(otherErr, pgx.ErrNoRows) {
			return model.Receipt{}, otherErr
		}
		verified = created
		if _, err = tx.Exec(ctx, `INSERT INTO identidades_externas VALUES($1,$2,$3,$4)`, in.PartnerID, in.ExternalUserID, userID, verified); err != nil {
			return model.Receipt{}, err
		}
	} else if err != nil {
		return model.Receipt{}, err
	}
	state := "pending_verification"
	if verified {
		state = "ready"
	}
	_, err = tx.Exec(ctx, `INSERT INTO registros_externos(id,partner_id,evento_id,clave,huella,usuario_id,external_user_id,plataforma,puntos,motivo,referencia,estado,token_hash,token_cifrado,token_expira) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`, in.ID, in.PartnerID, in.EventID, in.Key, fingerprint, userID, in.ExternalUserID, in.Platform, in.Points, in.Reason, in.Reference, state, tokenHash, encrypted, expires)
	if err != nil {
		return model.Receipt{}, err
	}
	if !verified {
		if _, err = tx.Exec(ctx, `INSERT INTO correos_pendientes(registro_id,tipo) VALUES($1,'verification')`, in.ID); err != nil {
			return model.Receipt{}, err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO claves_registro VALUES($1,$2,$3)`, in.PartnerID, in.Key, in.ID); err != nil {
		return model.Receipt{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Receipt{}, err
	}
	return model.Receipt{ID: in.ID, EventID: in.EventID, ExternalUserID: in.ExternalUserID, Status: "accepted"}, nil
}

func (p *Postgres) Registration(ctx context.Context, partner, event string) (model.Receipt, error) {
	var r model.Receipt
	var state string
	err := p.pool.QueryRow(ctx, `SELECT id::text,evento_id,external_user_id,estado FROM registros_externos WHERE partner_id=$1 AND evento_id=$2`, partner, event).Scan(&r.ID, &r.EventID, &r.ExternalUserID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, model.ErrNotFound
	}
	if state == "completed" {
		r.Status = "completed"
	} else {
		r.Status = "accepted"
	}
	return r, err
}

func (p *Postgres) Activate(ctx context.Context, tokenHash, passwordHash []byte, now time.Time) error {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var id, user, partner, external string
	err = tx.QueryRow(ctx, `SELECT id::text,usuario_id::text,partner_id::text,external_user_id FROM registros_externos WHERE token_hash=$1 AND NOT token_consumido AND token_expira>$2 FOR UPDATE`, tokenHash, now).Scan(&id, &user, &partner, &external)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	var current []byte
	if err = tx.QueryRow(ctx, `SELECT password_hash FROM usuarios WHERE id=$1 FOR UPDATE`, user).Scan(&current); err != nil {
		return err
	}
	if len(current) == 0 {
		if len(passwordHash) == 0 {
			return model.ErrInvalid
		}
		if _, err = tx.Exec(ctx, `UPDATE usuarios SET password_hash=$2 WHERE id=$1 AND password_hash IS NULL`, user, passwordHash); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `UPDATE identidades_externas SET verificada=true WHERE partner_id=$1 AND external_user_id=$2`, partner, external); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE registros_externos SET token_consumido=true,token_cifrado=NULL,estado=CASE WHEN estado='pending_verification' THEN 'ready' ELSE estado END WHERE id=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) Password(ctx context.Context, email string) (model.User, []byte, error) {
	var u model.User
	var hash []byte
	err := p.pool.QueryRow(ctx, `SELECT id::text,nombre,email,registro_externo,rol,password_hash FROM usuarios WHERE email=$1`, email).Scan(&u.ID, &u.Name, &u.Email, &u.External, &u.Role, &hash)
	return u, hash, err
}
func (p *Postgres) User(ctx context.Context, id string) (model.User, error) {
	var u model.User
	err := p.pool.QueryRow(ctx, `SELECT id::text,nombre,email,registro_externo,rol FROM usuarios WHERE id=$1`, id).Scan(&u.ID, &u.Name, &u.Email, &u.External, &u.Role)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, model.ErrUnauthorized
	}
	return u, err
}

func (p *Postgres) UpgradePassword(ctx context.Context, id string, old, upgraded []byte) error {
	tag, err := p.pool.Exec(ctx, `UPDATE usuarios SET password_hash=$3 WHERE id=$1 AND password_hash=$2`, id, old, upgraded)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return model.ErrUnauthorized
	}
	return nil
}

func (p *Postgres) ProcessRegistration(ctx context.Context, fulfill func(context.Context, model.Fulfillment) (int64, error), generate func(string) ([]byte, []byte, error), now time.Time) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var f model.Fulfillment
	err = tx.QueryRow(ctx, `SELECT id::text,partner_id::text,usuario_id::text,external_user_id,puntos,motivo,referencia,evento_id FROM registros_externos WHERE estado='ready' AND proximo_intento<=now() ORDER BY proximo_intento,creado_en LIMIT 1 FOR UPDATE SKIP LOCKED`).Scan(&f.RegistrationID, &f.PartnerID, &f.UserID, &f.ExternalUserID, &f.Points, &f.Reason, &f.Reference, &f.EventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	balance, callErr := fulfill(ctx, f)
	if callErr != nil {
		_, err = tx.Exec(ctx, `UPDATE registros_externos SET intentos=intentos+1,proximo_intento=now()+make_interval(secs=>LEAST(3600,5*(intentos+1))) WHERE id=$1`, f.RegistrationID)
	} else {
		var consumed bool
		if err = tx.QueryRow(ctx, `SELECT token_consumido FROM registros_externos WHERE id=$1`, f.RegistrationID).Scan(&consumed); err != nil {
			return true, err
		}
		if !consumed {
			hash, encrypted, err := generate(f.RegistrationID)
			if err != nil {
				return true, err
			}
			if _, err = tx.Exec(ctx, `UPDATE registros_externos SET token_hash=$2,token_cifrado=$3,token_expira=$4 WHERE id=$1`, f.RegistrationID, hash, encrypted, now.Add(24*time.Hour)); err != nil {
				return true, err
			}
		}
		_, err = tx.Exec(ctx, `UPDATE registros_externos SET estado='completed',saldo_acreditado=$2 WHERE id=$1`, f.RegistrationID, balance)
		if err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO correos_pendientes(registro_id,tipo) VALUES($1,'welcome') ON CONFLICT DO NOTHING`, f.RegistrationID)
		}
	}
	if err != nil {
		return true, err
	}
	if err = tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, callErr
}

func (p *Postgres) ProcessEmail(ctx context.Context, send func(context.Context, string, []byte, int64, bool, string, string) error) (bool, error) {
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	var mailID int64
	var id, email, kind string
	var token []byte
	var balance int64
	var needsPassword bool
	err = tx.QueryRow(ctx, `SELECT c.id,r.id::text,u.email,r.token_cifrado,COALESCE(r.saldo_acreditado,0),u.password_hash IS NULL,c.tipo FROM correos_pendientes c JOIN registros_externos r ON r.id=c.registro_id JOIN usuarios u ON u.id=r.usuario_id WHERE c.enviado_en IS NULL AND c.proximo_intento<=now() ORDER BY c.proximo_intento,c.id LIMIT 1 FOR UPDATE OF c SKIP LOCKED`).Scan(&mailID, &id, &email, &token, &balance, &needsPassword, &kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	callErr := send(ctx, email, token, balance, needsPassword, id, kind)
	if callErr == nil {
		_, err = tx.Exec(ctx, `UPDATE correos_pendientes SET enviado_en=now() WHERE id=$1`, mailID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE correos_pendientes SET intentos=intentos+1,proximo_intento=now()+make_interval(secs=>LEAST(3600,5*(intentos+1))) WHERE id=$1`, mailID)
	}
	if err != nil {
		return true, err
	}
	if err = tx.Commit(ctx); err != nil {
		return true, err
	}
	return true, callErr
}
