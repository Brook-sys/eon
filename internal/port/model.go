// model.go — o contrato MÍNIMO de provider do M1: um método, uma chamada
// em voo (D10). O adapter real OpenAI-compat (catálogo multi-provedor,
// ADR-0015; perfis de dialeto, D05) chega no M2 POR ADR — interface sem
// segundo lado é invenção (port/doc.go).
package port

import (
	"context"
)

// ModelCall: UM microturn — o modelo vê só o passo (03-LOOP.md).
type ModelCall struct {
	Purpose   string // telemetria/calibração (M2); não vai ao modelo
	System    string
	Prompt    string
	MaxTokens int
}

// ModelReply: resposta do provider (M1: texto do fake; M2 adiciona
// parse/formatos por ADR).
type ModelReply struct {
	Text       string
	TokensUsed int
}

// ModelProvider: chamada serial de modelo. Erros canônicos do pacote
// domain: ErrQuotaExceeded (→ Ciclo 2 acelera) e ErrTransient (→ retry
// com backoff). Tudo mais é falha do passo — derruba a tentativa, nunca
// o autômato (invariante 3).
type ModelProvider interface {
	Call(ctx context.Context, c ModelCall) (ModelReply, error)
}
