package proxy

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
)

// Etapa 10: matriz gruesa de training.v1 por rol (RN-30, CA-23.5, CA-24.2,
// CA-34.3). training-service vuelve a autorizar sobre el alumno.
func TestTrainingAllowed(t *testing.T) {
	const id = "6f1c2a8e-3b7d-4c1e-9a2b-1d2e3f4a5b6c"
	casos := []struct {
		method, path string
		permitidos   []string
	}{
		{http.MethodGet, "/api/v1/training/plans/me", []string{access.Student}},
		{http.MethodGet, "/api/v1/training/plans", []string{access.Teacher}},
		{http.MethodPost, "/api/v1/training/plans", []string{access.Teacher}},
		{http.MethodGet, "/api/v1/nutrition/plans/me", []string{access.Student}},
		{http.MethodPost, "/api/v1/nutrition/plans", []string{access.Nutritionist}},
		{http.MethodGet, "/api/v1/nutrition/plans", []string{access.Nutritionist}},
		{http.MethodGet, "/api/v1/nutrition/measurements/me", []string{access.Student}},
		{http.MethodPost, "/api/v1/nutrition/measurements", []string{access.Nutritionist}},
		{http.MethodGet, "/api/v1/nutrition/questions/me", []string{access.Student}},
		{http.MethodPost, "/api/v1/nutrition/questions", []string{access.Student}},
		{http.MethodGet, "/api/v1/nutrition/questions", []string{access.Nutritionist}},
		{http.MethodPost, "/api/v1/nutrition/questions/" + id + "/answer", []string{access.Nutritionist}},
		{http.MethodPut, "/api/v1/nutrition/measurements", nil},
		{http.MethodDelete, "/api/v1/nutrition/measurements", nil},
		{http.MethodPost, "/api/v1/nutrition/questions//answer", nil},
	}
	for _, c := range casos {
		for _, rol := range []string{access.Student, access.Teacher, access.Nutritionist} {
			esperado := false
			for _, p := range c.permitidos {
				esperado = esperado || p == rol
			}
			require.Equal(t, esperado, trainingAllowed(c.method, c.path, rol), "%s %s como %s", c.method, c.path, rol)
		}
	}
}
