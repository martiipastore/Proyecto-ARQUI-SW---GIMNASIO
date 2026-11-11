package melt

// Test de contrato del consumidor (enunciado §4, etapa 13): compara lo que
// enviamos e interpretamos con el OpenAPI publicado por MELT y versionado en
// docs/contracts/providers/melt. Si el proveedor cambia algo incompatible
// (nombres de campos, cabecera de clave, códigos de respuesta o esquema del
// pendiente), este test falla al actualizar la copia del contrato.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const contrato = "../../../../../docs/contracts/providers/melt/acreditar-puntos-v1.openapi.yaml"

func cargarContrato(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(contrato))
	require.NoError(t, err, "falta la copia versionada del contrato de MELT")
	var doc map[string]any
	require.NoError(t, yaml.Unmarshal(raw, &doc))
	return doc
}

func nodo(t *testing.T, doc map[string]any, ruta ...string) map[string]any {
	t.Helper()
	cur := doc
	for _, k := range ruta {
		next, ok := cur[k].(map[string]any)
		require.Truef(t, ok, "el contrato no tiene %s", strings.Join(ruta, " > "))
		cur = next
	}
	return cur
}

func TestContratoMELT_VersionYOperacion(t *testing.T) {
	doc := cargarContrato(t)
	info := nodo(t, doc, "info")
	require.True(t, strings.HasPrefix(info["version"].(string), "1."), "integración construida sobre la versión 1.x del contrato")
	post := nodo(t, doc, "paths", "/beneficios/acreditaciones", "post")
	sec := post["security"].([]any)
	require.Contains(t, sec[0].(map[string]any), "PartnerKey")
	key := nodo(t, doc, "components", "securitySchemes", "PartnerKey")
	require.Equal(t, "header", key["in"])
	require.Equal(t, "X-Partner-Key", key["name"], "la cabecera de la clave que enviamos")
	servers := doc["servers"].([]any)
	require.Equal(t, "https://melt-gateway-prueba.onrender.com/api/v1", servers[0].(map[string]any)["url"], "URL base configurada por defecto")
}

func TestContratoMELT_NuestraSolicitudCumpleElEsquema(t *testing.T) {
	doc := cargarContrato(t)
	schema := nodo(t, doc, "components", "schemas", "Acreditacion")
	require.Equal(t, false, schema["additionalProperties"], "MELT rechaza propiedades desconocidas")
	props := nodo(t, doc, "components", "schemas", "Acreditacion", "properties")

	body, err := json.Marshal(Solicitud{Email: "alumno@gym.local", Puntos: 100, Referencia: "metalfitness-asistencia-33333333-3333-4333-8333-333333333333", Descripcion: "Asistencia a clase en Metal Fitness"})
	require.NoError(t, err)
	var enviado map[string]any
	require.NoError(t, json.Unmarshal(body, &enviado))
	for campo := range enviado {
		require.Containsf(t, props, campo, "enviamos %q y el contrato no lo define", campo)
	}
	for _, req := range schema["required"].([]any) {
		require.Contains(t, enviado, req.(string))
	}
	// Exactamente un identificador de operación y un destinatario (oneOf).
	require.Contains(t, enviado, "referencia")
	require.NotContains(t, enviado, "solicitudId")
	require.Contains(t, enviado, "email")
	require.NotContains(t, enviado, "usuarioId")

	ref := props["referencia"].(map[string]any)
	require.LessOrEqual(t, len(enviado["referencia"].(string)), ref["maxLength"].(int))
	desc := props["descripcion"].(map[string]any)
	require.LessOrEqual(t, len(enviado["descripcion"].(string)), desc["maxLength"].(int))
	puntos := props["puntos"].(map[string]any)
	require.Equal(t, "integer", puntos["type"])
	require.GreaterOrEqual(t, 100, puntos["minimum"].(int))
}

func TestContratoMELT_CodigosQueInterpretamos(t *testing.T) {
	doc := cargarContrato(t)
	responses := nodo(t, doc, "paths", "/beneficios/acreditaciones", "post", "responses")
	for _, code := range []string{"200", "201", "202", "400", "401", "404", "409", "415", "422", "503"} {
		require.Containsf(t, responses, code, "el adaptador interpreta %s y el contrato ya no lo declara", code)
	}
	pendiente := nodo(t, doc, "components", "schemas", "Pendiente")
	required := map[string]bool{}
	for _, r := range pendiente["required"].([]any) {
		required[r.(string)] = true
	}
	require.True(t, required["registroUrl"], "leemos registroUrl del 202")
	require.True(t, required["venceEn"], "leemos venceEn del 202")
	errProps := nodo(t, doc, "components", "schemas", "Error", "properties", "error", "properties")
	require.Contains(t, errProps, "code", "leemos error.code para el motivo del rechazo")
}
