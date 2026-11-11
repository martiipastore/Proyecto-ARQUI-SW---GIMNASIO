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
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GuadiViti/Arquitectura-de-Software/pkg/access"
	"github.com/GuadiViti/Arquitectura-de-Software/pkg/messaging"
	amqp "github.com/rabbitmq/amqp091-go"
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
	// Metrics guarda mediciones (etapa 8: retraso escritura → búsqueda).
	Metrics map[string]float64 `json:"metrics,omitempty"`
}

var result = report{Stage: "CB-03", Started: time.Now().UTC().Format(time.RFC3339), Scope: "Local real HTTP services, partner API v2, isolated PostgreSQL 16, Mailpit v1.20 SMTP and injected lost callback response; no production or browser automation"}
var client = &http.Client{Timeout: 5 * time.Second}
var ctx context.Context
var withMemberships = flag.Bool("memberships", false, "agregar escenarios CB-04 reutilizando el flujo CB-03")
var withBooking = flag.Bool("booking", false, "verificar el recorrido de reservas de la etapa 4 con servicios reales")
var withMessaging = flag.Bool("messaging", false, "agregar RabbitMQ, MongoDB y notificaciones de la etapa 5")
var withAttendance = flag.Bool("attendance", false, "agregar consumo Booking → Benefits, historial y recuperación de etapa 6")
var withCache = flag.Bool("cache", false, "etapa 9: caché del catálogo de actividades en Redis, medida con y sin caché bajo la misma carga")
var withSearch = flag.Bool("search", false, "agregar búsqueda de clases en OpenSearch de etapa 8: indexer, reindexado, retraso, caída y revalidación")
var withClub = flag.Bool("club", false, "agregar administración del Club de etapa 7: integraciones, vinculación y reversiones")

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
	eventuallyWithin(label, 45*time.Second, predicate)
}

func eventuallyWithin(label string, limit time.Duration, predicate func() bool) {
	deadline := time.Now().Add(limit)
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
	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	directory, err := os.MkdirTemp("", "gym-cb03-")
	must(err)
	prefix := "gym-cb03-" + randomHex()[:12]
	postgresName, mailName := prefix+"-postgres", prefix+"-mailpit"
	for _, name := range []string{postgresName, mailName, prefix + "-rabbitmq", prefix + "-mongo", prefix + "-opensearch", prefix + "-redis"} {
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
		// El servidor temporal de initdb solo escucha Unix; esperar TCP evita
		// crear fixtures mientras el entrypoint todavía reinicia PostgreSQL.
		return exec.CommandContext(ctx, "docker", "exec", postgresName, "pg_isready", "-h", "127.0.0.1", "-U", "postgres").Run() == nil
	})
	pgAddress := docker("", "port", postgresName, "5432/tcp")
	sql := func(database, query string) string {
		return docker(query, "exec", "-i", postgresName, "psql", "-U", "postgres", "-d", database, "-X", "-v", "ON_ERROR_STOP=1", "-Atq")
	}
	sql("postgres", fmt.Sprintf("CREATE ROLE members LOGIN PASSWORD '%s'; CREATE ROLE benefits LOGIN PASSWORD '%s';", password, password))
	sql("postgres", "CREATE DATABASE members_db OWNER members;")
	sql("postgres", "CREATE DATABASE benefits_db OWNER benefits;")
	if *withBooking {
		sql("postgres", fmt.Sprintf("CREATE ROLE booking LOGIN PASSWORD '%s';", password))
		sql("postgres", "CREATE DATABASE booking_db OWNER booking;")
	}
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
	var rabbitURL, notificationURL string
	if *withMessaging {
		amqpAddress, managementAddress, mongoAddress := port(), port(), port()
		docker("", "run", "-d", "--name", prefix+"-rabbitmq", "-e", "RABBITMQ_DEFAULT_USER=fixture", "-e", "RABBITMQ_DEFAULT_PASS="+password, "-e", "RABBITMQ_SERVER_ADDITIONAL_ERL_ARGS=+S 2:2", "-p", amqpAddress+":5672", "-p", managementAddress+":15672", "rabbitmq:3-management")
		docker("", "run", "-d", "--name", prefix+"-mongo", "-p", mongoAddress+":27017", "mongo:7")
		rabbitURL = "http://" + managementAddress
		// Espera sin emitir credenciales ni cuerpos en el informe.
		eventuallyWithin("RabbitMQ management", 2*time.Minute, func() bool {
			r, _ := http.NewRequestWithContext(ctx, "GET", rabbitURL+"/api/overview", nil)
			r.SetBasicAuth("fixture", password)
			res, err := client.Do(r)
			if err != nil {
				return false
			}
			defer res.Body.Close()
			return res.StatusCode == 200
		})
		membersEnv["MEMBERS_MESSAGING_ENABLED"] = "true"
		membersEnv["LOG_LEVEL"] = "info"
		membersEnv["MEMBERS_AMQP_URL"] = "amqp://fixture:" + password + "@" + amqpAddress + "/"
		address := port()
		notificationURL = "http://" + address
		workerEnv := map[string]string{"NOTIFICATION_HTTP_ADDR": address, "NOTIFICATION_MONGO_URI": "mongodb://" + mongoAddress, "NOTIFICATION_MESSAGING_ENABLED": "true", "NOTIFICATION_AMQP_URL": membersEnv["MEMBERS_AMQP_URL"], "NOTIFICATION_SMTP_HOST": smtpHost, "NOTIFICATION_SMTP_PORT": smtpPort, "NOTIFICATION_SMTP_FROM": "fixture@example.test", "NOTIFICATION_SMTP_ALLOW_PLAINTEXT": "true", "LOG_LEVEL": "error", "OTEL_SDK_DISABLED": "true"}
		stopNotification := startBinary(directory, "notification", "./services/notification-worker/cmd/worker", workerEnv)
		defer stopNotification()
		ready(notificationURL)
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
	if *withAttendance {
		benefitsEnv["BENEFITS_MESSAGING_ENABLED"] = "true"
		benefitsEnv["BENEFITS_AMQP_URL"] = membersEnv["MEMBERS_AMQP_URL"]
	}
	stopBenefits := startBinary(directory, "benefits", "./services/benefits-service/cmd/api", benefitsEnv)
	defer func() { stopBenefits() }()
	resumeBenefits := func() {
		stopBenefits = startBinary(directory, "benefits", "./services/benefits-service/cmd/api", benefitsEnv)
		ready(benefitsURL)
	}
	ready(benefitsURL)
	gatewayEnv := map[string]string{}
	for k, v := range common {
		gatewayEnv[k] = v
	}
	for k, v := range map[string]string{"GATEWAY_HTTP_ADDR": gatewayAddress, "GATEWAY_JWT_PUBLIC_KEY": public, "GATEWAY_MEMBERS_URL": membersURL, "GATEWAY_BENEFITS_URL": benefitsURL} {
		gatewayEnv[k] = v
	}
	var search searchFixture
	if *withBooking {
		address := port()
		bookingEnv := map[string]string{}
		for k, v := range common {
			bookingEnv[k] = v
		}
		for k, v := range map[string]string{"BOOKING_HTTP_ADDR": address, "BOOKING_DB_DSN": "postgres://booking:" + password + "@" + pgAddress + "/booking_db?sslmode=disable", "BOOKING_ENABLED": "true", "BOOKING_MEMBERS_URL": membersURL, "BOOKING_CLOSE_INTERVAL": "1s"} {
			bookingEnv[k] = v
		}
		if *withCache {
			redisAddress := port()
			docker("", "run", "-d", "--name", prefix+"-redis", "-p", redisAddress+":6379", "redis:7-alpine", "redis-server", "--requirepass", password)
			eventually("Redis", func() bool {
				return exec.CommandContext(ctx, "docker", "exec", prefix+"-redis", "redis-cli", "-a", password, "--no-auth-warning", "ping").Run() == nil
			})
			bookingEnv["BOOKING_CACHE_ENABLED"] = "false"
			bookingEnv["BOOKING_REDIS_ADDR"] = redisAddress
			bookingEnv["BOOKING_REDIS_PASSWORD"] = password
		}
		if *withAttendance {
			bookingEnv["BOOKING_MESSAGING_ENABLED"] = "true"
			bookingEnv["BOOKING_AMQP_URL"] = membersEnv["MEMBERS_AMQP_URL"]
		}
		stopBooking := startBinary(directory, "booking", "./services/booking-service/cmd/api", bookingEnv)
		defer func() { stopBooking() }()
		ready("http://" + address)
		gatewayEnv["GATEWAY_BOOKING_URL"] = "http://" + address
		search = searchFixture{bookingURL: "http://" + address, prefix: prefix, directory: directory, amqpURL: bookingEnv["BOOKING_AMQP_URL"], bookingDSN: bookingEnv["BOOKING_DB_DSN"]}
		// Etapa 8: la búsqueda se activa recién en searchFlow, después de que las
		// etapas 4 a 7 se verificaron con la búsqueda en PostgreSQL.
		search.restartBooking = func(extra map[string]string) {
			stopBooking()
			for k, v := range extra {
				bookingEnv[k] = v
			}
			stopBooking = startBinary(directory, "booking", "./services/booking-service/cmd/api", bookingEnv)
			ready("http://" + address)
		}
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
		sql("members_db", fmt.Sprintf("INSERT INTO membresias(id,usuario_id,tipo_codigo,tipo_snapshot,fecha_inicio,fecha_vencimiento,estado,motivo,modificada_por,actualizada_en) SELECT '%s','%s',codigo,to_jsonb(t)-ARRAY['modificada_por','motivo','actualizada_en'],(now() AT TIME ZONE 'America/Argentina/Buenos_Aires')::date-40,(now() AT TIME ZONE 'America/Argentina/Buenos_Aires')::date-1,'ACTIVA','Fixture histórica','%s',now() FROM tipos_membresia t WHERE codigo='MENSUAL';", oldMembership, studentID, adminID))
		eventually("vencimiento materializado", func() bool {
			return sql("members_db", fmt.Sprintf("SELECT estado FROM membresias WHERE id='%s';", oldMembership)) == "VENCIDA"
		})
		status, expiredPage := request(gatewayURL, "GET", "/api/v1/memberships/me", userHeaders, nil)
		ensure(status == 200 && expiredPage["activa"] == false, "Membresía vencida sigue vigente")
		checkPoints()
		mark("CB04_membresia_vencida_conserva_300_puntos_y_movimientos")
		renewBody := map[string]any{"tipo_codigo": "MENSUAL", "motivo": "Retorno al gimnasio"}
		status, renewed := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-renew-001"), renewBody)
		ensure(status == 201, "No renueva al retornar")
		renewedMembership := renewed["membresia"].(map[string]any)
		ensure(renewedMembership["fecha_inicio"] == today && renewedMembership["activa"] == true, "Inicio de retorno incorrecto")
		status, replayed := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-renew-001"), renewBody)
		ensure(status == 201 && replayed["membresia"].(map[string]any)["id"] == renewedMembership["id"], "Renovación duplicada")
		checkPoints()
		mark("CB04_retorno_y_renovacion_idempotente_sin_perdida")
		status, _ = request(gatewayURL, "POST", "/api/v1/memberships", adminHeaders("membership-overlap-001"), map[string]any{"usuario_id": studentID, "tipo_codigo": "ANUAL", "fecha_inicio": today, "motivo": "Prueba solapamiento"})
		ensure(status == 409, "No rechaza períodos superpuestos")
		status, _ = request(gatewayURL, "POST", "/api/v1/memberships/"+renewedMembership["id"].(string)+"/cancel", adminHeaders("membership-cancel-001"), map[string]any{"motivo": "Cambio de tipo solicitado"})
		ensure(status == 200, "No cancela para cambio de tipo")
		status, _ = request(gatewayURL, "PUT", "/api/v1/memberships/types/CLASES", adminHeaders("membership-type-001"), map[string]any{"nombre": "Clases de prueba", "duracion_dias": 30, "clases": 8, "habilitado": true, "motivo": "Condición sintética E2E"})
		ensure(status == 200, "No configura tipo por clases")
		status, classResult := request(gatewayURL, "POST", "/api/v1/memberships", adminHeaders("membership-class-001"), map[string]any{"usuario_id": studentID, "tipo_codigo": "CLASES", "fecha_inicio": today, "motivo": "Cambio a clases"})
		ensure(status == 201, "No asigna membresía por clases")
		classMembership := classResult["membresia"].(map[string]any)
		ensure(classMembership["tipo"].(map[string]any)["clases"] == float64(8), "Cupo incorrecto")
		status, _ = request(gatewayURL, "PUT", "/api/v1/memberships/types/CLASES", adminHeaders("membership-type-002"), map[string]any{"nombre": "Nueva condición de prueba", "duracion_dias": 60, "clases": 12, "habilitado": true, "motivo": "Otra condición sintética"})
		ensure(status == 200, "No permite reconfigurar catálogo")
		status, page := request(gatewayURL, "GET", "/api/v1/memberships/me", userHeaders, nil)
		ensure(status == 200 && page["total"] == float64(3) && page["membresia_vigente"].(map[string]any)["tipo"].(map[string]any)["clases"] == float64(8), "Configuración reescribe membresías previas")
		checkPoints()
		mark("CB04_cambio_de_tipo_clases_snapshot_e_historial_preservados")
		status, future := request(gatewayURL, "POST", "/api/v1/memberships/users/"+studentID+"/renew", adminHeaders("membership-annual-001"), map[string]any{"tipo_codigo": "ANUAL", "motivo": "Renovación futura de prueba"})
		ensure(status == 201 && future["membresia"].(map[string]any)["condicion"] == "PROGRAMADA", "Renovación futura incorrecta")
		status, page = request(gatewayURL, "GET", "/api/v1/memberships/me?pagina=2&tamano=2", userHeaders, nil)
		ensure(status == 200 && page["total"] == float64(4) && len(page["items"].([]any)) == 2, "Historial no pagina correctamente")
		ensure(sql("members_db", fmt.Sprintf("SELECT count(*) FROM outbox_membresias o JOIN membresias m ON m.id=o.membresia_id WHERE m.usuario_id='%s';", studentID)) == "5", "Eventos de membresía perdidos o duplicados")
		checkPoints()
		mark("CB04_renovacion_programada_paginacion_y_outbox_atomico")
		if *withBooking {
			bookingFlow(gatewayURL, mailURL, sql, adminHeaders, login, passwordUser, today)
		}
		if *withCache {
			cacheFlow(gatewayURL, passwordUser, search, sql, adminHeaders, login)
		}
		if *withMessaging {
			messagingFlow(gatewayURL, mailURL, rabbitURL, notificationURL, password, prefix, mailName, sql, adminHeaders)
		}
		if *withAttendance {
			attendanceFlow(gatewayURL, benefitsURL, mailURL, membersEnv["MEMBERS_AMQP_URL"], passwordUser, today, sql, adminHeaders, login, func() { stopBenefits() }, resumeBenefits)
		}
		if *withClub {
			clubFlow(gatewayURL, benefitsURL, passwordUser, sql, adminHeaders, login)
		}
		if *withSearch {
			searchFlow(gatewayURL, passwordUser, search, sql, adminHeaders, login)
		}
	}
	result.Passed = true
}

// bookingFlow cubre HU-06/07/13/14/15/21/22, RN-12/13/15/17/21/38.
// Solo usa SQL para inspeccionar resultados y preparar fechas históricas: todas
// las operaciones del usuario atraviesan el gateway y los servicios reales.
func bookingFlow(base, mailURL string, sql func(string, string) string, admin func(string) map[string]string, login func(string, string, int) string, password, today string) {
	create := func(role, email, dni string) (string, string) {
		status, person := request(base, "POST", "/api/v1/users", admin(""), map[string]string{"rol": role, "nombre": "Persona", "apellido": "Booking", "dni": dni, "email": email, "fecha_nacimiento": "2000-01-01"})
		ensure(status == 201, "No crea persona para Booking")
		_, text := mail(mailURL, email, "")
		status, _ = request(base, "POST", "/api/v1/auth/activate", nil, map[string]string{"token": activation(text), "password": password})
		ensure(status == 200, "No activa persona de Booking")
		return person["id"].(string), login(email, password, 200)
	}
	professor, professorJWT := create("PROFESOR", "booking-prof@example.test", "30111222")
	student, studentJWT := create("ALUMNO", "booking-student@example.test", "40111222")
	headers := func(key string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + studentJWT, "Idempotency-Key": key}
	}
	status, _ := request(base, "POST", "/api/v1/memberships", admin("booking-membership-001"), map[string]string{"usuario_id": student, "tipo_codigo": "MENSUAL", "fecha_inicio": today, "motivo": "Recorrido Booking E2E"})
	ensure(status == 201, "No asigna membresía para Booking")
	mark("booking_alta_activacion_login_y_membresia_reales")
	status, catalog := request(base, "GET", "/api/v1/activities", headers(""), nil)
	ensure(status == 200, "No consulta actividades")
	activity := ""
	for _, item := range catalog["items"].([]any) {
		a := item.(map[string]any)
		if a["nombre"] == "Zumba" {
			activity = a["id"].(string)
		}
	}
	ensure(activity != "", "Falta Zumba")
	// Horario de mañana: no depende de la hora a la que se ejecute la prueba.
	zone, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	must(err)
	tomorrow := time.Now().In(zone).AddDate(0, 0, 1)
	weekday := int(tomorrow.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	status, _ = request(base, "POST", "/api/v1/activities/"+activity+"/schedules", admin(""), map[string]any{"dia_semana": weekday, "inicio": "10:00", "duracion_minutos": 60, "profesor_id": professor, "capacidad": 2})
	ensure(status == 201, "No crea horario con profesor real")
	date := tomorrow.Format(time.DateOnly)
	status, generated := request(base, "POST", "/api/v1/classes/generate", admin(""), map[string]string{"desde": date, "hasta": date})
	ensure(status == 200 && generated["creadas"] == float64(1), "No genera clase")
	status, generated = request(base, "POST", "/api/v1/classes/generate", admin(""), map[string]string{"desde": date, "hasta": date})
	ensure(status == 200 && generated["creadas"] == float64(0), "Generación duplica clases")
	status, classes := request(base, "GET", "/api/v1/classes?fecha_desde="+date+"&fecha_hasta="+date, headers(""), nil)
	ensure(status == 200 && classes["total"] == float64(1), "Búsqueda no encuentra clase")
	classID := classes["items"].([]any)[0].(map[string]any)["id"].(string)
	mark("booking_catalogo_horario_generacion_idempotente_y_busqueda")
	status, reserved := request(base, "POST", "/api/v1/bookings", headers("booking-reserve-001"), map[string]string{"clase_id": classID})
	ensure(status == 201, fmt.Sprintf("No reserva clase: HTTP %d, code %v", status, reserved["code"]))
	reservation := reserved["reserva"].(map[string]any)["id"].(string)
	status, replay := request(base, "POST", "/api/v1/bookings", headers("booking-reserve-001"), map[string]string{"clase_id": classID})
	ensure(status == 201 && replay["reserva"].(map[string]any)["id"] == reservation, "Reserva no es idempotente")
	status, duplicate := request(base, "POST", "/api/v1/bookings", headers("booking-reserve-002"), map[string]string{"clase_id": classID})
	ensure(status == 409 && duplicate["code"] == "RESERVA_DUPLICADA", "No rechaza duplicado")
	status, _ = request(base, "POST", "/api/v1/bookings/"+reservation+"/cancel", headers("booking-cancel-001"), nil)
	ensure(status == 200, "No cancela reserva futura")
	status, detail := request(base, "GET", "/api/v1/classes/"+classID, headers(""), nil)
	ensure(status == 200 && detail["clase"].(map[string]any)["ocupacion"] == float64(0), "Cancelar no libera cupo")
	status, _ = request(base, "POST", "/api/v1/bookings", headers("booking-reserve-003"), map[string]string{"clase_id": classID})
	ensure(status == 201, "No puede reservar otra vez tras cancelar")
	mark("booking_reserva_idempotencia_duplicado_cancelacion_y_cupo")
	status, _ = request(base, "GET", "/api/v1/classes", nil, nil)
	ensure(status == 401, "Booking permite acceso anónimo")
	status, _ = request(base, "POST", "/api/v1/check-ins", headers("booking-forbidden-001"), map[string]string{"dni": "40111222"})
	ensure(status == 403, "Alumno puede operar recepción")
	status, mine := request(base, "GET", "/api/v1/classes/mine", map[string]string{"Authorization": "Bearer " + professorJWT}, nil)
	ensure(status == 200 && len(mine["items"].([]any)) == 1, "Profesor no ve su clase")
	mark("booking_autorizacion_gateway_y_clases_del_profesor")
	// Adelantar la fixture permite probar ingreso/cierre sin esperar hasta mañana.
	sql("booking_db", fmt.Sprintf("UPDATE clases SET inicio=now()+interval '5 minutes',fin=now()+interval '65 minutes' WHERE id='%s'; UPDATE reservas SET inicio=(SELECT inicio FROM clases WHERE id=clase_id),fin=(SELECT fin FROM clases WHERE id=clase_id) WHERE clase_id='%s';", classID, classID))
	status, denied := request(base, "POST", "/api/v1/bookings/"+sql("booking_db", fmt.Sprintf("SELECT id FROM reservas WHERE clase_id='%s' AND estado='CONFIRMADA';", classID))+"/cancel", headers("booking-too-late-001"), nil)
	ensure(status == 422 && denied["code"] == "PLAZO_CANCELACION_VENCIDO", "Permite cancelar fuera de plazo")
	status, entered := request(base, "POST", "/api/v1/check-ins", admin("booking-checkin-001"), map[string]string{"dni": "40111222"})
	ensure(status == 201 && entered["resultado"] == "VERDE" && entered["puntos"] == float64(500), "No registra ingreso")
	status, entered = request(base, "POST", "/api/v1/check-ins", admin("booking-checkin-002"), map[string]string{"dni": "40111222"})
	ensure(status == 200 && entered["puntos"] == float64(0), "Reingreso duplica puntos")
	ensure(sql("booking_db", "SELECT count(*) FROM asistencias;") == "1", "Asistencia duplicada")
	ensure(sql("booking_db", "SELECT count(*) FROM outbox WHERE event_type='asistencia.registrada';") == "1", "Evento de asistencia duplicado")
	status, _ = request(base, "POST", "/api/v1/check-ins", admin("booking-unknown-001"), map[string]string{"dni": "99999999"})
	ensure(status == 404, "DNI inexistente no devuelve 404")
	mark("booking_plazo_ingreso_reingreso_y_evento_unico")
	// Una reserva histórica sintética prueba el cierre automático del proceso real.
	sql("booking_db", fmt.Sprintf("WITH historica AS (INSERT INTO clases(actividad_id,inicio,fin,profesor_id,profesor_nombre,capacidad,ocupacion) VALUES('%s',now()-interval '2 hours',now()-interval '1 hour','%s','Fixture',2,1) RETURNING id,inicio,fin) INSERT INTO reservas(clase_id,alumno_id,estado,inicio,fin,creada_en,actualizada_en) SELECT id,'%s','CONFIRMADA',inicio,fin,now(),now() FROM historica;", activity, professor, student))
	eventually("cierre automático y ausencia", func() bool {
		return sql("booking_db", "SELECT count(*) FROM outbox WHERE event_type='inasistencia.registrada';") == "1"
	})
	status, history := request(base, "GET", "/api/v1/bookings/me?estado=AUSENTE", headers(""), nil)
	ensure(status == 200 && history["total"] == float64(1), "Ausencia no aparece en historial")
	mark("booking_cierre_automatico_ausencia_y_outbox")
}

// RN-05/ADR-005: dependencias reales y fallas aisladas; no usa el stack habitual.
func messagingFlow(base, mailURL, rabbitURL, workerURL, password, prefix, mailName string, sql func(string, string) string, admin func(string) map[string]string) {
	mongo := func(js string) string {
		return docker("", "exec", prefix+"-mongo", "mongosh", "--quiet", "notifications_db", "--eval", js)
	}
	mq := func(method, path string, body any) map[string]any {
		var encoded []byte
		if body != nil {
			encoded, _ = json.Marshal(body)
		}
		r, err := http.NewRequestWithContext(ctx, method, rabbitURL+path, bytes.NewReader(encoded))
		must(err)
		r.SetBasicAuth("fixture", password)
		r.Header.Set("Content-Type", "application/json")
		res, err := client.Do(r)
		must(err)
		defer res.Body.Close()
		ensure(res.StatusCode >= 200 && res.StatusCode < 300, "RabbitMQ management no responde")
		var v map[string]any
		must(json.NewDecoder(res.Body).Decode(&v))
		return v
	}
	student := sql("members_db", "SELECT id FROM usuarios WHERE email='booking-student@example.test';")
	processed := func() int64 {
		res, err := client.Get(workerURL + "/metrics")
		must(err)
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		must(err)
		var count int64
		_, err = fmt.Sscanf(string(raw), "notification_processed_total %d", &count)
		must(err)
		return count
	}
	eventID := sql("members_db", fmt.Sprintf("SELECT o.event_id FROM outbox_membresias o JOIN membresias m ON m.id=o.membresia_id WHERE m.usuario_id='%s' AND o.event_type='membresia.activada' ORDER BY o.occurred_at DESC LIMIT 1;", student))
	sent := func(id string) bool {
		return mongo("db.notifications.countDocuments({_id:'"+id+"',estado:'ENVIADA'})") == "1"
	}
	eventually("notificación durable", func() bool { return sent(eventID) })
	_, text := mail(mailURL, "booking-student@example.test", "")
	ensure(strings.Contains(text, "membresía"), "Correo de membresía no recibido")
	ensure(sql("members_db", fmt.Sprintf("SELECT publicado_en IS NOT NULL FROM outbox_membresias WHERE event_id='%s';", eventID)) == "t", "Evento enviado sin marcar outbox")
	mark("mensajeria_outbox_confirmado_mongo_y_SMTP_reales")
	body := sql("members_db", fmt.Sprintf("SELECT jsonb_build_object('event_id',event_id,'event_type',event_type,'event_version',event_version,'occurred_at',occurred_at,'data',data)::text FROM outbox_membresias WHERE event_id='%s';", eventID))
	// Se conserva la representación del relay para la huella durable del consumidor.
	var envelope struct {
		ID       string          `json:"event_id"`
		Type     string          `json:"event_type"`
		Version  int             `json:"event_version"`
		Occurred time.Time       `json:"occurred_at"`
		Data     json.RawMessage `json:"data"`
	}
	must(json.Unmarshal([]byte(body), &envelope))
	canonical, err := json.Marshal(envelope)
	must(err)
	// PostgreSQL jsonb puede ordenar data diferente: deduplicar semánticamente.
	publish := func(body string) {
		v := mq("POST", "/api/exchanges/%2F/gym.events/publish", map[string]any{"properties": map[string]any{"delivery_mode": 2, "content_type": "application/json"}, "routing_key": "membresia.activada", "payload": body, "payload_encoding": "string"})
		ensure(v["routed"] == true, "Mensaje sin ruta")
	}
	beforeReplay := processed()
	publish(string(canonical))
	eventually("replay consumido", func() bool { return processed() > beforeReplay })
	ensure(mongo("db.notifications.findOne({_id:'"+eventID+"'}).intentos") == "1", "Replay repitió efecto lógico")
	mark("mensajeria_reentrega_no_duplica_notificacion_logica")
	// La escritura se confirma aunque no haya broker; al volver se publica.
	docker("", "stop", prefix+"-rabbitmq")
	status, renewed := request(base, "POST", "/api/v1/memberships/users/"+student+"/renew", admin("messaging-broker-outage-001"), map[string]string{"tipo_codigo": "MENSUAL", "motivo": "Prueba durable sin broker"})
	ensure(status == 201, "RabbitMQ caído revierte membresía")
	membership := renewed["membresia"].(map[string]any)["id"].(string)
	id := sql("members_db", fmt.Sprintf("SELECT event_id FROM outbox_membresias WHERE membresia_id='%s' AND event_type='membresia.activada';", membership))
	ensure(sql("members_db", fmt.Sprintf("SELECT publicado_en IS NULL FROM outbox_membresias WHERE event_id='%s';", id)) == "t", "Outbox perdido con broker caído")
	docker("", "start", prefix+"-rabbitmq")
	eventuallyWithin("relay recupera broker", 2*time.Minute, func() bool { return sent(id) })
	mark("mensajeria_caida_broker_conserva_membresia_y_recupera_outbox")
	docker("", "stop", mailName)
	status, renewed = request(base, "POST", "/api/v1/memberships/users/"+student+"/renew", admin("messaging-smtp-outage-001"), map[string]string{"tipo_codigo": "MENSUAL", "motivo": "Prueba durable sin SMTP"})
	ensure(status == 201, "SMTP caído revierte membresía")
	membership = renewed["membresia"].(map[string]any)["id"].(string)
	id = sql("members_db", fmt.Sprintf("SELECT event_id FROM outbox_membresias WHERE membresia_id='%s' AND event_type='membresia.activada';", membership))
	eventually("reintento programado", func() bool {
		return mongo("db.notifications.countDocuments({_id:'"+id+"',estado:'PENDIENTE',intentos:{$gte:1}})") == "1"
	})
	docker("", "start", mailName)
	eventually("SMTP recuperado", func() bool { return sent(id) })
	ensure(mongo("db.notifications.findOne({_id:'"+id+"'}).intentos") != "1", "No se registró el reintento")
	mark("mensajeria_SMTP_caido_no_revierte_y_reintenta_con_estado_durable")
	publish(`{"event_type":"desconocido","event_version":99}`)
	eventually("mensaje inválido en DLQ", func() bool {
		v := mq("GET", "/api/queues/%2F/notification.membresia-activada.dlq", nil)
		return v["messages"] == float64(1)
	})
	ensure(mongo("db.failed_messages.countDocuments({estado:'DLQ'})") == "1", "DLQ sin diagnóstico durable")
	mark("mensajeria_mensaje_invalido_DLQ")
	// Fixture del cuarto intento: evita esperar 10 minutos para cubrir el límite.
	docker("", "stop", mailName)
	envelope.ID = uuid()
	terminal, err := json.Marshal(envelope)
	must(err)
	v := mq("POST", "/api/exchanges/%2F/gym.events/publish", map[string]any{"properties": map[string]any{"delivery_mode": 2, "content_type": "application/json", "headers": map[string]any{"x-attempt": 3}}, "routing_key": "membresia.activada", "payload": string(terminal), "payload_encoding": "string"})
	ensure(v["routed"] == true, "Intento terminal sin ruta")
	eventually("cuarto error en DLQ", func() bool {
		v := mq("GET", "/api/queues/%2F/notification.membresia-activada.dlq", nil)
		return v["messages"] == float64(2)
	})
	ensure(mongo("db.failed_messages.countDocuments({estado:'DLQ',intentos:4})") == "1", "Se excedió el límite de intentos")
	docker("", "start", mailName)
	status, health := request(workerURL, "GET", "/health/ready", nil, nil)
	ensure(status == 200 && health["status"] == "ready", "Worker no recuperó readiness")
	mark("mensajeria_cuarto_error_DLQ_y_readiness_recuperada")
}

// HU-19/22, RN-20/22/38/42: recorrido real y caída del consumidor. SQL solo
// inspecciona evidencia y prepara fechas de la fixture aislada de Booking.
func attendanceFlow(base, benefitsURL, mailURL, amqpURL, password, today string, sql func(string, string) string, admin func(string) map[string]string, login func(string, string, int) string, pause, resume func()) {
	student := sql("members_db", "SELECT id FROM usuarios WHERE email='booking-student@example.test';")
	jwt := login("booking-student@example.test", password, 200)
	headers := func(token string) map[string]string { return map[string]string{"Authorization": "Bearer " + token} }
	receipts := func(user string) string {
		return sql("benefits_db", fmt.Sprintf("SELECT count(*) FROM mensajes_booking_procesados p JOIN movimientos_puntos m ON m.id=p.movimiento_id JOIN cuentas c ON c.id=m.cuenta_id WHERE c.alumno_id='%s';", user))
	}
	countMoves := func(user string) string {
		return sql("benefits_db", fmt.Sprintf("SELECT count(*) FROM movimientos_puntos m JOIN cuentas c ON c.id=m.cuenta_id WHERE c.alumno_id='%s';", user))
	}
	metrics := func() int64 {
		res, err := client.Get(benefitsURL + "/metrics")
		must(err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		must(err)
		var count int64
		_, err = fmt.Sscanf(string(body), "booking_events_processed_total %d", &count)
		must(err)
		return count
	}
	assertAccount := func(user, token string) {
		balance := sql("benefits_db", fmt.Sprintf("SELECT saldo FROM cuentas WHERE alumno_id='%s';", user))
		ensure(balance == "400" || balance == "500", "Saldo no respeta orden de aplicación")
		ensure(sql("benefits_db", fmt.Sprintf("SELECT c.saldo=COALESCE(sum(m.puntos),0) AND c.ultima_secuencia=count(m.id) AND max(m.secuencia)=count(m.id) FROM cuentas c JOIN movimientos_puntos m ON m.cuenta_id=c.id WHERE c.alumno_id='%s' GROUP BY c.id;", user)) == "t", "Ledger no explica saldo y secuencia")
		status, out := request(base, "GET", "/api/v1/benefits/me", headers(token), nil)
		ensure(status == 200 && fmt.Sprint(out["balance"]) == balance, "Saldo propio no refleja los eventos")
		status, page := request(base, "GET", "/api/v1/benefits/me/movements?pagina=1&tamano=20", headers(token), nil)
		ensure(status == 200 && page["total"] == float64(2), "Historial propio no muestra ambos hechos")
		items := page["items"].([]any)
		ensure(items[0].(map[string]any)["secuencia"] == float64(2), "Historial no ordenado por aplicación")
		kinds := map[string]bool{}
		for _, item := range items {
			kinds[item.(map[string]any)["origen"].(string)] = true
		}
		ensure(kinds["ASISTENCIA"] && kinds["INASISTENCIA"], "Historial pierde los orígenes")
	}
	eventually("dos hechos iniciales consumidos", func() bool { return receipts(student) == "2" })
	assertAccount(student, jwt)
	mark("puntos_asistencia_y_ausencia_en_ledger_saldo_y_historial_propios")
	status, _ := request(base, "GET", "/api/v1/benefits/me/movements", nil, nil)
	ensure(status == 401, "Historial anónimo permitido")
	status, _ = request(base, "GET", "/api/v1/benefits/me/movements?usuario_id="+uuid(), headers(jwt), nil)
	ensure(status == 400, "Historial permite elegir identidad por query")
	forged := headers(jwt)
	forged["X-User-Id"] = uuid()
	forged["X-User-Role"] = "ADMINISTRADOR"
	status, page := request(base, "GET", "/api/v1/benefits/me/movements?tamano=1&pagina=2", forged, nil)
	ensure(status == 200 && page["total"] == float64(2) && len(page["items"].([]any)) == 1, "Paginación o frontera de identidad falló")
	mark("puntos_historial_autenticado_identidad_no_manipulable_y_paginacion")
	before := metrics()
	sql("booking_db", "UPDATE outbox SET publicado_en=NULL WHERE event_type<>'clase.actualizada';")
	eventually("reentregas confirmadas por consumidor", func() bool { return metrics() >= before+2 })
	ensure(countMoves(student) == "2" && receipts(student) == "2", "Replay agregó puntos o secuencias")
	bus, err := messaging.Dial(ctx, amqpURL)
	must(err)
	defer bus.Close()
	body := sql("booking_db", "SELECT jsonb_build_object('event_id',event_id,'event_type',event_type,'event_version',event_version,'occurred_at',occurred_at,'producer','booking-service','data',data)::text FROM outbox WHERE event_type='asistencia.registrada' LIMIT 1;")
	var alias map[string]any
	must(json.Unmarshal([]byte(body), &alias))
	alias["event_id"] = uuid()
	encoded, err := json.Marshal(alias)
	must(err)
	before = metrics()
	must(bus.Publish(ctx, "gym.events", "asistencia.registrada", amqp.Publishing{ContentType: "application/json", Body: encoded}))
	eventually("mismo origen con nuevo event_id", func() bool { return metrics() > before })
	ensure(countMoves(student) == "2" && receipts(student) == "3", "Nuevo event_id duplicó el origen")
	mark("puntos_reentrega_y_nuevo_event_id_no_duplican_hecho_ni_secuencia")
	// Usuario real creado antes de la caída; el ingreso sigue funcionando con
	// Benefits apagado. No se inserta directamente un movimiento de puntos.
	status, person := request(base, "POST", "/api/v1/users", admin(""), map[string]string{"rol": "ALUMNO", "nombre": "Persona", "apellido": "Recuperación", "dni": "40111333", "email": "recovery-student@example.test", "fecha_nacimiento": "2000-01-01"})
	ensure(status == 201, "No crea alumno de recuperación")
	user := person["id"].(string)
	_, text := mail(mailURL, "recovery-student@example.test", "")
	status, _ = request(base, "POST", "/api/v1/auth/activate", nil, map[string]string{"token": activation(text), "password": password})
	ensure(status == 200, "No activa alumno de recuperación")
	secondJWT := login("recovery-student@example.test", password, 200)
	status, _ = request(base, "POST", "/api/v1/memberships", admin("points-membership-001"), map[string]string{"usuario_id": user, "tipo_codigo": "MENSUAL", "fecha_inicio": today, "motivo": "Recuperación con Benefits caído"})
	ensure(status == 201, "No asigna membresía de recuperación")
	classID := sql("booking_db", fmt.Sprintf("SELECT clase_id FROM reservas WHERE alumno_id='%s' AND estado='ASISTIDA';", student))
	pause()
	secondHeaders := headers(secondJWT)
	secondHeaders["Idempotency-Key"] = "points-reserve-001"
	status, _ = request(base, "POST", "/api/v1/bookings", secondHeaders, map[string]string{"clase_id": classID})
	ensure(status == 201, "Benefits caído impide reservar")
	status, _ = request(base, "POST", "/api/v1/check-ins", admin("points-checkin-001"), map[string]string{"dni": "40111333"})
	ensure(status == 201, "Benefits caído impide registrar ingreso")
	activity := sql("booking_db", "SELECT id FROM actividades WHERE nombre='Zumba';")
	professor := sql("booking_db", "SELECT profesor_id FROM clases WHERE id='"+classID+"';")
	sql("booking_db", fmt.Sprintf("WITH c AS (INSERT INTO clases(actividad_id,inicio,fin,profesor_id,profesor_nombre,capacidad,ocupacion) VALUES('%s',now()-interval '4 hours',now()-interval '3 hours','%s','Fixture',2,1) RETURNING id,inicio,fin) INSERT INTO reservas(clase_id,alumno_id,estado,inicio,fin,creada_en,actualizada_en) SELECT id,'%s','CONFIRMADA',inicio,fin,now(),now() FROM c;", activity, professor, user))
	eventually("outbox publicado sin consumidor", func() bool {
		return sql("booking_db", "SELECT count(*) FROM outbox WHERE publicado_en IS NOT NULL AND event_type<>'clase.actualizada';") == "4"
	})
	ensure(countMoves(user) == "0", "Consumidor detenido produjo puntos")
	mark("puntos_caida_Benefits_no_bloquea_reserva_ingreso_ni_cierre")
	resume()
	eventually("Benefits recupera mensajes durables", func() bool { return receipts(user) == "2" })
	assertAccount(user, secondJWT)
	before = metrics()
	sql("booking_db", "UPDATE outbox SET publicado_en=NULL WHERE event_type<>'clase.actualizada';")
	eventually("replay tras reiniciar Benefits", func() bool { return metrics() >= before+4 })
	ensure(countMoves(user) == "2" && countMoves(student) == "2", "Reinicio perdió la deduplicación")
	mark("puntos_recuperacion_y_replay_tras_reinicio_sin_perdida_ni_duplicados")
	alias["event_id"] = uuid()
	alias["data"].(map[string]any)["puntos"] = 2000
	invalid, err := json.Marshal(alias)
	must(err)
	must(bus.Publish(ctx, "gym.events", "asistencia.registrada", amqp.Publishing{ContentType: "application/json", Body: invalid}))
	eventually("importe externo no permitido en DLQ", func() bool {
		q, err := bus.Channel.QueueInspect("benefits.asistencia-registrada.dlq")
		return err == nil && q.Messages == 1
	})
	ensure(countMoves(student) == "2", "Evento inválido modificó puntos")
	mark("puntos_evento_invalido_en_DLQ_sin_modificar_ledger")
}

// clubFlow cubre la etapa 7: HU-10 (CA-10.1 a 10.4), HU-11 (CA-11.1 a 11.3),
// CA-35.3 y HU-CB-07. Todas las operaciones pasan por el gateway con JWT real;
// SQL solo inspecciona resultados. Catálogo, canjes y débitos quedan fuera (D-76).
func clubFlow(base, benefitsURL, password string, sql func(string, string) string, admin func(string) map[string]string, login func(string, string, int) string) {
	student := sql("members_db", "SELECT id FROM usuarios WHERE email='booking-student@example.test';")
	other := sql("members_db", "SELECT id FROM usuarios WHERE email='recovery-student@example.test';")
	studentJWT := login("booking-student@example.test", password, 200)
	bearer := func(token string) map[string]string { return map[string]string{"Authorization": "Bearer " + token} }
	ledgerOK := func(user string) {
		ensure(sql("benefits_db", fmt.Sprintf("SELECT c.saldo>=0 AND c.saldo=COALESCE(sum(m.puntos),0) AND c.ultima_secuencia=count(m.id) FROM cuentas c JOIN movimientos_puntos m ON m.cuenta_id=c.id WHERE c.alumno_id='%s' GROUP BY c.id;", user)) == "t", "Ledger no explica el saldo")
	}

	status, created := request(base, "POST", "/api/v1/benefits/partners", admin(""), map[string]any{"nombre": "Integración E2E", "max_puntos_operacion": 1000, "max_puntos_dia": 5000, "max_solicitudes_minuto": 600})
	ensure(status == 201 && created["estado"] == "ACTIVO" && created["permite_registro"] == false, "Alta de integración incorrecta")
	partner, key := created["id"].(string), created["api_key"].(string)
	ensure(len(key) == 64, "API key no emitida")
	ensure(sql("benefits_db", fmt.Sprintf("SELECT api_key_hash=sha256(convert_to('%s','UTF8')) FROM partners WHERE id='%s';", key, partner)) == "t", "No guarda solo el hash")
	status, list := request(base, "GET", "/api/v1/benefits/partners", admin(""), nil)
	raw, err := json.Marshal(list)
	must(err)
	ensure(status == 200 && !strings.Contains(string(raw), key), "El listado expone la API key")
	mark("club_alta_integracion_activa_y_api_key_una_sola_vez")

	status, _ = request(base, "GET", "/api/v1/benefits/partners", bearer(studentJWT), nil)
	ensure(status == 403, "Alumno administra integraciones")
	status, _ = request(base, "POST", "/api/v1/benefits/movements/"+uuid()+"/revert", map[string]string{"Authorization": "Bearer " + studentJWT, "Idempotency-Key": "alumno-revert-01", "X-User-Role": "ADMINISTRADOR"}, map[string]string{"motivo": "Escalada"})
	ensure(status == 403, "Cabecera de rol falsa aceptada")
	status, _ = request(benefitsURL, "GET", "/api/v1/benefits/partners", map[string]string{"X-User-Id": student, "X-User-Role": "ADMINISTRADOR"}, nil)
	ensure(status == 401, "Benefits acepta identidad sin el gateway")
	mark("club_solo_administrador_e_identidad_del_gateway")

	link := func(user, external string) int {
		status, _ := request(base, "POST", "/api/v1/benefits/partners/"+partner+"/links", admin(""), map[string]string{"usuario_id": user, "external_user_id": external})
		return status
	}
	ensure(link(student, "socio-e2e") == 201, "No vincula la cuenta")
	ensure(link(student, "socio-e2e") == 200, "Vínculo repetido con efectos")
	ensure(link(other, "socio-e2e") == 409, "CA-10.4: identificador reutilizado")
	partnerHeaders := func(apiKey, idem string) map[string]string {
		return map[string]string{"X-API-Key": apiKey, "Idempotency-Key": idem}
	}
	status, credit := request(base, "POST", "/partner-api/v2/accounts/socio-e2e/credits", partnerHeaders(key, "club-e2e-credit-01"), map[string]any{"eventId": "club-e2e-01", "points": 100, "reason": "Compra vinculada E2E", "reference": "club-e2e-01"})
	ensure(status == 201, "CA-10.3: el partner no opera con su identificador")
	creditID := credit["movementId"].(string)
	ledgerOK(student)
	mark("club_vinculacion_CA10_3_y_rechazo_CA10_4")

	status, rotated := request(base, "POST", "/api/v1/benefits/partners/"+partner+"/rotate-key", admin(""), nil)
	ensure(status == 200, "No rota la credencial")
	newKey := rotated["api_key"].(string)
	status, _ = request(base, "GET", "/partner-api/v2/accounts/socio-e2e/balance", map[string]string{"X-API-Key": key}, nil)
	ensure(status == 401, "La key anterior sigue vigente")
	status, _ = request(base, "GET", "/partner-api/v2/accounts/socio-e2e/balance", map[string]string{"X-API-Key": newKey}, nil)
	ensure(status == 200, "La key nueva no autentica")
	config := map[string]any{"nombre": "Integración E2E", "activo": false, "permite_consulta": true, "permite_puntos": true, "permite_registro": false, "requiere_evento": true, "max_puntos_operacion": 1000, "max_puntos_dia": 5000, "max_solicitudes_minuto": 600}
	status, _ = request(base, "PUT", "/api/v1/benefits/partners/"+partner, admin(""), config)
	ensure(status == 200, "No desactiva la integración")
	status, _ = request(base, "GET", "/partner-api/v2/accounts/socio-e2e/balance", map[string]string{"X-API-Key": newKey}, nil)
	ensure(status == 403, "CA-10.2: integración inactiva opera")
	config["activo"] = true
	status, _ = request(base, "PUT", "/api/v1/benefits/partners/"+partner, admin(""), config)
	ensure(status == 200, "No reactiva la integración")
	ensure(sql("benefits_db", fmt.Sprintf("SELECT count(*) FROM auditoria_club WHERE partner_id='%s';", partner)) == "5", "Auditoría incompleta")
	mark("club_rotacion_revoca_key_anterior_desactivacion_CA10_2_y_auditoria")

	before := sql("benefits_db", fmt.Sprintf("SELECT saldo FROM cuentas WHERE alumno_id='%s';", student))
	revert := func(movement, idem, reason string) (int, map[string]any) {
		return request(base, "POST", "/api/v1/benefits/movements/"+movement+"/revert", admin(idem), map[string]string{"motivo": reason})
	}
	status, reverted := revert(creditID, "club-revert-0001", "Compra anulada")
	ensure(status == 201 && reverted["movimiento"].(map[string]any)["puntos"] == float64(-100), "CA-11.1: compensatorio incorrecto")
	status, replay := revert(creditID, "club-revert-0001", "Compra anulada")
	ensure(status == 200 && replay["movimiento"].(map[string]any)["id"] == reverted["movimiento"].(map[string]any)["id"], "Reintento con la misma clave duplicó")
	status, conflict := revert(creditID, "club-revert-0001", "Otro motivo")
	ensure(status == 409 && conflict["code"] == "CONFLICTO_IDEMPOTENCIA", "Clave reutilizada con otros datos")
	status, again := revert(creditID, "club-revert-0002", "Compra anulada")
	ensure(status == 409 && again["code"] == "TRANSICION_INVALIDA", "CA-11.2: revierte dos veces")
	ledgerOK(student)
	status, account := request(base, "GET", "/api/v1/benefits/accounts/"+student, admin(""), nil)
	ensure(status == 200 && fmt.Sprint(account["saldo"]) == fmt.Sprint(int64(mustInt(before))-100), "Saldo administrado incorrecto")
	states := map[string]string{}
	for _, item := range account["items"].([]any) {
		m := item.(map[string]any)
		states[m["id"].(string)] = m["estado"].(string)
	}
	ensure(states[creditID] == "REVERTIDO", "Original no figura REVERTIDO")
	ensure(len(account["vinculaciones"].([]any)) == 1, "Vinculaciones de la cuenta incorrectas")
	status, own := request(base, "GET", "/api/v1/benefits/me/movements", bearer(studentJWT), nil)
	ensure(status == 200 && own["items"].([]any)[0].(map[string]any)["origen"] == "REVERSION", "El alumno no ve la reversión")
	status, partnerPage := request(base, "GET", "/partner-api/v2/accounts/socio-e2e/movements", map[string]string{"X-API-Key": newKey}, nil)
	ensure(status == 200 && partnerPage["totalItems"] == float64(1), "RN-26: el partner ve movimientos ajenos")
	mark("club_reversion_CA11_1_CA11_2_idempotente_estado_y_aislamiento")

	attendance := sql("benefits_db", fmt.Sprintf("SELECT m.id FROM movimientos_puntos m JOIN cuentas c ON c.id=m.cuenta_id WHERE c.alumno_id='%s' AND m.tipo='ASISTENCIA' LIMIT 1;", student))
	current := mustInt(sql("benefits_db", fmt.Sprintf("SELECT saldo FROM cuentas WHERE alumno_id='%s';", student)))
	status, result := revert(attendance, "club-revert-0003", "Ingreso registrado por error")
	// El orden asistencia/ausencia de la etapa 6 deja 400 o 500: con 400 aplica
	// CA-11.3 (saldo insuficiente); con 500 la reversión procede (CA-35.3).
	if current >= 500 {
		ensure(status == 201, "CA-35.3: no revierte la asistencia")
	} else {
		ensure(status == 422 && result["code"] == "SALDO_INSUFICIENTE", "CA-11.3: saldo negativo permitido")
	}
	ensure(sql("booking_db", fmt.Sprintf("SELECT count(*) FROM reservas WHERE alumno_id='%s' AND estado='ASISTIDA';", student)) == "1", "La reversión modificó la asistencia")
	ledgerOK(student)
	mark("club_reversion_asistencia_CA35_3_o_saldo_insuficiente_CA11_3")
}

// medicion resume una fase de carga constante.
type medicion struct {
	Requests, Errors                 int
	Codes                            map[int]int
	RPS, P50, P95, P99, ConsultasReq float64
}

// cargar ejecuta vus clientes en bucle cerrado durante d contra path y cuenta
// los accesos a la tabla fuente (idx_scan + seq_scan) por solicitud.
func cargar(base, path string, headers map[string]string, vus int, d time.Duration, scans func() float64) medicion {
	type sample struct {
		ms     float64
		ok     bool
		status int
	}
	out := make(chan sample, 1<<16)
	// Conexiones reutilizadas: el generador no debe agotar puertos efímeros.
	loader := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 64, IdleConnTimeout: 30 * time.Second}}
	defer loader.CloseIdleConnections()
	before := scans()
	start := time.Now()
	deadline := start.Add(d)
	done := make(chan struct{})
	for i := 0; i < vus; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for time.Now().Before(deadline) {
				req, err := http.NewRequestWithContext(ctx, "GET", base+path, nil)
				must(err)
				for k, v := range headers {
					req.Header.Set(k, v)
				}
				t0 := time.Now()
				res, err := loader.Do(req)
				ok := err == nil && res.StatusCode == 200
				status := -1
				if err == nil {
					status = res.StatusCode
					_, _ = io.Copy(io.Discard, res.Body)
					res.Body.Close()
				}
				out <- sample{float64(time.Since(t0).Microseconds()) / 1000, ok, status}
			}
		}()
	}
	for i := 0; i < vus; i++ {
		<-done
	}
	elapsed := time.Since(start).Seconds()
	close(out)
	var lat []float64
	m := medicion{}
	for s := range out {
		m.Requests++
		if !s.ok {
			m.Errors++
			if m.Codes == nil {
				m.Codes = map[int]int{}
			}
			m.Codes[s.status]++
		}
		lat = append(lat, s.ms)
	}
	sort.Float64s(lat)
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return lat[min(len(lat)-1, int(p*float64(len(lat))))]
	}
	m.RPS, m.P50, m.P95, m.P99 = float64(m.Requests)/elapsed, pct(0.50), pct(0.95), pct(0.99)
	if m.Requests > 0 {
		// PostgreSQL publica las estadísticas de un backend inactivo hasta 10 s después.
		time.Sleep(11 * time.Second)
		m.ConsultasReq = (scans() - before) / float64(m.Requests)
	}
	return m
}

// cacheFlow mide los catálogos que ADR-007 permite cachear (etapa 9) con la
// misma carga: 10 clientes concurrentes durante 20 s tras 3 s de calentamiento.
func cacheFlow(base, password string, fx searchFixture, sql func(string, string) string, admin func(string) map[string]string, login func(string, string, int) string) {
	if result.Metrics == nil {
		result.Metrics = map[string]float64{}
	}
	jwt := login("booking-student@example.test", password, 200)
	headers := map[string]string{"Authorization": "Bearer " + jwt}
	scans := func(db, table string) func() float64 {
		return func() float64 {
			var n float64
			_, err := fmt.Sscanf(sql(db, fmt.Sprintf("SELECT COALESCE(seq_scan,0)+COALESCE(idx_scan,0) FROM pg_stat_user_tables WHERE relname='%s'; SELECT pg_stat_clear_snapshot();", table)), "%g", &n)
			must(err)
			return n
		}
	}
	// Rondas alternadas A/B/A/B en la misma corrida: el ruido de la máquina afecta
	// a ambos modos por igual. Tipos de membresía (sin caché) es el control.
	type acumulado struct{ rps, p50, p95, p99, accesos float64 }
	totales := map[string]*acumulado{}
	for ronda, conCache := range []bool{false, true, false, true} {
		modo := "sin_cache"
		if conCache {
			modo = "con_cache"
		}
		fx.restartBooking(map[string]string{"BOOKING_CACHE_ENABLED": fmt.Sprint(conCache)})
		for _, target := range []struct{ name, path, db, table string }{
			{"actividades", "/api/v1/activities", "booking_db", "actividades"},
			{"control_tipos_membresia", "/api/v1/memberships/types", "members_db", "tipos_membresia"},
		} {
			cargar(base, target.path, headers, 10, 3*time.Second, func() float64 { return 0 })
			m := cargar(base, target.path, headers, 10, 20*time.Second, scans(target.db, target.table))
			ensure(m.Errors == 0, fmt.Sprintf("Errores durante la medición de %s: %v", target.name, m.Codes))
			key := target.name + "_" + modo
			if totales[key] == nil {
				totales[key] = &acumulado{}
			}
			t := totales[key]
			t.rps, t.p50, t.p95, t.p99, t.accesos = t.rps+m.RPS/2, t.p50+m.P50/2, t.p95+m.P95/2, t.p99+m.P99/2, t.accesos+m.ConsultasReq/2
			fmt.Printf("ronda %d %s %s: %.0f req/s p50=%.2fms p95=%.2fms p99=%.2fms accesos/req=%.2f\n", ronda+1, target.name, modo, m.RPS, m.P50, m.P95, m.P99, m.ConsultasReq)
		}
	}
	for key, t := range totales {
		for k, v := range map[string]float64{"rps": t.rps, "p50_ms": t.p50, "p95_ms": t.p95, "p99_ms": t.p99, "accesos_tabla_por_solicitud": t.accesos} {
			result.Metrics[key+"_"+k] = v
		}
	}
	ensure(totales["actividades_con_cache"].accesos < 0.01, "Con caché el catálogo sigue leyendo booking_db")
	mark("cache_medicion_alternada_misma_carga_con_control")
	metric := func(name string) float64 {
		res, err := client.Get(fx.bookingURL + "/metrics")
		must(err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		must(err)
		for _, line := range strings.Split(string(body), "\n") {
			var v float64
			if strings.HasPrefix(line, name+" ") {
				_, err := fmt.Sscanf(strings.TrimPrefix(line, name+" "), "%g", &v)
				must(err)
				return v
			}
		}
		panic("Métrica ausente: " + name)
	}
	hits, misses := metric("activities_cache_hits_total"), metric("activities_cache_misses_total")
	result.Metrics["actividades_redis_aciertos"] = hits
	result.Metrics["actividades_redis_fallos"] = misses
	result.Metrics["actividades_redis_tasa_aciertos"] = hits / (hits + misses)
	result.Metrics["actividades_redis_ms_promedio_por_operacion"] = metric("activities_cache_redis_seconds_total") * 1000 / metric("activities_cache_redis_operations_total")
	ensure(hits/(hits+misses) > 0.99, "La caché no está acertando")
	mark("cache_metricas_aciertos_fallos_y_latencia_expuestas")

	zumba := sql("booking_db", "SELECT id FROM actividades WHERE nombre='Zumba';")
	nombre := func() string {
		status, out := request(base, "GET", "/api/v1/activities", headers, nil)
		ensure(status == 200, "Catálogo no disponible")
		for _, item := range out["items"].([]any) {
			a := item.(map[string]any)
			if a["id"] == zumba {
				return a["nombre"].(string)
			}
		}
		return ""
	}
	ensure(nombre() == "Zumba", "Catálogo inicial incorrecto")
	status, _ := request(base, "PUT", "/api/v1/activities/"+zumba, admin(""), map[string]string{"nombre": "Zumba Cardio"})
	ensure(status == 200, "No modifica la actividad")
	ensure(nombre() == "Zumba Cardio", "La caché devolvió el nombre viejo después de modificar")
	ensure(metric("activities_cache_invalidations_total") >= 1, "No se invalidó la copia")
	mark("cache_invalidacion_por_escritura_visible_en_la_siguiente_lectura")

	docker("", "stop", fx.prefix+"-redis")
	errorsBefore := metric("activities_cache_errors_total")
	ensure(nombre() == "Zumba Cardio", "Sin Redis el catálogo no responde desde PostgreSQL")
	status, health := request(fx.bookingURL, "GET", "/health/ready", nil, nil)
	ensure(status == 200 && health["status"] == "degraded", "Readiness no informa Redis como dependencia parcial")
	ensure(metric("activities_cache_errors_total") > errorsBefore, "La falla de Redis no quedó registrada")
	m := cargar(base, "/api/v1/activities", headers, 10, 5*time.Second, func() float64 { return 0 })
	ensure(m.Errors == 0, "Sin Redis hubo errores en el catálogo")
	result.Metrics["actividades_sin_redis_p95_ms"] = m.P95
	status, _ = request(base, "PUT", "/api/v1/activities/"+zumba, admin(""), map[string]string{"nombre": "Zumba"})
	ensure(status == 200, "Sin Redis no se puede modificar la actividad")
	docker("", "start", fx.prefix+"-redis")
	eventually("readiness recuperada", func() bool {
		status, health := request(fx.bookingURL, "GET", "/health/ready", nil, nil)
		return status == 200 && health["status"] == "ready"
	})
	// La copia guardada antes de la caída sigue con el nombre viejo: la escritura
	// hecha sin Redis no pudo invalidarla. Se acota con el TTL (límite documentado)
	// y el E2E la limpia invalidando con una escritura nueva.
	status, _ = request(base, "PUT", "/api/v1/activities/"+zumba, admin(""), map[string]string{"nombre": "Zumba"})
	ensure(status == 200, "No modifica la actividad tras la recuperación")
	hitsBefore := metric("activities_cache_hits_total")
	ensure(nombre() == "Zumba" && nombre() == "Zumba", "Catálogo incorrecto tras la recuperación")
	ensure(metric("activities_cache_hits_total") > hitsBefore, "No vuelve a acertar tras recuperar Redis")
	mark("cache_redis_caido_degrada_lee_postgres_y_se_recupera")
}

// searchFixture conecta searchFlow con los procesos que arrancó run().
type searchFixture struct {
	bookingURL, prefix, directory, amqpURL, bookingDSN string
	restartBooking                                     func(map[string]string)
}

// searchFlow cubre la etapa 8 (ADR-014, D-45, HU-13, RN-14): indexer por
// eventos, reindexado con alias, filtros/orden/paginación iguales a PostgreSQL,
// retraso < 2 s, versión vieja descartada, índice desactualizado que no decide
// la reserva y caída de OpenSearch con 503 explícito y recuperación.
func searchFlow(base, password string, fx searchFixture, sql func(string, string) string, admin func(string) map[string]string, login func(string, string, int) string) {
	if result.Metrics == nil {
		result.Metrics = map[string]float64{}
	}
	name := fx.prefix + "-opensearch"
	osAddress := port()
	docker("", "run", "-d", "--name", name, "-p", osAddress+":9200", "-e", "discovery.type=single-node", "-e", "DISABLE_SECURITY_PLUGIN=true", "-e", "DISABLE_INSTALL_DEMO_CONFIG=true", "-e", "OPENSEARCH_JAVA_OPTS=-Xms512m -Xmx512m", "-e", "action.auto_create_index=false", "opensearchproject/opensearch:2.17.1")
	osURL := "http://" + osAddress
	osUp := func() bool {
		res, err := client.Get(osURL + "/_cluster/health")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		return res.StatusCode == 200
	}
	eventuallyWithin("OpenSearch", 3*time.Minute, osUp)

	// Más clases futuras de dos actividades para que el índice tenga contenido.
	professor := sql("members_db", "SELECT id FROM usuarios WHERE email='booking-prof@example.test';")
	zone, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	must(err)
	tomorrow := time.Now().In(zone).AddDate(0, 0, 1)
	weekday := int(tomorrow.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	from, to := tomorrow.Format(time.DateOnly), tomorrow.AddDate(0, 0, 27).Format(time.DateOnly)
	funcional := sql("booking_db", "SELECT id FROM actividades WHERE nombre='Funcional';")
	status, _ := request(base, "POST", "/api/v1/activities/"+funcional+"/schedules", admin(""), map[string]any{"dia_semana": weekday, "inicio": "18:00", "duracion_minutos": 60, "profesor_id": professor, "capacidad": 20})
	ensure(status == 201, "No crea horario de Funcional")
	status, generated := request(base, "POST", "/api/v1/classes/generate", admin(""), map[string]string{"desde": from, "hasta": to})
	ensure(status == 200 && generated["creadas"].(float64) >= 7, "No genera clases para la búsqueda")

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	indexerAddress := port()
	indexerURL := "http://" + indexerAddress
	indexerEnv := map[string]string{"BOOKING_INDEXER_HTTP_ADDR": indexerAddress, "BOOKING_DB_DSN": fx.bookingDSN, "BOOKING_INDEXER_ENABLED": "true", "BOOKING_AMQP_URL": fx.amqpURL, "BOOKING_OPENSEARCH_URL": osURL, "LOG_LEVEL": "error", "OTEL_SDK_DISABLED": "true"}
	stopIndexer := startBinary(fx.directory, "indexer", "./services/booking-service/cmd/indexer", indexerEnv)
	defer func() { stopIndexer() }()
	ready(indexerURL)
	aliasIndex := func() string {
		status, out := request(osURL, "GET", "/_alias/clases-read", nil, nil)
		ensure(status == 200 && len(out) == 1, "El alias debe apuntar a un solo índice")
		for k := range out {
			return k
		}
		return ""
	}
	eventually("alias inicial", func() bool {
		status, _ := request(osURL, "GET", "/_alias/clases-read", nil, nil)
		return status == 200
	})
	initial := aliasIndex()
	// Reindexado completo con el indexer consumiendo al mismo tiempo (ADR-014).
	reindex := exec.CommandContext(ctx, filepath.Join(fx.directory, "indexer"+ext), "-reindex")
	reindex.Env = envWith(indexerEnv)
	if output, err := reindex.CombinedOutput(); err != nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		panic("Reindexado falló: " + lines[len(lines)-1])
	}
	summary := func(index string) string {
		request(osURL, "POST", "/"+index+"/_refresh", nil, nil)
		_, out := request(osURL, "POST", "/"+index+"/_search", nil, map[string]any{"size": 0, "track_total_hits": true, "aggs": map[string]any{"s": map[string]any{"sum": map[string]any{"field": "version"}}}})
		total := out["hits"].(map[string]any)["total"].(map[string]any)["value"].(float64)
		sum := out["aggregations"].(map[string]any)["s"].(map[string]any)["value"].(float64)
		return fmt.Sprintf("%d:%d", int64(total), int64(sum))
	}
	eventually("índice converge con booking_db", func() bool {
		return summary("clases-read") == sql("booking_db", "SELECT count(*)||':'||COALESCE(sum(version),0) FROM clases;")
	})
	current := aliasIndex()
	ensure(current != initial, "El reindexado no movió el alias")
	status, _ = request(osURL, "GET", "/"+initial, nil, nil)
	ensure(status == 404, "El índice anterior no se eliminó")
	mark("busqueda_indexer_y_reindexado_con_alias_convergen_con_postgres")

	fx.restartBooking(map[string]string{"BOOKING_SEARCH_ENABLED": "true", "BOOKING_OPENSEARCH_URL": osURL})
	studentJWT := login("booking-student@example.test", password, 200)
	student := func(key string) map[string]string {
		return map[string]string{"Authorization": "Bearer " + studentJWT, "Idempotency-Key": key}
	}
	apiIDs := func(headers map[string]string, query string) (string, []any) {
		status, page := request(base, "GET", "/api/v1/classes?fecha_desde="+from+"&fecha_hasta="+to+query, headers, nil)
		ensure(status == 200, fmt.Sprintf("Búsqueda HTTP %d", status))
		items := page["items"].([]any)
		ids := make([]string, 0, len(items))
		for _, item := range items {
			ids = append(ids, item.(map[string]any)["id"].(string))
		}
		return strings.Join(ids, ","), items
	}
	pgIDs := func(where string, limit, offset int) string {
		return sql("booking_db", fmt.Sprintf("SELECT COALESCE(string_agg(id::text, ',' ORDER BY inicio, id),'') FROM (SELECT id, inicio FROM clases WHERE inicio >= ('%s'::date)::timestamp AT TIME ZONE 'America/Argentina/Buenos_Aires' AND inicio < ('%s'::date + 1)::timestamp AT TIME ZONE 'America/Argentina/Buenos_Aires' %s ORDER BY inicio, id LIMIT %d OFFSET %d) x;", from, to, where, limit, offset))
	}
	all, items := apiIDs(admin(""), "&tamano=100")
	ensure(all == pgIDs("", 100, 0) && len(items) >= 8, "Orden o contenido distinto de PostgreSQL")
	page, _ := apiIDs(admin(""), "&tamano=3&pagina=2")
	ensure(page == pgIDs("", 3, 3), "Paginación distinta de PostgreSQL")
	byActivity, _ := apiIDs(admin(""), "&tamano=100&actividad_id="+funcional)
	ensure(byActivity == pgIDs("AND actividad_id='"+funcional+"'", 100, 0), "Filtro por actividad distinto")
	bySlot, _ := apiIDs(admin(""), "&tamano=100&franja=18:00")
	ensure(bySlot == pgIDs("AND to_char(inicio AT TIME ZONE 'America/Argentina/Buenos_Aires','HH24:MI')='18:00'", 100, 0), "Filtro por franja distinto")
	available, _ := apiIDs(admin(""), "&tamano=100&solo_disponibles=true")
	ensure(available == pgIDs("AND estado='PROGRAMADA' AND inicio>now() AND ocupacion<capacidad", 100, 0), "Filtro de disponibilidad distinto")
	status, _ = request(base, "GET", "/api/v1/classes?pagina=101&tamano=100", admin(""), nil)
	ensure(status == 400, "Profundidad mayor a 10.000 aceptada")
	classIDs := strings.Split(byActivity, ",")
	status, _ = request(base, "POST", "/api/v1/bookings", student("search-reserve-001"), map[string]string{"clase_id": classIDs[2]})
	ensure(status == 201, "No reserva clase encontrada en el índice")
	eventually("reserva visible como propia", func() bool {
		_, mine := apiIDs(student(""), "&tamano=100&actividad_id="+funcional)
		for _, item := range mine {
			m := item.(map[string]any)
			if m["id"] == classIDs[2] {
				return m["reservada_por_mi"] == true && m["ocupacion"] == float64(1)
			}
		}
		return false
	})
	mark("busqueda_filtros_orden_paginacion_iguales_a_postgres_profundidad_y_reservada_por_mi")

	visible := func(id, field string, value any) func() bool {
		return func() bool {
			status, out := request(base, "GET", "/api/v1/classes?fecha_desde="+from+"&fecha_hasta="+to+"&tamano=100&actividad_id="+funcional, admin(""), nil)
			if status != 200 {
				return false
			}
			for _, item := range out["items"].([]any) {
				m := item.(map[string]any)
				if m["id"] == id {
					return m[field] == value
				}
			}
			return false
		}
	}
	target := classIDs[0]
	var maxLag, sumLag float64
	for i := 0; i < 5; i++ {
		capacity := 21 + i
		start := time.Now()
		status, _ := request(base, "PUT", "/api/v1/classes/"+target+"/capacity", admin(""), map[string]int{"capacidad": capacity})
		ensure(status == 200, "No cambia la capacidad")
		// Sondeo cada 50 ms: la granularidad de la medición queda muy por debajo de 2 s.
		check := visible(target, "capacidad", float64(capacity))
		for !check() {
			ensure(time.Since(start) < 10*time.Second, "Tiempo agotado: cambio visible en la búsqueda")
			time.Sleep(50 * time.Millisecond)
		}
		lag := time.Since(start).Seconds()
		sumLag += lag
		maxLag = max(maxLag, lag)
	}
	result.Metrics["busqueda_retraso_max_s"] = maxLag
	result.Metrics["busqueda_retraso_promedio_s"] = sumLag / 5
	metric := func(name string) float64 {
		res, err := client.Get(indexerURL + "/metrics")
		must(err)
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		must(err)
		for _, line := range strings.Split(string(body), "\n") {
			var value float64
			if strings.HasPrefix(line, name+" ") {
				_, err := fmt.Sscanf(strings.TrimPrefix(line, name+" "), "%g", &value)
				must(err)
				return value
			}
		}
		panic("Métrica ausente: " + name)
	}
	// El máximo del indexer incluye el backlog encolado antes de que existiera
	// (etapas 4 a 7); el último valor es el retraso en régimen normal.
	result.Metrics["indexer_retraso_commit_a_indice_ultimo_s"] = metric("clase_index_lag_seconds_last")
	result.Metrics["indexer_retraso_max_con_backlog_inicial_s"] = metric("clase_index_lag_seconds_max")
	ensure(maxLag < 2, fmt.Sprintf("Retraso escritura → búsqueda %.3f s supera 2 s", maxLag))
	mark("busqueda_retraso_escritura_a_busqueda_menor_a_2s_medido")

	bus, err := messaging.Dial(ctx, fx.amqpURL)
	must(err)
	defer bus.Close()
	old := sql("booking_db", fmt.Sprintf("SELECT jsonb_build_object('event_id',gen_random_uuid(),'event_type','clase.actualizada','event_version',1,'occurred_at',occurred_at,'producer','booking-service','data',data)::text FROM outbox WHERE event_type='clase.actualizada' AND agregado_id='%s' ORDER BY (data->>'version')::bigint LIMIT 1;", target))
	stale := metric("clase_index_stale_total")
	must(bus.Publish(ctx, "gym.events", "clase.actualizada", amqp.Publishing{ContentType: "application/json", Body: []byte(old)}))
	eventually("evento viejo descartado", func() bool { return metric("clase_index_stale_total") > stale })
	ensure(visible(target, "capacidad", float64(25))(), "Una versión vieja pisó la nueva")
	mark("busqueda_evento_viejo_no_pisa_version_nueva")

	stopIndexer()
	cancelled := classIDs[3]
	status, _ = request(base, "POST", "/api/v1/classes/"+cancelled+"/cancel", admin(""), map[string]string{"motivo": "Prueba de revalidación"})
	ensure(status == 200, "No cancela la clase")
	ensure(visible(cancelled, "estado", "PROGRAMADA")(), "Sin indexer la búsqueda debería seguir atrasada")
	status, rejected := request(base, "POST", "/api/v1/bookings", student("search-reserve-002"), map[string]string{"clase_id": cancelled})
	ensure(status == 422 && rejected["code"] == "CLASE_CANCELADA", fmt.Sprintf("La reserva no se revalidó en PostgreSQL: HTTP %d %v", status, rejected["code"]))
	stopIndexer = startBinary(fx.directory, "indexer", "./services/booking-service/cmd/indexer", indexerEnv)
	ready(indexerURL)
	eventually("el índice alcanza la cancelación", visible(cancelled, "estado", "CANCELADA"))
	mark("busqueda_atrasada_no_decide_la_reserva_revalidada_en_postgres")

	docker("", "stop", name)
	status, down := request(base, "GET", "/api/v1/classes", admin(""), nil)
	ensure(status == 503 && down["code"] == "DEPENDENCIA_NO_DISPONIBLE", "Sin OpenSearch la búsqueda no responde 503")
	status, health := request(fx.bookingURL, "GET", "/health/ready", nil, nil)
	ensure(status == 200 && health["status"] == "degraded", "Readiness de Booking no informa degradación parcial")
	status, _ = request(base, "PUT", "/api/v1/classes/"+target+"/capacity", admin(""), map[string]int{"capacidad": 26})
	ensure(status == 200, "Sin OpenSearch no se puede escribir en Booking")
	status, _ = request(base, "GET", "/api/v1/classes/"+target, admin(""), nil)
	ensure(status == 200, "El detalle de la clase depende de OpenSearch")
	docker("", "start", name)
	eventuallyWithin("OpenSearch reiniciado", 3*time.Minute, osUp)
	eventuallyWithin("búsqueda recuperada con el cambio hecho durante la caída", 3*time.Minute, visible(target, "capacidad", float64(26)))
	eventually("readiness recuperada", func() bool {
		status, health := request(fx.bookingURL, "GET", "/health/ready", nil, nil)
		return status == 200 && health["status"] == "ready"
	})
	mark("busqueda_caida_OpenSearch_503_explicito_escrituras_siguen_y_recupera")
}

func mustInt(s string) int {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	must(err)
	return n
}

func main() {
	flag.Parse()
	if *withCache {
		*withBooking = true
	}
	if *withSearch {
		*withClub = true
	}
	if *withClub {
		*withAttendance = true
	}
	if *withAttendance {
		*withMessaging = true
	}
	if *withMessaging {
		*withBooking = true
	}
	if *withBooking {
		*withMemberships = true
	}
	if *withMemberships {
		result.Stage = "CB-04"
		result.Scope += "; membership expiry, return, type changes, ledger preservation and authorization"
	}
	if *withBooking {
		result.Stage = "Entrega final - etapa 4"
		result.Scope += "; real Booking, Members and Gateway HTTP flow; no browser automation or points consumer"
	}
	if *withMessaging {
		result.Stage = "Entrega final - etapa 5"
		result.Scope += "; isolated RabbitMQ, MongoDB and Mailpit, outbox relay, notification worker, duplicate, broker and SMTP outage, DLQ"
	}
	if *withAttendance {
		result.Stage = "Entrega final - etapa 6"
		result.Scope += "; Booking outbox, real Benefits consumer, internal points ledger, own movement history, deduplication and Benefits restart"
	}
	if *withClub {
		result.Stage = "Entrega final - etapa 7"
		result.Scope += "; Club administration through the gateway: integrations, one-time API keys, rotation, deactivation, account links, admin account view and idempotent reversals"
	}
	if *withCache {
		result.Stage = "Entrega final - etapa 9"
		result.Scope += "; isolated Redis 7, activities catalog cache-aside measured A/B/A/B in the same run (10 closed-loop clients, 20 s) with membership types as control, invalidation and Redis outage"
	}
	if *withSearch {
		result.Stage = "Entrega final - etapa 8"
		result.Scope += "; isolated OpenSearch 2.17, booking-indexer consuming clase.actualizada, alias reindex with concurrent consumer, filters compared with PostgreSQL, measured write-to-search lag, stale event, revalidation and OpenSearch outage"
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
	if *withBooking {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa4", "booking-e2e.json")
	}
	if *withMessaging {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa5", "messaging-e2e.json")
	}
	if *withAttendance {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa6", "attendance-e2e.json")
	}
	if *withClub {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa7", "club-e2e.json")
	}
	if *withCache {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa9", "cache-e2e.json")
	}
	if *withSearch {
		path = filepath.Join("docs", "evidence", "entrega-final", "etapa8", "search-e2e.json")
	}
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
