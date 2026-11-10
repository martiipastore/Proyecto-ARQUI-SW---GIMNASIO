// Package migrations conserva el SQL versionado de booking_db junto al binario.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
