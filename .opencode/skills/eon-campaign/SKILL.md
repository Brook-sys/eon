---
name: eon-campaign
description: Protocolo de campanha de modelo live no EON v2 (hipótese explícita, limites rígidos, harness único, relatório versionado). Use para qualquer experimento com modelo real que informe decisões do projeto. Campanha NUNCA muda estado oficial sozinha — vira Evidence/Lesson (Constituição Art. 5).
---

# eon-campaign — evidência, não autoridade

## Regras fundamentais

1. **Um harness, zero clones**: toda campanha roda pelo harness parametrizado único (futuro `eonctl campaign`; hoje, scripts versionados no repo). Criar `cmd/experimento-N` é a autópsia da v1 se repetindo.
2. **Campanha live NÃO roda no CI** — CI usa fixtures/replays herméticos.
3. **Resultado nunca muda estado oficial sozinho** — vira Lesson/Evidence que alimenta uma decisão com ADR.

## Checklist PRÉ (obrigatório antes de gastar quota)

1. [ ] **Hipótese em uma frase, falsificável** (ex.: "formato delimitado reduz parse failure vs JSON puro no perfil X").
2. [ ] **Limites rígidos declarados**: N chamadas máximo, timeout por chamada, custo máximo, critério de aborto antecipado.
3. [ ] **Formato de relatório versionado** definido ANTES de rodar (campos fixos; sem pós-hoc criativo).
4. [ ] **Baseline/controle** definido (com o que a hipótese compara?).
5. [ ] **Variáveis isoladas**: uma coisa muda por campanha (perfil, formato, temperatura, decomposição — NUNCA duas).
6. [ ] Seed/inputs congelados no relatório (reproduzível).

## Checklist EXECUÇÃO

1. [ ] Rodar pelo harness com os limites declarados; nenhum "só mais uma rodada" além do N.
2. [ ] Falha de provider tratada por classe (D29), não como resultado da hipótese (429 ≠ "modelo falhou na tarefa").
3. [ ] Log bruto preservado em artifact (fora do git se grande; commit só o relatório sumarizado).

## Checklist PÓS

1. [ ] Relatório no formato versionado (docs/knowledge/reports/ quando a F1 existir; antes, docs/journal/ + commit).
2. [ ] Veredito: hipótese confirmada / refutada / inconclusiva (com N e limites citados).
3. [ ] Se confirmada e estrutural → skill `eon-adr` com a Evidence como origem.
4. [ ] Se refutada → Lesson (o fracasso documentado é o motor da F3; invariante 10).
5. [ ] Inconclusiva → declarar o que faltaria; não "empurrar" para confirmada.

## Proibições

- ❌ Campanha sem hipótese pré-declarada ("vamos ver o que o modelo faz").
- ❌ Mudar formato de relatório pós-execução.
- ❌ Chery-picking de rodadas (toda rodada conta, inclusive as feias).
- ❌ Usar resultado de campanha como "verificado" — verificação é o gate (skill `eon-verify`).
