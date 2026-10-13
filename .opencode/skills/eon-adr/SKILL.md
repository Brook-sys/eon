---
name: eon-adr
description: Escrever um ADR (Registro de Decisão Arquiteturais) no EON v2. Use ANTES de implementar qualquer decisão estrutural — novo conceito de domínio, nova frente, mudança de fronteira de módulo, storage, protocolo (Constituição Art. 3). Nunca edite docs/plan/ (histórico congelado).
---

# eon-adr — decisão viva nasce aqui

## Checklist

1. **É decisão estrutural?** Conceito de domínio novo, frente, fronteira de módulo, storage, protocolo, dependência externa (D19), mudança de invariante → precisa ADR. Se é só implementação de algo já decidido, NÃO precisa (cite o ADR/decisão existente no commit).
2. **Conflito?** Leia `docs/adr/README.md` (índice) e `docs/plan/00-DECISIONS.md` (D01–D55, histórico). ADR novo que contradiz ADR existente deve declarar `superseded-by` explicitamente — nunca conviver em silêncio.
3. **Scaffold**: copie `docs/adr/ADR-0000-template.md` para `docs/adr/ADR-NNNN-<slug>.md` (próximo número livre; nunca reutilize número).
4. **Preencha**: Status (`proposto` ou `aceito` + data) / Origem (decision numbers ou problema concreto) / Contexto (a tensão) / Decisão (voz ativa, específica o bastante para ser enforçada) / Consequências (o que fica melhor, o que se paga, como se reverte).
5. **Atualize o índice** `docs/adr/README.md` (uma linha na tabela).
6. **Gate**: `go run ./tools/boundary` (docs não quebram fronteiras, mas o hábito é o hábito). Commit separado: `docs(adr): ADR-NNNN <título>`.

## Regras

- Um ADR = uma decisão. Não empilhe decisões num ADR só.
- Cite decisões de origem por número (Dxx) com o texto verificado de `docs/plan/00-DECISIONS.md` — NUNCA invente número.
- ADR não é blog: sem narrativa, sem "consideramos considerar". Tensão → decisão → consequência.
- Toda decisão precisa ser reversível NA PRÁTICA — descreva o caminho de volta.
- Se você está escrevendo um ADR para justificar algo que JÁ fez sem ADR: pare, escreva o ADR como `proposto`, e diga isso claramente na entrada do journal.
