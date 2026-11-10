package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
)

type MembershipTx interface {
	// LockUser bloquea al usuario destino y devuelve su rol.
	LockUser(context.Context, string) (string, error)
	Type(context.Context, string) (model.MembershipType, error)
	SaveType(context.Context, model.MembershipType, string, string, time.Time) error
	Last(context.Context, string, bool, string) (*model.Membership, error)
	Get(context.Context, string) (model.Membership, error)
	Insert(context.Context, model.Membership, string, time.Time) error
	Cancel(context.Context, string, string, string, time.Time) error
}
type MembershipRepository interface {
	RunMembership(context.Context, string, string, string, func(MembershipTx) (model.MembershipResult, error)) (model.MembershipResult, bool, error)
	MembershipTypes(context.Context) ([]model.MembershipType, error)
	MembershipPage(context.Context, string, string, int, int) (model.MembershipPage, error)
	ExpireMemberships(context.Context, string, time.Time) (int64, error)
	// ActiveMemberships devuelve si el alumno está ACTIVO y sus membresías ACTIVA.
	ActiveMemberships(context.Context, string) (bool, []model.Membership, error)
}
type Memberships struct {
	repo MembershipRepository
	now  func() time.Time
}

func NewMemberships(repo MembershipRepository, clock func() time.Time) *Memberships {
	if clock == nil {
		clock = time.Now
	}
	return &Memberships{repo, clock}
}
func validType(code string) bool {
	return code == "MENSUAL" || code == "TRIMESTRAL" || code == "ANUAL" || code == "CLASES"
}
func date(value string) (time.Time, error) {
	d, err := time.Parse("2006-01-02", value)
	if err != nil || len(value) != 10 || d.Year() < 1 {
		return time.Time{}, model.ErrInvalid
	}
	return d, nil
}
func admin(actor, role, key, reason string) error {
	if !uuidIdentity.MatchString(actor) || role != access.Admin {
		return model.ErrForbidden
	}
	if !operationKey.MatchString(key) || !textValid(reason, 3, 200) {
		return model.ErrInvalid
	}
	return nil
}
func fingerprint(operation string, input any) string {
	data, _ := json.Marshal([]any{operation, input})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func (s *Memberships) Types(ctx context.Context) ([]model.MembershipType, error) {
	return s.repo.MembershipTypes(ctx)
}
func (s *Memberships) List(ctx context.Context, actor, role, user string, page, size int) (model.MembershipPage, error) {
	if !uuidIdentity.MatchString(user) || page < 1 || page > 100000 || size < 1 || size > 100 {
		return model.MembershipPage{}, model.ErrInvalid
	}
	if actor != user && role != access.Admin {
		return model.MembershipPage{}, model.ErrForbidden
	}
	today := model.ArgentinaDate(s.now())
	out, err := s.repo.MembershipPage(ctx, user, today, page, size)
	if err != nil {
		return out, err
	}
	for i := range out.Items {
		out.Items[i] = out.Items[i].At(today)
	}
	if out.Current != nil {
		m := out.Current.At(today)
		out.Current = &m
		out.Active = m.Active
	}
	out.ReferenceDate = today
	return out, nil
}
func (s *Memberships) Configure(ctx context.Context, actor, role, key, reason string, t model.MembershipType) (model.MembershipResult, bool, error) {
	reason = strings.TrimSpace(reason)
	t.Name = strings.TrimSpace(t.Name)
	if err := admin(actor, role, key, reason); err != nil {
		return model.MembershipResult{}, false, err
	}
	positive := func(n *int) bool { return n == nil || (*n > 0 && int64(*n) <= 2147483647) }
	// Mensual, trimestral y anual definen su período (D-77); por clases puede
	// quedar sin duración hasta que el gimnasio la configure (ADR-020).
	if !validType(t.Code) || !textValid(t.Name, 1, 120) || !positive(t.Months) || !positive(t.Days) || !positive(t.Classes) || (t.Months != nil && t.Days != nil) || (t.Code != "CLASES" && t.Classes != nil) || (t.Code != "CLASES" && !t.HasDuration()) {
		return model.MembershipResult{}, false, model.ErrInvalid
	}
	return s.repo.RunMembership(ctx, actor, key, fingerprint("CONFIGURAR_TIPO", []any{t, reason}), func(tx MembershipTx) (model.MembershipResult, error) {
		if err := tx.SaveType(ctx, t, actor, reason, s.now().UTC()); err != nil {
			return model.MembershipResult{}, err
		}
		return model.MembershipResult{Type: &t}, nil
	})
}
func (s *Memberships) Assign(ctx context.Context, actor, role, key string, in model.MembershipInput, renew bool) (model.MembershipResult, bool, error) {
	in.Reason = strings.TrimSpace(in.Reason)
	if err := admin(actor, role, key, in.Reason); err != nil {
		return model.MembershipResult{}, false, err
	}
	if !uuidIdentity.MatchString(in.UserID) || !validType(in.TypeCode) {
		return model.MembershipResult{}, false, model.ErrInvalid
	}
	// El vencimiento es opcional: si se omite se calcula según el tipo; si se
	// envía debe coincidir con ese cálculo (CA-03.1, D-77).
	if in.End != "" {
		if _, err := date(in.End); err != nil {
			return model.MembershipResult{}, false, err
		}
	}
	if !renew {
		if _, err := date(in.Start); err != nil {
			return model.MembershipResult{}, false, err
		}
	} else if in.Start != "" {
		return model.MembershipResult{}, false, model.ErrInvalid
	}
	return s.repo.RunMembership(ctx, actor, key, fingerprint(fmt.Sprintf("ASIGNAR_RENOVAR_%t", renew), in), func(tx MembershipTx) (model.MembershipResult, error) {
		role, err := tx.LockUser(ctx, in.UserID)
		if err != nil {
			return model.MembershipResult{}, err
		}
		// La membresía habilita a un alumno; no se asigna a otros roles (HU-03).
		if role != access.Student {
			return model.MembershipResult{}, model.ErrInvalid
		}
		today := model.ArgentinaDate(s.now())
		start := in.Start
		var previous *string
		if renew {
			last, err := tx.Last(ctx, in.UserID, false, today)
			if err != nil {
				return model.MembershipResult{}, err
			}
			if last == nil {
				return model.MembershipResult{}, model.ErrMembershipTransition
			}
			previous = &last.ID
			start = today
			active, err := tx.Last(ctx, in.UserID, true, today)
			if err != nil {
				return model.MembershipResult{}, err
			}
			if active != nil {
				end, err := date(active.End)
				if err != nil {
					return model.MembershipResult{}, err
				}
				start = end.AddDate(0, 0, 1).Format("2006-01-02")
				previous = &active.ID
			}
		}
		if start < today {
			return model.MembershipResult{}, model.ErrInvalid
		}
		t, err := tx.Type(ctx, in.TypeCode)
		if err != nil {
			return model.MembershipResult{}, err
		}
		if !t.Enabled || (t.Code == "CLASES" && t.Classes == nil) {
			return model.MembershipResult{}, model.ErrInvalid
		}
		end := in.End
		startDate, err := date(start)
		if err != nil {
			return model.MembershipResult{}, err
		}
		if expected, ok := t.EndFor(startDate); ok {
			if end != "" && end != expected.Format("2006-01-02") {
				return model.MembershipResult{}, model.ErrInvalid
			}
			end = expected.Format("2006-01-02")
		}
		// Solo un tipo por clases sin duración configurada exige el vencimiento.
		if end == "" || end < start {
			return model.MembershipResult{}, model.ErrInvalid
		}
		id, err := newUUID()
		if err != nil {
			return model.MembershipResult{}, err
		}
		m := model.Membership{ID: id, UserID: in.UserID, Type: t, Start: start, End: end, State: "ACTIVA", PreviousID: previous, Reason: in.Reason}
		if err := tx.Insert(ctx, m, actor, s.now().UTC()); err != nil {
			return model.MembershipResult{}, err
		}
		m = m.At(today)
		return model.MembershipResult{Membership: &m}, nil
	})
}
func (s *Memberships) Cancel(ctx context.Context, actor, role, key, id, reason string) (model.MembershipResult, bool, error) {
	reason = strings.TrimSpace(reason)
	if err := admin(actor, role, key, reason); err != nil {
		return model.MembershipResult{}, false, err
	}
	if !uuidIdentity.MatchString(id) {
		return model.MembershipResult{}, false, model.ErrInvalid
	}
	return s.repo.RunMembership(ctx, actor, key, fingerprint("CANCELAR", []string{id, reason}), func(tx MembershipTx) (model.MembershipResult, error) {
		m, err := tx.Get(ctx, id)
		if err != nil {
			return model.MembershipResult{}, err
		}
		if _, err := tx.LockUser(ctx, m.UserID); err != nil {
			return model.MembershipResult{}, err
		}
		// Releer tras el lock del usuario para resolver renovaciones/cancelaciones concurrentes.
		m, err = tx.Get(ctx, id)
		if err != nil {
			return model.MembershipResult{}, err
		}
		if m.At(model.ArgentinaDate(s.now())).State != "ACTIVA" {
			return model.MembershipResult{}, model.ErrMembershipTransition
		}
		if err := tx.Cancel(ctx, id, actor, reason, s.now().UTC()); err != nil {
			return model.MembershipResult{}, err
		}
		m.State = "CANCELADA"
		m.Reason = reason
		m = m.At(model.ArgentinaDate(s.now()))
		return model.MembershipResult{Membership: &m}, nil
	})
}
func (s *Memberships) Expire(ctx context.Context) (int64, error) {
	return s.repo.ExpireMemberships(ctx, model.ArgentinaDate(s.now()), s.now().UTC())
}

// Coverage responde, sin caché, si el alumno tiene membresía vigente hoy y en
// cada fecha pedida, con la membresía que cubre cada una (RN-01, RN-12, D-03;
// ADR-007: la vigencia no se cachea). La usa booking para reservar.
func (s *Memberships) Coverage(ctx context.Context, user string, dates []string) (model.Coverage, error) {
	if !uuidIdentity.MatchString(user) || len(dates) > 31 {
		return model.Coverage{}, model.ErrInvalid
	}
	for _, d := range dates {
		if _, err := date(d); err != nil {
			return model.Coverage{}, err
		}
	}
	active, memberships, err := s.repo.ActiveMemberships(ctx, user)
	if err != nil {
		return model.Coverage{}, err
	}
	today := model.ArgentinaDate(s.now())
	out := model.Coverage{StudentActive: active, CoveredOn: map[string]model.DateCoverage{}, EvaluatedAt: s.now().UTC().Format(time.RFC3339)}
	// Un alumno dado de baja no tiene membresía vigente para reservar (RN-12).
	if m := model.CoveringMembership(memberships, today); active && m != nil {
		end := m.End
		out.CoveredToday, out.ExpiresToday = true, &end
	}
	for _, d := range dates {
		coverage := model.DateCoverage{}
		if m := model.CoveringMembership(memberships, d); active && m != nil {
			id, end := m.ID, m.End
			coverage = model.DateCoverage{Covered: true, MembershipID: &id, Expires: &end}
		}
		out.CoveredOn[d] = coverage
	}
	return out, nil
}
