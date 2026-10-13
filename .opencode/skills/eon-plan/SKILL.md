---
name: eon-plan
description: Guia de LEITURA do pacote de planejamento histórico do EON v2 em docs/plan/ (D01–D55, congelado). Use para localizar a decisão/documento-fonte de um conceito antes de citá-lo. NUNCA edite docs/plan/ — decisões vivas nascem em docs/adr/ (skill eon-adr).
---

# eon-plan — mapa do registro congelado

`docs/plan/` é **histórico congelado** (D48): a fonte original do plano. Você o LÊ para citar com fidelidade; nunca o edita. Decisão nova ou mudança = ADR (`docs/adr/`).

## Mapa de consulta rápida

| Quero saber sobre... | Leia |
|---|---|
| Decisões D01–D55 (número → texto exato) | `00-DECISIONS.md` |
| Invariantes e a definição de uma frase | `01-THESIS.md` (promovido: `docs/INVARIANTS.md`) |
| Camadas L1/L2/L3, sandbox, layout, git, deploy | `02-ARCHITECTURE.md` (promovido: `docs/ARCHITECTURE.md`) |
| Loop multiciclo C0–C4, scheduler, orçamento | `03-LOOP.md` |
| Ciência da missão, lista de leitura semente | `04-SCIENCE.md` |
| Domínio (13 conceitos), schema, storage | `05-DOMAIN-AND-PERSISTENCE.md` |
| Calibração, dialetos, ladder de formatos | `06-COGNITION.md` |
| Auto-melhoria F3, janela, Darwin Gödel Machine | `07-SELF-IMPROVEMENT.md` |
| Frentes F1/F2/F3 e contratos | `08-FRONTS.md` |
| Lei do desenvolvimento (autópsia da v1) | `09-CONSTITUTION.md` (promovido: `docs/CONSTITUTION.md`) |
| Marcos M0–M5 e critérios de aceite | `10-MILESTONES.md` |

## Regras de uso

1. **Cite por número com texto verificado**: "D26 (UCB1 com janela descontada)", nunca "acho que foi D26". Se o número não bate com o texto, o arquivo é a autoridade — e algo está errado em quem citou.
2. **Questões abertas (Axx)** são fila de iteração, pendências reconhecidas — não usar como desculpa para decidir por conta própria sem ADR.
3. **Divergência plano ↔ código**: o código está errado OU o docs vivo não foi atualizado no commit — os dois casos são violação (Constituição Art. 8). Resolver no docs VIVO (`docs/ARCHITECTURE.md` etc.), jamais no histórico.
4. Lendo para o autômato (F3): os checklists vivos estão nas skills (`.opencode/skills/`) e docs vivos — o pacote histórico é citação, não instrução de trabalho.
