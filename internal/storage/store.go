// store.go — a Store concreta: abre no modo EON (driver.go), migra e
// expõe os repositórios estreitos. Ninguém acima de storage conhece SQL
// (ADR-0002: storage implementa port).
package storage

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// unit: um ChangeSet aberto — 1 transação SQLite (D03). Sealed: só
// storage constrói units, só storage extrai o tx deles.
type unit struct{ tx *sql.Tx }

func (u *unit) Commit() error   { return u.tx.Commit() }
func (u *unit) Rollback() error { return u.tx.Rollback() }

func txOf(u port.Unit) (*sql.Tx, error) {
	t, ok := u.(*unit)
	if !ok {
		return nil, fmt.Errorf("storage: unit estranho (%T) — só storage abre units", u)
	}
	return t.tx, nil
}

// Store implementa port.Store.
type Store struct {
	db       *sql.DB
	missions missionRepo
	tasks    taskRepo
	steps    stepRepo
	events   eventLog
	outbox   outboxRepo
	kv       kvRepo
}

// NewStore: boot honesto — abre (WAL+FK verificados), migra (cadeia
// embutida) e devolve. Qualquer coisa fora da lei é erro alto, na hora.
func NewStore(path string) (*Store, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{
		db:       db,
		missions: missionRepo{db},
		tasks:    taskRepo{db},
		steps:    stepRepo{db},
		events:   eventLog{db},
		outbox:   outboxRepo{db},
		kv:       kvRepo{db},
	}, nil
}

// NewStoreReadonly: a MESMA Store sobre conexão mode=ro (eonctl, D15) —
// o driver recusa escrita. Sem migração: leitor não evolui schema; versão
// futura lê o que conhece e recusa o resto honestamente.
func NewStoreReadonly(path string) (*Store, error) {
	db, err := OpenReadonly(path)
	if err != nil {
		return nil, err
	}
	return &Store{
		db:       db,
		missions: missionRepo{db},
		tasks:    taskRepo{db},
		steps:    stepRepo{db},
		events:   eventLog{db},
		outbox:   outboxRepo{db},
		kv:       kvRepo{db},
	}, nil
}

// TaskCounts: contagem por status da frente num GROUP BY — suporte ao
// eonctl (D15). Método CONCRETO, não port: interface sem segundo lado é
// invenção; se uma frente precisar disto no código, promove a port.
func (s *Store) TaskCounts(front string) (map[domain.TaskStatus]int, error) {
	rows, err := s.db.Query(`SELECT status, count(*) FROM task
		WHERE front=? GROUP BY status`, front)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.TaskStatus]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[domain.TaskStatus(st)] = n
	}
	return out, rows.Err()
}

func (s *Store) Begin(ctx context.Context) (port.Unit, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &unit{tx: tx}, nil
}

func (s *Store) Missions() port.MissionRepo { return s.missions }
func (s *Store) Tasks() port.TaskRepo       { return s.tasks }
func (s *Store) Steps() port.StepRepo       { return s.steps }
func (s *Store) Events() port.EventLog      { return s.events }
func (s *Store) Outbox() port.OutboxRepo    { return s.outbox }
func (s *Store) KV() port.KVRepo            { return s.kv }
func (s *Store) Close() error               { return s.db.Close() }
