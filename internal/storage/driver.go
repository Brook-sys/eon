// driver.go — o banco no modo EON: WAL, foreign_keys, busy_timeout curto
// (D03/ADR-0016). Storage é o ÚNICO importador do driver puro-Go.
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registra o driver puro-Go (ADR-0016); storage é o único importador
)

// dsnPragmas: aplicados PELO DRIVER em cada conexão que o pool do
// database/sql abrir. PRAGMA via Exec valeria só para a primeira conexão —
// bug clássico de pooling; aqui é correto por construção.
//
//	busy_timeout 5s: commit é serial (D10); contenção real é bug, não estado.
//	foreign_keys: integridade pertence ao schema, não ao chamador (D03).
//	journal_mode WAL: leitores (eonctl) nunca bloqueiam o kernel.
//	synchronous NORMAL: durável o suficiente com WAL — o custo FULL não
//	 compra nada que o crash-replay não cubra melhor.
func dsn(path string) string {
	return "file:" + filepath.ToSlash(path) +
		"?_pragma=busy_timeout(5000)" +
		"&_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)"
}

// Open abre (criando se ausente) o banco canônico no modo EON. Boot
// honesto: os dois pragmas que são LEI (WAL, foreign_keys) são verificados
// na hora — falha de boot é barulhenta, nunca silenciosa.
func Open(path string) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("storage: criar %s: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite", dsn(path))
	if err != nil {
		return nil, fmt.Errorf("storage: abrir %s: %w", path, err)
	}

	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: journal_mode: %w", err)
	}
	if !strings.EqualFold(journal, "wal") {
		db.Close()
		return nil, fmt.Errorf("storage: journal_mode=%q, queria WAL (D03)", journal)
	}
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: foreign_keys: %w", err)
	}
	if fk != 1 {
		db.Close()
		return nil, fmt.Errorf("storage: foreign_keys=%d, queria 1 (D03)", fk)
	}
	return db, nil
}
