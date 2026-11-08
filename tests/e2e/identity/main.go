// Local CB-03 E2E: isolated PostgreSQL/Mailpit plus the real service binaries.
// Run from the repository root: go run ./tests/e2e/identity/main.go
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"golang.org/x/crypto/bcrypt"
)

type check struct {
	Name   string `json:"name"`
	Passed bool   `json:"passed"`
}
type report struct {
	Stage    string  `json:"stage"`
	Started  string  `json:"startedAtUtc"`
	Finished string  `json:"finishedAtUtc"`
	Passed   bool    `json:"passed"`
	Checks   []check `json:"checks"`
	Scope    string  `json:"scope"`
}

var result = report{Stage: "CB-03", Started: time.Now().UTC().Format(time.RFC3339), Scope: "Local real HTTP services, partner API v2, isolated PostgreSQL 16, Mailpit v1.20 SMTP and injected lost callback response; no production or browser automation"}
var client = &http.Client{Timeout: 5 * time.Second}
var ctx context.Context
var withMemberships = flag.Bool("memberships", false, "agregar escenarios CB-04 reutilizando el flujo CB-03")

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func ensure(ok bool, message string) {
	if !ok {
		panic(message)
	}
}
func mark(name string) {
	result.Checks = append(result.Checks, check{name, true})
	fmt.Println("OK: " + name)
}
func randomHex() string {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	must(err)
	return hex.EncodeToString(b)
}
func uuid() string {
	s := randomHex()
	return s[:8] + "-" + s[8:12] + "-4" + s[13:16] + "-8" + s[17:20] + "-" + s[20:32]
}

func docker(input string, args ...string) string {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("Docker falló en " + args[0] + " (verificar motor e imágenes requeridas)")
	}
	return strings.TrimSpace(string(out))
}

func port() string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	address := listener.Addr().String()
	must(listener.Close())
	return address
}
func envWith(extra map[string]string) []string {
	values := map[string]string{}
	for _, entry := range os.Environ() {
		parts := strings.SplitN(entry, "=", 2)
		if len(parts) == 2 {
			values[parts[0]] = parts[1]
		}
	}
	for key, value := range extra {
		values[key] = value
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		out = append(out, key+"="+value)
	}
	return out
}

func startBinary(directory, name, source string, env map[string]string) func() {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	binary := filepath.Join(directory, name+ext)
	build := exec.CommandContext(ctx, "go", "build", "-o", binary, source)
	build.Env = envWith(map[string]string{"CGO_ENABLED": "0"})
	if output, err := build.CombinedOutput(); err != nil {
		panic("Build de " + name + " falló: " + string(output))
	}
	log, err := os.Create(filepath.Join(directory, name+".log"))
	must(err)
	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = envWith(env)
	cmd.Stdout = log
	cmd.Stderr = log
	must(cmd.Start())
	return func() { _ = cmd.Process.Kill(); _ = cmd.Wait(); _ = log.Close() }
}

func request(base, method, path string, headers map[string]string, body any) (int, map[string]any) {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		must(err)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(data))
	must(err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	response, err := client.Do(req)
	must(err)
	defer response.Body.Close()
	out := map[string]any{}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	must(err)
	if len(raw) > 0 {
		must(json.Unmarshal(raw, &out))
	}
	return response.StatusCode, out
}

func eventually(label string, predicate func() bool) {
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	panic("Tiempo agotado: " + label)
}

func ready(base string) {
	eventually("servicio listo", func() bool {
		res, err := client.Get(base + "/health/ready")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		return res.StatusCode == 200
	})
}

func mail(base, email, previous string) (string, string) {
	var id, text string
	eventually("correo SMTP", func() bool {
		res, err := client.Get(base + "/api/v1/messages")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		var list struct {
			Messages []struct {
				ID string
				To []struct{ Address string }
			}
		}
		if json.NewDecoder(res.Body).Decode(&list) != nil {
			return false
		}
		for _, message := range list.Messages {
			if message.ID == previous {
				continue
			}
			for _, to := range message.To {
				if to.Address == email {
					res, err := client.Get(base + "/api/v1/message/" + message.ID)
					if err != nil {
						return false
					}
					defer res.Body.Close()
					var detail struct{ Text string }
					if json.NewDecoder(res.Body).Decode(&detail) != nil {
						return false
					}
					id, text = message.ID, detail.Text
					return true
				}
			}
		}
		return false
	})
	return id, text
}

func activation(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "http://") {
			u, err := url.Parse(line)
			must(err)
			values, err := url.ParseQuery(u.Fragment)
			must(err)
			if token := values.Get("activate"); token != "" {
				return token
			}
		}
	}
	panic("Correo sin enlace de activación")
}

func run() {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	directory, err := os.MkdirTemp("", "gym-cb03-")
	must(err)
	prefix := "gym-cb03-" + randomHex()[:12]
	postgresName, mailName := prefix+"-postgres", prefix+"-mailpit"
	for _, name := range []string{postgresName, mailName} {
		// Only these exact, uniquely generated container names may be removed.
		defer func(name string) {
			cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			_ = exec.CommandContext(cleanup, "docker", "rm", "-f", name).Run()
		}(name)
	}
	password := randomHex()
	docker("", "run", "-d", "--name", postgresName, "-e", "POSTGRES_PASSWORD="+password, "-p", "127.0.0.1::5432", "postgres:16-alpine")
	eventually("PostgreSQL", func() bool {
		return exec.CommandContext(ctx, "docker", "exec", postgresName, "pg_isready", "-U", "postgres").Run() == nil
	})
	pgAddress := docker("", "port", postgresName, "5432/tcp")
	sql := func(database, query string) string {
		return docker(query, "exec", "-i", postgresName, "psql", "-U", "postgres", "-d", database, "-X", "-v", "ON_ERROR_STOP=1", "-Atq")
	}
	sql("postgres", fmt.Sprintf("CREATE ROLE members LOGIN PASSWORD '%s'; CREATE ROLE benefits LOGIN PASSWORD '%s';", password, password))
	sql("postgres", "CREATE DATABASE members_db OWNER members;")
	sql("postgres", "CREATE DATABASE benefits_db OWNER benefits;")
	smtpAddress, mailAddress := port(), port()
	docker("", "run", "-d", "--name", mailName, "-p", smtpAddress+":1025", "-p", mailAddress+":8025", "axllent/mailpit:v1.20")
	mailURL := "http://" + mailAddress
	smtpHost, smtpPort, err := net.SplitHostPort(smtpAddress)
	must(err)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	private := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	must(err)
	public := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	membersAddress, benefitsAddress, gatewayAddress := port(), port(), port()
	membersURL, benefitsURL, gatewayURL := "http://"+membersAddress, "http://"+benefitsAddress, "http://"+gatewayAddress
	var dropNext atomic.Bool
	dropNext.Store(true)
	// Simulate a lost response AFTER the real Benefits commit. Worker must retry
	// the same durable event and recover its original result without a new credit.
	faultProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream, err := url.Parse(benefitsURL)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		req := r.Clone(r.Context())
		req.URL.Scheme = upstream.Scheme
		req.URL.Host = upstream.Host
		req.RequestURI = ""
		res, err := client.Do(req)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		if res.StatusCode == 200 && dropNext.Swap(false) {
			conn, _, err := w.(http.Hijacker).Hijack()
			if err == nil {
				_ = conn.Close()
			}
			return
		}
		for k, values := range res.Header {
			for _, value := range values {
				w.Header().Add(k, value)
			}
		}
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(body)
	}))
	defer faultProxy.Close()
	secret := randomHex()
	common := map[string]string{"LOG_LEVEL": "error", "GYM_INTERNAL_SERVICE_KEY": secret, "OTEL_SDK_DISABLED": "true", "OTEL_EXPORTER_OTLP_ENDPOINT": "", "MEMBERS_SMTP_USER": "", "MEMBERS_SMTP_PASSWORD": ""}
	membersEnv := map[string]string{}
	for k, v := range common {
		membersEnv[k] = v
	}
	for k, v := range map[string]string{"MEMBERS_DB_DSN": "postgres://members:" + password + "@" + pgAddress + "/members_db?sslmode=disable", "MEMBERS_HTTP_ADDR": membersAddress, "MEMBERS_IDENTITY_ENABLED": "true", "MEMBERS_ACTIVATION_ENCRYPTION_KEY": randomHex(), "MEMBERS_JWT_PRIVATE_KEY": private, "MEMBERS_PUBLIC_URL": gatewayURL, "MEMBERS_BENEFITS_URL": faultProxy.URL, "MEMBERS_SMTP_HOST": smtpHost, "MEMBERS_SMTP_PORT": smtpPort, "MEMBERS_SMTP_FROM": "fixture@example.test", "MEMBERS_SMTP_ALLOW_PLAINTEXT": "true"} {
		membersEnv[k] = v
	}
	stopMembers := startBinary(directory, "members", "./services/members-service/cmd/api", membersEnv)
	defer stopMembers()
	ready(membersURL)
	benefitsEnv := map[string]string{}
	for k, v := range common {
		benefitsEnv[k] = v
	}
	for k, v := range map[string]string{"BENEFITS_DB_DSN": "postgres://benefits:" + password + "@" + pgAddress + "/benefits_db?sslmode=disable", "BENEFITS_HTTP_ADDR": benefitsAddress, "BENEFITS_EXTERNAL_REGISTRATION_ENABLED": "true", "BENEFITS_MEMBERS_URL": membersURL} {
		benefitsEnv[k] = v
	}
	stopBenefits := startBinary(directory, "benefits", "./services/benefits-service/cmd/api", benefitsEnv)
	defer stopBenefits()
	ready(benefitsURL)
	gatewayEnv := map[string]string{}
	for k, v := range common {
		gatewayEnv[k] = v
	}
	for k, v := range map[string]string{"GATEWAY_HTTP_ADDR": gatewayAddress, "GATEWAY_JWT_PUBLIC_KEY": public, "GATEWAY_MEMBERS_URL": membersURL, "GATEWAY_BENEFITS_URL": benefitsURL} {
		gatewayEnv[k] = v
	}
	stopGateway := startBinary(directory, "gateway", "./gateway/cmd/gateway", gatewayEnv)
	defer stopGateway()
	eventually("gateway", func() bool {
		res, err := client.Get(gatewayURL + "/health/live")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		return res.StatusCode == 200
	})
	partner, other := uuid(), uuid()
	apiKey, otherKey := randomHex(), randomHex()
	hash, otherHash := sha256.Sum256([]byte(apiKey)), sha256.Sum256([]byte(otherKey))
	sql("benefits_db", fmt.Sprintf("INSERT INTO partners(id,nombre,api_key_hash,permite_registro,max_puntos_operacion,max_puntos_dia) VALUES('%s','Fixture A',decode('%x','hex'),true,500,2000),('%s','Fixture B',decode('%x','hex'),true,500,2000);", partner, hash, other, otherHash))
	mark("servicios_reales_y_bases_aisladas_listos")
	partnerHeaders := func(key string) map[string]string {
		return map[string]string{"X-API-Key": apiKey, "Idempotency-Key": key}
	}
	register := func(email, event, external string, points int) map[string]any {
		return map[string]any{"name": "Persona sintética", "email": email, "externalUserId": external, "eventId": event, "points": points, "reason": "Participación E2E", "reference": event}
	}
	waitRegistration := func(event string) {
		eventually("registro completado", func() bool {
			status, body := request(gatewayURL, "GET", "/partner-api/v2/external-registrations/"+event, map[string]string{"X-API-Key": apiKey}, nil)
			return status == 200 && body["status"] == "completed"
		})
	}
	activate := func(token, password string, expected int) {
		status, _ := request(gatewayURL, "POST", "/api/v1/auth/activate", nil, map[string]string{"token": token, "password": password})
		ensure(status == expected, "HTTP inesperado en activación")
	}
	login := func(email, password string, expected int) string {
		status, body := request(gatewayURL, "POST", "/api/v1/auth/login", nil, map[string]string{"email": email, "password": password})
		ensure(status == expected, "HTTP inesperado en login")
		if expected != 200 {
			return ""
		}
		ensure(body["expires_in"] == float64(900), "Duración JWT incorrecta")
		return body["access_token"].(string)
	}
	newBody := register("new@example.test", "new-001", "new-user", 150)
	status, receipt := request(gatewayURL, "POST", "/partner-api/v2/external-register", partnerHeaders("register-new-001"), newBody)
	ensure(status == 202, "No acepta registro nuevo")
	waitRegistration("new-001")
	_, text := mail(mailURL, "new@example.test", "")
	ensure(strings.Contains(text, "150 puntos"), "Bienvenida sin saldo")
	ensure(!dropNext.Load(), "No se inyectó la caída después del commit")
	ensure(sql("members_db", "SELECT intentos FROM registros_externos WHERE evento_id='new-001';") != "0", "No se recuperó el comando tras perder respuesta")
	ensure(sql("benefits_db", "SELECT count(*) FROM movimientos_puntos;") == "1", "Duplicó crédito después de commit")
	mark("alta_nueva_puntos_SMTP_y_recuperacion_despues_del_commit")
	passwordUser := "password-segura-123"
	activationToken := activation(text)
	activate(activationToken, passwordUser, 200)
	activate(activationToken, "password-otra-123", 401)
	userToken := login("new@example.test", passwordUser, 200)
	userHeaders := map[string]string{"Authorization": "Bearer " + userToken, "X-User-Id": uuid(), "X-User-Role": access.Admin}
	status, user := request(gatewayURL, "GET", "/api/v1/users/me", userHeaders, nil)
	ensure(status == 200 && user["external_registration"] == true && user["role"] == access.Student, "Identidad nueva incorrecta")
	status, balance := request(gatewayURL, "GET", "/api/v1/benefits/me", userHeaders, nil)
	ensure(status == 200 && balance["balance"] == float64(150), "Saldo propio incorrecto")
	ensure(sql("members_db", "SELECT count(*) FROM usuarios WHERE email='new@example.test';") == "1", "Usuario duplicado")
	mark("activacion_unica_login_JWT_y_saldo_propio_sin_membresia")
	status, _ = request(gatewayURL, "GET", "/api/v1/users", userHeaders, nil)
	ensure(status == 403, "Escalada de rol en gateway")
	status, _ = request(gatewayURL, "GET", "/api/v1/benefits/me", nil, nil)
	ensure(status == 401, "Saldo sin autenticación")
	signer, err := access.NewSigner(private)
	must(err)
	expired, err := signer.Issue(user["id"].(string), access.Student, time.Now().Add(-16*time.Minute))
	must(err)
	status, _ = request(gatewayURL, "GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer " + expired}, nil)
	ensure(status == 401, "JWT vencido aceptado")
	status, _ = request(gatewayURL, "GET", "/internal/v1/session", userHeaders, nil)
	ensure(status == 404, "Expone ruta interna")
	mark("roles_cabeceras_falsas_token_vencido_y_frontera_interna")
	status, replay := request(gatewayURL, "POST", "/partner-api/v2/external-register", partnerHeaders("register-new-002"), newBody)
	ensure(status == 202 && replay["registrationId"] == receipt["registrationId"], "No reutiliza recibo del evento")
	status, _ = request(gatewayURL, "POST", "/partner-api/v2/accounts/new-user/credits", partnerHeaders("credit-replay-001"), map[string]any{"eventId": "new-001", "points": 150, "reason": "Participación E2E", "reference": "new-001"})
	ensure(status == 201, "No recupera evento del registro")
	status, _ = request(gatewayURL, "GET", "/partner-api/v2/accounts/new-user/balance", map[string]string{"X-API-Key": apiKey}, nil)
	ensure(status == 200, "Saldo para integración autorizada")
	ensure(sql("benefits_db", "SELECT count(*) FROM movimientos_puntos;") == "1", "Evento duplicado entre endpoints")
	status, _ = request(gatewayURL, "GET", "/partner-api/v2/accounts/new-user/balance", map[string]string{"X-API-Key": otherKey}, nil)
	ensure(status == 404, "Otra integración consulta saldo ajeno")
	mark("evento_compartido_registro_creditos_e_integracion_aislada")
	existingID := uuid()
	oldHash, err := bcrypt.GenerateFromPassword([]byte(passwordUser), 10)
	must(err)
	sql("members_db", fmt.Sprintf("INSERT INTO usuarios(id,nombre,email,password_hash) VALUES('%s','Preexistente','existing@example.test',decode('%x','hex'));", existingID, oldHash))
	status, _ = request(gatewayURL, "POST", "/partner-api/v2/external-register", partnerHeaders("register-existing-001"), register("existing@example.test", "existing-001", "existing-user", 100))
	ensure(status == 202, "No acepta cuenta existente")
	_, verification := mail(mailURL, "existing@example.test", "")
	status, _ = request(gatewayURL, "GET", "/partner-api/v2/accounts/existing-user/balance", map[string]string{"X-API-Key": apiKey}, nil)
	ensure(status == 404, "Vinculó correo no confirmado")
	activate(activation(verification), "password-intento-reset", 200)
	waitRegistration("existing-001")
	existingToken := login("existing@example.test", passwordUser, 200)
	login("existing@example.test", "password-intento-reset", 401)
	status, existingUser := request(gatewayURL, "GET", "/api/v1/users/me", map[string]string{"Authorization": "Bearer " + existingToken}, nil)
	ensure(status == 200 && existingUser["id"] == existingID && existingUser["external_registration"] == false, "Alteró cuenta preexistente")
	hashText := sql("members_db", "SELECT encode(password_hash,'hex') FROM usuarios WHERE email='existing@example.test';")
	upgraded, err := hex.DecodeString(hashText)
	must(err)
	cost, err := bcrypt.Cost(upgraded)
	must(err)
	ensure(cost == 12, "No actualizó hash anterior")
	status, balance = request(gatewayURL, "GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer " + existingToken}, nil)
	ensure(status == 200 && balance["balance"] == float64(100), "Saldo preexistente incorrecto")
	mark("cuenta_existente_prueba_de_correo_sin_reset_y_bcrypt_12")
	docker("", "stop", mailName)
	status, _ = request(gatewayURL, "POST", "/partner-api/v2/external-register", partnerHeaders("register-smtp-001"), register("smtp@example.test", "smtp-001", "smtp-user", 150))
	ensure(status == 202, "No persiste con SMTP caído")
	waitRegistration("smtp-001")
	ensure(sql("members_db", "SELECT count(*) FROM correos_pendientes c JOIN registros_externos r ON r.id=c.registro_id WHERE r.evento_id='smtp-001' AND c.enviado_en IS NULL;") == "1", "No conservó correo pendiente")
	docker("", "start", mailName)
	_, smtpText := mail(mailURL, "smtp@example.test", "")
	ensure(strings.Contains(smtpText, "150 puntos"), "No recupera bienvenida")
	ensure(sql("benefits_db", "SELECT count(*) FROM eventos_acreditados WHERE evento_id='smtp-001';") == "1", "SMTP duplicó acreditación")
	mark("SMTP_caido_reintenta_sin_perder_ni_repetir_puntos")
	status, _ = request(gatewayURL, "POST", "/partner-api/v2/external-register", partnerHeaders("register-zero-001"), register("zero@example.test", "zero-001", "zero-user", 0))
	ensure(status == 202, "No acepta registro sin puntos")
	waitRegistration("zero-001")
	mailID, zeroText := mail(mailURL, "zero@example.test", "")
	zeroToken := activation(zeroText)
	sql("members_db", "UPDATE registros_externos SET token_expira=now()-interval '1 second' WHERE evento_id='zero-001';")
	activate(zeroToken, passwordUser, 401)
	status, _ = request(gatewayURL, "POST", "/api/v1/auth/resend-activation", nil, map[string]string{"email": "zero@example.test"})
	ensure(status == 202, "No acepta reenvío")
	_, renewedText := mail(mailURL, "zero@example.test", mailID)
	activate(zeroToken, passwordUser, 401)
	activate(activation(renewedText), passwordUser, 200)
	zeroJWT := login("zero@example.test", passwordUser, 200)
	status, balance = request(gatewayURL, "GET", "/api/v1/benefits/me", map[string]string{"Authorization": "Bearer " + zeroJWT}, nil)
	ensure(status == 200 && balance["balance"] == float64(0), "Cuenta sin puntos incorrecta")
	mark("registro_sin_puntos_enlace_vencido_reenvio_y_rotacion")
	status, _ = request(gatewayURL, "POST", "/api/v1/auth/logout", userHeaders, nil)
	ensure(status == 204, "Logout incorrecto")
	status, _ = request(gatewayURL, "GET", "/api/v1/benefits/me", userHeaders, nil)
	ensure(status == 200, "No respeta JWT sin revocación de ADR-006")
	mark("logout_sin_revocacion_y_vencimiento_limitado_por_ADR006")
	if *withMemberships {
		studentID := user["id"].(string)
		status, empty := request(gatewayURL, "GET", "/api/v1/memberships/me", userHeaders, nil)
		ensure(status == 200 && empty["activa"] == false && empty["total"] == float64(0), "Cuenta sin membresía no consultable")
		status, _ = request(gatewayURL, "POST", "/api/v1/memberships", userHeaders, map[string]any{})
		ensure(status == 403, "Alumno asigna membresías")
		status, _ = request(gatewayURL, "GET", "/api/v1/memberships/users/"+existingID, userHeaders, nil)
		ensure(status == 403, "Consulta membresías ajenas")
		mark("CB04_cuenta_sin_membresia_y_autorizacion_propia")
		adminID := uuid()
		sql("members_db", fmt.Sprintf("INSERT INTO usuarios(id,nombre,email,rol,password_hash) VALUES('%s','Admin fixture','admin@example.test','ADMINISTRADOR',decode('%x','hex'));", adminID, oldHash))
		adminJWT := login("admin@example.test", passwordUser, 200)
		adminHeaders := func(key string) map[string]string {
			return map[string]string{"Authorization": "Bearer " + adminJWT, "Idempotency-Key": key}
		}
		today := sql("members_db", `SELECT (now() AT TIME ZONE 'America/Argentina/Buenos_Aires')::date;`)
		calendar, err := time.Parse("2006-01-02", today)
		must(err)
		end := calendar.AddDate(0, 0, 29).Format("2006-01-02")
		status, _ = request(gatewayURL, "POST", "/partner-api/v2/accounts/new-user/credits", partnerHeaders("credit-preserve-300"), map[string]any{"eventId": "preserve-300", "points": 150, "reason": "Prueba conservación", "reference": "preserve-300"})
		ensure(status == 201, "No prepara saldo de 300")
		ledgerSQL := fmt.Sprintf("SELECT string_agg(m.id::text||':'||m.puntos::text,',' ORDER BY m.secuencia) FROM movimientos_puntos m JOIN cuentas c ON c.id=m.cuenta_id WHERE c.alumno_id='%s';", studentID)
		originalLedger := sql("benefits_db", ledgerSQL)
		checkPoints := func() {
			status, body := request(gatewayURL, "GET", "/api/v1/benefits/me", userHeaders, nil)
			ensure(status == 200 && body["balance"] == float64(300), "Membresía alteró saldo")
			ensure(sql("benefits_db", ledgerSQL) == originalLedger, "Membresía alteró movimientos")
		}
		oldMembership := uuid()
		sql("members_db", fmt.Sprintf("INSERT INTO membresias(id,usuario_id,tipo_codigo,tipo_snapshot,fecha_inicio,fecha_vencimiento,estado,motivo,modificada_por,actualizada_en) SELECT '%s','%s',codigo,to_jsonb(t)-ARRAY['modificada_por','motivo','actualizada_en'],CURRENT_DATE-40,CURRENT_DATE-1,'ACTIVA','Fixture histórica','%s',now() FROM tipos_membresia t WHERE codigo='MENSUAL';", oldMembership, studentID, adminID))
		eventually("vencimiento materializado", func() bool {
			return sql("members_db", fmt.Sprintf("SELECT estado FROM membresias WHERE id='%s';", oldMembership)) == "VENCIDA"
		})
		status, expiredPage := request(gatewayURL, "GET", "/api/v1/memberships/me", userHeaders, nil)
		ensure(status == 200 && expiredPage["activa"] == false, "Membresía vencida sigue vigente")
		checkPoints()
		mark("CB04_membresia_vencida_conserva_300_puntos_y_movimientos")
		renewBody := map[string]any{"tipo_codigo": "MENSUAL", "fecha_vencimiento": end, "motivo": "Retorno al gimnasio"}
		status, renewed := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-renew-001"), renewBody)
		ensure(status == 201, "No renueva al retornar")
		renewedMembership := renewed["membresia"].(map[string]any)
		ensure(renewedMembership["fecha_inicio"] == today && renewedMembership["activa"] == true, "Inicio de retorno incorrecto")
		status, replayed := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-renew-001"), renewBody)
		ensure(status == 201 && replayed["membresia"].(map[string]any)["id"] == renewedMembership["id"], "Renovación duplicada")
		checkPoints()
		mark("CB04_retorno_y_renovacion_idempotente_sin_perdida")
		status, _ = request(gatewayURL, "POST", "/api/v1/memberships", adminHeaders("membership-overlap-001"), map[string]any{"usuario_id": studentID, "tipo_codigo": "ANUAL", "fecha_inicio": today, "fecha_vencimiento": end, "motivo": "Prueba solapamiento"})
		ensure(status == 409, "No rechaza períodos superpuestos")
		status, _ = request(gatewayURL, "POST", "/api/v1/memberships/"+renewedMembership["id"].(string)+"/cancel", adminHeaders("membership-cancel-001"), map[string]any{"motivo": "Cambio de tipo solicitado"})
		ensure(status == 200, "No cancela para cambio de tipo")
		status, _ = request(gatewayURL, "PUT", "/api/v1/memberships/types/CLASES", adminHeaders("membership-type-001"), map[string]any{"nombre": "Clases de prueba", "duracion_dias": 30, "clases": 8, "habilitado": true, "motivo": "Condición sintética E2E"})
		ensure(status == 200, "No configura tipo por clases")
		status, classResult := request(gatewayURL, "POST", "/api/v1/memberships", adminHeaders("membership-class-001"), map[string]any{"usuario_id": studentID, "tipo_codigo": "CLASES", "fecha_inicio": today, "fecha_vencimiento": end, "motivo": "Cambio a clases"})
		ensure(status == 201, "No asigna membresía por clases")
		classMembership := classResult["membresia"].(map[string]any)
		ensure(classMembership["tipo"].(map[string]any)["clases"] == float64(8), "Cupo incorrecto")
		status, _ = request(gatewayURL, "PUT", "/api/v1/memberships/types/CLASES", adminHeaders("membership-type-002"), map[string]any{"nombre": "Nueva condición de prueba", "duracion_dias": 60, "clases": 12, "habilitado": true, "motivo": "Otra condición sintética"})
		ensure(status == 200, "No permite reconfigurar catálogo")
		status, page := request(gatewayURL, "GET", "/api/v1/memberships/me", userHeaders, nil)
		ensure(status == 200 && page["total"] == float64(3) && page["membresia_vigente"].(map[string]any)["tipo"].(map[string]any)["clases"] == float64(8), "Configuración reescribe membresías previas")
		checkPoints()
		mark("CB04_cambio_de_tipo_clases_snapshot_e_historial_preservados")
		status, future := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-annual-001"), map[string]any{"tipo_codigo": "ANUAL", "fecha_vencimiento": calendar.AddDate(0, 0, 400).Format("2006-01-02"), "motivo": "Renovación futura de prueba"})
		ensure(status == 201 && future["membresia"].(map[string]any)["condicion"] == "PROGRAMADA", "Renovación futura incorrecta")
		status, page = request(gatewayURL, "GET", "/api/v1/memberships/me?pagina=2&tamano=2", userHeaders, nil)
		ensure(status == 200 && page["total"] == float64(4) && len(page["items"].([]any)) == 2, "Historial no pagina correctamente")
		ensure(sql("members_db", fmt.Sprintf("SELECT count(*) FROM outbox_membresias o JOIN membresias m ON m.id=o.membresia_id WHERE m.usuario_id='%s';", studentID)) == "5", "Eventos de membresía perdidos o duplicados")
		checkPoints()
		mark("CB04_renovacion_programada_paginacion_y_outbox_atomico")
	}
	result.Passed = true
}

func main() {
	flag.Parse()
	if *withMemberships {
		result.Stage = "CB-04"
		result.Scope += "; membership expiry, return, type changes, ledger preservation and authorization"
	}
	failed := false
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				failed = true
				result.Checks = append(result.Checks, check{"ejecucion_completa", false})
				if message, ok := recovered.(string); ok {
					fmt.Fprintln(os.Stderr, message)
				}
			}
		}()
		run()
	}()
	result.Finished = time.Now().UTC().Format(time.RFC3339)
	file := "cb03-v2-e2e.json"
	if *withMemberships {
		file = "cb04-v2-e2e.json"
	}
	path := filepath.Join("docs", "evidence", "club", file)
	must(os.MkdirAll(filepath.Dir(path), 0755))
	data, err := json.MarshalIndent(result, "", "  ")
	must(err)
	must(os.WriteFile(path, append(data, '\n'), 0644))
	if failed {
		fmt.Fprintln(os.Stderr, result.Stage+" E2E falló; consultar el último escenario aprobado del informe. No se imprimen credenciales ni cuerpos.")
		os.Exit(1)
	}
	fmt.Printf("%s E2E: %d escenarios aprobados. Informe: %s\n", result.Stage, len(result.Checks), path)
}
