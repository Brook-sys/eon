// Package sqlite implements a durable SQLite checkpoint adapter for the
// backend-neutral Store contract. It deliberately reuses the validated memory
// reference model for domain rules while SQLite owns atomic durability.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"motor-autonomo/internal/domain"
	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"
	"time"

	_ "modernc.org/sqlite"
)

const (
	checkpointID         = 1
	runtimeApplicationID = 0x4d415554 // "MAUT"
	runtimeUserVersion   = 1
)

type Failpoint string

const (
	FailpointBeforeDurableCommit Failpoint = "before_durable_commit"
	FailpointAfterDurableCommit  Failpoint = "after_durable_commit"
)

type Options struct {
	Failpoint         func(Failpoint)
	BusyTimeout       time.Duration
	Synchronous       string // "FULL" (default) or "NORMAL"
	ObserveUpdate     func(UpdateTiming)
	WalAutoCheckpoint int // pages per WAL autocheckpoint; 0 = SQLite default (1000), -1 = disable
}

// UpdateTiming separates work performed by the in-memory transaction callback
// from database/sql/SQLite phases. WriteCAS includes SQLite lock acquisition,
// the checkpoint write, and evaluation of the optimistic CAS predicate; the
// driver does not expose those three sub-phases independently.
type UpdateTiming struct {
	Callback       time.Duration
	Begin          time.Duration
	WriteCAS       time.Duration
	Commit         time.Duration
	ConflictReload time.Duration
	PayloadBytes   int
}

type Store struct {
	mu               sync.RWMutex
	db               *sql.DB
	core             *memory.Store
	persistedFormat  int
	persistedPayload []byte
	failpoint        func(Failpoint)
	observeUpdate    func(UpdateTiming)
}

func Open(path string) (*Store, error) { return OpenWithOptions(path, Options{}) }

func OpenWithOptions(path string, options Options) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := configure(db, options.BusyTimeout, options.Synchronous, options.WalAutoCheckpoint); err != nil {
		db.Close()
		return nil, err
	}
	core, format, payload, err := loadCheckpoint(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, core: core, persistedFormat: format, persistedPayload: payload, failpoint: options.Failpoint, observeUpdate: options.ObserveUpdate}, nil
}

func configure(db *sql.DB, busyTimeout time.Duration, synchronous string, walAutoCheckpoint int) error {
	if walAutoCheckpoint < -1 {
		return fmt.Errorf("configure sqlite: wal autocheckpoint must be -1, 0, or a positive page count")
	}
	if busyTimeout <= 0 {
		busyTimeout = 5 * time.Second
	}
	busyMilliseconds := busyTimeout.Milliseconds()
	if busyMilliseconds < 1 {
		busyMilliseconds = 1
	}
	if synchronous == "" {
		synchronous = "FULL"
	}
	// WalAutoCheckpoint controls when SQLite automatically checkpoints the WAL
	// back into the main database. Zero leaves SQLite's connection default
	// unchanged (currently 1000 pages); -1 disables autocheckpoint entirely.
	var walAutoStmt string
	if walAutoCheckpoint == -1 {
		walAutoStmt = `PRAGMA wal_autocheckpoint=0`
	} else if walAutoCheckpoint > 0 {
		walAutoStmt = fmt.Sprintf(`PRAGMA wal_autocheckpoint=%d`, walAutoCheckpoint)
	}
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		fmt.Sprintf(`PRAGMA synchronous=%s`, synchronous),
		fmt.Sprintf(`PRAGMA busy_timeout=%d`, busyMilliseconds),
		`PRAGMA foreign_keys=ON`,
		fmt.Sprintf(`PRAGMA application_id=%d`, runtimeApplicationID),
		fmt.Sprintf(`PRAGMA user_version=%d`, runtimeUserVersion),
	}
	if walAutoStmt != "" {
		statements = append(statements, walAutoStmt)
	}
	statements = append(statements, `CREATE TABLE IF NOT EXISTS runtime_checkpoint (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		format_version INTEGER NOT NULL,
		payload BLOB NOT NULL
	) STRICT`)
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("configure sqlite with %q: %w", statement, err)
		}
	}
	return nil
}

func loadCheckpoint(db *sql.DB) (*memory.Store, int, []byte, error) {
	var formatVersion int
	var payload []byte
	err := db.QueryRow(`SELECT format_version, payload FROM runtime_checkpoint WHERE id = ?`, checkpointID).Scan(&formatVersion, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.New(), 0, nil, nil
	}
	if err != nil {
		return nil, 0, nil, fmt.Errorf("load sqlite checkpoint: %w", err)
	}
	if !memory.SupportsExternalCheckpointFormat(formatVersion) {
		return nil, 0, nil, fmt.Errorf("load sqlite checkpoint: %w: got %d, support %d", memory.ErrUnsupportedCheckpointFormat, formatVersion, memory.CheckpointFormatVersion)
	}
	if err := memory.ValidateExternalCheckpoint(formatVersion, payload); err != nil {
		return nil, 0, nil, fmt.Errorf("validate sqlite checkpoint: %w", err)
	}
	core, err := memory.NewFromBinary(payload)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("restore sqlite checkpoint: %w", err)
	}
	return core, formatVersion, payload, nil
}

func (s *Store) View(ctx context.Context, fn func(port.Reader) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.core.View(ctx, fn)
}

func (s *Store) LongTermMemory(key string) (domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.core.LongTermMemory(key)
}

func (s *Store) ListMemoriesByScope(scope domain.MemoryScope) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.core.ListMemoriesByScope(scope)
}

func (s *Store) ListExpiredMemories(now time.Time) ([]domain.LongTermMemory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.core.ListExpiredMemories(now)
}

// RuntimeVersion returns the SQLite engine version actually loaded by the
// configured driver, rather than inferring it from the Go module version.
func (s *Store) RuntimeVersion() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var version string
	if err := s.db.QueryRow(`SELECT sqlite_version()`).Scan(&version); err != nil {
		return "", fmt.Errorf("query sqlite runtime version: %w", err)
	}
	return version, nil
}

func (s *Store) Update(ctx context.Context, fn func(port.Transaction) error) error {
	timing := UpdateTiming{}
	if s.observeUpdate != nil {
		defer func() { s.observeUpdate(timing) }()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	before, err := s.core.MarshalBinary()
	if err != nil {
		return err
	}
	working, err := memory.NewFromBinary(before)
	if err != nil {
		return err
	}
	started := time.Now()
	err = working.Update(ctx, fn)
	timing.Callback = time.Since(started)
	if err != nil {
		return err
	}
	payload, err := working.MarshalBinary()
	if err != nil {
		return err
	}
	started = time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	timing.Begin = time.Since(started)
	if err != nil {
		return fmt.Errorf("begin sqlite checkpoint: %w", err)
	}
	defer tx.Rollback()
	started = time.Now()
	timing.PayloadBytes = len(payload)
	result, err := tx.ExecContext(ctx, `INSERT INTO runtime_checkpoint(id, format_version, payload)
		VALUES(?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET format_version=excluded.format_version, payload=excluded.payload
		WHERE runtime_checkpoint.format_version = ? AND runtime_checkpoint.payload = ?`,
		checkpointID, memory.CheckpointFormatVersion, payload, s.persistedFormat, s.persistedPayload)
	timing.WriteCAS = time.Since(started)
	if err != nil {
		return fmt.Errorf("write sqlite checkpoint: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect sqlite checkpoint write: %w", err)
	}
	if rows != 1 {
		if err := tx.Rollback(); err != nil {
			return fmt.Errorf("rollback stale sqlite checkpoint: %w", err)
		}
		started = time.Now()
		latest, format, persisted, err := loadCheckpoint(s.db)
		timing.ConflictReload = time.Since(started)
		if err != nil {
			return fmt.Errorf("reload sqlite checkpoint after conflict: %w", err)
		}
		s.core = latest
		s.persistedFormat = format
		s.persistedPayload = persisted
		return port.ErrConflict
	}
	if s.failpoint != nil {
		s.failpoint(FailpointBeforeDurableCommit)
	}
	started = time.Now()
	err = tx.Commit()
	timing.Commit = time.Since(started)
	if err != nil {
		return fmt.Errorf("commit sqlite checkpoint: %w", err)
	}
	if s.failpoint != nil {
		s.failpoint(FailpointAfterDurableCommit)
	}
	s.core = working
	s.persistedFormat = memory.CheckpointFormatVersion
	s.persistedPayload = append(s.persistedPayload[:0], payload...)
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}
