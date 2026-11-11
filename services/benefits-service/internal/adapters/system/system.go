// Package system implementa reloj e identidad técnica sin dependencias nuevas.
package system

import (
	"crypto/rand"
	"fmt"
	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/domain"
	"time"
)

type Clock struct{}

func (Clock) Now() time.Time { return time.Now().UTC() }

// Secrets emite API keys de 256 bits en hexadecimal, como cmd/partneradmin.
type Secrets struct{}

func (Secrets) NewSecret() (string, error) {
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", bytes[:]), nil
}

type IDs struct{}

func (IDs) NewID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

// Codigos emite el código del cupón de un canje (D-94) con crypto/rand.
type Codigos struct{}

func (Codigos) NewCodigo() (string, error) {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", err
	}
	return domain.CodigoDesdeBytes(bytes), nil
}
