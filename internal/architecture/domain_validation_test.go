package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestVersionedDomainTypesHaveValidationEntrypoint(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findVersionedTypesWithoutValidation(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-EVOL-001 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindVersionedTypesWithoutValidationReportsMissingMethod(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceFixture(t, root, "internal/domain/versioned.go", `package domain
type Missing struct { SchemaVersion int }
type Valid struct { SchemaVersion int }
func (Valid) Validate() error { return nil }
type Unversioned struct { Value string }
`)
	got, err := findVersionedTypesWithoutValidation(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/domain/versioned.go:2 versioned domain type Missing has no Validate*() error method"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findVersionedTypesWithoutValidation(root string) ([]string, error) {
	directory := filepath.Join(root, "internal", "domain")
	files := token.NewFileSet()
	versioned := make(map[string]string)
	validated := make(map[string]bool)

	err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return err
		}
		for _, declaration := range parsed.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					typeSpec, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					structType, ok := typeSpec.Type.(*ast.StructType)
					if !ok || !hasField(structType, "SchemaVersion") {
						continue
					}
					relative, err := filepath.Rel(root, path)
					if err != nil {
						return err
					}
					versioned[typeSpec.Name.Name] = fmt.Sprintf("%s:%d", filepath.ToSlash(relative), files.Position(typeSpec.Pos()).Line)
				}
			case *ast.FuncDecl:
				name, ok := validationReceiver(declaration)
				if ok {
					validated[name] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var violations []string
	for name, location := range versioned {
		if !validated[name] {
			violations = append(violations, fmt.Sprintf("%s versioned domain type %s has no Validate*() error method", location, name))
		}
	}
	sort.Strings(violations)
	return violations, nil
}

func hasField(structType *ast.StructType, name string) bool {
	for _, field := range structType.Fields.List {
		for _, fieldName := range field.Names {
			if fieldName.Name == name {
				return true
			}
		}
	}
	return false
}

func validationReceiver(declaration *ast.FuncDecl) (string, bool) {
	if !strings.HasPrefix(declaration.Name.Name, "Validate") || declaration.Recv == nil || len(declaration.Recv.List) != 1 || declaration.Type.Params.NumFields() != 0 || declaration.Type.Results.NumFields() != 1 {
		return "", false
	}
	result, ok := declaration.Type.Results.List[0].Type.(*ast.Ident)
	if !ok || result.Name != "error" {
		return "", false
	}
	receiver := declaration.Recv.List[0].Type
	if pointer, ok := receiver.(*ast.StarExpr); ok {
		receiver = pointer.X
	}
	identifier, ok := receiver.(*ast.Ident)
	if !ok {
		return "", false
	}
	return identifier.Name, true
}
