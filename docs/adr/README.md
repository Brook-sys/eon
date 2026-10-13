# ADRs — Registro de Decisões Arquiteturais

> Decisões VIVAS do EON v2. Toda decisão estrutural (conceito de domínio, frente, fronteira de módulo, storage, protocolo) tem ADR **antes** do código (Constituição Art. 3). Os ADRs fundacionais (0001–0014) promovem as decisões D01–D55 do pacote histórico `docs/plan/00-DECISIONS.md` (D48).

- Processo: skill `eon-adr` (`.opencode/skills/eon-adr/SKILL.md`)
- Template: [ADR-0000-template.md](ADR-0000-template.md)
- Estados: `proposto` → `aceito` → (`superseded-by ADR-XXXX`)

## Índice

| ADR | Decisão | Origem |
|---|---|---|
| [0001](ADR-0001.md) | Go único; binário estático único; três modos de processo | D01, D07, D42 |
| [0002](ADR-0002.md) | Módulos compile-time; fronteiras por checker AST; teto de diff | D02, D32, D45 |
| [0003](ADR-0003.md) | SQLite híbrido: tabelas + events + outbox; compactação; falha fatal | D03, D30, D31, D44 |
| [0004](ADR-0004.md) | Três camadas de processos L1/L2/L3; binários bons com SHA-256 | D06, D07, D41 |
| [0005](ADR-0005.md) | Domínio mínimo de 13 conceitos com vocabulário novo | D04, D33 |
| [0006](ADR-0006.md) | Modelo OpenAI-compat único; piso 7B; calibração e dialetos | D05, D27, D29 |
| [0007](ADR-0007.md) | Loop multiciclo serial C0–C4; sleep-time compute | D10, D12, D22, D28 |
| [0008](ADR-0008.md) | Scheduler bandit UCB1 com piso de justiça | D26 |
| [0009](ADR-0009.md) | Lessons mecânicas SOFT→HARD por corroboração | D24 |
| [0010](ADR-0010.md) | Auto-melhoria graduada, falha-segura, reversível (F3) | D16–D19, D25, D34, D35, D37 |
| [0011](ADR-0011.md) | Sandbox cego puro-Go com degradação full→basic→none | D09, D19 |
| [0012](ADR-0012.md) | Operador zero-obrigatório; PendingDemands; CLI primeiro | D14, D15 |
| [0013](ADR-0013.md) | Missões e frentes: 3 frentes intercaladas, F1 estuda o domínio | D13, D20, D21, D23 |
| [0014](ADR-0014.md) | Deploy e continuidade via Portainer; watchdog sidecar | D40, D47, D49–D55 |
| [0015](ADR-0015.md) | Catálogo multi-provedor/multi-modelo com discovery e seleção por capacidade | operador 2026-10-13; estende D05, D10, D27, D29 |
