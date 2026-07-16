# Invariantes do Runtime Epistemológico

Status: baseline v0.1

Este documento transforma princípios arquiteturais em propriedades verificáveis. Invariantes são condições que MUST permanecer verdadeiras em toda transição oficial; critérios de liveness descrevem progresso que MUST ocorrer sob premissas explícitas.

## 1. Autoridade

### INV-AUTH-001 — Modelo não produz efeito oficial

Nenhum texto, `ModelDecision` ou artifact bruto de modelo altera `KnowledgeState`, agenda, budget, política ou capability. Somente comando tipado produzido após validação e autorizado pelo kernel MAY originar efeito.

**Verificação:** testes com saídas válidas, desconhecidas e adversariais demonstram ausência de mutação antes de `AcceptedChangeSet` e commit.

### INV-AUTH-002 — Mudança canônica possui cadeia completa

Para todo `Commit`, existe exatamente um `AcceptedChangeSet`, um `ProposedChangeSet`, uma versão-base, uma `MissionRevision`, validadores executados e um evento/recibo correlacionável.

**Verificação:** property test sobre commits aceitos; tentativa com elo ausente falha sem escrita parcial.

### INV-AUTH-003 — Capability é autorizada no instante do efeito

Toda invocação externa possui capability tipada/versionada, argumentos validados, decisão de política válida, budget reservado e `IdempotencyKey` quando aplicável. Autorização expirada, de outro escopo ou de outra revisão não é reutilizável.

**Verificação:** policy tests para capability desconhecida, argumentos alterados e autorização obsoleta.

### INV-AUTH-004 — Revisão da missão é imutável

Uma `MissionRevision` aceita nunca é alterada in place. `UserAmendment` cria candidata distinta; somente aceitação explícita troca a revisão ativa e inicia reconciliação da agenda.

**Verificação:** round-trip e diff demonstram preservação da revisão anterior.

## 2. Continuidade e durabilidade

### INV-DUR-001 — Toda unidade não terminal é reavaliável

Toda `Inquiry` e `Operation` não terminal possui exatamente uma condição persistida de elegibilidade ou bloqueio: `ready`, dependência, `not_before`, evento, aprovação, throttle, lease ou replanejamento.

**Verificação:** enumeração de estados e validação estrutural rejeitam unidade órfã.

### INV-DUR-002 — Uma intenção lógica tem no máximo um efeito lógico

Para toda `IdempotencyKey`, commits e recibos representam no máximo um efeito lógico aceito. Replays MAY produzir novas tentativas e eventos, mas não um segundo resultado oficial equivalente.

**Verificação:** property/contract tests repetem comandos e eventos antes, durante e depois de falhas injetadas.

### INV-DUR-003 — Efeito incerto é reconciliado antes de retry

Se a certeza do efeito é `UNKNOWN` ou `PARTIAL`, a intenção original não volta a `READY` até reconciliação ou garantia documentada de idempotência no destino.

**Verificação:** timeout após envio gera `EFFECT_UNKNOWN` e operação de reconciliação, nunca chamada imediata duplicada.

### INV-DUR-004 — Lease não transfere certeza sobre efeito

Expiração ou perda de `Lease` remove exclusividade do worker, mas não prova que um efeito não ocorreu. Recuperação consulta intenção, recibos e estado externo quando necessário.

**Verificação:** crash em cada fronteira da capability e expiração por relógio virtual.

### INV-DUR-005 — Estado e evento correspondente são atômicos

Quando uma mudança local exige evento publicável, estado canônico e `OutboxRecord` são confirmados na mesma transação. Não existe mudança confirmada sem outbox exigida nem outbox referente a mudança abortada.

**Verificação:** falhas injetadas em todos os pontos da transação e contract tests do store.

### INV-DUR-006 — Tempo e aleatoriedade oficiais são injetados

Transições dependentes de tempo usam somente `Clock`; jitter e desempate não determinístico usam somente `RandomSource` registrado. Recuperação não depende do relógio de teste real nem de ordem de mapa/goroutine.

**Verificação:** mesma entrada e fontes controladas produzem mesma decisão e mesmos instantes lógicos.

## 3. Segurança e integridade

### INV-SAFE-001 — Falha fechada preserva estado anterior

Entrada inválida, capability desconhecida, falha não classificada, schema incompatível ou violação de invariante não amplia autoridade e não deixa mutação canônica parcial.

**Verificação:** corpus adversarial e fault injection com comparação exata da versão anterior.

### INV-SAFE-002 — Conteúdo não confiável permanece dado

Texto de fontes, callbacks e modelo não é interpretado como política, autorização, comando ou configuração. Sua proveniência e fronteira de confiança permanecem identificáveis após normalização.

**Verificação:** fixtures de prompt injection e campos que imitam comandos não mudam decisões do kernel.

### INV-SAFE-003 — Segredos não entram em estado epistemológico nem telemetria

Prompts persistidos, artifacts, eventos, erros, traces, métricas e commits MUST NOT conter valores de segredo conhecidos. Referências a segredo são indiretas e redaction ocorre antes da persistência observável.

**Verificação:** canários em headers, query, payload e erro; busca sobre todos os sinks após execução.

### INV-SAFE-004 — Integridade referencial e de conteúdo

Toda `Observation` resolve para `SourceFragment` ou `EvidenceReceipt`; todo `EvidenceLink` resolve seus extremos; artifacts resolvem commit-base e dependências; conteúdo endereçado por hash confere com o hash persistido.

**Verificação:** validação transacional e testes de corrupção deliberada.

### INV-SAFE-005 — Quarentena não contamina o canônico

Objeto em quarentena pode ser inspecionado e referenciado por diagnóstico, mas não pode satisfazer precondição epistemológica nem ser promovido sem nova validação explícita e evento de liberação.

**Verificação:** consultas e changesets recusam IDs apenas em quarentena.

## 4. Progresso e repouso

### INV-PROG-001 — Admissão exige delta observável

Toda `InquiryCandidate` admitida declara origem, novidade, custo, risco, condição de parada e alteração esperada em pelo menos uma dimensão do `KnowledgeState` ou obrigação operacional autorizada.

**Verificação:** testes de admissão rejeitam produção textual, duplicata e reflexão sem delta.

### INV-PROG-002 — Tentativas e replans são limitados

Toda operação que pode retry ou replanejar possui budget persistido e monotonicamente consumido. Nenhuma transição restaura budget sem nova decisão autorizada e auditável.

**Verificação:** modelo/dependência sempre falhos terminam em `EXHAUSTED`, `WAITING_APPROVAL`, `FAILED` ou espera futura explícita.

### INV-PROG-003 — Agenda vazia precede replenishment limitado

Com missão ativa e sem trabalho executável, o kernel executa no máximo o limite configurado de replenishment antes de entrar em `Rest`. Replenishment não pode crescer agenda/frontier sem bound, mas MUST preservar sementes ou obrigações suficientes para nova tentativa futura enquanto a missão permanecer ativa.

**Verificação:** relógio virtual e gerador adversarial demonstram limite de admissões e memória, preservação da frontier e nova tentativa na cadência seguinte.

### INV-PROG-004 — Repouso é continuidade em baixo consumo

`Rest` preserva missão ativa, razão, perguntas abertas, frontier e próxima condição interna de reavaliação temporal. Evento ou capacidade MAY antecipar o despertar, mas ausência de evento externo não elimina o próximo ciclo. Antes da condição elegível, nenhum novo ciclo oficial é executado.

**Verificação:** scheduler virtual avança tempo, comprova zero ciclos intermediários, despertar único sem evento externo e retorno ao replenishment.

### INV-PROG-005 — Sucesso corresponde a critério satisfeito

`Operation=SUCCEEDED` exige validação do output e critério do `OperationSpec`; `Inquiry=SUCCEEDED` exige condição de resposta satisfeita. Ausência de erro, produção de texto ou esgotamento de budget não equivale a sucesso.

**Verificação:** testes de máquina de estados separam `SUCCEEDED`, `EXHAUSTED` e `FAILED`.

### INV-PROG-006 — Estagnação é detectável e localizada

Sequência limitada de operações equivalentes sem novo recibo, evidência, mudança de versão ou redução justificável de incerteza gera `REPETITION_DETECTED` ou `NO_EPISTEMIC_DELTA` e força replanejamento, espera localizada ou intervenção nessa linha. A detecção MUST NOT paralisar linhas independentes nem concluir globalmente uma missão ativa.

**Verificação:** histórico sintético repetitivo aciona o `ProgressMonitor` no limiar configurado enquanto outra linha elegível continua.

### INV-PROG-007 — Bloqueio externo não é bloqueio global

Toda espera por usuário, aprovação, callback ou dependência referencia explicitamente as unidades afetadas. Existindo outra unidade elegível ou candidato independente admissível, o scheduler não pode entrar em estado global dependente daquela espera.

**Verificação:** cenários com pergunta sem resposta mantêm a espera persistida e demonstram seleção de trabalho independente.

### INV-PROG-008 — Missão ativa não conclui implicitamente

A ausência momentânea de agenda, resposta externa, fonte nova ou capacidade disponível não altera a missão para estado concluído. Somente comando autorizado de pausa/cancelamento, revisão que satisfaça condição terminal explícita ou falha fatal do armazenamento pode interromper a obrigação global de reavaliação.

**Verificação:** máquinas de estado rejeitam transição automática de agenda vazia ou `Rest` para conclusão global.

## 5. Propriedades de liveness condicionais

Invariantes de segurança sozinhas não garantem avanço. O runtime MUST satisfazer as propriedades abaixo quando as premissas declaradas permanecem verdadeiras:

### LIVE-001 — Trabalho elegível eventualmente é considerado

Se uma operação permanece `READY`, possui prioridade finita, recursos e budget disponíveis, não há pausa/cancelamento e o scheduler continua ciclando, ela é selecionada ou uma razão persistida explica por que deixou de ser elegível.

### LIVE-002 — Espera temporal eventualmente é reavaliada

Se `not_before <= Clock.Now()` e o runtime está operacional, a unidade volta a ser considerada sem exigir evento externo.

### LIVE-003 — Outbox confirmada eventualmente converge

Se o destino volta a responder e a política permite tentativas, todo `OutboxRecord` pendente é entregue ou termina em estado explícito reconciliável/quarentenado; não permanece silenciosamente esquecido.

### LIVE-004 — Agenda vazia entra em repouso vivo e reavalia

Se replenishment não encontra candidato admissível e não há trabalho/espera vencida, o ciclo entra em `Rest` em número limitado de passos. Se a missão continuar ativa e o armazenamento operacional, um prazo interno posterior eventualmente desperta o kernel para reconciliar esperas, revisar obrigações recorrentes e executar novo replenishment, mesmo sem evento externo.

### LIVE-005 — Linha bloqueada não impede linha independente

Se uma unidade aguarda usuário ou dependência e outra permanece elegível sob recursos e política disponíveis, a unidade independente eventualmente é considerada sem exigir resolução da espera original.

### LIVE-006 — Frentes finitas retornam ao ciclo permanente

Quando todas as unidades do horizonte atual atingem estado terminal, uma missão ativa eventualmente retorna a revisão da frontier, manutenção recorrente ou replenishment. O término do horizonte não é término do runtime.

**Nota:** essas propriedades não prometem efeito externo impossível quando dependências permanecem indisponíveis, budgets acabam, a missão é contraditória ou o armazenamento falha. Elas exigem que o motor permaneça vivo, explique a condição, preserve a intenção e continue qualquer trabalho independente permitido. Somente falha fatal do armazenamento ou comando autorizado pode interromper globalmente o ciclo.

## 6. Estratégia de verificação

A implementação SHOULD manter uma matriz `invariante → validador → testes → eventos`. O baseline exige:

- testes de tabela para cada transição operacional;
- property tests para atomicidade, idempotência e monotonicidade de budgets;
- fuzzing de parsers e entradas não confiáveis;
- contract tests reutilizáveis de armazenamento, outbox e adapters;
- fault injection em limites antes/durante/depois de commit e efeito externo;
- relógio, IDs e aleatoriedade controlados;
- teste de sistema com crash/replay e golden state;
- `go test -race ./...` para implementações concorrentes.

Uma violação detectada em runtime MUST gerar `INVARIANT_VIOLATION`, preservar diagnóstico redigido e impedir continuação no escopo afetado até recuperação segura.
