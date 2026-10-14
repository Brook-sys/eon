// repos.go — os repositórios estreitos de verdade (D03: um por agregado,
// anti-god-interface). Escrita SEMPRE via Unit; leitura direta (WAL:
// leitor nunca bloqueia o kernel).
package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// ErrNotFound: o agregado não existe — erro canônico para o kernel
// decidir (boot que não acha missão CRIA a stub; lookup que não acha
// task é bug).
var ErrNotFound = errors.New("storage: não encontrado")

func unix(t time.Time) int64 { return t.UTC().Unix() }

func fromUnix(sec int64) time.Time { return time.Unix(sec, 0).UTC() }

type scanner interface {
	Scan(dest ...any) error
}

// ---------------------------------------------------------------------------
// Mission

type missionRepo struct{ db *sql.DB }

func (r missionRepo) Put(u port.Unit, m *domain.Mission) error {
	tx, err := txOf(u)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO mission (id,title,purpose,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  title=excluded.title, purpose=excluded.purpose,
		  status=excluded.status, updated_at=excluded.updated_at`,
		m.ID, m.Title, m.Purpose, m.Status, unix(m.CreatedAt), unix(m.UpdatedAt))
	return err
}

func (r missionRepo) Get(id string) (*domain.Mission, error) {
	var m domain.Mission
	var c, up int64
	err := r.db.QueryRow(
		`SELECT id,title,purpose,status,created_at,updated_at FROM mission WHERE id=?`, id,
	).Scan(&m.ID, &m.Title, &m.Purpose, &m.Status, &c, &up)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.CreatedAt, m.UpdatedAt = fromUnix(c), fromUnix(up)
	return &m, nil
}

// ---------------------------------------------------------------------------
// Task

type taskRepo struct{ db *sql.DB }

const taskCols = `id,mission_id,front,title,derived_from,expected_gain,` +
	`novelty,cost_estimate,stop_condition,status,not_before,attempt,created_at,updated_at`

func scanTask(row scanner) (*domain.Task, error) {
	var (
		t       domain.Task
		derived sql.NullString
		nb      sql.NullInt64
		c, up   int64
	)
	err := row.Scan(&t.ID, &t.MissionID, &t.Front, &t.Title, &derived, &t.ExpectedGain,
		&t.Novelty, &t.CostEstimate, &t.StopCondition, &t.Status, &nb, &t.Attempt, &c, &up)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if derived.Valid {
		t.DerivedFrom = derived.String
	}
	if nb.Valid {
		w := fromUnix(nb.Int64)
		t.NotBefore = &w
	}
	t.CreatedAt, t.UpdatedAt = fromUnix(c), fromUnix(up)
	return &t, nil
}

func (r taskRepo) Put(u port.Unit, t *domain.Task) error {
	tx, err := txOf(u)
	if err != nil {
		return err
	}
	var nb any
	if t.NotBefore != nil {
		nb = unix(*t.NotBefore)
	}
	_, err = tx.Exec(`INSERT INTO task (`+taskCols+`)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  mission_id=excluded.mission_id, front=excluded.front,
		  title=excluded.title, derived_from=excluded.derived_from,
		  expected_gain=excluded.expected_gain, novelty=excluded.novelty,
		  cost_estimate=excluded.cost_estimate,
		  stop_condition=excluded.stop_condition, status=excluded.status,
		  not_before=excluded.not_before, attempt=excluded.attempt,
		  updated_at=excluded.updated_at`,
		t.ID, t.MissionID, t.Front, t.Title, nullStr(t.DerivedFrom), t.ExpectedGain,
		t.Novelty, t.CostEstimate, t.StopCondition, string(t.Status), nb,
		t.Attempt, unix(t.CreatedAt), unix(t.UpdatedAt))
	return err
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r taskRepo) Get(id string) (*domain.Task, error) {
	return scanTask(r.db.QueryRow(`SELECT `+taskCols+` FROM task WHERE id=?`, id))
}

// NextReady: a READY mais antiga da frente com espera vencida — ordem
// (created_at, id) determinística. A fila do scheduler (D26/D44).
func (r taskRepo) NextReady(front string, now time.Time) (*domain.Task, error) {
	return scanTask(r.db.QueryRow(`SELECT `+taskCols+` FROM task
		WHERE front=? AND status=? AND (not_before IS NULL OR not_before<=?)
		ORDER BY created_at ASC, id ASC LIMIT 1`,
		front, string(domain.TaskReady), unix(now)))
}

func (r taskRepo) CountReady(front string, now time.Time) (int, error) {
	var n int
	err := r.db.QueryRow(`SELECT count(*) FROM task
		WHERE front=? AND status=? AND (not_before IS NULL OR not_before<=?)`,
		front, string(domain.TaskReady), unix(now)).Scan(&n)
	return n, err
}

// ---------------------------------------------------------------------------
// Step

type stepRepo struct{ db *sql.DB }

func (r stepRepo) Put(u port.Unit, s *domain.Step) error {
	tx, err := txOf(u)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO step
		(id,task_id,seq,kind,input,input_hash,output,output_hash,duration_ms,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  output=excluded.output, output_hash=excluded.output_hash,
		  duration_ms=excluded.duration_ms`,
		s.ID, s.TaskID, s.Seq, s.Kind, s.Input, s.InputHash, s.Output, s.OutputHash,
		s.DurationMs, unix(s.CreatedAt))
	return err
}

func (r stepRepo) ByTask(taskID string) ([]domain.Step, error) {
	rows, err := r.db.Query(`SELECT
		id,task_id,seq,kind,input,input_hash,output,output_hash,duration_ms,created_at
		FROM step WHERE task_id=? ORDER BY seq ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Step
	for rows.Next() {
		var s domain.Step
		var c int64
		if err := rows.Scan(&s.ID, &s.TaskID, &s.Seq, &s.Kind, &s.Input, &s.InputHash,
			&s.Output, &s.OutputHash, &s.DurationMs, &c); err != nil {
			return nil, err
		}
		s.CreatedAt = fromUnix(c)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// EventLog

type eventLog struct{ db *sql.DB }

// Append: escreve o event DENTRO do Unit do changeset (D03: efeito e
// estado na mesma transação). Seq é autoridade do banco; hash é SHA-256
// do payload cru — citação auditável byte a byte (D36 heritage).
func (r eventLog) Append(u port.Unit, aggregateType, aggregateID, eventType string, payload []byte) (domain.Event, error) {
	tx, err := txOf(u)
	if err != nil {
		return domain.Event{}, err
	}
	sum := sha256.Sum256(payload)
	now := time.Now().UTC()
	res, err := tx.Exec(`INSERT INTO events
		(aggregate_type,aggregate_id,type,payload_hash,payload,occurred_at)
		VALUES (?,?,?,?,?,?)`,
		aggregateType, aggregateID, eventType, hex.EncodeToString(sum[:]), payload, unix(now))
	if err != nil {
		return domain.Event{}, err
	}
	seq, err := res.LastInsertId()
	if err != nil {
		return domain.Event{}, err
	}
	return domain.Event{
		Seq:           seq,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Type:          eventType,
		PayloadHash:   hex.EncodeToString(sum[:]),
		Payload:       append([]byte(nil), payload...),
		OccurredAt:    now,
	}, nil
}

// After: cursor pagination por seq — sem OFFSET (D44).
func (r eventLog) After(seq int64, limit int) ([]domain.Event, error) {
	rows, err := r.db.Query(`SELECT
		seq,aggregate_type,aggregate_id,type,payload_hash,payload,occurred_at
		FROM events WHERE seq>? ORDER BY seq ASC LIMIT ?`, seq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Event
	for rows.Next() {
		var e domain.Event
		var c int64
		if err := rows.Scan(&e.Seq, &e.AggregateType, &e.AggregateID, &e.Type,
			&e.PayloadHash, &e.Payload, &c); err != nil {
			return nil, err
		}
		e.OccurredAt = fromUnix(c)
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Outbox

type outboxRepo struct{ db *sql.DB }

func (r outboxRepo) Put(u port.Unit, kind string, payload []byte) (domain.OutboxItem, error) {
	tx, err := txOf(u)
	if err != nil {
		return domain.OutboxItem{}, err
	}
	var idraw [8]byte
	if _, err := rand.Read(idraw[:]); err != nil {
		return domain.OutboxItem{}, err
	}
	it := domain.OutboxItem{
		ID:        hex.EncodeToString(idraw[:]),
		Kind:      kind,
		Payload:   append([]byte(nil), payload...),
		CreatedAt: time.Now().UTC(),
	}
	_, err = tx.Exec(`INSERT INTO outbox (id,kind,payload,attempts,done,created_at)
		VALUES (?,?,?,0,0,?)`, it.ID, it.Kind, it.Payload, unix(it.CreatedAt))
	if err != nil {
		return domain.OutboxItem{}, err
	}
	return it, nil
}

func (r outboxRepo) Pending(limit int) ([]domain.OutboxItem, error) {
	rows, err := r.db.Query(`SELECT id,kind,payload,attempts,done,created_at
		FROM outbox WHERE done=0 ORDER BY created_at ASC, id ASC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.OutboxItem
	for rows.Next() {
		var it domain.OutboxItem
		var done int
		var c int64
		if err := rows.Scan(&it.ID, &it.Kind, &it.Payload, &it.Attempts, &done, &c); err != nil {
			return nil, err
		}
		it.Done = done != 0
		it.CreatedAt = fromUnix(c)
		out = append(out, it)
	}
	return out, rows.Err()
}

func (r outboxRepo) MarkDone(u port.Unit, id string) error {
	tx, err := txOf(u)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE outbox SET done=1 WHERE id=?`, id)
	return err
}

// ---------------------------------------------------------------------------
// KV

type kvRepo struct{ db *sql.DB }

func (r kvRepo) Set(u port.Unit, key string, value []byte) error {
	tx, err := txOf(u)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO kv_meta (key,value,updated_at) VALUES (?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, unix(time.Now().UTC()))
	return err
}

func (r kvRepo) Get(key string) ([]byte, error) {
	var v []byte
	err := r.db.QueryRow(`SELECT value FROM kv_meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}
