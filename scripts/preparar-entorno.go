// preparar-entorno crea el .env local a partir de .env.example con todas las
// funcionalidades prendidas y secretos aleatorios propios de cada máquina
// (enunciado §2.12: procedimiento único, automatizado y sin secretos en el
// repositorio). No pisa un .env existente salvo con -forzar.
//
//	go run scripts/preparar-entorno.go            # crea .env
//	go run scripts/preparar-entorno.go -forzar    # lo regenera (conserva BENEFITS_MELT_PARTNER_KEY)
//
// Lo usa `make demo`, que además levanta el stack completo y reconstruye el
// índice de búsqueda.
package main

import (
	"bufio"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Funcionalidades que el stack local de demostración necesita prendidas.
var prendidas = []string{
	"MEMBERS_IDENTITY_ENABLED", "MEMBERS_MESSAGING_ENABLED", "MEMBERS_BACKGROUND_WORKER_ENABLED",
	"BOOKING_ENABLED", "BOOKING_MESSAGING_ENABLED", "BOOKING_SEARCH_ENABLED", "BOOKING_INDEXER_ENABLED", "BOOKING_CACHE_ENABLED",
	"BENEFITS_MESSAGING_ENABLED", "BENEFITS_EXTERNAL_REGISTRATION_ENABLED",
	"NOTIFICATION_MESSAGING_ENABLED", "TRAINING_ENABLED", "GATEWAY_RATE_LIMIT_ENABLED",
	// Mailpit local no usa TLS.
	"MEMBERS_SMTP_ALLOW_PLAINTEXT", "NOTIFICATION_SMTP_ALLOW_PLAINTEXT",
}

func main() {
	forzar := flag.Bool("forzar", false, "regenerar .env aunque exista")
	flag.Parse()

	anterior := map[string]string{}
	if _, err := os.Stat(".env"); err == nil {
		if !*forzar {
			fmt.Println(".env ya existe: no se modifica. Usá -forzar para regenerarlo.")
			return
		}
		anterior = leer(".env")
	}
	ejemplo, err := os.ReadFile(".env.example")
	if err != nil {
		fallar("no se encontró .env.example; ejecutá el comando desde la raíz del repositorio")
	}

	servicio, activacion := aleatorio(), aleatorio()
	privada, publica := clavesJWT()
	valores := map[string]string{
		"GYM_INTERNAL_SERVICE_KEY":          servicio,
		"MEMBERS_ACTIVATION_ENCRYPTION_KEY": activacion,
		"MEMBERS_JWT_PRIVATE_KEY":           privada,
		"GATEWAY_JWT_PUBLIC_KEY":            publica,
	}
	for _, k := range prendidas {
		valores[k] = "true"
	}
	// La clave de MELT la entrega el otro grupo por privado: se conserva si ya
	// estaba y la integración se prende solo si hay clave.
	if k := anterior["BENEFITS_MELT_PARTNER_KEY"]; k != "" {
		valores["BENEFITS_MELT_PARTNER_KEY"] = k
		valores["BENEFITS_MELT_ENABLED"] = "true"
	}

	var out strings.Builder
	escritas := map[string]bool{}
	sc := bufio.NewScanner(strings.NewReader(string(ejemplo)))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		linea := sc.Text()
		clave, _, ok := strings.Cut(linea, "=")
		if v, existe := valores[clave]; ok && existe && !strings.HasPrefix(strings.TrimSpace(linea), "#") {
			linea = clave + "=" + v
			escritas[clave] = true
		}
		out.WriteString(linea + "\n")
	}
	for _, k := range append(prendidas, "BENEFITS_MELT_PARTNER_KEY", "BENEFITS_MELT_ENABLED") {
		if v, ok := valores[k]; ok && !escritas[k] {
			out.WriteString(k + "=" + v + "\n")
			escritas[k] = true
		}
	}
	if err := os.WriteFile(".env", []byte(out.String()), 0o600); err != nil {
		fallar(err.Error())
	}
	fmt.Println(".env creado con secretos propios y todas las funcionalidades prendidas.")
	if valores["BENEFITS_MELT_PARTNER_KEY"] == "" {
		fmt.Println("MELT queda apagado: cargá BENEFITS_MELT_PARTNER_KEY (la entrega MELT por privado) y BENEFITS_MELT_ENABLED=true.")
	}
}

func leer(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, l := range strings.Split(string(raw), "\n") {
		if k, v, ok := strings.Cut(strings.TrimRight(l, "\r"), "="); ok && !strings.HasPrefix(k, "#") {
			out[k] = v
		}
	}
	return out
}

func aleatorio() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		fallar(err.Error())
	}
	return hex.EncodeToString(b)
}

// clavesJWT genera el par RS256 de ADR-006 en una sola línea con \n escapados.
func clavesJWT() (string, string) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fallar(err.Error())
	}
	priv, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		fallar(err.Error())
	}
	pub, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
	if err != nil {
		fallar(err.Error())
	}
	linea := func(b []byte) string { return strings.ReplaceAll(strings.TrimSpace(string(b)), "\n", `\n`) }
	return linea(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: priv})),
		linea(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pub}))
}

func fallar(msg string) {
	fmt.Fprintln(os.Stderr, "preparar-entorno:", msg)
	os.Exit(1)
}
