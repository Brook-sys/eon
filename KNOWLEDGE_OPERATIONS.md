# Operações de Evolução do Conhecimento

Status: rascunho v0.1

## 1. Princípio

O runtime não “melhora um texto” como uma operação indivisível. Ele executa transformações epistemológicas tipadas sobre um estado versionado.

```text
KnowledgeOperation
  = preconditions
  + bounded read set
  + proposed change set
  + validation policy
  + epistemic effect
  + provenance
  + rollback path
```

Uma operação pode produzir um documento melhor sem melhorar a base canônica. Também pode melhorar a base sem alterar imediatamente qualquer documento. Essas duas camadas precisam permanecer separadas.

## 2. Camadas de dados

### L0 — Fonte imutável

- snapshots, hashes, metadados e fragmentos;
- nunca reescrita pelo modelo;
- pode receber nova versão da mesma fonte.

### L1 — Observações extraídas

- representação fiel do que a fonte declara;
- ancorada em fragmentos;
- corrigível por nova revisão, preservando histórico.

### L2 — Estado epistemológico

- claims;
- evidências;
- conflitos;
- qualificadores;
- inferências;
- perguntas e hipóteses;
- dependências e status.

### L3 — Visões sintetizadas

- resumos;
- mapas de literatura;
- relatórios;
- planos;
- respostas;
- documentos temáticos.

L3 é regenerável a partir de L0–L2. Não deve ser usado como fonte independente sem seguir suas dependências até a evidência original.

### L4 — Memória procedural

- estratégias de busca eficazes;
- templates;
- regras de normalização;
- falhas e correções recorrentes;
- perfis de modelo;
- testes de regressão.

Mudanças em L4 exigem avaliação própria e não podem ser promovidas apenas porque melhoraram uma execução.

## 3. Taxonomia de operações

### 3.1 `INGEST_SOURCE`

Adquire e identifica uma fonte.

**Produz:** `Source`, versão, hash, snapshot e metadados.

**Não autoriza:** aceitar o conteúdo como verdadeiro.

### 3.2 `SEGMENT_SOURCE`

Cria fragmentos endereçáveis sem alterar o conteúdo.

**Validação:** cobertura, ordem, offsets e round-trip para o original.

### 3.3 `EXTRACT_OBSERVATIONS`

Propõe observações estritamente ancoradas em fragmentos.

**Risco principal:** paráfrase acrescentar inferência inexistente.

**Controle:** cada observação precisa de localização e comparação textual.

### 3.4 `ATOMIZE_CLAIMS`

Divide uma observação complexa em proposições contestáveis.

**Risco:** perder população, tempo, modalidade, negação ou condição experimental.

**Controle:** qualificadores obrigatórios e vínculo à observação integral.

### 3.5 `LINK_EVIDENCE`

Propõe o tipo de relação entre fragmento/observação e claim.

**Importante:** citar um claim não implica apoiá-lo; `MENTIONS` é diferente de `SUPPORTS`.

### 3.6 `NORMALIZE_CONCEPT`

Relaciona termos equivalentes, mais amplos, mais estreitos ou apenas semelhantes.

**Risco:** fundir conceitos cuja distinção é relevante.

**Controle:** alias não é identidade; fusão exige política mais forte que associação.

### 3.7 `DETECT_DUPLICATE`

Produz candidatos a equivalência, subsunção ou sobreposição.

```text
EXACT_EQUIVALENT
SEMANTIC_EQUIVALENT_WITH_DIFFERENT_WORDING
SUBSUMES
OVERLAPS
DISTINCT
UNSURE
```

O modelo não executa a fusão. Um validador examina qualificadores e dependências.

### 3.8 `DETECT_CONFLICT`

Produz um candidato a conflito e sua possível explicação:

- contradição lógica;
- diferença temporal;
- população diferente;
- definição diferente;
- método diferente;
- versão diferente;
- incerteza ou erro de extração.

Contradição lexical não basta para marcar claims como contestados.

### 3.9 `SEARCH_GAP`

Transforma uma lacuna em estratégia de recuperação:

- subperguntas;
- termos e sinônimos;
- fontes preferidas;
- consultas positivas e de contraprova;
- critério de suficiência;
- limite de custo.

### 3.10 `UPDATE_CLAIM_STATUS`

Recalcula o estado de um claim diante de evidências adicionadas, removidas ou depreciadas.

Preferencialmente usa regras explícitas. O modelo pode explicar casos ambíguos, mas não decide sozinho.

### 3.11 `SYNTHESIZE_VIEW`

Gera uma visão materializada para uma pergunta e audiência específicas.

Uma síntese deve declarar:

- consulta e escopo;
- versão da missão;
- conjunto de claims considerado;
- política de seleção;
- data de corte;
- citações por afirmação relevante;
- conflitos e lacunas omitidos ou incluídos;
- nível de compressão.

### 3.12 `COMPRESS_VIEW`

Reduz uma visão sem alterar o estado canônico.

Critérios possíveis:

- limite de tokens;
- preservação de claims prioritários;
- preservação de divergências;
- preservação de citações;
- cobertura mínima por subtema.

Resumo de resumo deve ser evitado. A compressão deve consultar claims e fontes, não apenas a versão textual anterior.

### 3.13 `REFRESH_VIEW`

Atualiza somente partes afetadas por mudanças no grafo.

```text
commit novo
→ claims alterados
→ dependentes transitivos
→ seções afetadas
→ patch candidato
→ validação de cobertura e citação
```

Isso é preferível a reescrever o documento inteiro a cada ciclo.

### 3.14 `EXPAND_VIEW`

Adiciona detalhe porque a missão passou a exigir maior profundidade ou uma lacuna foi preenchida.

### 3.15 `RESTRUCTURE_VIEW`

Muda organização, taxonomia ou sequência explicativa sem declarar novos fatos.

Mudança editorial e mudança epistemológica devem aparecer separadamente no diff.

### 3.16 `CRITIQUE_COVERAGE`

Compara uma visão à missão, perguntas e claims disponíveis para identificar:

- pontos ausentes;
- excesso irrelevante;
- apoio fraco;
- desbalanceamento;
- conflitos escondidos;
- conteúdo desatualizado.

### 3.17 `DEPRECATE_KNOWLEDGE`

Marca objetos como obsoletos, superseded ou fora do escopo. Não apaga histórico.

### 3.18 `RECONCILE_MISSION_CHANGE`

Propaga uma nova revisão da missão sobre perguntas, agenda, claims e visões.

## 4. ChangeSet como unidade de escrita

Nenhuma chamada de modelo escreve diretamente no estado oficial. Ela gera um `ProposedChangeSet`:

```yaml
change_set_id: cs_...
base_commit: abc123
mission_revision: 8
operation: REFRESH_VIEW
read_set:
  claims: [c1, c2, c3]
  sources: [s1, s2]
preconditions:
  - c2.status == supported
changes:
  - update artifact_section section_4
epistemic_delta:
  added_claims: []
  removed_claims: []
  changed_support: []
validation_plan:
  - citation_entailment
  - no_unattributed_major_claim
  - preserve_contested_claims
generated_by:
  model: ...
  operation_spec: ...
status: PROPOSED
```

Ciclo:

```text
PROPOSED
→ STRUCTURALLY_VALID
→ EPISTEMICALLY_REVIEWED
→ ACCEPTED | REJECTED | NEEDS_RESEARCH
→ COMMITTED
```

## 5. Branches de trabalho

Branches podem isolar:

- uma investigação;
- atualização por novo conjunto de fontes;
- hipótese alternativa;
- reestruturação de taxonomia;
- alteração da missão;
- experimento de síntese.

Não devemos criar uma branch por microoperação automaticamente. Isso produziria ruído e conflitos artificiais. Uma branch deve corresponder a uma unidade revisável com propósito e condição de merge.

## 6. Merge semântico

Merge de conhecimento não é apenas merge de linhas ou células.

Casos:

1. **independente:** mudanças afetam subgrafos distintos;
2. **aditivo compatível:** novas evidências para o mesmo claim;
3. **concorrente editorial:** duas redações da mesma visão;
4. **conflito de identidade:** branches fundiram entidades de modos diferentes;
5. **conflito epistemológico:** uma promove e outra contesta um claim;
6. **conflito de missão:** branches derivam de revisões incompatíveis;
7. **conflito de deleção/depreciação:** uma depende de objeto depreciado pela outra.

O banco pode detectar conflito estrutural. A camada epistemológica precisa classificar e resolver o significado do conflito.

## 7. Métricas de uma operação de síntese

Uma síntese candidata pode ser avaliada por:

- `citation_precision`: citações realmente sustentam o trecho?
- `citation_coverage`: claims relevantes possuem citação?
- `claim_fidelity`: preserva qualificadores e modalidade?
- `mission_relevance`: responde à pergunta atual?
- `topic_coverage`: cobre dimensões obrigatórias?
- `conflict_visibility`: apresenta divergências materiais?
- `redundancy`: repete conteúdo sem ganho?
- `compression_ratio`: reduz tamanho quanto?
- `information_retention`: quais claims prioritários preserva?
- `freshness`: depende de claims vigentes?
- `novel_inference_rate`: quantas inferências novas foram introduzidas?

Algumas métricas exigem amostragem ou revisão humana. Não devem virar um único score opaco de “qualidade”.

## 8. Evitando degradação iterativa

O sistema nunca deve usar este loop como padrão:

```text
texto atual → “melhore” → novo texto → “melhore” → ...
```

Ele favorece drift, apagamento de detalhes e consenso artificial.

O loop correto é:

```text
estado canônico + missão + delta de evidência
→ identificar impacto
→ gerar patch localizado
→ validar contra claims e fontes
→ comparar métricas e diff
→ aceitar ou rejeitar
```

Periodicamente, pode haver regeneração integral da visão a partir do estado canônico para detectar acúmulo de artefatos editoriais.

## 9. Próximos experimentos

1. Reescrita integral versus refresh incremental.
2. Resumir texto anterior versus sintetizar do grafo canônico.
3. Modelo único versus extrator e revisor separados por protocolo.
4. Citações livres versus claims previamente selecionados.
5. Contraprova obrigatória versus busca apenas confirmatória.
6. Merge estrutural simples versus merge semântico classificado.
7. Modelos fortes e fracos sob o mesmo `OperationSpec`.
