package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

type IdentityRepository interface {
	RenewActivation(context.Context, string, func(string) ([]byte, []byte, error), time.Time) error
	TakeAuthQuota(context.Context, string, int, time.Time) error
	RegisterExternal(context.Context, model.ExternalRegistration, []byte, []byte, []byte, time.Time) (model.Receipt, error)
	Registration(context.Context, string, string) (model.Receipt, error)
	Activate(context.Context, []byte, []byte, time.Time) error
	Password(context.Context, string) (model.User, []byte, error)
	User(context.Context, string) (model.User, error)
	UpgradePassword(context.Context, string, []byte, []byte) error
	ProcessRegistration(context.Context, func(context.Context, model.Fulfillment) (int64, error), func(string) ([]byte, []byte, error), time.Time) (bool, error)
	ProcessEmail(context.Context, func(context.Context, string, []byte, int64, bool, string, string) error) (bool, error)
}

type BenefitsClient interface {
	Fulfill(context.Context, model.Fulfillment) (int64, error)
}
type Mailer interface {
	Send(context.Context, model.Email) error
}

type Identity struct {
	repo      IdentityRepository
	benefits  BenefitsClient
	mail      Mailer
	cipher    cipher.AEAD
	publicURL string
	now       func() time.Time
	signer    *access.Signer
}

func NewIdentity(repo IdentityRepository, benefits BenefitsClient, mailer Mailer, secret, publicURL string, signers ...*access.Signer) (*Identity, error) {
	u, err := url.Parse(publicURL)
	if len(secret) < 32 || err != nil || u.Host == "" || u.User != nil || (u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"))) || u.Fragment != "" || u.RawQuery != "" {
		return nil, fmt.Errorf("configuración de identidad inválida")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	identity := &Identity{repo: repo, benefits: benefits, mail: mailer, cipher: aead, publicURL: strings.TrimRight(publicURL, "/"), now: time.Now}
	if len(signers) > 0 {
		identity.signer = signers[0]
	}
	return identity, nil
}

var externalIdentity = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,64}$`)
var operationKey = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)
var registrationEvent = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:@-]{0,127}$`)
var uuidIdentity = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func opaqueToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Identity) activation(id string) ([]byte, []byte, error) {
	token, err := opaqueToken()
	if err != nil {
		return nil, nil, err
	}
	hash := sha256.Sum256([]byte(token))
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	encrypted := s.cipher.Seal(nonce, nonce, []byte(token), []byte(id))
	return hash[:], encrypted, nil
}

func (s *Identity) Resend(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return model.ErrInvalid
	}
	digest := sha256.Sum256([]byte(email))
	if err = s.repo.TakeAuthQuota(ctx, fmt.Sprintf("resend:%x", digest), 1, s.now().UTC()); err != nil {
		return err
	}
	return s.repo.RenewActivation(ctx, email, s.activation, s.now().UTC())
}
func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func textValid(s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return utf8.ValidString(s) && n >= min && n <= max && !strings.ContainsAny(s, "\x00\r\n")
}

func (s *Identity) RegisterExternal(ctx context.Context, in model.ExternalRegistration) (model.Receipt, error) {
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.Name = strings.TrimSpace(in.Name)
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 || !textValid(in.Name, 1, 120) || !uuidIdentity.MatchString(in.PartnerID) || !operationKey.MatchString(in.Key) || !registrationEvent.MatchString(in.EventID) || !textValid(in.Platform, 1, 120) || in.Points < 0 || in.Points > 1000000 || !textValid(in.Reason, 3, 200) || !textValid(in.Reference, 1, 100) {
		return model.Receipt{}, model.ErrInvalid
	}
	if in.ExternalUserID == "" {
		h := sha256.Sum256([]byte(in.PartnerID + "\x00" + in.Email))
		in.ExternalUserID = fmt.Sprintf("email-%x", h[:24])
	}
	if !externalIdentity.MatchString(in.ExternalUserID) {
		return model.Receipt{}, model.ErrInvalid
	}
	in.ID, err = newUUID()
	if err != nil {
		return model.Receipt{}, err
	}
	in.NewUserID, err = newUUID()
	if err != nil {
		return model.Receipt{}, err
	}
	hash, encrypted, err := s.activation(in.ID)
	if err != nil {
		return model.Receipt{}, err
	}
	// La huella no incluye la key HTTP ni los identificadores aleatorios.
	data := fmt.Sprintf("%q|%q|%q|%q|%d|%q|%q", in.Email, in.Name, in.ExternalUserID, in.EventID, in.Points, in.Reason, in.Reference)
	fingerprint := sha256.Sum256([]byte(data))
	return s.repo.RegisterExternal(ctx, in, fingerprint[:], hash, encrypted, s.now().UTC().Add(24*time.Hour))
}

func (s *Identity) Registration(ctx context.Context, partner, event string) (model.Receipt, error) {
	if !uuidIdentity.MatchString(partner) || !registrationEvent.MatchString(event) {
		return model.Receipt{}, model.ErrInvalid
	}
	return s.repo.Registration(ctx, partner, event)
}

func (s *Identity) Activate(ctx context.Context, token, password string) error {
	if err := s.repo.TakeAuthQuota(ctx, "activation", 120, s.now().UTC()); err != nil {
		return err
	}
	if len(token) != 43 {
		return model.ErrUnauthorized
	}
	var hash []byte
	if password != "" {
		if utf8.RuneCountInString(password) < 12 || len(password) > 72 {
			return model.ErrInvalid
		}
		var err error
		hash, err = bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return err
		}
	}
	tokenHash := sha256.Sum256([]byte(token))
	return s.repo.Activate(ctx, tokenHash[:], hash, s.now().UTC())
}

func (s *Identity) Login(ctx context.Context, email, password string) (string, error) {
	if err := s.repo.TakeAuthQuota(ctx, "login-global", 120, s.now().UTC()); err != nil {
		return "", err
	}
	email = strings.ToLower(strings.TrimSpace(email))
	digestEmail := sha256.Sum256([]byte(email))
	if err := s.repo.TakeAuthQuota(ctx, fmt.Sprintf("login:%x", digestEmail), 10, s.now().UTC()); err != nil {
		return "", err
	}
	if len(email) > 254 || len(password) > 72 {
		return "", model.ErrUnauthorized
	}
	user, hash, err := s.repo.Password(ctx, email)
	// Mismo coste para un correo inexistente; no revela altas externas.
	if err != nil || len(hash) == 0 {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(password))
		return "", model.ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil {
		return "", model.ErrUnauthorized
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		return "", model.ErrUnauthorized
	}
	if cost < 12 {
		upgraded, err := bcrypt.GenerateFromPassword([]byte(password), 12)
		if err != nil {
			return "", err
		}
		if err := s.repo.UpgradePassword(ctx, user.ID, hash, upgraded); err != nil {
			return "", err
		}
	}
	if s.signer == nil {
		return "", model.ErrUnauthorized
	}
	return s.signer.Issue(user.ID, user.Role, s.now().UTC())
}
func (s *Identity) CurrentUser(ctx context.Context, actorID string) (model.User, error) {
	// Actor validado por el gateway; la capa HTTP exige credencial interna.
	// Members es emisor, no vuelve a decodificar el JWT del cliente (ADR-006).
	if !uuidIdentity.MatchString(actorID) {
		return model.User{}, model.ErrUnauthorized
	}
	return s.repo.User(ctx, actorID)
}

func (s *Identity) Logout(ctx context.Context, actorID string) error {
	// ADR-006 v1: logout descarta el token en el cliente; no hay revocación.
	if !uuidIdentity.MatchString(actorID) {
		return model.ErrUnauthorized
	}
	return nil
}

// Generado una vez al iniciar, con el mismo costo que las cuentas nuevas.
var dummyPasswordHash = func() []byte {
	hash, err := bcrypt.GenerateFromPassword([]byte("not-a-user-password"), 12)
	if err != nil {
		panic("no se pudo inicializar bcrypt")
	}
	return hash
}()

func (s *Identity) Process(ctx context.Context) error {
	if _, err := s.repo.ProcessRegistration(ctx, s.benefits.Fulfill, s.activation, s.now().UTC()); err != nil {
		return err
	}
	_, err := s.repo.ProcessEmail(ctx, func(ctx context.Context, email string, encrypted []byte, balance int64, needsPassword bool, id, kind string) error {
		if kind == "welcome" && !needsPassword {
			return s.mail.Send(ctx, model.Email{To: email, Subject: "Beneficio acreditado en el gimnasio", Body: fmt.Sprintf("Se procesó tu beneficio. Tu saldo al acreditar era de %d puntos. Ingresá con tu cuenta habitual en %s.\n", balance, s.publicURL), MessageID: id + "-" + kind})
		}
		if len(encrypted) == 0 && kind == "verification" {
			return nil
		}
		n := s.cipher.NonceSize()
		if len(encrypted) < n {
			return fmt.Errorf("token cifrado inválido")
		}
		token, err := s.cipher.Open(nil, encrypted[:n], encrypted[n:], []byte(id))
		if err != nil {
			return err
		}
		subject := "Confirmá la vinculación con el gimnasio"
		body := "Una plataforma solicitó vincular tu cuenta. Confirmá solamente si reconocés esta solicitud.\n"
		if kind == "welcome" {
			subject = "Bienvenido al Club de Beneficios"
			body = fmt.Sprintf("Ya tenés una cuenta en nuestro gimnasio. Se procesó tu beneficio. Tu saldo al acreditar era de %d puntos.\n", balance)
		}
		if needsPassword {
			body += "Establecé tu contraseña (12 a 72 caracteres) y activá tu cuenta desde este enlace de un solo uso:\n"
		} else {
			body += "Confirmá la vinculación desde este enlace de un solo uso (tu contraseña actual se conserva):\n"
		}
		body += s.publicURL + "/#activate=" + url.QueryEscape(string(token)) + "\nEl enlace vence a las 24 horas. Si no solicitaste esta operación, ignorá este correo.\n"
		return s.mail.Send(ctx, model.Email{To: email, Subject: subject, Body: body, MessageID: id + "-" + kind})
	})
	return err
}
