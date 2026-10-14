// migrate.go — migrações versionadas EMBUTIDAS no binário (D03:
// golang-migrate-style, sem dependência externa). O ponteiro de versão é
// o user_version nativo do SQLite — transacional, sem tabela extra.
//
// Lei: toda mudança de schema é um item NOVO no fim da slice; migração
// já aplicada nunca se edita. A cadeia é testada (contract suite).
package storage

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
-- M1 — schema canônico (docs/plan/05, D44). Tabelas por agregado +
-- events append-only + outbox + kv_meta. O agregado Artifact nasce no
-- M2 por migração (D04: conceito quando a frente precisa).

CREATE TABLE mission (
  id         TEXT PRIMARY KEY,
  title      TEXT NOT NULL,
  purpose    TEXT NOT NULL,
  status     TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE task (
  id             TEXT PRIMARY KEY,
  mission_id     TEXT NOT NULL REFERENCES mission(id),
  front          TEXT NOT NULL,
  title          TEXT NOT NULL,
  derived_from   TEXT,
  expected_gain  TEXT NOT NULL,
  novelty        TEXT NOT NULL,
  cost_estimate  TEXT NOT NULL,
  stop_condition TEXT NOT NULL,
  status         TEXT NOT NULL,
  not_before     INTEGER,
  attempt        INTEGER NOT NULL DEFAULT 0,
  created_at     INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);
-- fila do scheduler: por frente, READY, espera vencida, mais antiga (D44)
CREATE INDEX idx_task_ready  ON task(front, status, not_before, created_at);
CREATE INDEX idx_task_status ON task(status);

CREATE TABLE step (
  id          TEXT PRIMARY KEY,
  task_id     TEXT NOT NULL REFERENCES task(id),
  seq         INTEGER NOT NULL,
  kind        TEXT NOT NULL,
  input       TEXT NOT NULL,
  input_hash  TEXT NOT NULL,
  output      TEXT NOT NULL DEFAULT '',
  output_hash TEXT NOT NULL DEFAULT '',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL
);
CREATE INDEX idx_step_task ON step(task_id, seq);

CREATE TABLE events (
  seq            INTEGER PRIMARY KEY AUTOINCREMENT,
  aggregate_type TEXT NOT NULL,
  aggregate_id   TEXT NOT NULL,
  type           TEXT NOT NULL,
  payload_hash   TEXT NOT NULL,
  payload        BLOB NOT NULL,
  occurred_at    INTEGER NOT NULL
);
CREATE INDEX idx_events_agg  ON events(aggregate_type, aggregate_id, seq);
CREATE INDEX idx_events_type ON events(type, seq);

CREATE TABLE outbox (
  id         TEXT PRIMARY KEY,
  kind       TEXT NOT NULL,
  payload    BLOB NOT NULL,
  attempts   INTEGER NOT NULL DEFAULT 0,
  done        INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE INDEX idx_outbox_pending ON outbox(done, created_at);

CREATE TABLE kv_meta (
  key        TEXT PRIMARY KEY,
  value      BLOB NOT NULL,
  updated_at INTEGER NOT NULL
);
`,
	},
}

// Migrate aplica migrações pendentes em ordem, cada uma em transação
// própria com o user_version avançado DENTRO dela — crash no meio deixa a
// versão consistente com o schema que chegou a ser aplicado.
func Migrate(db *sql.DB) error {
	var current int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&current); err != nil {
		return fmt.Errorf("storage: user_version: %w", err)
	}
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(m.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: migração %d: %w", m.version, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version=%d`, m.version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("storage: user_version=%d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("storage: commit da migração %d: %w", m.version, err)
		}
		current = m.version
	}
	return nil
}
