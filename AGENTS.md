# AGENTS.md — EON v2

Primeira leitura obrigatória de qualquer IA que trabalhe neste repositório. A lei completa do desenvolvimento está em **[docs/CONSTITUTION.md](docs/CONSTITUTION.md)**; a lei do produto em **[docs/INVARIANTS.md](docs/INVARIANTS.md)**. Este arquivo é o índice e o resumo operacional.

## O que é isto

EON v2: autômato epistêmico autônomo em Go — nunca para, três frentes (F1 projeto / F2 pensamentos / F3 self), execução cognitiva serial, auto-melhoria graduada-falha-segura-reversível, para modelos fracos/free-tier, zero intervenção humana obrigatória. A v1 morreu de processo (554 "fases", 158 clones, testes reescritos para passar) — a Constituição é a autópsia transformada em proibições.

## Como trabalhar aqui (ordem)

1. **Gate antes de qualquer "pronto"**: skill `eon-verify` — build+test+vet+fmt+boundary. O gate é o único juiz de "verificado".
2. **Passos pequenos**: skill `eon-step` — um objetivo por passo, origem citada (Dxx/ADR), commit atômico conventional.
3. **Decisão estrutural**: skill `eon-adr` PRIMEIRO (conceito de domínio, fronteira, storage, protocolo, dependência externa).
4. **Entrega/incidente/deploy**: skill `eon-journal` para registrar.
5. **Experimento com modelo real**: skill `eon-campaign`.
6. **Consultar o plano histórico**: skill `eon-plan` (docs/plan/ é congelado — leitura, nunca edição).

## Fronteiras de módulo (resumo — a lei é `tools/boundary/rules.json`)

`domain` só stdlib · `port` importado por quem implementa · `core` nunca importa frentes/infra · frentes nunca se importam · `supervisor` stdlib+statefile (o core é processo, não import) · dependência externa = ADR (hoje: **zero deps**, CGO off, binário estático).

## Invariantes invioláveis do produto

Nunca para (invariante 1) · serialidade cognitiva (5) · sandbox cego sem rede/segredos (6) · falha derruba a tentativa, nunca o autômato (3) · modelo propõe, código determinístico decide (2). Texto completo: docs/INVARIANTS.md.

## Proibições canônicas (a autópsia como autoridade)

Nunca: commitar binários/artefatos/dumps · `cmd/phaseNNN` · editar teste para código passar · god packages · segredo em flag/prompt/commit · estado canônico fora do kernel · journal gigante · docs mentirosos (contrato mudou = docs mudam NO MESMO commit).

## Estado atual

Branch `v2` (orphan; `main` = arquivo da v1). Em execução: **M0 — Fundação** (critérios em `docs/plan/10-MILESTONES.md`). Deploy via Portainer (D49–D55) com verificação fora-de-banda (runbook). **Não** faça push para origin sem autorização explícita do operador; **não** toque no stack Portainer até a imagem-casca existir e o operador autorizar o deploy.
