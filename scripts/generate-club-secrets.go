// Ejecutar: go run scripts/generate-club-secrets.go > .env.vercel-secrets
// El archivo resultante está ignorado por Git y contiene credenciales privadas.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

func main() {
	serviceSecret, err := randomHex()
	if err != nil {
		fail(err)
	}
	activationSecret, err := randomHex()
	if err != nil {
		fail(err)
	}
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		fail(err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(private)
	if err != nil {
		fail(err)
	}
	publicDER, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	if err != nil {
		fail(err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	fmt.Println("GYM_INTERNAL_SERVICE_KEY=" + serviceSecret)
	fmt.Println("MEMBERS_ACTIVATION_ENCRYPTION_KEY=" + activationSecret)
	fmt.Println("MEMBERS_JWT_PRIVATE_KEY=" + strings.ReplaceAll(strings.TrimSpace(string(privatePEM)), "\n", `\n`))
	fmt.Println("GATEWAY_JWT_PUBLIC_KEY=" + strings.ReplaceAll(strings.TrimSpace(string(publicPEM)), "\n", `\n`))
}

func randomHex() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
