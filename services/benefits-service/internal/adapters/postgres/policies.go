package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/jackc/pgx/v5"
)

// El bloqueo por integración serializa el presupuesto diario, incluso cuando
// dos eventos afectan cuentas distintas (ADR-016). Solo se toma si existe un
// presupuesto: sin él no hay consumo que proteger y las acreditaciones de la
// integración siguen en paralelo. PartnerActivo mantiene FOR SHARE sobre la
// fila, así que la política leída no cambia antes de que termine la transacción.
func (t *transaction) AutorizarCredito(ctx context.Context, partner string, puntos int64, evento string, ahora time.Time) error {
	var permitido, requiereEvento bool
	var maximo int64
	var diario *int64
	if err := t.tx.QueryRow(ctx, `SELECT permite_puntos,max_puntos_operacion,max_puntos_dia,requiere_evento FROM partners WHERE id=$1`, partner).Scan(&permitido, &maximo, &diario, &requiereEvento); err != nil {
		return err
	}
	if !permitido {
		return domain.ErrPartnerNoAutorizado
	}
	if err := domain.ValidarEvento(requiereEvento, evento); err != nil {
		return err
	}
	politica := domain.PoliticaAcreditacion{Permitido: permitido, MaximoOperacion: maximo, MaximoDia: diario}
	if diario != nil && puntos > 0 && permitido {
		if _, err := t.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,1))`, partner); err != nil {
			return err
		}
		err := t.tx.QueryRow(ctx, `SELECT COALESCE(sum(puntos),0) FROM movimientos_puntos WHERE partner_id=$1 AND aplicado_en >= date_trunc('day',$2::timestamptz AT TIME ZONE 'America/Argentina/Buenos_Aires') AT TIME ZONE 'America/Argentina/Buenos_Aires' AND aplicado_en < (date_trunc('day',$2::timestamptz AT TIME ZONE 'America/Argentina/Buenos_Aires')+interval '1 day') AT TIME ZONE 'America/Argentina/Buenos_Aires'`, partner, ahora).Scan(&politica.UsadosHoy)
		if err != nil {
			return err
		}
	}
	return politica.Validar(puntos)
}

func (t *transaction) EventoAcreditado(ctx context.Context, partner, evento, huella string) (domain.ResultadoOperacion, bool, error) {
	// Se toma antes de buscar para que el segundo request vea el commit del primero.
	if _, err := t.tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,2))`, partner+":"+evento); err != nil {
		return domain.ResultadoOperacion{}, false, err
	}
	var hash string
	var m domain.DatosMovimiento
	var saldo int64
	err := t.tx.QueryRow(ctx, `SELECT e.huella,m.id::text,m.cuenta_id::text,m.partner_id::text,m.puntos,m.secuencia,m.motivo,m.referencia,m.ocurrido_en,m.aplicado_en,e.saldo_resultante FROM eventos_acreditados e JOIN movimientos_puntos m ON m.id=e.movimiento_id WHERE e.partner_id=$1 AND e.evento_id=$2`, partner, evento).Scan(&hash, &m.ID, &m.CuentaID, &m.PartnerID, &m.Puntos, &m.Secuencia, &m.Motivo, &m.Referencia, &m.OcurridoEn, &m.AplicadoEn, &saldo)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ResultadoOperacion{}, false, nil
	}
	if err != nil {
		return domain.ResultadoOperacion{}, false, err
	}
	if hash != huella {
		return domain.ResultadoOperacion{}, false, domain.ErrConflictoIdempotencia
	}
	// timestamptz puede decodificarse en la zona local del proceso. El replay
	// debe conservar la representación UTC de la acreditación original.
	m.OcurridoEn = m.OcurridoEn.UTC()
	m.AplicadoEn = m.AplicadoEn.UTC()
	movimiento, err := domain.NuevoMovimiento(m)
	return domain.ResultadoOperacion{Movimiento: movimiento, Saldo: saldo}, true, err
}

func (t *transaction) GuardarEvento(ctx context.Context, partner, evento, huella, movimiento string) error {
	_, err := t.tx.Exec(ctx, `INSERT INTO eventos_acreditados(partner_id,evento_id,huella,movimiento_id,saldo_resultante) SELECT $1,$2,$3,$4,c.saldo FROM cuentas c JOIN movimientos_puntos m ON m.cuenta_id=c.id WHERE m.id=$4`, partner, evento, huella, movimiento)
	return err
}

func (t *transaction) autorizarConsulta(ctx context.Context, partner string) error {
	var permitido bool
	if err := t.tx.QueryRow(ctx, `SELECT permite_consulta FROM partners WHERE id=$1`, partner).Scan(&permitido); err != nil {
		return err
	}
	if !permitido {
		return domain.ErrPartnerNoAutorizado
	}
	return nil
}
