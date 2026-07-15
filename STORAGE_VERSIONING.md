# Armazenamento e Versionamento

Status: nota de decisão em investigação v0.1

## Questão

Como armazenar estado operacional, fontes, claims, evidências, dependências, mudanças e artefatos de modo auditável, recuperável e eficiente?

## Requisitos

1. transações atômicas para alterações relacionadas;
2. histórico imutável ou reconstruível;
3. diff entre revisões;
4. proveniência de cada alteração;
5. branches para mudanças experimentais ou revisáveis;
6. merge e conflitos explícitos;
7. consultas relacionais e por dependência;
8. snapshots e recuperação;
9. índices para busca textual e semântica;
10. exportação para formatos abertos;
11. operação local e gratuita;
12. baixo custo operacional no MVP;
13. separação entre tempo da fonte, tempo de validade e tempo de registro;
14. compatibilidade com event log e transactional outbox.

## Distinções temporais

Version control não substitui semântica temporal. Precisaremos ao menos distinguir:

- `source_published_at`: quando a fonte foi publicada;
- `source_observed_at`: quando o runtime a adquiriu;
- `claim_valid_from/to`: período ao qual a alegação se aplica, quando conhecido;
- `recorded_at`: quando entrou no sistema;
- `superseded_at`: quando deixou de ser a revisão vigente;
- `commit_id`: revisão lógica da base.

Isso se aproxima de modelagem bitemporal: tempo válido e tempo transacional. Um commit graph sozinho não responde “quando isto era verdadeiro no mundo?”.

## Opção A — Dolt como banco canônico

A documentação oficial descreve Dolt como banco relacional compatível com MySQL que armazena dados em um commit graph, possui múltiplas branches e permite diff e merge entre revisões. Operações de versionamento também são expostas por SQL.

### Vantagens esperadas

- commit, branch, diff, merge e histórico são nativos;
- SQL para dados estruturados;
- branches de investigação podem ser naturais;
- revisão de changesets de dados fica mais observável;
- rollback e comparação de estados são conceitos de primeira classe;
- reduz quantidade de infraestrutura própria para “Git for data”.

### Questões em aberto

- custo de operação e footprint para um runtime pequeno;
- desempenho com muitas microalterações e histórico extenso;
- concorrência e estratégia de commits;
- maturidade das bibliotecas e compatibilidade MySQL necessárias;
- conflitos de schema e migrações;
- backup, replicação e recuperação no ambiente-alvo;
- política adequada de branch/merge;
- como integrar busca full-text, vetorial e de grafos;
- granularidade de commit sem criar milhões de revisões inúteis;
- se merge estrutural nativo ajuda suficientemente o merge semântico.

## Opção B — SQLite + event log append-only + Git para artefatos

### Vantagens

- implantação mínima e biblioteca madura;
- transações locais fortes;
- fácil prototipagem;
- event log permite auditoria e reconstrução;
- Markdown e schemas podem permanecer em Git.

### Desvantagens

- branches e merges de dados precisam ser implementados ou evitados;
- diffs semânticos e snapshots exigem camada própria;
- projeções e migrações do event log aumentam complexidade;
- risco de criar uma versão inferior de recursos já oferecidos pelo Dolt.

## Opção C — PostgreSQL + temporal/event model + Git para artefatos

### Vantagens

- robustez operacional e ecossistema;
- concorrência, índices, full-text e extensões vetoriais;
- boa evolução para serviço multiusuário;
- modelagem temporal e auditoria podem ser explícitas.

### Desvantagens

- maior custo operacional inicial;
- branch/merge de dados não são semânticas nativas equivalentes ao Git;
- exige implementação de changesets, histórico e revisão.

## Opção D — armazenamento híbrido

Possível desenho:

```text
Dolt ou RDBMS
  estado canônico estruturado
  commits e mudanças

object store/diretório content-addressed
  snapshots e documentos brutos

Git
  schemas, templates, políticas e artefatos humanos

índice derivado
  full-text, embeddings e busca
```

Índices são projeções descartáveis e reconstruíveis. Não devem ser a fonte canônica.

## Recomendação provisória

Não fixar ainda a tecnologia. Adotar desde já uma interface de armazenamento e modelar `ChangeSet`, `Commit`, proveniência e tempos independentemente do backend.

Executar um spike comparando pelo menos:

1. Dolt;
2. SQLite com tabelas temporais/event log;
3. PostgreSQL, se o caso multiusuário entrar no horizonte próximo.

### Cenário do spike

O protocolo executável, dataset determinístico, crash harness subprocessado, métricas e critérios de decisão estão fixados em `STORAGE_SPIKE.md`. Em resumo:

- ingerir 1.000 fontes sintéticas;
- criar 10.000 claims e 30.000 relações;
- aplicar 1.000 changesets;
- criar branches de duas investigações;
- introduzir conflitos estruturais e epistemológicos;
- consultar histórico e impacto;
- reconstruir índices;
- provocar crash real antes/depois do commit durável e classificar estados parciais;
- medir tamanho, latência, recuperação, complexidade e clareza operacional sob suites comuns.

### Critérios de escolha

- correção e recuperabilidade;
- simplicidade total, não apenas número de linhas;
- qualidade de diff e revisão;
- custo de branch/merge;
- desempenho no workload real;
- portabilidade e exportação;
- manutenção do projeto e comunidade;
- facilidade de backup;
- integração estável com Go;
- possibilidade de migrar sem reescrever o modelo epistemológico.

## Política preliminar de commits

Um commit deve representar uma unidade epistemológica revisável, não cada token ou chamada.

Exemplos:

- aquisição e segmentação completa de uma fonte;
- lote validado de observações/claims de uma fonte;
- resolução de um conflito;
- atualização de uma síntese após changeset aceito;
- reconciliação de uma revisão de missão.

Cada commit registra:

- autor: usuário, runtime, importador ou revisão humana;
- operação e versão do protocolo;
- missão e revisão;
- IDs das fontes e tarefas;
- validadores executados;
- métricas antes/depois;
- justificativa curta;
- hash dos artefatos associados.

## Relação com padrões

O W3C PROV define proveniência em termos gerais de entidades, atividades e agentes, além de relações entre eles. Deve orientar interoperabilidade, mas provavelmente não substituirá o modelo específico de claims e evidências.

O desenho deve avaliar ainda:

- PROV-DM/PROV-O;
- nanopublications;
- SEPIO/ECO e ontologias de evidência;
- truth maintenance systems;
- temporal e bitemporal databases;
- event sourcing.

## Estado da decisão

`OPEN — DOLT IS A LEADING CANDIDATE, NOT YET SELECTED.`

A decisão está registrada como ADR proposto em `ADRS/0003-versioned-storage.md`.

Fonte inicial oficial sobre Dolt:

- https://www.dolthub.com/docs/sql-reference/version-control/
