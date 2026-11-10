package model

import "errors"

var (
	ErrInvalid      = errors.New("entrada inválida")
	ErrConflict     = errors.New("identidad o evento en conflicto")
	ErrUnauthorized = errors.New("credencial inválida o vencida")
	ErrRateLimited  = errors.New("demasiados intentos")
	// ErrNotFound distingue un recurso inexistente de una credencial inválida:
	// Benefits traduce 404 a cuenta/registro inexistente y 401 a falla interna.
	ErrNotFound = errors.New("registro no encontrado")
)

type ExternalRegistration struct {
	NewUserID      string `json:"-"`
	ID             string `json:"registration_id"`
	PartnerID      string `json:"partner_id"`
	Platform       string `json:"platform"`
	EventID        string `json:"event_id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	ExternalUserID string `json:"external_user_id"`
	Points         int64  `json:"points"`
	Reason         string `json:"reason"`
	Reference      string `json:"reference"`
}

// Receipt no expone si el correo ya existía, ni identificadores internos.
type Receipt struct {
	ID             string `json:"registration_id"`
	EventID        string `json:"event_id"`
	Status         string `json:"status"`
	ExternalUserID string `json:"external_user_id"`
}

type User struct {
	Role     string `json:"role"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	External bool   `json:"external_registration"`
	// State permite rechazar el acceso de un usuario dado de baja (CA-01.4).
	State string `json:"status"`
	// Person agrega los datos del padrón para "Mi cuenta" (users.v1).
	Person *Person `json:"persona,omitempty"`
}

type Fulfillment struct {
	EventID        string `json:"event_id"`
	RegistrationID string `json:"registration_id"`
	PartnerID      string `json:"partner_id"`
	UserID         string `json:"user_id"`
	ExternalUserID string `json:"external_user_id"`
	Points         int64  `json:"points"`
	Reason         string `json:"reason"`
	Reference      string `json:"reference"`
}

type Email struct{ To, Subject, Body, MessageID string }
