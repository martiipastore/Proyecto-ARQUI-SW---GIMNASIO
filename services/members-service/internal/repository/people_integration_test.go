// Etapa 3 de la entrega final — HU-01, HU-02, RN-01, RN-12, RN-30, RN-33,
// RN-34, RN-36, SPEC §9 y D-78, contra PostgreSQL real (testcontainers).
package repository

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"strconv"
	"strings"
	"sync"
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

func TestPadronPostgres(t *testing.T) {
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
	require.NoError(t, db.Migrate(ctx), "las migraciones se pueden repetir")
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	signer, err := access.NewSigner(string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})))
	require.NoError(t, err)
	mail := &mailStub{}
	identity, err := service.NewIdentity(db, &fulfillStub{}, mail, strings.Repeat("x", 32), "https://gym.example", signer)
	require.NoError(t, err)
	people := service.NewPeople(db, identity, nil)
	memberships := service.NewMemberships(db, nil)

	count := func(query string, args ...any) int {
		var n int
		require.NoError(t, db.pool.QueryRow(ctx, query, args...).Scan(&n))
		return n
	}
	created, err := people.SeedAdmin(ctx, "admin@gym.test", "Administración", "contraseña-admin-123")
	require.NoError(t, err)
	require.True(t, created)
	var admin string
	require.NoError(t, db.pool.QueryRow(ctx, `SELECT id::text FROM usuarios WHERE email='admin@gym.test'`).Scan(&admin))

	dni := 30000000
	student := func(email string) model.PersonInput {
		dni++
		return model.PersonInput{Role: access.Student, Name: "Martina", LastName: "Pastore", DNI: strconv.Itoa(dni), Email: email, Phone: "+54 351 555-0000", BirthDate: "2001-05-10"}
	}
	// El worker envía un correo pendiente por pasada; otras altas de la prueba
	// pueden estar antes en la cola.
	activate := func(t *testing.T, email, password string) {
		t.Helper()
		for i := 0; i < 50; i++ {
			for _, sent := range mail.emails {
				if sent.To == email {
					require.NoError(t, identity.Activate(ctx, mailToken(t, sent), password))
					return
				}
			}
			require.NoError(t, identity.Process(ctx))
		}
		t.Fatalf("no se envió el correo de activación a %s", email)
	}

	t.Run("admin_inicial_idempotente_y_login", func(t *testing.T) {
		again, err := people.SeedAdmin(ctx, "admin@gym.test", "Otro nombre", "otra-contraseña-456")
		require.NoError(t, err)
		require.False(t, again, "no recrea ni pisa la contraseña")
		_, err = identity.Login(ctx, "admin@gym.test", "contraseña-admin-123")
		require.NoError(t, err)
		_, err = people.SeedAdmin(ctx, "corto@gym.test", "Admin", "corta")
		require.ErrorIs(t, err, model.ErrInvalid)
	})

	t.Run("CA01_1_alta_activacion_outbox_y_auditoria", func(t *testing.T) {
		p, err := people.Create(ctx, admin, access.Admin, student("martina@example.test"))
		require.NoError(t, err)
		require.Equal(t, "ACTIVO", p.State)
		require.True(t, p.PendingPassword)
		require.Equal(t, 1, count(`SELECT count(*) FROM outbox_usuarios WHERE usuario_id=$1 AND event_type='alumno.creado'`, p.ID))
		require.Equal(t, 1, count(`SELECT count(*) FROM auditoria_usuarios WHERE entidad_id=$1 AND actor_id=$2`, p.ID, admin))
		_, err = db.pool.Exec(ctx, `DELETE FROM auditoria_usuarios WHERE entidad_id=$1`, p.ID)
		require.Error(t, err, "la auditoría es inmutable (RN-36)")
		activate(t, "martina@example.test", "contraseña-martina-1")
		_, err = identity.Login(ctx, "martina@example.test", "contraseña-martina-1")
		require.NoError(t, err)
		me, err := identity.CurrentUser(ctx, p.ID)
		require.NoError(t, err)
		require.NotNil(t, me.Person)
		require.Equal(t, p.DNI, me.Person.DNI, "Mi cuenta muestra los datos del padrón")
	})

	t.Run("CA01_2_RN33_duplicados_y_concurrencia", func(t *testing.T) {
		first := student("unico@example.test")
		_, err := people.Create(ctx, admin, access.Admin, first)
		require.NoError(t, err)
		sameDNI := first
		sameDNI.Email = "otro@example.test"
		_, err = people.Create(ctx, admin, access.Admin, sameDNI)
		require.ErrorIs(t, err, model.ErrDuplicate)
		_, err = people.Create(ctx, admin, access.Admin, student("UNICO@example.test "))
		require.ErrorIs(t, err, model.ErrDuplicate, "el email se normaliza antes de comparar")
		concurrent := student("concurrente-a@example.test")
		other := concurrent
		other.Email = "concurrente-b@example.test"
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, in := range []model.PersonInput{concurrent, other} {
			wg.Add(1)
			go func(in model.PersonInput) {
				defer wg.Done()
				_, err := people.Create(ctx, admin, access.Admin, in)
				results <- err
			}(in)
		}
		wg.Wait()
		close(results)
		ok := 0
		for err := range results {
			if err == nil {
				ok++
			} else {
				require.ErrorIs(t, err, model.ErrDuplicate)
			}
		}
		require.Equal(t, 1, ok, "un solo alta con el mismo DNI")
	})

	t.Run("D78_persona_externa_pasa_a_alumna_sin_perder_cuenta", func(t *testing.T) {
		hash, err := bcrypt.GenerateFromPassword([]byte("contraseña-externa-1"), 12)
		require.NoError(t, err)
		var external string
		require.NoError(t, db.pool.QueryRow(ctx, `INSERT INTO usuarios(id,nombre,email,registro_externo,password_hash) VALUES(gen_random_uuid(),'Persona externa','externa@example.test',true,$1) RETURNING id::text`, hash).Scan(&external))
		professional := student("externa@example.test")
		professional.Role = access.Teacher
		_, err = people.Create(ctx, admin, access.Admin, professional)
		require.ErrorIs(t, err, model.ErrDuplicate, "solo se completa como alumno")
		p, err := people.Create(ctx, admin, access.Admin, student("externa@example.test"))
		require.NoError(t, err)
		require.Equal(t, external, p.ID, "conserva identidad y por lo tanto cuenta de puntos")
		require.False(t, p.PendingPassword)
		require.Zero(t, count(`SELECT count(*) FROM activaciones_cuenta WHERE usuario_id=$1`, p.ID))
		_, err = identity.Login(ctx, "externa@example.test", "contraseña-externa-1")
		require.NoError(t, err, "conserva su contraseña")
	})

	t.Run("CA02_1_CA02_2_profesionales_subtipo_y_matricula", func(t *testing.T) {
		teacher := student("profe@example.test")
		teacher.Role, teacher.BirthDate = access.Teacher, ""
		p, err := people.Create(ctx, admin, access.Admin, teacher)
		require.NoError(t, err)
		require.Equal(t, access.Teacher, p.Role)
		nutritionist := student("nutri@example.test")
		nutritionist.Role, nutritionist.License = access.Nutritionist, "MP-1234"
		p, err = people.Create(ctx, admin, access.Admin, nutritionist)
		require.NoError(t, err)
		require.Equal(t, "MP-1234", *p.License)
		for _, role := range []string{"PROFESIONAL", access.Admin, ""} {
			in := student("rol-" + strings.ToLower(role) + "@example.test")
			in.Role = role
			_, err = people.Create(ctx, admin, access.Admin, in)
			require.ErrorIs(t, err, model.ErrInvalid, "rol %q", role)
		}
		withLicense := student("profe-matricula@example.test")
		withLicense.Role, withLicense.License = access.Teacher, "MP-1"
		_, err = people.Create(ctx, admin, access.Admin, withLicense)
		require.ErrorIs(t, err, model.ErrInvalid, "matrícula solo para nutricionistas")
	})

	t.Run("SPEC9_validaciones_de_alta", func(t *testing.T) {
		future := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
		for name, mutate := range map[string]func(*model.PersonInput){
			"dni_corto":             func(in *model.PersonInput) { in.DNI = "123456" },
			"dni_con_letras":        func(in *model.PersonInput) { in.DNI = "30a00000" },
			"nacimiento_futuro":     func(in *model.PersonInput) { in.BirthDate = future },
			"nacimiento_1899":       func(in *model.PersonInput) { in.BirthDate = "1899-12-31" },
			"alumno_sin_nacimiento": func(in *model.PersonInput) { in.BirthDate = "" },
			"nombre_en_blanco":      func(in *model.PersonInput) { in.Name = "   " },
			"apellido_vacio":        func(in *model.PersonInput) { in.LastName = "" },
			"email_invalido":        func(in *model.PersonInput) { in.Email = "no-es-email" },
			"telefono_invalido":     func(in *model.PersonInput) { in.Phone = "llamame" },
		} {
			in := student("valida-" + strings.ReplaceAll(name, "_", "-") + "@example.test")
			mutate(&in)
			_, err := people.Create(ctx, admin, access.Admin, in)
			require.ErrorIs(t, err, model.ErrInvalid, name)
		}
		_, err := people.Create(ctx, admin, access.Student, student("sin-permiso@example.test"))
		require.ErrorIs(t, err, model.ErrForbidden)
	})

	t.Run("CA01_3_CA01_4_RN34_baja_logica", func(t *testing.T) {
		p, err := people.Create(ctx, admin, access.Admin, student("baja@example.test"))
		require.NoError(t, err)
		activate(t, "baja@example.test", "contraseña-baja-123")
		_, err = people.Deactivate(ctx, p.ID, access.Student, p.ID, "Pidió la baja")
		require.ErrorIs(t, err, model.ErrForbidden)
		out, err := people.Deactivate(ctx, admin, access.Admin, p.ID, "Pidió la baja")
		require.NoError(t, err)
		require.Equal(t, "INACTIVO", out.State)
		require.NotNil(t, out.DeactivatedAt)
		require.Equal(t, 1, count(`SELECT count(*) FROM outbox_usuarios WHERE usuario_id=$1 AND event_type='usuario.desactivado'`, p.ID))
		require.Equal(t, 1, count(`SELECT count(*) FROM usuarios WHERE id=$1`, p.ID), "no se elimina")
		_, err = identity.Login(ctx, "baja@example.test", "contraseña-baja-123")
		require.ErrorIs(t, err, model.ErrUnauthorized)
		_, err = people.Deactivate(ctx, admin, access.Admin, p.ID, "Otra vez")
		require.ErrorIs(t, err, model.ErrConflict)
		_, err = people.Deactivate(ctx, admin, access.Admin, admin, "Autobaja")
		require.ErrorIs(t, err, model.ErrInvalid)
	})

	t.Run("CA02_4_RN30_asignaciones", func(t *testing.T) {
		teacherIn := student("profe-asigna@example.test")
		teacherIn.Role, teacherIn.BirthDate = access.Teacher, ""
		teacher, err := people.Create(ctx, admin, access.Admin, teacherIn)
		require.NoError(t, err)
		alumno, err := people.Create(ctx, admin, access.Admin, student("asignado@example.test"))
		require.NoError(t, err)
		_, err = people.Assign(ctx, admin, access.Admin, alumno.ID, teacher.ID)
		require.ErrorIs(t, err, model.ErrInvalid, "roles invertidos")
		a, err := people.Assign(ctx, admin, access.Admin, teacher.ID, alumno.ID)
		require.NoError(t, err)
		require.Nil(t, a.Until)
		_, err = people.Assign(ctx, admin, access.Admin, teacher.ID, alumno.ID)
		require.ErrorIs(t, err, model.ErrAssignmentDuplicate)
		mine, err := people.MyStudents(ctx, teacher.ID, access.Teacher)
		require.NoError(t, err)
		require.Len(t, mine, 1)
		require.Equal(t, alumno.ID, mine[0].Student.ID)
		_, err = people.MyStudents(ctx, alumno.ID, access.Student)
		require.ErrorIs(t, err, model.ErrForbidden)
		ok, err := people.ActiveAssignment(ctx, teacher.ID, alumno.ID)
		require.NoError(t, err)
		require.True(t, ok)
		_, err = people.Revoke(ctx, admin, access.Admin, teacher.ID, alumno.ID)
		require.NoError(t, err)
		ok, err = people.ActiveAssignment(ctx, teacher.ID, alumno.ID)
		require.NoError(t, err)
		require.False(t, ok, "sin período de gracia (RN-30)")
		_, err = people.Revoke(ctx, admin, access.Admin, teacher.ID, alumno.ID)
		require.ErrorIs(t, err, model.ErrNotFound)
		_, err = people.Assign(ctx, admin, access.Admin, teacher.ID, alumno.ID)
		require.NoError(t, err, "se puede volver a asignar")
		_, _, err = people.Get(ctx, teacher.ID, access.Teacher, alumno.ID)
		require.ErrorIs(t, err, model.ErrForbidden)
		_, history, err := people.Get(ctx, admin, access.Admin, alumno.ID)
		require.NoError(t, err)
		require.Len(t, history, 2, "la asignación revocada se conserva")
		inactive, err := people.Create(ctx, admin, access.Admin, student("asigna-inactivo@example.test"))
		require.NoError(t, err)
		_, err = people.Deactivate(ctx, admin, access.Admin, inactive.ID, "Baja de prueba")
		require.NoError(t, err)
		_, err = people.Assign(ctx, admin, access.Admin, teacher.ID, inactive.ID)
		require.ErrorIs(t, err, model.ErrInvalid)
	})

	t.Run("APIs_internas_busqueda_y_vigencia", func(t *testing.T) {
		in := student("interna@example.test")
		p, err := people.Create(ctx, admin, access.Admin, in)
		require.NoError(t, err)
		found, err := people.StudentByDNI(ctx, in.DNI)
		require.NoError(t, err)
		require.Equal(t, p.ID, found.ID)
		_, err = people.StudentByDNI(ctx, "99999999")
		require.ErrorIs(t, err, model.ErrNotFound)
		summaries, err := people.StudentsByID(ctx, []string{p.ID, admin})
		require.NoError(t, err)
		require.Len(t, summaries, 1, "solo alumnos")
		page, err := people.List(ctx, admin, access.Admin, model.PersonFilter{Role: access.Student, Query: in.DNI, Page: 1, Size: 20})
		require.NoError(t, err)
		require.EqualValues(t, 1, page.Total)
		_, err = people.List(ctx, p.ID, access.Student, model.PersonFilter{Page: 1, Size: 20})
		require.ErrorIs(t, err, model.ErrForbidden)

		// RN-01/RN-12: sin membresía no hay vigencia; con una mensual, hoy y el
		// último día quedan cubiertos y el día siguiente no (D-77).
		today := model.ArgentinaDate(time.Now())
		empty, err := memberships.Coverage(ctx, p.ID, []string{today})
		require.NoError(t, err)
		require.True(t, empty.StudentActive)
		require.False(t, empty.CoveredToday)
		m, _, err := memberships.Assign(ctx, admin, access.Admin, "vigencia-001", model.MembershipInput{UserID: p.ID, TypeCode: "MENSUAL", Start: today, Reason: "Alta de prueba"}, false)
		require.NoError(t, err)
		end := m.Membership.End
		next, err := time.Parse("2006-01-02", end)
		require.NoError(t, err)
		after := next.AddDate(0, 0, 1).Format("2006-01-02")
		coverage, err := memberships.Coverage(ctx, p.ID, []string{today, end, after})
		require.NoError(t, err)
		require.True(t, coverage.CoveredToday)
		require.True(t, coverage.CoveredOn[end].Covered)
		require.Equal(t, m.Membership.ID, *coverage.CoveredOn[end].MembershipID)
		require.False(t, coverage.CoveredOn[after].Covered)
		_, err = people.Deactivate(ctx, admin, access.Admin, p.ID, "Baja de prueba")
		require.NoError(t, err)
		inactive, err := memberships.Coverage(ctx, p.ID, []string{today})
		require.NoError(t, err)
		require.False(t, inactive.StudentActive)
		require.False(t, inactive.CoveredToday, "un alumno dado de baja no puede reservar")
		_, err = memberships.Coverage(ctx, "no-es-uuid", nil)
		require.ErrorIs(t, err, model.ErrInvalid)
	})
}
