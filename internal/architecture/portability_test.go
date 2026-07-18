package architecture

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func TestProductionDoesNotRequireCgo(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findCgoImports(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-PORT-001 cgo violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindCgoImportsIgnoresTestsAndReportsProduction(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("internal/adapter/native.go", "package adapter\nimport \"C\"\n")
	write("internal/adapter/native_test.go", "package adapter\nimport \"C\"\n")

	got, err := findCgoImports(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/adapter/native.go:2 imports C and requires cgo"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findCgoImports(root string) ([]string, error) {
	files := token.NewFileSet()
	var violations []string
	for _, scope := range []string{"cmd", "internal"} {
		directory := filepath.Join(root, scope)
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return nil, err
		}
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(files, path, nil, parser.ImportsOnly)
			if err != nil {
				return err
			}
			for _, imported := range parsed.Imports {
				value, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if value != "C" {
					continue
				}
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				position := files.Position(imported.Pos())
				violations = append(violations, fmt.Sprintf("%s:%d imports C and requires cgo", filepath.ToSlash(relative), position.Line))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(violations)
	return violations, nil
}
