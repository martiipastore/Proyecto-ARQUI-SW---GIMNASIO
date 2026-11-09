// Comando de administración para emitir una credencial de integración una sola vez.
// Usar con BENEFITS_DB_DSN de producción después de aplicar las migraciones.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func main() {
	name := flag.String("name", "", "nombre del sistema consumidor")
	rotateID := flag.String("rotate-id", "", "ID del partner cuya API key se reemplazará")
	listUsersID := flag.String("list-users", "", "ID del partner cuyos externalUserId se listarán")
	flag.Parse()
	creating := strings.TrimSpace(*name) != ""
	rotating := strings.TrimSpace(*rotateID) != ""
	listing := strings.TrimSpace(*listUsersID) != ""
	selected := 0
	for _, enabled := range []bool{creating, rotating, listing} {
		if enabled {
			selected++
		}
	}
	if selected != 1 {
		fmt.Fprintln(os.Stderr, "indicá exactamente una opción: -name, -rotate-id o -list-users")
		os.Exit(2)
	}
	dsn := os.Getenv("BENEFITS_DB_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "BENEFITS_DB_DSN no está configurada")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo conectar a benefits_db:", err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if listing {
		rows, err := conn.Query(ctx, `SELECT external_user_id FROM vinculaciones WHERE partner_id=$1::uuid ORDER BY external_user_id`, strings.TrimSpace(*listUsersID))
		if err != nil {
			fmt.Fprintln(os.Stderr, "no se pudieron consultar los usuarios del partner:", err)
			os.Exit(1)
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var externalID string
			if err := rows.Scan(&externalID); err != nil {
				fmt.Fprintln(os.Stderr, "no se pudo leer un usuario:", err)
				os.Exit(1)
			}
			fmt.Println("externalUserId:", externalID)
			count++
		}
		if err := rows.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "no se pudo completar la consulta:", err)
			os.Exit(1)
		}
		if count == 0 {
			fmt.Println("No hay usuarios vinculados a este partner.")
		}
		return
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		fmt.Fprintln(os.Stderr, "no se pudo generar la API key:", err)
		os.Exit(1)
	}
	key := hex.EncodeToString(secret)
	hash := sha256.Sum256([]byte(key))
	var id string
	if creating {
		err = conn.QueryRow(ctx, `INSERT INTO partners
			(id,nombre,api_key_hash,permite_registro,permite_consulta,permite_puntos,requiere_evento)
			VALUES (gen_random_uuid(),$1,$2,true,true,true,true) RETURNING id::text`, strings.TrimSpace(*name), hash[:]).Scan(&id)
		if err != nil {
			fmt.Fprintln(os.Stderr, "no se pudo crear el partner:", err)
			os.Exit(1)
		}
	} else {
		err = conn.QueryRow(ctx, `UPDATE partners SET api_key_hash=$2 WHERE id=$1::uuid RETURNING id::text`, strings.TrimSpace(*rotateID), hash[:]).Scan(&id)
		if err != nil {
			fmt.Fprintln(os.Stderr, "no se pudo rotar la API key del partner:", err)
			os.Exit(1)
		}
	}
	fmt.Println("Partner ID:", id)
	fmt.Println("API key (se muestra una sola vez):", key)
}
