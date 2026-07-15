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

	"motor-autonomo/internal/port"
	"motor-autonomo/internal/storage/memory"

	_ "modernc.org/sqlite"
)

const checkpointID = 1

type Store struct {
	mu   sync.RWMutex
	db   *sql.DB
	core *memory.Store
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := configure(db); err != nil {
		db.Close()
		return nil, err
	}
	core, err := load(db)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, core: core}, nil
}

func configure(db *sql.DB) error {
	for _, statement := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA synchronous=FULL`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS runtime_checkpoint (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			format_version INTEGER NOT NULL,
			payload BLOB NOT NULL
		) STRICT`,
	} {
		if _, err := db.Exec(statement); err != nil {
			return fmt.Errorf("configure sqlite with %q: %w", statement, err)
		}
	}
	return nil
}

func load(db *sql.DB) (*memory.Store, error) {
	var payload []byte
	err := db.QueryRow(`SELECT payload FROM runtime_checkpoint WHERE id = ?`, checkpointID).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return memory.New(), nil
	}
	if err != nil {
		return nil, fmt.Errorf("load sqlite checkpoint: %w", err)
	}
	core, err := memory.NewFromBinary(payload)
	if err != nil {
		return nil, fmt.Errorf("restore sqlite checkpoint: %w", err)
	}
	return core, nil
}

func (s *Store) View(ctx context.Context, fn func(port.Reader) error) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.core.View(ctx, fn)
}

func (s *Store) Update(ctx context.Context, fn func(port.Transaction) error) error {
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
	if err := working.Update(ctx, fn); err != nil {
		return err
	}
	payload, err := working.MarshalBinary()
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlite checkpoint: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_checkpoint(id, format_version, payload)
		VALUES(?, 1, ?)
		ON CONFLICT(id) DO UPDATE SET format_version=excluded.format_version, payload=excluded.payload`, checkpointID, payload); err != nil {
		return fmt.Errorf("write sqlite checkpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit sqlite checkpoint: %w", err)
	}
	s.core = working
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
