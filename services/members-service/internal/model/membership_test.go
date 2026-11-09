// HU-CB-03, HU-12/27; RN-CB-01/02/08; RN-01/03: días argentinos inclusivos.
package model

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestMembresiaVigenciaCalendario(t *testing.T) {
	last := time.Date(2026, 11, 7, 2, 59, 59, 0, time.UTC)
	require.Equal(t, "2026-11-06", ArgentinaDate(last))
	require.Equal(t, "2026-11-07", ArgentinaDate(last.Add(time.Second)))
	m := Membership{Start: "2026-11-01", End: "2026-11-06", State: "ACTIVA"}
	require.True(t, m.At(ArgentinaDate(last)).Active)
	require.Equal(t, "VENCIDA", m.At(ArgentinaDate(last.Add(time.Second))).State)
	require.Equal(t, "PROGRAMADA", m.At("2026-10-31").Condition)
	m.State = "CANCELADA"
	require.False(t, m.At("2026-11-01").Active)
}

// CA-03.1, S-04 y D-77: el vencimiento es inclusivo y termina el día anterior
// al mismo día del período siguiente; si ese día no existe, el último del mes.
func TestMembresiaVencimientoSegunTipo(t *testing.T) {
	months := func(n int) MembershipType { return MembershipType{Months: &n} }
	days := func(n int) MembershipType { return MembershipType{Days: &n} }
	for _, tc := range []struct {
		tipo         MembershipType
		inicio, want string
	}{
		{months(1), "2026-11-09", "2026-12-08"},
		{months(3), "2026-11-09", "2027-02-08"},
		{months(12), "2026-11-09", "2027-11-08"},
		{months(1), "2026-12-01", "2026-12-31"},
		{months(1), "2027-01-31", "2027-02-28"},
		{months(1), "2028-01-31", "2028-02-29"},
		{months(1), "2027-01-28", "2027-02-27"},
		{months(1), "2026-12-15", "2027-01-14"},
		{days(30), "2026-11-09", "2026-12-08"},
		{days(1), "2026-11-09", "2026-11-09"},
	} {
		inicio, err := time.Parse("2006-01-02", tc.inicio)
		require.NoError(t, err)
		got, ok := tc.tipo.EndFor(inicio)
		require.True(t, ok)
		require.Equal(t, tc.want, got.Format("2006-01-02"), "inicio %s", tc.inicio)
	}
	_, ok := MembershipType{}.EndFor(time.Now())
	require.False(t, ok, "sin duración no hay vencimiento calculable")
}
