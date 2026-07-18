package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type storeContractExpectation struct {
	adapter string
	calls   []string
}

func TestConcreteStoresRunReusableContracts(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	expectations := []storeContractExpectation{
		{adapter: "memory", calls: []string{"TestStore"}},
		{adapter: "sqlite", calls: []string{"TestDurableStore", "TestStore"}},
		{adapter: "dolt", calls: []string{"TestDurableStore", "TestStore"}},
	}

	for _, expectation := range expectations {
		expectation := expectation
		t.Run(expectation.adapter, func(t *testing.T) {
			t.Parallel()
			directory := filepath.Join(root, "internal", "storage", expectation.adapter)
			calls, err := reusableContractCalls(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, required := range expectation.calls {
				if !calls[required] {
					t.Errorf("internal/storage/%s does not invoke contract.%s", expectation.adapter, required)
				}
			}
		})
	}
}

func TestReusableContractCallsIgnoreUnrelatedSelectors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	body := `package fixture
import (
	contract "motor-autonomo/internal/storage/contract"
	other "example.test/contract"
	"testing"
)
func TestContracts(t *testing.T) {
	contract.TestStore(t, nil)
	other.TestDurableStore(t, nil)
}
`
	if err := os.WriteFile(filepath.Join(root, "store_test.go"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	calls, err := reusableContractCalls(root)
	if err != nil {
		t.Fatal(err)
	}
	if !calls["TestStore"] {
		t.Fatal("contract.TestStore was not detected")
	}
	if calls["TestDurableStore"] {
		t.Fatal("selector from unrelated package was treated as a storage contract")
	}
}

func reusableContractCalls(directory string) (map[string]bool, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	files := token.NewFileSet()
	calls := make(map[string]bool)
	var testFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".go" && len(entry.Name()) > len("_test.go") && entry.Name()[len(entry.Name())-len("_test.go"):] == "_test.go" {
			testFiles = append(testFiles, entry.Name())
		}
	}
	sort.Strings(testFiles)
	for _, name := range testFiles {
		path := filepath.Join(directory, name)
		parsed, err := parser.ParseFile(files, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil, err
		}
		aliases := make(map[string]bool)
		for _, imported := range parsed.Imports {
			if imported.Path.Value != `"motor-autonomo/internal/storage/contract"` {
				continue
			}
			alias := "contract"
			if imported.Name != nil {
				alias = imported.Name.Name
			}
			aliases[alias] = true
		}
		if len(aliases) == 0 {
			continue
		}
		parsed, err = parser.ParseFile(files, path, nil, 0)
		if err != nil {
			return nil, err
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && aliases[identifier.Name] {
				calls[selector.Sel.Name] = true
			}
			return true
		})
	}
	if len(testFiles) == 0 {
		return nil, fmt.Errorf("no test files under %s", directory)
	}
	return calls, nil
}
