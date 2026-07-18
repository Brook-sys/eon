package sqlite_test

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	"motor-autonomo/internal/storage/memory"
	storage "motor-autonomo/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

func TestStoreContract(t *testing.T) {
	contract.TestStore(t, func() port.Store {
		store, err := storage.Open(filepath.Join(t.TempDir(), "runtime.sqlite"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Errorf("close sqlite store: %v", err)
			}
		})
		return store
	})
}

func TestDurableStoreContract(t *testing.T) {
	contract.TestDurableStore(t, func(t testing.TB) contract.DurableHarness {
		path := filepath.Join(t.TempDir(), "runtime.sqlite")
		store, err := storage.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		return &harness{t: t, path: path, store: store}
	})
}

func TestOpenRejectsUnsupportedCheckpointFormatBeforeDecodingPayload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_checkpoint(id, format_version, payload) VALUES(1, ?, X'00')`, memory.CheckpointFormatVersion+1); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = storage.Open(path)
	if !errors.Is(err, memory.ErrUnsupportedCheckpointFormat) {
		t.Fatalf("open error = %v, want unsupported checkpoint format", err)
	}
}

func TestOpenRejectsCorruptCurrentCheckpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_checkpoint(id, format_version, payload) VALUES(1, ?, X'00')`, memory.CheckpointFormatVersion); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Open(path); err == nil || errors.Is(err, memory.ErrUnsupportedCheckpointFormat) {
		t.Fatalf("open error = %v, want checkpoint decode failure", err)
	}
}

type harness struct {
	t     testing.TB
	path  string
	store *storage.Store
}

func (h *harness) Store() port.Store { return h.store }

func (h *harness) Restart() (port.Store, error) {
	if err := h.store.Close(); err != nil {
		return nil, err
	}
	store, err := storage.Open(h.path)
	if err != nil {
		return nil, err
	}
	h.store = store
	return store, nil
}

func (h *harness) Close() error { return h.store.Close() }
