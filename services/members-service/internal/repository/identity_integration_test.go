package repository

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"golang.org/x/crypto/bcrypt"
)

type fulfillStub struct {
	calls int
	fail  bool
}

func (f *fulfillStub) Fulfill(context.Context, model.Fulfillment) (int64, error) {
	f.calls++
	if f.fail {
		return 0, errors.New("conexión interrumpida")
	}
	return 150, nil
}

type mailStub struct {
	emails []model.Email
	fail   bool
}

func (m *mailStub) Send(_ context.Context, e model.Email) error {
	if m.fail {
		return errors.New("SMTP caído")
	}
	m.emails = append(m.emails, e)
	return nil
}
func mailToken(t *testing.T, m model.Email) string {
	t.Helper()
	for _, line := range strings.Split(m.Body, "\n") {
		if strings.HasPrefix(line, "https://") {
			u, err := url.Parse(line)
			require.NoError(t, err)
			values, err := url.ParseQuery(u.Fragment)
			require.NoError(t, err)
			return values.Get("activate")
		}
	}
	t.Fatal("falta enlace de activación")
	return ""
}

// Club §1/2/3/12: PostgreSQL real; no sustituye restricciones por mocks de base.
func TestIdentidadExternaPostgres(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:16-alpine", postgres.WithDatabase("members_db"), postgres.WithUsername("members"), postgres.WithPassword("fixture"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := NewPostgres(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Migrate(ctx))
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := access.NewSigner(string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})))
	require.NoError(t, err)
	partner := "11111111-1111-4111-8111-111111111111"
	registration := func(email, event, external string) model.ExternalRegistration {
		return model.ExternalRegistration{PartnerID: partner, Platform: "Estudiario", EventID: event, Key: "request-" + event, Name: "Persona", Email: email, ExternalUserID: external, Points: 150, Reason: "Participación", Reference: event}
	}
	t.Run("nuevo_activacion_unica_reintento_y_login", func(t *testing.T) {
		benefits, mail := &fulfillStub{}, &mailStub{}
		s, err := service.NewIdentity(db, benefits, mail, strings.Repeat("x", 32), "https://gym.example", signer)
		require.NoError(t, err)
		in := registration("nuevo@example.test", "nuevo-0001", "externo-nuevo")
		first, err := s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		again, err := s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		require.Equal(t, first, again)
		altered := in
		altered.Points++
		_, err = s.RegisterExternal(ctx, altered)
		require.ErrorIs(t, err, model.ErrConflict)
		alias := in
		alias.Key = "otra-clave-del-evento"
		aliasReceipt, err := s.RegisterExternal(ctx, alias)
		require.NoError(t, err)
		require.Equal(t, first, aliasReceipt)
		alias.EventID = "evento-distinto"
		_, err = s.RegisterExternal(ctx, alias)
		require.ErrorIs(t, err, model.ErrConflict)
		require.NoError(t, s.Process(ctx))
		require.Equal(t, 1, benefits.calls)
		require.Len(t, mail.emails, 1)
		require.Contains(t, mail.emails[0].Body, "150 puntos")
		token := mailToken(t, mail.emails[0])
		require.NoError(t, s.Activate(ctx, token, "contraseña-segura-123"))
		require.ErrorIs(t, s.Activate(ctx, token, "contraseña-segura-456"), model.ErrUnauthorized)
		session, err := s.Login(ctx, in.Email, "contraseña-segura-123")
		require.NoError(t, err)
		claims, err := signer.Verifier().Verify(session, time.Now())
		require.NoError(t, err)
		user, err := s.CurrentUser(ctx, claims.Subject)
		require.NoError(t, err)
		require.True(t, user.External)
		require.Equal(t, access.Student, user.Role)
		require.NoError(t, s.Logout(ctx, claims.Subject))
		_, err = signer.Verifier().Verify(session, time.Now())
		require.NoError(t, err)
		_, err = signer.Verifier().Verify(session, time.Unix(claims.ExpiresAt, 0))
		require.ErrorIs(t, err, access.ErrToken)
		var count int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM usuarios WHERE email=$1`, in.Email).Scan(&count))
		require.Equal(t, 1, count)
	})
	t.Run("existente_verifica_correo_sin_resetear_contraseña", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("contraseña-anterior-123"), bcrypt.DefaultCost)
		require.NoError(t, err)
		_, err = db.pool.Exec(ctx, `INSERT INTO usuarios(id,nombre,email,password_hash) VALUES('22222222-2222-4222-8222-222222222222','Existente','existente@example.test',$1)`, hash)
		require.NoError(t, err)
		benefits, mail := &fulfillStub{}, &mailStub{}
		s, err := service.NewIdentity(db, benefits, mail, strings.Repeat("x", 32), "https://gym.example", signer)
		require.NoError(t, err)
		in := registration("existente@example.test", "existente-0001", "externo-existente")
		_, err = s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		require.NoError(t, s.Process(ctx))
		require.Zero(t, benefits.calls)
		require.Len(t, mail.emails, 1)
		require.NoError(t, s.Activate(ctx, mailToken(t, mail.emails[0]), "contraseña-intento-reset"))
		require.NoError(t, s.Process(ctx))
		require.Equal(t, 1, benefits.calls)
		require.Len(t, mail.emails, 2)
		require.NotContains(t, mail.emails[1].Body, "#activate=")
		_, err = s.Login(ctx, in.Email, "contraseña-anterior-123")
		require.NoError(t, err)
		_, upgraded, err := db.Password(ctx, in.Email)
		require.NoError(t, err)
		cost, err := bcrypt.Cost(upgraded)
		require.NoError(t, err)
		require.Equal(t, 12, cost)
		_, err = s.Login(ctx, in.Email, "contraseña-intento-reset")
		require.ErrorIs(t, err, model.ErrUnauthorized)
		var external bool
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT registro_externo FROM usuarios WHERE email=$1`, in.Email).Scan(&external))
		require.False(t, external)
	})
	t.Run("fallo_remoto_no_pierde_registro_y_reintenta", func(t *testing.T) {
		benefits, mail := &fulfillStub{fail: true}, &mailStub{}
		s, err := service.NewIdentity(db, benefits, mail, strings.Repeat("x", 32), "https://gym.example", signer)
		require.NoError(t, err)
		in := registration("reintento@example.test", "reintento-0001", "externo-reintento")
		receipt, err := s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		require.Error(t, s.Process(ctx))
		require.Empty(t, mail.emails)
		benefits.fail = false
		_, err = db.pool.Exec(ctx, `UPDATE registros_externos SET proximo_intento=now() WHERE id=$1`, receipt.ID)
		require.NoError(t, err)
		require.NoError(t, s.Process(ctx))
		require.Len(t, mail.emails, 1)
		result, err := s.Registration(ctx, partner, in.EventID)
		require.NoError(t, err)
		require.Equal(t, "completed", result.Status)
	})
	t.Run("SMTP_caido_reintenta_sin_repetir_acreditacion", func(t *testing.T) {
		benefits, mail := &fulfillStub{}, &mailStub{fail: true}
		s, err := service.NewIdentity(db, benefits, mail, strings.Repeat("x", 32), "https://gym.example", signer)
		require.NoError(t, err)
		in := registration("smtp@example.test", "smtp-0001", "externo-smtp")
		receipt, err := s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		require.Error(t, s.Process(ctx))
		require.Equal(t, 1, benefits.calls)
		state, err := s.Registration(ctx, partner, in.EventID)
		require.NoError(t, err)
		require.Equal(t, "completed", state.Status)
		mail.fail = false
		_, err = db.pool.Exec(ctx, `UPDATE correos_pendientes SET proximo_intento=now() WHERE registro_id=$1`, receipt.ID)
		require.NoError(t, err)
		require.NoError(t, s.Process(ctx))
		require.Equal(t, 1, benefits.calls)
		require.Len(t, mail.emails, 1)
	})
	t.Run("enlace_vencido_reenvio_y_rotacion", func(t *testing.T) {
		benefits, mail := &fulfillStub{}, &mailStub{}
		s, err := service.NewIdentity(db, benefits, mail, strings.Repeat("x", 32), "https://gym.example", signer)
		require.NoError(t, err)
		in := registration("expira@example.test", "expira-0001", "externo-expira")
		receipt, err := s.RegisterExternal(ctx, in)
		require.NoError(t, err)
		require.NoError(t, s.Process(ctx))
		old := mailToken(t, mail.emails[0])
		_, err = db.pool.Exec(ctx, `UPDATE registros_externos SET token_expira=now()-interval '1 second' WHERE id=$1`, receipt.ID)
		require.NoError(t, err)
		require.ErrorIs(t, s.Activate(ctx, old, "contraseña-segura-123"), model.ErrUnauthorized)
		require.NoError(t, s.Resend(ctx, in.Email))
		require.NoError(t, s.Process(ctx))
		require.Len(t, mail.emails, 2)
		renewed := mailToken(t, mail.emails[1])
		require.NotEqual(t, old, renewed)
		require.ErrorIs(t, s.Activate(ctx, old, "contraseña-segura-123"), model.ErrUnauthorized)
		require.NoError(t, s.Activate(ctx, renewed, "contraseña-segura-123"))
		require.ErrorIs(t, s.Activate(ctx, renewed, "contraseña-otra-123"), model.ErrUnauthorized)
	})
}
