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
	input := func(user, code string, days int) model.MembershipInput {
		return model.MembershipInput{UserID: user, TypeCode: code, Start: model.ArgentinaDate(now), End: model.ArgentinaDate(now.AddDate(0, 0, days)), Reason: "Asignación de prueba"}
	}
	t.Run("usuario_sin_membresia_y_permisos", func(t *testing.T) {
		user := newUser(access.Student)
		out, err := app.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.False(t, out.Active)
		require.Empty(t, out.Items)
		_, err = app.List(ctx, newUser(access.Student), access.Student, user, 1, 20)
		require.ErrorIs(t, err, model.ErrForbidden)
		_, _, err = app.Assign(ctx, user, access.Student, "denegado-001", input(user, "MENSUAL", 30), false)
		require.ErrorIs(t, err, model.ErrForbidden)
		_, _, err = app.Assign(ctx, user, access.Admin, "rol-falso-001", input(user, "MENSUAL", 30), false)
		require.ErrorIs(t, err, model.ErrForbidden)
	})
	t.Run("asignacion_idempotente_outbox_auditoria", func(t *testing.T) {
		user := newUser(access.Student)
		in := input(user, "MENSUAL", 30)
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
				_, _, err := app.Assign(ctx, actor, access.Admin, fmt.Sprintf("simultaneo-%d", i), input(user, "ANUAL", 100), false)
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
		first, _, err := app.Assign(ctx, admin, access.Admin, "futuro-001", input(user, "MENSUAL", 20), false)
		require.NoError(t, err)
		in := input(user, "TRIMESTRAL", 50)
		in.Start = ""
		second, _, err := app.Assign(ctx, admin, access.Admin, "futuro-002", in, true)
		require.NoError(t, err)
		require.Equal(t, model.ArgentinaDate(now.AddDate(0, 0, 21)), second.Membership.Start)
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
		first, _, err := app.Assign(ctx, admin, access.Admin, "vence-001", input(user, "MENSUAL", 0), false)
		require.NoError(t, err)
		later := service.NewMemberships(db, func() time.Time { return now.AddDate(0, 0, 1) })
		out, err := later.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.False(t, out.Active)
		require.Equal(t, "VENCIDA", out.Items[0].State)
		_, err = later.Expire(ctx)
		require.NoError(t, err)
		in := input(user, "ANUAL", 365)
		in.Start = ""
		next, _, err := later.Assign(ctx, admin, access.Admin, "vence-002", in, true)
		require.NoError(t, err)
		require.Equal(t, model.ArgentinaDate(now.AddDate(0, 0, 1)), next.Membership.Start)
		_, _, err = later.Cancel(ctx, admin, access.Admin, "cancelar-vencida", first.Membership.ID, "No corresponde cancelar")
		require.ErrorIs(t, err, model.ErrMembershipTransition)
		_, _, err = later.Cancel(ctx, admin, access.Admin, "cancelar-activa", next.Membership.ID, "Cambio de plan solicitado")
		require.NoError(t, err)
		in = input(user, "TRIMESTRAL", 90)
		in.Start = model.ArgentinaDate(now.AddDate(0, 0, 1))
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
		_, _, err := app.Assign(ctx, admin, access.Admin, "clases-sin-config", input(user, "CLASES", 29), false)
		require.ErrorIs(t, err, model.ErrInvalid)
		classes, days := 8, 30
		typ := model.MembershipType{Code: "CLASES", Name: "Clases configurable", Classes: &classes, Days: &days, Enabled: true}
		_, _, err = app.Configure(ctx, admin, access.Admin, "config-clases-001", "Condición de prueba", typ)
		require.NoError(t, err)
		first, _, err := app.Assign(ctx, admin, access.Admin, "clases-001", input(user, "CLASES", 29), false)
		require.NoError(t, err)
		require.Equal(t, 8, *first.Membership.Type.Classes)
		classes = 12
		typ.Enabled = false
		_, _, err = app.Configure(ctx, admin, access.Admin, "config-clases-002", "Otra condición de prueba", typ)
		require.NoError(t, err)
		out, err := app.List(ctx, user, access.Student, user, 1, 20)
		require.NoError(t, err)
		require.Equal(t, 8, *out.Current.Type.Classes)
		_, _, err = app.Assign(ctx, admin, access.Admin, "clases-inactivo", input(newUser(access.Student), "CLASES", 29), false)
		require.ErrorIs(t, err, model.ErrInvalid)
	})
	t.Run("fechas_invalidas_y_rollback", func(t *testing.T) {
		user := newUser(access.Student)
		in := input(user, "MENSUAL", 10)
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
}
