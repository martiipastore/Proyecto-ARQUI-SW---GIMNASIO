package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/ports"
	"time"
)

var ErrCredencialInvalida = errors.New("credencial de partner inválida")
var ErrPartnerInactivo = errors.New("partner inactivo")

type Authenticator struct{ partners ports.Partners }

func NewAuthenticator(partners ports.Partners) *Authenticator {
	return &Authenticator{partners: partners}
}
func (a *Authenticator) Autenticar(ctx context.Context, key string) (string, error) {
	if key == "" {
		return "", ErrCredencialInvalida
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	p, err := a.partners.BuscarPartner(ctx, sha256.Sum256([]byte(key)))
	if err != nil {
		return "", err
	}
	if p.ID == "" {
		return "", ErrCredencialInvalida
	}
	if !p.Activo {
		return "", ErrPartnerInactivo
	}
	return p.ID, nil
}
