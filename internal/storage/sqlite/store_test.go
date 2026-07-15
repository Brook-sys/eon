package sqlite_test

import (
	"path/filepath"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	storage "motor-autonomo/internal/storage/sqlite"
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
