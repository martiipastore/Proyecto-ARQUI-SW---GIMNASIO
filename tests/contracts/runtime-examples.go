// Genera un OpenAPI temporal con respuestas reales como ejemplos. Reutiliza
// yaml.v3, ya presente en el workspace; Spectral hace la validación de esquemas.
package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	if len(os.Args) != 3 {
		return fmt.Errorf("uso: runtime-examples <respuestas.json> <openapi-temporal.json>")
	}
	contents, err := os.ReadFile("docs/contracts/benefits-api.v1.yaml")
	if err != nil {
		return err
	}
	var contract map[string]any
	if err = yaml.Unmarshal(contents, &contract); err != nil {
		return err
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		return err
	}
	var responses []struct {
		Path, Method, Status, Schema string
		Body                         any
	}
	if err = json.Unmarshal(raw, &responses); err != nil {
		return err
	}
	for i, e := range responses {
		operation := contract["paths"].(map[string]any)[e.Path].(map[string]any)[strings.ToLower(e.Method)].(map[string]any)
		response, ok := operation["responses"].(map[string]any)[e.Status].(map[string]any)
		if !ok {
			return fmt.Errorf("respuesta no declarada: %s %s", e.Path, e.Status)
		}
		if ref, ok := response["$ref"].(string); ok {
			parts := strings.Split(ref, "/")
			response = contract["components"].(map[string]any)["responses"].(map[string]any)[parts[len(parts)-1]].(map[string]any)
		}
		media := "application/json"
		if e.Schema == "Problem" {
			media = "application/problem+json"
		}
		content := response["content"].(map[string]any)[media].(map[string]any)
		if content["schema"].(map[string]any)["$ref"] != "#/components/schemas/"+e.Schema {
			return fmt.Errorf("esquema inesperado")
		}
		examples, ok := content["examples"].(map[string]any)
		if !ok {
			examples = map[string]any{}
			content["examples"] = examples
		}
		examples[fmt.Sprintf("runtime%d", i)] = map[string]any{"value": e.Body}
	}
	out, err := json.Marshal(contract)
	if err != nil {
		return err
	}
	if err = os.WriteFile(os.Args[2], out, 0600); err != nil {
		return err
	}
	fmt.Printf("%d respuestas reales preparadas para Spectral\n", len(responses))
	return nil
}
