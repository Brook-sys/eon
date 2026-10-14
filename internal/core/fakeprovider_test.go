package core

import (
	"context"
	"errors"
	"testing"

	"github.com/brook-sys/eon/internal/domain"
	"github.com/brook-sys/eon/internal/port"
)

// O fake é a infraestrutura de prova do M1 (D39) — determinismo e cota
// são CONTRATOS do soak e do crash-replay: testados como contratos.

func TestFakeProviderDeterministic(t *testing.T) {
	p := NewFakeProvider(1000)
	call := port.ModelCall{System: "s", Prompt: "explique a serialidade", MaxTokens: 10}
	r1, err := p.Call(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	r2, err := p.Call(context.Background(), call)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Text != r2.Text {
		t.Fatalf("mesma pergunta, respostas diferentes: %q vs %q", r1.Text, r2.Text)
	}
	// prompt diferente → resposta diferente (função do input, não constante)
	r3, err := p.Call(context.Background(), port.ModelCall{System: "s", Prompt: "outra coisa", MaxTokens: 10})
	if err != nil {
		t.Fatal(err)
	}
	if r3.Text == r1.Text {
		t.Fatal("prompts diferentes produziram a mesma resposta — não é função do input")
	}
	// e o formato é o que o verificador espera
	if err := verifyFakeReply(r1); err != nil {
		t.Fatalf("reply do fake não passa no próprio verificador: %v", err)
	}
}

func TestFakeProviderQuotaExhausts(t *testing.T) {
	p := NewFakeProvider(10)
	for i := 0; i < 2; i++ {
		if _, err := p.Call(context.Background(), port.ModelCall{Prompt: "x", MaxTokens: 5}); err != nil {
			t.Fatalf("chamada %d deveria vencer: %v", i+1, err)
		}
	}
	_, err := p.Call(context.Background(), port.ModelCall{Prompt: "x", MaxTokens: 5})
	if !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("cota esgotada: erro=%v, queria ErrQuotaExceeded", err)
	}
	if p.QuotaLeft() != 0 {
		t.Fatalf("QuotaLeft=%d, queria 0", p.QuotaLeft())
	}
	// cota morta permanece morta — M1 não reabastece
	if _, err := p.Call(context.Background(), port.ModelCall{Prompt: "x", MaxTokens: 1}); !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatal("cota morta ressuscitou sozinha")
	}
}

func TestFakeProviderZeroQuotaIsDead(t *testing.T) {
	p := NewFakeProvider(0)
	_, err := p.Call(context.Background(), port.ModelCall{Prompt: "x", MaxTokens: 1})
	if !errors.Is(err, domain.ErrQuotaExceeded) {
		t.Fatalf("cota 0 deveria estar morta no boot: %v", err)
	}
}

func TestVerifyFakeReply(t *testing.T) {
	ok := port.ModelReply{Text: "fake:0123456789abcdef"}
	if err := verifyFakeReply(ok); err != nil {
		t.Fatalf("formato válido reprovado: %v", err)
	}
	bad := []string{
		"",                       // vazio
		"fake:",                  // sem sufixo
		"fake:0123456789ABCDEF",  // hex maiúsculo
		"fake:zzzzzzzzzzzzzzzz",  // não é hex
		"real:0123456789abcdef",  // prefixo errado
		"fake:0123456789abcde",   // 15 chars
		" fake:0123456789abcdef", // espaço
	}
	for _, s := range bad {
		if err := verifyFakeReply(port.ModelReply{Text: s}); err == nil {
			t.Errorf("formato inválido aceito: %q", s)
		}
	}
}
