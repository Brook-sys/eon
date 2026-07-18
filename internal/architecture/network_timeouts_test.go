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

func TestNetworkAdaptersDoNotUseUnboundedDefaultHTTPClient(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findDefaultHTTPClient(root, []string{"internal/provider", "internal/channel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-PERF-001 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindDefaultHTTPClientResolvesAliases(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceFixture(t, root, "internal/provider/unsafe.go", `package provider
import web "net/http"
var client = web.DefaultClient
`)
	writeSourceFixture(t, root, "internal/channel/safe.go", `package channel
import (
  "net/http"
  "time"
)
var client = &http.Client{Timeout: 30 * time.Second}
`)

	got, err := findDefaultHTTPClient(root, []string{"internal/provider", "internal/channel"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/provider/unsafe.go:3 uses http.DefaultClient without a request-wide timeout"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findDefaultHTTPClient(root string, scopes []string) ([]string, error) {
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
			httpAliases := make(map[string]bool)
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if importPath != "net/http" {
					continue
				}
				alias := "http"
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				if alias != "_" && alias != "." {
					httpAliases[alias] = true
				}
			}
			ast.Inspect(parsed, func(node ast.Node) bool {
				selector, ok := node.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "DefaultClient" {
					return true
				}
				identifier, ok := selector.X.(*ast.Ident)
				if !ok || !httpAliases[identifier.Name] {
					return true
				}
				position := files.Position(selector.Pos())
				relative, relErr := filepath.Rel(root, path)
				if relErr == nil {
					violations = append(violations, fmt.Sprintf("%s:%d uses http.DefaultClient without a request-wide timeout", filepath.ToSlash(relative), position.Line))
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
