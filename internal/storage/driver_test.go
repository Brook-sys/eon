package storage

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// Os testes deste pacote provam o que o EON USA (lição M0: probe prova o
// uso, não o correlato): WAL persistente, FK por conexão, FTS5 (D44) e
// transação que aplica/relata inteira (ChangeSet, D03).

func TestOpenEONMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eon.db")

	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var journal string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("journal_mode=%q, queria wal", journal)
	}
	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil {
		t.Fatal(err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys=%d, queria 1", fk)
	}
	db.Close()

	// WAL é propriedade do ARQUIVO: reopen mantém.
	db2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	if err := db2.QueryRow("PRAGMA journal_mode").Scan(&journal); err != nil {
		t.Fatal(err)
	}
	if journal != "wal" {
		t.Fatalf("reopen: journal_mode=%q, queria wal", journal)
	}
}

// TestFTS5Available (D44): relevância BM25 é requisito do ContextCompiler
// desde o M2 — se o driver não trouxesse FTS5, é AGORA que precisa saber.
func TestFTS5Available(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "eon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE VIRTUAL TABLE mem USING fts5(scope, body)`); err != nil {
		t.Fatalf("FTS5 indisponível (D44): %v", err)
	}
	for _, row := range []struct{ scope, body string }{
		{"f2", "o autômato nunca para"},
		{"f2", "serialidade cognitiva é lei"},
	} {
		if _, err := db.Exec(`INSERT INTO mem VALUES (?, ?)`, row.scope, row.body); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := db.Query(`SELECT body, bm25(mem) FROM mem WHERE mem MATCH ? ORDER BY rank LIMIT 1`, `serialidade`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("MATCH sem resultado")
	}
	var body string
	var score float64
	if err := rows.Scan(&body, &score); err != nil {
		t.Fatal(err)
	}
	if body != "serialidade cognitiva é lei" {
		t.Fatalf("bm25 ranqueou %q, queria a linha certa", body)
	}
}

// TestTransactionAllOrNothing: ChangeSet aplica inteiro ou rejeita inteiro
// (D03) — a semântica de transação do driver tem que ser a de sempre.
func TestTransactionAllOrNothing(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "eon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE probe (id TEXT PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}

	// Rollback descarta o lote inteiro.
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO probe VALUES ('a')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, 0)

	// Commit aplica o lote inteiro.
	tx, err = db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO probe VALUES ('b')`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO probe VALUES ('c')`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	assertCount(t, db, 2)
}

func assertCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM probe`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != want {
		t.Fatalf("count=%d, queria %d", n, want)
	}
}
