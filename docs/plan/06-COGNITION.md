# 06 — Cognição Adaptativa

> A espinha da tese: competência vem do harness calibrado à capacidade do modelo, não do tamanho do modelo (TinyGSM, LLM-Modulo, Large Language Monkeys — ver 04-SCIENCE.md).

## O que é "capacidade de um modelo"

Um modelo tem, para nossos fins, quatro eixos mensuráveis:

1. **Aderência a instrução** — segue o formato pedido?
2. **Aderência a formato** — produz saída parseável (JSON, delimitadores, token fechado)?
3. **Coerência multi-passo** — mantém restrições ao longo de um passo composto?
4. **Janela útil** — contexto real que respeita antes de degradar.

## Calibração (D27 — definida)

- **Gatilhos**: boot + troca de binding/perfil + **anomalia** (taxa de parse failure acima do limiar dispara re-probe de formato) + **semanal** com cap de gasto. Resultados **cacheados por (perfil, modelo, versão)** — mesma configuração não recalibra sem motivo.
- **Suíte**: 4 famílias de probes determinísticas e baratas (~12–15 chamadas no total): (1) aderência a formato (JSON e delimitado), (2) seguimento de instrução exata (string literal), (3) coerência multi-passo (mini-cadeia de 2–3 passos), (4) janela útil (recall em contexto preenchido). Probes críticos medidos em **pass^5** (k≈5): consistência, não sorte (τ-bench: pass@1 alto com pass^8 baixo = agente que "tem sorte").
- **Atribuição conservadora por mínimo (fail-safe)**: nível = min(nível de formato, nível de instrução), com ≥4/5 no probe crítico. Coerência e janela útil **nunca sobem** o nível — só ajustam granularidade de decomposição e tamanho de contexto.
- Resultado → **nível de capacidade L0–L5**:

| Nível | Perfil | Estratégia do harness |
|---|---|---|
| L5 | segue JSON complexo, multi-parte | formatos estruturados, menos microturnos |
| L4 | JSON simples confiável | JSON + validação de schema |
| L3 | texto delimitado confiável | campos delimitados, 1 decisão por chamada |
| L2 | escolha fechada confiável | menu de opções (A/B/C), microturnos mínimos |
| L1 | texto curto aderente | sim/não + tokens fechados; reparo determinístico |
| L0 | sem modelo / cota esgotada | só trabalho determinístico (F2/F3-sandbox); sleep-time compute |

- Scores ficam versionados como artifacts; recalibração é uma operação auditável do Ciclo 3.
- **Dialeto ≠ capacidade**: o perfil de dialeto (Groq/NIM/Ollama/OpenRouter — `max_tokens` vs `max_completion_tokens`, reasoning tokens, truncamento, Retry-After) é ortogonal ao nível e herdado do conhecimento empírico da v1 (results/).

## Ladder de formatos (degradar com segurança)

```
JSON nativo (com schema) → JSON reparável → campos delimitados
  → token fechado → sim/não → determinístico/humano(nunca obrigatório)
```

Cada degrau tem parser e validador próprios; falha de parse → reparo determinístico → se insalvável, desce um degrau e **re-pede com formato mais simples** — nunca joga a operação fora, nunca repete a chamada igual (anti-busywork de rede).

## Parser ladder (herança empírica da v1)

As 11 variantes de parsing que a v1 pagou 11 fases para descobrir (quoted keys, header prefix, bracket separator, arrow assignment, multiline folding, XML angle bracket, HTML decoding, bare key, truncation, JSON fallback) viram **uma única ladder de reparo, testada por tabela** — um mecanismo, não onze fire tests.

## Microturnos

- 1 chamada = 1 decisão. Escolha fechada sempre que possível; geração aberta só quando necessário e sempre com verificador.
- Separar interpretar / decidir / produzir / revisar em passos diferentes.
- Reparo determinístico antes de qualquer nova chamada.
- O passo composto (chain) é curto; estado do que já foi decidido vive no store, não no contexto.

## n-Sampling com verificação (orçamento-permitido)

Quando o orçamento permite e a tarefa é verificável de forma barata: n amostras + validação + (votação se houver empate válido). Cobertura escala log-linearmente (Large Language Monkeys). Política: `n` ajustado ao custo/benefício da tarefa; amostras são artifacts independentes.

## ContextCompiler (Ciclo 0)

- Seleção por `recência × importância × relevância` (Generative Agents):
  - **recência**: decay exponencial por último acesso;
  - **importância**: heurística determinística primeiro (tipo de artifact, evidência vinculada, uso recente por frentes); score por modelo em níveis ≥ L3;
  - **relevância**: FTS5/BM25 no início; embeddings como otimização futura (nunca como fonte canônica — lição da v1).
- Prompt versionado e reproduzível por (id, versão): template + fatos compactos + opções válidas + espaço de saída reservado. **Sem histórico conversacional.**
- Injeção: conteúdo web/arquivos é sempre enquadrado como **dado** (fences, sem instruções privilegiadas); validadores confiam em códigos/estruturas, não na boa vontade do modelo.

## Orçamento e degradação

- Caps diários (requests, tokens) por config + cota real do provider; burn visível no dashboard.
- Esgotou → nível efetivo L0: Ciclo 2 acelera (sleep-time compute), F3-sandbox valida offline, F1-síntese pausa. **Loop nunca dorme.**

## Resiliência a erros de provider (D29 — definida)

**Tabela de classes de erro** — cada classe com protocolo próprio:

| Classe | Protocolo |
|---|---|
| `429` / rate limit | espera **exata** do `Retry-After` (herança empírica Groq da v1) |
| `5xx` / timeout | backoff exponencial com jitter e cap |
| `4xx` de payload | **1** reparo determinístico; nunca retry cego |
| falha de parse | ladder de reparo / degradação de formato; nunca re-chamada igual |
| cota diária esgotada | `THROTTLED` + agendamento; Ciclo 2 acelera |

**Circuit breaker**: 5 falhas consecutivas → nível efetivo degrada a **L0 imediatamente** (Ciclo 2 acelera, nada queima cota); **half-open**: probe barata periódica (~15min) restaura o nível quando passa. Falha de provider vira evento, não cascata. Erro transiente nunca vira Lesson espúria.

## Perguntas abertas

- A30: valores exatos dos caps/jitters de backoff por perfil de dialeto (tunar empiricamente no M2 com dados reais)
