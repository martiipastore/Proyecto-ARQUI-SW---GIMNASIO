package service

import (
	"context"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"golang.org/x/crypto/bcrypt"
)

// PeopleRepository es lo que el padrón necesita de la persistencia. Las
// funciones de verificación las aporta el servicio y el repositorio las ejecuta
// bajo el bloqueo de la transacción: las reglas no viven en el repositorio.
type PeopleRepository interface {
	CreatePerson(ctx context.Context, actor, id string, in model.PersonInput, canComplete func(existing model.Person) bool, activation func(string) ([]byte, []byte, error), now time.Time) (model.Person, error)
	UpdatePerson(ctx context.Context, actor, id string, in model.PersonUpdate, now time.Time) (model.Person, error)
	DeactivatePerson(ctx context.Context, actor, id, reason string, check func(model.Person) error, now time.Time) (model.Person, error)
	Person(ctx context.Context, id string) (model.Person, error)
	People(ctx context.Context, filter model.PersonFilter) (model.PersonPage, error)
	Assign(ctx context.Context, actor, id, professional, student string, check func(professional, student model.Person) error, now time.Time) (model.Assignment, error)
	Revoke(ctx context.Context, actor, professional, student string, now time.Time) (model.Assignment, error)
	Assignments(ctx context.Context, person string) ([]model.Assignment, error)
	StudentsOf(ctx context.Context, professional string) ([]model.Assignment, error)
	ProfessionalsOf(ctx context.Context, student string) ([]model.Assignment, error)
	SummariesByID(ctx context.Context, ids []string, role string) ([]model.StudentSummary, error)
	StudentByDNI(ctx context.Context, dni string) (model.StudentSummary, error)
	ActiveAssignment(ctx context.Context, professional, student string) (bool, error)
	EnsureAdmin(ctx context.Context, id, email, name string, hash []byte) (bool, error)
}

// People gestiona alumnos, profesionales y asignaciones (HU-01, HU-02, RN-30,
// RN-33, RN-34 y RN-36; contrato docs/api/users.v1.md).
type People struct {
	repo       PeopleRepository
	activation func(string) ([]byte, []byte, error)
	now        func() time.Time
}

// NewPeople reutiliza la generación de enlaces de activación de Identity
// (D-78): quien recibe el alta elige su contraseña desde un enlace de un solo uso.
func NewPeople(repo PeopleRepository, identity *Identity, clock func() time.Time) *People {
	if clock == nil {
		clock = time.Now
	}
	return &People{repo: repo, activation: identity.activation, now: clock}
}

var dniPattern = regexp.MustCompile(`^[0-9]{7,8}$`)
var phonePattern = regexp.MustCompile(`^[0-9+() -]{6,30}$`)

func requireAdmin(actor, role string) error {
	if !uuidIdentity.MatchString(actor) || role != access.Admin {
		return model.ErrForbidden
	}
	return nil
}

// personText exige texto no vacío después de recortar espacios (D-78).
func personText(value string, min, max int) (string, bool) {
	value = strings.TrimSpace(value)
	return value, textValid(value, min, max)
}

// birthDate exige una fecha real, no anterior a 1900 y anterior a hoy en
// Argentina (SPEC §9: "fecha de nacimiento en el pasado"); sin edad mínima (D-78).
func (s *People) birthDate(value string) bool {
	d, err := date(value)
	return err == nil && value >= "1900-01-01" && d.Format("2006-01-02") < model.ArgentinaDate(s.now())
}

func (s *People) validProfile(role, phone, birth, license string) bool {
	if phone != "" && !phonePattern.MatchString(phone) {
		return false
	}
	if role == access.Student && !s.birthDate(birth) {
		return false
	}
	if role != access.Student && birth != "" && !s.birthDate(birth) {
		return false
	}
	if license != "" {
		if role != access.Nutritionist {
			return false
		}
		if _, ok := personText(license, 1, 40); !ok {
			return false
		}
	}
	return true
}

// Create da de alta un alumno o profesional (HU-01, HU-02, CA-01.1, CA-02.1/2).
func (s *People) Create(ctx context.Context, actor, actorRole string, in model.PersonInput) (model.Person, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.Person{}, err
	}
	var ok bool
	if in.Role != access.Student && in.Role != access.Teacher && in.Role != access.Nutritionist {
		return model.Person{}, model.ErrInvalid
	}
	if in.Name, ok = personText(in.Name, 1, 80); !ok {
		return model.Person{}, model.ErrInvalid
	}
	if in.LastName, ok = personText(in.LastName, 1, 80); !ok {
		return model.Person{}, model.ErrInvalid
	}
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	address, err := mail.ParseAddress(in.Email)
	if err != nil || address.Address != in.Email || len(in.Email) > 254 {
		return model.Person{}, model.ErrInvalid
	}
	in.DNI = strings.TrimSpace(in.DNI)
	in.Phone = strings.TrimSpace(in.Phone)
	in.License = strings.TrimSpace(in.License)
	if !dniPattern.MatchString(in.DNI) || !s.validProfile(in.Role, in.Phone, in.BirthDate, in.License) {
		return model.Person{}, model.ErrInvalid
	}
	id, err := newUUID()
	if err != nil {
		return model.Person{}, err
	}
	// D-78: una persona externa del Club (sin DNI) con el mismo email pasa a
	// ser alumna conservando cuenta, puntos y contraseña; cualquier otro email
	// o DNI repetido es duplicado (RN-33, CA-01.2).
	canComplete := func(existing model.Person) bool {
		return in.Role == access.Student && existing.Role == access.Student && existing.DNI == nil && existing.External
	}
	return s.repo.CreatePerson(ctx, actor, id, in, canComplete, s.activation, s.now().UTC())
}

// deactivable: solo se dan de baja personas del padrón activas (RN-34).
func deactivable(p model.Person) error {
	if p.DNI == nil || p.Role == access.Admin {
		return model.ErrInvalid
	}
	if p.State != "ACTIVO" {
		return model.ErrConflict
	}
	return nil
}

// assignable: profesional (profesor o nutricionista) y alumno del padrón,
// ambos ACTIVO (SPEC §9, RN-30).
func assignable(professional, student model.Person) error {
	if professional.DNI == nil || professional.State != "ACTIVO" || (professional.Role != access.Teacher && professional.Role != access.Nutritionist) {
		return model.ErrInvalid
	}
	if student.DNI == nil || student.State != "ACTIVO" || student.Role != access.Student {
		return model.ErrInvalid
	}
	return nil
}

// Update modifica datos personales; DNI, email y rol no cambian.
func (s *People) Update(ctx context.Context, actor, actorRole, id string, in model.PersonUpdate) (model.Person, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.Person{}, err
	}
	if !uuidIdentity.MatchString(id) {
		return model.Person{}, model.ErrInvalid
	}
	current, err := s.repo.Person(ctx, id)
	if err != nil {
		return model.Person{}, err
	}
	if current.DNI == nil || current.Role == access.Admin {
		// Solo se editan personas del padrón; el administrador no se gestiona acá.
		return model.Person{}, model.ErrInvalid
	}
	var ok bool
	if in.Name, ok = personText(in.Name, 1, 80); !ok {
		return model.Person{}, model.ErrInvalid
	}
	if in.LastName, ok = personText(in.LastName, 1, 80); !ok {
		return model.Person{}, model.ErrInvalid
	}
	in.Phone = strings.TrimSpace(in.Phone)
	in.License = strings.TrimSpace(in.License)
	if !s.validProfile(current.Role, in.Phone, in.BirthDate, in.License) {
		return model.Person{}, model.ErrInvalid
	}
	return s.repo.UpdatePerson(ctx, actor, id, in, s.now().UTC())
}

// Deactivate da de baja lógica a un alumno o profesional (RN-34, CA-01.3). El
// evento usuario.desactivado queda en el outbox para la etapa 5.
func (s *People) Deactivate(ctx context.Context, actor, actorRole, id, reason string) (model.Person, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.Person{}, err
	}
	var ok bool
	if reason, ok = personText(reason, 3, 200); !ok || !uuidIdentity.MatchString(id) || id == actor {
		return model.Person{}, model.ErrInvalid
	}
	return s.repo.DeactivatePerson(ctx, actor, id, reason, deactivable, s.now().UTC())
}

// Get devuelve una persona y sus asignaciones; solo el administrador o la propia
// persona pueden verla (RN-30).
func (s *People) Get(ctx context.Context, actor, actorRole, id string) (model.Person, []model.Assignment, error) {
	if !uuidIdentity.MatchString(id) {
		return model.Person{}, nil, model.ErrInvalid
	}
	if actorRole != access.Admin && actor != id {
		return model.Person{}, nil, model.ErrForbidden
	}
	person, err := s.repo.Person(ctx, id)
	if err != nil {
		return model.Person{}, nil, err
	}
	assignments, err := s.repo.Assignments(ctx, id)
	return person, assignments, err
}

// List busca personas del padrón para el administrador.
func (s *People) List(ctx context.Context, actor, actorRole string, filter model.PersonFilter) (model.PersonPage, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.PersonPage{}, err
	}
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Page < 1 || filter.Page > 100000 || filter.Size < 1 || filter.Size > 100 || len(filter.Query) > 80 {
		return model.PersonPage{}, model.ErrInvalid
	}
	if filter.Role != "" && !access.ValidRole(filter.Role) {
		return model.PersonPage{}, model.ErrInvalid
	}
	if filter.State != "" && filter.State != "ACTIVO" && filter.State != "INACTIVO" {
		return model.PersonPage{}, model.ErrInvalid
	}
	return s.repo.People(ctx, filter)
}

// Assign asigna un alumno a un profesional; ambos deben estar ACTIVO y no
// puede existir otra asignación vigente igual (SPEC §9, CA-02.4).
func (s *People) Assign(ctx context.Context, actor, actorRole, professional, student string) (model.Assignment, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.Assignment{}, err
	}
	if !uuidIdentity.MatchString(professional) || !uuidIdentity.MatchString(student) || professional == student {
		return model.Assignment{}, model.ErrInvalid
	}
	id, err := newUUID()
	if err != nil {
		return model.Assignment{}, err
	}
	return s.repo.Assign(ctx, actor, id, professional, student, assignable, s.now().UTC())
}

// Revoke cierra una asignación vigente; desde ese momento deja de autorizar
// (RN-30: sin período de gracia).
func (s *People) Revoke(ctx context.Context, actor, actorRole, professional, student string) (model.Assignment, error) {
	if err := requireAdmin(actor, actorRole); err != nil {
		return model.Assignment{}, err
	}
	if !uuidIdentity.MatchString(professional) || !uuidIdentity.MatchString(student) {
		return model.Assignment{}, model.ErrInvalid
	}
	return s.repo.Revoke(ctx, actor, professional, student, s.now().UTC())
}

// MyStudents lista los alumnos asignados al profesional autenticado (CA-02.4,
// RN-30). Los alumnos con reserva en sus clases se suman con booking (D-09).
func (s *People) MyStudents(ctx context.Context, actor, actorRole string) ([]model.Assignment, error) {
	if !uuidIdentity.MatchString(actor) || (actorRole != access.Teacher && actorRole != access.Nutritionist) {
		return nil, model.ErrForbidden
	}
	return s.repo.StudentsOf(ctx, actor)
}

// StudentsByID entrega datos mínimos a otros servicios (API interna, HU-21).
func (s *People) StudentsByID(ctx context.Context, ids []string) ([]model.StudentSummary, error) {
	return s.summaries(ctx, ids, access.Student)
}

// TeachersByID permite a booking validar que el responsable de un horario sea un
// PROFESOR activo (RN-08, CA-07.6); un nutricionista no aparece en la respuesta.
func (s *People) TeachersByID(ctx context.Context, ids []string) ([]model.StudentSummary, error) {
	return s.summaries(ctx, ids, access.Teacher)
}

func (s *People) summaries(ctx context.Context, ids []string, role string) ([]model.StudentSummary, error) {
	if len(ids) == 0 || len(ids) > 100 {
		return nil, model.ErrInvalid
	}
	for _, id := range ids {
		if !uuidIdentity.MatchString(id) {
			return nil, model.ErrInvalid
		}
	}
	return s.repo.SummariesByID(ctx, ids, role)
}

// ContactEmail entrega el email de un alumno activo a Benefits para acreditarle
// puntos en el proveedor externo (etapa 13, D-92). Solo por la API interna.
func (s *People) ContactEmail(ctx context.Context, id string) (string, error) {
	if !uuidIdentity.MatchString(id) {
		return "", model.ErrInvalid
	}
	person, err := s.repo.Person(ctx, id)
	if err != nil {
		return "", err
	}
	if person.Role != access.Student || person.State != "ACTIVO" {
		return "", model.ErrNotFound
	}
	return person.Email, nil
}

// StudentByDNI identifica al alumno en el ingreso por DNI (etapa 4, D-36).
func (s *People) StudentByDNI(ctx context.Context, dni string) (model.StudentSummary, error) {
	if !dniPattern.MatchString(dni) {
		return model.StudentSummary{}, model.ErrInvalid
	}
	return s.repo.StudentByDNI(ctx, dni)
}

// ActiveAssignment responde si la asignación está vigente al momento de la
// consulta, sin caché (RN-30, D-33).
func (s *People) ActiveAssignment(ctx context.Context, professional, student string) (bool, error) {
	if !uuidIdentity.MatchString(professional) || !uuidIdentity.MatchString(student) {
		return false, model.ErrInvalid
	}
	return s.repo.ActiveAssignment(ctx, professional, student)
}

// ActiveAssignments lista las asignaciones vigentes de un alumno (sus
// profesionales) o de un profesional (sus alumnos) para training (etapa 10,
// RN-30, RN-37). Se consulta en cada operación, sin caché (D-33).
func (s *People) ActiveAssignments(ctx context.Context, student, professional string) ([]model.Assignment, error) {
	switch {
	case student != "" && professional == "" && uuidIdentity.MatchString(student):
		return s.repo.ProfessionalsOf(ctx, student)
	case professional != "" && student == "" && uuidIdentity.MatchString(professional):
		return s.repo.StudentsOf(ctx, professional)
	default:
		return nil, model.ErrInvalid
	}
}

// SeedAdmin crea el administrador inicial si todavía no existe ese email; nunca
// sobrescribe una cuenta existente ni su contraseña.
func (s *People) SeedAdmin(ctx context.Context, email, name, password string) (bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email {
		return false, model.ErrInvalid
	}
	var ok bool
	if name, ok = personText(name, 1, 80); !ok {
		return false, model.ErrInvalid
	}
	if len([]rune(password)) < 12 || len(password) > 72 {
		return false, model.ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return false, err
	}
	id, err := newUUID()
	if err != nil {
		return false, err
	}
	return s.repo.EnsureAdmin(ctx, id, email, name, hash)
}
