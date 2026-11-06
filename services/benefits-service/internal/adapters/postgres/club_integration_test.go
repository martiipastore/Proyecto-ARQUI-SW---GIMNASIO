package postgres

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	benefitshttp "github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/http"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/system"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/application"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/migrations"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Reuses the isolated PostgreSQL fixture; never points at a development database.
func verificarClub(t *testing.T, db *DB, dsn string) {
	ctx := context.Background()
	t.Run("API_evento_repetido_conserva_JSON_y_precision", func(t *testing.T) {
		f := nuevaFixture(t, db)
		clock := relojPrueba{time.Date(2026, 11, 6, 12, 34, 56, 123456789, time.UTC)}
		app := application.New(db, clock, system.IDs{}, benefitshttp.ResultadoEncoder{})
		r := gin.New()
		benefitshttp.RegisterPartners(r, app, application.NewAuthenticator(db))
		do := func(key, body string) *httptest.ResponseRecorder {
			req := httptest.NewRequest("POST", "/v1/accounts/"+f.usuario+"/credits", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-API-Key", "fixture-"+f.partner)
			req.Header.Set("Idempotency-Key", key)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			return w
		}
		body := `{"points":150,"reason":"Evento confirmado","reference":"precision","eventId":"precision-001"}`
		first := do("precision-key-001", body)
		require.Equal(t, 201, first.Code)
		require.Contains(t, first.Body.String(), "2026-11-06T12:34:56.123456Z")
		replay := do("precision-key-002", body)
		require.Equal(t, 201, replay.Code)
		require.Equal(t, first.Body.String(), replay.Body.String())
		require.Equal(t, "true", replay.Header().Get("Idempotent-Replayed"))
		again := do("precision-key-002", body)
		require.Equal(t, first.Body.String(), again.Body.String())
	})
	t.Run("evento_sobrevive_retencion_respuesta_HTTP", func(t *testing.T) {
		f := nuevaFixture(t, db)
		past := time.Now().UTC().Add(-40 * 24 * time.Hour)
		app := application.New(db, relojPrueba{past}, system.IDs{})
		in := f.credito("retencion-evento-001", 150)
		in.EventoID = "hecho-no-caduca"
		first, err := app.Acreditar(ctx, in)
		require.NoError(t, err)
		tag, err := db.pool.Exec(ctx, `DELETE FROM operaciones_partner WHERE partner_id=$1`, f.partner)
		require.NoError(t, err)
		require.EqualValues(t, 1, tag.RowsAffected())
		in.Clave = "retencion-evento-002"
		replay, err := f.app.Acreditar(ctx, in)
		require.NoError(t, err)
		require.True(t, replay.Repetida)
		require.Equal(t, first.Resultado, replay.Resultado)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.EqualValues(t, 150, saldo)
		_, err = db.pool.Exec(ctx, `DELETE FROM eventos_acreditados WHERE partner_id=$1`, f.partner)
		require.Error(t, err)
	})
	t.Run("cuota_atomica_solicitudes_y_revocacion", func(t *testing.T) {
		// Si cruza exactamente el minuto se repite con otra integración: no
		// confundir dos ventanas válidas con exceso dentro de una ventana.
		for attempt := 0; attempt < 3; attempt++ {
			f := nuevaFixture(t, db)
			_, err := db.pool.Exec(ctx, `UPDATE partners SET max_solicitudes_minuto=3 WHERE id=$1`, f.partner)
			require.NoError(t, err)
			var before, after time.Time
			require.NoError(t, db.pool.QueryRow(ctx, `SELECT date_trunc('minute',now())`).Scan(&before))
			key := sha256.Sum256([]byte("fixture-" + f.partner))
			start := make(chan struct{})
			results := make(chan error, 8)
			for i := 0; i < 8; i++ {
				go func() { <-start; _, err := db.BuscarPartner(ctx, key); results <- err }()
			}
			close(start)
			accepted, throttled := 0, 0
			for i := 0; i < 8; i++ {
				err := <-results
				if err == nil {
					accepted++
				} else if errors.Is(err, domain.ErrLimiteSolicitudes) {
					throttled++
				} else {
					require.NoError(t, err)
				}
			}
			require.NoError(t, db.pool.QueryRow(ctx, `SELECT date_trunc('minute',now())`).Scan(&after))
			if !before.Equal(after) {
				continue
			}
			require.Equal(t, 3, accepted)
			require.Equal(t, 5, throttled)
			auth := application.NewAuthenticator(db)
			_, err = db.pool.Exec(ctx, `UPDATE partners SET activo=false WHERE id=$1`, f.partner)
			require.NoError(t, err)
			_, err = auth.Autenticar(ctx, "fixture-"+f.partner)
			require.ErrorIs(t, err, application.ErrPartnerInactivo)
			newHash := sha256.Sum256([]byte("rotada-" + f.partner))
			_, err = db.pool.Exec(ctx, `UPDATE partners SET api_key_hash=$2 WHERE id=$1`, f.partner, newHash[:])
			require.NoError(t, err)
			_, err = auth.Autenticar(ctx, "fixture-"+f.partner)
			require.ErrorIs(t, err, application.ErrCredencialInvalida)
			return
		}
		t.Fatal("No se pudo verificar una ventana de cuota estable")
	})
	t.Run("migracion_desde_ledger_previo_con_historial", func(t *testing.T) {
		_, err := db.pool.Exec(ctx, `CREATE SCHEMA club_upgrade`)
		require.NoError(t, err)
		connection, err := url.Parse(dsn)
		require.NoError(t, err)
		query := connection.Query()
		query.Set("search_path", "club_upgrade")
		connection.RawQuery = query.Encode()
		old, err := Open(ctx, connection.String())
		require.NoError(t, err)
		defer old.Close()
		_, err = old.pool.Exec(ctx, `CREATE TABLE schema_migrations(version text PRIMARY KEY,huella text NOT NULL,aplicada_en timestamptz NOT NULL DEFAULT now())`)
		require.NoError(t, err)
		for _, name := range []string{"001_ledger.sql", "002_fecha_cuenta.sql"} {
			sql, err := migrations.Files.ReadFile(name)
			require.NoError(t, err)
			_, err = old.pool.Exec(ctx, string(sql))
			require.NoError(t, err)
			_, err = old.pool.Exec(ctx, `INSERT INTO schema_migrations(version,huella) VALUES($1,$2)`, name, fmt.Sprintf("%x", sha256.Sum256(sql)))
			require.NoError(t, err)
		}
		ids := system.IDs{}
		partner, err := ids.NewID()
		require.NoError(t, err)
		account, err := ids.NewID()
		require.NoError(t, err)
		user, err := ids.NewID()
		require.NoError(t, err)
		movement, err := ids.NewID()
		require.NoError(t, err)
		hash := sha256.Sum256([]byte(partner))
		tx, err := old.pool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `INSERT INTO partners(id,nombre,api_key_hash) VALUES($1,'Integración anterior',$2)`, partner, hash[:])
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO cuentas(id,alumno_id,saldo,ultima_secuencia) VALUES($1,$2,300,1)`, account, user)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES($1,'anterior',$2)`, partner, account)
		require.NoError(t, err)
		_, err = tx.Exec(ctx, `INSERT INTO movimientos_puntos(id,cuenta_id,partner_id,secuencia,puntos,motivo,referencia,ocurrido_en,aplicado_en) VALUES($1,$2,$3,1,300,'Crédito anterior','historia',now(),now())`, movement, account, partner)
		require.NoError(t, err)
		require.NoError(t, tx.Commit(ctx))
		require.NoError(t, old.Migrate(ctx))
		require.NoError(t, old.Migrate(ctx))
		app := application.New(old, system.Clock{}, ids)
		balance, err := app.ConsultarSaldo(ctx, partner, "anterior")
		require.NoError(t, err)
		require.EqualValues(t, 300, balance)
		page, err := app.ListarMovimientos(ctx, partner, "anterior", 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, page.Total)
		require.Equal(t, movement, page.Movimientos[0].Datos().ID)
		var required bool
		require.NoError(t, old.pool.QueryRow(ctx, `SELECT requiere_evento FROM partners WHERE id=$1`, partner).Scan(&required))
		require.False(t, required)
		_, err = app.Acreditar(ctx, application.AcreditarPuntos{PartnerID: partner, ExternalUserID: "anterior", Clave: "cliente-anterior-001", Puntos: 50, Motivo: "Cliente compatible", Referencia: "posterior"})
		require.NoError(t, err)
	})
	t.Run("evento_obligatorio_configurable_y_default_seguro", func(t *testing.T) {
		f := nuevaFixture(t, db)
		_, err := db.pool.Exec(ctx, `UPDATE partners SET requiere_evento=true WHERE id=$1`, f.partner)
		require.NoError(t, err)
		in := f.credito("obligatorio-001", 150)
		_, err = f.app.Acreditar(ctx, in)
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.Zero(t, saldo)
		in.EventoID = "hecho-obligatorio"
		_, err = f.app.Acreditar(ctx, in)
		require.NoError(t, err)
		id, err := (system.IDs{}).NewID()
		require.NoError(t, err)
		hash := sha256.Sum256([]byte(id))
		var required bool
		require.NoError(t, db.pool.QueryRow(ctx, `INSERT INTO partners(id,nombre,api_key_hash) VALUES($1,'Nueva integración',$2) RETURNING requiere_evento`, id, hash[:]).Scan(&required))
		require.True(t, required)
	})
	t.Run("mismo_evento_concurrente_distintas_keys", func(t *testing.T) {
		f := nuevaFixture(t, db)
		const total = 8
		start := make(chan struct{})
		results := make(chan application.RespuestaAcreditacion, total)
		errors := make(chan error, total)
		var group sync.WaitGroup
		for i := 0; i < total; i++ {
			group.Add(1)
			go func(i int) {
				defer group.Done()
				in := f.credito(fmt.Sprintf("mismo-evento-%03d", i), 150)
				in.EventoID = "evento-concurrente"
				<-start
				out, err := f.app.Acreditar(ctx, in)
				results <- out
				errors <- err
			}(i)
		}
		close(start)
		group.Wait()
		close(results)
		close(errors)
		for err := range errors {
			require.NoError(t, err)
		}
		var movement string
		replays := 0
		for out := range results {
			if movement == "" {
				movement = out.Resultado.Movimiento.Datos().ID
			}
			require.Equal(t, movement, out.Resultado.Movimiento.Datos().ID)
			require.EqualValues(t, 150, out.Resultado.Saldo)
			if out.Repetida {
				replays++
			}
		}
		require.Equal(t, total-1, replays)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.EqualValues(t, 150, saldo)
		page, err := f.app.ListarMovimientos(ctx, f.partner, f.usuario, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 1, page.Total)
	})
	t.Run("evento_repetido_conserva_saldo_original_y_no_reasigna_usuario", func(t *testing.T) {
		f := nuevaFixture(t, db)
		in := f.credito("saldo-original-001", 150)
		in.EventoID = "saldo-original"
		first, err := f.app.Acreditar(ctx, in)
		require.NoError(t, err)
		_, err = f.app.Acreditar(ctx, f.credito("saldo-adicional-001", 50))
		require.NoError(t, err)
		in.Clave = "saldo-original-002"
		replay, err := f.app.Acreditar(ctx, in)
		require.NoError(t, err)
		require.Equal(t, first.Resultado, replay.Resultado)
		saldo, err := f.app.ConsultarSaldo(ctx, f.partner, f.usuario)
		require.NoError(t, err)
		require.EqualValues(t, 200, saldo)
		other := nuevaFixture(t, db)
		_, err = db.pool.Exec(ctx, `INSERT INTO vinculaciones(partner_id,external_user_id,cuenta_id) VALUES($1,'otra-persona',$2)`, f.partner, other.cuenta)
		require.NoError(t, err)
		in.Clave = "saldo-original-003"
		in.ExternalUserID = "otra-persona"
		_, err = f.app.Acreditar(ctx, in)
		require.ErrorIs(t, err, domain.ErrConflictoIdempotencia)
	})
	t.Run("presupuesto_diario_usa_fecha_argentina", func(t *testing.T) {
		f := nuevaFixture(t, db)
		_, err := db.pool.Exec(ctx, `UPDATE partners SET max_puntos_dia=100 WHERE id=$1`, f.partner)
		require.NoError(t, err)
		before := application.New(db, relojPrueba{time.Date(2026, 11, 6, 2, 59, 0, 0, time.UTC)}, system.IDs{})
		after := application.New(db, relojPrueba{time.Date(2026, 11, 6, 3, 0, 0, 0, time.UTC)}, system.IDs{})
		_, err = before.Acreditar(ctx, f.credito("fecha-anterior-001", 100))
		require.NoError(t, err)
		_, err = after.Acreditar(ctx, f.credito("fecha-siguiente-001", 100))
		require.NoError(t, err)
		_, err = after.Acreditar(ctx, f.credito("fecha-siguiente-002", 1))
		require.ErrorIs(t, err, domain.ErrOperacionInvalida)
	})
}
