// stubgen.go — gerador de trabalho stub do M1: as frentes são cascas
// por desenho do marco (a missão F1 real chega no M2). Stub NÃO é
// desculpa para task mentirosa — toda task declara os campos
// obrigatórios (03-LOOP.md: proveniência, ganho esperado, novidade,
// custo, stop condition) e contador monotônico por frente no kv_meta,
// incrementado DENTRO do Unit do criador (criação e contador atômicos).
package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// StubMissionID: a missão-stub do M1 (critério de aceite do marco).
const StubMissionID = "m-stub"

// StubMission: a definição da missão-stub — criada no boot se ausente.
func StubMission(now time.Time) *domain.Mission {
	return &domain.Mission{
		ID:        StubMissionID,
		Title:     "Missão stub M1",
		Purpose:   "provar continuidade do loop: 1000+ ciclos, quota esgota, nunca para",
		Status:    domain.MissionActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// StepKindOf: o C0 da frente no M1 — F2 é a frente determinística (D12:
// cota esgotada → F2 vira full-time), F1/F3 são microturnos de modelo.
func StepKindOf(front string) string {
	if front == domain.FrontThoughts {
		return domain.StepKindCapability
	}
	return domain.StepKindModel
}

// frontAbbr: prefixo curto e ÚNICO por frente no id da task.
func frontAbbr(front string) string {
	switch front {
	case domain.FrontProject:
		return "p"
	case domain.FrontThoughts:
		return "th"
	case domain.FrontSelf:
		return "s"
	}
	return "x"
}

// nextStubCounter: contador monotônico por frente no kv_meta, incrementado
// DENTRO do Unit recebido (rollback do criador desfaz o incremento junto
// com a task — sem buracos nem duplicatas na numeração canônica).
func nextStubCounter(u port.Unit, kv port.KVRepo, front string) (int64, error) {
	key := "gen." + front
	var n int64
	raw, err := kv.Get(key)
	switch {
	case err == nil:
		if err := json.Unmarshal(raw, &n); err != nil {
			return 0, fmt.Errorf("stub: contador %s corrompido: %w", key, err)
		}
	case errors.Is(err, port.ErrNotFound):
		// primeira task da frente — contador nasce em 1
	default:
		return 0, err
	}
	n++
	raw, err = json.Marshal(n)
	if err != nil {
		return 0, err
	}
	if err := kv.Set(u, key, raw); err != nil {
		return 0, err
	}
	return n, nil
}

// newStubTask: UMA task stub de frente, com id determinístico
// t-<front>-<n>. derivedFrom é a proveniência: "gen:<front>" para
// replenishment, o id do pai para candidatos-filho (C1).
func newStubTask(u port.Unit, kv port.KVRepo, front, derivedFrom string, now time.Time) (*domain.Task, error) {
	n, err := nextStubCounter(u, kv, front)
	if err != nil {
		return nil, err
	}
	id := fmt.Sprintf("t-%s-%05d", frontAbbr(front), n)

	var title, gain, novelty, cost, stop string
	switch front {
	case domain.FrontProject:
		title = fmt.Sprintf("estudar o domínio: passada %d", n)
		gain = "resposta verificada e citável"
		novelty = fmt.Sprintf("passada %d ainda não foi feita", n)
		cost = "1 chamada de modelo fake"
		stop = "reply no formato fake:[16 hex]"
	case domain.FrontThoughts:
		title = fmt.Sprintf("consolidação determinística %d", n)
		gain = "memória do ciclo consolidada sem gastar modelo"
		novelty = fmt.Sprintf("janela %d da reflexão", n)
		cost = "0 tokens (capability local)"
		stop = "saída no formato refl:<ciclo>:<16 hex>"
	case domain.FrontSelf:
		title = fmt.Sprintf("auto-auditoria %d", n)
		gain = "estado interno auditado e citável"
		novelty = fmt.Sprintf("auditoria %d do próprio loop", n)
		cost = "1 chamada de modelo fake"
		stop = "reply no formato fake:[16 hex]"
	default:
		return nil, fmt.Errorf("stub: frente desconhecida %q", front)
	}

	return &domain.Task{
		ID:            id,
		MissionID:     StubMissionID,
		Front:         front,
		Title:         title,
		DerivedFrom:   derivedFrom,
		ExpectedGain:  gain,
		Novelty:       novelty,
		CostEstimate:  cost,
		StopCondition: stop,
		Status:        domain.TaskNew,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, nil
}
