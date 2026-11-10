// ADR-006; RN-CB-03/11/13: identidad verificada y cuenta propia.
package server

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/stretchr/testify/require"
)

var jwtFixture struct {
	once            sync.Once
	private, public string
	err             error
}

func testJWTKeys(t *testing.T) (string, string) {
	t.Helper()
	jwtFixture.once.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			jwtFixture.err = err
			return
		}
		jwtFixture.private = string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
		der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
		if err != nil {
			jwtFixture.err = err
			return
		}
		jwtFixture.public = string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	})
	require.NoError(t, jwtFixture.err)
	return jwtFixture.private, jwtFixture.public
}

func testJWT(t *testing.T, role string, now time.Time) string {
	t.Helper()
	private, _ := testJWTKeys(t)
	signer, err := access.NewSigner(private)
	require.NoError(t, err)
	token, err := signer.Issue("11111111-1111-4111-8111-111111111111", role, now)
	require.NoError(t, err)
	return token
}

func adminHeaders(t *testing.T) map[string]string {
	return map[string]string{"Authorization": "Bearer " + testJWT(t, access.Admin, time.Now())}
}

func TestCuentaPropiaJWTEliminaIdentidadFalsificada(t *testing.T) {
	secret := strings.Repeat("s", 32)
	members := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("No debe consultar sesiones en Members para verificar JWT")
		w.WriteHeader(500)
	}))
	defer members.Close()
	benefits := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "11111111-1111-4111-8111-111111111111", r.Header.Get("X-User-Id"))
		require.Equal(t, access.Student, r.Header.Get("X-User-Role"))
		require.Equal(t, secret, r.Header.Get("X-Internal-Service-Key"))
		require.Empty(t, r.Header.Get("Authorization"), "el servicio recibe actor, no JWT")
		_, _ = w.Write([]byte(`{"balance":150}`))
	}))
	defer benefits.Close()
	f := newFixture(t, map[string]string{"GATEWAY_MEMBERS_URL": members.URL, "GATEWAY_BENEFITS_URL": benefits.URL, "GYM_INTERNAL_SERVICE_KEY": secret})
	require.Equal(t, 401, f.do("GET", "/api/v1/benefits/me", map[string]string{"X-User-Id": "victima", "X-Internal-Service-Key": secret}).Code)
	require.Equal(t, 401, f.do("GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer token-opaco-anterior"}).Code)
	require.Equal(t, 401, f.do("GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer " + testJWT(t, access.Student, time.Now().Add(-16*time.Minute))}).Code)
	require.Equal(t, 200, f.do("GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer " + testJWT(t, access.Student, time.Now()), "X-User-Id": "victima", "X-User-Role": access.Admin}).Code)
	require.Equal(t, 401, f.do("GET", "/api/v1/benefits/me/movements", map[string]string{"X-User-Id": "victima", "X-Internal-Service-Key": secret}).Code)
	require.Equal(t, 200, f.do("GET", "/api/v1/benefits/me/movements", map[string]string{"Authorization": "Bearer " + testJWT(t, access.Student, time.Now()), "X-User-Id": "victima", "X-User-Role": access.Admin}).Code)
}

func TestRutasPrivadasYRolesFallanCerrados(t *testing.T) {
	f := newFixture(t, nil)
	for _, role := range []string{access.Student, access.Teacher, access.Nutritionist} {
		headers := map[string]string{"Authorization": "Bearer " + testJWT(t, role, time.Now()), "X-User-Role": access.Admin}
		require.Equal(t, 403, f.do("GET", "/api/v1/users", headers).Code)
		require.Equal(t, 200, f.do("GET", "/api/v1/users/me", headers).Code)
		require.Equal(t, 200, f.do("GET", "/api/v1/memberships/me", headers).Code)
		require.Equal(t, 200, f.do("GET", "/api/v1/memberships/types", headers).Code)
		require.Equal(t, 403, f.do("GET", "/api/v1/memberships/users/22222222-2222-4222-8222-222222222222", headers).Code)
		require.Equal(t, 403, f.do("POST", "/api/v1/memberships", headers).Code)
	}
	require.Equal(t, 200, f.do("GET", "/api/v1/users", adminHeaders(t)).Code)
	require.Equal(t, 401, f.do("GET", "/api/v1/users", nil).Code)
	require.Equal(t, 200, f.do("POST", "/api/v1/auth/login", nil).Code)
	require.Equal(t, 200, f.do("POST", "/api/v1/auth/activate", nil).Code)
	require.Equal(t, 200, f.do("POST", "/api/v1/auth/resend-activation", nil).Code)
	missing := newFixture(t, map[string]string{"GATEWAY_JWT_PUBLIC_KEY": ""})
	require.Equal(t, 401, missing.do("GET", "/api/v1/users/me", adminHeaders(t)).Code)
	req := httptest.NewRequest("GET", "/api/v1/users/me", nil)
	req.Header.Add("Authorization", "Bearer "+testJWT(t, access.Student, time.Now()))
	req.Header.Add("Authorization", "Bearer otro")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	require.Equal(t, 401, w.Code)
}

// users.v1 y RN-30: "mis alumnos" solo para profesor y nutricionista (y
// administración); un alumno recibe 403 aunque falsifique su rol.
func TestMisAlumnosSoloProfesionales(t *testing.T) {
	f := newFixture(t, nil)
	for role, want := range map[string]int{access.Teacher: 200, access.Nutritionist: 200, access.Student: 403, access.Admin: 200} {
		headers := map[string]string{"Authorization": "Bearer " + testJWT(t, role, time.Now()), "X-User-Role": access.Teacher}
		require.Equal(t, want, f.do("GET", "/api/v1/users/me/students", headers).Code, role)
	}
	teacher := map[string]string{"Authorization": "Bearer " + testJWT(t, access.Teacher, time.Now())}
	require.Equal(t, 403, f.do("GET", "/api/v1/users", teacher).Code, "el listado completo es solo de administración")
}

// booking.v1, D-15 y D-82: matriz gruesa de roles de booking en el gateway.
func TestRutasDeBookingPorRol(t *testing.T) {
	f := newFixture(t, nil)
	const id = "00000000-0000-4000-8000-000000000010"
	casos := []struct {
		method, path string
		permitido    map[string]bool
	}{
		{"GET", "/api/v1/activities", map[string]bool{access.Student: true, access.Teacher: true, access.Nutritionist: true}},
		{"POST", "/api/v1/activities", nil},
		{"POST", "/api/v1/activities/" + id + "/schedules", nil},
		{"POST", "/api/v1/classes/generate", nil},
		{"GET", "/api/v1/classes", map[string]bool{access.Student: true, access.Teacher: true}},
		{"GET", "/api/v1/classes/" + id, map[string]bool{access.Student: true, access.Teacher: true}},
		{"GET", "/api/v1/classes/mine", map[string]bool{access.Teacher: true}},
		{"PUT", "/api/v1/classes/" + id + "/capacity", nil},
		{"POST", "/api/v1/classes/" + id + "/cancel", nil},
		{"POST", "/api/v1/bookings", map[string]bool{access.Student: true}},
		{"GET", "/api/v1/bookings/me", map[string]bool{access.Student: true}},
		{"POST", "/api/v1/bookings/" + id + "/cancel", map[string]bool{access.Student: true}},
		{"POST", "/api/v1/check-ins", nil},
	}
	for _, c := range casos {
		for _, role := range []string{access.Student, access.Teacher, access.Nutritionist, access.Admin} {
			want := 403
			if role == access.Admin || c.permitido[role] {
				want = 200
			}
			headers := map[string]string{"Authorization": "Bearer " + testJWT(t, role, time.Now())}
			require.Equal(t, want, f.do(c.method, c.path, headers).Code, "%s %s como %s", c.method, c.path, role)
		}
	}
}
