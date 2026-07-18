package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

var forbiddenTestSources = map[string]map[string]string{
	"time": {
		"After":     "wall-clock wait",
		"Now":       "wall clock",
		"NewTicker": "wall-clock ticker",
		"NewTimer":  "wall-clock timer",
		"Sleep":     "wall-clock sleep",
		"Since":     "wall-clock elapsed time",
		"Tick":      "wall-clock ticker",
		"Until":     "wall-clock elapsed time",
	},
}

func TestCoreTestsAreOfflineAndDeterministic(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findForbiddenTestSources(root, []string{"internal/domain", "internal/kernel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-TEST-001 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindForbiddenTestSourcesResolvesAliasesAndDetectsViolations(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceFixture(t, root, "internal/domain/domain_test.go", `package domain
import clock "time"
func TestSomething(t *testing.T) { _ = clock.Now() }
`)
	writeSourceFixture(t, root, "internal/kernel/kernel_test.go", `package kernel
import (
  "net/http"
  "os/exec"
)
func TestSomething(t *testing.T) { _ = exec.Command("echo"); _, _ = http.Get("https://example.invalid") }
`)
	writeSourceFixture(t, root, "internal/kernel/kernel.go", `package kernel
import "time"
func waitInProd() { time.Sleep(1) } // Ignored by this specific test because it's not a _test.go file
`)

	got, err := findForbiddenTestSources(root, []string{"internal/domain", "internal/kernel"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/domain/domain_test.go:3 uses wall clock via time.Now",
		"internal/kernel/kernel_test.go:3 imports direct network facility net/http",
		"internal/kernel/kernel_test.go:6 uses external process via os/exec",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findForbiddenTestSources(root string, scopes []string) ([]string, error) {
	files := token.NewFileSet()
	var violations []string
	for _, scope := range scopes {
		directory := filepath.Join(root, filepath.FromSlash(scope))
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), "_test.go") {
				return nil
			}
			parsed, err := parser.ParseFile(files, path, nil, 0)
			if err != nil {
				return err
			}
			aliases := make(map[string]string)
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				alias := filepath.Base(importPath)
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				if alias != "_" && alias != "." {
					aliases[alias] = importPath
				}
				if importPath == "net" || importPath == "net/http" {
					position := files.Position(imported.Pos())
					relative, relErr := filepath.Rel(root, path)
					if relErr == nil {
						violations = append(violations, fmt.Sprintf("%s:%d imports direct network facility %s", filepath.ToSlash(relative), position.Line, importPath))
					}
				}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.SelectorExpr:
					identifier, ok := n.X.(*ast.Ident)
					if !ok {
						return true
					}
					importPath, tracked := aliases[identifier.Name]
					if !tracked {
						return true
					}

					if importPath == "os/exec" {
						position := files.Position(n.Pos())
						relative, err := filepath.Rel(root, path)
						if err == nil {
							violations = append(violations, fmt.Sprintf("%s:%d uses external process via os/exec", filepath.ToSlash(relative), position.Line))
						}
						return true
					}

					reason, forbidden := forbiddenTestSources[importPath][n.Sel.Name]
					if !forbidden {
						return true
					}
					position := files.Position(n.Pos())
					relative, err := filepath.Rel(root, path)
					if err == nil {
						violations = append(violations, fmt.Sprintf("%s:%d uses %s via %s.%s", filepath.ToSlash(relative), position.Line, reason, importPath, n.Sel.Name))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(violations)
	return violations, nil
}
