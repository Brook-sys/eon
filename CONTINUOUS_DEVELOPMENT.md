# Programa de Desenvolvimento Contínuo

Status: ativo

## Missão operacional

Transformar incrementalmente os documentos de pesquisa em um runtime epistemológico executável, testável e recuperável em Go, sem antecipar decisões ainda não validadas.

Cada heartbeat executa normalmente um lote de 2 a 4 melhorias relacionadas. Um único item pode ocupar o ciclo apenas quando for substancial — por exemplo, implementação acompanhada de testes, investigação comparativa ou correção estrutural ampla. O estado deste arquivo é a coordenação persistente entre ciclos.

## Restrições aceitas

- núcleo em Go;
- APIs OpenAI-compatible por adapter desacoplado;
- contrato mínimo de modelo `text → text`;
- modelo sem autoridade sobre estado ou capabilities;
- kernel determinístico e estado persistente;
- continuidade sem busy loop;
- conhecimento rastreável por fontes, observações, claims e evidências;
- alterações oficiais somente por `ProposedChangeSet` validado;
- Dolt condicionado a spike comparativo;
- automação geral fora do MVP.

## Definição de lote concluído

Um lote é concluído somente quando possui:

1. objetivo comum delimitado;
2. normalmente 2 a 4 melhorias observáveis e relacionadas;
3. integração coerente entre documentos, contratos, código e testes afetados;
4. verificação executada para o conjunto;
5. documentação das decisões ou contratos afetados;
6. um ou mais commits atômicos, salvo se o resultado for apenas investigação inconclusiva registrada.

Exemplos de lotes adequados:

- corrigir duas contradições, adicionar o requisito normativo correspondente e atualizar rastreabilidade;
- definir uma interface Go, implementar o fake e criar contract tests;
- pesquisar duas alternativas, registrar evidências e atualizar um ADR sem ainda aceitá-lo;
- implementar uma transição de estado, testes de tabela, fuzz test e documentação da invariante.

Não contam como várias melhorias mudanças cosméticas repetidas, subdivisões artificiais do mesmo parágrafo ou arquivos sem conteúdo verificável.

## Ordem de desenvolvimento

### Fase 0 — coerência da especificação

- [x] `DONE` Auditar a arquitetura central em busca de contradições e resíduos do antigo MVP de automação/programação.
  - Evidência: `ARCHITECTURE.md` v0.6 remove shell/código do catálogo do MVP, substitui Goal/WorkItem/MemoryStore e alinha agenda a Inquiry/Operation.
- [x] `DONE` Criar glossário normativo para Mission, Inquiry, Operation, ChangeSet, Claim, Evidence, Commit e Artifact.
  - Evidência: `GLOSSARY.md` baseline v0.1, com convenção BCP 14 e termos substituídos.
- [x] `DONE` Auditar `WEAK_MODEL_PROTOCOL.md` e `RESEARCH_PLAN.md` para aplicar o glossário sem apagar terminologia útil da literatura.
  - Evidência: protocolo alinhado a `MissionRevision → Question → InquiryCandidate → Inquiry → Operation`; HTN mantido apenas como termo da literatura.
- [x] `DONE` Consolidar requisitos funcionais e não funcionais com IDs rastreáveis.
  - Evidência: `REQUIREMENTS.md` baseline v0.1 define FR/NFR, critérios de aceitação e matriz inicial de verificação.
- [x] `DONE` Criar taxonomia inicial de falhas específica do runtime epistemológico.
  - Evidência: `FAILURE_TAXONOMY.md` baseline v0.1 define `FailureRecord`, eixos ortogonais, códigos mínimos, certeza de efeito e disposições seguras.
- [x] `DONE` Formalizar invariantes de autoridade, continuidade, segurança e progresso.
  - Evidência: `INVARIANTS.md` baseline v0.1 fixa 21 invariantes, quatro propriedades condicionais de liveness e estratégia de verificação.

### Fase 1 — esqueleto Go determinístico

- [x] `DONE` Inicializar módulo Go sem framework e definir layout mínimo.
  - Evidência: `go.mod`, entrypoint inerte `cmd/runtime` e pacotes internos sem dependências externas.
- [x] `DONE` Implementar tipos de domínio sem dependências externas.
  - Evidência: `internal/domain` cobre missão, pergunta, inquiry, operação, conhecimento, changesets, commit e falha com IDs distintos, schemas versionados e validação estrutural inicial.
- [x] `DONE` Implementar máquina de estados pura e testes de tabela.
  - Evidência: `internal/domain/transition.go` define eventos tipados e transição pura; testes cobrem caminhos legais, terminais, payloads inválidos, pureza e a obrigação de reconciliar efeitos `UNKNOWN`/`PARTIAL` antes de retry.
- [x] `DONE` Implementar `Clock`, `IDGenerator` e `RandomSource` injetáveis.
  - Evidência: portas e implementações de sistema/teste em `internal/runtime/source`, com relógio manual e sequências determinísticas thread-safe.
- [x] `DONE` Implementar store em memória com contract tests.
  - Evidência: portas estreitas de missão/agenda em `internal/port`; transações copy-on-write com rollback, validação, isolamento de slices e erros tipados em `internal/storage/memory`; suite reutilizável em `internal/storage/contract`.
- [x] `DONE` Implementar event log em memória e idempotência básica.
  - Evidência: `domain.Event` recebe sequência monotônica do store; portas e backend em memória oferecem append/listagem/lookup transacionais; `IdempotencyRecord` implementa reserva e conclusão monotônicas com replay idêntico e conflito de intenção/resultado divergente; contract tests cobrem ordem, paginação, rollback e deduplicação.

### Fase 2 — primeiro vertical slice simulado

- [x] `DONE` Carregar e validar `MissionSpec` versionada.
  - Evidência: `internal/mission` faz decode JSON estrito e limitado, valida schema/conteúdo/status ativo, deriva IDs/tempo por fontes injetadas e instala revisão, ponteiro ativo e evento de auditoria atomicamente; testes cobrem entrada válida, campos desconhecidos, trailing data, oversize, status inativo, duplicatas e rollback sem mutação.
- [x] `DONE` Persistir pergunta, operação e condição de retomada.
  - Evidência: catálogo imutável de `OperationSpec`, integridade referencial da agenda e `agenda.Bootstrapper` persistem `Question → InquiryCandidate → Inquiry → Operation`, condições `READY` e evento correlacionado em uma transação; contract tests impedem linhagem órfã e mutação de campos estruturais.
- [x] `DONE` Implementar servidor OpenAI-compatible falso para testes.
  - Evidência: `internal/provider/openai/fakeserver` oferece servidor HTTP roteado, script determinístico, captura de requests e registro de mismatches para testes offline.
- [x] `DONE` Implementar provider mínimo Chat Completions texto→texto.
  - Evidência: porta neutra `ModelProvider` e adapter `internal/provider/openai` fazem POST limitado a `/v1/chat/completions`, extraem texto/usage, classificam HTTP/transporte/resposta inválida e não expõem corpo de erro.
- [x] `DONE` Compilar um `OperationSpec` sob budget.
  - Evidência: `internal/prompt` compila envelope texto→texto versionado, usa o menor limite entre spec/provider, reserva saída e margem, seleciona fatos opcionais por prioridade sem truncamento e falha quando conteúdo obrigatório não cabe; testes de fronteira e fuzz verificam o limite.
- [x] `DONE` Produzir, validar e aplicar `ProposedChangeSet` atomicamente.
  - Evidência: `internal/changeset` preserva texto bruto antes do parse, rejeita JSON não canônico/duplicado/desconhecido, confere linhagem e validadores do kernel e confirma proposta, recibos, commit, estado canônico e evento em uma transação; store contracts cobrem base obsoleta, rollback e replay idempotente.
- [x] `DONE` Simular crash e comprovar retomada sem efeito duplicado.
  - Evidência: checkpoints injetáveis cobrem persistência do output bruto, cada fronteira transacional e confirmação pós-commit; teste reinicia o processor em todas as fronteiras e comprova um único commit, evento e efeito canônico.
- [x] `SUPERSEDED` Comprovar repouso sem busy loop usando relógio virtual.
  - Substituído pela decisão de 2026-07-15: missão `ACTIVE` não possui repouso global; o código e os testes de `Rest` devem ser removidos em favor de continuidade por famílias de trabalho.
  - Evidência histórica: o comportamento anterior foi implementado e testado, mas foi removido após a revisão da semântica de continuidade; esperas temporais continuam locais às operações.

### Fase 3 — operações epistemológicas mínimas

- [x] `DONE` Ingerir uma fonte fixture imutável.
  - Evidência: `internal/ingest` limita bytes, calcula SHA-256, persiste `Source → SourceVersion → SourceSnapshot` e evento atomicamente; store contracts verificam endereçamento por conteúdo, isolamento de bytes e rollback.
- [x] `DONE` Segmentar com round-trip verificável.
  - Evidência: `internal/segment` divide texto UTF-8 deterministicamente sem alterar bytes; `SourceFragment` registra offsets/hash, store exige cobertura contígua completa e contract tests comprovam ordem, rollback e round-trip.
- [x] `DONE` Propor observação ancorada.
  - Evidência: `internal/observe` persiste proposta e evento atomicamente; store resolve o fragmento e exige citação exata byte a byte antes de aceitar a observação, com contract tests de âncora ausente, quote divergente e rollback.
- [x] `DONE` Propor claim e vínculo de evidência.
  - Evidência: `internal/claim` persiste claim atômico com qualificadores explícitos, um ou mais `EvidenceLink`s tipados, endpoints resolvidos e evento na mesma transação; contract tests cobrem isolamento e rollback de vínculos órfãos.
- [x] `DONE` Gerar uma visão citada a partir do estado canônico.
  - Evidência: `internal/view` materializa Markdown determinístico com claim, relação tipada, observação, quote exata e localização da fonte; artefato registra commit-base, hash e dependências completas.
- [x] `DONE` Atualizar a visão por patch após delta de evidência.
  - Evidência: patch tipado acrescenta `EvidenceLink` a claim existente, marca a visão anterior obsoleta e persiste sucessora regenerada atomicamente; testes cobrem rollback e isolamento.

### Fase 4 — persistência real e spike de Dolt

- [x] `DONE` Fixar suite de contract tests e protocolo comparável de armazenamento.
  - Evidência: `contract.TestStore` permanece a suite funcional comum; `contract.TestDurableStore` formaliza reopen, rollback e idempotência entre processos; `STORAGE_SPIKE.md` fixa dataset, crash harness, métricas e critérios de decisão.
- [x] `DONE` Implementar cenário comparável em SQLite + event log.
  - Evidência: adapter `internal/storage/sqlite` usa SQLite WAL/synchronous FULL para checkpoint atômico do modelo de referência completo; passa `contract.TestStore` e `contract.TestDurableStore`, incluindo rollback, reopen e idempotência persistente.
- [x] `DONE` Implementar cenário comparável em Dolt.
  - Evidência: adapter `internal/storage/dolt` preserva o mesmo checkpoint binário do cenário SQLite em repositório Dolt real, cria commit Dolt por atualização e passa `contract.TestStore` e `contract.TestDurableStore` com close/reopen; `DOLT_BIN` fixa explicitamente a versão exercitada.
- [x] `DONE` Implementar gerador, runner medido e crash harness subprocessado para ambos os backends.
  - Evidência parcial: `internal/storage/spike` gera dataset/manifesto determinísticos, executa fases backend-neutral com métricas, classifica intenção durável e comprova reopen por subprocesso em SQLite; adapters expõem hooks de fronteira sem contaminar `port.Store`.
  - Evidência parcial adicional: runner aceita batching configurável e registra p50/p95/p99 por batch/consulta; medidor de footprint lógico percorre somente arquivos regulares sem seguir symlinks.
  - Evidência parcial adicional: métricas v2 registram footprint antes/depois/delta e o writer atômico gera `manifest.json`, `metrics.json` e resumo `report.md`, rejeitando mistura de datasets.
  - Evidência parcial adicional: `cmd/storage-spike-worker` publica intenção sincronizada, encerra abruptamente nos hooks e é exercitado por subprocesso em SQLite e Dolt CLI com reopen fresco.
  - Evidência parcial adicional: campanhas configuráveis exigem ao menos 30 trials isolados, agregam os três outcomes e reprovam worker sem crash ou qualquer estado parcial.
  - Evidência parcial adicional: inspector composto classifica evento, commit, recibo, head, idempotência e entidade canônica como conjunto indivisível e detecta vínculos cruzados; runner aceita classificadores compostos.
  - Evidência parcial adicional: worker aceita fixture oficial versionada e executa toda a linhagem + changeset + seis registros observáveis em uma única transação; crashes SQLite e Dolt CLI antes/depois do commit são classificados por reopen como `NOT_APPLIED`/`APPLIED`, nunca parcial.
  - Evidência parcial adicional: campanhas repetidas aceitam inspector composto, preservando a invariante oficial em 30+ trials em vez de reduzi-la a evento sentinela.
  - Evidência parcial adicional: adapter medido mantém um `dolt sql-server` persistente, usa driver MySQL, passa as suites funcional/durável e expõe separadamente as fronteiras `SQL COMMIT` e `DOLT_COMMIT`.
  - Evidência parcial adicional: worker `dolt-server` mata abruptamente o processo servidor e o writer nas três fronteiras; reopen classifica antes de SQL como `NOT_APPLIED`, após `DOLT_COMMIT` como `APPLIED` e detecta a janela SQL-only como `INVALID_PARTIAL` pelo working set divergente. Campanhas oficiais de 30 trials por fronteira estão codificadas sob `STORAGE_SPIKE_FULL=1`.
  - Evidência parcial adicional: as 90 execuções oficiais foram concluídas e persistidas em `results/dolt-server/2026-07-15/crash`: 30/30 `NOT_APPLIED` antes de `SQL COMMIT`, 30/30 `INVALID_PARTIAL` após `SQL COMMIT` e 30/30 `APPLIED` após `DOLT_COMMIT`. O writer preserva cada trial e os agregados em JSON auditável.
  - Evidência final: `cmd/storage-spike-runner` executa o dataset completo comum e registra ambiente reproduzível; artefatos em `results/{sqlite,dolt-server}/2026-07-15/workload` usam o mesmo SHA-256 e mostram footprint Dolt 3,90× maior, latências direcionais e alta variância entre rodadas.
- [x] `DONE` Medir footprint, latência, recuperação, diff e complexidade.
  - Evidência: workload comum e 90 crashes Dolt persistidos; footprint Dolt 3,90× maior; latências de rodada única tratadas como direcionais; complexidade operacional e LOC sintetizadas; diff/branch/merge não foram pontuados após o bloqueador absoluto porque não são observáveis pela porta comum nem podem reverter `INVALID_PARTIAL`.
- [x] `DONE` Registrar ADR final do backend.
  - Evidência: ADR-0003 aceita SQLite + event log para o MVP, rejeita Dolt `sql-server` 2.2.0 na configuração medida e fixa critérios explícitos para reconsideração.

### Fase 5 — fontes reais e avaliação cognitiva

- [x] `DONE` Adapter de busca web com fixtures e replay.
  - Evidência: porta neutra `WebSearcher`, adapter SearXNG JSON com resposta limitada/erros não vazáveis e adapter de replay determinístico com captura de requests; testes rodam integralmente offline.
- [x] `DONE` Aquisição segura com limites de bytes e tipos.
  - Evidência: porta `WebFetcher` e adapter HTTP(S) impõem esquema, redirects limitados e revalidados, bloqueio de destinos IP especiais por padrão, allowlist MIME e limite prévio/streaming de bytes; bytes adquiridos entram na linhagem imutável via `IngestFetched`.
- [x] `DONE` Matriz de compatibilidade OpenAI-compatible.
  - Evidência: `OPENAI_COMPATIBILITY.md` delimita o subconjunto portátil, perfis e contract test por implantação; adapter seleciona explicitamente `max_tokens` ou `max_completion_tokens`, com fake/testes cobrindo ambos sem fallback duplicador.
- [x] `DONE` Benchmark 2k/4k/8k para operações selecionadas.
  - Evidência: `internal/evaluation` carrega fixtures estritas, compila a matriz contexto × formato pelo mesmo budget do runtime, mede validade/acerto/tokens/latência e gera artefatos atômicos; `cmd/model-benchmark-runner` executa providers OpenAI-compatible reais sem registrar credenciais.
- [ ] `READY` Avaliar extração, síntese, conflito e reparo por modelo/formato.
  - Preparado: corpus inicial `cognitive-v1` cobre as quatro operações em escolha, campos delimitados e JSON; relatórios agora agregam acerto, erros e omissões por operação, formato e contexto. Falta executar contra pelo menos um modelo pequeno/local e um baseline superior e interpretar os resultados.
  - Ambiente atual: nenhum node de inferência/Ollama está disponível e não há servidor local em `127.0.0.1:11434`; manter o item `READY` até existir endpoint OpenAI-compatible sem credenciais pagas.

### Fase 6 — control plane e dashboard operacional

- [x] `DONE` Definir a arquitetura de autonomia supervisionável e do dashboard.
  - Evidência: `CONTROL_PLANE.md` separa UI, Control API, command/event inbox, projeções e kernel; fixa superfícies, segurança, consistência, slices e critérios de aceitação.
- [x] `DONE` Formalizar schemas de `OperatorCommand`, `CommandReceipt` e `ExternalEvent` e portas do control plane.
  - Evidência: `internal/domain/control.go` define comandos allowlisted com optimistic concurrency, recibos que distinguem aceitação de efeito confirmado e eventos externos tipados/limitados/correlacionados; `internal/port/control.go` separa inboxes de transporte da autoridade de avanço do kernel; testes rejeitam autoridade textual, payload ambíguo/oversize e recibos semanticamente inválidos.
- [x] `DONE` Formalizar e persistir `OperatorQuestion`, `UserAnswer`, `QuestionGate`, estados, expiração e escopo bloqueado local.
  - Evidência: `internal/domain/operator_question*.go` define perguntas/respostas tipadas, correlação por identidade e revisão e transições puras; `internal/kernel/question_gate.go` implementa `ADMIT/SUPPRESS/DEFER` com prioridade, alternativas, default reversível, duplicação, limite, taxa, cooldown e quiet hours; ports/store/checkpoint persistem perguntas e respostas com optimistic concurrency e deduplicação de transporte; contract tests cobrem rollback; `question_blocking.go` bloqueia e retoma somente operações explicitamente declaradas.
- [x] `DONE` Implementar read models e API de inspeção correlacionada sobre estado/event log.
  - Evidência: pacote `internal/inspect` materializa overview/health, agenda, timeline paginada e inspetores de operação/commit/comando sem mutar o store; `inspect.API` expõe endpoints REST somente-leitura; testes cobrem projeção, paginação, correlação de commit e superfície HTTP.
- [x] `DONE` Implementar command inbox idempotente com pause/resume/cancel/shutdown e gate do scheduler.
  - Evidência: domínio puro em `control_state.go` (`ControlState`, `ApplyOperatorCommand`, receipts monotônicos); portas `ControlReader`/`ControlWriter`; store em memória + checkpoint SQLite/gob; `control.CommandInbox` com dedupe por ID/idempotency; `kernel.CommandProcessor` aplica em uma transação e emite eventos; scheduler consulta `AllowsDispatch` (pause bloqueia novo despacho, waits locais ainda retomam); contract/unit tests cobrem replay, conflito, rejeição por revisão obsoleta e gate.
  - Residual histórico: submit HTTP e crash-replay SQLite fechados no ciclo de 2026-07-16 02:00; inspeção HTTP somente-leitura permanece em `internal/inspect`.
- [x] `DONE` Completar external event inbox genérico (mensagem/despertar) com persistência e processador kernel.
  - Evidência: `ExternalEventDisposition` e avanço monotônico no domínio; portas `CreateExternalEvent`/`PendingExternalEvents` no store; checkpoint gob; `control.ExternalEventInbox` durable com dedupe por ID/chave; `kernel.ExternalEventProcessor` aplica respostas (`AcceptUserAnswer` + `ResumeQuestionWait`) e wakes tipados (`user.message`/`source.available`/`authorized.source`) sem interpretar texto como autoridade; contract/unit tests cobrem replay, conflito, ignore e reject.
  - Residual histórico: submit HTTP e crash-replay SQLite fechados no ciclo de 2026-07-16 02:00.
- [x] `DONE` Expor superfícies HTTP de submit/consulta de comandos e eventos externos e comprovar crash-replay dos processadores.
  - Evidência: `control.API` implementa `POST/GET /commands` e `POST/GET /external-events` com JSON estrito, limite de corpo, HTTP 202 de aceitação de inbox e lookup de recibo/disposição; factories injetáveis de receipt/disposition; reopen SQLite em `kernel` prova apply único após crash e pure replay terminal sem eventos extras.
- [ ] `READY` Implementar perguntas/respostas no dashboard com formulário vinculado por `question_id`.
- [ ] `READY` Implementar adapter Telegram com bot próprio, outbox, inline keyboard, reply correlacionado, allowlist e deduplicação.
- [ ] `READY` Persistir decisões do `QuestionGate` e integrá-las à criação/outbox; completar deduplicação semântica assistida, digest, budget versionado e política de lembretes.
- [ ] `READY` Implementar configuração versionada com validação, diff e fronteira segura de aplicação.
- [ ] `READY` Implementar dashboard web mínimo: overview, timeline SSE, inspetor, interação e configuração.
- [ ] `READY` Exportar telemetria OpenTelemetry opcional sem torná-la fonte canônica ou autoridade.

### Fase 7 — atividade contínua sem repouso

- [x] `DONE` Substituir conceitualmente repouso por trabalho contínuo orientado à missão.
  - Evidência: `CONTINUOUS_WORK.md` define compromisso de liveness, portfólio extensível de famílias, rotação, antiatividade artificial e `CONTINUITY_BLOCKED`.
- [x] `DONE` Remover `Rest`, `DecisionRest`, portas de persistência e testes associados.
  - Evidência: domínio e store não contêm estado global de repouso; scheduler tenta `ContinuityStrategy`s e retorna despacho ou diagnóstico `CONTINUITY_BLOCKED`; testes cobrem rotação e despacho por outra família.
- [x] `DONE` Implementar registro de estratégias de continuidade e decisão `EXPAND/DIAGNOSE`.
  - Evidência: `kernel.StrategyRegistry` ordena famílias por prioridade/nome; `PlanContinuityAction` escolhe `EXPAND` enquanto restam estratégias e `DIAGNOSE` ao esgotá-las; scheduler consome o registry e anexa `StrategiesTried`/horizonte na decisão.
- [x] `DONE` Modelar `WorkOpportunity`, derivação pai-filho e horizonte executável com `low_watermark`, alvo e limites versionados.
  - Evidência: `domain.WorkOpportunity`, `HorizonPolicy`/`ExecutableHorizon`, `DeriveChild`/`CanSpawnChild`; portas `ContinuityReader`/`ContinuityWriter`; store memória + checkpoint gob; contract test memory/SQLite.
- [ ] `READY` Implementar replenishment preventivo e decomposição recursiva limitada por profundidade, fan-out, budget e novidade.
  - Preparado: política e validação de fan-out/depth/dedup existem; falta materializar admission `WorkOpportunity → Inquiry/Operation` e reabastecimento preventivo com work ainda ready.
- [x] `DONE` Persistir eventos e diagnóstico detalhado de `CONTINUITY_BLOCKED`, incluindo capabilities indisponíveis e condições de recuperação.
  - Evidência: `ContinuityDiagnosis` com strategies tried, recovery conditions e counts; append de `continuity.blocked`; `LatestContinuityDiagnosis` no store; scheduler grava diagnóstico antes de retornar `DecisionContinuityBlocked`.
- [ ] `READY` Implementar famílias iniciais de gap scan, conflict/evidence review, artifact refresh, integrity audit e harness evaluation.
- [ ] `READY` Testar longevidade sem repouso, diversidade entre famílias, budgets e ausência de atividade sem delta.

## Política de seleção

Entre itens `READY`, escolher nesta ordem:

1. inconsistência que possa contaminar implementação;
2. contrato necessário ao próximo vertical slice;
3. teste que exponha uma hipótese importante;
4. implementação mínima coberta por testes;
5. pesquisa necessária para decisão bloqueada;
6. melhoria editorial com ganho real de precisão.

## Registro de ciclos

Adicionar entradas curtas somente quando houver mudança ou bloqueio relevante:

```text
YYYY-MM-DD HH:MM — ITEM — RESULTADO — VERIFICAÇÃO — COMMIT/NEXT
```

Não transformar este arquivo em log detalhado; Git contém o histórico completo.

2026-07-15 08:26 — Fase 0/coerência — glossário normativo criado; arquitetura alinhada ao domínio epistemológico; interfaces de persistência separadas — verificação: RFC 2119/8174, grep de resíduos e `git diff --check` — próximo: requisitos rastreáveis e auditoria dos protocolos auxiliares.
2026-07-15 08:34 — Fase 0/contratos — protocolo de modelo fraco e plano de pesquisa alinhados ao glossário; requisitos FR/NFR rastreáveis consolidados — verificação: grep de termos substituídos e `git diff --check` — próximo: taxonomia de falhas e invariantes formais.
2026-07-15 08:54 — Fase 0/falhas e invariantes — `FailureRecord`, política de retry/reconciliação, invariantes safety/liveness e requisito FR-DUR-006 formalizados — verificação: fontes primárias Temporal/OpenTelemetry/RFC 9457, inspeção de rastreabilidade e `git diff --check` — próximo: módulo Go, tipos de domínio e fontes determinísticas.
2026-07-15 09:20 — Fase 1/bootstrap Go — módulo/entrypoint criados; tipos persistíveis e validadores iniciais implementados; fontes de tempo, ID e aleatoriedade injetadas com fakes determinísticos — verificação: Go 1.26.5 oficial temporário, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque a imagem não possui toolchain C — próximo: máquina de estados pura e ports/store em memória.
2026-07-15 09:40 — Fase 1/máquina de estados — transição operacional pura implementada; referências de lease/bloqueio tornadas obrigatórias; retry ambíguo bloqueado em favor de reconciliação — verificação: testes de tabela/pureza, `go test ./...`, `go vet ./...`, `git diff --check`; race segue indisponível sem C toolchain — próximo: definir ports e contract tests do store em memória.

2026-07-15 10:00 — Fase 1/store transacional — portas de missão/agenda definidas; store em memória copy-on-write implementado; contract tests reutilizáveis cobrem imutabilidade, round-trip, rollback, validação e cancelamento — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem C toolchain — próximo: event log append-only e idempotência básica.
2026-07-15 10:20 — Fase 1/eventos e idempotência — event log transacional com sequência monotônica e lookup/paginação implementado; reserva/conclusão idempotente rejeita reutilização divergente; contract tests cobrem replay, conflito e rollback — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem C toolchain — próximo: iniciar vertical slice carregando `MissionSpec` versionada.
2026-07-15 10:40 — Fase 2/MissionSpec — loader estrito e limitado implementado; revisão, ativação e evento auditável instalados em transação única com tempo/IDs injetáveis; dependências da fase 2 reclassificadas — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: persistir o primeiro conjunto pergunta/operação/retomada e definir catálogo de `OperationSpec`.
2026-07-15 11:00 — Fase 2/agenda recuperável — catálogo versionado de `OperationSpec` persistido; referências de missão/spec/linhagem validadas no store; bootstrap atômico cria pergunta, candidata, inquiry, operação, wake conditions e evento auditável — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: servidor OpenAI-compatible falso e provider texto→texto.

2026-07-15 11:20 — Fase 2/provider simulado — servidor OpenAI-compatible falso roteado e provider Chat Completions texto→texto implementados; limites de resposta, erros tipados, retryability e não vazamento de corpo cobertos — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: compilar `OperationSpec` sob budget e preservar resposta bruta antes da validação.
2026-07-15 11:40 — Fase 2/compilação sob budget — contrato de `OperationSpec` passou a versionar template e reservar saída/margem; compilador determinístico seleciona contexto opcional por prioridade e rejeita conteúdo obrigatório excessivo — verificação: testes de fronteira/fuzz, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: preservar resposta bruta e implementar changeset validado/atômico.
2026-07-15 12:00 — Fase 2/changeset atômico — resposta texto preservada antes da validação; decoder estrito rejeita campos desconhecidos/não canônicos/duplicados; cadeia `Proposed → Accepted → Commit → evento/recibo/estado canônico` implementada com base versionada e replay idempotente — verificação: contract tests de rollback/base obsoleta, testes adversariais, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: injetar crashes nas fronteiras do processamento e comprovar retomada sem duplicação.
2026-07-15 12:20 — Fase 2/crash-replay — failpoints determinísticos adicionados nas sete fronteiras de durabilidade do processamento; retomada com processor novo converge para um único commit/evento/entidade inclusive quando o crash ocorre após commit durável — verificação: teste de crash em tabela, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem toolchain C — próximo: scheduler mínimo em repouso dirigido por relógio virtual.
2026-07-15 12:40 — Fase 2/repouso determinístico — scheduler mínimo seleciona por ordem estável, retoma `not_before` vencido, limita replenishment e persiste/encerra `Rest`; relógio manual bloqueia por sinal até deadline virtual sem polling — verificação: contract tests, testes de zero ciclo intermediário/despertar único, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem toolchain C — próximo: iniciar Fase 3 com ingestão de fonte fixture imutável.
2026-07-15 13:00 — Fase 3/ingestão fixture — bytes limitados são preservados em snapshot imutável endereçado por SHA-256; fonte, versão, snapshot e evento são gravados atomicamente com validação de linhagem/hash — verificação: unit/contract tests de isolamento, oversize, hash divergente e rollback, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: segmentação determinística com cobertura, offsets e round-trip.
2026-07-15 13:20 — Fase 3/segmentação — texto UTF-8 segmentado deterministicamente por offsets de bytes; store exige cobertura total ordenada, localização/hash coerentes e grava evento atomicamente — verificação: contract tests de rollback/lacunas e teste Unicode de round-trip, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: propor observações estritamente ancoradas em fragmentos recuperáveis.
2026-07-15 13:40 — Fase 3/observações ancoradas — propostas preservam declaração, citação exata e proveniência separadas; store resolve âncora e rejeita quote inventada sem evento parcial — verificação: contract/unit tests, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível porque requer cgo/toolchain C — próximo: claims versionados e vínculos de evidência tipados.
2026-07-15 14:00 — Fase 3/claims e evidência — claims exigem qualificadores explícitos e são persistidos atomicamente com vínculos tipados para observações existentes e evento auditável — verificação: unit/contract tests de isolamento, endpoint órfão e rollback, `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem toolchain C — próximo: materializar uma visão citada do estado canônico.

2026-07-15 14:20 — Fase 3/visões citadas — visão materializada determinística criada sobre claim/evidência/fonte; patch de evidência gera sucessora e torna a anterior detectavelmente obsoleta em transação única — verificação: contract/unit tests de isolamento e rollback, `go test ./...`, `go vet ./...`, `git diff --check`; stdlib escolhida após preflight, sem dependência de JSON Patch — próximo: fixar suite comparável de storage e iniciar spikes Dolt/SQLite.
2026-07-15 14:40 — Fase 4/protocolo do spike — suite durável separada formaliza reopen/rollback/idempotência; plano fixa dataset determinístico, crash subprocessado, métricas, bloqueadores e pesos comuns para SQLite/Dolt — verificação: fontes oficiais Dolt/SQLite/modernc, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: adapter SQLite + event log sob ambas as suites.
2026-07-15 15:00 — Fase 4/SQLite durável — checkpoint completo e isolado do modelo de referência persistido atomicamente em SQLite WAL/FULL; adapter passa suites funcional e durável com reopen, rollback e idempotência — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; race indisponível sem toolchain C — próximo: adapter Dolt sob as mesmas suites e depois crash harness subprocessado.
2026-07-15 15:20 — Fase 4/Dolt contratual — adapter por processo externo persiste o mesmo checkpoint integral, cria commit Dolt e reabre repositório real; modo medido separado como `sql-server` para evitar viés de startup — verificação: Dolt 2.2.0 oficial com SHA-256 validado, suites funcional/durável, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: gerador/runner comum e crash subprocessado.
2026-07-15 15:40 — Fase 4/harness baseline — dataset/manifesto determinísticos, runner comum, classificação de intenção e reopen por subprocesso implementados; hooks de durabilidade adicionados sem ampliar a porta de domínio — verificação: subprocess test SQLite, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: batching/percentis/footprint e worker de crash real, depois Dolt `sql-server`.
2026-07-15 16:00 — Fase 4/métricas do runner — transações em lotes configuráveis, p50/p95/p99 por batch/consulta e medição confinada de footprint implementados — verificação: testes de batching/nearest-rank/symlink, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: worker CLI de crash abrupto e integração do footprint aos artefatos.
2026-07-15 16:20 — Fase 4/artefatos medidos — runner passou a registrar footprint inicial/final/delta; writer atômico emite manifesto, métricas e relatório Markdown com vínculo obrigatório ao SHA-256 do dataset — verificação: testes de footprint/artefatos, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: worker CLI de crash abrupto nos hooks SQLite/Dolt.
2026-07-15 16:40 — Fase 4/crash worker — CLI subprocessada grava marcador de intenção sincronizado e morre nos hooks reais; reopen classifica `NOT_APPLIED` antes e `APPLIED` depois do commit em SQLite e Dolt CLI — verificação: testes de integração com ambos os binários, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: classificação composta/repetição e lifecycle Dolt `sql-server`.
2026-07-15 17:00 — Fase 4/campanhas de crash — runner repete no mínimo 30 trials independentes, preserva resultados individuais, agrega outcomes e exige morte real do worker — verificação: testes de repetição/agregação/saída normal, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: intenção composta e lifecycle Dolt `sql-server`.
2026-07-15 17:20 — Fase 4/classificação oficial composta — atomicidade de crash passou a exigir evento, commit, recibo, head, idempotência concluída e entidade canônica completos e coerentes; runner aceita inspector composto — verificação: testes de ausente/parcial/completo/vínculo cruzado, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: worker aplicar essa mutação composta e lifecycle Dolt `sql-server`.
2026-07-15 17:40 — Fase 4/mutação oficial no worker — fixture versionada instala pré-requisitos e aplica changeset oficial em uma transação; worker seleciona fixture sem duplicar lógica e crash SQLite comprova visibilidade composta tudo-ou-nada — verificação: testes unitários e subprocessados do pacote spike — próximo: repetir fixture oficial em Dolt e implementar lifecycle medido `sql-server`.
2026-07-15 18:00 — Fase 4/crash oficial Dolt — fixture composta exercitada nas duas fronteiras Dolt CLI com reopen fresco; campanhas ganharam plano de inspector composto para preservar os seis registros oficiais em 30+ trials — verificação: `go test ./internal/storage/spike` com Dolt 2.2.0 explícito e `git diff --check` — próximo: adapter/lifecycle medido `dolt sql-server` e campanhas completas.
2026-07-15 18:20 — Fase 4/Dolt medido — adapter `sql-server` persistente implementado com readiness/shutdown controlados, driver MySQL e fronteiras distintas para commit do working set e commit Dolt; suites funcional/durável comprovam reopen real — verificação: `go test ./...` com Dolt 2.2.0 explícito, `go vet ./...`, `git diff --check` — próximo: crash worker matar o servidor nas três fronteiras e executar campanhas oficiais completas.
2026-07-15 18:40 — Fase 4/crash do servidor Dolt — worker agora mata writer + `sql-server` nas três fronteiras; reopen verifica também limpeza do working set e expõe como `INVALID_PARTIAL` a janela após SQL commit/antes do commit Dolt; campanhas oficiais completas codificadas com opt-in — verificação: testes subprocessados reais em Dolt 2.2.0, packages `storage/spike` e `storage/dolt` — próximo: executar 90 trials completos e persistir métricas/artefatos.
2026-07-15 19:00 — Fase 4/campanha completa Dolt — 90 crashes reais executados; resultados por trial persistidos: 30/30 não aplicados antes do SQL commit, 30/30 parciais inválidos na janela SQL-only e 30/30 aplicados após commit Dolt — verificação: `TestDoltServerOfficialCrashCampaigns` com Dolt 2.2.0, writer/round-trip JSON e `go vet` — próximo: workload medido comum e relatório comparativo; a janela SQL-only é bloqueador arquitetural a resolver ou aceitar contra Dolt.
2026-07-15 19:20 — Fase 4/workload completo — runner CLI reproduzível criado e dataset comum executado em SQLite 3.50.4 e Dolt 2.2.0; footprint Dolt foi 3,90× maior e latências de execução única mostraram variância, enquanto o bloqueador de crash permanece decisivo — verificação: `go test ./...`, `go vet ./...`, `git diff --check`, artefatos JSON/Markdown com SHA-256 idêntico — próximo: síntese curta de diff/complexidade e ADR, ou tentativa explicitamente delimitada de reconciliação Dolt.
2026-07-15 19:40 — Fase 4/decisão de storage — spike encerrado; complexidade e limites de diff sintetizados; ADR-0003 aceita SQLite + event log e rejeita Dolt 2.2.0 na configuração medida por 30/30 estados parciais na janela SQL-only — verificação: rastreabilidade documental, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: iniciar fontes reais com aquisição segura e replay.
2026-07-15 20:00 — Fase 5/busca e aquisição web — portas neutras, adapter SearXNG, replay determinístico e fetch HTTP(S) hostil por padrão implementados; snapshots web reutilizam ingestão content-addressed — verificação: testes offline de limites/tipos/redirects/replay, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: matriz OpenAI-compatible e benchmark de contexto.
2026-07-15 20:20 — Fase 5/compatibilidade de modelos — matriz portátil e protocolo de qualificação por deploy definidos; adapter passou a versionar o dialeto de limite de saída (`max_tokens`/`max_completion_tokens`) sem fallback arriscado — verificação: fontes primárias OpenAI/Ollama/vLLM/llama.cpp, testes dos dois dialetos, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: harness benchmark 2k/4k/8k e fixtures cognitivas.

2026-07-15 20:43 — Fase 5/harness cognitivo — benchmark reproduzível 2k/4k/8k implementado com fixtures de extração, síntese, conflito e reparo; formatos estritos, métricas e artefatos atômicos integrados a provider OpenAI-compatible — verificação: testes de corpus/parser/matriz/artefatos e CLI, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: executar modelos pequeno/local e baseline superior, comparar formato/operação e registrar avaliação.
2026-07-15 23:40 — Fase 5/análise comparável — relatório passou a agregar acerto, falhas e fatos omitidos por operação, formato e contexto; execução real permanece pronta mas sem endpoint local disponível — verificação: `go test ./...`, `go vet ./...`, `git diff --check`, descoberta de nodes e probe Ollama — próximo: executar dois baselines quando houver provider OpenAI-compatible local.
2026-07-16 00:00 — Fase 7/continuidade sem repouso — semântica global de `Rest` removida de domínio, portas e stores; scheduler percorre famílias de continuidade e despacha trabalho admitido ou retorna `CONTINUITY_BLOCKED`; arquitetura, requisitos e invariantes alinhados a horizonte renovável e esperas somente locais — verificação: `go test ./...`, `go vet ./...`, `git diff --check`, grep de resíduos — próximo: registro versionado de estratégias e modelo persistido de `WorkOpportunity`/horizonte.
2026-07-16 00:20 — Fase 6/contratos do control plane — comandos, recibos e eventos externos formalizados com allowlists, revisão esperada, limites, correlação e portas sem escrita canônica direta — verificação: testes de domínio, `go test ./...`, `go vet ./...`, `git diff --check` — próximo: read models e API de inspeção correlacionada.
2026-07-16 00:40 — Fase 6/perguntas ao operador — contratos tipados, transições otimistas, gate antispam básico, persistência transacional de respostas e waits estritamente locais implementados — verificação: `go test ./...`, `go vet ./...`, `git diff --check`; contract tests passam em memory/SQLite/Dolt — commits `0e3698d`, `7c91a99`; próximo: external event inbox e outbox persistida.
2026-07-16 01:05 — Fase 6/Slice B control plane — estado de processo/missão, inbox/recibos de comando, processador kernel e gate do scheduler para pause/resume/cancel/shutdown — verificação: `go test ./...`, `go vet` nos pacotes afetados, contract test memory/SQLite, `git diff --check`; race indisponível sem cgo — próximo: crash-replay do processador e read models/API de inspeção.
2026-07-16 01:25 — Fase 6/Slice A inspeção — read models e Control API somente-leitura: overview/health, agenda, timeline paginada e inspetores de operação/commit/comando sobre store + event log; HTTP REST sem mutação — verificação: `go test ./...`, `go vet ./internal/inspect`, `git diff --check` — próximo: external event inbox genérico, HTTP submit de comandos e crash-replay do processador.
2026-07-16 01:50 — Fase 6/external event durable — inbox store-backed, disposition monotônica, processor de `USER_ANSWER` e wakes tipados sem autoridade textual — verificação: `go test ./...`, `go vet ./...`, `git diff --check` — próximo: HTTP submit de comandos/eventos e crash-replay dos processadores; ou continuidade (`WorkOpportunity`/estratégias).
2026-07-16 02:00 — Fase 6/Slice B residual — Control API mutável com submit/consulta de comandos e eventos externos; retries reutilizam identidade por chave; crash-replay SQLite dos processadores de comando e evento comprova apply único e pure replay terminal — verificação: `go test ./internal/control ./internal/kernel ./internal/inspect`, `go vet` nos pacotes afetados, `git diff --check` — próximo: dashboard de perguntas/respostas, persistência da decisão do `QuestionGate`/outbox, ou continuidade (`WorkOpportunity`/estratégias).
2026-07-16 02:25 — Fase 7/horizonte e registry — `WorkOpportunity`/`ExecutableHorizon`/`HorizonPolicy`/`ContinuityDiagnosis` modelados; store+portas+checkpoint gob; `StrategyRegistry` e `EXPAND/DIAGNOSE`; scheduler observa horizonte, tenta famílias e persiste `continuity.blocked` — verificação: `go test ./...`, `go vet` nos pacotes afetados, contract memory/SQLite, `git diff --check` — próximo: replenishment preventivo com admission real e famílias iniciais de gap/conflict/integrity.
