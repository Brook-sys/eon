# Requisitos Técnicos do Runtime Epistemológico

Status: baseline v0.1

## 1. Decisões firmes

### TR-001 — Linguagem do núcleo: Go

O runtime será implementado em **Go**.

Motivações de engenharia:

- binário estático ou quase autocontido;
- boa portabilidade entre Linux, macOS e Windows;
- consumo previsível de recursos;
- concorrência e cancelamento bem suportados por goroutines e `context.Context`;
- biblioteca padrão forte para HTTP, serialização, testes e observabilidade;
- implantação simples em máquinas modestas;
- tipagem estática adequada para contratos do kernel.

Go não garante automaticamente baixo consumo, tolerância a falhas ou portabilidade. Essas propriedades precisam de budgets, limites de concorrência, builds reproduzíveis, testes em plataformas-alvo e medição.

### TR-002 — Compatibilidade de modelos: APIs OpenAI-compatible

O adaptador principal de modelos implementará APIs **OpenAI-compatible**, sem depender de um fornecedor ou modelo específico.

O contrato interno do runtime continuará sendo mais simples e estável que qualquer API externa:

```go
type ModelProvider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error)
    Probe(ctx context.Context) (ProviderProfile, error)
}
```

O runtime não deve espalhar tipos da API OpenAI pelos módulos de planejamento, conhecimento ou kernel. O provider converte entre o contrato interno e o dialeto remoto.

### TR-003 — Contrato cognitivo mínimo

O único requisito universal para um modelo é:

```text
texto de entrada → texto de saída
```

Recursos como mensagens com papéis, streaming, JSON mode, JSON Schema, tool calling, logprobs, seed, imagens e Responses API são capacidades opcionais descobertas e habilitadas por perfil.

### TR-004 — Backend de versionamento desacoplado

Dolt é o candidato líder para o estado epistemológico versionado, mas a seleção depende de spike e benchmark. O domínio dependerá de interfaces próprias, não de chamadas Dolt distribuídas pelo código.

### TR-005 — Testabilidade como requisito arquitetural

Todo componente determinístico deve ser testável sem rede, relógio real, modelo remoto ou processo externo.

## 2. Compatibilidade OpenAI não é binária

“OpenAI-compatible” descreve uma família de dialetos parcialmente compatíveis. Providers podem divergir em:

- `/v1/chat/completions` versus `/v1/responses`;
- suporte a `system` ou `developer` messages;
- nomes e semântica de parâmetros;
- `max_tokens` versus `max_completion_tokens`;
- formato de erros;
- SSE e chunks de streaming;
- contagem de usage;
- tool calls e chamadas paralelas;
- JSON mode e JSON Schema;
- modelos listáveis por `/v1/models`;
- campos aceitos, ignorados ou rejeitados;
- autenticação e headers adicionais;
- limites de contexto, output e requisições;
- tratamento de `temperature`, `seed`, `stop` e logprobs.

Portanto, o provider precisa de **perfil de compatibilidade**, não apenas `base_url` e chave.

## 3. ProviderProfile

```go
type ProviderProfile struct {
    Name                  string
    BaseURL               string
    APIStyle              APIStyle // chat_completions, responses, auto
    SupportsSystemRole    bool
    SupportsStreaming     bool
    SupportsStreamUsage   bool
    SupportsJSONMode      bool
    SupportsJSONSchema    bool
    SupportsTools         bool
    SupportsParallelTools bool
    SupportsSeed          bool
    SupportsLogprobs      bool
    MaxContextTokens      int
    MaxOutputTokens       int
    RequestTimeout        time.Duration
    QuirkSet              []string
}
```

Capacidades podem ser:

1. declaradas em configuração;
2. detectadas por probes seguros;
3. aprendidas por falhas observadas;
4. sobrescritas pelo operador.

O resultado é persistido com versão e data. Detecção nunca deve consumir orçamento indefinidamente.

## 4. Requisição interna normalizada

```go
type CompletionRequest struct {
    OperationID     string
    Model           string
    Prompt          string
    System          string
    MaxOutputTokens int
    Temperature     *float64
    Stop            []string
    OutputContract  OutputContract
    IdempotencyKey  string
    Metadata        map[string]string
}
```

O MVP pode serializar tudo em um prompt textual único. Separação de papéis e recursos nativos são otimizações do adaptador, desde que não alterem a semântica do `OperationSpec`.

## 5. Política de degradação

```text
JSON Schema nativo
→ JSON mode
→ campos delimitados
→ token fechado
→ texto curto + parser
→ fallback determinístico/humano
```

O runtime escolhe o formato conforme:

- exigência da operação;
- perfil declarado;
- confiabilidade empírica por modelo e formato;
- custo de falha;
- possibilidade de validação externa.

Tool calling nativo nunca é obrigatório. O `CapabilityBinder` continua sendo a autoridade.

## 6. Configuração mínima de provider

```yaml
providers:
  local:
    kind: openai-compatible
    base_url: http://127.0.0.1:11434/v1
    api_style: chat_completions
    auth:
      type: bearer
      env: LOCAL_LLM_API_KEY
    defaults:
      model: small-model
      timeout: 60s
    capabilities:
      system_role: true
      streaming: false
      json_schema: false
      tools: false
```

Segredos nunca são gravados em commits epistemológicos, logs de prompts ou mensagens de erro.

## 7. Módulos Go iniciais

```text
cmd/runtime/                 processo principal e CLI
internal/kernel/             supervisor, scheduler e transições
internal/mission/            MissionSpec e revisões
internal/agenda/             frontier, admissão e prioridade
internal/knowledge/          claims, evidências, perguntas e operações
internal/model/              contrato interno de modelo
internal/model/openai/       dialetos OpenAI-compatible
internal/prompt/             compilação de OperationSpec
internal/validation/         schemas, invariantes e verificadores
internal/storage/            interfaces, transações e migrations
internal/storage/memory/     fake determinístico para testes
internal/storage/dolt/       candidato, condicionado ao spike
internal/events/             event log e outbox
internal/resources/          rate limits, budgets e circuit breakers
internal/clock/              relógio real e virtual
internal/observability/      logs, métricas e traces
pkg/contracts/               somente contratos públicos estáveis, se necessários
testdata/                    fixtures e golden files
```

Regra: `internal/knowledge` não importa `internal/storage/dolt` nem `internal/model/openai`.

## 8. Modularidade verificável

Um módulo é substituível quando:

- possui interface pequena e orientada ao domínio;
- não vaza tipos de fornecedor;
- tem suite de contract tests reutilizável;
- suas falhas são classificadas em erros do domínio;
- sua configuração é validada;
- pode ser substituído por fake em testes;
- mudanças incompatíveis são versionadas.

Interfaces não devem ser criadas para cada struct “por modularidade”. Elas existem nas fronteiras que realmente precisam de substituição, isolamento ou teste.

## 9. Continuidade verificável

O runtime satisfaz continuidade quando:

1. trabalho aceito sobrevive a reinício;
2. esperas possuem `not_before` ou condição persistida;
3. leases expirados são recuperados;
4. operações interrompidas são repetidas com idempotência ou reconciliadas;
5. uma agenda vazia dispara replenishment antes do repouso;
6. repouso não consome CPU em busy loop;
7. eventos antecipam reavaliação, mas não são necessários para manter a missão ativa.

## 10. Tolerância a falhas verificável

Classes mínimas:

- timeout e cancelamento;
- 429 com ou sem `Retry-After`;
- 5xx transitório;
- 4xx permanente;
- resposta truncada ou inválida;
- indisponibilidade do banco;
- crash antes/depois de efeito externo;
- lease órfão;
- evento duplicado;
- outbox parcialmente entregue;
- corrupção ou incompatibilidade de schema;
- fonte alterada durante ingestão;
- índice derivado ausente ou obsoleto.

Controles:

- timeouts explícitos;
- backoff exponencial com jitter e teto;
- circuit breaker;
- budgets de retry;
- idempotency keys;
- at-least-once + deduplicação;
- transactional outbox;
- checksums e invariantes;
- recuperação determinística;
- dead-letter/quarantine para itens não reconciliáveis.

## 11. Estratégia de testes

### 11.1 Testes unitários

- parsers e normalizadores;
- máquinas de estado;
- cálculo de prioridade;
- políticas de admissão;
- budgets e rate limits;
- compilação de prompts;
- validação de ChangeSets;
- diffs de missão;
- propagação de impacto epistemológico.

### 11.2 Property-based e fuzzing

- parsers nunca entram em panic com saída arbitrária;
- transições inválidas nunca são aceitas;
- normalização é idempotente;
- deduplicação não cria segundo efeito;
- serialização possui round-trip;
- merge não perde objetos sem registro explícito;
- prompt compiler respeita budgets.

Usar o fuzzing nativo de Go onde adequado.

### 11.3 Contract tests

A mesma suite deve ser executada contra:

- todo `ModelProvider`;
- todo backend de armazenamento;
- todo capability adapter;
- todo verificador da mesma família.

### 11.4 Testes de integração

- servidor HTTP OpenAI-compatible falso;
- cenários de streaming fragmentado;
- respostas e erros divergentes por provider;
- banco real em container/processo isolado;
- migrations;
- commit, branch, diff e merge;
- outbox e recuperação após crash.

### 11.5 Testes de sistema

Executar uma missão completa com:

- web e modelo simulados;
- relógio virtual;
- falhas injetadas;
- reinícios em pontos definidos;
- comparação do estado final com golden state.

### 11.6 Testes de longevidade

- milhares de ciclos;
- quotas pequenas;
- eventos duplicados e fora de ordem;
- dependências flapping;
- crescimento do histórico;
- detecção de leaks de goroutines, memória e conexões;
- ausência de atividade sem progresso.

### 11.7 Avaliação cognitiva

Separada dos testes determinísticos:

- corpus versionado de operações;
- modelos e parâmetros registrados;
- métricas sintáticas e semânticas distintas;
- seeds quando suportadas;
- múltiplas execuções;
- regressões por modelo/perfil;
- avaliação com janelas artificiais de 2k, 4k e 8k.

## 12. Determinismo de teste

Fronteiras injetáveis obrigatórias:

```go
type Clock interface { Now() time.Time }
type IDGenerator interface { New() string }
type RandomSource interface { Int63n(n int64) int64 }
type ModelProvider interface { /* ... */ }
type Store interface { /* ... */ }
type SourceFetcher interface { /* ... */ }
```

Retries, jitter, deadlines e agendamentos devem poder usar relógio e aleatoriedade controlados.

## 13. CI e qualidade

Baseline:

```text
go test ./...
go test -race ./...
go vet ./...
gofmt check
staticcheck ./...
```

Além disso:

- cobertura por componente, sem meta global enganosa;
- builds para plataformas-alvo;
- SBOM e scanning de dependências;
- testes de migrations em upgrade e downgrade quando suportado;
- benchmarks para caminhos críticos;
- fixtures sem segredos;
- nenhuma chamada de rede em testes unitários.

## 14. Budgets operacionais

Configuração explícita para:

- máximo de goroutines de workers;
- conexões HTTP e de banco;
- tamanho de resposta;
- bytes por fonte;
- tokens por operação;
- operações simultâneas por provider;
- requests por período;
- retries por item e por missão;
- tamanho de agenda e frontier;
- histórico retido e compactação;
- tempo máximo por ciclo;
- memória e disco monitorados.

## 15. Critérios técnicos do primeiro vertical slice

O primeiro slice estará pronto quando puder:

1. iniciar de um binário Go;
2. carregar e validar uma missão;
3. persistir uma pergunta e uma operação;
4. chamar um servidor OpenAI-compatible falso usando somente texto;
5. validar a resposta e criar `ProposedChangeSet`;
6. aplicar o changeset atomicamente;
7. registrar evento e evidência;
8. sofrer crash simulado e retomar sem duplicar o efeito;
9. entrar em repouso sem busy loop;
10. passar unitários, integração, race detector e teste de sistema determinístico.

Pesquisa web real, Dolt e modelos reais entram depois que esse caminho for correto com fakes.
