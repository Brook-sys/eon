# Protocolo para modelos fracos

Status: rascunho v0.1

## Objetivo

Permitir que modelos antigos, pequenos, gratuitos, com contexto limitado e sem tool calling confiável contribuam para um runtime autônomo contínuo.

O modelo não recebe autonomia. Ele executa uma operação cognitiva estreita e retorna uma proposta. O runtime preserva estado, controla ferramentas, valida saídas e decide a transição.

```text
modelo = função cognitiva limitada
runtime = organização, continuidade e autoridade
```

## Hipótese adversarial de capacidade

O protocolo deve continuar funcionando quando o modelo:

- aceita apenas texto de entrada e saída;
- não possui function calling;
- não garante JSON válido;
- segue mal instruções longas;
- esquece detalhes no meio do prompt;
- confunde fatos com instruções;
- inventa opções não oferecidas;
- repete ações anteriores;
- explica demais quando foi solicitada uma escolha;
- tem baixa capacidade de planejar muitos passos;
- possui janela de contexto de 2k a 8k tokens;
- sofre rate limiting severo.

Recursos modernos são acelerações opcionais, nunca requisitos estruturais. O baseline mínimo não deve produzir um teto artificial: capacidades adicionais podem ser exploradas quando descobertas e empiricamente confiáveis, sem criar um segundo modo de runtime nem enfraquecer as garantias do caminho mínimo.

## Regra de autoridade

Somente o kernel pode:

- alterar estado oficial;
- admitir `InquiryCandidate`s como `Inquiry`s;
- criar ou despachar `Operation`s;
- executar capabilities autorizadas;
- consumir orçamento;
- declarar critérios satisfeitos;
- encerrar, pausar ou redirecionar uma missão.

O modelo pode apenas propor dados para um contrato específico.

## Pipeline de uma chamada

```text
OperationSpec
  → selecionar template
  → selecionar contexto mínimo
  → serializar prompt
  → chamar modelo
  → extrair resposta
  → normalizar
  → validar
  → reparar deterministicamente
  → aceitar, repetir com correção curta ou usar fallback
  → registrar decisão e evidência
```

Nenhuma resposta atravessa diretamente do modelo para uma ferramenta.

## OperationSpec

Toda chamada cognitiva nasce de uma especificação versionada:

```yaml
id: choose-next-action
version: 1
purpose: escolher uma ação entre opções permitidas
input_fields:
  - task
  - facts
  - options
output_kind: closed_choice
max_input_tokens: 700
max_output_tokens: 40
temperature: 0
validator: allowed-option
retry_policy: correct-once-then-fallback
```

Isso torna cada uso do modelo observável, reproduzível e testável.

## Classes de operação cognitiva

Preferência da mais fácil para a mais difícil:

1. `CLASSIFY`: escolher um rótulo.
2. `RANK`: ordenar poucos candidatos.
3. `SELECT`: escolher uma opção fornecida.
4. `EXTRACT`: extrair fatos de um texto curto.
5. `COMPARE`: apontar diferenças relevantes.
6. `FILL`: completar campos restritos.
7. `PROPOSE`: sugerir um próximo incremento.
8. `TRANSFORM`: produzir um artefato delimitado.
9. `CRITIQUE`: encontrar um problema específico.
10. `DECOMPOSE`: gerar poucos filhos imediatos.

Operações abertas como `PROPOSE` e `DECOMPOSE` devem ser usadas apenas quando regras e escolhas fechadas não bastarem.

## Envelope textual mínimo

O formato primário deve funcionar sem JSON mode:

```text
TASK
Escolha a próxima ação.

FACTS
F1: teste login falha com KeyError token
F2: arquivo auth.py contém função create_token

OPTIONS
A: READ_AUTH
B: RUN_LOGIN_TEST
C: SEARCH_TOKEN_USAGE

RULES
- Escolha exatamente uma opção.
- Não invente opções.

ANSWER
Somente: A, B ou C
```

Resposta esperada:

```text
C
```

O parser aceita variações toleráveis como `C.`, `Opção C` ou `ANSWER: C`, mas normaliza todas para o mesmo valor.

## Formatos por robustez

### Nível 0 — token fechado

```text
A
```

Usar para seleção, classificação e confirmação.

### Nível 1 — campos delimitados

```text
CHOICE: C
REASON: falta localizar os usos de token
```

Usar quando uma justificativa curta ajuda a auditoria. A escolha é validada independentemente da justificativa.

### Nível 2 — bloco estruturado simples

```text
<result>
title=Localizar usos de token
kind=search
target=token
</result>
```

Usar quando são necessários poucos campos. Parser próprio, sem exigir JSON perfeito.

### Nível 3 — JSON

Usar apenas se o perfil do modelo demonstrar confiabilidade suficiente. JSON não é o protocolo universal.

## PromptCompiler

O prompt não é escrito livremente a cada turno. Ele é compilado de:

```text
template versionado
+ tarefa atômica
+ fatos selecionados
+ opções permitidas
+ restrições locais
+ formato de resposta
```

Regras:

1. instrução principal no início e formato de resposta no fim;
2. frases curtas e vocabulário concreto;
3. evitar múltiplas negativas;
4. numerar fatos e opções;
5. remover fatos não usados pela operação;
6. não incluir logs crus quando um resumo factual basta;
7. não pedir raciocínio longo;
8. reservar tokens para a saída;
9. um template por tipo de decisão;
10. incluir no máximo um ou dois exemplos curtos quando medidamente úteis.

## Orçamento de contexto

Cada chamada recebe um orçamento explícito:

```text
janela do modelo
- margem do tokenizer
- saída reservada
- template fixo
= orçamento para fatos e conteúdo
```

Exemplo em uma janela de 4096 tokens:

```text
janela nominal:       4096
margem de segurança: -400
saída reservada:      -150
template e regras:    -450
conteúdo disponível: 3096
```

Uma operação comum deve usar muito menos que o máximo. Meta inicial:

- classificação/seleção: 150–700 tokens de entrada;
- extração/comparação: 300–1500;
- transformação localizada: 800–2500;
- decomposição: 400–1200.

## Contexto em camadas

O modelo recebe somente uma visão compilada. Os rótulos abaixo nomeiam seções do envelope de prompt, não novas entidades persistidas do domínio:

1. `Operation Card`: o que a `Operation` precisa propor agora;
2. `Local Facts`: fatos diretamente relevantes;
3. `Constraints`: proibições e limites desta operação;
4. `Allowed Outputs`: respostas válidas;
5. `Artifact Slice`: apenas o trecho necessário do artefato.

Não recebe por padrão:

- conversa inteira;
- histórico completo da missão;
- todos os arquivos;
- pensamentos de turnos anteriores;
- documentação não relacionada;
- saídas extensas de ferramentas.

## Decomposição em microturnos

Uma investigação complexa é um grafo persistido de perguntas e operações, não um prompt grande.

```text
MissionRevision
  └─ Question
      └─ InquiryCandidate
          └─ Inquiry
              └─ Operation
```

`Question`, `InquiryCandidate`, `Inquiry` e `Operation` possuem contratos e estados próprios conforme aplicável. O modelo vê normalmente apenas a `Operation`, com um resumo mínimo da `Inquiry`, da `Question` e das restrições da `MissionRevision`.

Exemplo de decomposição:

```text
Objetivo: corrigir falha de autenticação

Turno 1 — CLASSIFY
Qual é a classe do erro?

Turno 2 — SELECT
Qual fonte de informação consultar primeiro?

Turno 3 — EXTRACT
Extraia as condições da função relevante.

Turno 4 — COMPARE
Compare comportamento esperado e observado.

Turno 5 — PROPOSE
Proponha uma alteração localizada.

Executor determinístico aplica patch.

Turno 6 — CRITIQUE
O patch viola alguma restrição listada?

Verificador determinístico executa testes.
```

Cada turno pode usar um modelo diferente ou ser repetido sem reconstruir toda a conversa.

## Expansão hierárquica controlada

O modelo nunca recebe simplesmente “faça um plano completo”. Quando regras não bastam, o runtime solicita no máximo poucas propostas imediatas de `Operation` ou `InquiryCandidate`:

```text
Produza de 1 a 3 próximos passos.
Cada passo deve:
- terminar em evidência observável;
- caber em uma operação ou ser marcado DECOMPOSE;
- não repetir os itens listados;
- respeitar as capacidades permitidas.
```

Depois, validadores determinísticos verificam:

- quantidade de propostas;
- ausência de ciclos e duplicações;
- rastreabilidade à pergunta, investigação e revisão da missão;
- condição de resposta ou conclusão;
- capability existente;
- custo e risco permitidos;
- profundidade máxima.

O grafo é expandido sob demanda, não antecipadamente. A saída do modelo permanece proposta: admissão e criação oficial são decisões determinísticas.

## Tool calling sem tool calling

O modelo não precisa emitir chamadas de função. O runtime apresenta ações simbólicas:

```text
A: READ_FILE
B: SEARCH_TEXT
C: RUN_TEST
D: ASK_DECOMPOSE
```

Depois da escolha, um segundo microturno opcional preenche argumentos limitados, ou o runtime os deriva deterministicamente do estado.

Exemplo:

```text
ACTION: SEARCH_TEXT
TARGET_OPTIONS:
A: token
B: create_token
C: KeyError
ANSWER: A, B ou C
```

O `CapabilityBinder` converte a escolha validada em uma invocação interna. O modelo nunca fornece nome de executável, comando shell ou argumentos irrestritos quando uma enumeração pode ser usada.

## Escada de recuperação

Quando a saída é inválida:

1. normalizar espaços, pontuação e delimitadores;
2. extrair opção conhecida ou campos reconhecidos;
3. validar contra contrato;
4. se houver uma única interpretação segura, aceitar normalizada;
5. enviar uma correção curta contendo somente erro e formato esperado;
6. trocar para formato mais simples;
7. usar fallback de modelo ou estratégia determinística;
8. adiar ou solicitar intervenção se não houver saída segura.

Não reenviar automaticamente todo o prompt várias vezes. Isso desperdiça rate limit e pode reproduzir o mesmo erro.

### Implementação local (passos 1–4, sem provider)

O pacote `internal/modeltext` materializa os passos determinísticos e authority-free:

| Passo | JSON (`NormalizeJSONCandidate` / `BestJSONCandidate`) | Escolha fechada (`NormalizeClosedToken`) |
| --- | --- | --- |
| 1 | strip BOM, trim | strip BOM, trim, última linha não vazia |
| 2 | uma fence markdown externa (` ``` ` / ` ```json `) | prefixos `ANSWER:` / `Opção` / `option:`, aspas, pontuação |
| 3–4 | extrair o primeiro objeto `{...}` balanceado (respeita strings) e validar no decoder estrito | token único; maiúscula de letra isolada; allowlist no caller |

Regras firmes:

- a saída **crua** do provider permanece no artefato `RawModelOutput` (bytes exatos);
- a forma normalizada só alimenta decode/validação tipados (`changeset.DecodeStrict`, `evaluation.Parse` JSON, injeção de linhagem em `ensureProposalLineage`);
- não inventa chaves, não fecha chaves truncadas, não funde múltiplos objetos;
- se não houver interpretação segura, o caminho falha/replana sem loop de `Complete` (FR-MODEL-004).

Passos 5–8 (correção curta reenviada, formato mais simples, fallback de modelo) permanecem planejados e exigem budget de retry persistido + endpoint; não estão no caminho mínimo offline.

## Perfil adaptativo de modelo e provider

Cada combinação de provider, modelo, versão e configuração possui um perfil versionado. O perfil separa quatro fontes de informação:

1. capacidades declaradas pelo operador ou pelo endpoint;
2. capacidades confirmadas por probes seguros e limitados;
3. confiabilidade empírica observada por operação, formato e faixa de contexto;
4. overrides explícitos, inclusive para desabilitar uma capacidade anunciada mas instável.

```yaml
provider: local-openai-compatible
model: llama-3.1-8b
model_revision: unknown
profile_version: 3
observed_at: 2026-07-15T20:47:00-03:00
capabilities:
  text_completion: confirmed
  system_role: confirmed
  tool_calling: unsupported
  json_mode: unreliable
  streaming: untested
limits:
  context_tokens:
    declared: 8192
    safe_observed: 7168
  output_tokens:
    declared: 2048
    safe_observed: 768
reliable_formats:
  closed_choice: 0.99
  delimited_fields: 0.93
  json: 0.71
competencies:
  classify: 0.90
  extract: 0.84
  decompose: 0.58
preferred_prompt_style: concise
max_safe_options: 5
```

Ausência de evidência é `unknown`, não `supported`. Probes devem ter orçamento, timeout, cache, validade e condição de nova execução; nunca podem formar um loop de autodetecção. Falhas reais podem rebaixar temporariamente uma capacidade e abrir circuit breaker, mas promoção exige evidência suficiente ou confirmação do operador.

O router escolhe operação, template, formato e modelo com base em resultados reais, não apenas no tamanho declarado ou no marketing do modelo.

## Adaptação monotônica e segura

A adaptação segue uma escada única. O caminho mínimo `text -> text` permanece sempre disponível; níveis superiores somente substituem partes do transporte ou aumentam eficiência:

1. **Baseline:** prompt textual curto, resposta textual, parser e validação externa.
2. **Formato assistido:** campos delimitados, JSON mode ou JSON Schema quando a taxa de validade justificar o uso.
3. **Tool calling nativo:** pode reduzir serialização e parsing, mas a chamada recebida continua sendo somente uma proposta validada e vinculada pelo `CapabilityBinder`.
4. **Contexto ampliado:** permite selecionar mais evidência relevante, reduzir compressão com perda ou executar comparação local mais rica; não autoriza enviar histórico integral nem abandonar microturnos.
5. **Competência superior:** pode receber operações cognitivas um pouco mais abertas ou horizonte imediato maior, dentro de limites explícitos e com verificação igual ou mais forte.

Toda promoção deve ser reversível por chamada. Se um recurso falhar, o runtime degrada para o próximo nível seguro sem perder a `Operation`, seu budget ou a possibilidade de retomada. Uma capacidade melhor MAY melhorar qualidade, velocidade ou custo; MUST NOT ampliar autoridade, ignorar validadores, alterar a semântica do `OperationSpec` ou tornar o estado dependente de contexto conversacional oculto.

A adaptação é conservadora:

- usar a menor capacidade que cumpra a operação com confiabilidade suficiente;
- reservar margens abaixo do limite de contexto anunciado;
- limitar o aumento de contexto, opções e horizonte por política;
- comparar sucesso semântico, não somente validade de formato;
- rebaixar rapidamente diante de incompatibilidade e promover lentamente;
- preservar registro de qual perfil e nível produziram cada decisão.

## Melhoria contínua do harness

O runtime melhora seus procedimentos, não seus objetivos de forma independente.

Registrar por chamada:

- modelo e versão;
- `OperationSpec` e template;
- tokens estimados/reais;
- latência;
- saída crua e normalizada;
- erros de validação;
- reparos aplicados;
- resultado posterior da ação;
- custo e evidência final.

Com isso, pode:

- identificar templates que falham;
- reduzir contexto sem perder acurácia;
- preferir formatos mais confiáveis;
- rotear operações por competência;
- promover regras determinísticas para padrões recorrentes;
- criar testes de regressão a partir de falhas reais.

Alterações no próprio protocolo devem ser versionadas, avaliadas offline e promovidas por política. O motor não reescreve silenciosamente seus contratos em produção.

## Benchmark mínimo

Matriz inicial:

- modelos: pelo menos um pequeno/antigo local e um baseline superior;
- contexto máximo: 2k, 4k e 8k;
- formatos: token fechado, campos delimitados e JSON;
- operações: classify, select, extract, decompose e critique;
- com e sem histórico conversacional;
- com e sem reparo determinístico.

Métricas:

- validade sintática;
- aderência às opções;
- acurácia da decisão;
- tokens por decisão aceita;
- chamadas por operação concluída;
- recuperação após saída inválida;
- repetição de ações;
- progresso final da tarefa;
- taxa de execução segura sem tool calling nativo.

## Harness executável inicial

O baseline do benchmark está implementado em `internal/evaluation` e é
executável por `cmd/model-benchmark-runner`. O corpus versionado
`cognitive-v1` cobre extração, síntese, detecção de conflito e reparo ancorado.
Cada caso declara fatos, formatos permitidos e resposta de referência; o runner
cruza os casos com 2k/4k/8k, reutiliza o compilador sob budget e registra:

- falha de compilação antes de consultar o modelo;
- validade sintática por formato estrito;
- acerto semântico por referência exata;
- fatos opcionais omitidos, tokens estimados/reais e latência;
- relatório JSON completo e resumo Markdown publicados atomicamente.

A fixture não é evidência de qualidade de nenhum modelo. Ela fixa o protocolo
para que execuções posteriores contra um modelo pequeno/local e um baseline
superior sejam comparáveis. Ferramentas existentes como LM Evaluation Harness
foram consideradas no preflight; o harness local permanece estreito porque
precisa exercitar diretamente `OperationSpec`, o compilador e a porta
`ModelProvider`, sem introduzir um runtime Python paralelo no primeiro slice.

## Continuidade permanente acima de sofisticação

Enquanto a `MissionRevision` estiver ativa e o armazenamento permanecer operacional, o runtime é um processo permanente: não existe conclusão global implícita. Concluir uma `Operation` ou `Inquiry` deve revelar, atualizar ou invalidar próximos candidatos; concluir uma frente deve devolver o controle ao replenisher, à manutenção recorrente e à revisão da missão.

O runtime mantém uma fronteira limitada, mas renovável, de trabalho futuro derivado da missão, das lacunas, dos riscos, da necessidade de manutenção e do estado persistido. A fronteira não precisa materializar um plano infinito: precisa preservar sementes legítimas e condições determinísticas para gerar o próximo horizonte curto sempre que o atual diminuir.

Perguntas ao usuário, aprovações, callbacks e dependências externas bloqueiam somente as unidades que realmente dependem deles. O scheduler deve procurar outras linhas independentes de investigação, validação, manutenção ou melhoria. Silêncio do usuário nunca encerra a missão nem paralisa trabalho não dependente.

Não existe `Rest` global enquanto a missão estiver ativa. Se nenhuma operação imediata for admissível em uma frente, o motor tenta outras famílias de descoberta, verificação, manutenção, síntese, avaliação e melhoria. Esperas permanecem locais. Se nenhuma família puder produzir trabalho útil, o runtime registra uma falha de continuidade diagnosticável, em vez de normalizar inatividade.

“Continuar sempre” não autoriza busy loop, trabalho fictício, retries ilimitados ou ações arriscadas. A continuidade estável inclui:

- persistir antes de atravessar fronteiras frágeis;
- preferir ações pequenas, idempotentes, verificáveis e reversíveis;
- manter mais de uma linha de progresso quando a missão permitir;
- evitar dependência desnecessária de recursos opcionais do modelo;
- limitar retries e reconciliar efeitos ambíguos antes de repetir;
- preservar alternativas e fallback quando uma ação bloqueia;
- transformar falha ou estagnação em diagnóstico, replanejamento e novo trabalho preventivo;
- executar manutenção recorrente, revalidação e busca de melhoria quando frentes finitas terminarem;
- degradar qualidade, velocidade ou frequência antes de degradar integridade;
- trocar de família quando uma ação imediata for insegura ou impossível, registrando `CONTINUITY_BLOCKED` se nenhuma alternativa legítima existir.

## Critério arquitetural de sucesso

O harness está cumprindo sua proposta quando:

1. trocar por um modelo mais fraco degrada principalmente velocidade e qualidade local, não a integridade do runtime;
2. uma saída malformada não produz efeito colateral;
3. a tarefa pode ser retomada em outro turno ou modelo apenas com estado persistido;
4. tool calling nativo melhora eficiência, mas sua ausência não impede execução;
5. contexto maior melhora seletivamente evidência ou qualidade, sem crescimento descontrolado do prompt;
6. o contexto médio por chamada permanece pequeno e previsível;
7. o runtime detecta, promove, rebaixa e audita capacidades sem interromper o caminho mínimo;
8. erros recorrentes se transformam em validadores, templates ou regras melhores;
9. a agenda e a frontier mantêm próximos passos úteis ou sementes legítimas para produzi-los sem loops artificiais;
10. espera por usuário ou dependência bloqueia apenas a linha dependente, enquanto linhas independentes continuam;
11. com missão ativa, término de uma frente sempre retorna a manutenção, replenishment e melhoria, nunca a uma conclusão global implícita.
