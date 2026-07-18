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

var forbiddenOfficialSources = map[string]map[string]string{
	"time": {
		"After":     "wall-clock wait",
		"Now":       "wall clock",
		"NewTicker": "wall-clock ticker",
		"Sleep":     "wall-clock sleep",
		"Since":     "wall-clock elapsed time",
		"Tick":      "wall-clock ticker",
		"Until":     "wall-clock elapsed time",
	},
	"math/rand": {
		"Float32":     "unregistered randomness",
		"Float64":     "unregistered randomness",
		"Int":         "unregistered randomness",
		"Int31":       "unregistered randomness",
		"Int31n":      "unregistered randomness",
		"Int63":       "unregistered randomness",
		"Int63n":      "unregistered randomness",
		"Intn":        "unregistered randomness",
		"NormFloat64": "unregistered randomness",
		"Perm":        "unregistered randomness",
		"Read":        "unregistered randomness",
		"Shuffle":     "unregistered randomness",
		"Uint32":      "unregistered randomness",
		"Uint64":      "unregistered randomness",
	},
	"crypto/rand": {
		"Int":  "unregistered randomness",
		"Read": "unregistered randomness",
	},
}

func TestOfficialCoreUsesInjectedTimeAndRandomness(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findForbiddenOfficialSources(root, []string{"internal/domain", "internal/kernel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("INV-DUR-006 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindForbiddenOfficialSourcesResolvesAliasesAndIgnoresTests(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceFixture(t, root, "internal/domain/domain.go", `package domain
import clock "time"
func observedAt() { _ = clock.Now() }
`)
	writeSourceFixture(t, root, "internal/kernel/kernel.go", `package kernel
import (
  random "math/rand"
  secure "crypto/rand"
)
func choose() { _ = random.Intn(2); _, _ = secure.Read(nil) }
`)
	writeSourceFixture(t, root, "internal/kernel/kernel_test.go", `package kernel
import "time"
func waitInTest() { time.Sleep(1) }
`)

	got, err := findForbiddenOfficialSources(root, []string{"internal/domain", "internal/kernel"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"internal/domain/domain.go:3 uses wall clock via time.Now; inject port.Clock",
		"internal/kernel/kernel.go:6 uses unregistered randomness via crypto/rand.Read; inject port.RandomSource",
		"internal/kernel/kernel.go:6 uses unregistered randomness via math/rand.Intn; inject port.RandomSource",
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func writeSourceFixture(t *testing.T, root, path, body string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func findForbiddenOfficialSources(root string, scopes []string) ([]string, error) {
	files := token.NewFileSet()
	var violations []string
	for _, scope := range scopes {
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
			aliases := make(map[string]string)
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if _, tracked := forbiddenOfficialSources[importPath]; !tracked {
					continue
				}
				alias := filepath.Base(importPath)
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				if alias != "_" && alias != "." {
					aliases[alias] = importPath
				}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if !ok {
					return true
				}
				importPath, tracked := aliases[identifier.Name]
				if !tracked {
					return true
				}
				reason, forbidden := forbiddenOfficialSources[importPath][selector.Sel.Name]
				if !forbidden {
					return true
				}
				position := files.Position(selector.Pos())
				relative, err := filepath.Rel(root, path)
				if err != nil {
					return false
				}
				injection := "port.Clock"
				if importPath != "time" {
					injection = "port.RandomSource"
				}
				violations = append(violations, fmt.Sprintf("%s:%d uses %s via %s.%s; inject %s", filepath.ToSlash(relative), position.Line, reason, importPath, selector.Sel.Name, injection))
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
