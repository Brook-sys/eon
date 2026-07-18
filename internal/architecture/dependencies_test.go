package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var forbiddenCoreImportPrefixes = []string{
	"motor-autonomo/internal/provider/",
	"motor-autonomo/internal/storage/",
}

var forbiddenInspectStoreSelectors = map[string]bool{
	"Store":       true,
	"Transaction": true,
}

func TestCoreDoesNotImportConcreteAdapters(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findForbiddenImports(root, []string{"internal/domain", "internal/kernel"}, forbiddenCoreImportPrefixes)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-MOD-001 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindForbiddenImportsReportsProviderAndStorage(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture("internal/domain/domain.go", `package domain
import _ "motor-autonomo/internal/provider/openai"
`)
	writeFixture("internal/kernel/kernel.go", `package kernel
import _ "motor-autonomo/internal/storage/sqlite"
`)
	writeFixture("internal/kernel/kernel_test.go", `package kernel
import _ "motor-autonomo/internal/provider/openai/fakeserver"
`)

	got, err := findForbiddenImports(root, []string{"internal/domain", "internal/kernel"}, forbiddenCoreImportPrefixes)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/domain/domain.go:2 imports concrete adapter motor-autonomo/internal/provider/openai",
		"internal/kernel/kernel.go:2 imports concrete adapter motor-autonomo/internal/storage/sqlite",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func TestInspectDependsOnlyOnReadStore(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findForbiddenPortSelectors(root, "internal/inspect", forbiddenInspectStoreSelectors)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("FR-CTRL-007 read-only boundary violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindForbiddenPortSelectorsResolvesAliases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFixture := func(path, body string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeFixture("internal/inspect/bad.go", `package inspect
import boundary "motor-autonomo/internal/port"
type bad struct { store boundary.Store; tx boundary.Transaction }
`)
	got, err := findForbiddenPortSelectors(root, "internal/inspect", forbiddenInspectStoreSelectors)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/inspect/bad.go:3 references write-capable port.Store",
		"internal/inspect/bad.go:3 references write-capable port.Transaction",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findForbiddenPortSelectors(root, scope string, forbidden map[string]bool) ([]string, error) {
	files := token.NewFileSet()
	var violations []string
	directory := filepath.Join(root, filepath.FromSlash(scope))
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
		aliases := make(map[string]bool)
		for _, imported := range parsed.Imports {
			value, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			if value != "motor-autonomo/internal/port" {
				continue
			}
			alias := "port"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			if alias != "_" && alias != "." {
				aliases[alias] = true
			}
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok || !forbidden[selector.Sel.Name] {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if !ok || !aliases[identifier.Name] {
				return true
			}
			position := files.Position(selector.Pos())
			relative, err := filepath.Rel(root, path)
			if err == nil {
				violations = append(violations, fmt.Sprintf("%s:%d references write-capable port.%s", filepath.ToSlash(relative), position.Line, selector.Sel.Name))
			}
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(violations)
	return violations, nil
}

func findForbiddenImports(root string, scopes, forbiddenPrefixes []string) ([]string, error) {
	var violations []string
	files := token.NewFileSet()
	for _, scope := range scopes {
		directory := filepath.Join(root, filepath.FromSlash(scope))
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
				for _, prefix := range forbiddenPrefixes {
					if value == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(value, prefix) {
						position := files.Position(imported.Pos())
						relative, err := filepath.Rel(root, path)
						if err != nil {
							return err
						}
						violations = append(violations, fmt.Sprintf("%s:%d imports concrete adapter %s", filepath.ToSlash(relative), position.Line, value))
					}
				}
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

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("locate module root: %v", err)
	}
	return root
}
