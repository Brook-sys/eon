// loop.go — vocabulário do loop multiciclo (docs/plan/03-LOOP.md e
// docs/plan/05-DOMAIN-AND-PERSISTENCE.md; D04/D13/D26/D44).
//
// Puro por lei (ADR-0002): sem I/O, sem infra. O kernel (internal/core)
// executa; este arquivo é o contrato que kernel, storage e eonctl
// compartilham. Conceito novo só entra quando uma frente precisa (D04).
package domain

import (
	"errors"
	"time"
)

// Front: as três frentes — e os braços do bandit (D26).
const (
	FrontProject  = "project"
	FrontThoughts = "thoughts"
	FrontSelf     = "self"
)

// Fronts: ordem canônica — determinismo do scheduler (sem RNG, D26).
var Fronts = []string{FrontProject, FrontThoughts, FrontSelf}

// ErrQuotaExceeded: a cota acabou (real ou fake). Classe que o kernel
// trata acelerando o Ciclo 2 (sleep-time compute, D12) — nunca parando
// (invariante 1).
var ErrQuotaExceeded = errors.New("cota esgotada")

// ErrTransient: falha transitória de provider — retry com backoff, NUNCA
// falha da tarefa (D29 chega completo no M2; o kernel já distingue).
var ErrTransient = errors.New("falha transitória de provider")

// Mission (D13): N lógicas, 1 executando por vez. Não se auto-cria; só
// nasce de operador ou pipeline versionado.
type Mission struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Purpose   string    `json:"purpose"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MissionActive: o único status que o M1 conhece — missão parada é
// decisão de operador (M5 decide a semântica completa).
const MissionActive = "active"

// TaskStatus — máquina de estados (03-LOOP.md; herança boa da v1).
type TaskStatus string

const (
	TaskNew             TaskStatus = "NEW"
	TaskReady           TaskStatus = "READY"
	TaskRunning         TaskStatus = "RUNNING"
	TaskVerifying       TaskStatus = "VERIFYING"
	TaskSucceeded       TaskStatus = "SUCCEEDED"
	TaskWaitingTime     TaskStatus = "WAITING_TIME"
	TaskWaitingEvent    TaskStatus = "WAITING_EVENT"
	TaskWaitingOperator TaskStatus = "WAITING_OPERATOR"
	TaskThrottled       TaskStatus = "THROTTLED"
	TaskBlocked         TaskStatus = "BLOCKED"
	TaskExhausted       TaskStatus = "EXHAUSTED"
	TaskReplanning      TaskStatus = "REPLANNING"
	TaskFailed          TaskStatus = "FAILED"
	TaskCancelled       TaskStatus = "CANCELLED"
)

// taskTerminal: estados sem saída — a tarefa acabou (bem, mal ou por
// decisão); história congelada. Sucessores são tarefas NOVAS com
// derived_from, nunca mutação do morto.
var taskTerminal = map[TaskStatus]bool{
	TaskSucceeded: true,
	TaskFailed:    true,
	TaskCancelled: true,
}

// taskTransitions: a lei da máquina de estados, como tabela. O kernel
// valida TODA transição aqui antes de persistir (invariante 2: código
// determinístico decide — a decisão é tabela, não opinião).
var taskTransitions = map[TaskStatus][]TaskStatus{
	TaskNew:   {TaskReady, TaskCancelled},
	TaskReady: {TaskRunning, TaskCancelled},
	TaskRunning: {TaskVerifying, TaskWaitingTime, TaskWaitingEvent,
		TaskWaitingOperator, TaskThrottled, TaskBlocked, TaskExhausted,
		TaskReplanning, TaskFailed, TaskCancelled},
	TaskVerifying:       {TaskSucceeded, TaskFailed, TaskReplanning, TaskCancelled},
	TaskWaitingTime:     {TaskReady, TaskCancelled},
	TaskWaitingEvent:    {TaskReady, TaskCancelled},
	TaskWaitingOperator: {TaskReady, TaskCancelled},
	TaskThrottled:       {TaskReady, TaskCancelled},
	TaskBlocked:         {TaskReady, TaskCancelled},
	TaskExhausted:       {TaskCancelled},
	TaskReplanning:      {TaskReady, TaskCancelled},
}

// CanTransition: verdadeiro se a transição é legal. Cancelar é sempre
// direito (de qualquer estado vivo); estados terminais são congelados.
func CanTransition(from, to TaskStatus) bool {
	if taskTerminal[from] {
		return false
	}
	if to == TaskCancelled {
		return true
	}
	for _, t := range taskTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Task: unidade de agenda (Ciclo 1). Declaração obrigatória: proveniência,
// ganho esperado, novidade, custo estimado, stop condition (03-LOOP.md).
type Task struct {
	ID            string     `json:"id"`
	MissionID     string     `json:"mission_id"`
	Front         string     `json:"front"` // domain.Front*
	Title         string     `json:"title"`
	DerivedFrom   string     `json:"derived_from,omitempty"`
	ExpectedGain  string     `json:"expected_gain"`
	Novelty       string     `json:"novelty"`
	CostEstimate  string     `json:"cost_estimate"`
	StopCondition string     `json:"stop_condition"`
	Status        TaskStatus `json:"status"`
	NotBefore     *time.Time `json:"not_before,omitempty"` // esperas locais
	Attempt       int        `json:"attempt"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// Step: microturn do Ciclo 0 — 1 chamada de modelo OU 1 capability. O
// modelo vê só o passo (03-LOOP.md). M1: input/output in-line com hash
// SHA-256 (citável); o agregado Artifact nasce no M2, quando payloads
// reais de modelo existem (D04: conceito quando a frente precisa).
type Step struct {
	ID         string    `json:"id"`
	TaskID     string    `json:"task_id"`
	Seq        int       `json:"seq"`  // ordem dentro da task
	Kind       string    `json:"kind"` // model | capability
	Input      string    `json:"input"`
	InputHash  string    `json:"input_hash"`
	Output     string    `json:"output"`
	OutputHash string    `json:"output_hash"`
	DurationMs int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// Step kinds.
const (
	StepKindModel      = "model"
	StepKindCapability = "capability"
)

// Event: log append-only — toda transição oficial gera event (D03).
// Seq é autoridade do storage (AUTOINCREMENT); payload é JSON cru com
// hash para citação auditável.
type Event struct {
	Seq           int64     `json:"seq"`
	AggregateType string    `json:"aggregate_type"`
	AggregateID   string    `json:"aggregate_id"`
	Type          string    `json:"type"`
	PayloadHash   string    `json:"payload_hash"`
	Payload       []byte    `json:"payload"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// Tipos canônicos de event do M1 — novos nascem com o agregado que os
// produz, nunca "para o futuro".
const (
	EventMissionCreated    = "mission.created"
	EventTaskCreated       = "task.created"
	EventTaskTransition    = "task.transition"
	EventStepRecorded      = "step.recorded"
	EventOutboxQueued      = "outbox.queued"
	EventOutboxDrained     = "outbox.drained"
	EventKernelCycle       = "kernel.cycle" // telemetria do loop
	EventContinuityBlocked = "kernel.continuity_blocked"
)

// OutboxItem: efeito externo pendente (transactional outbox, D03) —
// gravado NA MESMA transação do changeset, drenado depois. M1 drena
// fake (o efeito real — git push — chega com o M3).
type OutboxItem struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Payload   []byte    `json:"payload"`
	Attempts  int       `json:"attempts"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}
