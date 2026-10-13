// Package exec é a capability de execução: todo exec externo passa pelo
// sandbox cego via worker L3 (D09/ADR-0011) — nunca direto do core.
//
// Lei (ADR-0002): importa port — nunca core.
//
// M0: vazio por desenho (M3). O worker L3 (internal/worker) já existe e é
// testado desde o M0.
package exec
