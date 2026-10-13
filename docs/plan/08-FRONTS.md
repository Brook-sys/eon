# 08 — Frentes de Trabalho (F1 / F2 / F3) e o Contrato WorkFront

## Contrato uniforme `WorkFront` (D20)

Cada frente é um módulo compile-time que implementa o mesmo contrato estreito:

```go
type WorkFront interface {
    // enumera candidatos de trabalho com proveniência, custo e ganho
    Enumerate(ctx FrontCtx) ([]Candidate, error)
    // executa uma Task como sequência de Steps (microturns)
    Execute(ctx FrontCtx, t Task, emit StepFn) (TaskResult, error)
    // verifica uma Proposal determinística ou por execução — nunca só o modelo
    Verify(ctx FrontCtx, p Proposal) (Evidence, error)
    // aprende com o outcome: Lessons + candidatos-filho
    Learn(ctx FrontCtx, r TaskResult) error
}
```

- O kernel conhece `WorkFront`, nunca implementações.
- Cada front recebe **só** os repositórios que usa (least-authority).
- Adicionar/evoluir uma frente nunca exige mudar o kernel.
- Sub-famílias internas por frente (ex.: F2-compressão, F2-reflexão) são configuração da frente, não mudanças de contrato.

## F1 — PROJETO (trabalho epistêmico sobre artefato-alvo)

**Primeira missão (D21): estudar o próprio domínio** — construir, continuamente, uma base de conhecimento citada sobre agentes contínuos, auto-melhoria e harnesses para modelos fracos.

- **Seeds**: a lista de referências do 04-SCIENCE.md (papers) + materiais locais do repo.
- **Fluxo típico de uma Task F1**: identificar pergunta/lacuna → coletar material (local; web opcional) → extrair observações citadas → formular claims com evidência → sintetizar/refinar documento do conhecimento → exportar markdown no repo (`docs/knowledge/`, diário público da pesquisa).
- **Citações byte-a-byte**: toda claim liga a Artifact com hash e trecho exato (herança boa da v1).
- **Export** (D36): `docs/knowledge/` = índice de claims ativas por tema + páginas por tema (claim → citação → artifact com hash) + log de revisões. Legível por humano e autômato, diffável no git, índice verificável por script.
- **Web (D23)**: fetch direto + search API free-tier via env, opcionais; degradação graciosa; conteúdo web = dado (injeção: ver 06).
- Recorrências: revisão periódica de cobertura (quais lacunas?), revalidação de claims antigos (NELL), síntese incremental.
- **Relatórios de operação** (D43): dados da própria operação (continuidade, promoções/rollbacks, calibração, custos) são publicados em `docs/knowledge/` — o estudo publica o experimento que ele mesmo é.

## F2 — PENSAMENTOS (curadoria da própria mente)

O autômato organiza o que sabe — a frente mais determinística (roda sem cota):

- **Memory stream**: observações/experiências/artifacts com timestamps.
- **Recuperação**: recência × importância × relevância (FTS5/BM25; embeddings depois).
- **Reflexões em árvore**: insights citados sobre observações, e sobre insights (Generative Agents), gerados no Ciclo 2.
- **Síntese de Lessons → regras** (ExpeL): consultáveis pelo scheduler.
- **Higiene**: compressão, deduplicação, revalidação de confiança (anti-NELL), esquecimento supervisionado — aprovado pelo próprio autômato via **policy versionada** (D35; checklist determinístico: idade, confiança, importância, uso, referências por lessons ativas). Mudança de estado com event, nunca DELETE silencioso; operador pode reverter via git.
- **Sleep-time compute (D12/D22)**: quando a cota esgota, a F2 **acelera** — o tempo sem modelo é tempo de consolidação.
- Curadoria da frontier (mapa QD: nichos frente × tema × dificuldade) também mora aqui como família.
- **Export da essência** (D31): snapshot incremental versionado — missão, lessons ativas, memórias de alta confiança — como markdown no repo git, a cada janela do Ciclo 2 com mudanças relevantes. É o backup vivo da continuidade (e a fonte de re-bootstrap numa falha fatal de storage).

## F3 — SELF (auto-melhoria)

Pipeline completo, níveis, observação, rollback, arquivo DGM e git flow: **ver 07-SELF-IMPROVEMENT.md** (documento dedicado).

Famílias internas da F3: análise de lessons acumuladas → proposta; melhoria de prompts (L0); políticas/parâmetros (L1); código (L2); elevação de autonomia (proposta especial); recalibração do modelo.

## Intercalação

O scheduler (03-LOOP.md) intercala as frentes por bandit com garantia mínima, modificado por orçamento, cotas, prazos e pendências. Missões distintas compartilham as frentes: a missão ativa define o alvo da F1 e as políticas; F2/F3 são da vida do autômato e servem todas as missões (D13).

## Perguntas abertas

- Nenhuma pendente neste documento — fila centralizada no [00-DECISIONS.md](00-DECISIONS.md).
