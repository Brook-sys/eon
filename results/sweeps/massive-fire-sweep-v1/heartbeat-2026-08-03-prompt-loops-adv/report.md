# Prompt-improvement loop — adversarial baseline failures (2026-08-03)

Fecho do ciclo anterior (interrompido antes do loop de conflito). Fonte das falhas:
sweep `heartbeat-2026-08-03-adv-baseline-a` (180 calls) catalogou 0/10 em
`adv-ambiguous-instruction` no `llama-3.3-70b-versatile` e 0/10 em
`adv-conflicting-data` no `llama-3.1-8b-instant` (allam-2-7b ~20%).

## Setup (bounded)

- 2 campanhas × 40 chamadas = 80 chamadas live reais (Groq), temp 0.0,
  max_tokens 48, 5 reps/célula, sem retry em falha semântica.
- Baseline v0 + 3 variantes de prompt por falha (prompt-improvement loop do HEARTBEAT.md).

## Loop 1 — adv-ambiguous-instruction (`ambiguous.json`)

- llama-3.3-70b-versatile: v0 **0/5** (ignora o formato DATE:/SOURCE: e responde
  prosa — falha de *formato sob ambiguidade*, não semântica: a escolha S-17/2025-11-03
  estava correta na prosa).
- Correção: qualquer restrição decisiva (v1 resolve-commit, v2 decisive, v3 few-shot)
  recupera **5/5**. Subvariante mais barata (v1) basta.
- llama-3.1-8b-instant (controle): baseline já 5/5; v3 few-shot **0/5** — few-shot
  copy-paste *regrediu* o 8B: o modelo repetiu o bloco de fatos do exemplo em vez de
  responder no formato (poisoning por exemplar; cenário adverso nº 8 do programa,
  confirmado in vivo).

## Loop 2 — adv-conflicting-data (`conflict.json`)

- llama-3.1-8b-instant: v0 **0/5** — dois modos simultâneos: `CONFLICT: NO`
  (aceita o hint "O-3 é consistente com O-1" como se resolvesse o par O-1/O-2) e
  stripping do prefixo `O-` (`PAIR: 1/2`). Todas as variantes v1/v2/v3 fecham **5/5**;
  v1 (par explícito + restatement do prefixo) é a mais barata.
- allam-2-7b: v0 **0/5** e v1 **0/5** (resposta bare `NO`, ignora o header de 2 linhas);
  v2 noise-label **5/5** e v3 few-shot **5/5**. Modelo de 7B precisa que o decoy seja
  explicitamente demovido, não basta nomear o par.

## Decisões (evidência → ação)

1. `adv-ambiguous-instruction`: instrução de formato precisa de commitment explícito
   ("you MUST respond ONLY with the two lines, no prose") — falha era formato, não
   compreensão. Variante v1 promovida ao sweep baseline.
2. `adv-conflicting-data`: nomear o par em disputa + restatement verbatim do prefixo
   fecha ambos os modos (semântico e formato) — variante v1 promovida. Para modelos
   ≤7B, adicionalmente demover explicitamente observações decoy (padrão v2).
3. Few-shot **não é contra-medida universal**: confirma poisoning em 8B (regressão
   5/5→0/5). Few-shot entra no sweep apenas quando a baseline falha inicial exige.

## Próximo

- Rerun do adv-baseline com os prompts v1 promovidos para medir melhoria
  (0/10 → alvo ≥8/10 nas duas células).
- Cobrir os cenários adversos restantes (prompt injection nos dados, degradação PT/EN,
  budget starvation, CoT poisoning) com a mesma disciplina de loop.
