// HU-CB-03; RN-CB-01/02/08/11; HU-03/04/05/12/27; RN-01–06.
package repository

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/model"
	"github.com/GuadiViti/Arquitectura-de-Software/services/members-service/internal/service"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMembresiasPostgres(t *testing.T) {
	requireDocker(t)
	ctx := context.Background()
	ctr, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("bootstrap"), tcpostgres.WithUsername("root"), tcpostgres.WithPassword("fixture"), testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = ctr.Terminate(context.Background()) })
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	root, err := NewPostgres(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(root.Close)
	_, err = root.pool.Exec(ctx, `CREATE ROLE membership_owner LOGIN PASSWORD 'fixture' NOSUPERUSER NOCREATEDB NOCREATEROLE`)
	require.NoError(t, err)
	_, err = root.pool.Exec(ctx, `CREATE DATABASE memberships OWNER membership_owner`)
	require.NoError(t, err)
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	u.Path = "/memberships"
	u.User = url.UserPassword("membership_owner", "fixture")
	db, err := NewPostgres(ctx, u.String())
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.Migrate(ctx))
	require.NoError(t, db.Migrate(ctx))
	newUser := func(role string) string {
		var id string
		require.NoError(t, db.pool.QueryRow(ctx, `INSERT INTO usuarios(id,nombre,email,rol) VALUES(gen_random_uuid(),'Fixture',gen_random_uuid()::text||'@example.test',$1) RETURNING id::text`, role).Scan(&id))
		return id
	}
	admin, otherAdmin := newUser(access.Admin), newUser(access.Admin)
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	app := service.NewMemberships(db, func() time.Time { return now })
	input := func(user, code string) model.MembershipInput {
		// Sin vencimiento: el servicio lo calcula según el tipo (D-77).
		return model.MembershipInput{UserID: user, TypeCode: code, Start: model.ArgentinaDate(now), Reason: "Asignación de prueba"}
	}
	t.Run("usuario_sin_membresia_y_permisos", func(t *testing.T) {
		user := newUser(access.Student)
		out, err := app.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.False(t, out.Active)
		require.Empty(t, out.Items)
		_, err = app.List(ctx, newUser(access.Student), access.Student, user, 1, 20)
		require.ErrorIs(t, err, model.ErrForbidden)
		_, _, err = app.Assign(ctx, user, access.Student, "denegado-001", input(user, "MENSUAL"), false)
		require.ErrorIs(t, err, model.ErrForbidden)
		_, _, err = app.Assign(ctx, user, access.Admin, "rol-falso-001", input(user, "MENSUAL"), false)
		require.ErrorIs(t, err, model.ErrForbidden)
	})
	t.Run("asignacion_idempotente_outbox_auditoria", func(t *testing.T) {
		user := newUser(access.Student)
		in := input(user, "MENSUAL")
		first, replay, err := app.Assign(ctx, admin, access.Admin, "asignar-001", in, false)
		require.NoError(t, err)
		require.False(t, replay)
		again, replay, err := app.Assign(ctx, admin, access.Admin, "asignar-001", in, false)
		require.NoError(t, err)
		require.True(t, replay)
		require.Equal(t, first, again)
		in.Reason = "Otro motivo"
		_, _, err = app.Assign(ctx, admin, access.Admin, "asignar-001", in, false)
		require.ErrorIs(t, err, model.ErrConflict)
		var count int
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM outbox_membresias WHERE membresia_id=$1`, first.Membership.ID).Scan(&count))
		require.Equal(t, 1, count)
		require.NoError(t, db.pool.QueryRow(ctx, `SELECT count(*) FROM historial_membresias WHERE entidad_id=$1`, first.Membership.ID).Scan(&count))
		require.Equal(t, 1, count)
		_, err = db.pool.Exec(ctx, `UPDATE historial_membresias SET motivo='alterado' WHERE entidad_id=$1`, first.Membership.ID)
		require.Error(t, err)
	})
	t.Run("no_solapamiento_concurrente", func(t *testing.T) {
		user := newUser(access.Student)
		start := make(chan struct{})
		results := make(chan error, 2)
		var group sync.WaitGroup
		for i, actor := range []string{admin, otherAdmin} {
			group.Add(1)
			go func(i int, actor string) {
				defer group.Done()
				<-start
				_, _, err := app.Assign(ctx, actor, access.Admin, fmt.Sprintf("simultaneo-%d", i), input(user, "ANUAL"), false)
				results <- err
			}(i, actor)
		}
		close(start)
		group.Wait()
		close(results)
		ok, conflicts := 0, 0
		for err := range results {
			if err == nil {
				ok++
			} else {
				require.ErrorIs(t, err, model.ErrMembershipOverlap)
				conflicts++
			}
		}
		require.Equal(t, 1, ok)
		require.Equal(t, 1, conflicts)
	})
	t.Run("renovar_futuro_preserva_original", func(t *testing.T) {
		user := newUser(access.Student)
		first, _, err := app.Assign(ctx, admin, access.Admin, "futuro-001", input(user, "MENSUAL"), false)
		require.NoError(t, err)
		in := input(user, "TRIMESTRAL")
		in.Start = ""
		second, _, err := app.Assign(ctx, admin, access.Admin, "futuro-002", in, true)
		require.NoError(t, err)
		// La mensual del 06/11 vence el 05/12 (D-77); la renovación empieza el 06/12.
		require.Equal(t, "2026-12-05", first.Membership.End)
		require.Equal(t, "2026-12-06", second.Membership.Start)
		require.Equal(t, "2027-03-05", second.Membership.End)
		require.False(t, second.Membership.Active)
		require.Equal(t, "PROGRAMADA", second.Membership.Condition)
		out, err := app.List(ctx, user, access.Student, user, 1, 1)
		require.NoError(t, err)
		require.EqualValues(t, 2, out.Total)
		require.Equal(t, first.Membership.ID, out.Current.ID)
		_, err = db.pool.Exec(ctx, `UPDATE membresias SET fecha_vencimiento=fecha_vencimiento+1 WHERE id=$1`, first.Membership.ID)
		require.Error(t, err)
	})
	t.Run("vencer_retornar_y_cambiar_tipo_sin_borrar_historia", func(t *testing.T) {
		user := newUser(access.Student)
		first, _, err := app.Assign(ctx, admin, access.Admin, "vence-001", input(user, "MENSUAL"), false)
		require.NoError(t, err)
		// Un mes después la mensual (06/11–05/12) ya venció.
		laterNow := now.AddDate(0, 1, 0)
		later := service.NewMemberships(db, func() time.Time { return laterNow })
		out, err := later.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.False(t, out.Active)
		require.Equal(t, "VENCIDA", out.Items[0].State)
		_, err = later.Expire(ctx)
		require.NoError(t, err)
		in := input(user, "ANUAL")
		in.Start = ""
		next, _, err := later.Assign(ctx, admin, access.Admin, "vence-002", in, true)
		require.NoError(t, err)
		require.Equal(t, model.ArgentinaDate(laterNow), next.Membership.Start)
		_, _, err = later.Cancel(ctx, admin, access.Admin, "cancelar-vencida", first.Membership.ID, "No corresponde cancelar")
		require.ErrorIs(t, err, model.ErrMembershipTransition)
		_, _, err = later.Cancel(ctx, admin, access.Admin, "cancelar-activa", next.Membership.ID, "Cambio de plan solicitado")
		require.NoError(t, err)
		in = input(user, "TRIMESTRAL")
		in.Start = model.ArgentinaDate(laterNow)
		_, _, err = later.Assign(ctx, admin, access.Admin, "cambiar-plan-001", in, false)
		require.NoError(t, err)
		out, err = later.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.EqualValues(t, 3, out.Total)
		require.True(t, out.Active)
		_, err = db.pool.Exec(ctx, `DELETE FROM membresias WHERE id=$1`, first.Membership.ID)
		require.Error(t, err)
	})
	t.Run("clases_configurables_snapshot_y_tipo_inactivo", func(t *testing.T) {
		user := newUser(access.Student)
		_, _, err := app.Assign(ctx, admin, access.Admin, "clases-sin-config", input(user, "CLASES"), false)
		require.ErrorIs(t, err, model.ErrInvalid)
		classes, days := 8, 30
		typ := model.MembershipType{Code: "CLASES", Name: "Clases configurable", Classes: &classes, Days: &days, Enabled: true}
		_, _, err = app.Configure(ctx, admin, access.Admin, "config-clases-001", "Condición de prueba", typ)
		require.NoError(t, err)
		first, _, err := app.Assign(ctx, admin, access.Admin, "clases-001", input(user, "CLASES"), false)
		require.NoError(t, err)
		require.Equal(t, 8, *first.Membership.Type.Classes)
		classes = 12
		typ.Enabled = false
		_, _, err = app.Configure(ctx, admin, access.Admin, "config-clases-002", "Otra condición de prueba", typ)
		require.NoError(t, err)
		out, err := app.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.Equal(t, 8, *out.Current.Type.Classes)
		_, _, err = app.Assign(ctx, admin, access.Admin, "clases-inactivo", input(newUser(access.Student), "CLASES"), false)
		require.ErrorIs(t, err, model.ErrInvalid)
	})
	t.Run("fechas_invalidas_y_rollback", func(t *testing.T) {
		user := newUser(access.Student)
		in := input(user, "MENSUAL")
		in.Start = model.ArgentinaDate(now.AddDate(0, 0, -1))
		_, _, err := app.Assign(ctx, admin, access.Admin, "fecha-pasada", in, false)
		require.ErrorIs(t, err, model.ErrInvalid)
		in.Start = model.ArgentinaDate(now)
		in.End = "2026-02-30"
		_, _, err = app.Assign(ctx, admin, access.Admin, "fecha-imposible", in, false)
		require.ErrorIs(t, err, model.ErrInvalid)
		out, err := app.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.Zero(t, out.Total)
	})
	// CA-03.1/CA-03.3, HU-03 y D-77: vencimiento según el tipo y solo alumnos.
	t.Run("vencimiento_segun_tipo_y_solo_alumnos", func(t *testing.T) {
		user := newUser(access.Student)
		in := input(user, "ANUAL")
		in.End = "2028-11-06"
		_, _, err := app.Assign(ctx, admin, access.Admin, "anual-largo", in, false)
		require.ErrorIs(t, err, model.ErrInvalid, "una anual no puede durar dos años")
		in.End = "2027-11-05"
		annual, _, err := app.Assign(ctx, admin, access.Admin, "anual-exacta", in, false)
		require.NoError(t, err)
		require.Equal(t, "2027-11-05", annual.Membership.End)
		for _, role := range []string{access.Admin, access.Teacher, access.Nutritionist} {
			_, _, err = app.Assign(ctx, admin, access.Admin, "rol-"+role, input(newUser(role), "MENSUAL"), false)
			require.ErrorIs(t, err, model.ErrInvalid, "rol %s", role)
		}
		_, _, err = app.Configure(ctx, admin, access.Admin, "mensual-sin-duracion", "Prueba sin período", model.MembershipType{Code: "MENSUAL", Name: "Mensual", Enabled: true})
		require.ErrorIs(t, err, model.ErrInvalid)
	})
}
