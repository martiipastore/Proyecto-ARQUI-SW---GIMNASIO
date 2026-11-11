package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Cuponera interna del Club (D-94, migración 009). Canje, débito y saldo se
// confirman juntos con la cuenta bloqueada, como el resto de las escrituras
// del ledger; la base vuelve a verificar saldo ≥ 0 y la coherencia cupón-débito.

var _ ports.Cuponera = (*DB)(nil)

const columnasBeneficio = `b.id::text,b.nombre,b.descripcion,b.costo,to_char(b.vigente_desde,'YYYY-MM-DD'),COALESCE(to_char(b.vigente_hasta,'YYYY-MM-DD'),''),b.activo,b.creado_en,b.actualizado_en,(SELECT count(*) FROM canjes k WHERE k.beneficio_id=b.id)`

func scanBeneficio(row pgx.Row) (domain.Beneficio, error) {
	var b domain.Beneficio
	err := row.Scan(&b.ID, &b.Nombre, &b.Descripcion, &b.Costo, &b.VigenteDesde, &b.VigenteHasta, &b.Activo, &b.CreadoEn, &b.ActualizadoEn, &b.Canjes)
	return b, err
}

const columnasCanje = `k.id::text,k.cuenta_id::text,k.usuario_id::text,k.beneficio_id::text,k.beneficio_nombre,k.codigo,k.estado,k.movimiento_id::text,COALESCE(k.anulacion_id::text,''),COALESCE(k.motivo_anulacion,''),k.puntos,k.creado_en,k.usado_en,k.anulado_en`

func scanCanje(row pgx.Row) (domain.CanjeBeneficio, error) {
	var k domain.CanjeBeneficio
	err := row.Scan(&k.ID, &k.CuentaID, &k.UsuarioID, &k.BeneficioID, &k.BeneficioNombre, &k.Codigo, &k.Estado, &k.MovimientoID, &k.AnulacionID, &k.MotivoAnulacion, &k.Puntos, &k.CreadoEn, &k.UsadoEn, &k.AnuladoEn)
	return k, err
}

func detalleBeneficio(b domain.Beneficio) []byte {
	var hasta any
	if b.VigenteHasta != "" {
		hasta = b.VigenteHasta
	}
	out, _ := json.Marshal(map[string]any{"beneficio_id": b.ID, "nombre": b.Nombre, "costo": b.Costo, "vigente_desde": b.VigenteDesde, "vigente_hasta": hasta, "activo": b.Activo})
	return out
}

func fechaNula(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (d *DB) ListarBeneficios(ctx context.Context, soloDisponibles bool, hoy string) (out []domain.Beneficio, err error) {
	ctx, finish := startDB(ctx, "postgres.listar_beneficios")
	defer func() { finish(err) }()
	defer func() { err = storageError(err) }()
	out = []domain.Beneficio{}
	query := `SELECT ` + columnasBeneficio + ` FROM beneficios b ORDER BY b.activo DESC, b.costo, b.nombre, b.id`
	args := []any{}
	if soloDisponibles {
		query = `SELECT ` + columnasBeneficio + ` FROM beneficios b WHERE b.activo AND b.vigente_desde <= $1::date AND (b.vigente_hasta IS NULL OR $1::date <= b.vigente_hasta) ORDER BY b.costo, b.nombre, b.id`
		args = append(args, hoy)
	}
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		b, err := scanBeneficio(rows)
		if err != nil {
			return out, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (d *DB) CrearBeneficio(ctx context.Context, a ports.Auditoria, b domain.Beneficio) (out domain.Beneficio, err error) {
	err = d.escribir(ctx, "postgres.crear_beneficio", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO beneficios(id,nombre,descripcion,costo,vigente_desde,vigente_hasta,activo,creado_en,actualizado_en) VALUES($1,$2,$3,$4,$5::date,$6::date,$7,$8,$9)`,
			b.ID, b.Nombre, b.Descripcion, b.Costo, b.VigenteDesde, fechaNula(b.VigenteHasta), b.Activo, b.CreadoEn, b.ActualizadoEn); err != nil {
			return err
		}
		if err := auditar(ctx, tx, a, "ALTA_BENEFICIO", nil, nil, detalleBeneficio(b)); err != nil {
			return err
		}
		var err error
		out, err = scanBeneficio(tx.QueryRow(ctx, `SELECT `+columnasBeneficio+` FROM beneficios b WHERE b.id=$1`, b.ID))
		return err
	})
	return out, err
}

func (d *DB) ActualizarBeneficio(ctx context.Context, a ports.Auditoria, b domain.Beneficio) (out domain.Beneficio, err error) {
	err = d.escribir(ctx, "postgres.actualizar_beneficio", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE beneficios SET nombre=$2,descripcion=$3,costo=$4,vigente_desde=$5::date,vigente_hasta=$6::date,activo=$7,actualizado_en=$8 WHERE id=$1`,
			b.ID, b.Nombre, b.Descripcion, b.Costo, b.VigenteDesde, fechaNula(b.VigenteHasta), b.Activo, b.ActualizadoEn)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return domain.ErrBeneficioNoEncontrado
		}
		if err := auditar(ctx, tx, a, "CAMBIO_BENEFICIO", nil, nil, detalleBeneficio(b)); err != nil {
			return err
		}
		out, err = scanBeneficio(tx.QueryRow(ctx, `SELECT `+columnasBeneficio+` FROM beneficios b WHERE b.id=$1`, b.ID))
		return err
	})
	return out, err
}

// sinCuenta representa a quien todavía no tiene cuenta de puntos: saldo 0, así
// que el dominio rechaza el canje sin crear nada. El id nunca se persiste.
const sinCuenta = "00000000-0000-4000-8000-000000000000"

// Canjear confirma cupón, débito, saldo y clave idempotente en una transacción.
// El beneficio se lee con FOR SHARE: un cambio de costo concurrente espera, y
// débito y cupón registran el mismo valor (CL-18). Los rechazos de negocio no
// se guardan y se vuelven a evaluar al reintentar.
func (d *DB) Canjear(ctx context.Context, p ports.PedidoCanje, aplicar ports.AplicarCanje) (out domain.ResultadoCanje, repetido bool, err error) {
	err = d.escribir(ctx, "postgres.canjear_beneficio", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "canje:"+p.UsuarioID+":"+p.Clave); err != nil {
			return err
		}
		var previo, huella string
		var saldo int64
		err := tx.QueryRow(ctx, `SELECT id::text,huella,saldo_resultante FROM canjes WHERE usuario_id=$1 AND clave=$2`, p.UsuarioID, p.Clave).Scan(&previo, &huella, &saldo)
		if err == nil {
			if huella != p.Huella {
				return domain.ErrConflictoIdempotencia
			}
			k, err := scanCanje(tx.QueryRow(ctx, `SELECT `+columnasCanje+` FROM canjes k WHERE k.id=$1`, previo))
			if err != nil {
				return err
			}
			out, repetido = domain.ResultadoCanje{Canje: k, Saldo: saldo}, true
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		b, err := scanBeneficio(tx.QueryRow(ctx, `SELECT `+columnasBeneficio+` FROM beneficios b WHERE b.id=$1 FOR SHARE`, p.BeneficioID))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrBeneficioNoEncontrado
		}
		if err != nil {
			return err
		}
		cuenta := domain.CuentaBeneficios{}
		var cuentaID string
		var saldoActual, secuencia int64
		err = tx.QueryRow(ctx, `SELECT id::text,saldo,ultima_secuencia FROM cuentas WHERE alumno_id=$1 FOR UPDATE`, p.UsuarioID).Scan(&cuentaID, &saldoActual, &secuencia)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			cuenta, err = domain.NuevaCuenta(sinCuenta, 0, 0)
		case err == nil:
			cuenta, err = domain.NuevaCuenta(cuentaID, saldoActual, secuencia)
		}
		if err != nil {
			return err
		}
		nueva, mov, canje, err := aplicar(cuenta, b)
		if err != nil {
			return err
		}
		if nueva.ID() == sinCuenta {
			return domain.ErrSaldoInsuficiente
		}
		m := mov.Datos()
		if _, err := tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,origen_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			m.ID, m.CuentaID, m.Secuencia, m.Puntos, m.Motivo, m.Referencia, m.OcurridoEn, m.AplicadoEn, m.Tipo, m.OrigenID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO canjes(id,cuenta_id,usuario_id,beneficio_id,beneficio_nombre,puntos,codigo,estado,movimiento_id,clave,huella,saldo_resultante,creado_en) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			canje.ID, canje.CuentaID, canje.UsuarioID, canje.BeneficioID, canje.BeneficioNombre, canje.Puntos, canje.Codigo, canje.Estado, canje.MovimientoID, p.Clave, p.Huella, nueva.Saldo(), canje.CreadoEn); err != nil {
			var pg *pgconn.PgError
			if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "canjes_codigo_key" {
				return domain.ErrCodigoCanjeEnConflicto
			}
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE cuentas SET saldo=$2,ultima_secuencia=$3 WHERE id=$1`, nueva.ID(), nueva.Saldo(), nueva.Secuencia()); err != nil {
			return err
		}
		out = domain.ResultadoCanje{Canje: canje, Saldo: nueva.Saldo()}
		return nil
	})
	return out, repetido, err
}

func (d *DB) ListarCanjes(ctx context.Context, f ports.FiltroCanjes) (out ports.PaginaCanjes, err error) {
	ctx, finish := startDB(ctx, "postgres.listar_canjes")
	defer func() { finish(err) }()
	defer func() { err = storageError(err) }()
	out.Items = []domain.CanjeBeneficio{}
	where, args := []string{}, []any{}
	for _, filtro := range []struct{ columna, valor string }{{"k.usuario_id", f.UsuarioID}, {"k.codigo", f.Codigo}, {"k.estado", f.Estado}} {
		if filtro.valor != "" {
			args = append(args, filtro.valor)
			cast := ""
			if filtro.columna == "k.usuario_id" {
				cast = "::uuid"
			}
			where = append(where, fmt.Sprintf("%s=$%d%s", filtro.columna, len(args), cast))
		}
	}
	cond := ""
	if len(where) > 0 {
		cond = " WHERE " + strings.Join(where, " AND ")
	}
	if err = d.pool.QueryRow(ctx, `SELECT count(*) FROM canjes k`+cond, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	args = append(args, f.Tamano, int64(f.Pagina-1)*int64(f.Tamano))
	rows, err := d.pool.Query(ctx, `SELECT `+columnasCanje+` FROM canjes k`+cond+fmt.Sprintf(` ORDER BY k.creado_en DESC, k.id LIMIT $%d OFFSET $%d`, len(args)-1, len(args)), args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		k, err := scanCanje(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, k)
	}
	return out, rows.Err()
}

func (d *DB) UsarCanje(ctx context.Context, a ports.Auditoria, id string, usar ports.AplicarUso) (out domain.CanjeBeneficio, cambio bool, err error) {
	err = d.escribir(ctx, "postgres.usar_canje", func(tx pgx.Tx) error {
		k, err := scanCanje(tx.QueryRow(ctx, `SELECT `+columnasCanje+` FROM canjes k WHERE k.id=$1 FOR UPDATE`, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCanjeNoEncontrado
		}
		if err != nil {
			return err
		}
		out, cambio, err = usar(k)
		if err != nil || !cambio {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE canjes SET estado=$2,usado_en=$3 WHERE id=$1`, out.ID, out.Estado, out.UsadoEn); err != nil {
			return err
		}
		detalle, _ := json.Marshal(map[string]any{"canje_id": out.ID, "codigo": out.Codigo, "beneficio_id": out.BeneficioID})
		return auditar(ctx, tx, a, "USO_CANJE", nil, nil, detalle)
	})
	return out, cambio, err
}

// AnularCanje bloquea primero la cuenta y después el cupón, el mismo orden que
// el canje, y confirma compensatorio, saldo, estado y auditoría juntos.
func (d *DB) AnularCanje(ctx context.Context, a ports.Auditoria, id string, anular ports.AplicarAnulacion) (out domain.ResultadoCanje, cambio bool, err error) {
	err = d.escribir(ctx, "postgres.anular_canje", func(tx pgx.Tx) error {
		var cuentaID string
		err := tx.QueryRow(ctx, `SELECT cuenta_id::text FROM canjes WHERE id=$1`, id).Scan(&cuentaID)
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrCanjeNoEncontrado
		}
		if err != nil {
			return err
		}
		var saldo, secuencia int64
		if err := tx.QueryRow(ctx, `SELECT saldo,ultima_secuencia FROM cuentas WHERE id=$1 FOR UPDATE`, cuentaID).Scan(&saldo, &secuencia); err != nil {
			return err
		}
		k, err := scanCanje(tx.QueryRow(ctx, `SELECT `+columnasCanje+` FROM canjes k WHERE k.id=$1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if k.Estado == domain.CanjeAnulado {
			out = domain.ResultadoCanje{Canje: k, Saldo: saldo}
			return nil
		}
		cuenta, err := domain.NuevaCuenta(cuentaID, saldo, secuencia)
		if err != nil {
			return err
		}
		debito, err := scanMovimiento(tx.QueryRow(ctx, `SELECT `+columnasMovimiento+` FROM movimientos_puntos WHERE id=$1`, k.MovimientoID))
		if err != nil {
			return err
		}
		nueva, mov, anulado, err := anular(cuenta, k, debito)
		if err != nil {
			return err
		}
		m := mov.Datos()
		if _, err := tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en,tipo,revierte_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			m.ID, m.CuentaID, m.Secuencia, m.Puntos, m.Motivo, m.Referencia, m.OcurridoEn, m.AplicadoEn, m.Tipo, m.RevierteID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE cuentas SET saldo=$2,ultima_secuencia=$3 WHERE id=$1`, nueva.ID(), nueva.Saldo(), nueva.Secuencia()); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE canjes SET estado=$2,anulacion_id=$3,motivo_anulacion=$4,anulado_en=$5 WHERE id=$1`, anulado.ID, anulado.Estado, anulado.AnulacionID, anulado.MotivoAnulacion, anulado.AnuladoEn); err != nil {
			return err
		}
		detalle, _ := json.Marshal(map[string]any{"canje_id": anulado.ID, "codigo": anulado.Codigo, "puntos": m.Puntos, "motivo": anulado.MotivoAnulacion})
		if err := auditar(ctx, tx, a, "ANULACION_CANJE", nil, &m.ID, detalle); err != nil {
			return err
		}
		out, cambio = domain.ResultadoCanje{Canje: anulado, Saldo: nueva.Saldo()}, true
		return nil
	})
	return out, cambio, err
}
