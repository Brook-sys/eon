# Programa de pesquisa — Motor Autônomo

Status: rascunho v0.1

## Objetivo científico e de engenharia

Projetar e avaliar um runtime epistemológico autônomo contínuo, orientado por missão, capaz de produzir progresso verificável na construção e revisão de conhecimento usando modelos de linguagem fracos, antigos, pequenos, rate-limited e sem tool calling confiável.

Restrições de implementação já aceitas: núcleo em Go e integração principal de modelos por APIs OpenAI-compatible, preservando contrato mínimo texto-para-texto. A escolha de Dolt continua como hipótese de engenharia a ser avaliada.

Não assumimos que a solução é inédita. A contribuição potencial só poderá ser formulada depois de uma revisão de literatura e comparação sistemática com planejamento clássico, arquiteturas de agentes, workflows duráveis e harnesses LLM existentes.

## Pergunta central

Até que ponto organização determinística, investigação persistente, contexto compilado, interfaces restritas, proveniência e verificação externa compensam limitações de capacidade, contexto e tool use de modelos fracos na manutenção contínua de uma base de conhecimento orientada por missão?

O domínio inicial está detalhado em `KNOWLEDGE_RUNTIME.md`. Automação geral, manipulação arbitrária de arquivos, execução de código e depuração ficam fora do MVP.

## Subperguntas

1. Qual representação de missão, objetivos, etapas e operações preserva rastreabilidade sem impor rigidez excessiva?
2. Quando uma decisão deve ser regra, busca, solver, modelo ou intervenção humana?
3. Como decompor incrementalmente sem explosão da árvore, perda semântica ou erro acumulado?
4. Como gerar novas tarefas úteis sem produzir atividade circular, deriva de missão ou expansão ilimitada?
5. Qual interface de saída é mais robusta por tipo e perfil de modelo: escolha fechada, slots, DSL, gramática, JSON ou texto interpretado?
6. Como compilar o menor contexto suficiente e detectar quando informação essencial foi omitida?
7. Como medir progresso em missões contínuas que não possuem um único estado terminal?
8. Como verificar resultados quando não existe ground truth imediato?
9. Como aprender com episódios sem corromper contratos, memória ou políticas?
10. Qual degradação ocorre em horizontes longos e como isolá-la por componente?
11. Como distinguir crescimento textual de ganho epistemológico real?
12. Como revisar conclusões e artefatos quando fontes, claims ou a própria missão mudam?
13. Como evitar falsa corroboração, viés de confirmação e circularidade entre conteúdos gerados pelo sistema?

## Hipóteses iniciais — ainda não confirmadas

H1. Modelos fracos apresentam maior confiabilidade quando operações abertas são substituídas por uma hierarquia de interfaces progressivamente restritas, escolhidas conforme a incerteza da decisão.

H2. Representar decomposição e execução fora do contexto, em uma estrutura persistente formal, reduz falhas de longo horizonte mais que aumentar simplesmente o contexto enviado ao modelo.

H3. Planejamento hierárquico incremental com verificação entre níveis reduz propagação de erro em comparação com planos completos gerados em uma chamada.

H4. Um roteador que usa regras e solvers antes do LLM reduz custo e aumenta confiabilidade sem perda relevante de cobertura.

H5. A melhoria do harness — templates, validadores, políticas e métodos — produz ganhos transferíveis entre modelos, mas pode superajustar aos benchmarks se não houver avaliação cruzada.

## Linhas de literatura

### A. Planejamento e acting clássicos

- Hierarchical Task Networks (HTN/HDDL)
- planning and acting integrados
- temporal planning e scheduling
- partial-order planning
- behavior trees
- Petri nets e workflow nets
- supervisory control e runtime verification

### B. Arquiteturas de agentes

- BDI: beliefs, desires, intentions
- MAPE-K e autonomic computing
- continual/lifelong agents
- mixed-initiative autonomy
- metacognitive control

### C. Raciocínio e planejamento com LLM

- ReAct
- Plan-and-Solve
- Least-to-Most
- Tree/Graph of Thoughts
- self-reflection e verifier-guided search
- program-aided e solver-aided reasoning

### D. Interação restrita e tool use

- constrained decoding por CFG/FSM
- slot filling e semantic parsing
- DSLs intermediárias
- JSON schema/constrained generation
- function-calling benchmarks
- tool use por modelos pequenos

### E. Memória e longo horizonte

- memória episódica, semântica e procedural
- event sourcing e durable execution
- continual learning e catastrophic forgetting
- benchmarks de memória e tarefas longas
- causal/provenance graphs

### F. Segurança e confiabilidade

- assurance cases
- fault tolerance
- error propagation
- drift e reward hacking
- specification gaming
- uncertainty calibration
- human oversight e interruptibility

## Método de revisão

Para cada fonte, registrar:

```yaml
id: identificador local
citation: referência completa
year: 2024
type: paper|survey|standard|system|benchmark
peer_reviewed: yes|no|unknown
url: ...
problem: ...
method: ...
models_or_systems: ...
tasks: ...
baselines: ...
metrics: ...
main_result: ...
limitations_reported: ...
limitations_observed: ...
relevance: ...
design_implication: ...
confidence: low|medium|high
```

### Hierarquia de evidência

1. resultados reproduzidos por nós;
2. estudos experimentais revisados por pares com baselines adequados;
3. preprints experimentais com código/dados;
4. surveys e revisões;
5. documentação de sistemas e standards;
6. relatos técnicos e artigos sem avaliação controlada;
7. opinião ou intuição arquitetural.

Uma decisão importante não deve ser apresentada como cientificamente estabelecida quando repousa apenas nos níveis 6 ou 7.

## Artefatos previstos

- `LITERATURE_MAP.md`: mapa temático e sínteses.
- `SOURCES.yaml`: catálogo estruturado das fontes.
- `REQUIREMENTS.md`: requisitos funcionais e não funcionais.
- `FAILURE_TAXONOMY.md`: taxonomia de falhas e controles.
- `FORMAL_MODEL.md`: estados, transições, invariantes e propriedades.
- `DECISION_PROTOCOL.md`: quando e como consultar modelos.
- `TASK_MODEL.md`: missão, objetivos, etapas, tarefas e operações.
- `EVALUATION.md`: benchmarks, ablações e métricas.
- `ADRS/`: registros de decisões arquiteturais e evidências.

## Fases

### Fase 0 — Terminologia e escopo

- definir “contínuo”, “autônomo”, “progresso”, “tarefa”, “etapa” e “melhoria”;
- distinguir runtime contínuo, missão contínua e tarefa finita;
- delimitar autonomia e autoridade concedida.

### Fase 1 — Revisão e taxonomia

- mapear literatura;
- identificar componentes e falhas conhecidas;
- evitar reinventar mecanismos clássicos;
- formular lacunas sem alegações prematuras de novidade.

### Fase 2 — Modelo formal mínimo

- definir entidades e transições;
- declarar invariantes de segurança, continuidade e progresso;
- modelar rate limits, recursos, tempo, interrupções e retomada;
- verificar propriedades em modelos pequenos ou simulações.

### Fase 3 — Protocolos para modelos fracos

- definir famílias de operações;
- parametrizar interfaces de entrada e saída;
- medir confiabilidade por modelo e operação;
- estabelecer escalada de restrição, reparo e fallback.

### Fase 4 — Simulador

- modelos falsos com taxas controláveis de erro;
- recursos e rate limits simulados;
- callbacks, crashes e indisponibilidade;
- medir propagação de erro sem depender de um domínio real.

### Fase 5 — MVP e benchmark de domínio

- escolher um domínio limitado;
- implementar apenas mecanismos justificados;
- comparar com baselines simples;
- executar ablações por componente.

### Fase 6 — Continuidade e melhoria

- missões de duração crescente;
- reinícios e pausas;
- qualidade da agenda autogerada;
- aprendizado procedural controlado;
- testes de regressão e transferência entre modelos.

## Parâmetros que precisam ser explícitos

### Missão e agenda

- profundidade máxima de decomposição;
- largura máxima por expansão;
- horizonte da agenda;
- limiar de admissão de tarefa;
- orçamento de exploração versus execução;
- política de recorrência;
- limite de tarefas autogeradas por ciclo;
- regras de deduplicação e subsunção.

### Modelo

- janela nominal e janela segura;
- tokens máximos de entrada e saída por operação;
- temperatura e sampling;
- número máximo de opções;
- formato permitido;
- confiança histórica por competência;
- retries e fallback;
- cota e custo.

### Controle

- timeout e lease;
- frequência de revisão;
- backoff;
- limiar de estagnação;
- critérios de rollback;
- nível de risco que exige aprovação;
- condições de pausa segura.

Nenhum valor deve ser tratado como universal. Valores default serão hipóteses submetidas a experimento.

## Avaliação e ablações

Comparar, mantendo o mesmo modelo:

- prompt monolítico vs. microturnos;
- histórico conversacional vs. estado compilado;
- plano completo vs. expansão incremental;
- texto aberto vs. escolha/slots/DSL/gramática;
- tool calling nativo vs. binding pelo runtime;
- sem verificador vs. verificador determinístico;
- fila fixa vs. agenda renovável;
- memória textual vs. estado estruturado;
- retry integral vs. reparo localizado;
- LLM para tudo vs. rule/solver-first.

Métricas devem incluir não apenas sucesso final:

- progresso útil por chamada/token/tempo;
- validade e segurança das transições;
- taxa de tarefas autogeradas aceitas/rejeitadas;
- duplicação, ciclos e deriva de missão;
- profundidade até erro e distância de recuperação;
- erro acumulado por horizonte;
- custo de verificação;
- retomada após crash;
- calibração e cobertura de incerteza;
- carga de supervisão humana.

## Regras epistemológicas do projeto

1. Não chamar algo de novo antes de busca de anterioridade suficiente.
2. Não confundir output bem formatado com decisão correta.
3. Não inferir confiabilidade de longo horizonte a partir de tarefas curtas.
4. Não avaliar somente em modelos fortes.
5. Não otimizar apenas para um modelo, prompt ou domínio.
6. Não esconder falhas por retries ilimitados.
7. Publicar também resultados negativos e condições de falha.
8. Separar claramente mecanismo proposto, evidência disponível e hipótese futura.
