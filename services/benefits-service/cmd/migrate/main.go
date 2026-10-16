// Comando migrate aplica el esquema antes de usar los casos de negocio.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/GuadiViti/Arquitectura-de-Software/services/benefits-service/internal/adapters/postgres"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("BENEFITS_DB_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "Falta BENEFITS_DB_DSN")
		os.Exit(1)
	}
	db, err := postgres.Open(ctx, dsn)
	if err == nil {
		defer db.Close()
		err = db.Migrate(ctx)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "No se pudieron aplicar las migraciones de Benefits")
		os.Exit(1)
	}
	fmt.Println("Migraciones de Benefits aplicadas")
}
