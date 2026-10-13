# 05 — Modelo de Domínio e Persistência

## Princípio

Domínio **novo, mínimo e evolutivo** (D04): 13 conceitos, zero dogma. Conceito novo só entra com ADR demonstrando que uma frente precisa dele. O domínio é puro (só stdlib) e não contém lógica de aplicação — o erro nº3 da v1 foi um "domain" com 212 tipos e casos de uso dentro.

## Vocabulário mínimo

| Conceito | Papel | Regras |
|---|---|---|
| `Mission` | propósito, domínios, políticas, orçamento, status | N lógicas, 1 executando por vez (D13). Não se auto-cria; emenda via operador ou pipeline versionado |
| `Task` | unidade de agenda | Declara obrigatoriamente: `derived_from`, ganho esperado, novidade, custo estimado, stop condition. Estados: ver 03-LOOP.md |
| `Step` | microturn dentro de uma Task | 1 chamada de modelo OU 1 capability. O modelo vê só o passo. Produz/referencia Artifacts |
| `Artifact` | conteúdo imutável | hash + id; tudo que entra/sai do modelo é artifact; citável |
| `Proposal` | saída tipada do modelo | parseada + normalizada + validada; **nunca** estado canônico |
| `ChangeSet` | lote atômico de mudanças | aplica inteiro ou rejeita inteiro; 1 transação SQLite + 1 event |
| `Evidence` | registro de verificação | produtor (validador/teste/schema), passed, artifact_ref |
| `Lesson` | outcome + análise + regra mecânica | obrigatória em toda falha da F3 e em erros representativos; consultada pelo scheduler; regra de um dos 3 tipos (DEMOTE/BLOCK/CONSTRAINT), SOFT→HARD por corroboração (D24) |
| `Memory` | conhecimento curado de longo prazo (F2) | escopo, tags, conexões, confiança revisável, compressível |
| `Improvement` | cidadão da F3 | alvo (prompt/política/código), nível (L0–L2+), estados do pipeline (07) |
| `PendingDemand` | pergunta/pendência com o operador | não-bloqueante, TTL, escopo local, política de re-ask (D14) |
| `Event` | log append-only | toda transição oficial gera event; replay e auditoria |
| `FrontierCandidate` | candidato do mapa quality-diversity (Ciclo 4) | agregado próprio (D33): nasce de propostas, **não executa**; nicho (frente × tema × dificuldade), score do nicho, idade, status |

## Persistência (D03): SQLite híbrido

### Estrutura

- **Tabelas canônicas por agregado** — `mission`, `task`, `step`, `artifact`, `proposal`, `changeset`, `evidence`, `lesson`, `memory`, `improvement`, `pending_demand`, `frontier_candidate`, `kv_meta`.
- **`events`** — append-only, índice por aggregate/type/timestamp. Base de auditoria, replay e análise pós-falha.
- **`outbox`** — efeitos externos pendentes (push git, notificações): gravado na mesma transação do changeset, drenado depois (transactional outbox).
- WAL; `foreign_keys=ON`; migrações versionadas **embutidas no binário** (golang-migrate-style, sem dependência externa).

### Transações e contratos

- 1 ChangeSet = 1 transação. Commit serial (D10) — sem contenção de escrita, sem leases.
- **Contratos estreitos por repositório** (`MissionStore`, `TaskStore`, `MemoryStore`, `ImprovementStore`, `LessonStore`, `EventLog`, `Outbox`...): a anti-god-interface. A v1 tinha uma `Transaction` com 156 métodos — qualquer componente escrevia em qualquer agregado. Aqui, cada front recebe **só** os repositórios que usa (least-authority real).
- Reader/Writer por repositório, definidos em `internal/port`. Implementação única canônica (SQLite); fake in-memory para testes só se um teste específico exigir, sem duplicar o modelo.

### Testes de persistência (herança boa da v1)

1. **Contract suite** por repositório — roda contra SQLite real;
2. **Crash-replay** — failpoints (crash antes/depois do commit) com `kill -9` e reopen; sem perda, sem duplicação;
3. **Idempotência** — reentrega de eventos/resultados não duplica efeitos;
4. **Migrações** — banco gerado por toda versão anterior migra para a atual (teste de cadeia).

### Anti-acúmulo (NELL)

- Confiança revisável em `memory` e `lesson`; revalidação periódica como família da F2 (Ciclo 2).
- Política de compactação (D30): artifacts >90d sem acesso → blob comprimido fora das tabelas quentes; memórias de baixa confiança/importância → candidatas à compressão; nada é apagado sem event (esquecimento supervisionado é uma mudança de estado, não um DELETE silencioso). Executor: família F2-higiene, comprovada por Evidence; teto de tamanho com alerta.

### Falha fatal de storage (D31)

SQLite irrecuperável → **incidente registrado** (log local + evento pós-recuperação) → storage novo criado → re-bootstrap do essencial (missão, config, calibração) → **F2 re-importa a essência**: export periódico versionado como markdown no repo git (missão, lessons ativas, memórias de alta confiança), delta incremental a cada janela do Ciclo 2 com mudanças relevantes. O autômato segue em modo degradado; nunca fica parado esperando humano.

## DDL — rascunho inicial (a detalhar em iteração)

```sql
-- exemplo: task
CREATE TABLE task (
  id TEXT PRIMARY KEY,
  mission_id TEXT NOT NULL REFERENCES mission(id),
  front TEXT NOT NULL,                -- project|thoughts|self
  title TEXT NOT NULL,
  derived_from TEXT,                  -- proveniência
  expected_gain TEXT NOT NULL,
  novelty TEXT NOT NULL,
  cost_estimate TEXT NOT NULL,
  stop_condition TEXT NOT NULL,
  status TEXT NOT NULL,
  not_before INTEGER,                 -- esperas locais
  attempt INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
-- events
CREATE TABLE events (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  aggregate_type TEXT NOT NULL,
  aggregate_id TEXT NOT NULL,
  type TEXT NOT NULL,
  payload_hash TEXT NOT NULL,
  payload BLOB NOT NULL,
  occurred_at INTEGER NOT NULL
);
-- demais agregados: mesmo padrão (id, campos do vocabulário, timestamps, FKs)
```

## Perguntas abertas

- Nenhuma pendente neste documento — índices e paginação definidos em D44; fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
