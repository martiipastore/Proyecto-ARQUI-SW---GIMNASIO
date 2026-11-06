package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"testing"
	"time"
)

type identityRepoStub struct {
	IdentityRepository
	input                                  model.ExternalRegistration
	fingerprint, hash, encrypted, password []byte
	expired                                time.Time
	quotaErr                               error
}

func (r *identityRepoStub) TakeAuthQuota(context.Context, string, int, time.Time) error {
	return r.quotaErr
}
func (r *identityRepoStub) RegisterExternal(_ context.Context, in model.ExternalRegistration, fingerprint, hash, encrypted []byte, expiry time.Time) (model.Receipt, error) {
	r.input = in
	r.fingerprint = fingerprint
	r.hash = hash
	r.encrypted = encrypted
	r.expired = expiry
	return model.Receipt{ID: in.ID, Status: "accepted"}, nil
}
func (r *identityRepoStub) Activate(_ context.Context, hash, password []byte, _ time.Time) error {
	r.hash = hash
	r.password = password
	return nil
}

func registrationInput() model.ExternalRegistration {
	return model.ExternalRegistration{PartnerID: "11111111-1111-4111-8111-111111111111", Platform: "Estudiario", EventID: "evento-0001", Key: "clave-0001", Name: "Usuario", Email: " Persona@Example.test ", Points: 150, Reason: "Participación confirmada", Reference: "evento-0001"}
}

// Club §2/3/12: correo canónico, origen, token de un uso y huella estable.
func TestRegistroValidaNormalizaYCifraToken(t *testing.T) {
	repo := &identityRepoStub{}
	s, err := NewIdentity(repo, nil, nil, strings.Repeat("x", 32), "https://gym.example")
	require.NoError(t, err)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	_, err = s.RegisterExternal(context.Background(), registrationInput())
	require.NoError(t, err)
	require.Equal(t, "persona@example.test", repo.input.Email)
	require.True(t, strings.HasPrefix(repo.input.ExternalUserID, "email-"))
	n := s.cipher.NonceSize()
	token, err := s.cipher.Open(nil, repo.encrypted[:n], repo.encrypted[n:], []byte(repo.input.ID))
	require.NoError(t, err)
	require.Len(t, token, 43)
	require.False(t, bytes.Contains(repo.encrypted, token))
	hash := sha256.Sum256(token)
	require.Equal(t, hash[:], repo.hash)
	require.Equal(t, now.Add(24*time.Hour), repo.expired)
	originalFingerprint := append([]byte(nil), repo.fingerprint...)
	_, err = s.RegisterExternal(context.Background(), registrationInput())
	require.NoError(t, err)
	require.Equal(t, originalFingerprint, repo.fingerprint)
}

func TestRegistroRechazaIdentidadYEntradasMalformadas(t *testing.T) {
	s, err := NewIdentity(&identityRepoStub{}, nil, nil, strings.Repeat("x", 32), "https://gym.example")
	require.NoError(t, err)
	for _, mutate := range []func(*model.ExternalRegistration){
		func(in *model.ExternalRegistration) { in.Email = "Nombre <a@example.test>" },
		func(in *model.ExternalRegistration) { in.ExternalUserID = "../otro" },
		func(in *model.ExternalRegistration) { in.PartnerID = "otro" },
		func(in *model.ExternalRegistration) { in.Points = -1 },
		func(in *model.ExternalRegistration) { in.Name = "Usuario\r\nBCC:otro" },
	} {
		in := registrationInput()
		mutate(&in)
		_, err = s.RegisterExternal(context.Background(), in)
		require.ErrorIs(t, err, model.ErrInvalid)
	}
}

func TestActivacionNoGuardaPasswordEnClaroYAplicaCuota(t *testing.T) {
	repo := &identityRepoStub{}
	s, err := NewIdentity(repo, nil, nil, strings.Repeat("x", 32), "https://gym.example")
	require.NoError(t, err)
	require.ErrorIs(t, s.Activate(context.Background(), strings.Repeat("a", 43), "corta"), model.ErrInvalid)
	require.NoError(t, s.Activate(context.Background(), strings.Repeat("a", 43), "contraseña-segura-123"))
	require.NotEqual(t, []byte("contraseña-segura-123"), repo.password)
	cost, err := bcrypt.Cost(repo.password)
	require.NoError(t, err)
	require.Equal(t, 12, cost)
	repo.quotaErr = model.ErrRateLimited
	require.ErrorIs(t, s.Activate(context.Background(), strings.Repeat("a", 43), "contraseña-segura-123"), model.ErrRateLimited)
}
