package core

import (
	"errors"
	"io"
	"log"
	"testing"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

func quietLog() *log.Logger { return log.New(io.Discard, "", 0) }

// fakeKV: fake in-memory do port.KVRepo/Unit para os testes de CORE —
// core NÃO importa storage (nem em teste; ADR-0002). A durabilidade real
// (WAL, reopen) é provada no contract suite do storage; aqui se prova o
// CONTRATO do bandit contra o port. Simplificação documentada: Set aplica
// na hora (rollback do fake não desfaz — o suite de storage cobre a
// semântica transacional de verdade).
type fakeUnit struct{}

func (fakeUnit) Commit() error   { return nil }
func (fakeUnit) Rollback() error { return nil }

type fakeKV struct{ m map[string][]byte }

func newFakeKV() *fakeKV { return &fakeKV{m: map[string][]byte{}} }
func (k *fakeKV) Set(_ port.Unit, key string, v []byte) error {
	k.m[key] = append([]byte(nil), v...)
	return nil
}
func (k *fakeKV) Get(key string) ([]byte, error) {
	v, ok := k.m[key]
	if !ok {
		return nil, errors.New("ausente")
	}
	return append([]byte(nil), v...), nil
}

// runBandit: roda n ciclos com a política de recompensa dada; devolve a
// sequência de picks e a contagem por frente.
func runBandit(b *Bandit, n int64, rewardFor func(front string) float64) ([]string, map[string]int) {
	picks := []string{}
	count := map[string]int{}
	for c := int64(1); c <= n; c++ {
		f := b.Pick(c)
		picks = append(picks, f)
		count[f]++
		if err := b.Record(fakeUnit{}, c, f, rewardFor(f)); err != nil {
			panic(err)
		}
	}
	return picks, count
}

// TestBanditDeterministic: mesma sequência de recompensas → mesmos picks,
// em INSTÂNCIAS DIFERENTES. Sem RNG, sem relógio (D26).
func TestBanditDeterministic(t *testing.T) {
	reward := func(f string) float64 {
		if f == domain.FrontProject {
			return 1.0 // só F1 prospera
		}
		return 0
	}
	a, _ := runBandit(NewBandit(newFakeKV(), domain.Fronts, quietLog()), 40, reward)
	b, _ := runBandit(NewBandit(newFakeKV(), domain.Fronts, quietLog()), 40, reward)
	if len(a) != 40 || len(b) != 40 {
		t.Fatalf("queria 40 picks: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("não determinístico no ciclo %d: %s vs %s", i+1, a[i], b[i])
		}
	}
}

// TestBanditJusticeFloor: braço de recompensa ZERO ainda é escolhido ao
// menos 1 vez por K ciclos (D26 — anti-starvation), e o vencedor domina.
func TestBanditJusticeFloor(t *testing.T) {
	b := NewBandit(newFakeKV(), domain.Fronts, quietLog())
	reward := func(f string) float64 {
		if f == domain.FrontProject {
			return 1.0
		}
		return 0
	}
	_, count := runBandit(b, int64(banditJusticeK*3), reward)
	for _, f := range domain.Fronts {
		if count[f] == 0 {
			t.Fatalf("piso de justiça falhou: %s nunca foi escolhida em %d ciclos (picks=%v)",
				f, banditJusticeK*3, count)
		}
	}
	if count[domain.FrontProject] < banditJusticeK {
		t.Fatalf("braço vencedor deveria dominar: picks=%v", count)
	}
}

// TestBanditPrefersWinningArm: fora do piso de justiça, o braço com
// recompensa consistente vence.
func TestBanditPrefersWinningArm(t *testing.T) {
	b := NewBandit(newFakeKV(), domain.Fronts, quietLog())
	reward := func(f string) float64 {
		if f == domain.FrontThoughts {
			return 1.0
		}
		return 0
	}
	runBandit(b, 60, reward)
	// Desarma o piso: pick recente para os perdedores — senão Pick(61) é
	// o DIREITO do faminto, não a preferência do UCB (D26 manda os dois
	// existirem).
	for _, loser := range []string{domain.FrontProject, domain.FrontSelf} {
		if err := b.Record(fakeUnit{}, 60, loser, 0.0); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Pick(61); got != domain.FrontThoughts {
		t.Fatalf("Pick(61)=%s, queria o braço consistente %s", got, domain.FrontThoughts)
	}
}

// TestBanditPersistsAcrossRehydrate: o estado hidrata do que foi
// persistido — crash perde no máximo a recompensa em voo, nunca a
// história (a durabilidade do kv é lei do storage, provada lá).
func TestBanditPersistsAcrossRehydrate(t *testing.T) {
	kv := newFakeKV()
	b1 := NewBandit(kv, domain.Fronts, quietLog())
	runBandit(b1, 25, func(string) float64 { return 1.0 })
	last := b1.Pick(26)

	b2 := NewBandit(kv, domain.Fronts, quietLog()) // "reopen" no mesmo kv
	if got := b2.Pick(26); got != last {
		t.Fatalf("rehydrate mudou a decisão: %s vs %s", got, last)
	}
}

// TestBanditCorruptedStateResets: estado ilegível → recomeça limpo, sem
// derrubar NADA (falha derruba a tentativa, nunca o autômato).
func TestBanditCorruptedStateResets(t *testing.T) {
	kv := newFakeKV()
	if err := kv.Set(fakeUnit{}, banditKVKey, []byte("não é json")); err != nil {
		t.Fatal(err)
	}
	b := NewBandit(kv, domain.Fronts, quietLog())
	if got := b.Pick(1); got != domain.FrontProject {
		t.Fatalf("estado corrompido deveria recomeçar limpo (pick canônico), veio %s", got)
	}
}
