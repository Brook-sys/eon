// bandit.go — scheduler v1: UCB1 com janela descontada sobre 3 braços =
// frentes (D26). Determinístico por construção: só estatísticas
// persistidas, zero RNG — mesma sequência de recompensas, mesmos picks
// (auditável, reproduzível).
//
// Recompensa M1: o kernel entrega progresso-verificado-por-custo já em
// [0,1] (sucesso=1, throttle/falha=0 — binário no mundo fake). A
// normalização robusta da D26 (mediana+MAD contra a própria história)
// entra no M2, quando recompensas têm escala real; com binário ela é
// identidade e código morto não é honestidade.
package core

import (
	"encoding/json"
	"fmt"
	"log"
	"math"

	"github.com/brook-sys/eon/internal/port"
)

// Constantes M1 (recalibrar é ADR, não flag mágica):
const (
	banditKVKey    = "bandit.state"
	banditWindow   = 20  // recompensas recentes por braço
	banditGamma    = 0.9 // desconto por idade dentro da janela
	banditExploreC = 1.0 // constante de exploração UCB
	banditJusticeK = 20  // piso de justiça: ≥1 pick por K ciclos (D26)
)

type armStats struct {
	Rewards       []float64 `json:"rewards"` // mais recente no fim
	Picks         int64     `json:"picks"`
	LastPickCycle int64     `json:"last_pick_cycle"`
}

type banditState struct {
	TotalCycles int64                `json:"total_cycles"`
	Arms        map[string]*armStats `json:"arms"`
}

// Bandit: seleção de frente por ciclo. Estado em memória hidratado do
// kv_meta no boot, persistido NA UNIDADE do changeset do ciclo — crash
// perde no máximo a recompensa do ciclo em voo, nunca a consistência.
type Bandit struct {
	kv     port.KVRepo
	fronts []string
	state  banditState
	log    *log.Logger
}

// NewBandit hidrata do kv (estado novo se nunca houve).
func NewBandit(kv port.KVRepo, fronts []string, lg *log.Logger) *Bandit {
	b := &Bandit{kv: kv, fronts: fronts, log: lg}
	b.state.Arms = map[string]*armStats{}
	for _, f := range fronts {
		b.state.Arms[f] = &armStats{}
	}
	if raw, err := kv.Get(banditKVKey); err == nil {
		if err := json.Unmarshal(raw, &b.state); err != nil {
			// estado corrompido: recomeça limpo — o bandit é estatística,
			// não estado canônico; perder a janela não perde a missão.
			lg.Printf("bandit: estado corrompido, recomeçando limpo: %v", err)
			b.state = banditState{Arms: map[string]*armStats{}}
			for _, f := range fronts {
				b.state.Arms[f] = &armStats{}
			}
		} else {
			// frentes novas (ou removidas) entraram desde o último boot
			for _, f := range fronts {
				if b.state.Arms[f] == nil {
					b.state.Arms[f] = &armStats{}
				}
			}
		}
	}
	return b
}

// Pick: a frente do próximo ciclo. Ordem de decisão:
//  1. piso de justiça — braço não escolhido há >= K ciclos vence (D26);
//  2. UCB1 descontado — média da janela com decaimento + bônus de
//     exploração; empate resolve pela ordem canônica das frentes.
func (b *Bandit) Pick(cycle int64) string {
	// 1. justiça: o mais faminto primeiro; empate pela ordem canônica.
	var starved string
	for _, f := range b.fronts {
		if cycle-b.state.Arms[f].LastPickCycle >= banditJusticeK {
			if starved == "" || firstInOrder(f, starved, b.fronts) {
				starved = f
			}
		}
	}
	if starved != "" {
		return starved
	}
	// 2. UCB1.
	best, bestScore := "", math.Inf(-1)
	for _, f := range b.fronts {
		s := b.ucb(f, cycle)
		if s > bestScore {
			best, bestScore = f, s
		}
	}
	return best
}

func (b *Bandit) ucb(front string, cycle int64) float64 {
	a := b.state.Arms[front]
	if a == nil {
		return math.Inf(-1)
	}
	// média descontada: r_i pesada por γ^(idade desde o fim da janela).
	var sum, wsum float64
	for i, r := range a.Rewards {
		age := len(a.Rewards) - 1 - i
		w := math.Pow(banditGamma, float64(age))
		sum += w * r
		wsum += w
	}
	mean := 0.0
	if wsum > 0 {
		mean = sum / wsum
	}
	bonus := banditExploreC * math.Sqrt(math.Log(float64(cycle+1))/(float64(a.Picks+1)))
	return mean + bonus
}

// Record: registra o pick e a recompensa do ciclo, persistindo o estado
// NA UNIDADE do changeset do ciclo (D03: decisão e efeito atômicos).
// reward em [0,1]; window cap mantém só as banditWindow mais recentes.
func (b *Bandit) Record(u port.Unit, cycle int64, front string, reward float64) error {
	a := b.state.Arms[front]
	if a == nil {
		return fmt.Errorf("bandit: frente desconhecida %q", front)
	}
	a.Picks++
	a.LastPickCycle = cycle
	a.Rewards = append(a.Rewards, reward)
	if len(a.Rewards) > banditWindow {
		a.Rewards = a.Rewards[len(a.Rewards)-banditWindow:]
	}
	b.state.TotalCycles = cycle
	raw, err := json.Marshal(b.state)
	if err != nil {
		return err
	}
	return b.kv.Set(u, banditKVKey, raw)
}

func firstInOrder(a, b string, order []string) bool {
	for _, f := range order {
		if f == a {
			return true
		}
		if f == b {
			return false
		}
	}
	return false
}
