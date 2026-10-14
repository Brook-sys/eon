// readonly_test.go — o contrato do leitor (eonctl, D15): mode=ro é
// recusa do DRIVER, não gentileza do chamador; banco ausente é erro
// honesto (leitor não cria mundo).
package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNewStoreReadonlyContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eon.db")
	s, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	u, err := s.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.KV().Set(u, "x", []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := u.Commit(); err != nil {
		t.Fatal(err)
	}
	s.Close()

	ro, err := NewStoreReadonly(path)
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer ro.Close()

	// leitura funciona
	if v, err := ro.KV().Get("x"); err != nil || string(v) != "1" {
		t.Fatalf("leitura ro: v=%q err=%v", v, err)
	}

	// escrita RECUSADA pelo driver — a lei do leitor
	u2, err := ro.Begin(ctx)
	if err != nil {
		t.Fatalf("begin em ro deveria abrir tx: %v", err)
	}
	if err := ro.KV().Set(u2, "x", []byte("2")); err == nil {
		_ = u2.Rollback()
		t.Fatal("ESCRITA ACEITA em conexão read-only — o driver devia recusar")
	}
	_ = u2.Rollback()

	// TaskCounts: GROUP BY no leitor
	counts, err := ro.TaskCounts("project")
	if err != nil {
		t.Fatalf("TaskCounts: %v", err)
	}
	if len(counts) != 0 {
		t.Fatalf("TaskCounts em banco vazio: %+v", counts)
	}

	// banco ausente: erro honesto, nada criado
	missing := filepath.Join(t.TempDir(), "nao-existe.db")
	if _, err := NewStoreReadonly(missing); err == nil {
		t.Fatal("open ro de banco ausente não falhou")
	}
}
