# Motor Autônomo — Arquitetura inicial

Status: rascunho v0.2

## Tese

A inteligência principal deve estar no sistema, não depender exclusivamente do modelo.
O motor deve continuar útil com modelos pequenos, antigos, gratuitos e com janelas de contexto reduzidas.

Seu propósito principal é **continuidade operacional**: permanecer vivo, preservar estado e voltar a avançar assim que houver trabalho, capacidade e permissão. O motor é contínuo; objetivos e operações podem ser finitos.

Continuidade não significa loop ocupado nem chamadas incessantes ao modelo. Significa que espera, rate limit, callback pendente, reinício, indisponibilidade e ausência de trabalho são estados normais e duráveis do runtime.

## Princípios

1. Núcleo determinístico; IA usada apenas onde ambiguidade e julgamento são necessários.
2. Estado persistente fora do contexto do modelo.
3. Contexto montado sob demanda e limitado por orçamento.
4. Toda ação produz evidência verificável.
5. Componentes substituíveis por contratos estáveis.
6. Reinício e retomada sem perda do progresso.
7. Falha fechada para ações sem permissão ou sem validação.
8. Um modelo fraco pode exigir mais passos, mas não deve romper o protocolo.
9. Esperar é trabalho válido: o motor deve dormir sem perder estado e acordar por evento ou prazo.
10. Limites são entradas do scheduler, não exceções improvisadas.
11. Toda operação com efeito colateral deve ser idempotente ou possuir chave de deduplicação.
12. Nenhuma dependência externa, incluindo o modelo, controla a continuidade do kernel.
13. Conversas entre agentes não fazem parte do núcleo; coordenação ocorre por estado, eventos e contratos.

## Visão em camadas

```text
┌──────────────────────────────────────────────────────────┐
│ Interfaces: CLI, API, UI, eventos                        │
├──────────────────────────────────────────────────────────┤
│ Control Plane: objetivos, políticas, orçamento, aprovação│
├──────────────────────────────────────────────────────────┤
│ Kernel: supervisor, scheduler, eventos, espera, retomada │
├──────────────────────────────────────────────────────────┤
│ Cognição: planner, selector, critic, context compiler    │
├──────────────────────────────────────────────────────────┤
│ Capacidades: tools, skills, workers, modelos             │
├──────────────────────────────────────────────────────────┤
│ Persistência: estado, artefatos, memória, logs, métricas │
└──────────────────────────────────────────────────────────┘
```

## Kernel mínimo

O kernel não interpreta linguagem natural. Ele somente opera estados e comandos válidos.

Estados iniciais de uma unidade de trabalho:

- `NEW`: objetivo recebido, ainda não normalizado.
- `READY`: há uma próxima unidade de trabalho executável.
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
recover → ingest events → release due waits → schedule → dispatch
        → persist → calculate next wake-up → sleep → recover
```

Se não houver trabalho executável, o kernel calcula o próximo prazo e dorme. Ele deve poder ser acordado antecipadamente por callback ou evento. Portanto, seu comportamento contínuo é **event-driven**, não polling agressivo.

## Invariante de continuidade

Enquanto não houver uma ordem explícita de desligamento ou falha fatal do armazenamento:

1. todo trabalho aceito permanece representado no estado persistente;
2. todo trabalho não terminal possui uma condição explícita para voltar a ser avaliado;
3. após reinício, o runtime reconstrói filas, esperas, leases e callbacks pendentes;
4. nenhuma resposta de modelo é necessária para o motor saber como retomar;
5. rate limits e dependências indisponíveis adiam trabalho, mas não apagam intenção;
6. o motor pode permanecer indefinidamente em repouso com consumo mínimo.

Isso diferencia:

- **continuidade do motor**: potencialmente indefinida;
- **continuidade de um objetivo**: até conclusão, cancelamento ou intervenção;
- **continuidade de uma tentativa**: curta, limitada por timeout e lease.

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
tool:shell
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

O mesmo modelo atende mensagens, webhooks, timers, conclusão de subprocessos, alterações de arquivos e respostas humanas.

## Entrega e efeitos colaterais

Não é realista prometer execução exatamente uma vez em todos os sistemas externos. O alvo deve ser:

```text
at-least-once delivery + idempotência + deduplicação
```

Antes de executar um efeito externo, o motor cria um registro de intenção com uma `idempotency_key`. Depois registra recibo e evidência. Em reinícios, consulta esse registro antes de repetir a ação.

Para emissão confiável de eventos, usar o padrão transactional outbox: a mudança de estado e o evento a publicar são confirmados na mesma transação local.

## Contratos fundamentais

### Goal

```json
{
  "id": "goal_...",
  "objective": "resultado desejado",
  "success_criteria": ["critério observável"],
  "constraints": ["limite ou proibição"],
  "budget": {"steps": 50, "tokens": 20000, "time_seconds": 1800},
  "status": "READY"
}
```

### WorkItem

```json
{
  "id": "work_...",
  "goal_id": "goal_...",
  "intent": "uma ação pequena",
  "required_context": ["fact_...", "artifact_..."],
  "expected_evidence": ["arquivo existe", "teste passa"],
  "attempt": 1,
  "status": "READY"
}
```

### Decision

```json
{
  "type": "invoke_capability",
  "capability": "filesystem.read",
  "arguments": {"path": "README.md"},
  "reason_code": "NEED_CURRENT_STATE",
  "confidence": 0.72
}
```

### Evidence

```json
{
  "work_id": "work_...",
  "kind": "test_result",
  "source": "pytest",
  "passed": true,
  "artifact_ref": "artifact_..."
}
```

## Módulos e portas

### 1. ModelProvider

Responsabilidade: completar uma solicitação estruturada.

```text
complete(request, budget) -> ModelResponse
capabilities() -> {json_mode, tool_calls, context_limit, ...}
```

Adaptadores possíveis: Ollama, llama.cpp, APIs compatíveis com OpenAI e provedores gratuitos.

### 2. Planner

Converte um objetivo ou falha em pequenos `WorkItem`s. Pode haver implementações:

- `RulePlanner`: regras determinísticas para fluxos conhecidos.
- `LLMPlanner`: decomposição via modelo.
- `HybridPlanner`: templates primeiro, modelo apenas para lacunas.

### 3. Selector

Escolhe o próximo item pronto com base em dependências, prioridade, custo e risco.

### 4. ContextCompiler

Monta um pacote mínimo para uma chamada. Pipeline sugerido:

```text
necessidades do WorkItem
  → busca de fatos/artefatos
  → ranking por relevância
  → deduplicação
  → compressão
  → corte pelo orçamento
  → envelope final
```

O contexto deve conter identidade da tarefa, critérios, fatos confirmados e formato de saída; não a conversa inteira.

### 5. CapabilityRegistry

Registro de capacidades instaláveis. Cada capability declara:

- nome e versão;
- schema de entrada e saída;
- efeitos colaterais;
- nível de risco;
- permissões necessárias;
- timeout e política de repetição;
- função opcional de verificação.

Exemplos: `filesystem.read`, `shell.run`, `web.search`, `http.fetch`, `code.test`.

### 6. Executor

Executa uma capability após autorização, aplica timeout e captura saída, erro e artefatos.

### 7. Verifier

Não pergunta apenas ao modelo se algo funcionou. Ordem de preferência:

1. verificador determinístico;
2. invariantes e schemas;
3. comparação com exemplos ou testes;
4. verificação por modelo independente;
5. revisão humana.

### 8. MemoryStore

Separar quatro tipos:

- `WorkingState`: estado exato do trabalho em curso;
- `Facts`: fatos confirmados com origem e confiança;
- `Episodes`: histórico resumido de tentativas e resultados;
- `Artifacts`: arquivos e saídas grandes referenciados por ID.

A primeira versão pode usar SQLite + diretório de artefatos, sem banco vetorial obrigatório.

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
- `planner`
- `context_source`
- `capability`
- `verifier`
- `memory_backend`
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

1. Saídas pequenas, tipadas e validadas.
2. Uma decisão cognitiva por chamada.
3. Vocabulário de ações limitado por estado.
4. Reparo automático de JSON antes de repetir toda a chamada.
5. Exemplos curtos específicos para a operação atual.
6. Contexto de fatos em vez de transcrições.
7. Planejamento progressivo: detalhar somente os próximos passos.
8. Verificadores externos ao modelo.
9. Fallback entre modelos por competência, não apenas por disponibilidade.
10. Possibilidade de votação ou crítica somente em decisões de alto impacto.

## Unidade de modularidade

A unidade principal não deve ser “um agente com personalidade”, mas uma operação:

```text
Operation = Contract + Context Policy + Decision Strategy + Capability + Verifier
```

Isso permite trocar o modelo, executor ou verificador sem reescrever o fluxo inteiro.

## MVP recomendado

Escopo: tarefas locais de programação em um repositório controlado.

Inclui:

- kernel serial e persistente;
- SQLite;
- um adaptador de modelo compatível com OpenAI;
- `HybridPlanner` simples;
- compilador de contexto por regras;
- capabilities de leitura, escrita, patch e testes;
- política de diretório permitido;
- verificação por testes e inspeção de arquivos;
- CLI para iniciar, inspecionar, pausar e retomar trabalhos;
- log estruturado em JSONL.

Não inclui inicialmente:

- múltiplos agentes conversando livremente;
- banco vetorial obrigatório;
- execução distribuída;
- interface visual complexa;
- geração dinâmica irrestrita de ferramentas;
- autonomia sem orçamento ou limites.

## Métrica central

Comparar o mesmo modelo com e sem o harness:

- taxa de conclusão;
- passos e tokens por tarefa;
- erros não detectados;
- recuperação após interrupção;
- repetição/loops;
- qualidade da evidência final;
- desempenho ao reduzir artificialmente a janela de contexto.

A promessa será demonstrada se o sistema mantiver boa taxa de conclusão mesmo com contexto e modelo reduzidos.

## Decisões ainda abertas

1. Linguagem do núcleo: Python, TypeScript ou outra.
2. Primeiro caso de uso: programação, pesquisa, automação pessoal ou generalista.
3. Execução local: processo direto, contêiner ou sandbox.
4. Grau de portabilidade entre Linux, Windows e macOS.
5. Limite de contexto alvo para o primeiro benchmark.
