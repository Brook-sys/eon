# EON — Eternal Orchestration Node

<p align="center">
  <strong>Um runtime de orquestração durável e inspecionável para trabalho contínuo orientado por missão.</strong>
</p>

<p align="center">
  <a href="README.md">English</a> · Português (Brasil)
</p>

> [!IMPORTANT]
> O EON é um projeto experimental de pesquisa e aprendizado. Está em desenvolvimento ativo e ainda não é um produto final pronto para produção.

## Por que EON?

**EON** significa:

- **Eternal**
- **Orchestration**
- **Node**

O nome resume a ideia central do projeto: um nó projetado para preservar estado, coordenar trabalho limitado, sobreviver a interrupções e continuar avançando uma missão definida pelo operador ao longo de sucessivos ciclos de execução.

“Eternal” significa que operação e evolução indefinidas são requisitos de primeira classe do runtime — não ação descontrolada nem busy loop. Enquanto uma missão estiver ativa, o EON não possui conclusão global implícita: deve preservar estado, continuar gerando trabalho legítimo e limitado, aprender com resultados verificados, melhorar seu harness sob controles versionados e sobreviver a esperas, falhas, reinícios e limites de recursos. Somente pausa ou cancelamento autorizado, ou uma condição terminal explícita da missão, pode encerrar esse ciclo de vida.

## O que é o EON?

O EON é um runtime em Go para estudar **autonomia durável e supervisionada sob restrições operacionais reais**. Ele coordena missões, investigações, operações, evidências, chamadas de modelos, retries, esperas, aprovações, budgets e recuperação por meio de contratos determinísticos e estado persistente.

O projeto investiga uma pergunta prática:

> Quanto progresso confiável e contínuo um sistema determinístico de orquestração consegue extrair de modelos de linguagem pequenos, antigos, baratos, limitados por cota ou restritos de outras formas?

O EON é e continuará sendo um **motor epistemológico experimental**. Sua direção de longo prazo não é trocar essa identidade por uma arquitetura genérica de agente, mas ampliar progressivamente as ferramentas, utilidades e ações autorizadas conectadas ao núcleo epistemológico. Com capabilities validadas suficientes, ele deverá conseguir desempenhar funções úteis comparáveis às de sistemas de agentes como OpenClaw ou Hermes, ainda fundamentando o trabalho em conhecimento persistente, evidência, verificação e aprendizado contínuo. Seu diferencial experimental é alcançar essa amplitude prática e autonomia crescente com modelos de linguagem fracos: a competência deve vir do sistema de orquestração — memória persistente, decomposição, verificação, ferramentas tipadas, recuperação, avaliação e evolução controlada do harness — e não da suposição de um modelo poderoso.

Em vez de tratar um LLM como um agente todo-poderoso, o EON trata o modelo como um resolvedor textual limitado. O kernel mantém autoridade sobre agendamento, transições de estado, capabilities, budgets, validação e commits.

## Estado do projeto

O EON já contém um runtime experimental extenso em Go, suites de testes determinísticos e de durabilidade, plano de controle e dashboard do operador, adapters OpenAI-compatible, persistência SQLite, capabilities limitadas de web e arquivos, campanhas de avaliação de modelos, observabilidade e componentes experimentais distribuídos/de subagentes.

A arquitetura e os contratos ainda estão evoluindo. Interfaces, schemas, flags e formatos de armazenamento podem mudar enquanto a pesquisa converge.

## Objetivos centrais

- **Progresso contínuo orientado por missão:** manter uma fronteira renovável de trabalho enquanto a missão estiver ativa.
- **Evolução controlada perpétua:** converter continuamente evidência operacional verificada em melhorias versionadas, sem autoautorizar mudanças.
- **Utilidade agêntica progressiva:** ampliar o motor epistemológico experimental com ferramentas e utilidades explícitas e testadas até que desempenhe funções agênticas cada vez mais amplas.
- **Execução durável:** persistir trabalho aceito, esperas, retries, leases, recibos e condições de recuperação.
- **Autoridade determinística:** manter decisões e efeitos oficiais sob controle do kernel e das políticas.
- **Resiliência com modelos fracos:** operar por um contrato mínimo `texto → texto` e degradar com segurança.
- **Supervisão humana:** permitir observar, pausar, retomar, alterar, aprovar e auditar.
- **Uso limitado de recursos:** tornar explícitos chamadas, tokens, concorrência, retries, tempo e crescimento de filas.
- **Desenvolvimento baseado em evidência:** usar testes reproduzíveis e campanhas live controladas para encontrar falhas reais.
- **Portabilidade entre providers:** isolar diferenças dos dialetos OpenAI-compatible por adapters e perfis.
- **Recuperação de crashes e idempotência:** retomar sem perder intenção nem duplicar efeitos cegamente.
- **Caminhos decisórios inspecionáveis:** expor inputs, políticas, decisões, outputs, validadores e recibos — não cadeia de pensamento oculta.

## Não objetivos

O EON não pretende ser:

- a substituição de seu núcleo epistemológico por uma abstração genérica de agente, nem um agente de automação irrestrito e não supervisionado; a utilidade ampliada continua fundamentada em evidência e limitada pela missão, políticas, budgets e capabilities definidos pelo operador;
- um shell controlado diretamente pela saída do modelo;
- um sistema que inventa sua própria missão, autoridade ou objetivos econômicos;
- um wrapper dependente de tool calling nativo ou de um único provider;
- um busy loop que fabrica atividade sem valor para parecer autônomo;
- um substituto para autorização explícita, validação ou supervisão humana.

## Princípios de projeto

1. **O kernel é determinístico.** Modelos podem propor; código validado decide.
2. **O estado vive fora do contexto do modelo.** Reinício e recuperação não dependem de memória conversacional.
3. **Todo efeito atravessa uma fronteira tipada de capability.** Capabilities desconhecidas ou não autorizadas falham de modo fechado.
4. **Toda saída do modelo é não confiável.** O output bruto é preservado, normalizado, validado e convertido em proposta antes de afetar o estado canônico.
5. **A espera é local, não global.** Uma operação pode aguardar tempo, cota, evento ou aprovação enquanto trabalho independente continua.
6. **Budgets são entradas do scheduler.** Limites são modelados explicitamente, não tratados como exceções improvisadas.
7. **A agenda de curto prazo é renovável.** Concluir uma tarefa deve revelar, validar ou gerar o próximo incremento limitado.
8. **Recuperação é projetada, não presumida.** Leases, chaves de idempotência, event logs, reconciliação e relógios virtuais fazem parte da arquitetura.
9. **Recursos do modelo são otimizações opcionais.** JSON mode, schemas, tools, streaming e contextos maiores devem preservar o protocolo textual básico e o fallback seguro.
10. **A observabilidade deve explicar o comportamento oficial.** Registros de auditoria mostram o que o sistema usou e decidiu sem exigir raciocínio privado do modelo.

## Modelo de execução

Um ciclo simplificado de operação é:

```text
observar → selecionar → preparar → agir → verificar → commitar → repetir
```

O loop mais amplo do runtime é:

```text
recuperar
  → observar estado persistido, capacidade e tempo
  → ingerir eventos externos opcionais
  → reabastecer e priorizar a agenda
  → despachar operações limitadas
  → validar resultados
  → commitar mudanças aceitas
  → atualizar a fronteira de trabalho
  → continuar
```

Estados persistidos típicos de uma operação incluem:

```text
NEW → READY → RUNNING → VERIFYING → SUCCEEDED
              │             │
              ├─ WAITING_TIME
              ├─ WAITING_EVENT
              ├─ WAITING_APPROVAL
              ├─ THROTTLED
              ├─ BLOCKED_DEPENDENCY
              └─ REPLANNING / EXHAUSTED / FAILED / CANCELLED
```

Uma fila executável vazia não é automaticamente interpretada como conclusão da missão. O EON aplica estratégias limitadas de replenishment ou emite um diagnóstico explícito `CONTINUITY_BLOCKED`.

## Arquitetura

```text
┌──────────────────────────────────────────────────────────────┐
│ Interfaces: CLI, Control API, dashboard e adapters de canal  │
├──────────────────────────────────────────────────────────────┤
│ Plano de controle: missões, políticas, comandos e aprovação  │
├──────────────────────────────────────────────────────────────┤
│ Kernel: supervisor, scheduler, transições e recuperação      │
├──────────────────────────────────────────────────────────────┤
│ Agenda: frontier, admissão, replenishment e prioridade       │
├──────────────────────────────────────────────────────────────┤
│ Estado epistêmico: fontes, observações, claims e evidências  │
├──────────────────────────────────────────────────────────────┤
│ Cognição: specs, prompts, modelos e validação                 │
├──────────────────────────────────────────────────────────────┤
│ Capabilities: modelo, web, arquivo, tools, clock e telemetria│
├──────────────────────────────────────────────────────────────┤
│ Persistência: estado canônico, event log, outbox e artifacts │
└──────────────────────────────────────────────────────────────┘
```

### Pacotes principais

```text
cmd/runtime/                    processo principal e CLI
internal/domain/                contratos de domínio e transições puras
internal/kernel/                scheduler, execução, recuperação e admissão
internal/agenda/                bootstrap da frontier e lógica da agenda
internal/mission/               carga, revisão e alteração de missão
internal/provider/openai/       adapter de modelos OpenAI-compatible
internal/prompt/                compilação de prompts sob budget
internal/storage/memory/        store determinístico em memória
internal/storage/sqlite/        backend durável canônico do MVP
internal/control/               comandos do operador e Control API
internal/dashboard/             dashboard experimental do operador
internal/observability/         auditoria, métricas e tracing
internal/evaluation/            campanhas offline e live de modelos
internal/network/               componentes experimentais de rede
internal/tool/                  executores limitados de tools
```

## Integração com modelos

O contrato cognitivo universal do EON é intencionalmente pequeno:

```text
texto de entrada → texto de saída
```

O adapter principal atende APIs OpenAI-compatible, mas compatibilidade é tratada como perfil, não como booleano. Deployments podem divergir em endpoints, roles, campos de tokens, formatos de erro, streaming, saída estruturada, tool calling, limites de contexto e relatório de usage.

Capabilities opcionais são adotadas progressivamente:

```text
JSON Schema nativo
  → JSON mode
  → campos delimitados
  → resposta de token fechado
  → texto curto com parser
  → fallback determinístico ou humano
```

A falha de uma otimização não pode ampliar silenciosamente a autoridade do modelo, perder a operação ou duplicar um efeito externo.

## Persistência e recuperação

O backend canônico do MVP é **SQLite com event log e commits lógicos do domínio**. A escolha ocorreu após um spike comparativo de armazenamento e campanhas de crash contra Dolt. Os contratos do domínio continuam independentes do backend, mas SQLite é atualmente o caminho operacional aceito.

O modelo de persistência enfatiza:

- mudanças canônicas atômicas;
- eventos append-only para auditoria;
- tratamento idempotente de intenção e resultado;
- leases e condições de retomada persistidos;
- reconciliação explícita de efeitos ambíguos;
- filas e esperas recuperáveis após reinício;
- procedimentos verificados de backup e restauração.

Consulte a [ADR-0003](ADRS/0003-versioned-storage.md) e o [runbook de backup SQLite](RUNBOOKS/sqlite-backup.md).

## Plano de controle e supervisão do operador

O dashboard e a Control API são clientes do runtime, não o runtime em si. Eles oferecem uma interface supervisionada para:

- inspecionar missões, inquiries, operações, eventos, budgets e falhas;
- observar a execução atual e histórica;
- submeter comandos tipados e idempotentes do operador;
- pausar, retomar, cancelar ou alterar missões;
- responder perguntas correlacionadas do operador;
- revisar chamadas de modelos, validação, changesets e commits;
- acompanhar atualizações live sem escrever diretamente no armazenamento canônico.

Fechar o dashboard não pode interromper uma missão ativa. Consulte [CONTROL_PLANE.md](CONTROL_PLANE.md).

## Modelo de segurança e autoridade

O EON segue um fluxo de proposta antes do efeito:

```text
modelo ou entrada externa
  → artifact não confiável
  → parsing e normalização
  → proposta tipada
  → validação determinística
  → autorização por política e capability
  → commit atômico ou rejeição explícita
```

Fronteiras importantes:

- a saída do modelo não altera diretamente o estado canônico;
- a saída do modelo não concede capabilities a si própria;
- segredos não devem aparecer em prompts, commits ou corpos de diagnóstico;
- conteúdo externo hostil é tratado como dado, não como instrução privilegiada;
- execução de shell fica desabilitada salvo ativação explícita;
- acesso a arquivos fica confinado às raízes configuradas;
- retries são limitados e efeitos ambíguos exigem reconciliação;
- o operador permanece a fonte de autoridade para missão e políticas.

## Requisitos

- **Go 1.24 ou mais recente**, conforme `go.mod`.
- Um compilador C não é necessário para o adapter SQLite canônico em Go puro.
- Credenciais de provider são necessárias apenas para campanhas live ou operações do runtime apoiadas por modelo.
- Infraestrutura opcional pode ser necessária para SearXNG, exportação OTLP, Telegram ou cenários P2P experimentais.

## Início rápido

### Forma mais rápida: container pronto

A branch `main` publica imagens multi-arquitetura (`linux/amd64` e
`linux/arm64`) no GHCR. Por padrão, o container usa SQLite persistente, roda
como usuário sem privilégios, expõe o dashboard na porta 8080 e inclui
healthcheck:

```bash
docker volume create eon-data
docker run --name eon --restart unless-stopped \
  -p 8080:8080 \
  -v eon-data:/data \
  ghcr.io/brook-sys/eon:latest
```

Abra <http://localhost:8080/>. Com Compose:

```bash
docker compose up -d
docker compose ps
```

Segredos de providers continuam em variáveis de ambiente e nunca entram na
imagem. Para habilitar um modelo, consulte o exemplo equivalente no
[README em inglês](README.md#fastest-start-prebuilt-container) e ajuste o
endpoint, modelo e nome da variável de credencial.

Para deployments reproduzíveis, fixe uma tag de release ou `sha-...` em vez
de `latest`. O pacote pode nascer privado conforme a visibilidade configurada
no GHCR; torne-o público nas configurações do pacote para permitir pulls
anônimos.

### 1. Clone e entre no repositório

```bash
git clone https://github.com/Brook-sys/eon.git
cd eon
```

A branch de desenvolvimento ativa pode variar enquanto o projeto estiver em fase experimental. Consulte as branches disponíveis caso a branch padrão ainda não tenha sido configurada.

### 2. Execute a suite de testes

```bash
go test ./...
```

Verificações adicionais usadas com frequência no projeto:

```bash
go test -race ./...
go vet ./...
gofmt -l .
```

Algumas campanhas longas, live, de crash ou específicas de provider são deliberadamente controladas por variáveis de ambiente ou comandos explícitos e não integram a suite offline padrão.

### 3. Inicie um runtime local em memória

```bash
go run ./cmd/runtime \
  -store=memory \
  -listen=127.0.0.1:8080
```

O dashboard experimental é ativado por padrão. Abra:

```text
http://127.0.0.1:8080/
```

Isso inicia o processo e as superfícies de controle, mas uma execução autônoma útil ainda exige uma missão instalada e as capabilities necessárias para essa missão.

### 4. Inicie com armazenamento SQLite durável

```bash
mkdir -p .local

go run ./cmd/runtime \
  -store=sqlite \
  -sqlite-path=.local/eon.db \
  -listen=127.0.0.1:8080
```

Não copie somente o arquivo SQLite principal enquanto o runtime estiver escrevendo. Use o procedimento verificado de backup documentado em [RUNBOOKS/sqlite-backup.md](RUNBOOKS/sqlite-backup.md).

### 5. Ative um provider de modelo OpenAI-compatible

Mantenha credenciais em variáveis de ambiente, nunca em arquivos commitados:

```bash
export EON_MODEL_API_KEY='substitua-aqui'

go run ./cmd/runtime \
  -store=sqlite \
  -sqlite-path=.local/eon.db \
  -model \
  -model-base-url=https://seu-provider.example/v1 \
  -model-name=seu-modelo \
  -model-api-key-env=EON_MODEL_API_KEY \
  -model-context-tokens=8000 \
  -model-max-output-field=max_tokens
```

O comportamento do provider e os campos suportados variam. Leia [OPENAI_COMPATIBILITY.md](OPENAI_COMPATIBILITY.md) antes de adicionar um deployment.

Para inspecionar todas as flags do runtime:

```bash
go run ./cmd/runtime -help
```

## Filosofia de testes

O EON é desenvolvido por duas formas complementares de evidência.

### Verificação determinística

- testes unitários e orientados por tabela;
- suites reutilizáveis de contratos de armazenamento;
- matrizes de crash e restart;
- relógios virtuais e fontes determinísticas de aleatoriedade;
- fuzzing e corpus de outputs malformados;
- race detector e análise estática;
- providers falsos limitados e adapters de replay;
- verificações de arquitetura e dependências.

### “Testes de fogo” live e controlados

Campanhas live exercitam providers reais — especialmente Groq e NVIDIA NIM — com hipóteses explícitas e limites rígidos. Elas medem mais do que o simples sucesso HTTP de uma chamada. São analisados:

- correção sintática e semântica;
- aderência às instruções e ao contrato de output;
- latência e uso de tokens;
- truncamento e framing malformado;
- comportamento de `429` e `Retry-After`;
- timeout, retry, fallback e recuperação;
- pressão de filas, concorrência e consumo de recursos;
- comportamento entre portes, famílias, formatos e janelas de contexto.

Resultados são evidência, não autoridade. Uma resposta do modelo nunca altera por si só o estado oficial ou a preferência de modelo; os achados precisam ser interpretados, reproduzidos e convertidos em mudanças validadas de código, política, prompt ou observabilidade.

## Mapa da documentação

- [ARCHITECTURE.md](ARCHITECTURE.md) — tese arquitetural, camadas e modelo do runtime.
- [REQUIREMENTS.md](REQUIREMENTS.md) — requisitos funcionais e não funcionais normativos.
- [TECHNICAL_REQUIREMENTS.md](TECHNICAL_REQUIREMENTS.md) — restrições técnicas aceitas e fronteiras de módulos.
- [INVARIANTS.md](INVARIANTS.md) — invariantes de autoridade, continuidade, segurança e progresso.
- [FAILURE_TAXONOMY.md](FAILURE_TAXONOMY.md) — falhas normalizadas e disposições de recuperação.
- [GLOSSARY.md](GLOSSARY.md) — vocabulário normativo do projeto.
- [CONTROL_PLANE.md](CONTROL_PLANE.md) — arquitetura da API e do dashboard do operador.
- [CONTINUOUS_WORK.md](CONTINUOUS_WORK.md) — famílias legítimas de trabalho contínuo e política anti-busywork.
- [WEAK_MODEL_PROTOCOL.md](WEAK_MODEL_PROTOCOL.md) — protocolo para modelos restritos e microturnos persistentes.
- [OPENAI_COMPATIBILITY.md](OPENAI_COMPATIBILITY.md) — subconjunto portátil OpenAI-compatible e perfis de provider.
- [PROVIDER_INTEGRATION_GROQ_NVIDIA.md](PROVIDER_INTEGRATION_GROQ_NVIDIA.md) — integração live e orientação de avaliação.
- [CONTINUOUS_DEVELOPMENT.md](CONTINUOUS_DEVELOPMENT.md) — registro ativo de desenvolvimento e backlog.
- [ADRS](ADRS/) — decisões arquiteturais aceitas.
- [RUNBOOKS](RUNBOOKS/) — procedimentos operacionais.

## Decisões arquiteturais atuais

- [ADR-0001](ADRS/0001-go-core.md): Go é a linguagem de implementação do núcleo.
- [ADR-0002](ADRS/0002-openai-compatible-provider.md): a integração principal de modelos é um adapter OpenAI-compatible isolado.
- [ADR-0003](ADRS/0003-versioned-storage.md): SQLite com event log é o backend canônico do MVP.

## Contribuindo

O projeto valoriza mudanças pequenas o suficiente para serem verificadas e substanciais o bastante para produzirem progresso observável. Uma contribuição deve normalmente incluir:

1. um objetivo claramente delimitado;
2. o requisito ou invariante relevante;
3. implementação e testes proporcionais;
4. evidência do comportamento de falha e recuperação;
5. atualização da documentação quando contratos mudarem;
6. `git diff --check` e os comandos Go aplicáveis.

Para mudanças de modelo, prompt, parsing, roteamento, cotas ou recuperação, inclua evidência live controlada quando houver credenciais e acesso ao provider. Nunca faça commit de chaves de API, segredos de providers, prompts privados brutos ou artifacts sensíveis.

## Licença

Atualmente não existe arquivo de licença no repositório. Até que uma licença seja adicionada, não se deve presumir licença open source nem autorização de redistribuição.

---

**Eternal. Orchestration. Node.**

Persistente o suficiente para continuar, limitado o suficiente para permanecer sob controle.
