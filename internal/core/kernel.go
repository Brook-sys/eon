// kernel.go — o loop multiciclo (docs/plan/03-LOOP.md) sobre storage via
// port. O kernel é o ÚNICO escritor canônico: serial, um ciclo por vez,
// transações curtas — a chamada de modelo NUNCA fica entre begin e
// commit (o adapter real do M2 pode demorar segundos; o crash no meio
// deixa no máximo uma RUNNING órfã, reconciliada no boot).
//
// Lei do ciclo: todo desfecho grava estado + event + telemetria do
// bandit NO MESMO changeset (D03). Erro de ciclo derruba a TENTATIVA,
// nunca o autômato (invariante 3).
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// Deps: tudo que o kernel usa, injetado pela raiz de composição
// (cmd/eon). Least-authority: Begin + SÓ os repositórios que usa.
type Deps struct {
	Begin    func(ctx context.Context) (port.Unit, error)
	Missions port.MissionRepo
	Tasks    port.TaskRepo
	Steps    port.StepRepo
	Events   port.EventLog
	Outbox   port.OutboxRepo
	KV       port.KVRepo
	Provider port.ModelProvider

	// Replenisher: trabalho novo para fila vazia (C4). nil → stub M1.
	// Seam de teste: injetar erro prova o CONTINUITY_BLOCKED.
	Replenisher func(u port.Unit, front string) (*domain.Task, error)

	Now           func() time.Time
	Log           *log.Logger
	CycleInterval time.Duration // <=0 → defaultCycleInterval
}

const (
	kernelCycleKey = "kernel.cycle"
	// defaultCycleInterval: cadência de produção do M1 — lenta de propósito:
	// o soak de 1000+ ciclos valida em teste com intervalo curto via Deps.
	defaultCycleInterval = 5 * time.Second
	// stubModelMaxTokens: custo fake de um microturn stub — cota de N
	// tokens ≈ N/16 chamadas de modelo.
	stubModelMaxTokens = 16
	// throttleRetryDelay: THROTTLED re-armam com espera local mínima —
	// a cadência real de retry é dominada pelos picks da frente
	// (piso de justiça K=20 limita o churn; D29 heritage).
	throttleRetryDelay = 30 * time.Second
	transientBackoff   = 10 * time.Second
	reviveBatch        = 10
	reconcileBatch     = 100
	drainBatch         = 10
)

// Kernel: o loop. Sem goroutine própria — serial por lei (D10).
type Kernel struct {
	deps   Deps
	bandit *Bandit
	cycle  int64
	now    func() time.Time
	log    *log.Logger
}

// NewKernel normaliza deps (defaults explícitos, nunca escondidos),
// monta o bandit e hidrata o contador de ciclos (crash = consistente
// com os events; contador corrompido recomeça em 0 com barulho).
func NewKernel(deps Deps) *Kernel {
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Now().UTC() }
	}
	if deps.Log == nil {
		deps.Log = log.New(os.Stderr, "core: ", log.LstdFlags)
	}
	now := deps.Now
	if deps.Replenisher == nil {
		kv := deps.KV
		deps.Replenisher = func(u port.Unit, front string) (*domain.Task, error) {
			return newStubTask(u, kv, front, "gen:"+front, now())
		}
	}
	k := &Kernel{
		deps:   deps,
		bandit: NewBandit(deps.KV, domain.Fronts, deps.Log),
		now:    now,
		log:    deps.Log,
	}
	if raw, err := deps.KV.Get(kernelCycleKey); err == nil {
		if err := json.Unmarshal(raw, &k.cycle); err != nil {
			k.log.Printf("contador de ciclos corrompido (%s), recomeçando em 0: %v", kernelCycleKey, err)
		}
	}
	return k
}

// CycleCount: o ciclo corrente (telemetria do heartbeat).
func (k *Kernel) CycleCount() int64 { return k.cycle }

// Bootstrap: missão-stub presente (cria se ausente) + reconciliação de
// crash: RUNNING/VERIFYING órfãs — não há executor vivo além deste
// processo, serial — voltam por REPLANNING→READY com events. Erro de
// boot é FATAL do processo: o supervisor decide (backoff/restart).
func (k *Kernel) Bootstrap(ctx context.Context) error {
	u, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	if _, err := k.deps.Missions.Get(StubMissionID); errors.Is(err, port.ErrNotFound) {
		m := StubMission(k.now())
		if err := k.deps.Missions.Put(u, m); err != nil {
			_ = u.Rollback()
			return err
		}
		if _, err := k.deps.Events.Append(u, "mission", m.ID, domain.EventMissionCreated, mustJSON(m)); err != nil {
			_ = u.Rollback()
			return err
		}
	} else if err != nil {
		_ = u.Rollback()
		return err
	}
	for _, front := range domain.Fronts {
		for _, status := range []domain.TaskStatus{domain.TaskRunning, domain.TaskVerifying} {
			orphans, err := k.deps.Tasks.ListByStatus(front, status, reconcileBatch)
			if err != nil {
				_ = u.Rollback()
				return err
			}
			for i := range orphans {
				tk := &orphans[i]
				if err := k.transition(u, tk, domain.TaskReplanning, "boot-reconcile:"+string(status)); err != nil {
					_ = u.Rollback()
					return err
				}
				if err := k.transition(u, tk, domain.TaskReady, "boot-reconcile"); err != nil {
					_ = u.Rollback()
					return err
				}
			}
		}
	}
	return u.Commit()
}

// Cycle: um ciclo do loop — revive, escolhe frente (bandit), trabalha
// ou repõe, registra tudo. Erro = tentativa derrubada; o chamador loga
// e o loop segue (invariante 3).
func (k *Kernel) Cycle(ctx context.Context) error {
	k.cycle++
	cycle := k.cycle
	front := k.bandit.Pick(cycle)
	now := k.now()

	if err := k.reviveThrottled(ctx, front, now); err != nil {
		return err
	}

	tk, err := k.deps.Tasks.NextReady(front, now)
	switch {
	case err == nil:
		return k.workTask(ctx, cycle, front, tk, now)
	case errors.Is(err, port.ErrNotFound):
		return k.emptyQueue(ctx, cycle, front, now)
	default:
		return fmt.Errorf("kernel: fila da frente %s: %w", front, err)
	}
}

// reviveThrottled: THROTTLED da frente re-armam a READY com espera
// local — recovery automático quando a cota volta, sem intervenção.
func (k *Kernel) reviveThrottled(ctx context.Context, front string, now time.Time) error {
	items, err := k.deps.Tasks.ListByStatus(front, domain.TaskThrottled, reviveBatch)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	u, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	for i := range items {
		tk := &items[i]
		later := now.Add(throttleRetryDelay)
		tk.NotBefore = &later
		if err := k.transition(u, tk, domain.TaskReady, "revive:quota-retry"); err != nil {
			_ = u.Rollback()
			return err
		}
	}
	return u.Commit()
}

// workTask: claim (changeset curto) → microturn SEM transação aberta →
// desfecho: step + transições + verificação determinística + filho +
// telemetria, tudo num changeset.
func (k *Kernel) workTask(ctx context.Context, cycle int64, front string, tk *domain.Task, now time.Time) error {
	u, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	if err := k.transition(u, tk, domain.TaskRunning, "claim"); err != nil {
		_ = u.Rollback()
		return err
	}
	tk.Attempt++
	if err := k.deps.Tasks.Put(u, tk); err != nil {
		_ = u.Rollback()
		return err
	}
	if err := u.Commit(); err != nil {
		return err
	}

	// C0: agir — sem transação aberta.
	kind := StepKindOf(front)
	input := stubInput(tk)
	started := time.Now()
	var (
		output     string
		tokensUsed int
		stepErr    error
	)
	switch kind {
	case domain.StepKindModel:
		reply, err := k.deps.Provider.Call(ctx, port.ModelCall{
			Purpose:   "stub:" + front,
			System:    "Você é um microturn serial do EON.",
			Prompt:    input,
			MaxTokens: stubModelMaxTokens,
		})
		if err != nil {
			stepErr = err
		} else {
			output, tokensUsed = reply.Text, reply.TokensUsed
		}
	default:
		output, stepErr = k.reflect(cycle, tk)
	}
	durationMs := time.Since(started).Milliseconds()

	u2, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = u2.Rollback() }() // pós-Commit é no-op

	outcome, reward := "step-failed", 0.0
	switch {
	case stepErr == nil && kind == domain.StepKindModel:
		if err := k.recordStep(u2, tk, kind, input, output, durationMs); err != nil {
			return err
		}
		if err := k.transition(u2, tk, domain.TaskVerifying, "step-done"); err != nil {
			return err
		}
		if err := verifyFakeReply(port.ModelReply{Text: output}); err != nil {
			if err := k.failTask(u2, tk, "verify:"+err.Error()); err != nil {
				return err
			}
			outcome = "verify-failed"
		} else {
			if err := k.succeedTask(u2, tk); err != nil {
				return err
			}
			outcome, reward = "succeeded", 1.0
		}
	case stepErr == nil: // capability: F2 determinística (D12)
		if err := k.recordStep(u2, tk, kind, input, output, durationMs); err != nil {
			return err
		}
		if err := k.transition(u2, tk, domain.TaskVerifying, "step-done"); err != nil {
			return err
		}
		if err := verifyReflect(output, cycle); err != nil {
			if err := k.failTask(u2, tk, "verify:"+err.Error()); err != nil {
				return err
			}
			outcome = "verify-failed"
		} else {
			if err := k.succeedTask(u2, tk); err != nil {
				return err
			}
			outcome, reward = "succeeded", 1.0
		}
	case errors.Is(stepErr, domain.ErrQuotaExceeded):
		if err := k.transition(u2, tk, domain.TaskThrottled, "quota-exhausted"); err != nil {
			return err
		}
		outcome = "throttled"
	case errors.Is(stepErr, domain.ErrTransient):
		if err := k.transition(u2, tk, domain.TaskReplanning, "transient"); err != nil {
			return err
		}
		later := k.now().Add(transientBackoff)
		tk.NotBefore = &later
		if err := k.transition(u2, tk, domain.TaskReady, "transient-retry"); err != nil {
			return err
		}
		outcome = "transient"
	default:
		if err := k.failTask(u2, tk, "step-error:"+stepErr.Error()); err != nil {
			return err
		}
	}

	if err := k.settle(u2, cycle, front, outcome, tk.ID, reward, tokensUsed); err != nil {
		return err
	}
	return u2.Commit()
}

// emptyQueue: C4 — fila da frente vazia. Trabalho ESPERANDO (READY com
// not_before futuro, THROTTLED) não é fila vazia, é espera legítima.
// Fila de verdade vazia → replenishment; replenishment falhando →
// CONTINUITY_BLOCKED diagnosticado — nunca idle silencioso.
func (k *Kernel) emptyQueue(ctx context.Context, cycle int64, front string, now time.Time) error {
	waiting, err := k.hasWaiting(front)
	if err != nil {
		return err
	}
	if waiting {
		return k.finishCycle(ctx, cycle, front, "waiting", "", 0, 0)
	}

	u, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = u.Rollback() }()
	tk, err := k.deps.Replenisher(u, front)
	if err != nil {
		// A tentativa caiu; o diagnóstico fica registrado e o loop segue.
		_ = u.Rollback()
		u2, err2 := k.deps.Begin(ctx)
		if err2 != nil {
			return err2
		}
		if err2 := k.blockedEpisode(u2, cycle, front, err); err2 != nil {
			_ = u2.Rollback()
			return err2
		}
		return u2.Commit()
	}
	if err := k.deps.Tasks.Put(u, tk); err != nil {
		return err
	}
	if _, err := k.deps.Events.Append(u, "task", tk.ID, domain.EventTaskCreated, mustJSON(tk)); err != nil {
		return err
	}
	if err := k.transition(u, tk, domain.TaskReady, "replenished"); err != nil {
		return err
	}
	if err := k.settle(u, cycle, front, "replenished", tk.ID, 0, 0); err != nil {
		return err
	}
	return u.Commit()
}

func (k *Kernel) hasWaiting(front string) (bool, error) {
	ready, err := k.deps.Tasks.ListByStatus(front, domain.TaskReady, 1)
	if err != nil {
		return false, err
	}
	if len(ready) > 0 {
		return true, nil
	}
	throttled, err := k.deps.Tasks.ListByStatus(front, domain.TaskThrottled, 1)
	if err != nil {
		return false, err
	}
	return len(throttled) > 0, nil
}

// finishCycle: ciclo sem efeito de estado — só telemetria (waiting).
func (k *Kernel) finishCycle(ctx context.Context, cycle int64, front, outcome, taskID string, reward float64, tokens int) error {
	u, err := k.deps.Begin(ctx)
	if err != nil {
		return err
	}
	if err := k.settle(u, cycle, front, outcome, taskID, reward, tokens); err != nil {
		_ = u.Rollback()
		return err
	}
	return u.Commit()
}

// blockedEpisode: o diagnóstico C4 no event log, com a causa crua.
func (k *Kernel) blockedEpisode(u port.Unit, cycle int64, front string, cause error) error {
	payload := mustJSON(map[string]any{"cycle": cycle, "front": front, "cause": cause.Error()})
	if _, err := k.deps.Events.Append(u, "kernel", "kernel", domain.EventContinuityBlocked, payload); err != nil {
		return err
	}
	return k.settle(u, cycle, front, "continuity-blocked", "", 0, 0)
}

// settle: telemetria do ciclo (event kernel.cycle) + recompensa do braço
// + contador persistido — no MESMO changeset do desfecho (D03/D26).
func (k *Kernel) settle(u port.Unit, cycle int64, front, outcome, taskID string, reward float64, tokens int) error {
	payload := mustJSON(map[string]any{
		"cycle": cycle, "front": front, "outcome": outcome,
		"task": taskID, "reward": reward, "tokens": tokens,
	})
	if _, err := k.deps.Events.Append(u, "kernel", "kernel", domain.EventKernelCycle, payload); err != nil {
		return err
	}
	if err := k.bandit.Record(u, cycle, front, reward); err != nil {
		return err
	}
	return k.deps.KV.Set(u, kernelCycleKey, mustJSON(cycle))
}

// transition: valida contra a tabela (invariante 2), persiste a task e o
// event NA MESMA transação. Transição ilegal é BUG do kernel — barulho
// alto, nunca silêncio.
func (k *Kernel) transition(u port.Unit, tk *domain.Task, to domain.TaskStatus, reason string) error {
	if !domain.CanTransition(tk.Status, to) {
		return fmt.Errorf("kernel: transição ilegal %s→%s (task %s, motivo %q)", tk.Status, to, tk.ID, reason)
	}
	from := tk.Status
	tk.Status = to
	tk.UpdatedAt = k.now()
	if err := k.deps.Tasks.Put(u, tk); err != nil {
		return err
	}
	payload := mustJSON(map[string]string{"from": string(from), "to": string(to), "reason": reason})
	_, err := k.deps.Events.Append(u, "task", tk.ID, domain.EventTaskTransition, payload)
	return err
}

// succeedTask: VERIFYING→SUCCEEDED + candidato-filho de expansão (C1).
func (k *Kernel) succeedTask(u port.Unit, tk *domain.Task) error {
	if err := k.transition(u, tk, domain.TaskSucceeded, "verified"); err != nil {
		return err
	}
	_, err := k.child(u, tk.Front, tk.ID, "expansion")
	return err
}

// failTask: →FAILED + candidato-filho de reposição — fracasso da tarefa
// NUNCA é fracasso do autômato (invariante 3); a proveniência do morto
// vive no filho.
func (k *Kernel) failTask(u port.Unit, tk *domain.Task, reason string) error {
	if err := k.transition(u, tk, domain.TaskFailed, reason); err != nil {
		return err
	}
	_, err := k.child(u, tk.Front, tk.ID, "replacement")
	return err
}

// child: candidato-filho — nasce NEW, vira READY no mesmo changeset do
// desfecho (C1: a fila nunca depende de sorte).
func (k *Kernel) child(u port.Unit, front, parentID, why string) (*domain.Task, error) {
	tk, err := newStubTask(u, k.deps.KV, front, parentID, k.now())
	if err != nil {
		return nil, err
	}
	if err := k.deps.Tasks.Put(u, tk); err != nil {
		return nil, err
	}
	if _, err := k.deps.Events.Append(u, "task", tk.ID, domain.EventTaskCreated, mustJSON(tk)); err != nil {
		return nil, err
	}
	if err := k.transition(u, tk, domain.TaskReady, "child:"+why+":"+parentID); err != nil {
		return nil, err
	}
	return tk, nil
}

// recordStep: o microturn vira step com hashes (citação auditável), no
// MESMO changeset do desfecho.
func (k *Kernel) recordStep(u port.Unit, tk *domain.Task, kind, input, output string, durationMs int64) error {
	existing, err := k.deps.Steps.ByTask(tk.ID)
	if err != nil {
		return err
	}
	st := &domain.Step{
		ID:         fmt.Sprintf("%s.s%02d", tk.ID, len(existing)+1),
		TaskID:     tk.ID,
		Seq:        len(existing) + 1,
		Kind:       kind,
		Input:      input,
		InputHash:  sha256Hex(input),
		Output:     output,
		OutputHash: sha256Hex(output),
		DurationMs: durationMs,
		CreatedAt:  k.now(),
	}
	if err := k.deps.Steps.Put(u, st); err != nil {
		return err
	}
	_, err = k.deps.Events.Append(u, "step", st.ID, domain.EventStepRecorded, mustJSON(st))
	return err
}

// DrainOutbox: efeitos externos pendentes → done. M1: o efeito real
// (git push) chega no M3; hoje se prova o MECANISMO — marca + event no
// mesmo changeset. Erro volta pro chamador: próximo ciclo tenta de novo.
func (k *Kernel) DrainOutbox(ctx context.Context) error {
	items, err := k.deps.Outbox.Pending(drainBatch)
	if err != nil {
		return err
	}
	for _, it := range items {
		u, err := k.deps.Begin(ctx)
		if err != nil {
			return err
		}
		if err := k.deps.Outbox.MarkDone(u, it.ID); err != nil {
			_ = u.Rollback()
			return err
		}
		payload := mustJSON(map[string]string{"id": it.ID, "kind": it.Kind})
		if _, err := k.deps.Events.Append(u, "outbox", it.ID, domain.EventOutboxDrained, payload); err != nil {
			_ = u.Rollback()
			return err
		}
		if err := u.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// reflect: o microturn determinístico da F2 (Ciclo 2 acelerado quando a
// cota morre, D12) — consolida o ciclo sem gastar modelo.
func (k *Kernel) reflect(cycle int64, tk *domain.Task) (string, error) {
	sum := sha256.Sum256([]byte(fmt.Sprintf("refl|%d|%s|%d", cycle, tk.ID, tk.Attempt)))
	return fmt.Sprintf("refl:%d:%s", cycle, hex.EncodeToString(sum[:8])), nil
}

// verifyReflect: verificador determinístico (invariante 2) — formato
// ^refl:<ciclo>:<16 hex>$ com o ciclo certo.
func verifyReflect(output string, cycle int64) error {
	parts := strings.SplitN(output, ":", 3)
	if len(parts) != 3 || parts[0] != "refl" {
		return fmt.Errorf("saída fora do formato: %q", output)
	}
	got, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || got != cycle {
		return fmt.Errorf("ciclo errado na reflexão: %q queria %d", output, cycle)
	}
	if len(parts[2]) != 16 || !isLowerHex(parts[2]) {
		return fmt.Errorf("hash da reflexão inválido: %q", output)
	}
	return nil
}

// stubInput: o input do microturn stub — igual no provider e no step
// (o que o modelo viu é o que o registro diz que foi visto).
func stubInput(tk *domain.Task) string {
	return fmt.Sprintf("task %s — %s — critério: %s", tk.ID, tk.Title, tk.StopCondition)
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// mustJSON: payloads de kernel sempre marshalizam (structs de dados
// puros); o fallback mantém o event VIVO com a marca de erro — evento
// legível vale mais que ciclo morto.
func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"marshal":"erro"}`)
	}
	return b
}
