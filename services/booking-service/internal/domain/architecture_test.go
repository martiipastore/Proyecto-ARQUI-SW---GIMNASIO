package domain

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Reglas de ADR-002 para el patrón hexagonal:
//   - domain: solo biblioteca estándar, sin tags en las structs.
//   - application y ports: pueden usar domain, pero no adaptadores ni frameworks.

const module = "github.com/GuadiViti/Arquitectura-de-Software/services/booking-service"

var forbiddenOutsideAdapters = []string{
	"github.com/gin-gonic/",
	"github.com/jackc/",
	"go.mongodb.org/",
	"gorm.io/",
	"net/http",
	"database/sql",
	module + "/internal/adapters",
}

func parseDir(t *testing.T, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("leer %s: %v", dir, err)
	}
	var files []*ast.File
	fset := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parsear %s: %v", name, err)
		}
		files = append(files, f)
	}
	return files
}

func imports(f *ast.File) []string {
	var out []string
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		out = append(out, p)
	}
	return out
}

func TestArquitectura_DomainSoloUsaBibliotecaEstandar(t *testing.T) {
	for _, f := range parseDir(t, ".") {
		for _, p := range imports(f) {
			first := strings.SplitN(p, "/", 2)[0]
			if strings.Contains(first, ".") {
				t.Errorf("domain no puede importar %q (solo biblioteca estándar)", p)
			}
			for _, bad := range []string{"net/http", "database/sql", "encoding/json"} {
				if p == bad {
					t.Errorf("domain no puede importar %q", p)
				}
			}
		}
	}
}

func TestArquitectura_DomainSinTagsDeSerializacion(t *testing.T) {
	for _, f := range parseDir(t, ".") {
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok {
				return true
			}
			for _, field := range st.Fields.List {
				if field.Tag != nil {
					t.Errorf("las structs de domain no llevan tags: %s", field.Tag.Value)
				}
			}
			return true
		})
	}
}

func TestArquitectura_ApplicationYPortsNoDependenDeAdaptadores(t *testing.T) {
	for _, dir := range []string{"../application", "../ports"} {
		for _, f := range parseDir(t, dir) {
			for _, p := range imports(f) {
				for _, bad := range forbiddenOutsideAdapters {
					if strings.HasPrefix(p, bad) {
						t.Errorf("%s no puede importar %q", dir, p)
					}
				}
			}
		}
	}
}
