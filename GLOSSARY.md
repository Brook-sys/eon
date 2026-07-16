# Glossário Normativo

Status: baseline v0.2

Este documento fixa o vocabulário do runtime. Os termos normativos `MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT` e `MAY` são interpretados conforme BCP 14 (RFC 2119 e RFC 8174) somente quando aparecem em maiúsculas.

Fontes normativas desta convenção:

- RFC 2119: <https://www.rfc-editor.org/rfc/rfc2119>
- RFC 8174: <https://www.rfc-editor.org/rfc/rfc8174>

## 1. Autoridade e orientação

### MissionSpec

Representação versionada da intenção autorizada pelo operador. Contém propósito, perguntas, escopo, políticas, budgets, cadência, entregáveis e condições de pausa ou parada.

- O texto original do operador MUST ser preservado.
- Uma revisão MUST possuir identidade, número de revisão e proveniência.
- Uma mudança de missão MUST gerar diff semântico e análise de impacto antes de reconciliar a agenda.
- O runtime MUST NOT inventar uma nova missão final.

`Mission` pode ser usado informalmente para a missão ativa; em contratos e schemas, usar `MissionSpec`.

### MissionRevision

Versão imutável de uma `MissionSpec`. A revisão ativa orienta admissão e prioridade, mas revisões anteriores permanecem consultáveis.

### UserAmendment

Entrada do operador que propõe alteração da missão. Não é, por si só, uma revisão aceita.

## 2. Investigação e trabalho

### Question

Lacuna de conhecimento expressa como pergunta, com origem, relevância para a missão e condição de resposta.

### InquiryCandidate

Proposta de investigação ainda não admitida na agenda. Deve declarar origem, ganho epistemológico esperado, novidade, custo, risco, plano de fontes, condição de resposta, condição de parada e data/condição de revisão.

### Inquiry

Investigação admitida e persistida, derivada de um `InquiryCandidate`. Coordena operações necessárias para alterar o estado epistemológico em relação a uma pergunta.

Uma `Inquiry` MUST:

- estar ligada a uma revisão da missão;
- ter budget e condição de parada;
- possuir estado operacional recuperável;
- registrar por que foi admitida.

### Agenda

Horizonte curto, priorizado e limitado de `Inquiry`s e obrigações operacionais admitidas. Deve ser reabastecido preventivamente ao atingir a marca baixa, sem crescimento ilimitado.

### WorkFrontier

Reservatório persistido de perguntas, lacunas, conflitos, riscos, decomposições, melhorias e oportunidades ainda não admitidos como `Inquiry`. Possui deduplicação, limites, compactação e rastreabilidade de derivação.

### WorkOpportunity

Unidade persistida da `WorkFrontier` que descreve trabalho potencial antes da admissão. Registra origem, família, ganho esperado, novidade, dependências, custo, risco, condição de parada e, quando derivada recursivamente, pai e profundidade.

### ExecutableHorizon

Faixa limitada de trabalho admitido que o scheduler consegue considerar no curto prazo. É mantida por marcas de reabastecimento definidas em política, sem materializar toda a frontier como operações.

### OperationSpec

Contrato versionado para uma transformação cognitiva ou determinística estreita. Define entradas, read set, formato de saída, budget, estratégia de decisão, validadores, retry/fallback e autoridade máxima.

### Operation

Instância executável de uma `OperationSpec`, ligada a uma `Inquiry`, inputs concretos, tentativa, lease e estado operacional.

`Operation` é a menor unidade agendada que pode consumir um recurso. Uma chamada de modelo é apenas uma possível etapa de uma operação.

## 3. Fontes e estado epistemológico

### Source

Objeto identificado que pode ser observado: página, paper, arquivo, dataset, nota do usuário ou resultado de ferramenta. Identidade e metadados não implicam veracidade.

### SourceVersion

Versão observada de uma fonte, identificada por hash, versão externa, data de observação ou combinação equivalente.

### SourceFragment

Trecho endereçável de uma `SourceVersion`, com localização e conteúdo exato ou verificável. Fragmentos MUST permitir recuperar contexto suficiente para auditar a observação.

### Observation

Representação fiel do que uma fonte declara ou do que uma ferramenta determinística observou. Uma observação MUST ser ancorada em fonte/fragmento ou recibo de execução. Não é automaticamente aceita como fato.

### Claim

Proposição atômica, contestável e versionada. Deve preservar qualificadores relevantes como população, tempo, escopo, modalidade, método e condições.

Um `Claim` MUST NOT conter a citação como se ela fosse parte da própria proposição. Apoio e oposição pertencem a `EvidenceLink`s.

### EvidenceLink

Relação tipada entre evidência e claim, tal como `SUPPORTS`, `CONTRADICTS`, `QUALIFIES`, `REPLICATES`, `FAILS_TO_REPLICATE`, `DERIVED_FROM`, `MENTIONS` ou `SUPERSEDES`.

Uma referência que apenas menciona um claim MUST NOT ser promovida automaticamente a `SUPPORTS`.

### Inference

Conclusão derivada de claims e regras ou método declarados. Deve registrar premissas, procedimento, versão do modelo/template quando aplicável e incerteza.

### Hypothesis

Resposta provisória e testável a uma pergunta. Não é claim aceito apenas por ter sido gerada ou considerada plausível pelo modelo.

### KnowledgeState

Conjunto canônico versionado de fontes, fragmentos, observações, claims, evidências, inferências, perguntas, hipóteses e dependências aceitas.

### KnowledgeArtifact

Visão materializada, como relatório, mapa, resumo, plano ou resposta, produzida do `KnowledgeState`. É regenerável e MUST NOT substituir suas dependências canônicas.

## 4. Mudança e versionamento

### ProposedChangeSet

Proposta imutável de alteração sobre uma versão-base. Contém read set, precondições, alterações tipadas, delta epistemológico esperado, validações, proveniência e revisão da missão.

Um `ProposedChangeSet` não altera estado oficial.

### AcceptedChangeSet

`ProposedChangeSet` que passou por validação estrutural, revisão epistemológica e política de autoridade. A aceitação autoriza tentativa de commit, mas não prova que o commit ocorreu.

### Commit

Aplicação atômica e durável de um `AcceptedChangeSet`, produzindo nova versão oficial do estado e recibo identificável.

- `Commit` é conceito do domínio e MUST ser independente de Dolt ou Git.
- Falha antes do commit MUST deixar o changeset reaplicável ou reconciliável.
- Retry MUST NOT criar segundo efeito lógico.

### EpistemicDiff

Descrição tipada da mudança de significado entre versões: claim adicionado, qualificado, contestado, substituído ou depreciado; evidência adicionada/removida; dependente invalidado; artefato tornado obsoleto.

Diff estrutural de linhas ou registros não substitui `EpistemicDiff`.

### ArtifactRevision

Versão de uma visão materializada, ligada ao commit do estado canônico e às dependências usadas para renderização.

## 5. Execução durável

### OperationalState

Estado de execução de `Inquiry` ou `Operation`, como `READY`, `RUNNING`, `WAITING_TIME`, `THROTTLED`, `BLOCKED_DEPENDENCY`, `VERIFYING`, `SUCCEEDED`, `EXHAUSTED`, `FAILED` ou `CANCELLED`.

### Lease

Direito temporário e persistido de um worker executar uma operação. Expiração de lease MUST permitir recuperação sem presumir que o efeito externo não ocorreu.

### IdempotencyKey

Identificador estável de uma intenção lógica usado para impedir efeitos duplicados em retries e recuperação.

### Event

Fato imutável de execução ou domínio registrado no log. Um evento informa que algo foi observado/decidido; não é comando para repetir a ação descrita.

### OutboxRecord

Intenção durável de publicar ou entregar um evento após commit local. É confirmada na mesma transação que a mudança de estado correspondente.

### EvidenceReceipt

Evidência operacional de uma ação determinística: resposta HTTP, hash, resultado de validação, transição persistida ou outro recibo auditável. Não é sinônimo de `EvidenceLink`, embora possa originar observações.

### FailureRecord

Registro imutável e tipado de uma tentativa malsucedida. Separa código estável, classe, locus, disposição de recuperação, certeza do efeito, escopo, detalhe seguro e recibos. Mensagem de erro ou status de provider MUST NOT controlar retry diretamente. A taxonomia normativa está em `FAILURE_TAXONOMY.md`.

### Quarantine

Isolamento persistido de entrada, artifact ou estado que não pode ser aceito com segurança. Um objeto em quarentena preserva proveniência e diagnóstico, mas MUST NOT integrar o `KnowledgeState` nem satisfazer precondições até liberação explícita e revalidação.

### ContinuityBlocked

Condição degradada e explicitamente diagnosticada em que uma missão `ACTIVE` não conseguiu encontrar nenhuma operação segura e útil entre as famílias autorizadas, ou todas as capabilities necessárias estão indisponíveis. Não é repouso normal nem conclusão. Deve registrar estratégias tentadas, recursos bloqueados, último delta, alternativas eliminadas e condição concreta de recuperação ou intervenção.

### Rest

Termo removido do estado global. Uma missão `ACTIVE` não repousa. Usar estados locais `WAITING_TIME`, `WAITING_EVENT`, `WAITING_APPROVAL`, `THROTTLED` ou `BLOCKED_DEPENDENCY` para unidades específicas; usar `PAUSED` somente por comando autorizado e `ContinuityBlocked` para incapacidade global anormal.

## 6. Termos substituídos ou restritos

### Goal

Termo genérico anterior. SHOULD NOT ser usado como entidade central do domínio. Usar:

- `MissionSpec` para orientação autorizada de longo prazo;
- `Question` para lacuna epistemológica;
- `Inquiry` para investigação admitida;
- `Operation` para execução estreita.

### WorkItem e Task

Termos genéricos anteriores. SHOULD NOT aparecer em novos contratos do núcleo. Quando necessário em explicação informal, devem mapear explicitamente para `Inquiry` ou `Operation`.

### MemoryStore

Termo amplo demais. Substituir por portas explícitas, por exemplo `MissionRepository`, `KnowledgeRepository`, `AgendaRepository`, `EventLog`, `ArtifactStore` e `Outbox`.

### Capability

Adapter autorizado e tipado para uma interação externa ou operação privilegiada. No MVP experimental, capabilities são estreitas e limitadas a aquisição/leitura de fontes, armazenamento gerenciado, modelo e renderização. `shell.run` e execução arbitrária não fazem parte do escopo inicial.

### ModelDecision

Saída proposta por modelo. MUST NOT ser chamada simplesmente de `Decision` quando puder ser confundida com decisão oficial do runtime. A autoridade final pertence a política, validação e commit determinísticos.

## 7. Relações essenciais

```text
MissionRevision
  ├─ authorizes → Inquiry
  └─ constrains → OperationSpec

Question
  └─ motivates → InquiryCandidate → Inquiry → Operation

Source → SourceVersion → SourceFragment → Observation
Observation → Claim ← EvidenceLink
Claim → Inference → KnowledgeArtifact

Operation
  └─ proposes → ProposedChangeSet
ProposedChangeSet → AcceptedChangeSet → Commit → KnowledgeState(version n+1)
```

## 8. Regra de nomeação em código

- Tipos persistidos usam nomes completos (`ProposedChangeSet`, não `Patch`).
- IDs usam prefixo por entidade apenas para legibilidade; identidade não depende do prefixo.
- Status operacionais e epistemológicos são enums distintos.
- Falhas usam `FailureRecord` e códigos da taxonomia; não criar política baseada em comparação de mensagens.
- `Evidence` sem qualificador não deve ser usado em APIs: preferir `EvidenceLink`, `EvidenceReceipt` ou `SourceFragment` conforme o caso.
- `CommitID` representa a versão lógica do domínio; adapters podem associá-lo a commit Dolt, sequência SQLite ou outra implementação.
