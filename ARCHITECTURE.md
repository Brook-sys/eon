# Motor Autônomo — Arquitetura inicial

Status: rascunho v0.7

Decisões técnicas e critérios verificáveis estão em `TECHNICAL_REQUIREMENTS.md`. O vocabulário normativo está em `GLOSSARY.md`. ADRs aceitos fixam Go como linguagem do núcleo, OpenAI-compatible como adapter principal de modelos e SQLite + event log como backend canônico do MVP.

## Tese

A inteligência principal deve estar no sistema, não depender exclusivamente do modelo.
O motor deve continuar útil com modelos pequenos, antigos, gratuitos e com janelas de contexto reduzidas.

Seu propósito principal é **continuidade progressiva permanente**: permanecer vivo, preservar estado, sempre ultrapassar o horizonte atual e produzir a próxima frente útil de trabalho. Enquanto a missão estiver ativa e o armazenamento operacional, o motor não possui conclusão global implícita; objetivos, investigações e operações individuais são finitos, mas seu término retorna ao ciclo de manutenção, melhoria e replenishment.

Continuidade não significa loop ocupado nem chamadas incessantes ao modelo. Significa que o runtime nunca depende de comandos, respostas do usuário ou eventos externos para permanecer vivo e procurar avanço: ele reavalia periodicamente sua missão, seu estado e sua capacidade, incrementa a agenda com novas `Inquiry`s derivadas e executa a melhor `Operation` permitida. Eventos externos — inclusive respostas do usuário — alteram fatos, desbloqueiam linhas e repriorizam o trabalho, mas silêncio ou ausência de eventos bloqueiam apenas as unidades dependentes, nunca o motor inteiro.

## Princípios

1. Núcleo determinístico; IA usada apenas onde ambiguidade e julgamento são necessários.
2. Estado persistente fora do contexto do modelo.
3. Contexto montado sob demanda e limitado por orçamento.
4. Toda ação produz evidência verificável.
5. Componentes substituíveis por contratos estáveis.
6. Reinício e retomada sem perda do progresso.
7. Falha fechada para ações sem permissão ou sem validação.
8. Um modelo fraco pode exigir mais passos, mas não deve romper o protocolo.
9. Esperar é trabalho válido: o motor deve dormir sem perder estado e acordar por prazo, disponibilidade ou evento.
10. Limites são entradas do scheduler, não exceções improvisadas.
11. Toda operação com efeito colateral deve ser idempotente ou possuir chave de deduplicação.
12. Nenhuma dependência externa, incluindo o modelo, controla a continuidade do kernel.
13. Conversas entre agentes não fazem parte do núcleo; coordenação ocorre por estado, eventos e contratos.
14. A agenda é renovável: concluir uma tarefa deve revelar, validar ou gerar próximas tarefas alinhadas à missão.
15. Eventos externos influenciam o rumo, mas não são requisito para progresso.
16. A geração autônoma de tarefas permanece limitada pela missão, políticas e orçamento definidos pelo operador.
17. O modelo é um resolvedor textual limitado, não um agente, scheduler ou executor.
18. Tool calling nativo é uma otimização opcional; o protocolo básico funciona com texto simples.
19. Problemas complexos são atravessados por microturnos persistentes, não entregues inteiros ao modelo.
20. Prompts são compilados por operação, curtos, versionados, testáveis e sem histórico conversacional desnecessário.
21. O núcleo é implementado em Go e distribuído como binário portátil sempre que a plataforma permitir.
22. Integração com modelos ocorre por contrato interno e adapter OpenAI-compatible com capacidades graduais.
23. Modularidade, continuidade, tolerância a falhas e testabilidade são propriedades verificadas por suites e cenários, não apenas objetivos declarados.

## Visão em camadas

```text
┌────────────────────────────────────────────────────────────────┐
│ Interfaces: CLI, API de inspeção, eventos autorizados          │
├────────────────────────────────────────────────────────────────┤
│ Control Plane: MissionSpec, políticas, budgets e aprovação     │
├────────────────────────────────────────────────────────────────┤
│ Kernel: supervisor, scheduler, eventos, espera e retomada      │
├────────────────────────────────────────────────────────────────┤
│ Inquiry: agenda, frontier, admissão e prioridade               │
├────────────────────────────────────────────────────────────────┤
│ Epistemic: fontes, observações, claims, evidências e artefatos │
├────────────────────────────────────────────────────────────────┤
│ Cognition: OperationSpec, prompt compiler, modelo e verifier   │
├────────────────────────────────────────────────────────────────┤
│ Ports: web/file read, model, store, clock e observabilidade    │
├────────────────────────────────────────────────────────────────┤
│ Persistência: estado canônico, event log, outbox e artefatos   │
└────────────────────────────────────────────────────────────────┘
```

## Kernel mínimo

O kernel não interpreta linguagem natural. Ele somente opera estados e comandos válidos.

Estados iniciais de uma `Inquiry` ou `Operation`:

- `NEW`: unidade criada, ainda não normalizada ou admitida.
- `READY`: precondições satisfeitas e próxima execução permitida.
- `RUNNING`: uma ação está em execução.
- `VERIFYING`: o resultado está sendo comparado aos critérios.
- `WAITING_TIME`: aguarda um instante ou intervalo calculado.
- `WAITING_EVENT`: aguarda callback, mensagem, arquivo ou outro evento.
- `WAITING_APPROVAL`: aguarda autorização humana.
- `THROTTLED`: aguarda capacidade de um rate limiter.
- `BLOCKED_DEPENDENCY`: uma dependência externa está indisponível.
- `REPLANNING`: estratégia falhou ou estagnou.
- `SUCCEEDED`: critérios de conclusão satisfeitos.
- `EXHAUSTED`: orçamento próprio da tarefa terminou e exige nova decisão ou intervenção.
- `FAILED`: erro terminal explicitamente classificado como não recuperável.
- `CANCELLED`: interrompido externamente.

Loop:

```text
observe → select → prepare → act → verify → commit → repeat
```

Cada transição deve ser registrada em um log append-only.

O runtime global possui um ciclo diferente:

```text
recover → observe state/capacity/time → ingest optional events
        → replenish agenda → prioritize → dispatch → verify
        → learn/expand frontier → persist → calculate next cycle → sleep
```

O ciclo possui três fontes de ativação:

1. **tempo**: chegou o próximo ciclo periódico ou prazo interno;
2. **disponibilidade**: um recurso, cota ou capacidade voltou a estar livre;
3. **evento externo opcional**: chegou informação que pode alterar estado, prioridade ou direção.

Se a fila executável estiver vazia, o motor não conclui que terminou. Primeiro executa `replenish agenda`: examina missão, lacunas, resultados recentes, riscos, oportunidades e tarefas recorrentes para produzir candidatos. Se ainda não houver ação útil e permitida, calcula o próximo ciclo e dorme. Ele pode ser acordado antecipadamente por disponibilidade ou evento.

Seu comportamento contínuo é, portanto, **time-and-availability-driven**, com eventos externos como modificadores opcionais.

## Invariante de continuidade

Enquanto não houver uma ordem explícita de desligamento ou falha fatal do armazenamento:

1. todo trabalho aceito permanece representado no estado persistente;
2. todo trabalho não terminal possui uma condição explícita para voltar a ser avaliado;
3. após reinício, o runtime reconstrói filas, esperas, leases e callbacks pendentes;
4. nenhuma resposta de modelo é necessária para o motor saber como retomar;
5. rate limits e dependências indisponíveis adiam trabalho, mas não apagam intenção;
6. o motor pode permanecer em `Rest` com consumo mínimo, mas sempre preserva prazo interno de reavaliação e retorna ao ciclo mesmo sem evento externo;
7. uma agenda vazia dispara geração controlada de candidatos antes do `Rest`, e a frontier preserva sementes ou obrigações para ciclos futuros;
8. espera por usuário, aprovação ou dependência bloqueia somente as unidades afetadas; trabalho independente continua;
9. cada nova tarefa possui proveniência que demonstra de qual missão, objetivo, evidência ou obrigação recorrente ela foi derivada;
10. término do horizonte atual retorna à manutenção, revisão e replenishment, nunca a conclusão global implícita.

Isso diferencia:

- **continuidade do motor**: potencialmente indefinida;
- **continuidade de um objetivo**: até conclusão, cancelamento ou intervenção;
- **continuidade de uma tentativa**: curta, limitada por timeout e lease.

## Missão, agenda e fronteira de trabalho

Para continuar sem depender de eventos externos, o runtime precisa de algo mais durável que uma fila: uma **missão operacional**.

### MissionSpec

Define, de forma versionada, o espaço legítimo de progresso:

```json
{
  "id": "mission_...",
  "purpose": "resultado contínuo desejado pelo operador",
  "domains": ["escopos permitidos"],
  "policies": ["restrições obrigatórias"],
  "standing_objectives": ["objetivos permanentes"],
  "cadence": {"review_every_seconds": 900},
  "resource_budget": {"requests_per_day": 100},
  "status": "ACTIVE"
}
```

O motor não cria uma missão independente. Ele deriva trabalho apenas de missões configuradas, resultados observados e obrigações autorizadas.

### Agenda

Conjunto priorizado de `Inquiry`s e obrigações operacionais admitidas. Pode esvaziar temporariamente.

### Work Frontier

Conjunto de lacunas, hipóteses, riscos, oportunidades e próximos passos ainda não transformados em investigações admitidas.

### AgendaReplenisher

Quando necessário, transforma a fronteira em novos `InquiryCandidate`s e admite um conjunto limitado como `Inquiry`s. Fontes possíveis:

- perguntas explícitas ainda sem resposta;
- lacunas ou conflitos revelados por uma investigação concluída;
- claims relevantes sem evidência suficiente;
- evidências inconclusivas ou potencialmente desatualizadas;
- revisão epistemológica recorrente por tempo;
- recursos que voltaram a ficar disponíveis;
- revisão periódica da frontier e prioridades;
- eventos externos que mudaram o estado observado.

Pipeline:

```text
mission + current state + evidence + frontier + capacity
    → generate candidates
    → reject duplicates/out-of-scope/low-value items
    → estimate cost, risk and expected progress
    → admit bounded set into agenda
```

Isso deve ser incremental. O motor não gera um plano gigantesco; mantém apenas um horizonte curto de tarefas prontas e uma fronteira resumida.

## Progresso contínuo

Cada investigação ou operação concluída deve passar por uma etapa de expansão:

```text
resultado
  → o que mudou?
  → quais critérios foram satisfeitos?
  → quais lacunas apareceram?
  → qual é o próximo incremento útil?
  → criar, atualizar, adiar ou eliminar candidatos
```

O conceito central é uma **esteira de incrementos epistemológicos**:

```text
missão → inquiry → operação → evidência/changeset → estado atualizado → próxima inquiry
```

O motor segue em frente mesmo sem entradas externas porque seu próprio estado contém trabalho potencial, obrigações recorrentes e condições de revisão. Se não existe ação segura e útil no horizonte atual, ele persiste `Rest`, agenda nova reavaliação interna e posteriormente tenta ampliar ou renovar o horizonte. A continuidade global termina apenas por pausa/cancelamento autorizado, condição terminal explícita da missão ou falha fatal do armazenamento; nunca apenas porque a agenda atual acabou ou uma resposta externa não chegou.

Para evitar atividade vazia, todo `InquiryCandidate` autogerado deve declarar:

- `derived_from`: origem na missão, evidência ou recorrência;
- `expected_progress`: mudança observável esperada;
- `novelty`: por que não duplica trabalho anterior;
- `cost_estimate`;
- `stop_condition`;
- `review_after`: quando reavaliar caso seja adiada.

Sem progresso esperado demonstrável, o candidato não entra na agenda.

## Eventos externos como mudanças de rumo

Eventos não mantêm o motor vivo; eles atualizam sua percepção.

Um evento pode:

- acrescentar ou corrigir fatos;
- elevar ou reduzir prioridades;
- invalidar tarefas planejadas;
- introduzir nova restrição;
- pausar ou cancelar uma missão;
- desbloquear uma capacidade;
- exigir replanejamento imediato.

Após incorporar o evento, o motor recalcula apenas a parte afetada da agenda. Na ausência de eventos, o ciclo temporal e a expansão da fronteira continuam normalmente.

## Supervisor e scheduler durável

O kernel deve conter dois componentes centrais.

### Supervisor

- inicializa e recupera o estado;
- monitora workers e renova leases;
- converte processos interrompidos em trabalho recuperável;
- mantém health checks de dependências;
- executa shutdown gracioso;
- garante que falhas locais não encerrem o runtime inteiro.

### Scheduler

Escolhe apenas trabalhos que estejam simultaneamente:

- prontos por dependência;
- dentro do orçamento;
- permitidos pela política;
- autorizados pelo rate limiter;
- fora de cooldown ou backoff;
- sem lease ativo em outro worker;
- com recursos disponíveis.

O scheduler não pergunta ao modelo se pode executar. Essa decisão é estrutural e determinística.

## Tempo, rate limits e backpressure

Cada recurso limitado deve possuir um `ResourceGate`, por exemplo:

```text
model:ollama/local
model:provider/free-tier
web:searxng
file:authorized-root
domain:api.example.com
```

Contrato conceitual:

```text
acquire(resource, cost, priority) -> Permit | WaitUntil
report(result, headers, latency)
```

O gate deve entender:

- limites conhecidos por minuto/dia;
- `Retry-After` e cabeçalhos do provedor;
- concorrência máxima;
- custo estimado em tokens;
- cooldown por erro;
- exponential backoff com jitter;
- circuit breaker para dependências instáveis;
- cotas reservadas para operações críticas.

Quando não recebe uma permissão, a operação entra em `THROTTLED` com `not_before`. Nenhum worker fica bloqueado esperando e nenhuma chamada é desperdiçada.

Backpressure deve propagar para cima: se o modelo está saturado, o planner deixa de produzir trabalho cognitivo ilimitado; tarefas determinísticas ainda podem avançar.

## Callbacks e eventos

Callbacks devem entrar pelo mesmo barramento durável de eventos usado internamente.

Envelope mínimo:

```json
{
  "event_id": "evt_...",
  "type": "external.callback.received",
  "correlation_id": "op_...",
  "deduplication_key": "provider:event-id",
  "occurred_at": "...",
  "payload_ref": "artifact_..."
}
```

Requisitos:

- persistir antes de confirmar recebimento;
- deduplicar callbacks repetidos;
- correlacionar com a operação em espera;
- validar autenticidade quando aplicável;
- aceitar callback que chegue antes de o waiter ser registrado;
- definir timeout e política de callback tardio;
- reaplicar eventos após crash sem repetir efeitos colaterais.

O mesmo envelope pode representar timers, conclusão de aquisição web, mudança em arquivo autorizado, disponibilidade de recurso e resposta humana. Mensagens, webhooks e subprocessos só entram quando adapters específicos forem explicitamente autorizados; não fazem parte do MVP básico.

## Entrega e efeitos colaterais

Não é realista prometer execução exatamente uma vez em todos os sistemas externos. O alvo deve ser:

```text
at-least-once delivery + idempotência + deduplicação
```

Antes de executar um efeito externo, o motor cria um registro de intenção com uma `idempotency_key`. Depois registra recibo e evidência. Em reinícios, consulta esse registro antes de repetir a ação.

Para emissão confiável de eventos, usar o padrão transactional outbox: a mudança de estado e o evento a publicar são confirmados na mesma transação local.

## Contratos fundamentais

O vocabulário completo está em `GLOSSARY.md`. Os contratos centrais do domínio são:

### InquiryCandidate

```json
{
  "id": "inquiry_candidate_...",
  "mission_revision": 7,
  "question_id": "question_...",
  "derived_from": ["gap_..."],
  "expected_epistemic_gain": {"coverage": 0.3, "uncertainty_reduction": 0.5},
  "novelty": "não duplica investigação existente",
  "estimated_cost": {"model_calls": 3, "searches": 2},
  "risk": "low",
  "answer_condition": "duas fontes primárias analisadas",
  "stop_condition": "budget ou suficiência atingida",
  "review_after": "2026-07-16T00:00:00Z"
}
```

### Operation

```json
{
  "id": "operation_...",
  "inquiry_id": "inquiry_...",
  "spec_id": "extract_observations@1",
  "read_set": ["fragment_..."],
  "expected_output": "proposed_change_set",
  "attempt": 1,
  "lease_until": null,
  "status": "READY"
}
```

### ModelDecision

```json
{
  "operation_id": "operation_...",
  "proposal_type": "observation_candidates",
  "payload_ref": "artifact_...",
  "reason_code": "EXTRACTION_PROPOSAL",
  "model_confidence": 0.72
}
```

`ModelDecision` é somente proposta. Não autoriza capability nem commit.

### EvidenceReceipt

```json
{
  "operation_id": "operation_...",
  "kind": "schema_validation",
  "producer": "validator:observations@1",
  "passed": true,
  "artifact_ref": "artifact_..."
}
```

`EvidenceReceipt` registra verificação operacional; relações epistemológicas usam `EvidenceLink`.

## Módulos e portas

### 1. ModelProvider

Responsabilidade: completar uma solicitação estruturada.

```text
complete(request, budget) -> ModelResponse
capabilities() -> {json_mode, tool_calls, context_limit, ...}
```

Adaptadores possíveis: Ollama, llama.cpp, APIs compatíveis com OpenAI e provedores gratuitos.

O contrato mínimo exige somente `text -> text`. JSON mode, function calling, streaming e system prompt são capacidades opcionais detectadas pelo adaptador. O kernel nunca depende delas para funcionar.

### 2. InquiryPlanner

Converte perguntas, lacunas, conflitos ou falhas em `InquiryCandidate`s e expansões limitadas de `Operation`s. Pode haver implementações:

- `RuleInquiryPlanner`: regras determinísticas para fluxos conhecidos;
- `LLMInquiryPlanner`: formulação limitada de candidatos;
- `HybridInquiryPlanner`: regras e templates primeiro, modelo apenas para ambiguidades.

O planner não envia a missão inteira ao modelo e aceita um plano completo. Ele usa decomposição hierárquica, validadores e horizonte curto. Cada expansão produz poucos candidatos e nenhuma admissão ocorre sem política determinística.

### 3. Selector

Escolhe o próximo item pronto com base em dependências, prioridade, custo e risco.

### 4. ContextCompiler

Monta um pacote mínimo para uma chamada. Pipeline sugerido:

```text
necessidades da Operation
  → busca de fatos/artefatos
  → ranking por relevância
  → deduplicação
  → compressão
  → corte pelo orçamento
  → envelope final
```

O contexto deve conter identidade da operação, critérios, objetos canônicos necessários e formato de saída; não a conversa inteira.

O `ContextCompiler` também atua como `PromptCompiler`: seleciona um template específico à operação, substitui referências por fatos compactos, enumera opções válidas e reserva espaço de saída. O prompt resultante deve poder ser reproduzido por ID e versão.

### 5. CapabilityRegistry

Registro de capacidades instaláveis. Cada capability declara:

- nome e versão;
- schema de entrada e saída;
- efeitos colaterais;
- nível de risco;
- permissões necessárias;
- timeout e política de repetição;
- função opcional de verificação.

Exemplos do MVP: `file.discover`, `file.read`, `web.search`, `web.fetch`, `source.snapshot`, `model.complete` e `artifact.render`. Shell e execução arbitrária de código permanecem fora do produto MVP.

### 6. Executor

Executa uma capability após autorização, aplica timeout e captura saída, erro e artefatos.

### 7. Verifier

Não pergunta apenas ao modelo se algo funcionou. Ordem de preferência:

1. verificador determinístico;
2. invariantes e schemas;
3. comparação com exemplos ou testes;
4. verificação por modelo independente;
5. revisão humana.

### 8. Persistência por portas explícitas

Evitar um `MemoryStore` genérico. Separar responsabilidades em interfaces estreitas:

- `MissionRepository`: revisões e missão ativa;
- `AgendaRepository`: frontier, candidates, inquiries, operações e leases;
- `KnowledgeRepository`: fontes, observações, claims, evidências e dependências;
- `EventLog`: eventos append-only;
- `Outbox`: entregas transacionais pendentes;
- `ArtifactStore`: snapshots e materializações grandes.

A implementação fake/in-memory sustenta contract tests. SQLite + event log é o backend canônico do MVP após o spike comparativo; Dolt permanece adapter experimental e qualquer alternativa futura deve implementar os mesmos contratos antes de novo ADR.

### 9. PolicyEngine

Decide `allow`, `deny` ou `require_approval` usando a capability, argumentos, risco, escopo e orçamento.

### 10. ProgressMonitor

Detecta:

- repetição da mesma ação e argumentos;
- ausência de nova evidência;
- erros recorrentes;
- consumo anormal de orçamento;
- ciclos entre estados;
- degradação da confiança.

Pode ordenar replanejamento, fallback de modelo, redução de escopo ou intervenção humana.

## Sistema de plugins

Tipos de plugin iniciais:

- `model_provider`
- `inquiry_planner`
- `context_source`
- `capability` — somente portas autorizadas do escopo epistemológico
- `verifier`
- `storage_backend`
- `policy`
- `interface`
- `observer`

Manifesto conceitual:

```yaml
id: web.search.searxng
kind: capability
version: 1.0.0
api_version: 1
risk: network_read
input_schema: schemas/search-input.json
output_schema: schemas/search-output.json
entrypoint: plugin:SearchCapability
```

Plugins não acessam o kernel diretamente. Recebem interfaces estreitas e retornam objetos validados.

## Estratégias específicas para modelos fracos

O modelo deve ser tratado como um componente potencialmente inconsistente, com baixa capacidade de abstração, instrução e formatação. A arquitetura assume que ele pode ignorar schemas, inventar ferramentas, perder restrições e produzir texto extra.

1. Saídas pequenas, tipadas e validadas.
2. Uma decisão cognitiva por chamada.
3. Vocabulário de ações limitado por estado.
4. Protocolo textual básico sem dependência de tool calling.
5. Reparo determinístico antes de qualquer nova chamada.
6. Exemplos curtos específicos para a operação atual.
7. Contexto de fatos em vez de transcrições.
8. Planejamento progressivo: detalhar somente os próximos passos.
9. Verificadores externos ao modelo.
10. Fallback entre modelos por competência, não apenas por disponibilidade.
11. Escolha fechada sempre que possível; geração aberta somente quando necessária.
12. Separar interpretar, decidir, produzir e revisar em microturnos diferentes.
13. Nunca permitir que texto do modelo execute uma ferramenta diretamente.
14. Avaliar cada template com os modelos-alvo mais fracos.

O protocolo detalhado está em `WEAK_MODEL_PROTOCOL.md`.

## Unidade de modularidade

A unidade principal não deve ser “um agente com personalidade”, mas uma operação:

```text
Operation = Contract + Context Policy + Decision Strategy + Capability + Verifier
```

Isso permite trocar o modelo, executor ou verificador sem reescrever o fluxo inteiro.

## MVP recomendado

Escopo: construção e manutenção contínua de uma base de conhecimento orientada por missão.

Inclui:

- núcleo Go serial no primeiro slice, com concorrência limitada posteriormente;
- armazenamento por interface, com fake/in-memory para testes e SQLite + event log como backend canônico do MVP;
- adapter de modelo OpenAI-compatible;
- contrato cognitivo mínimo texto-para-texto;
- compilador de contexto por regras e `OperationSpec`;
- ingestão de fontes web e arquivos autorizados;
- operações sobre observações, claims, evidências, perguntas e sínteses;
- `ProposedChangeSet` validado antes de alteração oficial;
- CLI para iniciar, inspecionar, pausar e retomar missões;
- event log e observabilidade estruturada.

Não inclui inicialmente:

- automação geral de computador;
- shell ou execução arbitrária de código;
- múltiplos agentes conversando livremente;
- banco vetorial como fonte canônica;
- execução distribuída;
- interface visual complexa;
- geração dinâmica irrestrita de ferramentas;
- autonomia sem missão, orçamento ou limites.

## Métrica central

Comparar o mesmo modelo e a mesma missão com variantes do harness:

- ganho epistemológico por custo;
- cobertura de perguntas relevantes;
- precisão e cobertura de citações;
- claims sem evidência e conflitos não examinados;
- erros não detectados;
- recuperação após interrupção;
- repetição e atividade sem progresso;
- fidelidade de sínteses aos claims e fontes;
- desempenho ao reduzir artificialmente a janela de contexto.

A promessa será demonstrada se o sistema mantiver progresso epistemológico rastreável e baixa corrupção da base mesmo com contexto e modelo reduzidos.

## Decisões técnicas aceitas

1. Linguagem do núcleo: Go.
2. Primeiro caso de uso: runtime epistemológico contínuo.
3. Integração principal de modelos: APIs OpenAI-compatible por adapter desacoplado.
4. Contrato universal do modelo: texto para texto; recursos modernos são opcionais.

## Decisões ainda abertas

1. Execução local: processo direto, contêiner opcional ou ambos.
2. Plataformas oficialmente suportadas e matriz de builds/testes.
3. Limite de contexto alvo para o primeiro benchmark.
4. Estratégia de indexação textual e semântica derivada.
