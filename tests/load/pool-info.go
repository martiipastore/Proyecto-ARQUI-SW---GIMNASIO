// Diagnostico de configuracion: no abre conexiones ni imprime el DSN.
package main

import (
	"encoding/json"
	"log"
	"os"
	"runtime"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := pgxpool.ParseConfig(os.Getenv("GYM_LOAD_DSN"))
	if err != nil {
		log.Fatal("configuracion de pool invalida")
	}
	value := map[string]any{
		"max_conns": cfg.MaxConns, "min_conns": cfg.MinConns,
		"max_conn_lifetime": cfg.MaxConnLifetime.String(), "max_conn_idle_time": cfg.MaxConnIdleTime.String(),
		"health_check_period": cfg.HealthCheckPeriod.String(), "connect_timeout": cfg.ConnConfig.ConnectTimeout.String(),
		"parser_cpu_count": runtime.NumCPU(), "parsed_on": "Windows host; compare CPU count with Docker",
	}
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		log.Fatal("no se pudo escribir el diagnostico")
	}
}
