# 07 — Auto-Melhoria (F3): Pipeline, Segurança e Git

> O coração do estudo de caso. Fundamentação: Gödel Machine (ideal impraticável) → Darwin Gödel Machine (validação empírica + arquivo de variantes) → nossa versão com modelos fracos: propostas pequenas, gates determinísticos, observação mensurada, reversibilidade total (D16–D19).

## Estados da `Improvement`

```
DRAFT        proposta pequena e atômica (1 melhoria = 1 commit futuro)
  ↓
SANDBOXED    validação no worker cego: go build + go test no scratch copy
  ↓            (L0/L1 pulam sandbox de código; hot-apply validado por schema)
COMMITTED    commit em branch auto/improvement-N (author: eon)
  ↓
APPLIED      L0/L1: hot-apply versionado no store
             L2:    supervisor troca o binário (build do sandbox, tag)
  ↓
OBSERVED     janela de observação: métricas vs baseline (D25)
  ↓                    │
PROMOTED  ←────────────┤  merge na v2 + tag eon/vN + push origin
  ↓                    │
 (próxima etapa)       └→ ROLLED_BACK: binário anterior + git revert
                          + Lesson com análise OBRIGATÓRIA
                          (o que quebrou, hipótese de causa, invariante tocada)
```

**Regra do átomo**: 1 melhoria = 1 commit = 1 proposal. Mega-mudanças são divididas em etapas independentes. Cada etapa tem stop condition e valor isolado.

## Níveis de autonomia (teto configurável — D16)

| Nível | O que pode mudar | Gates | Aplicação |
|---|---|---|---|
| L0 | prompts e templates | schema + suíte offline de prompts (fixtures) | hot-apply versionado no store |
| L1 | políticas, parâmetros, capacidades novas (código novo em arquivo próprio, sem tocar kernel) | build + testes + aprovação por política | hot-apply versionado |
| L2 | código do harness em geral | sandbox completo + observação + supervisor swap | binário promovido pelo supervisor |

- **Teto por config** (default: L2). Elevação de nível = `Improvement` especial com evidência acumulada (taxa de promoção, lessons, métricas); humano **pode** conceder, nunca precisa — sem resposta, opera no teto atual (D14).
- O supervisor, a constituição e as regras de segurança **nunca** são alvo de melhoria autônoma. Toda proposta que os toca = rejeitada com Lesson ("fora do alcance da F3").

## Como a F3 lê o próprio código (D37)

Ferramentas determinísticas (grep, índice de símbolos via AST, métricas por pacote, resultados de testes recentes) constroem um **mapa do código**; a F3 recebe **trechos curtos e direcionados + métricas** — nunca o repo inteiro. Propostas chegam em formato estrito com **diff pequeno**. Respeita a janela útil de modelos fracos e o princípio de que o contexto é compilado, não despejado.

## Janela de observação (D25 — definida)

**Definição da janela (adaptativa com cap)**:
- **Mínimo**: N ciclos completos cobrindo cada frente (F1/F2/F3) pelo menos uma vez, sem anomalias ativas;
- **Máximo**: 48h — cap para não travar o pipeline de promoção;
- A janela **estende** (não decide) se o ambiente mudar materialmente durante ela: troca de binding de modelo, emenda de missão, incidente de storage.

**Métricas comparadas** (baseline × pós-apply, sempre as mesmas):

1. **Continuidade**: ciclos executados, tempo em CONTINUITY_BLOCKED, crashes;
2. **Progresso verificado por custo** (por frente): artifacts citados, lessons promovidas, memória comprimida — por token gasto;
3. **Qualidade**: taxa de parse success, taxa de verificação aprovada, retries;
4. **Orçamento**: burn rate vs normal;
5. **Invariantes**: qualquer violação registrada.

**Baseline**: snapshot **trailing pré-apply** (últimas 24h ou N ciclos) das mesmas métricas, gravado como **Artifact vinculado ao Improvement** — comparação apples-to-apples, auditável e reproduzível.

**Triggers de rollback (duas classes)**:
- **HARD — rollback imediato**: crash-loop do core; violação de invariante; falha de build/boot do novo binário.
- **SOFT — rollback + Lesson obrigatória**: regressão >20% de progresso-verificado-por-custo com amostra mínima suficiente; anomalia de orçamento >50% do normal.

Rollback é **operação de estado** (auditável, event-logged), não um crash: o loop continua enquanto o supervisor troca o binário.

## Protocolo de análise pós-falha (D17 — obrigatório)

Toda melhoria rejeitada/rolada-back produz `Lesson` com:

- o que se esperava vs o que aconteceu (evidências citadas);
- hipótese de causa (com grão de incerteza declarado);
- qual invariante ou critério foi tocado;
- **regra mecânica** resultante de um dos 3 tipos nucleares — DEMOTE/BLOCK/CONSTRAINT (D24);
- linking para o commit/branch/variant no **arquivo DGM**.

O arquivo de variantes (DGM-style) preserva **aplicadas E rejeitadas** com suas análises — variantes fracas são stepping stones para propostas melhores, e a F3 consulta o arquivo antes de propor (anti-repetição: "já sabemos que isso falha por causa X").

## Git flow (D11)

- Casa: branch `v2` deste repo, clonada no boot para `/data/repo`.
- `auto/improvement-N` = experimentos; commits `author: eon <eon@autonomo>`.
- Promovida → merge commit `--no-ff` na `v2` + tag `eon/vN` + push (D34; token do env, core only, via outbox — push falho fica pendente, nunca bloqueia).
- Rejeitada → branch preservada no arquivo; revert na v2 só se já promovida.
- Conflito humano × autômato na v2 = PendingDemand + trabalho segue em outras frentes; o core pode rebasear quando o humano resolve.
- GHCR rebuilda a **casca** (imagem); a melhoria viva de binário é local: `go build` no sandbox `basic` (deps vendorizadas) → artifact `/data/artifacts/eon/vN` → supervisor swap.

## Supervisor: promoção de binário

- Pipe local com o core: único canal por onde o core pede promoção/rollback.
- Supervisor valida: tag existe, SHA-256 do artifact verificado contra o evento de promoção (D41), healthcheck pós-swap. Últimos 3 binários bons em `/data/artifacts/eon/vN` (rotação N=3 — sempre há alvo de rollback).
- Crash-loop pós-swap → redeploy automático do último binário bom + evento de incidente.
- O supervisor tem regras próprias mínimas e **não importa** código do core.

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
