// Package migrations conserva el SQL versionado junto al binario.
package migrations

import "embed"

//go:embed *.sql
var Files embed.FS
