// storage.go — contratos de storage ESTREITOS por repositório (D03:
// anti-god-interface; a v1 tinha uma Transaction com 156 métodos — a
// vergonha que este port existe para nunca repetir).
//
// Lei do Unit: 1 ChangeSet = 1 transação (D03) — aplica inteiro ou
// rejeita inteiro. Escrita SÓ dentro de Unit; leitura vai direto (WAL:
// leitor — eonctl — nunca bloqueia o kernel). Commit serial (D10): sem
// leases, sem contenção esperada; busy_timeout é rede de segurança, não
// projeto.
package port

import (
	"context"
	"time"

	"github.com/brook-sys/eon/internal/domain"
)

// Unit: o ChangeSet aberto. Commit aplica o lote; Rollback descarta.
type Unit interface {
	Commit() error
	Rollback() error
}

// MissionRepo: repositório de Mission.
type MissionRepo interface {
	Put(u Unit, m *domain.Mission) error
	Get(id string) (*domain.Mission, error)
}

// TaskRepo: repositório de Task.
type TaskRepo interface {
	Put(u Unit, t *domain.Task) error
	Get(id string) (*domain.Task, error)
	// NextReady: a tarefa READY mais antiga da frente com not_before
	// vencido — a fila do scheduler, ordem determinística.
	NextReady(front string, now time.Time) (*domain.Task, error)
	// CountReady: tamanho da fila viva da frente (Ciclo 4 decide
	// replenishment vs CONTINUITY_BLOCKED com este número).
	CountReady(front string, now time.Time) (int, error)
}

// StepRepo: repositório de Step.
type StepRepo interface {
	Put(u Unit, s *domain.Step) error
	ByTask(taskID string) ([]domain.Step, error)
}

// EventLog: append-only (D03). Append é escrita (exige Unit); leitura
// por cursor de seq — paginação sem OFFSET (D44).
type EventLog interface {
	Append(u Unit, aggregateType, aggregateID, eventType string, payload []byte) (domain.Event, error)
	After(seq int64, limit int) ([]domain.Event, error)
}

// OutboxRepo: transactional outbox (D03).
type OutboxRepo interface {
	Put(u Unit, kind string, payload []byte) (domain.OutboxItem, error)
	Pending(limit int) ([]domain.OutboxItem, error)
	MarkDone(u Unit, id string) error
}

// KVRepo: kv_meta — meta pequeno que não pertence a agregado (o
// canônico tem tabela própria; aqui é só o resto auditável).
type KVRepo interface {
	Set(u Unit, key string, value []byte) error
	Get(key string) ([]byte, error)
}

// Store: a raiz de composição — abre Units e expõe os repositórios
// estreitos. Cada componente do kernel recebe SÓ os repositórios que
// usa (least-authority real); ninguém abaixo de cmd/eon segura a Store
// inteira.
type Store interface {
	Begin(ctx context.Context) (Unit, error)
	Missions() MissionRepo
	Tasks() TaskRepo
	Steps() StepRepo
	Events() EventLog
	Outbox() OutboxRepo
	KV() KVRepo
	Close() error
}
