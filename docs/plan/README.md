# EON v2 — Pacote de Planejamento

> ⚠️ **Registro histórico congelado** (promovido a `docs/` no M0): este pacote é a fonte ORIGINAL do plano (D01–D55) e permanece intacto como referência. Decisões VIVAS agora nascem em `docs/adr/` (processo D48). Fronteiras e layout vigentes estão em `docs/ARCHITECTURE.md`; a lei do desenvolvimento em `docs/CONSTITUTION.md`.

> **Status**: **ESTÁVEL / CONGELADO** — **55 decisões (D01–D55)**, passada de consistência concluída em 2026-10-13 (rodada 5: deploy via Portainer) · mudanças futuras de design só por ADR (D48) · em execução: **M0**
> **Regra de ouro**: nada é implementado antes de o plano ser iterado, refinado e declarado estável. Este pacote é a fonte única de verdade do EON v2 até ser promovido a `docs/` na branch `v2` (Marco 0).

## O que é o EON v2

Um autômato em Go que **nunca para** (continuidade é propriedade do sistema, não de um processo), trabalha em **três frentes modulares** intercaladas com execução cognitiva **serial**, **melhora a si mesmo** de forma graduada, reversível e falha-segura versionando no próprio git, **calibra o harness à capacidade do modelo** disponível (do 70B ao free-tier mais básico), e opera com **zero intervenção humana obrigatória**, permanecendo 100% observável e interferível em tempo real.

## Índice

| Doc | Conteúdo |
|---|---|
| [00-DECISIONS.md](00-DECISIONS.md) | Registro de decisões fundacionais — fechadas e abertas |
| [01-THESIS.md](01-THESIS.md) | Tese, definição do produto, invariantes, não-objetivos |
| [02-ARCHITECTURE.md](02-ARCHITECTURE.md) | Blueprint: camadas, processos, módulos, deploy, sandbox |
| [03-LOOP.md](03-LOOP.md) | O loop multiciclo (C0–C4) e o scheduler |
| [04-SCIENCE.md](04-SCIENCE.md) | Fundamentação científica e referências |
| [05-DOMAIN-AND-PERSISTENCE.md](05-DOMAIN-AND-PERSISTENCE.md) | Modelo de domínio mínimo e persistência SQLite |
| [06-COGNITION.md](06-COGNITION.md) | Cognição adaptativa: calibração, ladder de formatos, microturnos |
| [07-SELF-IMPROVEMENT.md](07-SELF-IMPROVEMENT.md) | Pipeline de auto-melhoria F3: segurança, reversibilidade, git |
| [08-FRONTS.md](08-FRONTS.md) | Frentes de trabalho F1/F2/F3 e o contrato WorkFront |
| [09-CONSTITUTION.md](09-CONSTITUTION.md) | Constituição do desenvolvimento IA: regras, anti-padrões, skills |
| [10-MILESTONES.md](10-MILESTONES.md) | Roadmap M0–M5, critérios de aceite, riscos |

## Como iterar sobre este plano

1. Todo ajuste de plano acontece **aqui** — nunca em conversa apenas. Se mudamos uma decisão, o `00-DECISIONS.md` muda junto.
2. Cada documento tem seção **"Perguntas abertas"** no fim — é a fila de iteração.
3. Quando uma pergunta aberta é resolvida em conversa, ela vira decisão registrada e sai da fila.
4. **O plano está congelado (D48)**: as **55 decisões** estão fechadas (rodada 5 adicionou D49–D55: deploy via Portainer) e a passada de consistência foi concluída. Os únicos itens adiados são A09 (pesquisa da F1) e A30 (tuning empírico no M2).
5. Mudanças de design a partir de agora só por **ADR** (a partir do M0, este pacote é promovido a `docs/` e os ADRs assumem o papel de registro de decisões).
6. Só então começa o Marco 0 (branch `v2`, constituição, ADRs formais, esqueleto).

## Fila de iteração (próximos aprofundamentos)

- [x] Definir o formato exato da `Lesson` mecânica → **D24**
- [x] Detalhar o design do bandit do scheduler → **D26**
- [x] Detalhar o protocolo de calibração pass^k → **D27**
- [x] Definir métricas exatas da janela de observação da F3 e thresholds de rollback → **D25**
- [x] Especificar o boundary checker (regras de import enforçáveis em CI) → **D32**
- [ ] Desenhar o schema SQL completo (DDL por agregado) — tarefa do M0 (estratégia de índices e paginação já definida: D44)
- [x] Especificar as skills OpenCode (formato, gatilhos, checklists) → **D38/D39**
- [x] Confirmar lista de leitura semente da F1 (papers do 04-SCIENCE.md) e como citar → lista confirmada no 04-SCIENCE.md; convenção de citação = **D36**
- [x] Decidir política de compactação/crescimento do SQLite (Ciclo 2/F2) → **D30**
- [x] Detalhar o protocolo de emergência de storage (falha fatal ≠ parada) → **D31**

## Relação com o EON v1

Este pacote planeja a **reescrita completa**. O código v1 permanece intacto na `main` como referência/arquivo morto. O que a v1 nos deixa de valor: a tese documentada, o vocabulário de falha de modelos fracos (WEAK_MODEL_PROTOCOL), o conhecimento empírico de providers (results/), os padrões de teste (contract suites, crash-replay) e a autópsia de processo que virou a constituição (09-CONSTITUTION.md).
