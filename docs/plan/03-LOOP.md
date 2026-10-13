# 03 — O Loop Multiciclo e o Scheduler

> Fundamentação: ver 04-SCIENCE.md (LIDA/GWT, sleep-time compute, double-loop learning, NELL, τ-bench, Reflexion/ExpeL, Generative Agents, quality-diversity).

## Por que multiciclo

O loop linear da v1 (`observe→select→prepare→act→verify→commit→repeat`) estava certo, mas raso. A literatura convergiu: cognição é feita de **ciclos em múltiplas escalas de tempo** — o passo é o átomo (LIDA), a consolidação de memória acontece fora do tempo de resposta (sleep-time compute), a revisão das próprias regras é um loop acima do loop de execução (double-loop learning), e um sistema que roda 24/7 por anos precisa de um ciclo de vida cuidando da missão (NELL). Cadeias longas de passos colapsam estatisticamente (95% por passo × 20 passos = 36% total; τ-bench pass^8 < 25%): por isso o EON opera em **microturnos curtos com verificação por passo e estado fora do modelo**.

## Os cinco ciclos

```
CICLO 0 — PASSO            (segundos)
  orientar  : montar contexto (recência × importância × relevância da memória)
  agir      : 1 microturn = 1 chamada de modelo OU 1 capability determinística
  verificar : validador determinístico POR PASSO (supervisão de processo)
  commit    : changeset atômico + event

CICLO 1 — TAREFA           (minutos–horas)
  decompor → passos → integrar → expandir
  falhou?  → extração Reflexion: Lesson citada + hipótese de causa
  ∴ gerar candidatos-filho (proveniência: derived_from)

CICLO 2 — REFLEXÃO         (periódico; acelera quando cota esgota — sleep-time)
  consolidar memória: reflexões em árvore sobre o stream (Generative Agents)
  sintetizar Lessons → regras gerais consultáveis (ExpeL)
  comprimir, deduplicar, revalidar confiança (anti-acúmulo de crenças falsas)

CICLO 3 — EVOLUÇÃO         (janela pós-melhoria; recalibração periódica)
  double-loop: questionar as PRÓPRIAS regras, não só os resultados
  F3: proposta → sandbox → auto/* → observação vs baseline → promote/rollback
  recalibrar capacidade do modelo (pass^k: consistência > sorte)

CICLO 4 — VIDA             (contínuo)
  manter a missão viva; curar a frontier como mapa quality-diversity
  (nichos frente × tema × dificuldade; melhor de cada nicho; poda por qualidade)
  CONTINUITY_BLOCKED é diagnóstico explícito; nunca repouso
```

### Ciclo 0 — Passo (o átomo)

- **Contexto por passo**: o `ContextCompiler` seleciona da memória por `recência × importância × relevância` (Generative Agents). Relevância via FTS5/BM25 no início (sem vector DB); importância por heurística determinística primeiro, score por modelo depois (nível de capacidade ≥ L3).
- **Uma decisão por chamada**: escolha fechada sempre que possível; o modelo vê só o passo atual — nunca a conversa inteira.
- **Verificação por passo**: validador determinístico obrigatório (schema, formato, range, executável); verificação por modelo só como camada auxiliar, nunca única (D-invariante 2).
- **Commit serial**: um changeset por vez, transação SQLite + evento.

### Ciclo 1 — Tarefa

- Toda `Task` declara: proveniência (`derived_from`), ganho esperado, novidade, custo estimado, stop condition.
- Conclusão → **expansão**: o que mudou? que lacunas apareceram? próximos incrementos → candidatos-filho.
- Fracasso → **Lesson** (Reflexion): diagnóstico verbal citado, hipótese de causa, e — obrigatoriamente — uma **regra mecânica** de um dos três tipos nucleares (D24): `DEMOTE` (rebaixa prioridade da classe), `BLOCK` (elimina classe de candidatos), `CONSTRAINT` (restringe parâmetros). Prosa sozinha não incorpora (Feedback Friction). Regras: SOFT na 1ª Lesson, HARD BLOCK após 2ª corroboração independente; TTL + decaimento de peso + override declarado por novidade.

### Ciclo 2 — Reflexão / sleep-time compute (gatilhos: D28)

- **Rotina base**: a cada N tarefas concluídas (N≈10); itens de memória com idade >X dias sem reflexão entram na fila;
- **Acelerador**: cota esgotada/idle → F2 vira full-time determinística (a degradação vira superpoder: D12);
- Reflexões em árvore: insights sobre observações, e insights sobre insights (Generative Agents), com ponteiros para as evidências citadas.
- Síntese de Lessons em regras gerais (ExpeL).
- Higiene: compressão, deduplicação, revalidação de confiança (NELL: crenças falsas se acumulam sem revalidação), esquecimento supervisionado.
- Pré-computação de contextos prováveis (sleep-time compute).
- **Export da essência** (D31): a cada janela com mudanças relevantes, delta incremental versionado no git (missão, lessons ativas, memórias de alta confiança) — o backup vivo da continuidade.

### Ciclo 3 — Evolução

- Pipeline completo da F3 (07-SELF-IMPROVEMENT.md): janela de observação com métricas vs baseline; recalibração do modelo (pass^k); revisão de políticas; propostas de elevação de autonomia.

### Ciclo 4 — Vida

- Manutenção da missão, curadoria da frontier como mapa QD (nichos frente × tema × dificuldade), balanceamento de orçamentos, health checks, auditoria de continuidade.
- Se nenhuma frente produz trabalho legítimo apesar de recursos disponíveis: registrar `CONTINUITY_BLOCKED` com diagnóstico — nunca idle silencioso.

## Máquina de estados de Task (herdada da v1 — era boa)

```
NEW → READY → RUNNING → VERIFYING → SUCCEEDED
               │           │
               ├─ WAITING_TIME      (espera local com not_before)
               ├─ WAITING_EVENT
               ├─ WAITING_OPERATOR   (PendingDemand — nunca bloqueia a missão)
               ├─ THROTTLED          (cota/rate — agendar retomada)
               ├─ BLOCKED            (dependência)
               ├─ EXHAUSTED          (orçamento da própria tarefa)
               └─ REPLANNING / FAILED / CANCELLED
```

## Scheduler: intercalação entre frentes

**Motor (D26 — definido)**: bandit **UCB1 com janela descontada** sobre **3 braços = frentes** (F1/F2/F3; sub-famílias só viram braços por ADR futuro). Recompensa = **progresso verificado por custo**, normalizada por frente em [0,1] contra a própria história recente (score robusto: mediana+MAD clampada) — braços comparáveis em escala comum. A escolha é **determinística** dado as estatísticas persistidas: auditável e reproduzível, sem RNG livre. Sobre o bandit:

1. **Piso de justiça** (anti-starvation): cada frente é escolhida ≥1 vez por K ciclos (K≈20), independente da estatística;
2. **Seleção intra-frente por ganho informacional**: candidatos ranqueados por `expected_epistemic_gain / custo` (active learning);
3. **Bônus de novidade**: candidatos precisam declarar novidade vs já-feito (quality-diversity; anti-busywork);
4. **Aplicação mecânica de Lessons** (D24): regras `DEMOTE` rebaixam candidatos da classe; regras `BLOCK` (apenas após 2ª corroboração independente) os eliminam; um candidato pode atravessar um BLOCK com declaração explícita de novidade ("por que desta vez é diferente") — se falhar de novo, a regra sai reforçada;
5. **Modificadores de contexto**: orçamento disponível, cota restante, prazo de obrigações recorrentes, urgência de pendências operador.

Determinismo: o bandit usa estatísticas do store (sem RNG livre; seed versionada para reprodutibilidade de simulação). O scheduler **nunca** pergunta ao modelo o que executar.

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
