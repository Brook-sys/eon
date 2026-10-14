// fakeprovider.go — provider fake determinístico do M1 (D39: CI/soak
// reprodutível; o adapter real OpenAI-compat chega no M2, ADR-0015).
//
// Determinismo: a resposta é FUNÇÃO do input (sha256) — mesmo prompt,
// mesma resposta, para sempre. Cota: orçamento TOTAL em tokens que não
// reabastece — o regime "cota esgotada → F2 full-time determinística"
// (D12) fica exercitado em produção de verdade, não só em teste.
//
// Formato da resposta é contrato do M1: ^fake:[0-9a-f]{16}$ — o
// verificador determinístico (invariante 2) mora aqui do lado.
package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// FakeProvider implementa port.ModelProvider.
type FakeProvider struct {
	mu          sync.Mutex
	totalTokens int
	used        int
	calls       int64
}

// NewFakeProvider: orçamento total em tokens. 0 = cota já morta no boot
// (regime D12 puro, usado pelos testes).
func NewFakeProvider(totalTokens int) *FakeProvider {
	return &FakeProvider{totalTokens: totalTokens}
}

// Call: UM microturn (D10 — serial por construção no kernel; o mutex é
// honestidade contra o futuro, não licença de paralelismo). Cota morta →
// domain.ErrQuotaExceeded. Erro transitório NÃO existe no fake — é
// injetado por providers de erro em teste específico.
func (p *FakeProvider) Call(_ context.Context, c port.ModelCall) (port.ModelReply, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.used >= p.totalTokens {
		return port.ModelReply{}, domain.ErrQuotaExceeded
	}
	cost := c.MaxTokens
	if cost < 1 {
		cost = 1
	}
	if p.used+cost > p.totalTokens {
		cost = p.totalTokens - p.used // a última chamada vence cortada, como no real
	}
	p.used += cost
	p.calls++
	sum := sha256.Sum256([]byte(c.System + "|" + c.Prompt))
	return port.ModelReply{
		Text:       "fake:" + hex.EncodeToString(sum[:8]),
		TokensUsed: cost,
	}, nil
}

// QuotaLeft/Used/Calls: telemetria para kernel, eonctl e testes.
func (p *FakeProvider) QuotaLeft() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.totalTokens - p.used
}

func (p *FakeProvider) Used() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.used
}

func (p *FakeProvider) Calls() int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// verifyFakeReply: o verificador determinístico do passo de modelo M1
// (invariante 2: código decide, não opinião). Formato: fake: + 16 hex.
func verifyFakeReply(reply port.ModelReply) error {
	if len(reply.Text) != len("fake:")+16 {
		return fmt.Errorf("reply fora do formato: %q", reply.Text)
	}
	if reply.Text[:5] != "fake:" {
		return fmt.Errorf("reply não começa com fake:: %q", reply.Text)
	}
	if !isLowerHex(reply.Text[5:]) {
		return fmt.Errorf("sufixo não é hex minúsculo: %q", reply.Text)
	}
	return nil
}

func isLowerHex(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
