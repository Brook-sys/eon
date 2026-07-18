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

func TestNetworkAdaptersBoundReadAllBuffers(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	violations, err := findUnboundedReadAll(root, []string{"internal/provider", "internal/channel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("NFR-PERF-001 violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestFindUnboundedReadAllResolvesAliasesAndLocalLimiters(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeSourceFixture(t, root, "internal/provider/unsafe.go", `package provider
import stream "io"
func decode(r stream.Reader) { _, _ = stream.ReadAll(r) }
`)
	writeSourceFixture(t, root, "internal/provider/safe.go", `package provider
import stream "io"
func direct(r stream.Reader) { _, _ = stream.ReadAll(stream.LimitReader(r, 1024)) }
func local(r stream.Reader) {
  limited := stream.LimitReader(r, 1024)
  _, _ = stream.ReadAll(limited)
}
`)
	writeSourceFixture(t, root, "internal/channel/safe.go", `package channel
import "io"
func decode(r io.Reader, maxResponseBytes int64) {
  _, _ = io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
}
`)

	got, err := findUnboundedReadAll(root, []string{"internal/provider", "internal/channel"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/provider/unsafe.go:3 reads an unbounded buffer via io.ReadAll; wrap the reader with io.LimitReader"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
}

func findUnboundedReadAll(root string, scopes []string) ([]string, error) {
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
			ioAliases := make(map[string]bool)
			for _, imported := range parsed.Imports {
				importPath, err := strconv.Unquote(imported.Path.Value)
				if err != nil {
					return err
				}
				if importPath != "io" {
					continue
				}
				alias := "io"
				if imported.Name != nil {
					alias = imported.Name.Name
				}
				if alias != "_" && alias != "." {
					ioAliases[alias] = true
				}
			}

			for _, declaration := range parsed.Decls {
				function, ok := declaration.(*ast.FuncDecl)
				if !ok || function.Body == nil {
					continue
				}
				limitedVariables := localLimitedReaders(function.Body, ioAliases)
				ast.Inspect(function.Body, func(node ast.Node) bool {
					call, ok := node.(*ast.CallExpr)
					if !ok || !isIOCall(call, ioAliases, "ReadAll") || len(call.Args) != 1 {
						return true
					}
					if isLimitedReaderExpression(call.Args[0], ioAliases, limitedVariables) {
						return true
					}
					position := files.Position(call.Pos())
					relative, relErr := filepath.Rel(root, path)
					if relErr == nil {
						violations = append(violations, fmt.Sprintf("%s:%d reads an unbounded buffer via io.ReadAll; wrap the reader with io.LimitReader", filepath.ToSlash(relative), position.Line))
					}
					return true
				})
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

func localLimitedReaders(body *ast.BlockStmt, ioAliases map[string]bool) map[string]bool {
	limited := make(map[string]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for index, right := range statement.Rhs {
				if index >= len(statement.Lhs) || !isLimitedReaderExpression(right, ioAliases, limited) {
					continue
				}
				if identifier, ok := statement.Lhs[index].(*ast.Ident); ok {
					limited[identifier.Name] = true
				}
			}
		case *ast.ValueSpec:
			for index, value := range statement.Values {
				if index >= len(statement.Names) || !isLimitedReaderExpression(value, ioAliases, limited) {
					continue
				}
				limited[statement.Names[index].Name] = true
			}
		}
		return true
	})
	return limited
}

func isLimitedReaderExpression(expression ast.Expr, ioAliases map[string]bool, limitedVariables map[string]bool) bool {
	switch value := expression.(type) {
	case *ast.CallExpr:
		return isIOCall(value, ioAliases, "LimitReader")
	case *ast.Ident:
		return limitedVariables[value.Name]
	case *ast.ParenExpr:
		return isLimitedReaderExpression(value.X, ioAliases, limitedVariables)
	default:
		return false
	}
}

func isIOCall(call *ast.CallExpr, ioAliases map[string]bool, name string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	return ok && ioAliases[identifier.Name]
}
