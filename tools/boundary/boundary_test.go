package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// O boundary checker é lei (Constituição Art. 1). Estes testes PROVAM que
// ele reprova: violação proposital reprova (aceite M0), módulo limpo passa.

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// goLines gera um arquivo Go válido com exatamente n linhas.
func goLines(pkg string, n int) string {
	var sb strings.Builder
	sb.WriteString("package " + pkg + "\n")
	for i := 2; i <= n; i++ {
		if i%20 == 0 {
			sb.WriteString("var _pad" + strconv.Itoa(i) + " = " + strconv.Itoa(i) + "\n")
		} else {
			sb.WriteString("// pad " + strconv.Itoa(i) + "\n")
		}
	}
	return sb.String()
}

func fixtureRules() *Rules {
	return &Rules{
		Module:          "example.com/fixture",
		MaxFileLines:    40,
		AllowedExternal: nil,
		Packages: map[string]Rule{
			"internal/a": {Imports: []string{"STDLIB"}, MaxLines: 50},
			"internal/b": {Imports: []string{"STDLIB"}, MaxLines: 50},
			"internal/d": {Imports: []string{"STDLIB"}, MaxLines: 50},
		},
	}
}

func hasViolation(violations []string, substr string) bool {
	for _, v := range violations {
		if strings.Contains(v, substr) {
			return true
		}
	}
	return false
}

// TestCheckCleanModulePasses: módulo dentro da lei → zero violações.
func TestCheckCleanModulePasses(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "internal", "a", "a.go"), goLines("a", 10))
	writeFile(t, filepath.Join(root, "internal", "b", "b.go"), goLines("b", 10))

	violations, stats, err := Check(root, fixtureRules())
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("módulo limpo não deveria ter violações: %v", violations)
	}
	if stats.Packages != 2 || stats.Files != 2 {
		t.Fatalf("stats inesperados: %+v", stats)
	}
}

// TestCheckDetectsDeliberateViolations (aceite M0): a violação proposital
// reprova — cada classe de crime tem que ser pega.
func TestCheckDetectsDeliberateViolations(t *testing.T) {
	root := t.TempDir()
	// fronteira furada: a importa b (b não está na allowlist de a)
	writeFile(t, filepath.Join(root, "internal", "a", "a.go"),
		"package a\n\nimport \"example.com/fixture/internal/b\"\n\nvar _ = b.X\n")
	// pacote não registrado
	writeFile(t, filepath.Join(root, "internal", "c", "c.go"), "package c\n")
	// dependência externa sem registro (D19)
	writeFile(t, filepath.Join(root, "internal", "d", "d.go"),
		"package d\n\nimport \"github.com/external/dep\"\n\nvar _ = dep.X\n")
	// arquivo gigante + estouro de orçamento de pacote (b: 10 + 45 = 55 > 50)
	writeFile(t, filepath.Join(root, "internal", "b", "b.go"), goLines("b", 10))
	writeFile(t, filepath.Join(root, "internal", "b", "big.go"), goLines("b", 45))

	violations, _, err := Check(root, fixtureRules())
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"import proibido \"example.com/fixture/internal/b\"",
		"pacote não registrado",
		"dependência externa não registrada",
		"arquivo com 45 linhas (teto 40)",
		"divida o pacote", // orçamento: b passou de 50
	} {
		if !hasViolation(violations, want) {
			t.Errorf("violação esperada não detectada: %q\ntodas: %v", want, violations)
		}
	}
}

// TestPrefixAllowlist: allowlist de pacote cobre subpacotes (fronteira de
// domínio: internal/domain vale para internal/domain/subpkg).
func TestPrefixAllowlist(t *testing.T) {
	root := t.TempDir()
	rules := &Rules{
		Module:       "example.com/fixture",
		MaxFileLines: 100,
		Packages: map[string]Rule{
			"internal/domain":        {Imports: []string{"STDLIB", "internal/domain"}, MaxLines: 100},
			"internal/domain/subpkg": {Imports: []string{"STDLIB"}, MaxLines: 100},
		},
	}
	writeFile(t, filepath.Join(root, "internal", "domain", "domain.go"),
		"package domain\n\nimport \"example.com/fixture/internal/domain/subpkg\"\n\nvar _ = subpkg.X\n")
	writeFile(t, filepath.Join(root, "internal", "domain", "subpkg", "sub.go"), "package subpkg\n")

	violations, _, err := Check(root, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("subpacote deveria ser permitido pela allowlist prefixo: %v", violations)
	}
}

// TestRealModuleIsCompliant: a ÁRVORE REAL do eon está dentro da própria lei.
// É o mesmo `go run ./tools/boundary` do CI — aqui como invariante de teste.
func TestRealModuleIsCompliant(t *testing.T) {
	root := "../.."
	rules, err := LoadRules(filepath.Join(root, "tools", "boundary", "rules.json"))
	if err != nil {
		t.Fatal(err)
	}
	violations, _, err := Check(root, rules)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("árvore real fora da lei:\n  - %s", strings.Join(violations, "\n  - "))
	}
}

// TestStdlibHeuristic: stdlib é stdlib; domínio é domínio.
func TestStdlibHeuristic(t *testing.T) {
	cases := map[string]bool{
		"fmt":           true,
		"os/exec":       true,
		"encoding/json": true,
		"github.com/brook-sys/eon/internal/domain": false,
		"golang.org/x/sys/unix":                    false,
	}
	for path, want := range cases {
		if got := isStdlib(path); got != want {
			t.Errorf("isStdlib(%q)=%v, queria %v", path, got, want)
		}
	}
}
