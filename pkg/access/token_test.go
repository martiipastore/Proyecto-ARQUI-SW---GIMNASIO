// ADR-006; RN-26/27/30/34: autenticación y frontera de confianza.
package access

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) (*Signer, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	private := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	require.NoError(t, err)
	public := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}))
	signer, err := NewSigner(private)
	require.NoError(t, err)
	return signer, public
}

func signRaw(t *testing.T, signer *Signer, header, body string) string {
	t.Helper()
	data := base64.RawURLEncoding.EncodeToString([]byte(header)) + "." + base64.RawURLEncoding.EncodeToString([]byte(body))
	digest := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, signer.key, crypto.SHA256, digest[:])
	require.NoError(t, err)
	return data + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func TestJWT_RS256_ClaimsDuracionYFirma(t *testing.T) {
	signer, public := fixture(t)
	verifier, err := NewVerifier(public)
	require.NoError(t, err)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for _, role := range []string{Admin, Student, Teacher, Nutritionist} {
		t.Run(role, func(t *testing.T) {
			token, err := signer.Issue("11111111-1111-4111-8111-111111111111", role, now)
			require.NoError(t, err)
			claims, err := verifier.Verify(token, now)
			require.NoError(t, err)
			require.Equal(t, role, claims.Role)
			require.EqualValues(t, 900, claims.ExpiresAt-claims.IssuedAt)
			require.Equal(t, Issuer, claims.Issuer)
			require.Equal(t, Audience, claims.Audience)
			_, err = verifier.Verify(token, now.Add(Lifetime))
			require.ErrorIs(t, err, ErrToken)
			// Verify compact JWS directly, independently of the profile decoder.
			parts := strings.Split(token, ".")
			signature, err := base64.RawURLEncoding.DecodeString(parts[2])
			require.NoError(t, err)
			digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
			require.NoError(t, rsa.VerifyPKCS1v15(verifier.key, crypto.SHA256, digest[:], signature))
		})
	}
	other, _ := fixture(t)
	token, err := other.Issue("11111111-1111-4111-8111-111111111111", Student, now)
	require.NoError(t, err)
	_, err = verifier.Verify(token, now)
	require.ErrorIs(t, err, ErrToken)
}

func TestJWT_RechazaAlgoritmoClaimsYFormato(t *testing.T) {
	signer, _ := fixture(t)
	now := time.Unix(1793966400, 0)
	valid := Claims{"11111111-1111-4111-8111-111111111111", Student, Issuer, Audience, now.Unix(), now.Add(Lifetime).Unix()}
	for _, tc := range []struct {
		name   string
		modify func(*Claims)
	}{
		{"emisor", func(c *Claims) { c.Issuer = "otro" }},
		{"audiencia", func(c *Claims) { c.Audience = "otro" }},
		{"rol_desconocido", func(c *Claims) { c.Role = "admin" }},
		{"sub_ausente", func(c *Claims) { c.Subject = "" }},
		{"iat_ausente", func(c *Claims) { c.IssuedAt = 0 }},
		{"exp_ausente", func(c *Claims) { c.ExpiresAt = 0 }},
		{"emitido_futuro", func(c *Claims) { c.IssuedAt = now.Add(time.Minute).Unix() }},
		{"vida_excesiva", func(c *Claims) { c.ExpiresAt = now.Add(time.Hour).Unix() }},
		{"vencido", func(c *Claims) { c.IssuedAt = now.Add(-Lifetime).Unix(); c.ExpiresAt = now.Unix() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := valid
			tc.modify(&claims)
			body, err := json.Marshal(claims)
			require.NoError(t, err)
			_, err = signer.Verifier().Verify(signRaw(t, signer, `{"alg":"RS256","typ":"JWT"}`, string(body)), now)
			require.ErrorIs(t, err, ErrToken)
		})
	}
	body, err := json.Marshal(valid)
	require.NoError(t, err)
	for _, header := range []string{`{"alg":"none","typ":"JWT"}`, `{"alg":"HS256","typ":"JWT"}`, `{"alg":"RS512","typ":"JWT"}`, `{"alg":"RS256","typ":"JWT","jku":"https://attacker.test"}`} {
		_, err := signer.Verifier().Verify(signRaw(t, signer, header, string(body)), now)
		require.ErrorIs(t, err, ErrToken)
	}
	for _, raw := range []string{"", "opaque-legacy-token", "a.b.c", strings.Repeat("a", 8193)} {
		_, err := signer.Verifier().Verify(raw, now)
		require.ErrorIs(t, err, ErrToken)
	}
}

func TestClavesPEM_FallanCerradas(t *testing.T) {
	signer, public := fixture(t)
	_, err := NewSigner(public)
	require.ErrorIs(t, err, ErrKey)
	_, err = NewVerifier("missing")
	require.ErrorIs(t, err, ErrKey)
	_, err = NewVerifier(public + public)
	require.ErrorIs(t, err, ErrKey)
	privateDER, err := x509.MarshalPKCS8PrivateKey(signer.key)
	require.NoError(t, err)
	pkcs8 := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}))
	_, err = NewSigner(strings.ReplaceAll(pkcs8, "\n", `\n`))
	require.NoError(t, err)
	weak, err := rsa.GenerateKey(rand.Reader, 1024)
	require.NoError(t, err)
	_, err = NewSigner(string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(weak)})))
	require.ErrorIs(t, err, ErrKey)
}
