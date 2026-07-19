// Package dolt implements a durable Dolt checkpoint adapter for the
// backend-neutral Store contract. Dolt remains an external process: the
// adapter invokes its SQL interface and never embeds the storage engine.
package dolt

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
	"time"
)

const checkpointID = 1

type Failpoint string

const (
	FailpointBeforeSQLAndDoltCommit Failpoint = "before_sql_and_dolt_commit"
	FailpointAfterSQLAndDoltCommit  Failpoint = "after_sql_and_dolt_commit"
)

type Options struct {
	Failpoint func(Failpoint)
}

type Store struct {
	mu        sync.RWMutex
	binary    string
	path      string
	core      *memory.Store
	closed    bool
	failpoint func(Failpoint)
}

// Open initializes or reopens an isolated Dolt repository. binary must name a
// Dolt executable; keeping discovery outside the adapter makes backend version
// selection explicit in tests and benchmark manifests.
func Open(binary, path string) (*Store, error) { return OpenWithOptions(binary, path, Options{}) }

func OpenWithOptions(binary, path string, options Options) (*Store, error) {
	if strings.TrimSpace(binary) == "" {
		return nil, errors.New("dolt binary path is required")
	}
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("dolt repository path is required")
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return nil, fmt.Errorf("create dolt repository directory: %w", err)
	}
	store := &Store{binary: binary, path: path, failpoint: options.Failpoint}
	if _, err := os.Stat(filepath.Join(path, ".dolt")); errors.Is(err, os.ErrNotExist) {
		if _, err := store.run(context.Background(), "init", "--name", "Motor Autonomo Runtime", "--email", "runtime@localhost.invalid"); err != nil {
			return nil, fmt.Errorf("initialize dolt repository: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("inspect dolt repository: %w", err)
	}
	if err := store.configure(); err != nil {
		return nil, err
	}
	core, err := store.load()
	if err != nil {
		return nil, err
	}
	store.core = core
	return store, nil
}

func (s *Store) configure() error {
	query := `CREATE TABLE IF NOT EXISTS runtime_checkpoint (
		id BIGINT PRIMARY KEY,
		format_version BIGINT NOT NULL,
		payload LONGBLOB NOT NULL
	);
	CALL DOLT_ADD('-A');
	CALL DOLT_COMMIT('--skip-empty', '-m', 'initialize runtime checkpoint');`
	if _, err := s.run(context.Background(), "sql", "-q", query); err != nil {
		return fmt.Errorf("configure dolt checkpoint: %w", err)
	}
	return nil
}

func (s *Store) load() (*memory.Store, error) {
	output, err := s.run(context.Background(), "sql", "-r", "json", "-q",
		fmt.Sprintf("SELECT format_version, HEX(payload) AS payload_hex FROM runtime_checkpoint WHERE id = %d", checkpointID))
	if err != nil {
		return nil, fmt.Errorf("load dolt checkpoint: %w", err)
	}
	var result struct {
		Rows []struct {
			FormatVersion int    `json:"format_version"`
			PayloadHex    string `json:"payload_hex"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("decode dolt checkpoint query: %w", err)
	}
	if len(result.Rows) == 0 {
		return memory.New(), nil
	}
	if len(result.Rows) != 1 {
		return nil, fmt.Errorf("dolt checkpoint query returned %d rows", len(result.Rows))
	}
	if !memory.SupportsExternalCheckpointFormat(result.Rows[0].FormatVersion) {
		return nil, fmt.Errorf("load dolt checkpoint: %w: got %d, support %d", memory.ErrUnsupportedCheckpointFormat, result.Rows[0].FormatVersion, memory.CheckpointFormatVersion)
	}
	payload, err := hex.DecodeString(result.Rows[0].PayloadHex)
	if err != nil {
		return nil, fmt.Errorf("decode dolt checkpoint payload: %w", err)
	}
	if err := memory.ValidateExternalCheckpoint(result.Rows[0].FormatVersion, payload); err != nil {
		return nil, fmt.Errorf("validate dolt checkpoint: %w", err)
	}
	core, err := memory.NewFromBinary(payload)
	if err != nil {
		return nil, fmt.Errorf("restore dolt checkpoint: %w", err)
	}
	return core, nil
}

func (s *Store) View(ctx context.Context, fn func(port.Reader) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return errors.New("dolt store is closed")
	}
	return s.core.View(ctx, fn)
}

func (s *Store) LongTermMemory(key string) (domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return domain.LongTermMemory{}, errors.New("store is closed")
	}
	return s.core.LongTermMemory(key)
}

func (s *Store) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("store is closed")
	}
	return s.core.ListMemoriesByScope(scope)
}

func (s *Store) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return nil, errors.New("store is closed")
	}
	return s.core.ListExpiredMemories(now)
}

func (s *Store) SaveMemory(mem domain.LongTermMemory) error {
	return s.Update(context.Background(), func(tx port.Transaction) error {
		return tx.SaveMemory(mem)
	})
}

func (s *Store) DeleteMemory(id domain.MemoryID) error {
	return s.Update(context.Background(), func(tx port.Transaction) error {
		return tx.DeleteMemory(id)
	})
}

func (s *Store) Update(ctx context.Context, fn func(port.Transaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New("dolt store is closed")
	}

	before, err := s.core.MarshalBinary()
	if err != nil {
		return err
	}
	working, err := memory.NewFromBinary(before)
	if err != nil {
		return err
	}
	if err := working.Update(ctx, fn); err != nil {
		return err
	}
	payload, err := working.MarshalBinary()
	if err != nil {
		return err
	}
	encoded := strings.ToUpper(hex.EncodeToString(payload))
	query := fmt.Sprintf(`INSERT INTO runtime_checkpoint(id, format_version, payload)
		VALUES(%d, %d, UNHEX('%s'))
		ON DUPLICATE KEY UPDATE format_version=VALUES(format_version), payload=VALUES(payload);
		CALL DOLT_ADD('-A');
		CALL DOLT_COMMIT('--skip-empty', '-m', 'runtime checkpoint');`, checkpointID, memory.CheckpointFormatVersion, encoded)
	if s.failpoint != nil {
		s.failpoint(FailpointBeforeSQLAndDoltCommit)
	}
	if _, err := s.run(ctx, "sql", "-q", query); err != nil {
		return fmt.Errorf("commit dolt checkpoint: %w", err)
	}
	if s.failpoint != nil {
		s.failpoint(FailpointAfterSQLAndDoltCommit)
	}
	s.core = working
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.core = nil
	return nil
}

func (s *Store) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, s.binary, args...)
	cmd.Dir = s.path
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 2048 {
			message = message[:2048]
		}
		return nil, fmt.Errorf("dolt %s: %w: %s", strings.Join(args, " "), err, message)
	}
	return output, nil
}
