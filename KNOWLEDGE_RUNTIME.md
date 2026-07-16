# Runtime Epistemológico Contínuo

Status: proposta de domínio v0.1

## 1. Recorte provisório

O primeiro domínio do motor será **construção e manutenção contínua de conhecimento orientado por missão**.

O sistema não nasce como agente geral de automação. Suas capacidades externas iniciais serão deliberadamente estreitas:

- pesquisar fontes na internet;
- descobrir e ler arquivos autorizados;
- ingerir documentos;
- extrair observações e alegações;
- organizar conceitos, perguntas, evidências e relações;
- comparar fontes;
- detectar lacunas, conflitos, desatualização e baixa cobertura;
- produzir sínteses, planos de investigação e relatórios;
- revisar, corrigir, versionar e depreciar conhecimento;
- responder ao usuário com proveniência.

Escrever em arquivos arbitrários, executar código, depurar sistemas e operar serviços ficam fora do MVP. O runtime poderá gerar **planos sobre essas atividades** como conhecimento, mas não executá-las sem uma expansão futura e explícita de autoridade.

## 2. Formulação

> Um runtime epistemológico de longa duração, delimitado por uma missão versionada, que transforma fontes em conhecimento rastreável, deriva continuamente perguntas e investigações úteis, e revisa suas conclusões diante de novas evidências ou mudanças da missão.

Em inglês, como termo de trabalho:

> **Mission-bounded long-lived epistemic runtime with persistent evidence, continual inquiry planning, and provenance-aware knowledge revision.**

“Continuous autonomous LLM” não será usado como categoria científica assumida. Devemos distinguir:

- execução durável;
- planejamento contínuo;
- memória persistente;
- investigação iterativa;
- revisão de conhecimento;
- aprendizagem lifelong;
- tarefas de longo horizonte.

## 3. O “prompt mestre” não pode ser apenas um prompt

O usuário poderá alterar a orientação a qualquer momento, mas a representação oficial deve ser uma `MissionSpec` versionada. Linguagem natural é a interface de entrada; o runtime normaliza a intenção em campos explícitos.

```yaml
mission_id: autonomous-runtime-research
revision: 7
purpose: investigar e projetar um runtime autônomo para modelos fracos
questions:
  - quais mecanismos determinísticos compensam limitações do modelo?
  - como medir progresso epistemológico contínuo?
scope:
  include: [LLM agents, planning, durable execution, knowledge systems]
  exclude: [automação irrestrita, ações externas sem aprovação]
quality_policy:
  evidence_preference: primary_sources
  minimum_corroboration: risk_dependent
  citation_required: true
  separate_fact_inference_hypothesis: true
resource_policy:
  daily_model_calls: 50
  daily_searches: 30
cadence:
  review_interval: 24h
deliverables:
  - literature_map
  - open_questions
  - architecture_decisions
stop_or_pause_conditions:
  - no_admissible_inquiry
  - budget_exhausted
```

O texto original do usuário permanece armazenado integralmente. A normalização não o substitui e precisa ser confirmável, auditável e reversível.

## 4. Mudança da missão

Uma edição do usuário gera uma nova revisão, nunca uma mutação silenciosa:

```text
MissionSpec r7
    + UserAmendment
    → candidate r8
    → semantic diff
    → impact analysis
    → acceptance
    → agenda reconciliation
```

A análise de impacto classifica:

- conhecimento ainda relevante;
- conhecimento válido, mas agora fora do escopo;
- alegações que precisam de nova verificação;
- investigações canceladas;
- novas lacunas;
- prioridades alteradas;
- restrições novas ou removidas.

Conhecimento antigo não é apagado. É versionado e marcado com sua relação à revisão atual da missão.

## 5. Unidade epistemológica, não documento monolítico

A base não deve ser somente um conjunto de textos gerados. A unidade central é uma alegação contextualizada e ligada a evidências.

### Entidades mínimas

#### Source

Objeto externo ou local identificado:

- URL, arquivo, paper, página, dataset ou nota do usuário;
- autor, data, versão e tipo;
- data de acesso;
- hash ou snapshot quando permitido;
- nível da fonte: primária, secundária ou opinião;
- escopo e possíveis conflitos de interesse.

#### SourceFragment

Trecho endereçável de uma fonte:

- página, seção, parágrafo, linhas ou timestamp;
- conteúdo exato ou hash;
- contexto suficiente para evitar citação enganosa.

#### Observation

Registro fiel do que uma fonte declara ou do que uma ferramenta observou. Ainda não é verdade aceita pelo sistema.

#### Claim

Proposição atômica, contestável e versionada.

```yaml
claim_id: claim_...
statement: planejamento incremental reduziu determinada métrica no experimento X
qualifiers:
  population: modelos avaliados no paper X
  domain: benchmark Y
  time: versão publicada em 2024
status: supported|contested|unsupported|obsolete|unknown
confidence: calibrated_value_or_bucket
mission_revision: 7
```

#### EvidenceLink

Relaciona fragmentos ou resultados a uma alegação:

- `SUPPORTS`;
- `CONTRADICTS`;
- `QUALIFIES`;
- `REPLICATES`;
- `FAILS_TO_REPLICATE`;
- `DERIVED_FROM`;
- `MENTIONS`, sem apoio probatório;
- `SUPERSEDES`.

#### Inference

Conclusão derivada de claims e regras explícitas. Deve registrar premissas, método, modelo/template usado e grau de incerteza.

#### Question

Lacuna de conhecimento com razão, prioridade e condição de resposta.

#### Hypothesis

Resposta provisória testável, não promovida a fato.

#### KnowledgeArtifact

Síntese, mapa, relatório, plano, FAQ ou decisão arquitetural gerada a partir do grafo. É uma visão materializada e regenerável, não a fonte canônica da verdade.

## 6. Separações obrigatórias

O runtime nunca deve colapsar automaticamente:

```text
fonte       ≠ evidência suficiente
observação  ≠ fato
alegação    ≠ conclusão aceita
correlação  ≠ causalidade
ausência de evidência ≠ evidência de ausência
resumo do modelo ≠ conteúdo original
consenso aparente ≠ verdade
confiança do modelo ≠ confiança epistemológica
texto novo  ≠ conhecimento melhor
```

Também deve distinguir:

- evidência direta e indireta;
- fonte primária e comentário secundário;
- achado do paper e nossa interpretação;
- validade interna e generalização;
- atualidade da fonte e atualidade da alegação;
- contradição real e diferença de escopo/definição.

## 7. Pipeline epistemológico

```text
MissionSpec
  → mapear perguntas e conceitos
  → inspecionar conhecimento existente
  → identificar lacunas e conflitos
  → formular InquiryCandidates
  → selecionar investigação
  → projetar estratégia de busca
  → descobrir fontes
  → avaliar e adquirir fontes
  → extrair observações com localização
  → propor claims atômicos
  → vincular evidências
  → procurar contraprovas e qualificadores
  → validar relações e escopo
  → atualizar estado epistemológico
  → regenerar artefatos afetados
  → medir ganho de conhecimento
  → derivar próximas perguntas
  → dormir ou repetir
```

Cada seta representa operações independentes, persistidas e recuperáveis. Não é uma única chamada ao modelo.

## 8. Agenda de investigação

O `AgendaReplenisher` passa a gerar prioritariamente `InquiryCandidate`s, não tarefas genéricas.

```yaml
inquiry_id: inquiry_...
question: ...
derived_from:
  mission_revision: 7
  gap_id: gap_...
expected_epistemic_gain:
  coverage: 0.3
  uncertainty_reduction: 0.5
  contradiction_resolution: 0.0
novelty: ...
source_plan: ...
estimated_cost: ...
risk: ...
answer_condition: ...
stop_condition: ...
review_after: ...
```

Fontes de novas investigações:

- pergunta explícita da missão ainda sem resposta;
- claim relevante sem evidência;
- claim apoiado apenas por fonte secundária;
- conflito entre fontes;
- conceito ambíguo;
- fonte possivelmente desatualizada;
- baixa cobertura de um subtema;
- limitação metodológica importante;
- resultado que precisa de replicação;
- mudança de missão;
- novo documento local ou evento externo;
- oportunidade de consolidar conhecimento redundante.

## 9. Admissão e prioridade

Não existe uma fórmula universal pronta. Inicialmente, a prioridade será uma política explicável composta por dimensões normalizadas:

```text
priority = policy(
  mission_relevance,
  expected_information_gain,
  decision_impact,
  uncertainty,
  contradiction_severity,
  staleness,
  coverage_gap,
  novelty,
  source_availability,
  cost,
  risk,
  redundancy
)
```

Regras duras antecedem o score:

- rejeitar fora do escopo;
- rejeitar duplicata ou item subsumido;
- rejeitar sem condição de parada;
- rejeitar investigação sem possível mudança de estado epistemológico;
- exigir aprovação para fontes, dados ou domínios sensíveis;
- limitar investigações abertas simultâneas;
- reservar orçamento para contraprovas, não apenas confirmação.

Pesos e limiares serão parâmetros experimentais, não constantes “corretas”.

## 10. Estados do conhecimento

Além dos estados operacionais da tarefa, claims possuem ciclo próprio:

```text
PROPOSED
  → EXTRACTED
  → EVIDENCE_LINKED
  → REVIEWED
  → SUPPORTED | CONTESTED | UNSUPPORTED | UNKNOWN
  → SUPERSEDED | OBSOLETE
```

Uma nova evidência não sobrescreve a anterior. Ela adiciona uma relação e dispara revisão dos claims e artefatos dependentes.

## 11. Truth maintenance e dependências

Toda síntese ou decisão deve possuir dependências rastreáveis:

```text
SourceFragment → Observation → Claim → Inference → KnowledgeArtifact/ADR
```

Quando um nó muda:

1. marcar dependentes como potencialmente obsoletos;
2. calcular impacto;
3. reavaliar somente o subgrafo afetado;
4. preservar versões anteriores;
5. registrar por que a conclusão mudou.

O projeto deve estudar Truth Maintenance Systems, argumentation frameworks, nanopublications e padrões W3C PROV antes de inventar um modelo próprio completo.

## 12. Operações cognitivas adequadas ao domínio

Escolha fechada continua útil, mas é apenas uma classe. O protocolo precisará suportar:

- classificação de tipo de fonte;
- extração ancorada em fragmento;
- segmentação de alegações;
- normalização terminológica;
- detecção candidata de duplicação;
- detecção candidata de contradição;
- comparação de escopo e métodos;
- geração de consultas;
- formulação de perguntas;
- síntese com citações obrigatórias;
- crítica de cobertura;
- avaliação de relevância;
- identificação de premissas e limitações.

Para cada operação, deve existir:

- contrato de entrada e saída;
- autoridade máxima da proposta;
- validador;
- necessidade de revisão cruzada;
- tolerância a falso positivo e falso negativo;
- perfil mínimo de modelo;
- benchmark específico.

Exemplo: um detector de contradição do LLM **cria um candidato a conflito**. Ele não muda sozinho dois claims para `CONTESTED`.

## 13. O que significa “melhorar a base”

Melhoria precisa ser uma mudança observável em uma ou mais dimensões:

- maior cobertura de perguntas relevantes;
- melhor apoio por fontes primárias;
- maior precisão de citações;
- menor número de claims sem evidência;
- redução de contradições não examinadas;
- menor desatualização;
- melhor distinção entre fato, inferência e hipótese;
- melhor compressão sem perda de rastreabilidade;
- maior utilidade para decisões e respostas;
- menor custo para recuperar o contexto necessário;
- correção de conhecimento anteriormente errado.

Não contam isoladamente como progresso:

- mais tokens;
- mais documentos;
- mais claims redundantes;
- reescrever uma síntese sem nova evidência;
- “refletir” repetidamente sobre o mesmo estado;
- pesquisar indefinidamente sem critério de suficiência.

## 14. Condições locais de término e continuidade global

Uma investigação termina quando:

- sua pergunta satisfaz o critério de resposta;
- o orçamento termina;
- não há fonte admissível disponível;
- o ganho marginal esperado cai abaixo do limiar;
- ela foi subsumida ou invalidada;
- requer decisão do usuário.

O término de uma investigação é local. Enquanto a missão estiver ativa, não representa conclusão do runtime. O motor retorna à frontier, às obrigações recorrentes e ao replenishment para manter a continuidade permanente.

O runtime entra em `Rest` de baixo consumo quando não existem investigações admissíveis imediatas e registra:

- quais perguntas permanecem abertas;
- por que não são acionáveis agora;
- quais linhas estão bloqueadas por usuário, evento ou recurso;
- quais linhas independentes foram consideradas;
- quais eventos, datas ou recursos podem antecipar a reavaliação;
- a próxima revisão temporal interna obrigatória.

Pergunta ao usuário ou evento externo nunca é a única condição global de despertar. Se a resposta não chegar, a linha permanece pendente sem impedir manutenção, revalidação, pesquisa de outras lacunas ou melhoria autorizada. Ao chegar a revisão programada, o motor reconcilia as esperas e tenta gerar novo horizonte útil mesmo sem qualquer evento externo.

## 15. Capacidades do MVP

### Permitidas

1. `web.search`
2. `web.fetch`
3. `file.discover` em raízes autorizadas
4. `file.read`
5. `source.snapshot/hash`
6. `text.segment`
7. `citation.locate`
8. `knowledge.propose`
9. `knowledge.review`
10. `artifact.render`

### Não permitidas por padrão

- shell arbitrário;
- execução de código proveniente de fontes;
- alteração de arquivos fora da base gerenciada;
- envio de mensagens ou publicação;
- login e ações em contas;
- modificações de sistema;
- aquisição autônoma de novos objetivos finais.

## 16. Baseline do MVP

Entrada:

- uma `MissionSpec` criada a partir do prompt mestre;
- um pequeno conjunto opcional de arquivos iniciais;
- acesso de leitura à web.

Saída contínua:

- mapa de perguntas;
- catálogo de fontes;
- claims com evidência e status;
- conflitos e lacunas;
- sínteses versionadas;
- agenda das próximas investigações;
- changelog epistemológico: o que foi aprendido, corrigido ou depreciado.

## 17. Riscos específicos

- contaminação da base por extrações incorretas;
- falsa corroboração por fontes que repetem a mesma origem;
- viés de confirmação na geração de buscas;
- perda de qualificadores ao atomizar claims;
- contradições falsas causadas por escopos diferentes;
- autoridade indevida concedida a rankings heurísticos;
- envelhecimento silencioso;
- circularidade entre sínteses geradas pelo próprio sistema;
- prompt injection em documentos e páginas;
- crescimento ilimitado e degradação de recuperação;
- otimização de métricas de cobertura sem utilidade real.

O desenho de controles para esses riscos antecede a implementação autônoma contínua.

## 18. Próximo passo de projeto

Antes de implementar o loop, devemos formalizar três contratos:

1. `MissionSpec` e semântica de revisão;
2. modelo `Source–Observation–Claim–Evidence–Inference–Question`;
3. `InquiryCandidate`, admissão, prioridade e condição de parada.

Somente depois construiremos um ciclo vertical pequeno:

```text
uma pergunta
→ uma busca
→ uma fonte adquirida
→ um fragmento localizado
→ um claim proposto
→ evidência vinculada
→ revisão
→ atualização de uma síntese
→ próxima lacuna ou repouso
```

As transformações sobre a base estão detalhadas em `KNOWLEDGE_OPERATIONS.md`. A investigação de Dolt e alternativas está em `STORAGE_VERSIONING.md`.
