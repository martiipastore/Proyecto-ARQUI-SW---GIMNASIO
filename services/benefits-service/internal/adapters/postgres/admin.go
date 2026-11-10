package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ ports.AdministracionClub = (*DB)(nil)

const columnasIntegracion = `p.id::text,p.nombre,p.activo,p.permite_consulta,p.permite_puntos,p.permite_registro,p.requiere_evento,p.max_puntos_operacion,p.max_puntos_dia,p.max_solicitudes_minuto,p.creada_en,(SELECT count(*) FROM vinculaciones v WHERE v.partner_id=p.id)`

func scanIntegracion(row pgx.Row) (domain.Integracion, error) {
	var i domain.Integracion
	c := &i.Config
	err := row.Scan(&i.ID, &c.Nombre, &c.Activo, &c.PermiteConsulta, &c.PermitePuntos, &c.PermiteRegistro, &c.RequiereEvento, &c.MaxPuntosOperacion, &c.MaxPuntosDia, &c.MaxSolicitudesMinuto, &i.CreadaEn, &i.Vinculaciones)
	return i, err
}

// detalleConfiguracion es la representación auditada: nunca incluye credenciales.
func detalleConfiguracion(c domain.ConfiguracionIntegracion) []byte {
	out, _ := json.Marshal(map[string]any{"nombre": c.Nombre, "activo": c.Activo, "permite_consulta": c.PermiteConsulta, "permite_puntos": c.PermitePuntos, "permite_registro": c.PermiteRegistro, "requiere_evento": c.RequiereEvento, "max_puntos_operacion": c.MaxPuntosOperacion, "max_puntos_dia": c.MaxPuntosDia, "max_solicitudes_minuto": c.MaxSolicitudesMinuto})
	return out
}

func auditar(ctx context.Context, tx pgx.Tx, a ports.Auditoria, accion string, partner, movimiento *string, detalle []byte) error {
	if detalle == nil {
		detalle = []byte("{}")
	}
	_, err := tx.Exec(ctx, `INSERT INTO auditoria_club(id,administrador_id,accion,partner_id,movimiento_id,detalle,ocurrido_en) VALUES($1,$2,$3,$4,$5,$6,$7)`, a.EventoID, a.AdministradorID, accion, partner, movimiento, detalle, a.Ahora)
	return err
}

// escribir ejecuta fn en una transacción y traduce fallas de disponibilidad.
func (d *DB) escribir(ctx context.Context, span string, fn func(pgx.Tx) error) (err error) {
	ctx, finish := startDB(ctx, span)
	defer func() { finish(err) }()
	defer func() { err = storageError(err) }()
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (d *DB) ListarIntegraciones(ctx context.Context) (out []domain.Integracion, err error) {
	ctx, finish := startDB(ctx, "postgres.listar_integraciones")
	defer func() { finish(err) }()
	defer func() { err = storageError(err) }()
	out = []domain.Integracion{}
	rows, err := d.pool.Query(ctx, `SELECT `+columnasIntegracion+` FROM partners p ORDER BY p.creada_en,p.nombre,p.id`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanIntegracion(rows)
		if err != nil {
			return out, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (d *DB) CrearIntegracion(ctx context.Context, a ports.Auditoria, id string, c domain.ConfiguracionIntegracion, hash [32]byte) (out domain.Integracion, err error) {
	err = d.escribir(ctx, "postgres.crear_integracion", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO partners(id,nombre,api_key_hash,activo,permite_consulta,permite_puntos,permite_registro,requiere_evento,max_puntos_operacion,max_puntos_dia,max_solicitudes_minuto,creada_en) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
			id, c.Nombre, hash[:], c.Activo, c.PermiteConsulta, c.PermitePuntos, c.PermiteRegistro, c.RequiereEvento, c.MaxPuntosOperacion, c.MaxPuntosDia, c.MaxSolicitudesMinuto, a.Ahora); err != nil {
			return err
		}
		if err := auditar(ctx, tx, a, "ALTA_INTEGRACION", &id, nil, detalleConfiguracion(c)); err != nil {
			return err
		}
		var err error
		out, err = scanIntegracion(tx.QueryRow(ctx, `SELECT `+columnasIntegracion+` FROM partners p WHERE p.id=$1`, id))
		return err
	})
	return out, err
}

func (d *DB) ActualizarIntegracion(ctx context.Context, a ports.Auditoria, id string, c domain.ConfiguracionIntegracion) (out domain.Integracion, err error) {
	err = d.escribir(ctx, "postgres.actualizar_integracion", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE partners SET nombre=$2,activo=$3,permite_consulta=$4,permite_puntos=$5,permite_registro=$6,requiere_evento=$7,max_puntos_operacion=$8,max_puntos_dia=$9,max_solicitudes_minuto=$10 WHERE id=$1`,
			id, c.Nombre, c.Activo, c.PermiteConsulta, c.PermitePuntos, c.PermiteRegistro, c.RequiereEvento, c.MaxPuntosOperacion, c.MaxPuntosDia, c.MaxSolicitudesMinuto)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrIntegracionNoEncontrada
		}
		if err := auditar(ctx, tx, a, "CAMBIO_INTEGRACION", &id, nil, detalleConfiguracion(c)); err != nil {
			return err
		}
		out, err = scanIntegracion(tx.QueryRow(ctx, `SELECT `+columnasIntegracion+` FROM partners p WHERE p.id=$1`, id))
		return err
	})
	return out, err
}

func (d *DB) RotarCredencial(ctx context.Context, a ports.Auditoria, id string, hash [32]byte) error {
	return d.escribir(ctx, "postgres.rotar_credencial", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE partners SET api_key_hash=$2 WHERE id=$1`, id, hash[:])
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrIntegracionNoEncontrada
		}
		return auditar(ctx, tx, a, "ROTACION_CREDENCIAL", &id, nil, nil)
	})
}

func (d *DB) Vincular(ctx context.Context, a ports.Auditoria, partner, usuario, externo string) (creada bool, err error) {
	err = d.escribir(ctx, "postgres.vincular_cuenta", func(tx pgx.Tx) error {
		var encontrado string
		err := tx.QueryRow(ctx, `SELECT id::text FROM partners WHERE id=$1 FOR SHARE`, partner).Scan(&encontrado)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrIntegracionNoEncontrada
		}
		if err != nil {
			return err
		}
		var cuenta string
		err = tx.QueryRow(ctx, `SELECT id::text FROM cuentas WHERE alumno_id=$1`, usuario).Scan(&cuenta)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCuentaNoVinculada
		}
		if err != nil {
			return err
		}
		// Serializa vinculaciones del mismo partner para responder 409 estable.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "vinculo:"+partner); err != nil {
			return err
		}
		var otraCuenta, otroExterno string
		err = tx.QueryRow(ctx, `SELECT cuenta_id::text FROM vinculaciones WHERE partner_id=$1 AND external_user_id=$2`, partner, externo).Scan(&otraCuenta)
		if err == nil {
			if otraCuenta == cuenta {
				return nil // Repetir el mismo vínculo no tiene efectos nuevos.
			}
			return domain.ErrVinculoEnConflicto // CA-10.4
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		err = tx.QueryRow(ctx, `SELECT external_user_id FROM vinculaciones WHERE partner_id=$1 AND cuenta_id=$2`, partner, cuenta).Scan(&otroExterno)
		if err == nil {
			return domain.ErrVinculoEnConflicto
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES($1,$2,$3)`, partner, externo, cuenta); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				return domain.ErrVinculoEnConflicto
			}
			return err
		}
		detalle, _ := json.Marshal(map[string]string{"usuario_id": usuario, "external_user_id": externo})
		creada = true
		return auditar(ctx, tx, a, "VINCULACION", &partner, nil, detalle)
	})
	return creada, err
}

const columnasMovimiento = `id::text,cuenta_id::text,COALESCE(partner_id::text,''),puntos,secuencia,motivo,referencia,ocurrido_en,aplicado_en,tipo,COALESCE(origen_id::text,''),COALESCE(revierte_id::text,'')`

func scanMovimiento(row pgx.Row) (domain.DatosMovimiento, error) {
	var m domain.DatosMovimiento
	if err := row.Scan(&m.ID, &m.CuentaID, &m.PartnerID, &m.Puntos, &m.Secuencia, &m.Motivo, &m.Referencia, &m.OcurridoEn, &m.AplicadoEn, &m.Tipo, &m.OrigenID, &m.RevierteID); err != nil {
		return m, err
	}
	if m.Tipo == "CREDITO_PARTNER" {
		m.Tipo = ""
	}
	return m, nil
}

// paginaCuenta lee una página del ledger completo en orden de aplicación (RN-42)
// e informa qué originales de la página tienen compensatorio.
func paginaCuenta(ctx context.Context, tx pgx.Tx, cuenta string, page, size int) (domain.PaginaMovimientos, error) {
	out := domain.PaginaMovimientos{Movimientos: []domain.MovimientoPuntos{}, RevertidoPor: map[string]string{}}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM movimientos_puntos WHERE cuenta_id=$1`, cuenta).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := tx.Query(ctx, `SELECT `+columnasMovimiento+` FROM movimientos_puntos WHERE cuenta_id=$1 ORDER BY secuencia DESC LIMIT $2 OFFSET $3`, cuenta, size, int64(page-1)*int64(size))
	if err != nil {
		return out, err
	}
	ids := []string{}
	for rows.Next() {
		m, err := scanMovimiento(rows)
		if err != nil {
			rows.Close()
			return out, err
		}
		mov, err := domain.NuevoMovimiento(m)
		if err != nil {
			rows.Close()
			return out, err
		}
		out.Movimientos = append(out.Movimientos, mov)
		ids = append(ids, m.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	if len(ids) == 0 {
		return out, nil
	}
	rev, err := tx.Query(ctx, `SELECT revierte_id::text,id::text FROM movimientos_puntos WHERE revierte_id = ANY($1::uuid[])`, ids)
	if err != nil {
		return out, err
	}
	defer rev.Close()
	for rev.Next() {
		var original, compensatorio string
		if err := rev.Scan(&original, &compensatorio); err != nil {
			return out, err
		}
		out.RevertidoPor[original] = compensatorio
	}
	return out, rev.Err()
}

func (d *DB) CuentaDeUsuario(ctx context.Context, usuario string, page, size int) (out domain.CuentaAdministrada, err error) {
	ctx, finish := startDB(ctx, "postgres.cuenta_de_usuario")
	defer func() { finish(err) }()
	defer func() { err = storageError(err) }()
	tx, err := d.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	out.UsuarioID = usuario
	err = tx.QueryRow(ctx, `SELECT id::text,saldo,creada_en FROM cuentas WHERE alumno_id=$1`, usuario).Scan(&out.CuentaID, &out.Saldo, &out.CreadaEn)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, domain.ErrCuentaNoVinculada
	}
	if err != nil {
		return out, err
	}
	if out.Pagina, err = paginaCuenta(ctx, tx, out.CuentaID, page, size); err != nil {
		return out, err
	}
	out.Vinculaciones = []domain.Vinculacion{}
	rows, err := tx.Query(ctx, `SELECT v.partner_id::text,p.nombre,v.external_user_id FROM vinculaciones v JOIN partners p ON p.id=v.partner_id WHERE v.cuenta_id=$1 ORDER BY p.nombre,v.partner_id`, out.CuentaID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v domain.Vinculacion
		if err = rows.Scan(&v.PartnerID, &v.PartnerNombre, &v.ExternalUserID); err != nil {
			rows.Close()
			return out, err
		}
		out.Vinculaciones = append(out.Vinculaciones, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	return out, tx.Commit(ctx)
}

// Revertir confirma compensatorio, saldo, clave idempotente y auditoría en la
// misma transacción, con la cuenta bloqueada como el resto de las escrituras.
// Los rechazos de negocio no se guardan: se vuelven a evaluar al reintentar.
func (d *DB) Revertir(ctx context.Context, p ports.ReversionPedida, aplicar ports.AplicarReversion) (out domain.ResultadoReversion, repetida bool, err error) {
	err = d.escribir(ctx, "postgres.revertir_movimiento", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "reversion:"+p.AdministradorID+":"+p.Clave); err != nil {
			return err
		}
		var huella, guardado string
		var saldo int64
		err := tx.QueryRow(ctx, `SELECT huella,movimiento_id::text,saldo_resultante FROM reversiones_admin WHERE administrador_id=$1 AND clave=$2`, p.AdministradorID, p.Clave).Scan(&huella, &guardado, &saldo)
		if err == nil {
			if huella != p.Huella {
				return domain.ErrConflictoIdempotencia
			}
			m, err := scanMovimiento(tx.QueryRow(ctx, `SELECT `+columnasMovimiento+` FROM movimientos_puntos WHERE id=$1`, guardado))
			if err != nil {
				return err
			}
			mov, err := domain.NuevoMovimiento(m)
			if err != nil {
				return err
			}
			out, repetida = domain.ResultadoReversion{Movimiento: mov, Saldo: saldo}, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var cuentaID string
		err = tx.QueryRow(ctx, `SELECT cuenta_id::text FROM movimientos_puntos WHERE id=$1`, p.MovimientoID).Scan(&cuentaID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrMovimientoNoEncontrado
		}
		if err != nil {
			return err
		}
		var saldoActual, secuencia int64
		if err := tx.QueryRow(ctx, `SELECT saldo,ultima_secuencia FROM cuentas WHERE id=$1 FOR UPDATE`, cuentaID).Scan(&saldoActual, &secuencia); err != nil {
			return err
		}
		cuenta, err := domain.NuevaCuenta(cuentaID, saldoActual, secuencia)
		if err != nil {
			return err
		}
		original, err := scanMovimiento(tx.QueryRow(ctx, `SELECT `+columnasMovimiento+` FROM movimientos_puntos WHERE id=$1`, p.MovimientoID))
		if err != nil {
			return err
		}
		var revertido bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM movimientos_puntos WHERE revierte_id=$1)`, p.MovimientoID).Scan(&revertido); err != nil {
			return err
		}
		nueva, mov, err := aplicar(cuenta, original, revertido)
		if err != nil {
			return err
		}
		m := mov.Datos()
		if _, err := tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,revierte_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, m.ID, m.CuentaID, m.Secuencia, m.Puntos, m.Motivo, m.Referencia, m.OcurridoEn, m.AplicadoEn, m.Tipo, m.RevierteID); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" {
				return domain.ErrMovimientoRevertido
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE cuentas SET saldo=$2,ultima_secuencia=$3 WHERE id=$1`, nueva.ID(), nueva.Saldo(), nueva.Secuencia()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO reversiones_admin(administrador_id,clave,huella,movimiento_id,saldo_resultante,creada_en) VALUES($1,$2,$3,$4,$5,$6)`, p.AdministradorID, p.Clave, p.Huella, m.ID, nueva.Saldo(), p.Ahora); err != nil {
			return err
		}
		detalle, _ := json.Marshal(map[string]any{"original_id": original.ID, "puntos": m.Puntos, "motivo": m.Motivo})
		if err := auditar(ctx, tx, p.Auditoria, "REVERSION", nil, &m.ID, detalle); err != nil {
			return err
		}
		out = domain.ResultadoReversion{Movimiento: mov, Saldo: nueva.Saldo()}
		return nil
	})
	return out, repetida, err
}
