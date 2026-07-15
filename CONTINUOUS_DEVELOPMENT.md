# Programa de Desenvolvimento Contínuo

Status: ativo

## Missão operacional

Transformar incrementalmente os documentos de pesquisa em um runtime epistemológico executável, testável e recuperável em Go, sem antecipar decisões ainda não validadas.

Cada heartbeat executa normalmente um lote de 2 a 4 melhorias relacionadas. Um único item pode ocupar o ciclo apenas quando for substancial — por exemplo, implementação acompanhada de testes, investigação comparativa ou correção estrutural ampla. O estado deste arquivo é a coordenação persistente entre ciclos.

## Restrições aceitas

- núcleo em Go;
- APIs OpenAI-compatible por adapter desacoplado;
- contrato mínimo de modelo `text → text`;
- modelo sem autoridade sobre estado ou capabilities;
- kernel determinístico e estado persistente;
- continuidade sem busy loop;
- conhecimento rastreável por fontes, observações, claims e evidências;
- alterações oficiais somente por `ProposedChangeSet` validado;
- Dolt condicionado a spike comparativo;
- automação geral fora do MVP.

## Definição de lote concluído

Um lote é concluído somente quando possui:

1. objetivo comum delimitado;
2. normalmente 2 a 4 melhorias observáveis e relacionadas;
3. integração coerente entre documentos, contratos, código e testes afetados;
4. verificação executada para o conjunto;
5. documentação das decisões ou contratos afetados;
6. um ou mais commits atômicos, salvo se o resultado for apenas investigação inconclusiva registrada.

Exemplos de lotes adequados:

- corrigir duas contradições, adicionar o requisito normativo correspondente e atualizar rastreabilidade;
- definir uma interface Go, implementar o fake e criar contract tests;
- pesquisar duas alternativas, registrar evidências e atualizar um ADR sem ainda aceitá-lo;
- implementar uma transição de estado, testes de tabela, fuzz test e documentação da invariante.

Não contam como várias melhorias mudanças cosméticas repetidas, subdivisões artificiais do mesmo parágrafo ou arquivos sem conteúdo verificável.

## Ordem de desenvolvimento

### Fase 0 — coerência da especificação

- [ ] `READY` Auditar documentos atuais em busca de contradições e resíduos do antigo MVP de automação/programação.
- [ ] `READY` Criar glossário normativo para Mission, Inquiry, Operation, ChangeSet, Claim, Evidence, Commit e Artifact.
- [ ] `READY` Consolidar requisitos funcionais e não funcionais com IDs rastreáveis.
- [ ] `READY` Criar taxonomia inicial de falhas específica do runtime epistemológico.
- [ ] `READY` Formalizar invariantes de autoridade, continuidade, segurança e progresso.

### Fase 1 — esqueleto Go determinístico

- [ ] `BLOCKED_BY:Fase0` Inicializar módulo Go sem framework e definir layout mínimo.
- [ ] `BLOCKED_BY:Fase0` Implementar tipos de domínio sem dependências externas.
- [ ] `BLOCKED_BY:Fase0` Implementar máquina de estados pura e testes de tabela.
- [ ] `BLOCKED_BY:Fase0` Implementar `Clock`, `IDGenerator` e `RandomSource` injetáveis.
- [ ] `BLOCKED_BY:Fase0` Implementar store em memória com contract tests.
- [ ] `BLOCKED_BY:Fase0` Implementar event log em memória e idempotência básica.

### Fase 2 — primeiro vertical slice simulado

- [ ] `BLOCKED_BY:Fase1` Carregar e validar `MissionSpec` versionada.
- [ ] `BLOCKED_BY:Fase1` Persistir pergunta, operação e condição de retomada.
- [ ] `BLOCKED_BY:Fase1` Implementar servidor OpenAI-compatible falso para testes.
- [ ] `BLOCKED_BY:Fase1` Implementar provider mínimo Chat Completions texto→texto.
- [ ] `BLOCKED_BY:Fase1` Compilar um `OperationSpec` sob budget.
- [ ] `BLOCKED_BY:Fase1` Produzir, validar e aplicar `ProposedChangeSet` atomicamente.
- [ ] `BLOCKED_BY:Fase1` Simular crash e comprovar retomada sem efeito duplicado.
- [ ] `BLOCKED_BY:Fase1` Comprovar repouso sem busy loop usando relógio virtual.

### Fase 3 — operações epistemológicas mínimas

- [ ] `BLOCKED_BY:Fase2` Ingerir uma fonte fixture imutável.
- [ ] `BLOCKED_BY:Fase2` Segmentar com round-trip verificável.
- [ ] `BLOCKED_BY:Fase2` Propor observação ancorada.
- [ ] `BLOCKED_BY:Fase2` Propor claim e vínculo de evidência.
- [ ] `BLOCKED_BY:Fase2` Gerar uma visão citada a partir do estado canônico.
- [ ] `BLOCKED_BY:Fase2` Atualizar a visão por patch após delta de evidência.

### Fase 4 — persistência real e spike de Dolt

- [ ] `BLOCKED_BY:Fase2` Fixar suite de contract tests de armazenamento.
- [ ] `BLOCKED_BY:Fase2` Implementar cenário comparável em Dolt.
- [ ] `BLOCKED_BY:Fase2` Implementar cenário comparável em SQLite + event log.
- [ ] `BLOCKED_BY:Fase2` Medir footprint, latência, recuperação, diff e complexidade.
- [ ] `BLOCKED_BY:Fase2` Registrar ADR final do backend.

### Fase 5 — fontes reais e avaliação cognitiva

- [ ] `BLOCKED_BY:Fase3,Fase4` Adapter de busca web com fixtures e replay.
- [ ] `BLOCKED_BY:Fase3,Fase4` Aquisição segura com limites de bytes e tipos.
- [ ] `BLOCKED_BY:Fase3,Fase4` Matriz de compatibilidade OpenAI-compatible.
- [ ] `BLOCKED_BY:Fase3,Fase4` Benchmark 2k/4k/8k para operações selecionadas.
- [ ] `BLOCKED_BY:Fase3,Fase4` Avaliar extração, síntese, conflito e reparo por modelo/formato.

## Política de seleção

Entre itens `READY`, escolher nesta ordem:

1. inconsistência que possa contaminar implementação;
2. contrato necessário ao próximo vertical slice;
3. teste que exponha uma hipótese importante;
4. implementação mínima coberta por testes;
5. pesquisa necessária para decisão bloqueada;
6. melhoria editorial com ganho real de precisão.

## Registro de ciclos

Adicionar entradas curtas somente quando houver mudança ou bloqueio relevante:

```text
YYYY-MM-DD HH:MM — ITEM — RESULTADO — VERIFICAÇÃO — COMMIT/NEXT
```

Não transformar este arquivo em log detalhado; Git contém o histórico completo.
