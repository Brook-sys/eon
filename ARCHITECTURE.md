# Motor Autônomo — Arquitetura inicial

Status: rascunho v0.1

## Tese

A inteligência principal deve estar no sistema, não depender exclusivamente do modelo.
O motor deve continuar útil com modelos pequenos, antigos, gratuitos e com janelas de contexto reduzidas.

## Princípios

1. Núcleo determinístico; IA usada apenas onde ambiguidade e julgamento são necessários.
2. Estado persistente fora do contexto do modelo.
3. Contexto montado sob demanda e limitado por orçamento.
4. Toda ação produz evidência verificável.
5. Componentes substituíveis por contratos estáveis.
6. Reinício e retomada sem perda do progresso.
7. Falha fechada para ações sem permissão ou sem validação.
8. Um modelo fraco pode exigir mais passos, mas não deve romper o protocolo.

## Visão em camadas

```text
┌──────────────────────────────────────────────────────────┐
│ Interfaces: CLI, API, UI, eventos                        │
├──────────────────────────────────────────────────────────┤
│ Control Plane: objetivos, políticas, orçamento, aprovação│
├──────────────────────────────────────────────────────────┤
│ Kernel: máquina de estados, scheduler, eventos, retomada │
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

Estados iniciais:

- `NEW`: objetivo recebido, ainda não normalizado.
- `READY`: há uma próxima unidade de trabalho executável.
- `RUNNING`: uma ação está em execução.
- `VERIFYING`: o resultado está sendo comparado aos critérios.
- `WAITING`: depende de tempo, evento ou aprovação humana.
- `REPLANNING`: estratégia falhou ou estagnou.
- `SUCCEEDED`: critérios de conclusão satisfeitos.
- `FAILED`: orçamento ou política impediu continuação segura.
- `CANCELLED`: interrompido externamente.

Loop:

```text
observe → select → prepare → act → verify → commit → repeat
```

Cada transição deve ser registrada em um log append-only.

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
