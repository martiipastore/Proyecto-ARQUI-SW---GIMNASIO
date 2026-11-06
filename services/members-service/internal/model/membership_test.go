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
