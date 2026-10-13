# docs/knowledge/ — Base de conhecimento citada da F1

> Formato D36 (ADR-0013). A missão-semente da F1 (D21) estuda o próprio domínio — agentes contínuos, auto-melhoria, harnesses para modelos fracos — e publica aqui o resultado: **conhecimento com claims citadas byte-a-byte**, legível por humano e autômato, diffável no git. A partir do M2/M3 este diretório é escrito pelo autômato (via ChangeSets validados); antes disso, semente humana/planejamento.

## Estrutura (nasce com a primeira coleção)

```
docs/knowledge/
  README.md            ← este contrato
  claims-index.md      ← índice de claims ativas por tema (verificável por script)
  themes/<tema>.md     ← página por tema: claims → citações → artifacts (com hash)
  revisions.log        ← log de revisões (claim adicionada/revisada/retirada + por quê)
```

## Regras de forma

1. **Claim é atômica**: uma asserção verificável por página, com ID estável (`C-<tema>-<n>`).
2. **Toda claim cita byte-a-byte**: a citação exata mora num Artifact (com hash SHA-256 no repositório de artifacts); claim sem citação é rascunho, não conhecimento.
3. **Confiança é explícita**: corroborada / única-fonte / contestada. Contestada exige a contraparte citada.
4. **Índice verificável**: `claims-index.md` lista claim → tema → status → artifact; um script (futuro `tools/`) confere que toda claim indexada existe e toda claim existente está indexada.
5. **Revisão é evento**: nada é apagado silenciosamente; `revisions.log` é append-only.
6. **Relatórios de operação (D43)** moram em `reports/`: continuidade, promoções/rollbacks, calibração, custos — o estudo publica o experimento que ele mesmo é.

## Sementes

- Lista de leitura semente: `docs/plan/04-SCIENCE.md` (histórico, congelado).
- Questão aberta A09 (ID arXiv do paper "AI Parenting") é a primeira tarefa de pesquisa da F1 — o plano não depende dela.
- Lições de engenharia da v1 (autópsia): `docs/plan/00-DECISIONS.md` e a Constituição — insumo para as primeiras claims quando a F1 abrir.

## Estado atual

Vazio por desenho (M0): a F1 abre no M3 com o adapter+calibração prontos (ordem D46). Este README é o contrato de forma; o conteúdo vem do trabalho do autômato, não de geração antecipada.
