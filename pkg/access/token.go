// Package access implements the narrow RS256 access-token profile of ADR-006.
// Cryptographic operations use Go's crypto/rsa; no algorithm negotiation,
// remote key lookup, refresh tokens or revocation list is supported in v1.
package access

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"regexp"
	"strings"
	"time"
)

const (
	Lifetime     = 15 * time.Minute
	Issuer       = "gym-members"
	Audience     = "gym-api"
	Admin        = "ADMINISTRADOR"
	Student      = "ALUMNO"
	Teacher      = "PROFESOR"
	Nutritionist = "NUTRICIONISTA"
)

var ErrToken = errors.New("access token inválido")
var ErrKey = errors.New("clave RSA inválida: requiere PEM y al menos 2048 bits")
var subject = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Claims struct {
	Subject   string `json:"sub"`
	Role      string `json:"role"`
	Issuer    string `json:"iss"`
	Audience  string `json:"aud"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

func ValidRole(role string) bool {
	return role == Admin || role == Student || role == Teacher || role == Nutritionist
}

type Signer struct{ key *rsa.PrivateKey }
type Verifier struct{ key *rsa.PublicKey }

func keyBlock(raw string) *pem.Block {
	block, rest := pem.Decode([]byte(strings.ReplaceAll(raw, `\n`, "\n")))
	if len(bytes.TrimSpace(rest)) != 0 {
		return nil
	}
	return block
}

func NewSigner(raw string) (*Signer, error) {
	block := keyBlock(raw)
	if block == nil {
		return nil, ErrKey
	}
	var key *rsa.PrivateKey
	switch block.Type {
	case "RSA PRIVATE KEY":
		key, _ = x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err == nil {
			key, _ = parsed.(*rsa.PrivateKey)
		}
	}
	if key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, ErrKey
	}
	key.Precompute()
	return &Signer{key}, nil
}

func NewVerifier(raw string) (*Verifier, error) {
	block := keyBlock(raw)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, ErrKey
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, ErrKey
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok || key.N == nil || key.N.BitLen() < 2048 || key.E < 3 || key.E%2 == 0 {
		return nil, ErrKey
	}
	return &Verifier{key}, nil
}

func (s *Signer) Verifier() *Verifier { return &Verifier{&s.key.PublicKey} }

func (s *Signer) Issue(id, role string, now time.Time) (string, error) {
	if s == nil || s.key == nil || !subject.MatchString(id) || !ValidRole(role) {
		return "", ErrToken
	}
	claims := Claims{id, role, Issuer, Audience, now.Unix(), now.Add(Lifetime).Unix()}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	data := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(body)
	digest := sha256.Sum256([]byte(data))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return data + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func decodeJSON(segment string, out any) error {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(segment)
	if err != nil {
		return ErrToken
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(out) != nil {
		return ErrToken
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return ErrToken
	}
	return nil
}

func (v *Verifier) Verify(token string, now time.Time) (Claims, error) {
	var claims Claims
	if v == nil || v.key == nil || len(token) > 8192 || strings.ContainsAny(token, " \r\n\t") {
		return claims, ErrToken
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return claims, ErrToken
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if decodeJSON(parts[0], &header) != nil || header.Algorithm != "RS256" || header.Type != "JWT" {
		return claims, ErrToken
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[2])
	if err != nil || len(signature) != v.key.Size() {
		return claims, ErrToken
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if rsa.VerifyPKCS1v15(v.key, crypto.SHA256, digest[:], signature) != nil {
		return claims, ErrToken
	}
	if decodeJSON(parts[1], &claims) != nil {
		return Claims{}, ErrToken
	}
	if !subject.MatchString(claims.Subject) || !ValidRole(claims.Role) || claims.Issuer != Issuer || claims.Audience != Audience || claims.IssuedAt <= 0 || claims.IssuedAt > now.Unix() || claims.ExpiresAt <= now.Unix() || claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > int64(Lifetime/time.Second) {
		return Claims{}, ErrToken
	}
	return claims, nil
}
