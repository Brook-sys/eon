// boundary checker — entrada CLI (D32/ADR-0002).
//
// Uso: go run ./tools/boundary [--root DIR] [--rules FILE]
// Exit 0 = fronteiras verdes; exit 1 = violações (o CI reprova).
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	root := flag.String("root", ".", "raiz do módulo")
	rulesPath := flag.String("rules", "tools/boundary/rules.json", "caminho do rules.json")
	flag.Parse()

	rules, err := LoadRules(*rulesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "boundary: %v\n", err)
		os.Exit(2)
	}

	violations, stats, err := Check(*root, rules)
	if err != nil {
		fmt.Fprintf(os.Stderr, "boundary: %v\n", err)
		os.Exit(2)
	}

	if len(violations) > 0 {
		fmt.Printf("boundary: %d VIOLAÇÕES\n", len(violations))
		for _, v := range violations {
			fmt.Println("  - " + v)
		}
		os.Exit(1)
	}

	fmt.Printf("boundaries OK: %d pacotes, %d arquivos, %d linhas de produção\n",
		stats.Packages, stats.Files, stats.Lines)
}
