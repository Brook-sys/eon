package dolt_test

import (
	"os"
	"path/filepath"
	"testing"

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	storage "motor-autonomo/internal/storage/dolt"
)

func TestStoreContract(t *testing.T) {
	binary := doltBinary(t)
	contract.TestStore(t, func() port.Store {
		store, err := storage.Open(binary, filepath.Join(t.TempDir(), "runtime"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Errorf("close dolt store: %v", err)
			}
		})
		return store
	})
}

func TestDurableStoreContract(t *testing.T) {
	binary := doltBinary(t)
	contract.TestDurableStore(t, func(t testing.TB) contract.DurableHarness {
		path := filepath.Join(t.TempDir(), "runtime")
		store, err := storage.Open(binary, path)
		if err != nil {
			t.Fatal(err)
		}
		return &harness{binary: binary, path: path, store: store}
	})
}

func doltBinary(t testing.TB) string {
	t.Helper()
	binary := os.Getenv("DOLT_BIN")
	if binary == "" {
		t.Skip("DOLT_BIN is not set; Dolt contract tests require an explicit binary")
	}
	return binary
}

type harness struct {
	binary string
	path   string
	store  *storage.Store
}

func (h *harness) Store() port.Store { return h.store }

func (h *harness) Restart() (port.Store, error) {
	if err := h.store.Close(); err != nil {
		return nil, err
	}
	store, err := storage.Open(h.binary, h.path)
	if err != nil {
		return nil, err
	}
	h.store = store
	return store, nil
}

func (h *harness) Close() error { return h.store.Close() }
