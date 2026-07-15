package dolt_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	storage "motor-autonomo/internal/storage/dolt"
)

func TestServerStoreContract(t *testing.T) {
	binary := doltBinary(t)
	contract.TestStore(t, func() port.Store {
		store, err := storage.OpenServer(binary, filepath.Join(t.TempDir(), "runtime"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Errorf("close dolt server store: %v", err)
			}
		})
		return store
	})
}

func TestServerStoreDurableContract(t *testing.T) {
	binary := doltBinary(t)
	contract.TestDurableStore(t, func(t testing.TB) contract.DurableHarness {
		path := filepath.Join(t.TempDir(), "runtime")
		store, err := storage.OpenServer(binary, path)
		if err != nil {
			t.Fatal(err)
		}
		return &serverHarness{binary: binary, path: path, store: store}
	})
}

type serverHarness struct {
	binary string
	path   string
	store  *storage.ServerStore
}

func (h *serverHarness) Store() port.Store { return h.store }

func (h *serverHarness) Restart() (port.Store, error) {
	if err := h.store.Close(); err != nil {
		return nil, err
	}
	store, err := storage.OpenServer(h.binary, h.path)
	if err != nil {
		return nil, err
	}
	h.store = store
	return store, nil
}

func (h *serverHarness) Close() error { return h.store.Close() }

func TestServerStoreSeparatesSQLAndDoltCommitBoundaries(t *testing.T) {
	binary := doltBinary(t)
	var observed []storage.ServerFailpoint
	store, err := storage.OpenServerWithOptions(binary, filepath.Join(t.TempDir(), "runtime"), storage.ServerOptions{Failpoint: func(point storage.ServerFailpoint) {
		observed = append(observed, point)
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(context.Background(), func(tx port.Transaction) error {
		_, err := tx.AppendEvent(domain.Event{SchemaVersion: domain.SchemaVersionV1, ID: "event-server-boundary", Kind: "test.server.boundary", OccurredAt: time.Unix(1, 0).UTC()})
		return err
	}); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	want := []storage.ServerFailpoint{storage.FailpointBeforeSQLCommit, storage.FailpointAfterSQLCommit, storage.FailpointAfterDoltCommit}
	if len(observed) != len(want) {
		t.Fatalf("observed failpoints = %v, want %v", observed, want)
	}
	for index := range want {
		if observed[index] != want[index] {
			t.Fatalf("observed failpoints = %v, want %v", observed, want)
		}
	}
}
