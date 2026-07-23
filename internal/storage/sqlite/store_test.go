package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/gob"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/contract"
	"motor-autonomo/internal/storage/memory"
	storage "motor-autonomo/internal/storage/sqlite"

	_ "modernc.org/sqlite"
)

func TestUpdateTimingSeparatesCallbackFromSQLitePhases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	var observed []storage.UpdateTiming
	store, err := storage.OpenWithOptions(path, storage.Options{ObserveUpdate: func(timing storage.UpdateTiming) {
		observed = append(observed, timing)
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.Update(context.Background(), func(port.Transaction) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(observed) != 1 {
		t.Fatalf("observations=%d want=1", len(observed))
	}
	got := observed[0]
	if got.Callback < 5*time.Millisecond {
		t.Fatalf("callback duration=%s want >=5ms", got.Callback)
	}
	if got.Begin <= 0 || got.WriteCAS <= 0 || got.Commit <= 0 {
		t.Fatalf("SQLite phase timing incomplete: %+v", got)
	}
	if got.ConflictReload != 0 {
		t.Fatalf("successful update reloaded conflict state: %+v", got)
	}
}

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

func TestModelCompletionReceiptSurvivesSQLiteRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	result := domain.ModelCompletionResult{Text: "answer", InputTokens: 3, OutputTokens: 2, Model: "m", FinishReason: "stop"}
	hash, err := result.Hash()
	if err != nil {
		t.Fatal(err)
	}
	receipt := domain.ModelCompletionReceipt{SchemaVersion: 1, OperationID: "op", Attempt: 4, ModelCall: 2, Result: result, PayloadHash: hash, RecordedAt: time.Date(2026, 7, 22, 19, 0, 0, 0, time.UTC)}
	if err := store.Update(t.Context(), func(tx port.Transaction) error { return tx.AppendModelCompletionReceipt(receipt) }); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.View(t.Context(), func(r port.Reader) error {
		got, err := r.ModelCompletionReceipt("op", 4, 2)
		if err != nil {
			return err
		}
		if got.PayloadHash != hash || got.Result.Text != "answer" {
			t.Fatalf("receipt after restart = %#v", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
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

func TestOpenMigratesV1ExternalVersionOnNextWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.sqlite")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	var legacyPayload bytes.Buffer
	if err := gob.NewEncoder(&legacyPayload).Encode(struct {
		FormatVersion int
		State         struct{}
	}{FormatVersion: 1}); err != nil {
		t.Fatal(err)
	}
	// A pre-v2 database may carry table-level format version 1. Its payload is
	// accepted by the decoder compatibility path and rewritten as v2 after the
	// next successful durable update.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO runtime_checkpoint(id, format_version, payload) VALUES(1, 1, ?)`, legacyPayload.Bytes()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(path)
	if err != nil {
		t.Fatalf("open v1 external checkpoint: %v", err)
	}
	if err := store.Update(t.Context(), func(port.Transaction) error { return nil }); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	var payload []byte
	if err := db.QueryRow(`SELECT format_version, payload FROM runtime_checkpoint WHERE id = 1`).Scan(&version, &payload); err != nil {
		t.Fatal(err)
	}
	if version != memory.CheckpointFormatVersion {
		t.Fatalf("external version = %d, want %d", version, memory.CheckpointFormatVersion)
	}
	// The rewritten payload must be a decodable envelope, not merely a bumped
	// table column. Decode generically here to avoid coupling to private fields.
	var envelope struct {
		FormatVersion int
	}
	if err := gob.NewDecoder(bytes.NewReader(payload)).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.FormatVersion != memory.CheckpointFormatVersion {
		t.Fatalf("envelope version = %d, want %d", envelope.FormatVersion, memory.CheckpointFormatVersion)
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
