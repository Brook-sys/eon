# Trabalho contínuo enquanto ativo

Status: decisão arquitetural v0.1

## 1. Regra central

Enquanto uma `MissionRevision` estiver em estado `ACTIVE` e o armazenamento estiver operacional, o runtime **não possui estado global de repouso**.

A ausência de uma operação pronta não é uma razão para dormir. É um sinal de que o horizonte de trabalho está insuficiente e deve acionar imediatamente descoberta, manutenção, revisão ou expansão de trabalho alinhado à missão.

O runtime mantém uma lista de trabalho permanentemente renovável, mas não uma fila materializada sem limite. Há duas camadas:

- **horizonte executável**: conjunto pequeno, priorizado e limitado de operações prontas ou próximas de prontas;
- **reservatório de oportunidades**: frontier persistida de lacunas, ideias, decomposições, melhorias, fontes, verificações e obrigações ainda não admitidas.

Antes que o horizonte fique criticamente baixo, o supervisor o reabastece a partir do reservatório. Ao concluir qualquer trabalho, registra não apenas o resultado, mas também os próximos passos legitimamente derivados. Assim, a lista permanece abastecida sem antecipar milhares de operações, congelar prioridades ou consumir recursos apenas para fabricar backlog.

O ciclo global é:

```text
observe
→ selecionar trabalho pronto
→ se não houver, diagnosticar por que o horizonte secou
→ executar uma estratégia de continuidade aplicável
→ admitir nova operação
→ executar
→ verificar e persistir o delta
→ expandir novamente
```

`PAUSED`, `STOPPING`, `CANCELLED` e falha fatal são condições explícitas diferentes de atividade. Esperas continuam existindo somente no escopo da unidade que depende de tempo, rate limit, aprovação, callback ou recurso. Uma espera local nunca se transforma em repouso global.

## 2. Compromisso de liveness

Em cada fronteira de scheduling de uma missão ativa, o supervisor MUST chegar a um dos resultados:

1. despachar uma `Operation` elegível;
2. executar uma estratégia determinística de continuidade que produza candidatos;
3. registrar uma violação de continuidade porque nenhuma ação segura e útil pôde ser encontrada, juntamente com diagnóstico e escalonamento explícito.

O resultado 3 é degradação/falha observável, não repouso normal. O runtime não deve representar “não encontrei trabalho” como comportamento esperado.

Esta regra não autoriza fabricar atividade. Cada operação autogerada continua exigindo origem, novidade, delta observável, budget e condição de parada. O que deve ser amplo é o repertório de trabalho legítimo, não a tolerância a loops vazios.

## 3. Portfólio de trabalho

A continuidade não deve depender de um único gerador. O runtime mantém um registro versionado e extensível de famílias de `OperationSpec`. Novas famílias podem ser adicionadas sem alterar o scheduler, desde que respeitem os contratos de autoridade, evidência e validação.

### 3.1 Descoberta e cobertura

- decompor pergunta ampla em lacunas menores;
- identificar perguntas ainda não representadas;
- localizar conceitos mencionados mas não definidos;
- detectar entidades, relações, períodos ou escopos ausentes;
- descobrir claims relevantes sem fonte;
- descobrir seções de artifacts sem cobertura suficiente;
- gerar consultas alternativas para lacunas existentes;
- procurar fontes primárias melhores;
- procurar fontes independentes para corroborar ou contestar;
- mapear áreas da missão ainda sem inquiries.

### 3.2 Aquisição e proveniência

- buscar novas fontes autorizadas;
- adquirir versões imutáveis;
- segmentar conteúdo;
- verificar hashes e offsets;
- reparar metadados incompletos;
- identificar origem comum entre fontes aparentemente independentes;
- classificar fonte por tipo e relação com a afirmação;
- reavaliar fonte que mudou ou envelheceu.

### 3.3 Extração e estruturação

- extrair observações atômicas;
- preservar qualificadores, modalidade, tempo e negação;
- localizar citações exatas;
- separar fato, inferência, opinião e hipótese;
- normalizar entidades e termos;
- vincular observações a fragmentos;
- dividir claims compostos;
- detectar duplicatas semânticas candidatas.

### 3.4 Evidência e conflito

- vincular evidência de apoio, oposição ou qualificação;
- identificar claims sem evidência suficiente;
- procurar contraevidência;
- comparar escopos antes de declarar contradição;
- investigar conflitos pendentes;
- detectar falsa corroboração;
- recalibrar confiança após nova evidência;
- revisar conclusões dependentes de claim alterado.

### 3.5 Síntese e artifacts

- criar síntese ausente;
- atualizar síntese obsoleta;
- melhorar cobertura de artifact;
- reduzir redundância sem perder proveniência;
- verificar se cada conclusão possui suporte navegável;
- produzir visão por público, escopo ou período autorizado;
- comparar revisões e explicar deltas;
- regenerar artifacts afetados por commits.

### 3.6 Verificação e qualidade

- validar integridade referencial;
- verificar citações byte a byte;
- verificar coerência entre claims e evidências;
- procurar artifacts obsoletos;
- detectar repetição e ausência de delta;
- auditar aderência à missão;
- revisar itens em quarentena;
- reexecutar validadores após mudança de schema;
- amostrar resultados anteriores para controle de qualidade;
- procurar regressões em operações antes confiáveis.

### 3.7 Manutenção temporal

- revalidar informação sujeita a envelhecimento;
- atualizar fontes dinâmicas;
- revisar deadlines, leases e efeitos ambíguos;
- reconciliar outbox e callbacks;
- reconsiderar trabalho antes bloqueado cuja condição mudou;
- revisar prioridades conforme idade, risco e dependências;
- compactar projeções derivadas preservando o canônico;
- aplicar políticas de retenção autorizadas.

Política MVP (`store-retention.v1` em `domain.StoreRetentionPolicy`):

- o **event log canônico é append-only** — prune/GC de eventos **não** é ação autorizada;
- ações autorizadas: refresh de artefatos derivados obsoletos (FR-KNOW-005), higiene/compactação de frontier (`WorkOpportunity`), trim de buffers descartáveis de export OTLP (`ExportRetention`);
- pressão de crescimento do head de eventos e contagem de artefatos stale são **alertas de apresentação** (soft thresholds), nunca gatilhos de exclusão;
- backup/export (`sqlite.BackupTo`) é o caminho operacional para controle de footprint, não delete seletivo do log.
- execução autorizada de refresh: `view.Refresher` regenera apenas `cited_claim_view` already-stale sob novo `ArtifactID` (prior permanece stale); `LocalExecutor` family `artifact_refresh` marca base≠head e regenera batch bounded; dry-run operador em `GET /store/retention` (sem mutação).

### 3.8 Melhoria do harness

- avaliar operação contra corpus versionado;
- comparar formatos de prompt e saída;
- identificar padrão recorrente de falha;
- propor validador determinístico;
- propor decomposição menor;
- ajustar seleção de contexto por evidência empírica;
- promover ou rebaixar perfil de capacidade do modelo;
- comparar modelo barato com baseline;
- detectar uso desnecessário de modelo onde regra basta;
- registrar oportunidade de reduzir tokens, latência ou retries.

Mudança no próprio protocolo, código ou política continua sujeita à autoridade definida. O runtime pode diagnosticar e propor melhorias sem receber autoridade automática para reescrever seus controles.

### 3.9 Gestão da própria frontier

- mesclar candidatos duplicados;
- dividir candidato grande demais;
- substituir estratégia esgotada;
- estimar ganho marginal;
- revisar candidatos adiados;
- diversificar frentes para evitar fixação;
- verificar cobertura por dimensão da missão;
- gerar o próximo horizonte curto;
- registrar por que uma frente foi abandonada e qual a substituiu.

#### Lifecycle executável do reservatório (MVP)

O domínio expõe transições puras de higiene em `WorkOpportunity` (sem autoridade de admissão):

| Evento | De | Para | Efeito |
|--------|----|------|--------|
| `DEFER` | OPEN | DEFERRED | parqueia com razão opcional; recuperável |
| `REOPEN` | DEFERRED | OPEN | reativa; limpa detalhe de estacionamento |
| `ABANDON` | OPEN/DEFERRED | ABANDONED | elimina com razão obrigatória |
| `SUPERSEDE` | OPEN/DEFERRED | SUPERSEDED | substitui por id sucessor distinto |

`ADMITTED` não entra na higiene: a saída da agenda permanece com o Admitter/kernel de operações.

`PlanFrontierReservoirHygiene` (e o wrapper `PlanFrontierHygiene` só-OPEN) aplica `HorizonPolicy` de forma determinística sobre OPEN∪DEFERRED:

1. `SUPERSEDE` duplicatas com a mesma `DedupSignature` exata (vencedor: OPEN antes de DEFERRED, maior prioridade, `UpdatedAt` mais novo, id menor);
2. `ABANDON` unidades OPEN com `depth > max_depth` (crescimento residual ilegal);
3. se o restante OPEN excede `max_candidates`, `DEFER` as de menor prioridade (desempate: `UpdatedAt` mais antigo, depois id);
4. enquanto houver vagas sob `max_candidates`, `REOPEN` as DEFERRED de maior prioridade — unidades deferidas no mesmo plano **não** reabrem no mesmo ciclo.

A família local `frontier_management` (`LocalExecutor`) materializa o plano na mesma transação da operação: `SaveWorkOpportunity`, eventos `continuity.opportunity_*` e resumo `continuity.frontier_compacted`, além do artefato `frontier_manage_report` com contagens/findings (inclui `hygiene_superseded_count` / `hygiene_reopened_count`). Modelo não escolhe vencedores, reaberturas nem abandons.

### 3.10 Decomposição e melhoria recursiva

#### Catálogo versionado e split estrutural (MVP executável)

- `StrategyRegistry` carrega um portfólio com `CatalogVersion` explícito (`continuity-catalog.v3`) e cada família com `StrategyDescriptor.Version`.
- FR-DUR-011: a estratégia local `recurring_obligations@v1` (prioridade 40) materializa `MissionRevision.RecurringObligations` em raízes `WorkOpportunity` com cadência, budget, `delta_criterion` e anti-repetição; assinaturas por bucket e fingerprint de head commit impedem atividade vazia.
- Diagnosis e auditoria usam refs estáveis `name@version` (`StrategyDescriptor.Ref` / `StrategyRefs`); cooldown continua indexado só pelo `Name()` da estratégia.
- `PlanChildDraftsFromStoreWithPolicy` decompõe inventários agregados de gap/coverage/integrity/conflict em drafts ortogonais (assinaturas distintas, prioridade estável) e aplica `HorizonPolicy.MaxChildren` via `capChildDrafts` antes do `Decomposer`.
- Modelo não escolhe o split: contagens e ordem vêm de joins determinísticos; fallback estático permanece quando o grafo não apresenta gap.

- decompor missão, objetivo, inquiry, tarefa ou artifact amplo em unidades menores;
- decompor conhecimento anterior em claims, perguntas, pressupostos, dependências e testes;
- transformar resumo em mapa de lacunas e mapa de lacunas em novas inquiries;
- melhorar proposta, plano, instrução, consulta, resumo ou conteúdo existente quando houver critério objetivo de ganho;
- comparar versões e preservar somente melhorias com delta verificável;
- identificar detalhes omitidos, ambiguidades, contradições e pressupostos frágeis;
- sugerir ferramentas, adapters, validadores e fontes de dados relevantes;
- pesquisar alternativas existentes antes de propor mecanismo próprio;
- preparar pesquisa superficial, focal ou aprofundada conforme valor e budget;
- converter falha, bloqueio ou baixa confiança em diagnóstico e tarefas menores;
- revisar conhecimento antigo sob nova evidência, novo schema ou nova perspectiva autorizada;
- derivar novas tarefas dos próprios resultados sem conceder autoridade ao modelo para admiti-las diretamente.

Essa recursão precisa terminar localmente: cada decomposição possui profundidade, fan-out, budget e condição de parada. Uma tarefa filha deve reduzir escopo, aumentar verificabilidade, revelar uma dependência real ou produzir outro ganho declarado; renomear ou parafrasear a tarefa pai não conta como progresso.

## 4. Lista de trabalho permanentemente abastecida

O supervisor usa marcas configuráveis para manter um horizonte saudável:

```text
target_ready        quantidade desejada de trabalho pronto
low_watermark       nível que aciona replenishment preventivo
max_ready           limite de operações prontas materializadas
max_candidates      limite do reservatório ativo antes de compactação/revisão
max_children        fan-out máximo por decomposição
max_depth           profundidade máxima de decomposição recursiva
```

Esses valores são políticas versionadas, não decisões livres do modelo. O modelo pode propor candidatos e relações; código determinístico controla:

- identidade e idempotência;
- deduplicação exata e semântica assistida;
- aderência à missão e à revisão ativa;
- dependências e precedência;
- budgets e capabilities permitidas;
- novidade e ganho esperado;
- risco, reversibilidade e autoridade;
- prioridade, admissão e limites de cardinalidade;
- cooldown de estratégias sem delta;
- promoção, adiamento, compactação e eliminação.

Cada ciclo relevante deve preservar uma das seguintes condições:

1. há trabalho pronto suficiente no horizonte;
2. há replenishment em andamento com candidatos persistidos;
3. há oportunidades futuras legítimas com condições locais de ativação;
4. existe diagnóstico persistido explicando por que a continuidade foi violada.

O alvo operacional normal é 1 ou 2. A condição 3 não autoriza repouso global: outras famílias continuam sendo procuradas. A condição 4 é `CONTINUITY_BLOCKED` e exige correção ou mudança observável de capacidade.

## 5. Estratégias quando uma frente seca

O scheduler não repete indefinidamente o mesmo replenisher. Ele percorre estratégias de continuidade em ordem política e com memória de tentativas recentes:

```text
ready work
→ due local work
→ unresolved high-value gaps
→ dependent artifact refresh
→ conflict and evidence review
→ mission coverage scan
→ source freshness scan
→ quality/integrity audit
→ harness evaluation
→ frontier diversification
→ continuity diagnosis
```

Uma estratégia que não produz delta entra em cooldown local e outra família é tentada. Cooldown da estratégia não é repouso do runtime.

A política deve evitar fixação usando:

- diversidade mínima de famílias;
- penalidade por repetição;
- histórico de ganho marginal;
- limite por frente e por família;
- cobertura de dimensões da missão;
- preferência por lacunas de alto valor ainda não tentadas;
- rotação entre descoberta, verificação, manutenção e síntese.

## 6. Recursos indisponíveis

Rate limit, budget de uma capability, espera humana ou provider indisponível bloqueiam somente operações que dependem deles. O supervisor procura trabalho local ou outra capability.

Perguntar ao operador é uma capability legítima de aquisição de informação. O modelo pode propor uma pergunta estruturada quando preferência, esclarecimento ou conhecimento privado puder alterar uma decisão relevante. A entrega passa por política determinística de necessidade e antispam. A pergunta cria `WAITING_EVENT` somente nas unidades explicitamente dependentes; silêncio do operador nunca suspende outras famílias nem impede replenishment. Dashboard e Telegram são os primeiros canais previstos, ambos correlacionados ao mesmo objeto canônico de pergunta.

Exemplos de trabalho possível sem modelo ou rede:

- auditoria de integridade;
- análise da frontier;
- detecção de duplicatas por regras;
- propagação de obsolescência;
- renderização de artifacts já suportados;
- avaliação de cobertura;
- reconciliação de estado;
- preparação de consultas e contextos futuros;
- execução de testes e benchmarks autorizados;
- diagnóstico e propostas de melhoria do harness.

Se todas as capabilities relevantes estiverem indisponíveis e nenhum trabalho local útil existir, o runtime registra `CONTINUITY_BLOCKED` com causas concretas. Ele pode bloquear tecnicamente aguardando a primeira condição recuperável, mas isso é uma incapacidade operacional excepcional e observável, não um estado normal chamado repouso. Ao recuperar qualquer capacidade, retoma imediatamente a procura de trabalho.

## 7. Antiatividade artificial

“Trabalhar sempre” não significa consumir CPU, tokens ou rede sem ganho. São proibidos:

- polling apertado;
- reescrita sem novo delta;
- repetição de busca equivalente sem hipótese nova;
- retries além do budget;
- criação de perguntas apenas para manter fila cheia;
- geração de texto que não altera conhecimento, evidência, artifact, diagnóstico ou capacidade;
- alternância infinita entre estados sem recibo novo;
- manutenção antes da cadência ou sem gatilho verificável.

Cada operação deve produzir ao menos um dos seguintes resultados:

- mudança canônica validada;
- evidência ou fonte nova;
- redução mensurável de incerteza;
- conflito ou lacuna nova e justificada;
- artifact útil atualizado;
- falha diagnosticada com nova informação;
- melhoria proposta e empiricamente sustentada;
- eliminação justificada de trabalho inválido ou redundante.

## 8. Migração do runtime

A implementação anterior de `Rest`, `DecisionRest`, `SaveRest` e `Scheduler.Wait` representava a decisão substituída e foi removida do domínio, portas, store e scheduler.

A migração deve:

1. manter `Rest` ausente do domínio e das portas de armazenamento;
2. evoluir `CONTINUITY_BLOCKED` para diagnóstico persistido e correlacionado;
3. ampliar o registro extensível de estratégias de continuidade;
4. manter `WAITING_TIME`, `THROTTLED`, `WAITING_EVENT`, `WAITING_APPROVAL` e backoff como estados locais;
5. adicionar evento `CONTINUITY_GAP_DETECTED` e falha `CONTINUITY_BLOCKED`;
6. testar que uma agenda vazia tenta famílias alternativas até admitir trabalho ou produzir diagnóstico explícito;
7. testar que uma linha bloqueada não impede outra família;
8. testar longevidade com diversidade, delta e ausência de polling vazio.
9. implementar marcas de reabastecimento, limites de frontier e decomposição recursiva limitada.

## 9. Critério experimental

O runtime demonstra continuidade ativa quando, durante uma execução longa com missão ampla e recursos disponíveis:

- nunca entra em estado global de repouso;
- sempre há operação em execução, pronta ou sendo legitimamente gerada;
- o horizonte executável permanece próximo do alvo configurado sem crescimento ilimitado;
- resultados alimentam o reservatório com próximos passos rastreáveis;
- frentes esgotadas são substituídas por outras;
- o portfólio de famílias evita dependência de um único padrão de tarefa;
- operações repetitivas sem delta são detectadas e abandonadas;
- consumo permanece dentro dos budgets;
- toda atividade continua explicável pela missão;
- pausa só ocorre por comando explícito do operador;
- bloqueio global aparece como falha/degradação que exige correção, não como comportamento esperado.
