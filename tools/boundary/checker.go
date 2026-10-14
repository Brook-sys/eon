// checker.go — lógica pura do boundary checker, testável (D32/ADR-0002).
package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Rule é o teto de um pacote: allowlist de imports + orçamento de linhas.
type Rule struct {
	Imports  []string `json:"imports"`
	MaxLines int      `json:"max_lines"`
}

// Rules é o contrato declarativo inteiro (rules.json).
type Rules struct {
	Module          string          `json:"module"`
	MaxFileLines    int             `json:"max_file_lines"`
	AllowedExternal []string        `json:"allowed_external"`
	Packages        map[string]Rule `json:"packages"`
}

// Stats resume o que foi verificado.
type Stats struct {
	Packages int
	Files    int
	Lines    int
}

func LoadRules(path string) (*Rules, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r Rules
	if err := json.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("rules %s: %w", path, err)
	}
	if r.Module == "" {
		return nil, fmt.Errorf("rules %s: sem module", path)
	}
	return &r, nil
}

// Check percorra o módulo e devolve violações. Cada violação é uma linha
// pronta para log/CI. Sem violações = fronteiras verdes.
func Check(root string, rules *Rules) ([]string, Stats, error) {
	var violations []string
	var stats Stats

	pkgs, err := discoverPackages(root)
	if err != nil {
		return nil, stats, err
	}

	type pkgAcc struct {
		lines int
	}
	acc := map[string]*pkgAcc{}

	for _, pkg := range pkgs {
		rule, registered := rules.Packages[pkg.path]
		if !registered {
			violations = append(violations,
				fmt.Sprintf("%s: pacote não registrado em rules.json (toda fronteira é declarada — ADR-0002)", pkg.path))
			continue
		}

		a := &pkgAcc{}
		acc[pkg.path] = a
		stats.Packages++

		for _, f := range pkg.files {
			src, err := os.ReadFile(f.path)
			if err != nil {
				return nil, stats, err
			}
			n := countLines(string(src))
			stats.Files++
			if !strings.HasSuffix(f.path, "_test.go") {
				a.lines += n
				stats.Lines += n
			}
			if rules.MaxFileLines > 0 && n > rules.MaxFileLines {
				violations = append(violations,
					fmt.Sprintf("%s: arquivo com %d linhas (teto %d) — divida (Constituição Art. 2.4)",
						rel(root, f.path), n, rules.MaxFileLines))
			}

			// Imports (mesma regra para _test.go — testes não furam fronteiras).
			fset := token.NewFileSet()
			af, err := parser.ParseFile(fset, f.path, src, parser.ImportsOnly)
			if err != nil {
				return nil, stats, fmt.Errorf("parse %s: %w", f.path, err)
			}
			for _, imp := range af.Imports {
				ipath, err := strconv.Unquote(imp.Path.Value)
				if err != nil {
					continue
				}
				if v := checkImport(ipath, rule, rules); v != "" {
					violations = append(violations,
						fmt.Sprintf("%s: import proibido %q (%s)", rel(root, f.path), ipath, v))
				}
			}
		}
	}

	for pkgPath, a := range acc {
		rule := rules.Packages[pkgPath]
		if rule.MaxLines > 0 && a.lines > rule.MaxLines {
			violations = append(violations,
				fmt.Sprintf("%s: %d linhas de produção (orçamento %d) — divida o pacote (Constituição Art. 2.4)",
					pkgPath, a.lines, rule.MaxLines))
		}
	}

	sort.Strings(violations)
	return violations, stats, nil
}

// checkImport devolve "" se permitido, senão o motivo.
//
// Externa (ADR-0016) exige DUAS chaves: registro global em allowed_external
// (D19: dependência é ADR) E declaração na allowlist do PACOTE importador
// (storage é o único importador do driver SQLite, por exemplo). Uma chave só
// é violação — o mesmo import nunca "escapa" por registro global genérico.
func checkImport(ipath string, rule Rule, rules *Rules) string {
	registeredGlobally := false
	for _, g := range rules.AllowedExternal {
		if ipath == g || strings.HasPrefix(ipath, g+"/") {
			registeredGlobally = true
			break
		}
	}
	for _, allow := range rule.Imports {
		if allow == "*" {
			return ""
		}
		if allow == "STDLIB" && isStdlib(ipath) {
			return ""
		}
		if isModulePath(ipath, rules.Module, allow) {
			return ""
		}
		// Entrada externa na allowlist do pacote: só vale registrada no
		// ADR (allowed_external). As duas chaves, nunca uma.
		if !isStdlib(allow) && allow != rules.Module && !strings.HasPrefix(allow, rules.Module+"/") {
			if ipath == allow || strings.HasPrefix(ipath, allow+"/") {
				if registeredGlobally {
					return ""
				}
				return "externa na allowlist do pacote mas ausente de allowed_external (D19: registro é ADR)"
			}
		}
	}
	// Classifica para a mensagem.
	if isStdlib(ipath) {
		return "não está na allowlist do pacote"
	}
	if strings.HasPrefix(ipath, rules.Module+"/") || ipath == rules.Module {
		return "não está na allowlist do pacote (fronteiras de import, ADR-0002)"
	}
	if registeredGlobally {
		return "registrada em allowed_external mas fora da allowlist do PACOTE (ADR-0016: o importador é declarado)"
	}
	return "dependência externa não registrada em allowed_external (D19: dependência é ADR, não acidente)"
}

// isStdlib: primeiro segmento sem ponto (heurística padrão de ferramenta Go).
func isStdlib(ipath string) bool {
	first := ipath
	if i := strings.IndexByte(ipath, '/'); i >= 0 {
		first = ipath[:i]
	}
	return !strings.Contains(first, ".")
}

// isModulePath: ipath == allow ou allow é prefixo de diretório de ipath.
func isModulePath(ipath, modulePrefix, allow string) bool {
	if allow == "STDLIB" || allow == "*" {
		return false
	}
	full := modulePrefix
	if allow != "" {
		full = modulePrefix + "/" + strings.TrimPrefix(allow, modulePrefix+"/")
	}
	if ipath == full {
		return true
	}
	return strings.HasPrefix(ipath, full+"/")
}

type pkgFiles struct {
	path  string // caminho do pacote relativo ao root, ex.: internal/domain
	files []struct{ path string }
}

// discoverPackages percorra cmd/, internal/ e tools/ do root e agrupa arquivos
// .go por diretório (um diretório = um pacote; simplificação válida para este
// módulo — sem _test only dirs, sem multi-package dirs).
func discoverPackages(root string) ([]pkgFiles, error) {
	dirs := []string{"cmd", "internal", "tools"}
	var found []pkgFiles
	for _, d := range dirs {
		base := filepath.Join(root, d)
		if _, err := os.Stat(base); err != nil {
			continue
		}
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				switch entry.Name() {
				case "testdata", "vendor", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			relPath, err := filepath.Rel(root, filepath.Dir(path))
			if err != nil {
				return err
			}
			pkgPath := filepath.ToSlash(relPath)
			var pf *pkgFiles
			for i := range found {
				if found[i].path == pkgPath {
					pf = &found[i]
					break
				}
			}
			if pf == nil {
				found = append(found, pkgFiles{path: pkgPath})
				pf = &found[len(found)-1]
			}
			pf.files = append(pf.files, struct{ path string }{path})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return found, nil
}

func countLines(s string) int {
	n := strings.Count(s, "\n")
	if len(s) > 0 && !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(r)
}
